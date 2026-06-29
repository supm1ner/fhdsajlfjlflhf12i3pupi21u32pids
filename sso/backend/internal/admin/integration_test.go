//go:build integration

// Package admin integration tests exercise the admin console store, lifecycle
// service, and HTTP handlers against a real PostgreSQL instance started via
// testcontainers. Run with: go test -tags=integration ./internal/admin/...
// Requires Docker.
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"cotton-id/internal/audit"
	"cotton-id/internal/auth"
	"cotton-id/internal/database"
	"cotton-id/internal/mailer"
	"cotton-id/internal/observability"
	"cotton-id/migrations"
)

const sessionCookie = "cid_session"

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

// testEnv bundles the stores + router the integration tests drive.
type testEnv struct {
	pool     *pgxpool.Pool
	users    *auth.UserStore
	sessions *auth.SessionStore
	authSvc  *auth.Service
	reader   *audit.Reader
	mailer   *recordingMailer
	router   http.Handler
}

// recordingMailer captures the last admin "message user" send for assertion.
type recordingMailer struct {
	last mailer.Message
	sent int
	err  error
}

func (m *recordingMailer) Send(_ context.Context, msg mailer.Message) error {
	m.last = msg
	m.sent++
	return m.err
}

func (m *recordingMailer) SendPasswordReset(context.Context, string, string) error { return nil }

// noopReset satisfies the resetIssuer seam without a mailer dependency for the
// tests that do not exercise the reset path.
type noopReset struct{ issued []uuid.UUID }

func (n *noopReset) IssueResetForUser(_ context.Context, id uuid.UUID) error {
	n.issued = append(n.issued, id)
	return nil
}

// noopHydra satisfies hydraRevoker without a live Hydra.
type noopHydra struct{}

func (noopHydra) RevokeAllConsentSessions(context.Context, string) error { return nil }
func (noopHydra) RevokeLoginSessions(context.Context, string) error      { return nil }

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	pool := setupDB(t)
	log := observability.NewLogger("error")

	users := auth.NewUserStore(pool)
	sessions := auth.NewSessionStore(pool)
	resets := auth.NewResetTokenStore(pool)
	authn := auth.NewPasswordAuthenticator(users, auth.DefaultArgon2Params())
	authSvc := auth.NewService(auth.Config{
		SessionTTL:         time.Hour,
		SessionRememberTTL: 24 * time.Hour,
		PasswordResetTTL:   30 * time.Minute,
		FrontendBaseURL:    "http://localhost:3000",
		Argon2Params:       auth.DefaultArgon2Params(),
	}, users, sessions, resets, authn, mailer.NewLogMailer(log))

	store := NewStore(pool)
	reader := audit.NewReader(pool)
	writer := audit.NewWriter(pool, log)
	svc := NewService(ServiceDeps{
		Users:    users,
		Owners:   store,
		Sessions: sessions,
		Resets:   &noopReset{},
		Hydra:    noopHydra{},
	})

	rec := &recordingMailer{}
	deps := Deps{
		Logger:   log,
		Store:    store,
		Service:  svc,
		Users:    users,
		Sessions: sessions,
		Audit:    writer,
		Journal:  reader,
		Services: nil, // no Hydra in tests; overview reports 0 services
		Mailer:   rec,
	}

	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) {
		api.Group(func(g chi.Router) {
			g.Use(auth.RequireRole(auth.RoleAdmin, authSvc, sessionCookie, log))
			Mount(g, deps)
		})
	})

	return &testEnv{pool: pool, users: users, sessions: sessions, authSvc: authSvc, reader: reader, mailer: rec, router: r}
}

// makeUser inserts a user with the given role/status directly.
func (e *testEnv) makeUser(t *testing.T, username, email, role, status string) *auth.User {
	t.Helper()
	ctx := context.Background()
	hash, _ := auth.HashPassword("Strong-Pass-1!", auth.DefaultArgon2Params())
	u, err := e.users.Create(ctx, auth.CreateUserParams{
		Email: email, Username: username, DisplayName: username, PasswordHash: hash,
	})
	if err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	if role != auth.RoleUser {
		if err := e.users.SetRole(ctx, u.ID, role); err != nil {
			t.Fatalf("set role: %v", err)
		}
		u.Role = role
	}
	if status != auth.StatusActive {
		if err := e.users.SetStatus(ctx, u.ID, status); err != nil {
			t.Fatalf("set status: %v", err)
		}
		u.Status = status
	}
	return u
}

