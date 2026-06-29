package auth

import (
	"context"
	"errors"
)

// Authentication errors. Login handlers map ErrInvalidCredentials and
// ErrAccountNotActive uniformly so the response never discloses which field was
// wrong or whether an email exists.
var (
	// ErrInvalidCredentials is returned for any wrong-email-or-password case.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrAccountNotActive is returned when credentials are correct but the
	// account is suspended/invited.
	ErrAccountNotActive = errors.New("account is not active")
)

// Authenticator is the seam that lets later changes add passkey/social methods
// without reworking the login flow. Each method verifies a credential and, on
// success, returns the authenticated user.
type Authenticator interface {
	// Method names the authentication method (e.g. "password", "passkey").
	Method() string
	// Authenticate verifies the supplied credentials and returns the user on
	// success, or ErrInvalidCredentials / ErrAccountNotActive.
	Authenticate(ctx context.Context, cred Credentials) (*User, error)
}

// Credentials carries the inputs for an authentication attempt. Different
// Authenticators read different fields (the password impl uses Identifier+Secret).
type Credentials struct {
	// Identifier is the login identifier — an email for password auth.
	Identifier string
	// Secret is the password (or other method secret).
	Secret string
}

// PasswordAuthenticator authenticates by email + password against the user store
// using argon2id. It is the only Authenticator in this change.
type PasswordAuthenticator struct {
	users  *UserStore
	params Argon2Params
}

// NewPasswordAuthenticator builds the password Authenticator.
func NewPasswordAuthenticator(users *UserStore, params Argon2Params) *PasswordAuthenticator {
	return &PasswordAuthenticator{users: users, params: params}
}

// Method returns "password".
func (a *PasswordAuthenticator) Method() string { return "password" }

// Authenticate verifies email+password. To resist user enumeration and timing
// side-channels it performs an argon2 verification even when the user does not
// exist (against a dummy hash), and returns ErrInvalidCredentials uniformly for
// both the unknown-email and wrong-password cases.
func (a *PasswordAuthenticator) Authenticate(ctx context.Context, cred Credentials) (*User, error) {
	user, err := a.users.GetByEmail(ctx, cred.Identifier)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Spend comparable time so presence of the account isn't timeable.
			_, _ = VerifyPassword(cred.Secret, dummyHash)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if user.PasswordHash == nil {
		// Social-only account: no password credential to verify.
		_, _ = VerifyPassword(cred.Secret, dummyHash)
		return nil, ErrInvalidCredentials
	}

	ok, err := VerifyPassword(cred.Secret, *user.PasswordHash)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}

	if user.Status != StatusActive {
		return nil, ErrAccountNotActive
	}
	return user, nil
}

// dummyHash is a precomputed valid argon2id PHC hash used to equalize timing for
// the unknown-account path so account existence is not timeable. Its plaintext
// is a fixed throwaway and never matches real user input in practice. Generated
// once with DefaultArgon2Params; it decodes and verifies as a real hash.
const dummyHash = "$argon2id$v=19$m=65536,t=3,p=4$m/lZYI9pugItAQ8u+JpyDA$LmzEs+0lJfwNaRx64oWIVMzBNjclU0a2xRFfhyev3KY"

var _ Authenticator = (*PasswordAuthenticator)(nil)
