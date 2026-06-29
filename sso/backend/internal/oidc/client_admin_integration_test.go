//go:build integration

// Package oidc integration test: exercises the OAuth2 client CRUD + per-client
// consent primitives the console Services tab relies on, against a real Ory Hydra
// admin API. It is gated behind the `integration` build tag and only runs when
// HYDRA_ADMIN_URL is set (under `docker compose`, not the unit pass).
//
// Run (from backend/, with the compose stack up):
//
//	go test -tags integration ./internal/oidc/ -run TestClientAdminRoundTrip -v
//
// Required env:
//
//	HYDRA_ADMIN_URL   e.g. http://localhost:4445
//
// It covers: create -> list -> get -> update (edit) -> delete on a throwaway
// confidential client, and the best-effort per-client consent revoke primitive
// (subject+client) Hydra supports.
package oidc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestClientAdminRoundTrip(t *testing.T) {
	adminURL := envOrSkip(t, "HYDRA_ADMIN_URL")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin := NewHydraClient(adminURL)

	// --- create (confidential → Hydra issues a secret once) ---
	name := "console-roundtrip-" + uuid.NewString()
	created, err := admin.CreateClient(ctx, OAuth2Client{
		ClientName:              name,
		RedirectURIs:            []string{"https://app.example/cb"},
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		Scope:                   "openid profile email",
		TokenEndpointAuthMethod: "client_secret_basic",
	})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	id := created.ClientID
	t.Cleanup(func() { _ = admin.DeleteClient(context.Background(), id) })
	if created.ClientSecret == "" {
		t.Fatalf("confidential create should return a secret once")
	}

	// --- list (must include the new client) ---
	clients, err := admin.ListClients(ctx)
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	found := false
	for _, c := range clients {
		if c.ClientID == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created client %s not in list", id)
	}

	// --- get (secret is NOT re-served on a read) ---
	got, err := admin.GetClient(ctx, id)
	if err != nil {
		t.Fatalf("get client: %v", err)
	}
	if got.ClientName != name {
		t.Errorf("get name = %q, want %q", got.ClientName, name)
	}
	if got.ClientSecret != "" {
		t.Errorf("get must not re-serve the secret, got %q", got.ClientSecret)
	}
	if got.TokenEndpointAuthMethod != "client_secret_basic" {
		t.Errorf("auth method = %q, want client_secret_basic", got.TokenEndpointAuthMethod)
	}

	// --- update (edit name + scope; keep the same id + type) ---
	got.ClientName = name + "-renamed"
	got.Scope = "openid email"
	got.RedirectURIs = []string{"https://app.example/cb2"}
	updated, err := admin.UpdateClient(ctx, id, *got)
	if err != nil {
		t.Fatalf("update client: %v", err)
	}
	if updated.ClientName != name+"-renamed" {
		t.Errorf("update name = %q", updated.ClientName)
	}
	reread, err := admin.GetClient(ctx, id)
	if err != nil {
		t.Fatalf("re-get client: %v", err)
	}
	if reread.ClientName != name+"-renamed" || reread.Scope != "openid email" {
		t.Errorf("edit not persisted: name=%q scope=%q", reread.ClientName, reread.Scope)
	}

	// --- per-client consent revoke primitive (subject+client; idempotent) ---
	// No grant exists for this throwaway client, so the revoke is a no-op but must
	// not error (404 treated as success). This is the call the console composes
	// across subjects for the best-effort per-client revoke.
	if err := admin.RevokeConsentSessions(ctx, uuid.NewString(), id); err != nil {
		t.Errorf("revoke consent (subject+client) should be idempotent, got %v", err)
	}

	// --- delete (idempotent) ---
	if err := admin.DeleteClient(ctx, id); err != nil {
		t.Fatalf("delete client: %v", err)
	}
	if _, err := admin.GetClient(ctx, id); err == nil {
		t.Errorf("get after delete should fail (client gone)")
	}
	if err := admin.DeleteClient(ctx, id); err != nil {
		t.Errorf("second delete should be idempotent, got %v", err)
	}
}
