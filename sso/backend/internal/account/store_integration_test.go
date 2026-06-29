//go:build integration

// Package account integration tests exercise the profile/preferences/image
// stores, the session list/revoke scoping, and the account-delete FK cascade
// against a real PostgreSQL instance (via testcontainers) through the real
// migrations and pgx stores.
// Run with: go test -tags=integration ./internal/account/...
// Requires Docker.
package account

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"cotton-id/internal/auth"
	"cotton-id/internal/database"
	"cotton-id/internal/observability"
	"cotton-id/internal/passkey"
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

// TestProfileAndPreferenceStore: the new defaults are applied, and UpdateProfile /
// UpdatePreferences persist and round-trip.
func TestProfileAndPreferenceStore(t *testing.T) {
	pool := setupDB(t)
	store := auth.NewUserStore(pool)
	ctx := context.Background()

	u := seedUser(t, pool, "p@x.com", "prof")

	// Migration defaults.
	if u.PrefTheme != "system" || u.PrefLang != "ru" || !u.LoginNotifications {
		t.Fatalf("preference defaults wrong: theme=%q lang=%q notif=%v", u.PrefTheme, u.PrefLang, u.LoginNotifications)
	}
	if u.BannerURL != nil {
		t.Errorf("banner_url should default null, got %v", u.BannerURL)
	}

	updated, err := store.UpdateProfile(ctx, u.ID, auth.UpdateProfileParams{
		DisplayName: "Profiled", About: "hello", Location: "Almaty",
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if updated.DisplayName != "Profiled" || updated.About != "hello" || updated.Location != "Almaty" {
		t.Errorf("profile not persisted: %+v", updated)
	}

	prefs, err := store.UpdatePreferences(ctx, u.ID, auth.UpdatePreferencesParams{
		Theme: "dark", Lang: "en", LoginNotifications: false,
	})
	if err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	if prefs.PrefTheme != "dark" || prefs.PrefLang != "en" || prefs.LoginNotifications {
		t.Errorf("prefs not persisted: %+v", prefs)
	}

	// Re-read confirms persistence.
	reread, _ := store.GetByID(ctx, u.ID)
	if reread.PrefTheme != "dark" || reread.DisplayName != "Profiled" {
		t.Errorf("re-read mismatch: %+v", reread)
	}
}

// TestImageStoreUpsertGet: upsert stores bytes + content type; a second upsert
// overwrites; SetImageURL points the user's avatar_url at the served route.
func TestImageStoreUpsertGet(t *testing.T) {
	pool := setupDB(t)
	images := NewImageStore(pool)
	users := auth.NewUserStore(pool)
	ctx := context.Background()

	u := seedUser(t, pool, "img@x.com", "imguser")
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x01}

	if err := images.Upsert(ctx, u.ID, KindAvatar, "image/png", png); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := images.Get(ctx, u.ID, KindAvatar)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ContentType != "image/png" || string(got.Bytes) != string(png) {
		t.Errorf("stored image mismatch: %+v", got)
	}

	// Overwrite with a JPEG.
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x02}
	if err := images.Upsert(ctx, u.ID, KindAvatar, "image/jpeg", jpeg); err != nil {
		t.Fatalf("upsert overwrite: %v", err)
	}
	got, _ = images.Get(ctx, u.ID, KindAvatar)
	if got.ContentType != "image/jpeg" || len(got.Bytes) != len(jpeg) {
		t.Errorf("overwrite mismatch: %+v", got)
	}

	// Banner is a separate row.
	if _, err := images.Get(ctx, u.ID, KindBanner); err != ErrImageNotFound {
		t.Errorf("banner get err = %v, want ErrImageNotFound", err)
	}

	if err := users.SetImageURL(ctx, u.ID, "avatar", "http://host/api/v1/account/images/avatar"); err != nil {
		t.Fatalf("set image url: %v", err)
	}
	reread, _ := users.GetByID(ctx, u.ID)
	if reread.AvatarURL == nil || *reread.AvatarURL != "http://host/api/v1/account/images/avatar" {
		t.Errorf("avatar_url not set: %v", reread.AvatarURL)
	}
}

// TestUpsertWithURLAtomicCommit: the happy path commits the blob AND the user's
// avatar_url in one transaction — both are visible afterward.
func TestUpsertWithURLAtomicCommit(t *testing.T) {
	pool := setupDB(t)
	images := NewImageStore(pool)
	users := auth.NewUserStore(pool)
	ctx := context.Background()

	u := seedUser(t, pool, "atomic@x.com", "atomicuser")
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x01}
	url := "http://host/api/v1/account/images/avatar"

	if err := images.UpsertWithURL(ctx, u.ID, KindAvatar, "image/png", png, url); err != nil {
		t.Fatalf("UpsertWithURL: %v", err)
	}

	got, err := images.Get(ctx, u.ID, KindAvatar)
	if err != nil {
		t.Fatalf("blob not committed: %v", err)
	}
	if got.ContentType != "image/png" || string(got.Bytes) != string(png) {
		t.Errorf("blob mismatch: %+v", got)
	}
	reread, _ := users.GetByID(ctx, u.ID)
	if reread.AvatarURL == nil || *reread.AvatarURL != url {
		t.Errorf("avatar_url not committed: %v", reread.AvatarURL)
	}
}

