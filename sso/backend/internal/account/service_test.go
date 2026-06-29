package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"cotton-id/internal/auth"
)

// --- fakes for the service seams ---

type fakeUserStore struct {
	user           *auth.User
	updatedHash    string
	deleted        bool
	imageURLByKind map[string]string
}

func (f *fakeUserStore) GetByID(_ context.Context, _ uuid.UUID) (*auth.User, error) {
	return f.user, nil
}
func (f *fakeUserStore) UpdateProfile(_ context.Context, _ uuid.UUID, p auth.UpdateProfileParams) (*auth.User, error) {
	f.user.DisplayName = p.DisplayName
	f.user.About = p.About
	f.user.Location = p.Location
	return f.user, nil
}
func (f *fakeUserStore) UpdatePreferences(_ context.Context, _ uuid.UUID, p auth.UpdatePreferencesParams) (*auth.User, error) {
	f.user.PrefTheme = p.Theme
	f.user.PrefLang = p.Lang
	f.user.LoginNotifications = p.LoginNotifications
	return f.user, nil
}
func (f *fakeUserStore) UpdatePassword(_ context.Context, _ uuid.UUID, hash string) error {
	f.updatedHash = hash
	return nil
}
func (f *fakeUserStore) SetImageURL(_ context.Context, _ uuid.UUID, kind, urlValue string) error {
	if f.imageURLByKind == nil {
		f.imageURLByKind = map[string]string{}
	}
	f.imageURLByKind[kind] = urlValue
	return nil
}
func (f *fakeUserStore) Delete(_ context.Context, _ uuid.UUID) error {
	f.deleted = true
	return nil
}

type fakeSessionStore struct {
	rows            []auth.Session
	deletedID       string
	deletedScopeErr error
	exceptKeep      string
	exceptCount     int64
}

func (f *fakeSessionStore) ListByUser(_ context.Context, _ uuid.UUID) ([]auth.Session, error) {
	return f.rows, nil
}
func (f *fakeSessionStore) DeleteForUser(_ context.Context, _ uuid.UUID, id string) error {
	if f.deletedScopeErr != nil {
		return f.deletedScopeErr
	}
	f.deletedID = id
	return nil
}
func (f *fakeSessionStore) DeleteByUserExcept(_ context.Context, _ uuid.UUID, keepID string) (int64, error) {
	f.exceptKeep = keepID
	return f.exceptCount, nil
}

type fakePasskeyLister struct{ count int }

func (f fakePasskeyLister) CountByUser(_ context.Context, _ uuid.UUID) (int, error) {
	return f.count, nil
}

type fakeConsent struct {
	records      []consentRecord
	listErr      error
	revokedOne   string
	revokedAll   bool
	revokedLogin bool
}

func (f *fakeConsent) ListConsentSessions(_ context.Context, _ string) ([]consentRecord, error) {
	return f.records, f.listErr
}
func (f *fakeConsent) RevokeConsentSessions(_ context.Context, _, client string) error {
	f.revokedOne = client
	return nil
}
func (f *fakeConsent) RevokeAllConsentSessions(_ context.Context, _ string) error {
	f.revokedAll = true
	return nil
}
func (f *fakeConsent) RevokeLoginSessions(_ context.Context, _ string) error {
	f.revokedLogin = true
	return nil
}

// fakeAuthn passes only when the secret equals correctPassword.
type fakeAuthn struct {
	correctPassword string
	user            *auth.User
}

func (f fakeAuthn) Authenticate(_ context.Context, cred auth.Credentials) (*auth.User, error) {
	if cred.Secret != f.correctPassword {
		return nil, auth.ErrInvalidCredentials
	}
	return f.user, nil
}

// fakeHasher accepts any new password >= 8 chars and "hashes" by prefixing.
type fakeHasher struct{}

func (fakeHasher) Validate(pw string) error {
	if len(pw) < 8 {
		return auth.ErrPasswordTooShort
	}
	return nil
}
func (fakeHasher) Hash(pw string) (string, error) { return "hashed:" + pw, nil }

func ptr(s string) *string { return &s }