// sessionFor mints a real session for a user and returns the cookie token.
func (e *testEnv) sessionFor(t *testing.T, u *auth.User) string {
	t.Helper()
	es, err := e.authSvc.EstablishSession(context.Background(), u.ID, false, "test", "127.0.0.1")
	if err != nil {
		t.Fatalf("establish session: %v", err)
	}
	return es.Token
}

// do issues a request as the given actor (session cookie) and returns the recorder.
func (e *testEnv) do(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

// --- RBAC gate ---

func TestAdminGateRejectsNonAdmin(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "plainuser", "user@example.com", auth.RoleUser, auth.StatusActive)
	tok := e.sessionFor(t, user)

	if rec := e.do(t, http.MethodGet, "/api/v1/admin/overview", tok, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin overview = %d, want 403", rec.Code)
	}
	// Anonymous → 401.
	if rec := e.do(t, http.MethodGet, "/api/v1/admin/overview", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous overview = %d, want 401", rec.Code)
	}
}

// --- user listing ---

func TestListUsersFilterSearchPaginate(t *testing.T) {
	e := newEnv(t)
	admin := e.makeUser(t, "boss", "boss@example.com", auth.RoleOwner, auth.StatusActive)
	tok := e.sessionFor(t, admin)

	e.makeUser(t, "alice", "alice@example.com", auth.RoleUser, auth.StatusActive)
	e.makeUser(t, "alex", "alex@example.com", auth.RoleAdmin, auth.StatusActive)
	e.makeUser(t, "bob", "bob@example.com", auth.RoleUser, auth.StatusSuspended)

	// Search "al" matches alice + alex (case-insensitive).
	rec := e.do(t, http.MethodGet, "/api/v1/admin/users?query=AL", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp usersResponse
	mustJSON(t, rec, &resp)
	if resp.Total < 2 {
		t.Fatalf("search 'al' total = %d, want >= 2", resp.Total)
	}
	for _, u := range resp.Users {
		if u.Username != "alice" && u.Username != "alex" {
			t.Fatalf("unexpected user %q in 'al' search", u.Username)
		}
	}

	// Status filter: only the suspended user.
	rec = e.do(t, http.MethodGet, "/api/v1/admin/users?status=suspended", tok, nil)
	mustJSON(t, rec, &resp)
	if resp.Total != 1 || resp.Users[0].Username != "bob" {
		t.Fatalf("suspended filter = %+v", resp.Users)
	}

	// Role filter: only the owner (boss) + admin (alex).
	rec = e.do(t, http.MethodGet, "/api/v1/admin/users?role=admin", tok, nil)
	mustJSON(t, rec, &resp)
	if resp.Total != 1 || resp.Users[0].Username != "alex" {
		t.Fatalf("admin role filter = %+v", resp.Users)
	}

	// Pagination: pageSize 2 returns 2 of the 4 users; total reflects all.
	rec = e.do(t, http.MethodGet, "/api/v1/admin/users?pageSize=2&page=1", tok, nil)
	mustJSON(t, rec, &resp)
	if len(resp.Users) != 2 || resp.Total != 4 {
		t.Fatalf("page 1 size 2: users=%d total=%d, want 2/4", len(resp.Users), resp.Total)
	}

	// Bad status filter → 400.
	if rec := e.do(t, http.MethodGet, "/api/v1/admin/users?status=bogus", tok, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad status filter = %d, want 400", rec.Code)
	}
}

// --- suspend revokes sessions + audits ---

func TestSuspendRevokesSessionsAndAudits(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.makeUser(t, "owner1", "owner1@example.com", auth.RoleOwner, auth.StatusActive)
	tok := e.sessionFor(t, admin)
	victim := e.makeUser(t, "victim", "victim@example.com", auth.RoleUser, auth.StatusActive)
	victimTok := e.sessionFor(t, victim)

	// Victim has a live session before suspension.
	if _, err := e.sessions.GetByToken(ctx, victimTok); err != nil {
		t.Fatalf("victim session should exist pre-suspend: %v", err)
	}

	rec := e.do(t, http.MethodPost, "/api/v1/admin/users/"+victim.ID.String()+"/suspend", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("suspend = %d body=%s", rec.Code, rec.Body.String())
	}

	// Status flipped.
	got, _ := e.users.GetByID(ctx, victim.ID)
	if got.Status != auth.StatusSuspended {
		t.Fatalf("status = %q, want suspended", got.Status)
	}
	// Sessions revoked.
	if _, err := e.sessions.GetByToken(ctx, victimTok); err != auth.ErrSessionNotFound {
		t.Fatalf("victim session after suspend err = %v, want ErrSessionNotFound", err)
	}
	// Audit entry written with actor + target.
	entries, _, err := e.reader.Query(ctx, audit.Filter{Action: ActionUserSuspend})
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("suspend audit entries = %d, want 1", len(entries))
	}
	if entries[0].ActorID == nil || *entries[0].ActorID != admin.ID {
		t.Fatalf("audit actor = %v, want %v", entries[0].ActorID, admin.ID)
	}
	if entries[0].TargetID != victim.ID.String() {
		t.Fatalf("audit target = %q, want %q", entries[0].TargetID, victim.ID.String())
	}

	// Reactivate restores the account.
	rec = e.do(t, http.MethodPost, "/api/v1/admin/users/"+victim.ID.String()+"/reactivate", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reactivate = %d body=%s", rec.Code, rec.Body.String())
	}
	got, _ = e.users.GetByID(ctx, victim.ID)
	if got.Status != auth.StatusActive {
		t.Fatalf("status after reactivate = %q, want active", got.Status)
	}
}

