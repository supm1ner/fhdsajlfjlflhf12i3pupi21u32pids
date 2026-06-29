// Package mailer abstracts outbound email. It ships a development [LogMailer]
// (logs messages instead of delivering them, the safe local default) and a real
// [SMTPMailer] (net/smtp, optional STARTTLS + PLAIN auth) selected by main.go
// when SMTP is configured.
//
// All transactional sends go through the [Mailer] interface. Higher layers treat
// delivery as BEST-EFFORT: a send error is logged and never blocks or fails the
// user action (password reset, login-notification, admin message).
package mailer

import (
	"context"
	"log/slog"
)

// Message is one outbound email. Callers populate To, Subject, and Body (plain
// text). From is filled in by the transport (the configured envelope/header
// sender) so callers need not know it.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Mailer sends transactional email. Implementations must be safe for concurrent
// use. A delivery error is returned so the caller can log it; callers MUST treat
// every send as best-effort and never fail the user action on a mail error.
type Mailer interface {
	// Send delivers a generic transactional message. It backs the
	// login-notification and admin "message user" features.
	Send(ctx context.Context, msg Message) error

	// SendPasswordReset delivers a password-reset link to the recipient. The
	// link already embeds the single-use token.
	SendPasswordReset(ctx context.Context, to, resetLink string) error

	// SendVerificationCode delivers a one-time code for email verification.
	SendVerificationCode(ctx context.Context, to, code string) error
}

// LogMailer is the development Mailer: it logs each message via slog rather than
// delivering it, so resets/notifications can be exercised end-to-end without an
// SMTP server. The reset link (and thus token) is logged at info level —
// acceptable in dev, and explicitly called out as a known gap for production.
type LogMailer struct {
	log *slog.Logger
}

// NewLogMailer builds a LogMailer over the given logger.
func NewLogMailer(log *slog.Logger) *LogMailer {
	return &LogMailer{log: log}
}

// Send logs the message (dev: not actually delivered).
func (m *LogMailer) Send(_ context.Context, msg Message) error {
	m.log.Info("email (dev: not actually sent)",
		slog.String("to", msg.To),
		slog.String("subject", msg.Subject),
		slog.String("body", msg.Body),
	)
	return nil
}

// SendPasswordReset logs the reset link.
func (m *LogMailer) SendPasswordReset(_ context.Context, to, resetLink string) error {
	m.log.Info("password reset email (dev: not actually sent)",
		slog.String("to", to),
		slog.String("reset_link", resetLink),
	)
	return nil
}

// SendVerificationCode logs the code.
func (m *LogMailer) SendVerificationCode(_ context.Context, to, code string) error {
	m.log.Info("verification code email (dev: not actually sent)",
		slog.String("to", to),
		slog.String("code", code),
	)
	return nil
}

// Ensure LogMailer satisfies Mailer.
var _ Mailer = (*LogMailer)(nil)