func newTestService(t *testing.T) (*Service, *fakeUserStore, *fakeSessionStore, *fakeConsent) {
	t.Helper()
	user := &auth.User{
		ID:           uuid.New(),
		Email:        "user@example.com",
		Username:     "user",
		DisplayName:  "User",
		PasswordHash: ptr("$argon2id$existing"),
		Status:       auth.StatusActive,
		Role:         "user",
		PrefTheme:    "system",
		PrefLang:     "ru",
	}
	us := &fakeUserStore{user: user}
	ss := &fakeSessionStore{}
	cs := &fakeConsent{}
	svc := newService(serviceDeps{
		Users:    us,
		Sessions: ss,
		Passkeys: fakePasskeyLister{count: 2},
		Consent:  cs,
		Authn:    fakeAuthn{correctPassword: "current-Pass-1!", user: user},
		Hasher:   fakeHasher{},
	})
	return svc, us, ss, cs
}

func TestGetProfileCounts(t *testing.T) {
	t.Parallel()
	svc, _, ss, cs := newTestService(t)
	ss.rows = []auth.Session{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	cs.records = []consentRecord{{ClientID: "app1"}}

	prof, err := svc.GetProfile(context.Background(), svc.usersUser())
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if prof.Counts.Sessions != 3 {
		t.Errorf("sessions count = %d, want 3", prof.Counts.Sessions)
	}
	if prof.Counts.Passkeys != 2 {
		t.Errorf("passkeys count = %d, want 2", prof.Counts.Passkeys)
	}
	if prof.Counts.Connections != 1 {
		t.Errorf("connections count = %d, want 1", prof.Counts.Connections)
	}
}

func TestGetProfileToleratesHydraError(t *testing.T) {
	t.Parallel()
	svc, _, ss, cs := newTestService(t)
	ss.rows = []auth.Session{{ID: "a"}}
	cs.listErr = errors.New("hydra down")

	prof, err := svc.GetProfile(context.Background(), svc.usersUser())
	if err != nil {
		t.Fatalf("GetProfile must tolerate a Hydra error, got %v", err)
	}
	if prof.Counts.Connections != 0 {
		t.Errorf("connections count on hydra error = %d, want 0", prof.Counts.Connections)
	}
	if prof.Counts.Sessions != 1 {
		t.Errorf("sessions count = %d, want 1", prof.Counts.Sessions)
	}
}

// usersUser is a tiny test accessor: the fake user store always returns the seeded
// user, which the handler would have resolved from the session.
func (s *Service) usersUser() *auth.User {
	u, _ := s.users.GetByID(context.Background(), uuid.Nil)
	return u
}

func TestUpdateProfileValidation(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTestService(t)
	user := svc.usersUser()

	// Blank display name rejected.
	if _, err := svc.UpdateProfile(context.Background(), user, UpdateProfileInput{DisplayName: "  "}); !errors.Is(err, ErrDisplayNameRequired) {
		t.Errorf("blank name err = %v, want ErrDisplayNameRequired", err)
	}
	// Over-long display name rejected.
	long := strings.Repeat("x", maxDisplayNameLen+1)
	if _, err := svc.UpdateProfile(context.Background(), user, UpdateProfileInput{DisplayName: long}); !errors.Is(err, ErrDisplayNameTooLong) {
		t.Errorf("long name err = %v, want ErrDisplayNameTooLong", err)
	}
	// Over-long about rejected.
	if _, err := svc.UpdateProfile(context.Background(), user, UpdateProfileInput{DisplayName: "ok", About: strings.Repeat("a", maxAboutLen+1)}); !errors.Is(err, ErrAboutTooLong) {
		t.Errorf("long about err = %v, want ErrAboutTooLong", err)
	}
	// Valid update trims and saves.
	updated, err := svc.UpdateProfile(context.Background(), user, UpdateProfileInput{DisplayName: "  Alex  ", About: " hi ", Location: " Almaty "})
	if err != nil {
		t.Fatalf("valid update: %v", err)
	}
	if updated.DisplayName != "Alex" || updated.About != "hi" || updated.Location != "Almaty" {
		t.Errorf("update not trimmed/saved: %+v", updated)
	}
}

func TestUpdatePreferencesValidation(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTestService(t)
	user := svc.usersUser()

	if _, err := svc.UpdatePreferences(context.Background(), user, UpdatePreferencesInput{Theme: "neon", Lang: "ru"}); !errors.Is(err, ErrInvalidTheme) {
		t.Errorf("bad theme err = %v, want ErrInvalidTheme", err)
	}
	if _, err := svc.UpdatePreferences(context.Background(), user, UpdatePreferencesInput{Theme: "dark", Lang: "de"}); !errors.Is(err, ErrInvalidLang) {
		t.Errorf("bad lang err = %v, want ErrInvalidLang", err)
	}
	updated, err := svc.UpdatePreferences(context.Background(), user, UpdatePreferencesInput{Theme: "light", Lang: "en", LoginNotifications: false})
	if err != nil {
		t.Fatalf("valid prefs: %v", err)
	}
	if updated.PrefTheme != "light" || updated.PrefLang != "en" || updated.LoginNotifications {
		t.Errorf("prefs not saved: %+v", updated)
	}
}

func TestChangePasswordWrongCurrentRejected(t *testing.T) {
	t.Parallel()
	svc, us, ss, _ := newTestService(t)
	user := svc.usersUser()

	_, err := svc.ChangePassword(context.Background(), user, "wrong-Pass", "new-Pass-2!", "current-sess")
	if !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong current err = %v, want ErrWrongPassword", err)
	}
	if us.updatedHash != "" {
		t.Error("password must not be updated on wrong current password")
	}
	if ss.exceptKeep != "" {
		t.Error("no sessions should be revoked on a rejected change")
	}
}

