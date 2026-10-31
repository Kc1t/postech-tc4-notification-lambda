// Package mail envia e-mail por SMTP (Brevo: smtp-relay.brevo.com:587 com STARTTLS).
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/Kc1t/postech-tc4-notification-lambda/internal/templates"
)

// ErrNotConfigured indica envio sem SMTP_HOST.
var ErrNotConfigured = errors.New("mail: SMTP não configurado")

// SMTP é o remetente. Com o servidor oferecendo STARTTLS a conexão é cifrada antes da
// autenticação; sem STARTTLS, o net/smtp só aceita autenticar em localhost.
type SMTP struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	FromName string
	Timeout  time.Duration
}

// Send entrega a mensagem para um destinatário.
func (s *SMTP) Send(ctx context.Context, toEmail, toName string, m templates.Message) error {
	if s.Host == "" {
		return ErrNotConfigured
	}
	to, err := mail.ParseAddress(toEmail)
	if err != nil {
		return fmt.Errorf("mail: destinatário inválido: %w", err)
	}
	to.Name = sanitize(toName)
	from := &mail.Address{Name: s.FromName, Address: s.From}

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(s.Host, s.Port))
	if err != nil {
		return fmt.Errorf("mail: conectar: %w", err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: handshake: %w", err)
	}
	defer func() { _ = c.Close() }()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if s.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return fmt.Errorf("mail: autenticar: %w", err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := w.Write(Build(from, to, m)); err != nil {
		return fmt.Errorf("mail: escrever: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: finalizar: %w", err)
	}
	return c.Quit()
}

// Build monta a mensagem MIME multipart/alternative (texto + HTML).
func Build(from, to *mail.Address, m templates.Message) []byte {
	boundary := newBoundary()
	var b bytes.Buffer
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", from.String())
	header("To", to.String())
	header("Subject", mime.QEncoding.Encode("utf-8", sanitize(m.Subject)))
	header("Date", time.Now().UTC().Format(time.RFC1123Z))
	header("MIME-Version", "1.0")
	header("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	part := func(contentType, body string) {
		fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", boundary, contentType)
		qp := quotedprintable.NewWriter(&b)
		_, _ = qp.Write([]byte(body))
		_ = qp.Close()
		b.WriteString("\r\n")
	}
	part("text/plain", m.Text)
	part("text/html", m.HTML)
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes()
}

// sanitize impede injeção de cabeçalhos (CR/LF em nome ou assunto).
func sanitize(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

func newBoundary() string {
	buf := make([]byte, 12)
	_, _ = rand.Read(buf)
	return "oficina-" + hex.EncodeToString(buf)
}