// TestUpsertWithURLRollback: when the transaction fails (here: the user row does
// not exist, so the FK-bound blob insert + URL update cannot both succeed),
// UpsertWithURL returns an error and NOTHING is persisted — neither a stray blob
// nor an avatar_url. This is the atomicity guarantee: no partial state survives a
// failure.
func TestUpsertWithURLRollback(t *testing.T) {
	pool := setupDB(t)
	images := NewImageStore(pool)
	ctx := context.Background()

	ghost := uuid.New() // never inserted into users
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x01}

	err := images.UpsertWithURL(ctx, ghost, KindAvatar, "image/png", png, "http://host/x")
	if err == nil {
		t.Fatal("UpsertWithURL for a non-existent user should fail")
	}

	// No blob row was committed for the ghost user (the transaction rolled back).
	if _, gerr := images.Get(ctx, ghost, KindAvatar); gerr != ErrImageNotFound {
		t.Errorf("rollback failed: a blob persisted for the ghost user (err=%v)", gerr)
	}
	var count int
	if qerr := pool.QueryRow(ctx, `SELECT count(*) FROM profile_images WHERE user_id = $1`, ghost).Scan(&count); qerr != nil {
		t.Fatalf("count: %v", qerr)
	}
	if count != 0 {
		t.Errorf("rollback failed: %d profile_images rows persisted for the ghost user", count)
	}
}

// TestSessionListRevokeScoping: ListByUser returns only the user's sessions;
// DeleteForUser is scoped (cannot revoke another user's session); DeleteByUserExcept
// keeps the current one.
func TestSessionListRevokeScoping(t *testing.T) {
	pool := setupDB(t)
	sessions := auth.NewSessionStore(pool)
	ctx := context.Background()

	alice := seedUser(t, pool, "sa@x.com", "salice")
	bob := seedUser(t, pool, "sb@x.com", "sbob")

	mk := func(userID uuid.UUID, id string) {
		if err := sessions.Create(ctx, &auth.Session{
			ID: id, UserID: userID, ExpiresAt: time.Now().Add(time.Hour),
		}); err != nil {
			t.Fatalf("create session %s: %v", id, err)
		}
	}
	mk(alice.ID, "a1")
	mk(alice.ID, "a2")
	mk(alice.ID, "a3")
	mk(bob.ID, "b1")

	aliceList, err := sessions.ListByUser(ctx, alice.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(aliceList) != 3 {
		t.Fatalf("alice sessions = %d, want 3", len(aliceList))
	}
	for _, s := range aliceList {
		if s.UserID != alice.ID {
			t.Errorf("SECURITY: alice's list contained another user's session: %+v", s)
		}
	}

	// Bob cannot revoke Alice's session.
	if err := sessions.DeleteForUser(ctx, bob.ID, "a1"); err != auth.ErrSessionNotFound {
		t.Errorf("cross-user revoke err = %v, want ErrSessionNotFound", err)
	}
	if stillThere, _ := sessions.ListByUser(ctx, alice.ID); len(stillThere) != 3 {
		t.Errorf("SECURITY: Alice's sessions affected by Bob's revoke (now %d)", len(stillThere))
	}

	// Alice revokes all but a2 (her current).
	revoked, err := sessions.DeleteByUserExcept(ctx, alice.ID, "a2")
	if err != nil {
		t.Fatalf("delete except: %v", err)
	}
	if revoked != 2 {
		t.Errorf("revoked = %d, want 2", revoked)
	}
	remaining, _ := sessions.ListByUser(ctx, alice.ID)
	if len(remaining) != 1 || remaining[0].ID != "a2" {
		t.Errorf("after revoke-others, remaining = %+v, want only a2", remaining)
	}
}

// TestAccountDeleteCascade: deleting the user removes their sessions, passkeys,
// and profile images via FK ON DELETE CASCADE.
func TestAccountDeleteCascade(t *testing.T) {
	pool := setupDB(t)
	users := auth.NewUserStore(pool)
	sessions := auth.NewSessionStore(pool)
	images := NewImageStore(pool)
	creds := passkey.NewCredentialStore(pool)
	ctx := context.Background()

	u := seedUser(t, pool, "del@x.com", "deluser")

	if err := sessions.Create(ctx, &auth.Session{ID: "d1", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := images.Upsert(ctx, u.ID, KindAvatar, "image/png", []byte{0x89, 0x50}); err != nil {
		t.Fatalf("upsert image: %v", err)
	}
	if _, err := creds.Create(ctx, passkey.CreateParams{
		UserID: u.ID, CredentialID: []byte{0xAA}, PublicKey: []byte{0xBB}, Name: "k",
	}); err != nil {
		t.Fatalf("create credential: %v", err)
	}

	if err := users.Delete(ctx, u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	if _, err := users.GetByID(ctx, u.ID); err != auth.ErrUserNotFound {
		t.Errorf("user get after delete = %v, want ErrUserNotFound", err)
	}
	if list, _ := sessions.ListByUser(ctx, u.ID); len(list) != 0 {
		t.Errorf("sessions not cascaded: %d remain", len(list))
	}
	if _, err := images.Get(ctx, u.ID, KindAvatar); err != ErrImageNotFound {
		t.Errorf("image not cascaded: %v", err)
	}
	if list, _ := creds.ListByUser(ctx, u.ID); len(list) != 0 {
		t.Errorf("passkeys not cascaded: %d remain", len(list))
	}

	// Deleting a non-existent user is ErrUserNotFound.
	if err := users.Delete(ctx, uuid.New()); err != auth.ErrUserNotFound {
		t.Errorf("delete missing user = %v, want ErrUserNotFound", err)
	}
}
