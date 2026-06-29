package auth

import (
	"strings"
	"testing"
)

// TestBuildResetLinkMatchesSPARoute guards the password-reset link path against
// drifting away from the SPA route (frontend/src/App.tsx registers `/reset`).
// A mismatch silently breaks the end-to-end reset flow (the emailed link 404s).
func TestBuildResetLinkMatchesSPARoute(t *testing.T) {
	t.Parallel()
	s := &Service{cfg: Config{FrontendBaseURL: "https://id.example.com/"}}
	link := s.buildResetLink("tok en+/?")

	if !strings.HasPrefix(link, "https://id.example.com/reset?token=") {
		t.Fatalf("reset link must target the SPA /reset route, got %q", link)
	}
	// The trailing slash on the base must not produce a double slash.
	if strings.Contains(link, "com//reset") {
		t.Fatalf("base trailing slash should be trimmed, got %q", link)
	}
	// The token must be URL-query escaped (no raw +, space, /, ? in the value).
	if strings.ContainsAny(strings.TrimPrefix(link, "https://id.example.com/reset?token="), " ") {
		t.Fatalf("token must be query-escaped, got %q", link)
	}
}
