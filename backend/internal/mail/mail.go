// Package mail sends customer emails over SMTP: Mailpit locally, any
// provider's SMTP endpoint in production. With no SMTP host configured,
// messages are logged instead, so development works without a mail server.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Message is one email. HTML is optional; Text is always sent.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// New returns an SMTP sender, or a logging one when cfg.Host is empty.
func New(cfg Config, logger *slog.Logger) Sender {
	if cfg.Host == "" {
		return logSender{logger}
	}
	return &smtpSender{cfg: cfg}
}

type logSender struct{ logger *slog.Logger }

func (l logSender) Send(_ context.Context, m Message) error {
	l.logger.Info("email (SMTP not configured, not sent)", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}

type smtpSender struct{ cfg Config }

const sendTimeout = 15 * time.Second

func (s *smtpSender) Send(ctx context.Context, m Message) error {
	from, err := mail.ParseAddress(s.cfg.From)
	if err != nil {
		return fmt.Errorf("invalid MAIL_FROM: %w", err)
	}
	to, err := mail.ParseAddress(m.To)
	if err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	body, err := build(from, to, m)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("connect to smtp: %w", err)
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp hello: %w", err)
	}
	defer c.Close()

	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if s.cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := c.Rcpt(to.Address); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return c.Quit()
}

// build writes a multipart/alternative message (plain text + optional
// HTML), quoted-printable so long lines and £ signs survive any relay.
func build(from, to *mail.Address, m Message) ([]byte, error) {
	var b bytes.Buffer
	boundary := randomBoundary()
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }

	header("From", from.String())
	header("To", to.String())
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", time.Now().Format(time.RFC1123Z))
	header("Message-ID", fmt.Sprintf("<%s@%s>", randomBoundary(), domainOf(from.Address)))
	header("MIME-Version", "1.0")

	if m.HTML == "" {
		header("Content-Type", `text/plain; charset="utf-8"`)
		header("Content-Transfer-Encoding", "quoted-printable")
		b.WriteString("\r\n")
		if err := writeQP(&b, m.Text); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}

	header("Content-Type", fmt.Sprintf(`multipart/alternative; boundary="%s"`, boundary))
	b.WriteString("\r\n")
	for _, part := range []struct{ ctype, body string }{
		{"text/plain", m.Text},
		{"text/html", m.HTML},
	} {
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		fmt.Fprintf(&b, "Content-Type: %s; charset=\"utf-8\"\r\n", part.ctype)
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		if err := writeQP(&b, part.body); err != nil {
			return nil, err
		}
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes(), nil
}

func writeQP(b *bytes.Buffer, s string) error {
	w := quotedprintable.NewWriter(b)
	if _, err := w.Write([]byte(strings.ReplaceAll(s, "\n", "\r\n"))); err != nil {
		return err
	}
	return w.Close()
}

func randomBoundary() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func domainOf(address string) string {
	if i := strings.LastIndex(address, "@"); i >= 0 {
		return address[i+1:]
	}
	return "localhost"
}
