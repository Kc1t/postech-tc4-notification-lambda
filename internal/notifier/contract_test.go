package notifier

// Contrato do consumidor: cada exemplo publicado no schema de os.ServiceOrderStatusChanged,
// entregue à Lambda como o corpo da mensagem SQS (raw delivery do tópico os-events), é aceito e
// vira o e-mail do status para o cliente do exemplo.

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/Kc1t/postech-tc4-notification-lambda/pkg/contracts"
	"github.com/aws/aws-lambda-go/events"
)

func TestContrato_ConsumidorAceitaOsExemplosDoStatusDaOS(t *testing.T) {
	examples, err := contracts.Examples(contracts.EvtServiceOrderStatusChanged)
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) == 0 {
		t.Fatal("schema sem exemplos")
	}
	subjects := map[string]string{
		"received":         "OS #%d aberta",
		"awaiting_payment": "Pagamento da OS #%d",
		"cancelled":        "OS #%d cancelada",
	}
	for i, body := range examples {
		env, err := contracts.Parse(body)
		if err != nil {
			t.Fatal(err)
		}
		evt, err := contracts.Decode[contracts.ServiceOrderStatusChanged](env)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(evt.Status, func(t *testing.T) {
			h, snd, st := newHandler()
			resp, err := h.Handle(context.Background(), events.SQSEvent{Records: []events.SQSMessage{
				{MessageId: "sqs-" + strconv.Itoa(i), Body: string(body)},
			}})
			if err != nil || len(resp.BatchItemFailures) != 0 {
				t.Fatalf("exemplo recusado: %+v %v", resp, err)
			}
			want, ok := subjects[evt.Status]
			if !ok {
				t.Fatalf("exemplo com status %q sem assunto esperado no teste", evt.Status)
			}
			if len(snd.sent) != 1 || snd.sent[0].to != evt.Requester.Email || snd.sent[0].name != evt.Requester.Name ||
				snd.sent[0].subject != fmt.Sprintf(want, evt.OSCode) {
				t.Fatalf("e-mail do exemplo: %+v", snd.sent)
			}
			if !st.claimed[env.MessageID] {
				t.Fatal("o message_id do exemplo é a chave de idempotência")
			}
		})
	}
}