func TestChangePasswordRevokesOthers(t *testing.T) {
	t.Parallel()
	svc, us, ss, _ := newTestService(t)
	user := svc.usersUser()
	ss.exceptCount = 3

	revoked, err := svc.ChangePassword(context.Background(), user, "current-Pass-1!", "new-Pass-2!", "current-sess-id")
	if err != nil {
		t.Fatalf("valid change: %v", err)
	}
	if revoked != 3 {
		t.Errorf("revoked = %d, want 3", revoked)
	}
	if us.updatedHash != "hashed:new-Pass-2!" {
		t.Errorf("updated hash = %q, want hashed:new-Pass-2!", us.updatedHash)
	}
	if ss.exceptKeep != "current-sess-id" {
		t.Errorf("kept session = %q, want current-sess-id (current device stays signed in)", ss.exceptKeep)
	}
}

func TestChangePasswordWeakNewRejected(t *testing.T) {
	t.Parallel()
	svc, us, _, _ := newTestService(t)
	user := svc.usersUser()
	if _, err := svc.ChangePassword(context.Background(), user, "current-Pass-1!", "short", "s"); !errors.Is(err, auth.ErrPasswordTooShort) {
		t.Fatalf("weak new err = %v, want ErrPasswordTooShort", err)
	}
	if us.updatedHash != "" {
		t.Error("password must not change when the new one fails policy")
	}
}

func TestChangePasswordPasswordlessAccount(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTestService(t)
	user := svc.usersUser()
	user.PasswordHash = nil // social/passkey-only
	if _, err := svc.ChangePassword(context.Background(), user, "anything", "new-Pass-2!", "s"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("passwordless change err = %v, want ErrWrongPassword", err)
	}
}

