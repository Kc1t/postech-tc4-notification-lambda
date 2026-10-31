// Package notifier consome os.ServiceOrderStatusChanged (fila notification-os-events, assinada
// no tópico os-events) e avisa o cliente por e-mail.
package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Kc1t/postech-tc4-notification-lambda/internal/mask"
	"github.com/Kc1t/postech-tc4-notification-lambda/internal/templates"
	"github.com/Kc1t/postech-tc4-notification-lambda/pkg/contracts"
	"github.com/aws/aws-lambda-go/events"
	"github.com/newrelic/go-agent/v3/newrelic"
)

// Sender envia o e-mail.
type Sender interface {
	Send(ctx context.Context, toEmail, toName string, m templates.Message) error
}

// Store reserva o message_id para não enviar duas vezes a mesma notificação.
type Store interface {
	Claim(ctx context.Context, id string) (bool, error)
	Release(ctx context.Context, id string) error
}

type Handler struct {
	sender    Sender
	store     Store
	portalURL string
	log       *slog.Logger
}

func New(sender Sender, store Store, portalURL string, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{sender: sender, store: store, portalURL: portalURL, log: log}
}

// errPermanent marca mensagens que não adianta reprocessar; elas vão para a DLQ depois do
// maxReceiveCount, para análise.
var errPermanent = errors.New("mensagem inválida")

// Handle processa o lote e devolve as mensagens que falharam (ReportBatchItemFailures): só elas
// voltam para a fila, o resto do lote é confirmado. A invocação devolve nil ao runtime, então a
// falha é marcada na transação (NoticeError) para aparecer na taxa de erro e no Errors Inbox.
func (h *Handler) Handle(ctx context.Context, ev events.SQSEvent) (events.SQSEventResponse, error) {
	acceptTrace(ctx, ev)
	var resp events.SQSEventResponse
	for _, r := range ev.Records {
		if err := h.process(ctx, r); err != nil {
			resp.BatchItemFailures = append(resp.BatchItemFailures, events.SQSBatchItemFailure{ItemIdentifier: r.MessageId})
			if txn := newrelic.FromContext(ctx); txn != nil {
				txn.NoticeError(err)
			}
		}
	}
	return resp, nil
}

// withTrace põe trace.id, span.id e entity.guid nos logs (logs in context do New Relic). Depois
// do acceptTrace o trace.id é o mesmo da requisição que mudou a OS no os-service.
func withTrace(ctx context.Context, l *slog.Logger) *slog.Logger {
	txn := newrelic.FromContext(ctx)
	if txn == nil {
		return l
	}
	md := txn.GetLinkingMetadata()
	if md.TraceID == "" {
		return l
	}
	return l.With("trace.id", md.TraceID, "span.id", md.SpanID, "entity.guid", md.EntityGUID)
}

func (h *Handler) process(ctx context.Context, r events.SQSMessage) error {
	log := withTrace(ctx, h.log).With("sqs_message_id", r.MessageId)
	env, err := contracts.Parse([]byte(unwrapSNS(r.Body)))
	if err != nil {
		log.Error("notification_invalid_message", "error", err)
		return fmt.Errorf("%w: %w", errPermanent, err)
	}
	log = log.With("message_id", env.MessageID, "message_type", env.Type, "saga_id", env.SagaID, "os_id", env.OSID)
	if env.Type != contracts.EvtServiceOrderStatusChanged {
		log.Info("notification_ignored", "reason", "tipo não notificado")
		return nil
	}
	evt, err := contracts.Decode[contracts.ServiceOrderStatusChanged](env)
	if err != nil {
		log.Error("notification_invalid_payload", "error", err)
		return fmt.Errorf("%w: %w", errPermanent, err)
	}
	log = log.With("service_order_code", evt.OSCode, "status", evt.Status)
	if !templates.Supports(evt.Status) {
		log.Info("notification_ignored", "reason", "status sem e-mail")
		return nil
	}
	if evt.Requester.Email == "" {
		log.Warn("notification_ignored", "reason", "cliente sem e-mail", "document", mask.Document(evt.Requester.Document))
		return nil
	}
	msg, err := templates.Render(templates.Data{
		OSCode: evt.OSCode, Name: evt.Requester.Name, Status: evt.Status, TotalAmount: evt.TotalAmount,
		CheckoutURL: evt.CheckoutURL, Reason: evt.Reason, PortalURL: h.portalURL,
	})
	if err != nil {
		log.Error("notification_render_failed", "error", err)
		return err
	}

	claimed, err := h.store.Claim(ctx, env.MessageID)
	if err != nil {
		log.Error("notification_dedup_failed", "error", err)
		return err
	}
	if !claimed {
		log.Info("notification_duplicate")
		return nil
	}
	to := mask.Email(evt.Requester.Email)
	if err := h.sender.Send(ctx, evt.Requester.Email, evt.Requester.Name, msg); err != nil {
		log.Error("notification_send_failed", "to", to, "error", err)
		if rerr := h.store.Release(ctx, env.MessageID); rerr != nil {
			log.Error("notification_release_failed", "error", rerr)
		}
		return err
	}
	log.Info("notification_sent", "to", to, "subject", msg.Subject)
	if txn := newrelic.FromContext(ctx); txn != nil {
		txn.Application().RecordCustomEvent("NotificationSent", map[string]any{
			"status": evt.Status, "serviceOrderCode": evt.OSCode, "messageId": env.MessageID,
		})
	}
	return nil
}

// acceptTrace liga a invocação ao trace distribuído que veio do os-service. Os headers viajam
// como MessageAttributes (raw delivery). Com batch_size = 1 há um trace por invocação.
func acceptTrace(ctx context.Context, ev events.SQSEvent) {
	txn := newrelic.FromContext(ctx)
	if txn == nil || len(ev.Records) == 0 {
		return
	}
	h := http.Header{}
	for _, k := range []string{"traceparent", "tracestate", "newrelic"} {
		if a, ok := ev.Records[0].MessageAttributes[k]; ok && a.StringValue != nil {
			h.Set(k, *a.StringValue)
		}
	}
	if len(h) > 0 {
		txn.AcceptDistributedTraceHeaders(newrelic.TransportQueue, h)
	}
}

// unwrapSNS aceita também o corpo no formato de notificação SNS (assinatura sem raw delivery).
func unwrapSNS(body string) string {
	var n struct {
		Type    string `json:"Type"`
		Message string `json:"Message"`
	}
	if json.Unmarshal([]byte(body), &n) == nil && n.Type == "Notification" && n.Message != "" {
		return n.Message
	}
	return body
}
