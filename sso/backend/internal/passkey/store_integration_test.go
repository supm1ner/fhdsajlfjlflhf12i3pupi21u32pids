//go:build integration

// Package passkey integration tests exercise the credential store against a real
// PostgreSQL instance (via testcontainers), validating CRUD and the cross-user
// scoping guard end-to-end through the real migration + pgx store.
// Run with: go test -tags=integration ./internal/passkey/...
// Requires Docker.
//
// Note on full-ceremony coverage: a true end-to-end register→login ceremony
// requires a virtual authenticator (browser/WebAuthn client). The go-webauthn
// library (v0.17.4) does not export a public virtual-authenticator test helper,
// so the ceremony glue is covered by: the cookie-codec round-trip tests, the
// sign-count-regression decision test, the RP-config validation tests, and these
// store tests. The remaining gap is the protocol crypto inside CreateCredential /
// ValidatePasskeyLogin, which is exercised by the library's own test suite.
package passkey

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

func seedUser(t *testing.T, pool *pgxpool.Pool, email, username string) *auth.User {
	t.Helper()
	u, err := auth.NewUserStore(pool).Create(context.Background(), auth.CreateUserParams{
		Email: email, Username: username, DisplayName: username, PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return u
}

// TestCredentialCRUD: create, list, get-by-credential-id, update sign count, delete.
func TestCredentialCRUD(t *testing.T) {
	pool := setupDB(t)
	store := NewCredentialStore(pool)
	ctx := context.Background()

	user := seedUser(t, pool, "owner@x.com", "owner")
	credID := []byte{0x01, 0x02, 0x03, 0x04}

	created, err := store.Create(ctx, CreateParams{
		UserID:          user.ID,
		CredentialID:    credID,
		PublicKey:       []byte{0xAA, 0xBB},
		AttestationType: "none",
		AAGUID:          []byte{0x00},
		SignCount:       0,
		Transports:      []string{"internal", "hybrid"},
		Name:            "Test Key",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Name != "Test Key" || created.UserID != user.ID {
		t.Errorf("created mismatch: %+v", created)
	}

	list, err := store.ListByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || string(list[0].CredentialID) != string(credID) {
		t.Fatalf("list = %+v, want one matching credential", list)
	}
	if len(list[0].Transports) != 2 {
		t.Errorf("transports = %v, want 2", list[0].Transports)
	}

	got, err := store.GetByCredentialID(ctx, credID)
	if err != nil {
		t.Fatalf("get-by-credential-id: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("get id = %v, want %v", got.ID, created.ID)
	}

	if err := store.UpdateSignCount(ctx, credID, 42); err != nil {
		t.Fatalf("update sign count: %v", err)
	}
	got, _ = store.GetByCredentialID(ctx, credID)
	if got.SignCount != 42 {
		t.Errorf("sign count = %d, want 42", got.SignCount)
	}
	if got.LastUsedAt == nil {
		t.Error("last_used_at must be set after UpdateSignCount")
	}

	if err := store.DeleteForUser(ctx, user.ID, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetByCredentialID(ctx, credID); err != ErrCredentialNotFound {
		t.Errorf("after delete, get err = %v, want ErrCredentialNotFound", err)
	}
}

// TestCrossUserDeleteRefused is the scoping guard end-to-end: user B cannot delete
// user A's credential, and A's credential is unaffected.
func TestCrossUserDeleteRefused(t *testing.T) {
	pool := setupDB(t)
	store := NewCredentialStore(pool)
	ctx := context.Background()

	alice := seedUser(t, pool, "alice@x.com", "alice")
	bob := seedUser(t, pool, "bob@x.com", "bob")

	aliceCred, err := store.Create(ctx, CreateParams{
		UserID: alice.ID, CredentialID: []byte{0x10}, PublicKey: []byte{0x20}, Name: "Alice Key",
	})
	if err != nil {
		t.Fatalf("create alice cred: %v", err)
	}

	// Bob attempts to delete Alice's credential by its id.
	if err := store.DeleteForUser(ctx, bob.ID, aliceCred.ID); err != ErrCredentialNotFound {
		t.Fatalf("cross-user delete err = %v, want ErrCredentialNotFound", err)
	}

	// Alice's credential must be unaffected.
	if _, err := store.GetByCredentialID(ctx, []byte{0x10}); err != nil {
		t.Errorf("SECURITY: Alice's credential was affected by Bob's delete: %v", err)
	}

	// And Bob's own list never includes Alice's credential.
	bobList, _ := store.ListByUser(ctx, bob.ID)
	if len(bobList) != 0 {
		t.Errorf("Bob's list = %+v, want empty (never another user's credential)", bobList)
	}
}

// TestListByUserScoping confirms a user only ever sees their own credentials.
func TestListByUserScoping(t *testing.T) {
	pool := setupDB(t)
	store := NewCredentialStore(pool)
	ctx := context.Background()

	alice := seedUser(t, pool, "a2@x.com", "alice2")
	bob := seedUser(t, pool, "b2@x.com", "bob2")

	_, _ = store.Create(ctx, CreateParams{UserID: alice.ID, CredentialID: []byte{0x01}, PublicKey: []byte{0x01}})
	_, _ = store.Create(ctx, CreateParams{UserID: alice.ID, CredentialID: []byte{0x02}, PublicKey: []byte{0x02}})
	_, _ = store.Create(ctx, CreateParams{UserID: bob.ID, CredentialID: []byte{0x03}, PublicKey: []byte{0x03}})

	aliceList, _ := store.ListByUser(ctx, alice.ID)
	if len(aliceList) != 2 {
		t.Errorf("alice list = %d, want 2", len(aliceList))
	}
	for _, c := range aliceList {
		if c.UserID != alice.ID {
			t.Errorf("SECURITY: alice's list contained another user's credential: %+v", c)
		}
	}
}
