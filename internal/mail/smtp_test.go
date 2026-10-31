package mail

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-notification-lambda/internal/templates"
)

// fakeSMTP é um servidor SMTP mínimo para os testes (sem TLS, em localhost).
type fakeSMTP struct {
	ln       net.Listener
	mu       sync.Mutex
	commands []string
	data     string
	rejectTo bool
}

func startFake(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln}
	go f.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return f
}

func (f *fakeSMTP) port() string { _, p, _ := net.SplitHostPort(f.ln.Addr().String()); return p }

func (f *fakeSMTP) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeSMTP) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	say := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	say("220 fake")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimSpace(line)
		f.mu.Lock()
		f.commands = append(f.commands, cmd)
		f.mu.Unlock()
		switch up := strings.ToUpper(cmd); {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			say("250-fake\r\n250 AUTH PLAIN")
		case strings.HasPrefix(up, "AUTH"):
			say("235 ok")
		case strings.HasPrefix(up, "RCPT") && f.rejectTo:
			say("550 no such user")
		case strings.HasPrefix(up, "MAIL"), strings.HasPrefix(up, "RCPT"):
			say("250 ok")
		case up == "DATA":
			say("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.data = b.String()
			f.mu.Unlock()
			say("250 queued")
		case up == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

var msg = templates.Message{Subject: "Orçamento da OS #7", Text: "Olá, Ana!\nValor: R$ 10,00", HTML: "<p>Olá</p>"}

func TestSend(t *testing.T) {
	f := startFake(t)
	s := &SMTP{Host: "127.0.0.1", Port: f.port(), Username: "u", Password: "p", From: "oficina@example.com", FromName: "Oficina", Timeout: 2 * time.Second}
	if err := s.Send(context.Background(), "ana@example.com", "Ana\r\nBcc: x@evil", msg); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	joined := strings.Join(f.commands, "|")
	if !strings.Contains(joined, "AUTH PLAIN") || !strings.Contains(joined, "MAIL FROM:<oficina@example.com>") ||
		!strings.Contains(joined, "RCPT TO:<ana@example.com>") {
		t.Fatalf("comandos: %s", joined)
	}
	if strings.Contains(f.data, "\r\nBcc:") {
		t.Fatal("injeção de cabeçalho não foi neutralizada")
	}
	if !strings.Contains(f.data, "multipart/alternative") || !strings.Contains(f.data, "=?utf-8?q?") {
		t.Fatalf("mensagem MIME: %s", f.data)
	}
}

func TestSend_Erros(t *testing.T) {
	if err := (&SMTP{}).Send(context.Background(), "a@b.com", "", msg); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	if err := (&SMTP{Host: "127.0.0.1", Port: "1"}).Send(context.Background(), "invalido", "", msg); err == nil {
		t.Fatal("destinatário inválido")
	}
	if err := (&SMTP{Host: "127.0.0.1", Port: "1", Timeout: time.Second}).Send(context.Background(), "a@b.com", "", msg); err == nil {
		t.Fatal("conexão recusada")
	}
	f := startFake(t)
	f.rejectTo = true
	if err := (&SMTP{Host: "127.0.0.1", Port: f.port(), From: "o@x.com"}).Send(context.Background(), "a@b.com", "", msg); err == nil {
		t.Fatal("RCPT recusado deve falhar")
	}
}

func TestBuild(t *testing.T) {
	raw := string(Build(&mail.Address{Address: "o@x.com"}, &mail.Address{Name: "Ana", Address: "a@b.com"}, msg))
	for _, want := range []string{"From: <o@x.com>", `To: "Ana" <a@b.com>`, "text/plain; charset=utf-8", "text/html; charset=utf-8", "MIME-Version: 1.0"} {
		if !strings.Contains(raw, want) {
			t.Errorf("faltou %q", want)
		}
	}
}
