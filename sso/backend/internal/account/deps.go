package account

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"cotton-id/internal/auth"
	"cotton-id/internal/observability"
	"cotton-id/internal/oidc"
	"cotton-id/internal/passkey"
)

// Deps are the account handlers' dependencies; main.go populates this with the
// concrete stores/clients. The adapters below bridge those concrete types to the
// service's narrow seams so service.go stays HTTP- and dependency-agnostic and
// unit-testable with fakes.
type Deps struct {
	Logger      *slog.Logger
	Metrics     *observability.Metrics
	Users       *auth.UserStore
	Sessions    *auth.SessionStore
	Credentials *passkey.CredentialStore
	Hydra       *oidc.HydraClient
	Images      *ImageStore

	// Auth resolves the current session to a user (auth gate) and re-authenticates
	// the password for password-change / delete. *auth.Service satisfies it.
	Auth SessionResolver
	// Authn verifies a password for re-auth. *auth.PasswordAuthenticator satisfies it.
	Authn *auth.PasswordAuthenticator
	// Lockout is the SAME per-account lockout the login handler uses, so the
	// password-verifying re-auth paths share its brute-force protection.
	Lockout auth.Lockout
	// Params are the argon2id parameters used to hash a changed password.
	Params auth.Argon2Params

	// PublicBaseURL is the backend's external URL, used to build served image URLs.
	PublicBaseURL string
	// SessionCookieName / CookieSecure mirror the auth handlers.
	SessionCookieName string
	CookieSecure      bool
}

// SessionResolver resolves a raw session-cookie token to the active user.
// *auth.Service satisfies it (via UserForSession). It is the auth gate for every
// account route (mirrors passkey's SessionAuthenticator.UserForSession).
type SessionResolver interface {
	UserForSession(ctx context.Context, token string) (*auth.User, error)
}

// --- adapters bridging concrete types to the service seams ---

// passkeyCounter adapts passkey.CredentialStore.ListByUser to a count, for the
// security-overview tally (the service never needs the credential bytes).
type passkeyCounter struct {
	store *passkey.CredentialStore
}

func (a passkeyCounter) CountByUser(ctx context.Context, userID uuid.UUID) (int, error) {
	creds, err := a.store.ListByUser(ctx, userID)
	if err != nil {
		return 0, err
	}
	return len(creds), nil
}

// consentRecord is the account package's own projection of a Hydra consent
// session, so service.go does not depend on the oidc wire shape directly.
type consentRecord struct {
	ClientID   string
	ClientName string
	GrantScope []string
	HandledAt  string
}

// hydraConsentAdapter adapts *oidc.HydraClient to the consentLister seam,
// projecting Hydra's OAuth2ConsentSession to consentRecord and tolerating the
// missing-client/missing-fields cases.
type hydraConsentAdapter struct {
	client *oidc.HydraClient
}

func (a hydraConsentAdapter) ListConsentSessions(ctx context.Context, subject string) ([]consentRecord, error) {
	records, err := a.client.ListConsentSessions(ctx, subject)
	if err != nil {
		return nil, err
	}
	out := make([]consentRecord, 0, len(records))
	for _, rec := range records {
		var clientID, clientName string
		if rec.ConsentRequest != nil && rec.ConsentRequest.Client != nil {
			clientID = rec.ConsentRequest.Client.ClientID
			clientName = rec.ConsentRequest.Client.ClientName
		}
		out = append(out, consentRecord{
			ClientID:   clientID,
			ClientName: clientName,
			GrantScope: rec.GrantScope,
			HandledAt:  rec.HandledAt,
		})
	}
	return out, nil
}

func (a hydraConsentAdapter) RevokeConsentSessions(ctx context.Context, subject, client string) error {
	return a.client.RevokeConsentSessions(ctx, subject, client)
}

func (a hydraConsentAdapter) RevokeAllConsentSessions(ctx context.Context, subject string) error {
	return a.client.RevokeAllConsentSessions(ctx, subject)
}

func (a hydraConsentAdapter) RevokeLoginSessions(ctx context.Context, subject string) error {
	return a.client.RevokeLoginSessions(ctx, subject)
}

// argon2Hasher adapts the auth password policy + hasher to the service's
// passwordHasher seam.
type argon2Hasher struct {
	params auth.Argon2Params
}

func (h argon2Hasher) Validate(pw string) error { return auth.ValidatePassword(pw) }
func (h argon2Hasher) Hash(pw string) (string, error) {
	return auth.HashPassword(pw, h.params)
}

// buildService assembles the domain service from the concrete deps via the
// adapters above.
func buildService(d Deps) *Service {
	return newService(serviceDeps{
		Users:    d.Users,
		Sessions: d.Sessions,
		Passkeys: passkeyCounter{store: d.Credentials},
		Consent:  hydraConsentAdapter{client: d.Hydra},
		Authn:    d.Authn,
		Hasher:   argon2Hasher{params: d.Params},
		Lockout:  d.Lockout,
		Log:      d.Logger,
	})
}

// compile-time seam checks against the concrete types.
var (
	_ SessionResolver = (*auth.Service)(nil)
	_ authenticator   = (*auth.PasswordAuthenticator)(nil)
)