func TestListSessionsCurrentFlag(t *testing.T) {
	t.Parallel()
	svc, _, ss, _ := newTestService(t)
	user := svc.usersUser()
	now := time.Now()
	ss.rows = []auth.Session{
		{ID: "sess-a", UserAgent: "Safari", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{ID: "sess-b", UserAgent: "Chrome", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	}
	views, err := svc.ListSessions(context.Background(), user, "sess-b")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("len = %d, want 2", len(views))
	}
	var current, other *SessionView
	for i := range views {
		if views[i].ID == "sess-b" {
			current = &views[i]
		} else {
			other = &views[i]
		}
	}
	if current == nil || !current.Current {
		t.Error("sess-b should be flagged current")
	}
	if other == nil || other.Current {
		t.Error("sess-a should not be flagged current")
	}
}

func TestRevokeSessionScoped(t *testing.T) {
	t.Parallel()
	svc, _, ss, _ := newTestService(t)
	user := svc.usersUser()
	if err := svc.RevokeSession(context.Background(), user, "sess-x"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if ss.deletedID != "sess-x" {
		t.Errorf("deleted id = %q, want sess-x", ss.deletedID)
	}
	// A not-found (or cross-user) revoke surfaces ErrSessionNotFound.
	ss.deletedScopeErr = auth.ErrSessionNotFound
	if err := svc.RevokeSession(context.Background(), user, "other"); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Errorf("scoped revoke err = %v, want ErrSessionNotFound", err)
	}
}

func TestListConnectionsProjection(t *testing.T) {
	t.Parallel()
	svc, _, _, cs := newTestService(t)
	user := svc.usersUser()
	cs.records = []consentRecord{
		{ClientID: "app1", ClientName: "App One", GrantScope: []string{"openid", "profile"}, HandledAt: "2026-01-01T00:00:00Z"},
	}
	conns, err := svc.ListConnections(context.Background(), user)
	if err != nil {
		t.Fatalf("list connections: %v", err)
	}
	if len(conns) != 1 || conns[0].ClientID != "app1" || conns[0].ClientName != "App One" {
		t.Fatalf("projection wrong: %+v", conns)
	}
	if len(conns[0].GrantedScopes) != 2 {
		t.Errorf("scopes = %v, want 2", conns[0].GrantedScopes)
	}
}

func TestRevokeConnection(t *testing.T) {
	t.Parallel()
	svc, _, _, cs := newTestService(t)
	user := svc.usersUser()
	if err := svc.RevokeConnection(context.Background(), user, "app1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if cs.revokedOne != "app1" {
		t.Errorf("revoked client = %q, want app1", cs.revokedOne)
	}
}

func TestDeleteAccountRequiresReauth(t *testing.T) {
	t.Parallel()
	svc, us, _, _ := newTestService(t)
	user := svc.usersUser()

	// Password account, no password supplied → re-auth required.
	if err := svc.DeleteAccount(context.Background(), user, DeleteAccountInput{}); !errors.Is(err, ErrReauthRequired) {
		t.Errorf("missing password err = %v, want ErrReauthRequired", err)
	}
	// Wrong password → rejected.
	if err := svc.DeleteAccount(context.Background(), user, DeleteAccountInput{CurrentPassword: "nope"}); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("wrong password err = %v, want ErrWrongPassword", err)
	}
	if us.deleted {
		t.Fatal("account must not be deleted without valid re-auth")
	}
}

func TestDeleteAccountSuccess(t *testing.T) {
	t.Parallel()
	svc, us, _, cs := newTestService(t)
	user := svc.usersUser()

	if err := svc.DeleteAccount(context.Background(), user, DeleteAccountInput{CurrentPassword: "current-Pass-1!"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !us.deleted {
		t.Error("account should be deleted")
	}
	if !cs.revokedAll || !cs.revokedLogin {
		t.Error("Hydra consent + login sessions should be best-effort revoked on delete")
	}
}

func TestDeleteAccountPasswordlessConfirm(t *testing.T) {
	t.Parallel()
	svc, us, _, _ := newTestService(t)
	user := svc.usersUser()
	user.PasswordHash = nil // social/passkey-only

	// No confirm → re-auth required.
	if err := svc.DeleteAccount(context.Background(), user, DeleteAccountInput{}); !errors.Is(err, ErrReauthRequired) {
		t.Errorf("passwordless no-confirm err = %v, want ErrReauthRequired", err)
	}
	if us.deleted {
		t.Fatal("must not delete a passwordless account without confirm")
	}
	// Confirm=true → deletes.
	if err := svc.DeleteAccount(context.Background(), user, DeleteAccountInput{Confirm: true}); err != nil {
		t.Fatalf("passwordless confirmed delete: %v", err)
	}
	if !us.deleted {
		t.Error("passwordless account with confirm should be deleted")
	}
}
