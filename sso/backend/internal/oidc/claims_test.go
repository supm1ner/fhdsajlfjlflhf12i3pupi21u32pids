package oidc

import (
	"testing"

	"github.com/google/uuid"

	"cotton-id/internal/auth"
)

func testUser() *auth.User {
	return &auth.User{
		ID:            uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Email:         "alex@cotton-id.io",
		EmailVerified: true,
		Username:      "alex",
		DisplayName:   "Alex Cotton",
	}
}

func TestClaimsForUser_SubjectAlwaysSet(t *testing.T) {
	u := testUser()
	// Even with no scopes the stable subject must be present.
	c := ClaimsForUser(u, nil)
	if c.Subject != u.ID.String() {
		t.Fatalf("subject = %q, want %q", c.Subject, u.ID.String())
	}
	if c.Email != "" || c.Name != "" || c.PreferredUsername != "" {
		t.Fatalf("no scope-gated claims should be set without scopes: %+v", c)
	}
	if c.EmailVerified {
		t.Fatalf("email_verified must not be set without the email scope")
	}
}

func TestClaimsForUser_EmailScope(t *testing.T) {
	u := testUser()
	c := ClaimsForUser(u, []string{ScopeOpenID, ScopeEmail})
	if c.Email != u.Email {
		t.Fatalf("email = %q, want %q", c.Email, u.Email)
	}
	if !c.EmailVerified {
		t.Fatalf("email_verified should be true for a verified account with the email scope")
	}
	// profile claims must NOT be present without the profile scope.
	if c.Name != "" || c.PreferredUsername != "" {
		t.Fatalf("profile claims leaked without the profile scope: %+v", c)
	}
}

func TestClaimsForUser_ProfileScope(t *testing.T) {
	u := testUser()
	c := ClaimsForUser(u, []string{ScopeOpenID, ScopeProfile})
	if c.Name != u.DisplayName {
		t.Fatalf("name = %q, want %q", c.Name, u.DisplayName)
	}
	if c.PreferredUsername != u.Username {
		t.Fatalf("preferred_username = %q, want %q", c.PreferredUsername, u.Username)
	}
	// email claims must NOT be present without the email scope.
	if c.Email != "" {
		t.Fatalf("email leaked without the email scope: %+v", c)
	}
}

func TestClaimsForUser_AllScopes(t *testing.T) {
	u := testUser()
	c := ClaimsForUser(u, []string{ScopeOpenID, ScopeProfile, ScopeEmail})
	if c.Subject != u.ID.String() ||
		c.Email != u.Email ||
		!c.EmailVerified ||
		c.Name != u.DisplayName ||
		c.PreferredUsername != u.Username {
		t.Fatalf("unexpected claims with all scopes: %+v", c)
	}
}

func TestClaimsForUser_UnverifiedEmail(t *testing.T) {
	u := testUser()
	u.EmailVerified = false
	c := ClaimsForUser(u, []string{ScopeEmail})
	if c.Email != u.Email {
		t.Fatalf("email should still be set: %q", c.Email)
	}
	if c.EmailVerified {
		t.Fatalf("email_verified must reflect the account: got true, want false")
	}
}

func TestScopeIntersection(t *testing.T) {
	cases := []struct {
		name      string
		requested []string
		allowed   []string
		want      []string
	}{
		{"empty requested", nil, []string{"openid"}, nil},
		{"subset", []string{"openid", "email"}, []string{"openid", "profile", "email"}, []string{"openid", "email"}},
		{"drops disallowed", []string{"openid", "admin", "email"}, []string{"openid", "email"}, []string{"openid", "email"}},
		{"preserves requested order", []string{"email", "openid"}, []string{"openid", "email"}, []string{"email", "openid"}},
		{"none allowed", []string{"openid"}, nil, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scopeIntersection(tc.requested, tc.allowed)
			if !equalStrings(got, tc.want) {
				t.Fatalf("scopeIntersection(%v, %v) = %v, want %v", tc.requested, tc.allowed, got, tc.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
