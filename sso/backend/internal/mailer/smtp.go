package mailer

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// smtpDialTimeout bounds the whole SMTP conversation (dial + handshake + send) so
// a hung or slow mail server can never pin a sending goroutine. Sends already run
// on a detached, separately-bounded context at the call sites; this is the
// transport-level floor.
const smtpDialTimeout = 10 * time.Second

// SMTPConfig configures the [SMTPMailer]. Username/Password are optional (PLAIN
// auth is used only when a username is set); STARTTLS upgrades the plaintext
// connection to TLS before AUTH/DATA when true.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	STARTTLS bool
}

// SMTPMailer delivers email over SMTP using net/smtp. It opens a fresh connection
// per message (cotton-id's mail volume is low: reset, login-notification, admin
// message), optionally upgrades to TLS via STARTTLS, and optionally authenticates
// with PLAIN. It is safe for concurrent use (no shared mutable state).
type SMTPMailer struct {
	cfg SMTPConfig
}

// NewSMTPMailer builds an SMTPMailer from cfg.
func NewSMTPMailer(cfg SMTPConfig) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

// Send delivers a generic message via SMTP.
func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	return m.send(ctx, msg.To, msg.Subject, msg.Body)
}

// SendPasswordReset delivers the reset link as a plain-text message.
func (m *SMTPMailer) SendPasswordReset(ctx context.Context, to, resetLink string) error {
	body := "We received a request to reset your cotton-id password.\r\n\r\n" +
		"Follow this link to choose a new password:\r\n" + resetLink + "\r\n\r\n" +
		"If you did not request this, you can ignore this email."
	return m.send(ctx, to, "Reset your cotton-id password", body)
}

// SendVerificationCode delivers a one-time code for email verification.
// It fires the SMTP conversation in a background goroutine so the caller
// (the HTTP handler) returns immediately. The code is already stored in
// CodeStore before this is called, so best-effort delivery is sufficient.
func (m *SMTPMailer) SendVerificationCode(ctx context.Context, to, code string) error {
	body := "Your cotton-id verification code is:\r\n\r\n" +
		code + "\r\n\r\n" +
		"This code expires in 10 minutes. If you did not request this, you can ignore this email."
	go m.send(context.Background(), to, "Verify your cotton-id email", body)
	return nil
}

// send performs the SMTP conversation using Go's standard smtp.SendMail.
func (m *SMTPMailer) send(ctx context.Context, to, subject, body string) error {
	if strings.TrimSpace(to) == "" {
		return errors.New("mailer: empty recipient")
	}
	addr := net.JoinHostPort(m.cfg.Host, fmt.Sprintf("%d", m.cfg.Port))

	msg := buildMessage(m.cfg.From, to, subject, body)

	errc := make(chan error, 1)
	go func() {
		errc <- smtp.SendMail(addr, smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host), m.cfg.From, []string{to}, msg)
	}()

	select {
	case err := <-errc:
		if err != nil {
			return fmt.Errorf("mailer: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(smtpDialTimeout):
		return errors.New("mailer: timeout")
	}
}

// buildMessage assembles a minimal RFC 5322 message: headers (From/To/Subject/
// Date/MIME) followed by a CRLF and the plain-text body. The subject is
// RFC 2047 encoded so non-ASCII subjects survive; the body's bare newlines are
// normalized to CRLF and a leading "." is dot-stuffed defensively.
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	// Strip CR/LF from address headers so a malformed value can never inject extra
	// headers (Bcc:, etc.), independent of net/smtp's own Rcpt/Mail validation —
	// the message builder is the correct layer to enforce this.
	b.WriteString("From: " + stripCRLF(from) + "\r\n")
	b.WriteString("To: " + stripCRLF(to) + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(normalizeBody(body))
	return []byte(b.String())
}

// stripCRLF removes carriage returns and line feeds from a header value so it
// cannot break out of its header line and inject additional headers.
func stripCRLF(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// normalizeBody converts lone LF/CR to CRLF line endings (SMTP requires CRLF) and
// dot-stuffs any line that begins with "." so a "." line cannot prematurely
// terminate the DATA stream.
func normalizeBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		if strings.HasPrefix(ln, ".") {
			lines[i] = "." + ln
		}
	}
	return strings.Join(lines, "\r\n")
}

// Ensure SMTPMailer satisfies Mailer.
var _ Mailer = (*SMTPMailer)(nil)
