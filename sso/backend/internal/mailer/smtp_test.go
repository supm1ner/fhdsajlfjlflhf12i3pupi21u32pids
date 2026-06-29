package mailer

import (
	"strings"
	"testing"
)

func TestBuildMessageHeadersAndBody(t *testing.T) {
	raw := string(buildMessage("noreply@cotton-id.io", "alex@example.com", "Hello", "line one\nline two"))

	for _, want := range []string{
		"From: noreply@cotton-id.io\r\n",
		"To: alex@example.com\r\n",
		"Subject: Hello\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=\"utf-8\"\r\n",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("message missing header %q\n--- message ---\n%s", want, raw)
		}
	}

	// Header/body separator is a blank CRLF line, and the body uses CRLF endings.
	if !strings.Contains(raw, "\r\n\r\nline one\r\nline two") {
		t.Errorf("body not CRLF-normalized after header separator:\n%s", raw)
	}
}

func TestBuildMessageEncodesNonASCIISubject(t *testing.T) {
	raw := string(buildMessage("from@x", "to@x", "Привет", "body"))
	// The subject line must be RFC 2047 encoded (no raw multibyte runes in headers).
	if strings.Contains(raw, "Привет") {
		t.Errorf("non-ASCII subject was not encoded:\n%s", raw)
	}
	if !strings.Contains(raw, "Subject: =?utf-8?q?") {
		t.Errorf("expected RFC2047 q-encoded subject, got:\n%s", raw)
	}
}

func TestNormalizeBodyDotStuffsAndNormalizesEndings(t *testing.T) {
	got := normalizeBody(".hidden\r\nnormal\rmixed\n.dotted")
	want := "..hidden\r\nnormal\r\nmixed\r\n..dotted"
	if got != want {
		t.Errorf("normalizeBody:\n got %q\nwant %q", got, want)
	}
}

func TestSendRejectsEmptyRecipient(t *testing.T) {
	m := NewSMTPMailer(SMTPConfig{Host: "localhost", Port: 587, From: "from@x"})
	if err := m.send(t.Context(), "  ", "s", "b"); err == nil {
		t.Fatal("expected an error for an empty recipient, got nil")
	}
}