// --- message user sends mail + audits + validates ---

func TestMessageUserSendsAndAudits(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.makeUser(t, "msgadmin", "msgadmin@example.com", auth.RoleAdmin, auth.StatusActive)
	tok := e.sessionFor(t, admin)
	target := e.makeUser(t, "msgtarget", "msgtarget@example.com", auth.RoleUser, auth.StatusActive)

	body := messageUserRequest{Subject: "Hello there", Body: "Please verify your email."}
	rec := e.do(t, http.MethodPost, "/api/v1/admin/users/"+target.ID.String()+"/message", tok, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("message = %d body=%s", rec.Code, rec.Body.String())
	}

	// The mail went to the target's address with the supplied subject/body.
	if e.mailer.sent != 1 {
		t.Fatalf("mailer sent = %d, want 1", e.mailer.sent)
	}
	if e.mailer.last.To != target.Email {
		t.Fatalf("mail To = %q, want %q", e.mailer.last.To, target.Email)
	}
	if e.mailer.last.Subject != "Hello there" || e.mailer.last.Body != "Please verify your email." {
		t.Fatalf("mail content = %+v", e.mailer.last)
	}

	// Audited with actor + target + delivered=true.
	entries, _, err := e.reader.Query(ctx, audit.Filter{Action: ActionUserMessage})
	if err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("message audit entries = %d, want 1", len(entries))
	}
	if entries[0].ActorID == nil || *entries[0].ActorID != admin.ID {
		t.Fatalf("audit actor = %v, want %v", entries[0].ActorID, admin.ID)
	}
	if entries[0].TargetID != target.ID.String() {
		t.Fatalf("audit target = %q, want %q", entries[0].TargetID, target.ID.String())
	}
	if d, ok := entries[0].Metadata["delivered"].(bool); !ok || !d {
		t.Fatalf("audit metadata delivered = %v, want true", entries[0].Metadata["delivered"])
	}
}

func TestMessageUserBestEffortOnMailFailure(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.mailer.err = context.DeadlineExceeded // simulate a delivery failure
	admin := e.makeUser(t, "msgadmin2", "msgadmin2@example.com", auth.RoleAdmin, auth.StatusActive)
	tok := e.sessionFor(t, admin)
	target := e.makeUser(t, "msgtarget2", "msgtarget2@example.com", auth.RoleUser, auth.StatusActive)

	// A delivery failure must NOT fail the action (best-effort): still 200 + audited.
	rec := e.do(t, http.MethodPost, "/api/v1/admin/users/"+target.ID.String()+"/message", tok,
		messageUserRequest{Body: "hi"})
	if rec.Code != http.StatusOK {
		t.Fatalf("message on mail failure = %d, want 200 (best-effort)", rec.Code)
	}
	entries, _, _ := e.reader.Query(ctx, audit.Filter{Action: ActionUserMessage})
	if len(entries) != 1 {
		t.Fatalf("message audit entries = %d, want 1", len(entries))
	}
	if d, ok := entries[0].Metadata["delivered"].(bool); !ok || d {
		t.Fatalf("audit metadata delivered = %v, want false", entries[0].Metadata["delivered"])
	}
}

