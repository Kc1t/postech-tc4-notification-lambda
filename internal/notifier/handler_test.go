package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Kc1t/postech-tc4-notification-lambda/internal/templates"
	"github.com/Kc1t/postech-tc4-notification-lambda/pkg/contracts"
	"github.com/aws/aws-lambda-go/events"
	"github.com/newrelic/go-agent/v3/newrelic"
)

type sent struct{ to, name, subject string }

type fakeSender struct {
	err  error
	sent []sent
}

func (f *fakeSender) Send(_ context.Context, to, name string, m templates.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, sent{to, name, m.Subject})
	return nil
}

type fakeStore struct {
	claimed  map[string]bool
	err      error
	released []string
}

func (f *fakeStore) Claim(_ context.Context, id string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	if f.claimed[id] {
		return false, nil
	}
	f.claimed[id] = true
	return true, nil
}

func (f *fakeStore) Release(_ context.Context, id string) error {
	delete(f.claimed, id)
	f.released = append(f.released, id)
	return nil
}

func record(t *testing.T, id, msgType string, payload any) events.SQSMessage {
	t.Helper()
	e, err := contracts.NewEnvelope(msgType, "os-service", contracts.Meta{SagaID: "s1", OSID: "o1"}, payload)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := e.Marshal()
	tp := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	return events.SQSMessage{MessageId: id, Body: string(body), MessageAttributes: map[string]events.SQSMessageAttribute{
		"traceparent": {StringValue: &tp, DataType: "String"},
	}}
}

func status(s, email string) contracts.ServiceOrderStatusChanged {
	return contracts.ServiceOrderStatusChanged{
		OSCode: 42, Status: s, Requester: contracts.Requester{Name: "Ana Souza", Email: email, Document: "52998224725"},
		TotalAmount: 330, CheckoutURL: "https://mp.test/1",
	}
}

func newHandler() (*Handler, *fakeSender, *fakeStore) {
	snd, st := &fakeSender{}, &fakeStore{claimed: map[string]bool{}}
	return New(snd, st, "https://portal.test", nil), snd, st
}

func TestHandle_EnviaEFiltra(t *testing.T) {
	h, snd, _ := newHandler()
	app, _ := newrelic.NewApplication(newrelic.ConfigAppName("t"), newrelic.ConfigEnabled(false))
	txn := app.StartTransaction("lambda")
	ctx := newrelic.NewContext(context.Background(), txn)

	resp, err := h.Handle(ctx, events.SQSEvent{Records: []events.SQSMessage{
		record(t, "1", contracts.EvtServiceOrderStatusChanged, status("awaiting_payment", "ana@example.com")),
		record(t, "2", contracts.EvtServiceOrderStatusChanged, status("awaiting_budget", "ana@example.com")),
		record(t, "3", contracts.EvtBudgetCreated, contracts.BudgetCreated{}),
		record(t, "4", contracts.EvtServiceOrderStatusChanged, status("finished", "")),
	}})
	if err != nil || len(resp.BatchItemFailures) != 0 {
		t.Fatalf("%+v %v", resp, err)
	}
	if len(snd.sent) != 1 || snd.sent[0].to != "ana@example.com" || snd.sent[0].subject != "Pagamento da OS #42" {
		t.Fatalf("enviados: %+v", snd.sent)
	}
}

func TestHandle_DuplicadaNaoReenvia(t *testing.T) {
	h, snd, _ := newHandler()
	r := record(t, "1", contracts.EvtServiceOrderStatusChanged, status("finished", "ana@example.com"))
	for i := 0; i < 2; i++ {
		if resp, _ := h.Handle(context.Background(), events.SQSEvent{Records: []events.SQSMessage{r}}); len(resp.BatchItemFailures) != 0 {
			t.Fatal("duplicada não é falha")
		}
	}
	if len(snd.sent) != 1 {
		t.Fatalf("e-mail duplicado: %d", len(snd.sent))
	}
}

func TestHandle_FalhaParcialDoLote(t *testing.T) {
	h, snd, st := newHandler()
	snd.err = errors.New("smtp fora")
	good := record(t, "ok", contracts.EvtBudgetCreated, contracts.BudgetCreated{})
	bad := record(t, "falha", contracts.EvtServiceOrderStatusChanged, status("cancelled", "ana@example.com"))
	invalid := events.SQSMessage{MessageId: "lixo", Body: "{"}
	payload := record(t, "payload", contracts.EvtServiceOrderStatusChanged, nil)
	var e contracts.Envelope
	_ = json.Unmarshal([]byte(payload.Body), &e)
	e.Payload = []byte(`{"os_code":"x"}`)
	b, _ := e.Marshal()
	payload.Body = string(b)

	resp, _ := h.Handle(context.Background(), events.SQSEvent{Records: []events.SQSMessage{good, bad, invalid, payload}})
	if len(resp.BatchItemFailures) != 3 {
		t.Fatalf("falhas: %+v", resp.BatchItemFailures)
	}
	if len(st.released) != 1 || len(st.claimed) != 0 {
		t.Fatal("envio que falhou precisa liberar a reserva para a próxima tentativa")
	}

	st.err = errors.New("dynamo fora")
	snd.err = nil
	if resp, _ := h.Handle(context.Background(), events.SQSEvent{Records: []events.SQSMessage{bad}}); len(resp.BatchItemFailures) != 1 {
		t.Fatal("falha no dedup deve voltar para a fila")
	}
}

func TestUnwrapSNS(t *testing.T) {
	h, snd, _ := newHandler()
	r := record(t, "1", contracts.EvtServiceOrderStatusChanged, status("delivered", "ana@example.com"))
	wrapped, _ := json.Marshal(map[string]string{"Type": "Notification", "Message": r.Body})
	r.Body = string(wrapped)
	if resp, _ := h.Handle(context.Background(), events.SQSEvent{Records: []events.SQSMessage{r}}); len(resp.BatchItemFailures) != 0 || len(snd.sent) != 1 {
		t.Fatal("deve aceitar o envelope SNS")
	}
	if unwrapSNS("texto") != "texto" {
		t.Fatal("corpo comum passa direto")
	}
}
