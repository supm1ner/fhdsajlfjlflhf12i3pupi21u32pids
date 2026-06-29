package account

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"cotton-id/internal/auth"
)

// fakeLockout records Fail/Reset calls and can report "locked".
type fakeLockout struct {
	locked bool
	fails  int
	resets int
}

func (f *fakeLockout) Locked(string) (bool, time.Duration) {
	if f.locked {
		return true, time.Minute
	}
	return false, 0
}
func (f *fakeLockout) Fail(string) (bool, time.Duration) { f.fails++; return false, 0 }
func (f *fakeLockout) Reset(string)                      { f.resets++ }

func serviceWithLockout(user *auth.User, lk accountLockout) *Service {
	return newService(serviceDeps{
		Users:    &fakeUserStore{user: user},
		Sessions: &fakeSessionStore{},
		Passkeys: fakePasskeyLister{},
		Consent:  &fakeConsent{},
		Authn:    fakeAuthn{correctPassword: "current-Pass-1!", user: user},
		Hasher:   fakeHasher{},
		Lockout:  lk,
	})
}

// TestChangePasswordRespectsLockout locks in the security-review fix: the
// password-change re-auth shares the login lockout (refuse when locked, count
// failures, reset on success) so it cannot bypass brute-force protection.
func TestChangePasswordRespectsLockout(t *testing.T) {
	t.Parallel()
	user := &auth.User{
		ID: uuid.New(), Email: "user@example.com", Username: "user",
		PasswordHash: ptr("$argon2id$existing"), Status: auth.StatusActive,
	}
	lk := &fakeLockout{locked: true}
	svc := serviceWithLockout(user, lk)
	ctx := context.Background()

	// Locked → refuse before even checking the password.
	if _, err := svc.ChangePassword(ctx, user, "current-Pass-1!", "new-Password-2!", "sid"); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("locked account: want ErrAccountLocked, got %v", err)
	}

	// Unlocked + wrong current password → records a failure.
	lk.locked = false
	if _, err := svc.ChangePassword(ctx, user, "WRONG", "new-Password-2!", "sid"); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong password: want ErrWrongPassword, got %v", err)
	}
	if lk.fails != 1 {
		t.Errorf("a failed re-auth must call lockout.Fail (got %d)", lk.fails)
	}

	// Correct current password → success resets the counter.
	if _, err := svc.ChangePassword(ctx, user, "current-Pass-1!", "new-Password-2!", "sid"); err != nil {
		t.Fatalf("correct password should succeed, got %v", err)
	}
	if lk.resets != 1 {
		t.Errorf("a successful re-auth must call lockout.Reset (got %d)", lk.resets)
	}
}

func TestDeleteAccountRespectsLockout(t *testing.T) {
	t.Parallel()
	user := &auth.User{
		ID: uuid.New(), Email: "user@example.com", Username: "user",
		PasswordHash: ptr("$argon2id$existing"), Status: auth.StatusActive,
	}
	lk := &fakeLockout{locked: true}
	svc := serviceWithLockout(user, lk)

	if err := svc.DeleteAccount(context.Background(), user, DeleteAccountInput{CurrentPassword: "current-Pass-1!"}); !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("locked account deletion: want ErrAccountLocked, got %v", err)
	}
}
