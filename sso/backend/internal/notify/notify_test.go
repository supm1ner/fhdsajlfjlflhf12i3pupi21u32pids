package notify

import (
	"context"
	"strings"
	"testing"

	"cotton-id/internal/mailer"
)

func TestIsNewDevice(t *testing.T) {
	prior := []Device{
		{UserAgent: "Firefox", IP: "1.1.1.1"},
		{UserAgent: "Safari", IP: "2.2.2.2"},
	}
	tests := []struct {
		name    string
		current Device
		want    bool
	}{
		{"exact match is not new", Device{UserAgent: "Firefox", IP: "1.1.1.1"}, false},
		{"same UA different IP is new", Device{UserAgent: "Firefox", IP: "9.9.9.9"}, true},
		{"different UA same IP is new", Device{UserAgent: "Chrome", IP: "1.1.1.1"}, true},
		{"unseen device is new", Device{UserAgent: "Edge", IP: "3.3.3.3"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNewDevice(prior, tc.current); got != tc.want {
				t.Errorf("IsNewDevice(%+v) = %v, want %v", tc.current, got, tc.want)
			}
		})
	}
}

func TestIsNewDeviceEmptyPriorIsNew(t *testing.T) {
	if !IsNewDevice(nil, Device{UserAgent: "X", IP: "1.2.3.4"}) {
		t.Error("with no prior sessions a sign-in must be treated as new")
	}
}

func TestExcludingOneDropsExactlyOneMatch(t *testing.T) {
	current := Device{UserAgent: "Firefox", IP: "1.1.1.1"}
	// Two sessions match the current fingerprint (e.g. a prior login + the
	// just-created one); ExcludingOne must drop exactly ONE, leaving a prior match
	// so the device is correctly recognized as NOT new.
	sessions := []Device{
		{UserAgent: "Firefox", IP: "1.1.1.1"},
		{UserAgent: "Firefox", IP: "1.1.1.1"},
		{UserAgent: "Safari", IP: "2.2.2.2"},
	}
	prior := ExcludingOne(sessions, current)
	if len(prior) != 2 {
		t.Fatalf("ExcludingOne kept %d devices, want 2", len(prior))
	}
	if IsNewDevice(prior, current) {
		t.Error("a remaining prior match must make the device NOT new")
	}
}

func TestExcludingOneOnlyCurrentSessionYieldsNew(t *testing.T) {
	current := Device{UserAgent: "Firefox", IP: "1.1.1.1"}
	// Only the just-created session matches → after dropping it, no prior match →
	// the device is genuinely new.
	sessions := []Device{{UserAgent: "Firefox", IP: "1.1.1.1"}}
	prior := ExcludingOne(sessions, current)
	if !IsNewDevice(prior, current) {
		t.Error("after dropping the only (current) session, the device must be new")
	}
}

// recordingMailer captures the last message sent for assertion.
type recordingMailer struct {
	last mailer.Message
	sent int
}

func (m *recordingMailer) Send(_ context.Context, msg mailer.Message) error {
	m.last = msg
	m.sent++
	return nil
}

func (m *recordingMailer) SendPasswordReset(_ context.Context, _, _ string) error { return nil }

func TestSendLoginNotificationComposesMessage(t *testing.T) {
	rec := &recordingMailer{}
	n := NewNotifier(rec, nil, "cotton-id")
	err := n.SendLoginNotification(context.Background(), "alex@example.com", "Alex",
		Device{UserAgent: "Firefox/1.0", IP: "1.2.3.4"})
	if err != nil {
		t.Fatalf("SendLoginNotification: %v", err)
	}
	if rec.sent != 1 {
		t.Fatalf("expected 1 send, got %d", rec.sent)
	}
	if rec.last.To != "alex@example.com" {
		t.Errorf("To = %q", rec.last.To)
	}
	if !strings.Contains(rec.last.Subject, "New sign-in") {
		t.Errorf("Subject = %q", rec.last.Subject)
	}
	for _, want := range []string{"Alex", "Firefox/1.0", "1.2.3.4"} {
		if !strings.Contains(rec.last.Body, want) {
			t.Errorf("body missing %q:\n%s", want, rec.last.Body)
		}
	}
}

func TestSendLoginNotificationNilMailerIsNoOp(t *testing.T) {
	n := NewNotifier(nil, nil, "")
	if err := n.SendLoginNotification(context.Background(), "x@y", "X", Device{}); err != nil {
		t.Errorf("nil mailer should be a no-op, got %v", err)
	}
}

func TestNilNotifierIsNoOp(t *testing.T) {
	var n *Notifier
	if err := n.SendLoginNotification(context.Background(), "x@y", "X", Device{}); err != nil {
		t.Errorf("nil notifier should be a no-op, got %v", err)
	}
	// Async variant must also be safe on a nil receiver.
	n.SendLoginNotificationAsync(context.Background(), "x@y", "X", Device{})
}
