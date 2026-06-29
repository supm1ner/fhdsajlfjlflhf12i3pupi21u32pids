package passkey

import (
	"github.com/go-webauthn/webauthn/webauthn"

	"cotton-id/internal/auth"
)

// webauthnUser adapts a cotton-id account (auth.User) plus its stored credentials
// to the library's webauthn.User interface. The library reads these to build the
// ceremony options and to verify responses.
//
// Per the WebAuthn spec, the user handle (WebAuthnID) is the stable, opaque key
// that authentication decisions are made on — here the account UUID's 16 bytes,
// NOT the username or display name (which are display-only and user-mutable).
type webauthnUser struct {
	user  *auth.User
	creds []webauthn.Credential
}

// newWebauthnUser builds the adapter from an account and its stored credentials.
func newWebauthnUser(user *auth.User, stored []StoredCredential) *webauthnUser {
	creds := make([]webauthn.Credential, 0, len(stored))
	for _, c := range stored {
		creds = append(creds, c.toLibraryCredential())
	}
	return &webauthnUser{user: user, creds: creds}
}

// WebAuthnID returns the user handle: the account UUID's raw 16 bytes.
func (u *webauthnUser) WebAuthnID() []byte {
	id := u.user.ID // uuid.UUID is a [16]byte
	return id[:]
}

// WebAuthnName returns the human-palatable account name (the username).
func (u *webauthnUser) WebAuthnName() string { return u.user.Username }

// WebAuthnDisplayName returns the display name shown by the authenticator UI.
func (u *webauthnUser) WebAuthnDisplayName() string { return u.user.DisplayName }

// WebAuthnCredentials returns the user's registered credentials.
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// ensure the adapter satisfies the library interface at compile time.
var _ webauthn.User = (*webauthnUser)(nil)
