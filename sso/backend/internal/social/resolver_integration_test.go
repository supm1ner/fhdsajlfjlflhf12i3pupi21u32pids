//go:build integration

// Package social integration tests exercise the account resolver against a real
// PostgreSQL instance (via testcontainers), validating the find/link/create and
// the unverified-no-link guard end-to-end through the real auth stores.
// Run with: go test -tags=integration ./internal/social/...
// Requires Docker.
package social

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"cotton-id/internal/auth"
	"cotton-id/internal/database"
	"cotton-id/internal/observability"
	"cotton-id/migrations"
)

func setupDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	pg, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("cottonid"),
		postgres.WithUsername("cotton"),
		postgres.WithPassword("cotton"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	log := observability.NewLogger("error")
	db, err := database.Connect(ctx, dsn, log)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(db.Close)
	if err := db.RunMigrations(ctx, migrations.FS, log); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db.Pool
}

func newResolverDB(pool *pgxpool.Pool) *resolver {
	return newResolver(auth.NewUserStore(pool), auth.NewSocialIdentityStore(pool))
}

// TestResolveCreateThenReturning: first sign-in creates+links; a second sign-in
// with the same (provider,subject) returns the same account (no duplicate).
func TestResolveCreateThenReturning(t *testing.T) {
	pool := setupDB(t)
	r := newResolverDB(pool)
	ctx := context.Background()

	id := &Identity{Subject: "g-100", Email: "create@x.com", EmailVerified: true, Username: "creator", Name: "Cre Ator"}
	first, err := r.Resolve(ctx, ProviderGoogle, id)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if first.Outcome != outcomeCreated {
		t.Fatalf("first outcome = %s, want created", first.Outcome)
	}

	second, err := r.Resolve(ctx, ProviderGoogle, id)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if second.Outcome != outcomeExisting || second.User.ID != first.User.ID {
		t.Errorf("returning user not matched: outcome=%s id=%v want %v", second.Outcome, second.User.ID, first.User.ID)
	}
}

// TestResolveLinkVerifiedEmailToPasswordAccount: a password account exists with a
// verified email; a verified-email social sign-in links to it.
func TestResolveLinkVerifiedEmail(t *testing.T) {
	pool := setupDB(t)
	users := auth.NewUserStore(pool)
	r := newResolverDB(pool)
	ctx := context.Background()

	existing, err := users.Create(ctx, auth.CreateUserParams{
		Email: "link@x.com", Username: "linkme", DisplayName: "Link Me", PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	res, err := r.Resolve(ctx, ProviderGitHub, &Identity{Subject: "gh-link", Email: "link@x.com", EmailVerified: true})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Outcome != outcomeLinked || res.User.ID != existing.ID {
		t.Errorf("got outcome=%s id=%v, want linked to %v", res.Outcome, res.User.ID, existing.ID)
	}
}

// TestResolveUnverifiedNeverLinks is the takeover guard end-to-end: an unverified
// email matching an existing account creates a SEPARATE account.
func TestResolveUnverifiedNeverLinks(t *testing.T) {
	pool := setupDB(t)
	users := auth.NewUserStore(pool)
	r := newResolverDB(pool)
	ctx := context.Background()

	victim, err := users.Create(ctx, auth.CreateUserParams{
		Email: "victim@x.com", Username: "victim", DisplayName: "Victim", PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("seed victim: %v", err)
	}

	res, err := r.Resolve(ctx, ProviderGitHub, &Identity{Subject: "attacker", Email: "victim@x.com", EmailVerified: false, Username: "victim"})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.User.ID == victim.ID {
		t.Fatal("SECURITY: unverified email linked to the victim account")
	}
	if res.Outcome != outcomeUnverif {
		t.Errorf("outcome = %s, want created_unverified", res.Outcome)
	}
	if res.User.EmailVerified {
		t.Error("separate account must be email_verified=false")
	}
	if res.User.Username == "victim" {
		t.Errorf("username collision not suffixed: %q", res.User.Username)
	}
}
