//go:build integration

// Package auth integration tests exercise the user/session/reset stores and the
// HTTP handlers against a real PostgreSQL instance started via testcontainers.
// Run with: go test -tags=integration ./internal/auth/...
// Requires Docker.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"cotton-id/internal/database"
	"cotton-id/internal/httpx"
	"cotton-id/internal/mailer"
	"cotton-id/internal/notify"
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

func newTestService(t *testing.T, pool *pgxpool.Pool) *Service {
	t.Helper()
	return NewService(Config{
		SessionTTL:         time.Hour,
		SessionRememberTTL: 24 * time.Hour,
		PasswordResetTTL:   30 * time.Minute,
		FrontendBaseURL:    "http://localhost:3000",
		Argon2Params:       DefaultArgon2Params(),
	},
		NewUserStore(pool),
		NewSessionStore(pool),
		NewResetTokenStore(pool),
		NewPasswordAuthenticator(NewUserStore(pool), DefaultArgon2Params()),
		mailer.NewLogMailer(observability.NewLogger("error")),
	)
}

func TestUserStoreCRUD(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	store := NewUserStore(pool)

	u, err := store.Create(ctx, CreateUserParams{
		Email: "crud@example.com", Username: "crud", DisplayName: "CRUD", PasswordHash: "x",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Duplicate email / username map to typed errors.
	if _, err := store.Create(ctx, CreateUserParams{Email: "crud@example.com", Username: "other", DisplayName: "X", PasswordHash: "x"}); err != ErrEmailTaken {
		t.Errorf("dup email err = %v, want ErrEmailTaken", err)
	}
	if _, err := store.Create(ctx, CreateUserParams{Email: "other@example.com", Username: "crud", DisplayName: "X", PasswordHash: "x"}); err != ErrUsernameTaken {
		t.Errorf("dup username err = %v, want ErrUsernameTaken", err)
	}

	// citext: email lookup is case-insensitive.
	got, err := store.GetByEmail(ctx, "CRUD@EXAMPLE.COM")
	if err != nil || got.ID != u.ID {
		t.Fatalf("case-insensitive email lookup failed: %v", err)
	}

	if err := store.SetStatus(ctx, u.ID, StatusSuspended); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetByID(ctx, u.ID)
	if got.Status != StatusSuspended {
		t.Errorf("status = %s, want suspended", got.Status)
	}
}

func TestSessionStoreLifecycle(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	users := NewUserStore(pool)
	sessions := NewSessionStore(pool)

	u, _ := users.Create(ctx, CreateUserParams{Email: "sess@example.com", Username: "sess", DisplayName: "S", PasswordHash: "x"})

	token, id, _ := GenerateSessionToken()
	sess := &Session{ID: id, UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := sessions.Create(ctx, sess); err != nil {
		t.Fatal(err)
	}

	got, err := sessions.GetByToken(ctx, token)
	if err != nil || got.UserID != u.ID {
		t.Fatalf("GetByToken failed: %v", err)
	}

	if err := sessions.DeleteByToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.GetByToken(ctx, token); err != ErrSessionNotFound {
		t.Errorf("deleted session err = %v, want ErrSessionNotFound", err)
	}

	// Expired session is not honored.
	token2, id2, _ := GenerateSessionToken()
	_ = sessions.Create(ctx, &Session{ID: id2, UserID: u.ID, ExpiresAt: time.Now().Add(-time.Minute)})
	if _, err := sessions.GetByToken(ctx, token2); err != ErrSessionNotFound {
		t.Errorf("expired session err = %v, want ErrSessionNotFound", err)
	}
}

// TestSessionLastSeenBumpAndThrottle: BumpLastSeen advances a stale session's
// last_seen_at (surfaced via GetByToken/ListByUser), but a second immediate bump
// is a no-op because the SQL throttle only matches a row older than the window.
func TestSessionLastSeenBumpAndThrottle(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	users := NewUserStore(pool)
	sessions := NewSessionStore(pool)

	u, _ := users.Create(ctx, CreateUserParams{Email: "seen@example.com", Username: "seen", DisplayName: "S", PasswordHash: "x"})
	token, id, _ := GenerateSessionToken()
	if err := sessions.Create(ctx, &Session{ID: id, UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Force last_seen_at into the past so the throttle window is exceeded.
	if _, err := pool.Exec(ctx, `UPDATE sessions SET last_seen_at = now() - interval '10 minutes' WHERE id = $1`, id); err != nil {
		t.Fatalf("backdate last_seen_at: %v", err)
	}
	before, err := sessions.GetByToken(ctx, token)
	if err != nil {
		t.Fatalf("get before: %v", err)
	}

	// A stale session is bumped to ~now.
	if err := sessions.BumpLastSeen(ctx, id); err != nil {
		t.Fatalf("bump: %v", err)
	}
	after, _ := sessions.GetByToken(ctx, token)
	if !after.LastSeenAt.After(before.LastSeenAt) {
		t.Fatalf("last_seen_at not advanced: before=%v after=%v", before.LastSeenAt, after.LastSeenAt)
	}
	if time.Since(after.LastSeenAt) > time.Minute {
		t.Errorf("bumped last_seen_at not ~now: %v", after.LastSeenAt)
	}

	// An immediate second bump is throttled (no change): the row is now fresh.
	if err := sessions.BumpLastSeen(ctx, id); err != nil {
		t.Fatalf("second bump: %v", err)
	}
	again, _ := sessions.GetByToken(ctx, token)
	if !again.LastSeenAt.Equal(after.LastSeenAt) {
		t.Errorf("throttle violated: a fresh session was bumped again (%v -> %v)", after.LastSeenAt, again.LastSeenAt)
	}

	// last_seen_at is surfaced through ListByUser too.
	list, _ := sessions.ListByUser(ctx, u.ID)
	if len(list) != 1 || list[0].LastSeenAt.IsZero() {
		t.Errorf("ListByUser did not surface last_seen_at: %+v", list)
	}
}

func TestSignupLoginLogoutFlow(t *testing.T) {
	pool := setupDB(t)
	svc := newTestService(t, pool)
	h := buildAuthRouter(t, svc)

	// Signup.
	rec, cookies := doJSON(t, h, http.MethodPost, "/api/v1/auth/signup", map[string]any{
		"displayName": "Flow User", "username": "flow", "email": "flow@example.com", "password": "Strong-Pass-1!",
	}, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(cookies) == 0 {
		t.Fatal("signup did not set a session cookie")
	}

	// Session check with the signup cookie.
	rec, _ = doJSON(t, h, http.MethodGet, "/api/v1/auth/session", nil, cookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("session status = %d", rec.Code)
	}

	// Login (fresh).
	rec, loginCookies := doJSON(t, h, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "flow@example.com", "password": "Strong-Pass-1!", "remember": true,
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}

	// Wrong password is 401.
	rec, _ = doJSON(t, h, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "flow@example.com", "password": "wrong",
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad login status = %d, want 401", rec.Code)
	}

	// Logout revokes the session.
	rec, _ = doJSON(t, h, http.MethodPost, "/api/v1/auth/logout", nil, loginCookies)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", rec.Code)
	}
	rec, _ = doJSON(t, h, http.MethodGet, "/api/v1/auth/session", nil, loginCookies)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("session after logout = %d, want 401", rec.Code)
	}
}

func TestPasswordResetFlow(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	svc := newTestService(t, pool)

	// Create a user directly.
	hash, _ := HashPassword("Original-1!", DefaultArgon2Params())
	u, _ := NewUserStore(pool).Create(ctx, CreateUserParams{Email: "reset@example.com", Username: "reset", DisplayName: "R", PasswordHash: hash})

	// Forgot for unknown email is a no-op (non-enumerating).
	if err := svc.RequestPasswordReset(ctx, "nobody@example.com"); err != nil {
		t.Fatalf("forgot unknown should succeed silently: %v", err)
	}

	// Issue a real token by inserting directly (mailer would log the link).
	token, thash, _ := GenerateResetToken()
	if err := NewResetTokenStore(pool).Create(ctx, u.ID, thash, time.Now().Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}

	// Reset with the valid token.
	if err := svc.ResetPassword(ctx, token, "Brand-New-2!"); err != nil {
		t.Fatalf("reset: %v", err)
	}

	// Old password no longer works; new one does.
	a := NewPasswordAuthenticator(NewUserStore(pool), DefaultArgon2Params())
	if _, err := a.Authenticate(ctx, Credentials{Identifier: "reset@example.com", Secret: "Original-1!"}); err != ErrInvalidCredentials {
		t.Errorf("old password should fail, got %v", err)
	}
	if _, err := a.Authenticate(ctx, Credentials{Identifier: "reset@example.com", Secret: "Brand-New-2!"}); err != nil {
		t.Errorf("new password should work, got %v", err)
	}

	// Token is single-use: second reset rejected.
	if err := svc.ResetPassword(ctx, token, "Another-3!"); err != ErrResetTokenInvalid {
		t.Errorf("reused token err = %v, want ErrResetTokenInvalid", err)
	}
}

// --- login-notification trigger ---

// countingMailer records each Send for the login-notification assertions. Its
// internal counter is mutex-guarded because the send runs on a goroutine.
type countingMailer struct {
	mu   sync.Mutex
	sent []mailer.Message
}

func (m *countingMailer) Send(_ context.Context, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}
func (m *countingMailer) SendPasswordReset(context.Context, string, string) error { return nil }
func (m *countingMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

func TestLoginNotificationOnNewDevice(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	svc := newTestService(t, pool)
	sessions := NewSessionStore(pool)

	cm := &countingMailer{}
	notifier := notify.NewNotifier(cm, observability.NewLogger("error"), "cotton-id")

	log := observability.NewLogger("error")
	metrics := observability.NewMetrics()
	limiter := httpx.NewTokenBucketLimiter(1000, 1000, time.Minute)
	hs := NewHandlers(svc, log, metrics, limiter, NewMemoryLockout(5), HandlersConfig{
		SessionCookieName: "cid_session", CSRFCookieName: "cid_csrf", CookieSecure: false,
	}).WithLoginNotifier(notifier, sessions)
	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) { hs.Mount(api) })

	// Create a user with login notifications ENABLED.
	hash, _ := HashPassword("Strong-Pass-1!", DefaultArgon2Params())
	u, _ := NewUserStore(pool).Create(ctx, CreateUserParams{
		Email: "notify@example.com", Username: "notifyme", DisplayName: "Notify", PasswordHash: hash,
	})
	if _, err := NewUserStore(pool).UpdatePreferences(ctx, u.ID, UpdatePreferencesParams{
		Theme: "dark", Lang: "en", LoginNotifications: true,
	}); err != nil {
		t.Fatalf("enable login notifications: %v", err)
	}

	// First login from a new device → a notification is sent (best-effort/async).
	rec, _ := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "notify@example.com", "password": "Strong-Pass-1!",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d body=%s", rec.Code, rec.Body.String())
	}
	waitFor(t, func() bool { return cm.count() == 1 })
	if to := cm.sent[0].To; to != "notify@example.com" {
		t.Fatalf("notification To = %q", to)
	}

	// Second login from the SAME device (same UA + IP via httptest default) → NOT
	// new → no additional notification.
	rec, _ = doJSON(t, r, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "notify@example.com", "password": "Strong-Pass-1!",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("second login = %d", rec.Code)
	}
	// Give any (erroneous) async send a moment; the count must stay at 1.
	time.Sleep(200 * time.Millisecond)
	if cm.count() != 1 {
		t.Fatalf("same-device login should not re-notify; sends = %d", cm.count())
	}
}

func TestNoLoginNotificationWhenPreferenceDisabled(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()
	svc := newTestService(t, pool)
	sessions := NewSessionStore(pool)

	cm := &countingMailer{}
	notifier := notify.NewNotifier(cm, observability.NewLogger("error"), "cotton-id")
	hs := NewHandlers(svc, observability.NewLogger("error"), observability.NewMetrics(),
		httpx.NewTokenBucketLimiter(1000, 1000, time.Minute), NewMemoryLockout(5), HandlersConfig{
			SessionCookieName: "cid_session", CSRFCookieName: "cid_csrf", CookieSecure: false,
		}).WithLoginNotifier(notifier, sessions)
	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) { hs.Mount(api) })

	// User with login notifications explicitly DISABLED. The column DEFAULTs to
	// TRUE, so a freshly created account has them ON — the preference must be
	// turned off for this test to prove that a disabled preference suppresses the
	// email (otherwise this first, new-device login would notify).
	hash, _ := HashPassword("Strong-Pass-1!", DefaultArgon2Params())
	u, err := NewUserStore(pool).Create(ctx, CreateUserParams{
		Email: "quiet@example.com", Username: "quiet", DisplayName: "Quiet", PasswordHash: hash,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := NewUserStore(pool).UpdatePreferences(ctx, u.ID, UpdatePreferencesParams{
		Theme: "system", Lang: "ru", LoginNotifications: false,
	}); err != nil {
		t.Fatalf("disable login notifications: %v", err)
	}

	rec, _ := doJSON(t, r, http.MethodPost, "/api/v1/auth/login", map[string]any{
		"email": "quiet@example.com", "password": "Strong-Pass-1!",
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d", rec.Code)
	}
	time.Sleep(200 * time.Millisecond)
	if cm.count() != 0 {
		t.Fatalf("no notification expected when preference disabled; sends = %d", cm.count())
	}
}

// waitFor polls cond up to ~2s for the async notification send to land.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

// --- helpers ---

func buildAuthRouter(t *testing.T, svc *Service) http.Handler {
	t.Helper()
	log := observability.NewLogger("error")
	metrics := observability.NewMetrics()
	limiter := httpx.NewTokenBucketLimiter(1000, 1000, time.Minute)
	lockout := NewMemoryLockout(5)
	hs := NewHandlers(svc, log, metrics, limiter, lockout, HandlersConfig{
		SessionCookieName: "cid_session", CSRFCookieName: "cid_csrf", CookieSecure: false,
	})
	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) {
		hs.Mount(api)
	})
	return r
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any, cookies []*http.Cookie) (*httptest.ResponseRecorder, []*http.Cookie) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, rec.Result().Cookies()
}
