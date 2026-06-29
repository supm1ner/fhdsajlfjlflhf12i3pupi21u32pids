package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// --- a minimal in-memory fake of PgxPool sufficient for authenticator tests ---

// fakeRow returns either a prepared user (in userColumns order) or ErrNoRows.
type fakeRow struct {
	user *User
	err  error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	u := r.user
	// Order must match userColumns / scanUser.
	*(dest[0].(*uuid.UUID)) = u.ID
	*(dest[1].(*string)) = u.Email
	*(dest[2].(*bool)) = u.EmailVerified
	*(dest[3].(*string)) = u.Username
	*(dest[4].(*string)) = u.DisplayName
	*(dest[5].(**string)) = u.PasswordHash
	*(dest[6].(*string)) = u.Status
	*(dest[7].(*string)) = u.Role
	*(dest[8].(*string)) = u.About
	*(dest[9].(*string)) = u.Location
	*(dest[10].(**string)) = u.AvatarURL
	*(dest[11].(**string)) = u.BannerURL
	*(dest[12].(*string)) = u.PrefTheme
	*(dest[13].(*string)) = u.PrefLang
	*(dest[14].(*bool)) = u.LoginNotifications
	// created_at, updated_at left zero-valued.
	return nil
}

// fakePool implements PgxPool; only QueryRow is exercised by Authenticate.
type fakePool struct {
	byEmail map[string]*User
}

func (p *fakePool) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (p *fakePool) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}
func (p *fakePool) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	email, _ := args[0].(string)
	u, ok := p.byEmail[email]
	if !ok {
		return fakeRow{err: pgx.ErrNoRows}
	}
	return fakeRow{user: u}
}

func ptr(s string) *string { return &s }

func newTestAuthenticator(t *testing.T) (*PasswordAuthenticator, *fakePool, string) {
	t.Helper()
	hash, err := HashPassword("Valid-Pass-1!", DefaultArgon2Params())
	if err != nil {
		t.Fatal(err)
	}
	pool := &fakePool{byEmail: map[string]*User{
		"active@example.com": {
			ID: uuid.New(), Email: "active@example.com", Username: "active",
			DisplayName: "Active", PasswordHash: ptr(hash), Status: StatusActive, Role: "user",
		},
		"suspended@example.com": {
			ID: uuid.New(), Email: "suspended@example.com", Username: "susp",
			DisplayName: "Susp", PasswordHash: ptr(hash), Status: StatusSuspended, Role: "user",
		},
		"social@example.com": {
			ID: uuid.New(), Email: "social@example.com", Username: "social",
			DisplayName: "Social", PasswordHash: nil, Status: StatusActive, Role: "user",
		},
	}}
	return NewPasswordAuthenticator(NewUserStore(pool), DefaultArgon2Params()), pool, hash
}

func TestAuthenticateSuccess(t *testing.T) {
	t.Parallel()
	a, _, _ := newTestAuthenticator(t)
	u, err := a.Authenticate(context.Background(), Credentials{Identifier: "active@example.com", Secret: "Valid-Pass-1!"})
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u.Email != "active@example.com" {
		t.Fatalf("wrong user: %s", u.Email)
	}
}

func TestAuthenticateWrongPasswordUniform(t *testing.T) {
	t.Parallel()
	a, _, _ := newTestAuthenticator(t)

	_, errWrong := a.Authenticate(context.Background(), Credentials{Identifier: "active@example.com", Secret: "nope"})
	_, errUnknown := a.Authenticate(context.Background(), Credentials{Identifier: "ghost@example.com", Secret: "nope"})

	if !errors.Is(errWrong, ErrInvalidCredentials) {
		t.Fatalf("wrong password error = %v, want ErrInvalidCredentials", errWrong)
	}
	if !errors.Is(errUnknown, ErrInvalidCredentials) {
		t.Fatalf("unknown email error = %v, want ErrInvalidCredentials", errUnknown)
	}
}

func TestAuthenticateSuspendedAccount(t *testing.T) {
	t.Parallel()
	a, _, _ := newTestAuthenticator(t)
	_, err := a.Authenticate(context.Background(), Credentials{Identifier: "suspended@example.com", Secret: "Valid-Pass-1!"})
	if !errors.Is(err, ErrAccountNotActive) {
		t.Fatalf("suspended login error = %v, want ErrAccountNotActive", err)
	}
}

func TestAuthenticateSocialOnlyAccount(t *testing.T) {
	t.Parallel()
	a, _, _ := newTestAuthenticator(t)
	// No password set: must be rejected as invalid credentials (not a 500).
	_, err := a.Authenticate(context.Background(), Credentials{Identifier: "social@example.com", Secret: "anything"})
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("social-only login error = %v, want ErrInvalidCredentials", err)
	}
}

func TestValidEmail(t *testing.T) {
	t.Parallel()
	good := []string{"a@b.co", "alex.cotton@cotton-id.io"}
	bad := []string{"", "notanemail", "a@", "@b.co", "Alex <a@b.co>", "a@b.co (x)"}
	for _, e := range good {
		if !validEmail(e) {
			t.Errorf("validEmail(%q) = false, want true", e)
		}
	}
	for _, e := range bad {
		if validEmail(e) {
			t.Errorf("validEmail(%q) = true, want false", e)
		}
	}
}

func TestUsernameRegex(t *testing.T) {
	t.Parallel()
	good := []string{"abc", "alex_cotton", "a.b-c", "User123"}
	bad := []string{"ab", "has space", "emoji😀", "way_too_long_username_exceeding_thirty_two_chars"}
	for _, u := range good {
		if !usernameRe.MatchString(u) {
			t.Errorf("username %q should be valid", u)
		}
	}
	for _, u := range bad {
		if usernameRe.MatchString(u) {
			t.Errorf("username %q should be invalid", u)
		}
	}
}
