package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidRequestID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		id   string
		want bool
	}{
		{"", false},
		{"3f9a1c2e-uuid_style.ok", true},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false}, // too long
		{"bad value with spaces", false}, // space not allowed
		{"inject\nnewline", false},       // control char
		{"semi;colon", false},            // disallowed punctuation
	}
	for _, c := range cases {
		if got := validRequestID(c.id); got != c.want {
			t.Errorf("validRequestID(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}

func TestRequestIDMiddlewareRejectsHostileInbound(t *testing.T) {
	t.Parallel()
	var seen string
	h := RequestIDMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestID(r.Context())
	}))

	// A hostile, oversized id must be replaced with a fresh generated one.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, strings.Repeat("x", 200)+"\n")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if seen == strings.Repeat("x", 200)+"\n" {
		t.Fatal("hostile inbound request id must not be reused verbatim")
	}
	if !validRequestID(seen) {
		t.Fatalf("generated request id should be valid, got %q", seen)
	}
	if rec.Header().Get(RequestIDHeader) != seen {
		t.Fatal("response header should echo the sanitized id")
	}
}

func TestRequestIDMiddlewareKeepsValidInbound(t *testing.T) {
	t.Parallel()
	var seen string
	h := RequestIDMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestID(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "trusted-correlation-id-123")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if seen != "trusted-correlation-id-123" {
		t.Fatalf("valid inbound id should be preserved, got %q", seen)
	}
}
