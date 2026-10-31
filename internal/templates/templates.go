// Package templates monta o e-mail de cada mudança de status da OS.
package templates

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
)

//go:embed html/*.html
var htmlFS embed.FS

// Data são os campos disponíveis nos templates.
type Data struct {
	OSCode      int
	Name        string
	Status      string
	TotalAmount float64
	CheckoutURL string
	Reason      string
	PortalURL   string
}

// Message é o e-mail pronto para envio.
type Message struct {
	Subject string
	Text    string
	HTML    string
}

type spec struct {
	subject string
	title   string
	body    string // texto do corpo; o mesmo vai para a versão HTML
}

// specs define o que o cliente recebe em cada status. awaiting_budget é etapa interna (o
// orçamento ainda está sendo gerado) e não gera e-mail.
var specs = map[string]spec{
	"received": {
		subject: "OS #{{.OSCode}} aberta",
		title:   "Recebemos seu veículo",
		body:    "Sua ordem de serviço #{{.OSCode}} foi aberta. Avisaremos quando o diagnóstico começar.",
	},
	"in_diagnosis": {
		subject: "OS #{{.OSCode}} em diagnóstico",
		title:   "Diagnóstico em andamento",
		body:    "Nossa equipe começou o diagnóstico do seu veículo (OS #{{.OSCode}}). Em seguida você recebe o orçamento.",
	},
	"awaiting_approval": {
		subject: "Orçamento da OS #{{.OSCode}}: {{money .TotalAmount}}",
		title:   "Seu orçamento está pronto",
		body: "O orçamento da OS #{{.OSCode}} ficou em {{money .TotalAmount}}. Aprove ou recuse pelo portal do cliente" +
			"{{if .PortalURL}} ({{.PortalURL}}){{end}}, entrando com o seu CPF.",
	},
	"awaiting_payment": {
		subject: "Pagamento da OS #{{.OSCode}}",
		title:   "Orçamento aprovado",
		body: "Obrigado pela aprovação. O valor é {{money .TotalAmount}}." +
			"{{if .CheckoutURL}} Pague pelo link seguro do Mercado Pago: {{.CheckoutURL}}{{else}} O link de pagamento chega em instantes.{{end}}",
	},
	"in_execution": {
		subject: "Pagamento confirmado: OS #{{.OSCode}} em execução",
		title:   "Mãos à obra",
		body:    "Recebemos o pagamento e o serviço da OS #{{.OSCode}} entrou na fila de execução.",
	},
	"finished": {
		subject: "OS #{{.OSCode}} finalizada",
		title:   "Seu veículo está pronto",
		body:    "O serviço da OS #{{.OSCode}} foi concluído. Seu veículo já pode ser retirado.",
	},
	"delivered": {
		subject: "OS #{{.OSCode}} entregue",
		title:   "Obrigado pela preferência",
		body:    "Registramos a entrega do veículo da OS #{{.OSCode}}. Qualquer dúvida, é só responder este e-mail.",
	},
	"cancelled": {
		subject: "OS #{{.OSCode}} cancelada",
		title:   "Ordem de serviço cancelada",
		body: "A OS #{{.OSCode}} foi cancelada{{if .Reason}}: {{.Reason}}{{end}}. " +
			"Se houve pagamento, o estorno é feito automaticamente pelo Mercado Pago.",
	},
}

var funcs = texttemplate.FuncMap{"money": Money}

var layout = htmltemplate.Must(htmltemplate.ParseFS(htmlFS, "html/layout.html"))

// Supports diz se o status gera e-mail.
func Supports(status string) bool {
	_, ok := specs[status]
	return ok
}

// Render monta o e-mail do status.
func Render(d Data) (Message, error) {
	sp, ok := specs[d.Status]
	if !ok {
		return Message{}, fmt.Errorf("templates: status sem e-mail: %q", d.Status)
	}
	subject, err := execText(sp.subject, d)
	if err != nil {
		return Message{}, err
	}
	body, err := execText(sp.body, d)
	if err != nil {
		return Message{}, err
	}
	greeting := "Olá"
	if first := strings.Fields(d.Name); len(first) > 0 {
		greeting += ", " + first[0]
	}
	text := fmt.Sprintf("%s!\n\n%s\n\n%s\n\nOficina Reservoir Devs\n", greeting, sp.title, body)

	var html bytes.Buffer
	if err := layout.Execute(&html, map[string]any{
		"Greeting": greeting, "Title": sp.title, "Body": body, "CheckoutURL": d.CheckoutURL,
		"ShowButton": d.Status == "awaiting_payment" && d.CheckoutURL != "", "Code": d.OSCode,
	}); err != nil {
		return Message{}, err
	}
	return Message{Subject: subject, Text: text, HTML: html.String()}, nil
}

func execText(src string, d Data) (string, error) {
	t, err := texttemplate.New("t").Funcs(funcs).Parse(src)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Money formata em reais: 1234.5 → R$ 1.234,50.
func Money(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	cents := int64(v*100 + 0.5)
	intPart, frac := cents/100, cents%100
	s := fmt.Sprintf("%d", intPart)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := fmt.Sprintf("R$ %s,%02d", b.String(), frac)
	if neg {
		out = "-" + out
	}
	return out
}