func TestMessageUserValidatesBody(t *testing.T) {
	e := newEnv(t)
	admin := e.makeUser(t, "msgadmin3", "msgadmin3@example.com", auth.RoleAdmin, auth.StatusActive)
	tok := e.sessionFor(t, admin)
	target := e.makeUser(t, "msgtarget3", "msgtarget3@example.com", auth.RoleUser, auth.StatusActive)

	// Empty body → 400, no mail sent.
	rec := e.do(t, http.MethodPost, "/api/v1/admin/users/"+target.ID.String()+"/message", tok,
		messageUserRequest{Subject: "x", Body: "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty body = %d, want 400", rec.Code)
	}
	if e.mailer.sent != 0 {
		t.Fatalf("mailer should not send on validation failure, sent=%d", e.mailer.sent)
	}

	// Unknown target → 404.
	rec = e.do(t, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/message", tok,
		messageUserRequest{Body: "hi"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("message unknown target = %d, want 404", rec.Code)
	}
}

// --- suspend guards via HTTP ---

func TestSuspendGuards(t *testing.T) {
	e := newEnv(t)
	admin := e.makeUser(t, "adm", "adm@example.com", auth.RoleAdmin, auth.StatusActive)
	tok := e.sessionFor(t, admin)
	owner := e.makeUser(t, "own", "own@example.com", auth.RoleOwner, auth.StatusActive)

	// Cannot suspend self → 409.
	if rec := e.do(t, http.MethodPost, "/api/v1/admin/users/"+admin.ID.String()+"/suspend", tok, nil); rec.Code != http.StatusConflict {
		t.Fatalf("self-suspend = %d, want 409", rec.Code)
	}
	// Admin cannot suspend an owner → 403.
	if rec := e.do(t, http.MethodPost, "/api/v1/admin/users/"+owner.ID.String()+"/suspend", tok, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("admin suspends owner = %d, want 403", rec.Code)
	}
	// Unknown id → 404.
	if rec := e.do(t, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/suspend", tok, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("suspend unknown = %d, want 404", rec.Code)
	}
}

// --- role change guards via HTTP (owner-only) ---

func TestRoleChangeGuards(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := e.makeUser(t, "owner2", "owner2@example.com", auth.RoleOwner, auth.StatusActive)
	ownerTok := e.sessionFor(t, owner)
	admin := e.makeUser(t, "admin2", "admin2@example.com", auth.RoleAdmin, auth.StatusActive)
	adminTok := e.sessionFor(t, admin)
	target := e.makeUser(t, "u2", "u2@example.com", auth.RoleUser, auth.StatusActive)

	// An admin cannot change roles → 403.
	rec := e.do(t, http.MethodPatch, "/api/v1/admin/users/"+target.ID.String()+"/role", adminTok, changeRoleRequest{Role: auth.RoleAdmin})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin role change = %d, want 403", rec.Code)
	}

	// The owner grants admin → 200, persisted.
	rec = e.do(t, http.MethodPatch, "/api/v1/admin/users/"+target.ID.String()+"/role", ownerTok, changeRoleRequest{Role: auth.RoleAdmin})
	if rec.Code != http.StatusOK {
		t.Fatalf("owner grant admin = %d body=%s", rec.Code, rec.Body.String())
	}
	got, _ := e.users.GetByID(ctx, target.ID)
	if got.Role != auth.RoleAdmin {
		t.Fatalf("role = %q, want admin", got.Role)
	}

	// The owner cannot demote themselves (would risk last owner / self) → 409.
	rec = e.do(t, http.MethodPatch, "/api/v1/admin/users/"+owner.ID.String()+"/role", ownerTok, changeRoleRequest{Role: auth.RoleAdmin})
	if rec.Code != http.StatusConflict {
		t.Fatalf("owner self-demote = %d, want 409", rec.Code)
	}

	// Invalid role → 400.
	rec = e.do(t, http.MethodPatch, "/api/v1/admin/users/"+target.ID.String()+"/role", ownerTok, changeRoleRequest{Role: "superuser"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid role = %d, want 400", rec.Code)
	}
}

// --- delete guards via HTTP (owner-only, last owner, self) ---

func TestDeleteGuards(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := e.makeUser(t, "owner3", "owner3@example.com", auth.RoleOwner, auth.StatusActive)
	ownerTok := e.sessionFor(t, owner)
	admin := e.makeUser(t, "admin3", "admin3@example.com", auth.RoleAdmin, auth.StatusActive)
	adminTok := e.sessionFor(t, admin)
	target := e.makeUser(t, "u3", "u3@example.com", auth.RoleUser, auth.StatusActive)

	// Admin cannot delete → 403.
	if rec := e.do(t, http.MethodDelete, "/api/v1/admin/users/"+target.ID.String(), adminTok, nil); rec.Code != http.StatusForbidden {
		t.Fatalf("admin delete = %d, want 403", rec.Code)
	}
	// Owner cannot delete the last owner (only one owner exists) → 409.
	if rec := e.do(t, http.MethodDelete, "/api/v1/admin/users/"+owner.ID.String(), ownerTok, nil); rec.Code != http.StatusConflict {
		t.Fatalf("delete last owner = %d, want 409", rec.Code)
	}
	// Owner deletes a regular user → 204, row gone, audited.
	rec := e.do(t, http.MethodDelete, "/api/v1/admin/users/"+target.ID.String(), ownerTok, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("owner delete user = %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := e.users.GetByID(ctx, target.ID); err != auth.ErrUserNotFound {
		t.Fatalf("deleted user lookup err = %v, want ErrUserNotFound", err)
	}
	entries, _, _ := e.reader.Query(ctx, audit.Filter{Action: ActionUserDelete})
	if len(entries) != 1 || entries[0].TargetID != target.ID.String() {
		t.Fatalf("delete audit = %+v", entries)
	}
}

// --- user detail ---

func TestUserDetail(t *testing.T) {
	e := newEnv(t)
	admin := e.makeUser(t, "owner4", "owner4@example.com", auth.RoleOwner, auth.StatusActive)
	tok := e.sessionFor(t, admin)
	target := e.makeUser(t, "u4", "u4@example.com", auth.RoleUser, auth.StatusActive)
	_ = e.sessionFor(t, target) // give the target a session to show in detail

	rec := e.do(t, http.MethodGet, "/api/v1/admin/users/"+target.ID.String(), tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp userDetailResponse
	mustJSON(t, rec, &resp)
	if resp.User.ID != target.ID.String() {
		t.Fatalf("detail user id = %q, want %q", resp.User.ID, target.ID.String())
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("detail sessions = %d, want 1", len(resp.Sessions))
	}
}

// --- overview aggregates ---

func TestOverviewAggregates(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.makeUser(t, "owner5", "owner5@example.com", auth.RoleOwner, auth.StatusActive)
	tok := e.sessionFor(t, admin)
	e.makeUser(t, "u5a", "u5a@example.com", auth.RoleUser, auth.StatusActive)
	e.makeUser(t, "u5b", "u5b@example.com", auth.RoleUser, auth.StatusActive)

	// Write a login-ok audit entry so active-today is non-zero.
	w := audit.NewWriter(e.pool, observability.NewLogger("error"))
	_ = w.Append(ctx, audit.Entry{Action: audit.ActionLoginOK}.WithActor(admin.ID, admin.Username))

	rec := e.do(t, http.MethodGet, "/api/v1/admin/overview", tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("overview = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp overviewResponse
	mustJSON(t, rec, &resp)
	if resp.TotalUsers != 3 {
		t.Fatalf("totalUsers = %d, want 3", resp.TotalUsers)
	}
	if resp.NewThisWeek != 3 {
		t.Fatalf("newThisWeek = %d, want 3", resp.NewThisWeek)
	}
	if resp.ActiveToday < 1 {
		t.Fatalf("activeToday = %d, want >= 1", resp.ActiveToday)
	}
	if len(resp.Signups) != 30 {
		t.Fatalf("signups series len = %d, want 30", len(resp.Signups))
	}
	if len(resp.RecentSignups) == 0 {
		t.Fatal("expected recent signups")
	}
}

// --- audit Journal query via HTTP ---

func TestAuditJournalQuery(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.makeUser(t, "owner6", "owner6@example.com", auth.RoleOwner, auth.StatusActive)
	tok := e.sessionFor(t, admin)
	target := e.makeUser(t, "u6", "u6@example.com", auth.RoleUser, auth.StatusActive)

	// Generate an auditable action.
	if rec := e.do(t, http.MethodPost, "/api/v1/admin/users/"+target.ID.String()+"/suspend", tok, nil); rec.Code != http.StatusOK {
		t.Fatalf("suspend setup = %d", rec.Code)
	}

	// Journal filtered by action returns the suspend entry.
	rec := e.do(t, http.MethodGet, "/api/v1/admin/audit?action="+ActionUserSuspend, tok, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("audit = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp auditResponse
	mustJSON(t, rec, &resp)
	if resp.Total != 1 || len(resp.Entries) != 1 {
		t.Fatalf("audit by action total=%d entries=%d, want 1/1", resp.Total, len(resp.Entries))
	}
	if resp.Entries[0].Action != ActionUserSuspend {
		t.Fatalf("entry action = %q, want %q", resp.Entries[0].Action, ActionUserSuspend)
	}

	// Filter by actor returns the same row.
	rec = e.do(t, http.MethodGet, "/api/v1/admin/audit?actor="+admin.ID.String(), tok, nil)
	mustJSON(t, rec, &resp)
	if resp.Total < 1 {
		t.Fatalf("audit by actor total = %d, want >= 1", resp.Total)
	}
	_ = ctx
}

func mustJSON(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, rec.Body.String())
	}
}
