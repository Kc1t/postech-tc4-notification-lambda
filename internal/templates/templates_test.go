package templates

import (
	"strings"
	"testing"
)

func TestRender_TodosOsStatus(t *testing.T) {
	for status := range specs {
		m, err := Render(Data{OSCode: 42, Name: "Ana Souza", Status: status, TotalAmount: 330})
		if err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if !strings.Contains(m.Subject, "42") || !strings.HasPrefix(m.Text, "Olá, Ana!") || !strings.Contains(m.HTML, "OS #42") {
			t.Errorf("%s: e-mail incompleto: %+v", status, m)
		}
	}
	if Supports("awaiting_budget") || !Supports("cancelled") {
		t.Fatal("Supports")
	}
}

func TestRender_PagamentoComLinkEBotao(t *testing.T) {
	m, err := Render(Data{OSCode: 7, Name: "Bruno", Status: "awaiting_payment", TotalAmount: 1234.5,
		CheckoutURL: "https://www.mercadopago.com.br/checkout/v1/redirect?pref_id=1&x=<y>"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.Text, "R$ 1.234,50") || !strings.Contains(m.Text, "pref_id=1") {
		t.Fatalf("texto: %s", m.Text)
	}
	if !strings.Contains(m.HTML, "Pagar com Mercado Pago") || strings.Contains(m.HTML, "<y>") {
		t.Fatal("HTML deve ter o botão e escapar a URL")
	}
	semLink, _ := Render(Data{OSCode: 7, Status: "awaiting_payment"})
	if strings.Contains(semLink.HTML, "Pagar com") || !strings.Contains(semLink.Text, "chega em instantes") {
		t.Fatal("sem link não mostra botão")
	}
	if !strings.HasPrefix(semLink.Text, "Olá!") {
		t.Fatal("sem nome usa saudação genérica")
	}
}

func TestRender_CanceladaComMotivoEAprovacaoComPortal(t *testing.T) {
	m, _ := Render(Data{OSCode: 9, Status: "cancelled", Reason: "orçamento recusado pelo cliente"})
	if !strings.Contains(m.Text, "cancelada: orçamento recusado") {
		t.Fatal(m.Text)
	}
	a, _ := Render(Data{OSCode: 9, Status: "awaiting_approval", TotalAmount: 10, PortalURL: "https://oficina.test"})
	if !strings.Contains(a.Subject, "R$ 10,00") || !strings.Contains(a.Text, "https://oficina.test") {
		t.Fatal(a.Subject, a.Text)
	}
	if _, err := Render(Data{Status: "awaiting_budget"}); err == nil {
		t.Fatal("status sem e-mail deve falhar")
	}
}

func TestMoney(t *testing.T) {
	for v, want := range map[float64]string{0: "R$ 0,00", 5.5: "R$ 5,50", 1000: "R$ 1.000,00",
		1234567.891: "R$ 1.234.567,89", -12: "-R$ 12,00"} {
		if got := Money(v); got != want {
			t.Errorf("Money(%v) = %q, esperava %q", v, got, want)
		}
	}
}
