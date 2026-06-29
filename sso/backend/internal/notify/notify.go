// Package notify sends best-effort, non-blocking user notifications over the
// mailer — currently the login-notification email sent when a user signs in from
// a new device/IP and has the preference enabled.
//
// It is deliberately free of any dependency on internal/auth so the auth/social/
// passkey handler packages (which live in / import internal/auth) can use it
// without an import cycle. Callers adapt their session/user types to the small
// value types defined here.
//
// Every send is BEST-EFFORT: the caller runs it on a detached, time-bounded
// context in a goroutine, so a slow or failing mail server never blocks or fails
// the sign-in. A returned error is for logging only.
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cotton-id/internal/mailer"
)

// loginNotifyTimeout bounds the detached login-notification send so a slow mail
// server cannot leak goroutines.
const loginNotifyTimeout = 15 * time.Second

// Device is the coarse fingerprint of a sign-in: the user-agent string and the
// client IP. Two sign-ins are considered the "same device" when both match
// exactly (the heuristic is intentionally simple; see IsNewDevice).
type Device struct {
	UserAgent string
	IP        string
}

// fingerprint is the comparison key for new-device detection: the exact
// (user-agent, ip) pair. Kept private so the heuristic stays centralized.
func (d Device) fingerprint() string {
	return strings.TrimSpace(d.UserAgent) + "\x00" + strings.TrimSpace(d.IP)
}

// ExcludingOne returns prior with a SINGLE occurrence of current removed. Login
// handlers capture the user's recent sessions AFTER the new session is created, so
// the just-created session (which always matches current) would otherwise mask a
// genuinely new device; dropping one occurrence yields the truly-prior set for
// IsNewDevice.
func ExcludingOne(prior []Device, current Device) []Device {
	out := make([]Device, 0, len(prior))
	dropped := false
	cur := current.fingerprint()
	for _, p := range prior {
		if !dropped && p.fingerprint() == cur {
			dropped = true
			continue
		}
		out = append(out, p)
	}
	return out
}

// IsNewDevice reports whether current matches NONE of the prior sign-in
// fingerprints — i.e. this (user-agent, ip) pair has not been seen among the
// user's recent sessions. Callers pass the fingerprints of the user's recent
// sessions captured BEFORE the new session is created. An empty prior slice
// means "no recent sessions" → treated as new (e.g. first-ever or all expired).
//
// The heuristic is coarse by design (design D2): no stateful known-devices table.
// It can yield false "new device" results when an IP rotates or a UA-string
// changes; that is acceptable for a best-effort security heads-up gated behind
// the per-account preference.
func IsNewDevice(prior []Device, current Device) bool {
	cur := current.fingerprint()
	for _, p := range prior {
		if p.fingerprint() == cur {
			return false
		}
	}
	return true
}

// Notifier composes the mailer to send user notifications. A nil *Notifier is a
// valid no-op so handlers can hold it as an optional dependency. It is safe for
// concurrent use.
type Notifier struct {
	mailer  mailer.Mailer
	log     *slog.Logger
	appName string
}

// NewNotifier builds a Notifier over the given mailer. appName labels the product
// in the email (defaults to "cotton-id" when empty). A nil mailer disables sends
// (every call is a no-op), so tests and unconfigured deployments are safe.
func NewNotifier(m mailer.Mailer, log *slog.Logger, appName string) *Notifier {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(appName) == "" {
		appName = "cotton-id"
	}
	return &Notifier{mailer: m, log: log, appName: appName}
}

// SendLoginNotification emails the account that a sign-in occurred from the given
// device/IP. It is BEST-EFFORT: a nil Notifier or nil mailer is a no-op, and a
// delivery error is returned for the caller to log (the caller runs this detached
// so it never blocks the sign-in). The caller is responsible for the preference
// check and the new-device gate; this method only composes + sends the message.
func (n *Notifier) SendLoginNotification(ctx context.Context, toEmail, displayName string, d Device) error {
	if n == nil || n.mailer == nil {
		return nil
	}
	if strings.TrimSpace(toEmail) == "" {
		return nil
	}

	name := strings.TrimSpace(displayName)
	if name == "" {
		name = "there"
	}
	subject := fmt.Sprintf("New sign-in to your %s account", n.appName)

	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s,\n\n", name)
	fmt.Fprintf(&b, "We noticed a new sign-in to your %s account from a device or location we haven't seen recently.\n\n", n.appName)
	if ua := strings.TrimSpace(d.UserAgent); ua != "" {
		fmt.Fprintf(&b, "Device: %s\n", ua)
	}
	if ip := strings.TrimSpace(d.IP); ip != "" {
		fmt.Fprintf(&b, "IP address: %s\n", ip)
	}
	b.WriteString("\nIf this was you, no action is needed. If you don't recognize this activity, ")
	b.WriteString("change your password and review your active sessions right away.")

	return n.mailer.Send(ctx, mailer.Message{
		To:      toEmail,
		Subject: subject,
		Body:    b.String(),
	})
}

// SendLoginNotificationAsync sends the login-notification email on a DETACHED,
// time-bounded context in a background goroutine, so it NEVER blocks or fails the
// sign-in. The context is detached from the request (context.WithoutCancel) so a
// client disconnect / handler return does not cancel the in-flight send, while
// still carrying request-scoped values (e.g. the correlation id) for logging. A
// nil Notifier is a safe no-op. Shared by the auth, social, and passkey login
// paths; the caller is responsible for the preference + new-device gate.
func (n *Notifier) SendLoginNotificationAsync(reqCtx context.Context, toEmail, displayName string, d Device) {
	if n == nil || n.mailer == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(reqCtx), loginNotifyTimeout)
		defer cancel()
		if err := n.SendLoginNotification(ctx, toEmail, displayName, d); err != nil {
			n.log.Warn("login notification send failed",
				slog.String("to", toEmail), slog.String("error", err.Error()))
		}
	}()
}
