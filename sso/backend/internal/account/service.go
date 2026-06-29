// Package account implements cotton-id's account self-service surface: a
// signed-in user managing their own profile (display name/about/location, avatar
// and banner images), security (password change, active-session listing and
// revocation), connected apps (OAuth consent grants via Hydra), preferences
// (theme/language/login-notifications), and account deletion.
//
// It is a thin service composing the existing stores (auth.UserStore,
// auth.SessionStore, passkey list-only for the security overview), the Hydra admin
// client (consent grants), and a small image blob store, plus the password
// Authenticator for re-authentication on password change and account deletion. All
// HTTP handlers (handlers.go) require an active session (resolved via
// auth.Service.UserForSession) and live in the /api/v1 CSRF group.
package account

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"cotton-id/internal/auth"
)

// Field bounds for the editable profile (validation; keeps rows/API responses
// bounded). Lengths are counted in runes.
const (
	maxDisplayNameLen = 80
	maxAboutLen       = 500
	maxLocationLen    = 120
)

// Validation / domain errors surfaced to handlers (mapped to field problems).
var (
	// ErrDisplayNameRequired is returned when the display name is blank.
	ErrDisplayNameRequired = errors.New("display name is required")
	// ErrDisplayNameTooLong / ErrAboutTooLong / ErrLocationTooLong bound the fields.
	ErrDisplayNameTooLong = errors.New("display name is too long")
	ErrAboutTooLong       = errors.New("about is too long")
	ErrLocationTooLong    = errors.New("location is too long")

	// ErrInvalidTheme / ErrInvalidLang reject out-of-range preference values.
	ErrInvalidTheme = errors.New("theme must be one of: dark, light, system")
	ErrInvalidLang  = errors.New("language must be one of: ru, en")

	// ErrWrongPassword is returned when current-password re-auth fails (password
	// change and account deletion).
	ErrWrongPassword = errors.New("current password is incorrect")
	// ErrReauthRequired is returned when a destructive action lacks the required
	// re-authentication (e.g. account deletion without the current password or a
	// confirmation flag for a passwordless account).
	ErrReauthRequired = errors.New("re-authentication is required")
	// ErrAccountLocked is returned when the per-account lockout is engaged for the
	// re-auth (password change / deletion), mirroring the login lockout so these
	// password-verifying paths cannot be used to bypass brute-force protection.
	ErrAccountLocked = errors.New("too many failed attempts, try again later")
)

// allowed preference value sets.
var (
	validThemes = map[string]bool{"dark": true, "light": true, "system": true}
	validLangs  = map[string]bool{"ru": true, "en": true}
)

// userStore is the user-persistence seam the service needs. *auth.UserStore
// satisfies it.
type userStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*auth.User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, p auth.UpdateProfileParams) (*auth.User, error)
	UpdatePreferences(ctx context.Context, id uuid.UUID, p auth.UpdatePreferencesParams) (*auth.User, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	SetImageURL(ctx context.Context, id uuid.UUID, kind, urlValue string) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// sessionStore is the session-persistence seam the service needs. *auth.SessionStore
// satisfies it.
type sessionStore interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]auth.Session, error)
	DeleteForUser(ctx context.Context, userID uuid.UUID, id string) error
	DeleteByUserExcept(ctx context.Context, userID uuid.UUID, keepID string) (int64, error)
}

// passkeyLister returns the count of a user's passkeys for the security overview.
// passkey.CredentialStore satisfies it (via ListByUser) through the adapter in
// deps.go.
type passkeyLister interface {
	CountByUser(ctx context.Context, userID uuid.UUID) (int, error)
}

// consentLister is the Hydra seam for the connected-apps view + revocation.
// *oidc.HydraClient satisfies it.
type consentLister interface {
	ListConsentSessions(ctx context.Context, subject string) ([]consentRecord, error)
	RevokeConsentSessions(ctx context.Context, subject, client string) error
	RevokeAllConsentSessions(ctx context.Context, subject string) error
	RevokeLoginSessions(ctx context.Context, subject string) error
}

// authenticator verifies a user's password for re-authentication. *auth.PasswordAuthenticator
// satisfies it.
type authenticator interface {
	Authenticate(ctx context.Context, cred auth.Credentials) (*auth.User, error)
}

// passwordHasher hashes a new password and enforces the password policy. The
// concrete implementation wraps auth.ValidatePassword + auth.HashPassword.
type passwordHasher interface {
	Validate(pw string) error
	Hash(pw string) (string, error)
}

// accountLockout is the per-account incremental-backoff lockout seam, shared with
// the login handler so the password-verifying re-auth paths (change/delete) cannot
// be used to bypass brute-force protection. *auth.MemoryLockout satisfies it.
type accountLockout interface {
	Locked(key string) (bool, time.Duration)
	Fail(key string) (locked bool, retryAfter time.Duration)
	Reset(key string)
}

// Service is the account self-service domain service. It has no knowledge of HTTP.
type Service struct {
	users    userStore
	sessions sessionStore
	passkeys passkeyLister
	consent  consentLister
	authn    authenticator
	hasher   passwordHasher
	lockout  accountLockout
	log      *slog.Logger
}

// Deps groups the service's collaborators (see deps.go for the wiring helper).
type serviceDeps struct {
	Users    userStore
	Sessions sessionStore
	Passkeys passkeyLister
	Consent  consentLister
	Authn    authenticator
	Hasher   passwordHasher
	Lockout  accountLockout
	Log      *slog.Logger
}

// newService wires the account service from its collaborators.
func newService(d serviceDeps) *Service {
	return &Service{
		users:    d.Users,
		sessions: d.Sessions,
		passkeys: d.Passkeys,
		consent:  d.Consent,
		authn:    d.Authn,
		hasher:   d.Hasher,
		lockout:  d.Lockout,
		log:      d.Log,
	}
}

// reauthKey is the lockout/throttle key for an account's password re-auth — the
// SAME key the login handler uses, so failed login + failed re-auth share the
// lockout counter for an account.
func reauthKey(email string) string { return "acct:" + strings.ToLower(strings.TrimSpace(email)) }

// Counts is the security-overview tally returned with the full profile.
type Counts struct {
	Sessions    int `json:"sessions"`
	Passkeys    int `json:"passkeys"`
	Connections int `json:"connections"`
}

// Profile is the full self-service profile: the user (incl. preferences) and the
// security-overview counts.
type Profile struct {
	User   *auth.User
	Counts Counts
}

// GetProfile returns the user's full profile plus the session/passkey/connection
// counts for the security overview.
func (s *Service) GetProfile(ctx context.Context, user *auth.User) (*Profile, error) {
	sessions, err := s.sessions.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	passkeyCount, err := s.passkeys.CountByUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	connCount := 0
	if records, cerr := s.consent.ListConsentSessions(ctx, user.ID.String()); cerr == nil {
		connCount = len(records)
	}
	// Connection count is best-effort: a Hydra hiccup must not blank the whole
	// profile, so a list error leaves connCount at 0 rather than failing.
	return &Profile{
		User: user,
		Counts: Counts{
			Sessions:    len(sessions),
			Passkeys:    passkeyCount,
			Connections: connCount,
		},
	}, nil
}

// UpdateProfileInput is the validated-on-entry profile edit.
type UpdateProfileInput struct {
	DisplayName string
	About       string
	Location    string
}

// UpdateProfile validates and saves the user's editable profile fields, returning
// the refreshed user.
func (s *Service) UpdateProfile(ctx context.Context, user *auth.User, in UpdateProfileInput) (*auth.User, error) {
	name := strings.TrimSpace(in.DisplayName)
	about := strings.TrimSpace(in.About)
	loc := strings.TrimSpace(in.Location)

	if name == "" {
		return nil, ErrDisplayNameRequired
	}
	if utf8.RuneCountInString(name) > maxDisplayNameLen {
		return nil, ErrDisplayNameTooLong
	}
	if utf8.RuneCountInString(about) > maxAboutLen {
		return nil, ErrAboutTooLong
	}
	if utf8.RuneCountInString(loc) > maxLocationLen {
		return nil, ErrLocationTooLong
	}
	return s.users.UpdateProfile(ctx, user.ID, auth.UpdateProfileParams{
		DisplayName: name,
		About:       about,
		Location:    loc,
	})
}

// UpdatePreferencesInput is the validated-on-entry preferences edit.
type UpdatePreferencesInput struct {
	Theme              string
	Lang               string
	LoginNotifications bool
}

// UpdatePreferences validates and persists the user's theme/language/login-
// notification preferences, returning the refreshed user.
func (s *Service) UpdatePreferences(ctx context.Context, user *auth.User, in UpdatePreferencesInput) (*auth.User, error) {
	theme := strings.TrimSpace(in.Theme)
	lang := strings.TrimSpace(in.Lang)
	if !validThemes[theme] {
		return nil, ErrInvalidTheme
	}
	if !validLangs[lang] {
		return nil, ErrInvalidLang
	}
	return s.users.UpdatePreferences(ctx, user.ID, auth.UpdatePreferencesParams{
		Theme:              theme,
		Lang:               lang,
		LoginNotifications: in.LoginNotifications,
	})
}

// ChangePassword re-authenticates the user with their current password, enforces
// the password policy on the new one, rehashes it, and revokes the user's OTHER
// sessions (keeping the current one identified by currentSessionID). It returns
// the number of other sessions revoked.
//
// currentSessionID is the sha256 hex id of the request's session cookie so the
// in-flight session is preserved; pass "" to revoke nothing extra to keep (not
// used by the handler, which always has a current session).
func (s *Service) ChangePassword(ctx context.Context, user *auth.User, currentPassword, newPassword, currentSessionID string) (revoked int64, err error) {
	// Re-auth: verify the current password via the password authenticator. A
	// social/passkey-only account (no password hash) cannot "change" a password
	// it never had — treat as wrong current password (uniform rejection).
	if user.PasswordHash == nil {
		return 0, ErrWrongPassword
	}
	// Per-account brute-force protection (parity with the login handler): refuse
	// while locked, count failures, reset on success — sharing the login lockout.
	key := reauthKey(user.Email)
	if s.lockout != nil {
		if locked, _ := s.lockout.Locked(key); locked {
			return 0, ErrAccountLocked
		}
	}
	if _, aerr := s.authn.Authenticate(ctx, auth.Credentials{
		Identifier: user.Email,
		Secret:     currentPassword,
	}); aerr != nil {
		if errors.Is(aerr, auth.ErrInvalidCredentials) {
			if s.lockout != nil {
				s.lockout.Fail(key)
			}
			return 0, ErrWrongPassword
		}
		return 0, aerr
	}
	if s.lockout != nil {
		s.lockout.Reset(key)
	}

	if verr := s.hasher.Validate(newPassword); verr != nil {
		return 0, verr
	}
	hash, herr := s.hasher.Hash(newPassword)
	if herr != nil {
		return 0, herr
	}
	if uerr := s.users.UpdatePassword(ctx, user.ID, hash); uerr != nil {
		return 0, uerr
	}
	// Revoke every other session (keep the current one). Mirrors reset semantics
	// minus the token, but does not log the user out of the device they used.
	return s.sessions.DeleteByUserExcept(ctx, user.ID, currentSessionID)
}

// SessionView is the client-safe projection of one active session, with the
// request's own session flagged as current.
type SessionView struct {
	ID         string
	UserAgent  string
	IP         string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
	Current    bool
}

// ListSessions returns the user's active sessions, flagging the one whose id is
// currentSessionID as the current device. Sessions are returned newest-first.
func (s *Service) ListSessions(ctx context.Context, user *auth.User, currentSessionID string) ([]SessionView, error) {
	rows, err := s.sessions.ListByUser(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	out := make([]SessionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, SessionView{
			ID:         r.ID,
			UserAgent:  r.UserAgent,
			IP:         r.IP,
			CreatedAt:  r.CreatedAt,
			ExpiresAt:  r.ExpiresAt,
			LastSeenAt: r.LastSeenAt,
			Current:    r.ID == currentSessionID,
		})
	}
	return out, nil
}

// RevokeSession deletes one of the user's sessions by its id, scoped to the user
// (a user cannot revoke another account's session). Returns auth.ErrSessionNotFound
// when no such session exists for that user.
func (s *Service) RevokeSession(ctx context.Context, user *auth.User, id string) error {
	return s.sessions.DeleteForUser(ctx, user.ID, id)
}

// RevokeOtherSessions revokes all of the user's sessions except the current one,
// returning the number revoked.
func (s *Service) RevokeOtherSessions(ctx context.Context, user *auth.User, currentSessionID string) (int64, error) {
	return s.sessions.DeleteByUserExcept(ctx, user.ID, currentSessionID)
}

// Connection is the client-safe projection of one Hydra consent grant.
type Connection struct {
	ClientID      string
	ClientName    string
	GrantedScopes []string
	GrantedAt     string
}

// ListConnections returns the user's connected apps (Hydra consent grants),
// projected to client id/name + granted scopes + grant timestamp.
func (s *Service) ListConnections(ctx context.Context, user *auth.User) ([]Connection, error) {
	records, err := s.consent.ListConsentSessions(ctx, user.ID.String())
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(records))
	for _, rec := range records {
		out = append(out, Connection{
			ClientID:      rec.ClientID,
			ClientName:    rec.ClientName,
			GrantedScopes: rec.GrantScope,
			GrantedAt:     rec.HandledAt,
		})
	}
	return out, nil
}

// RevokeConnection revokes the user's consent for a single client, so that client
// must obtain consent again on its next authorization.
func (s *Service) RevokeConnection(ctx context.Context, user *auth.User, clientID string) error {
	return s.consent.RevokeConsentSessions(ctx, user.ID.String(), clientID)
}

// DeleteAccountInput carries the re-auth material for account deletion: the
// current password for password accounts, or Confirm=true for a passwordless
// (social/passkey-only) account.
type DeleteAccountInput struct {
	CurrentPassword string
	Confirm         bool
}

// DeleteAccount re-authenticates the user, deletes the user row (FK cascade
// removes sessions, passkeys, social identities, reset tokens, and profile
// images), and best-effort revokes the subject's Hydra login + consent sessions.
//
// Re-auth rule: a password account MUST supply the correct current password; a
// passwordless account (no password hash, e.g. social/passkey-only) MUST supply
// Confirm=true. A missing/incorrect credential refuses the deletion and leaves the
// account intact.
func (s *Service) DeleteAccount(ctx context.Context, user *auth.User, in DeleteAccountInput) error {
	if user.PasswordHash != nil {
		if strings.TrimSpace(in.CurrentPassword) == "" {
			return ErrReauthRequired
		}
		// Per-account brute-force protection (parity with login) on the re-auth.
		key := reauthKey(user.Email)
		if s.lockout != nil {
			if locked, _ := s.lockout.Locked(key); locked {
				return ErrAccountLocked
			}
		}
		if _, aerr := s.authn.Authenticate(ctx, auth.Credentials{
			Identifier: user.Email,
			Secret:     in.CurrentPassword,
		}); aerr != nil {
			if errors.Is(aerr, auth.ErrInvalidCredentials) {
				if s.lockout != nil {
					s.lockout.Fail(key)
				}
				return ErrWrongPassword
			}
			return aerr
		}
		if s.lockout != nil {
			s.lockout.Reset(key)
		}
	} else if !in.Confirm {
		// Passwordless account: require the explicit confirmation flag.
		return ErrReauthRequired
	}

	if derr := s.users.Delete(ctx, user.ID); derr != nil {
		return derr
	}

	// Best-effort: revoke the subject's OIDC sessions. The local account is already
	// gone; a Hydra error here must not fail the deletion, but is logged so an
	// operator can detect and re-run revocation (already-issued tokens stay valid
	// until their natural expiry on a Hydra outage — see SECURITY.md).
	subject := user.ID.String()
	if cerr := s.consent.RevokeAllConsentSessions(ctx, subject); cerr != nil && s.log != nil {
		s.log.Warn("account delete: hydra consent revoke failed (tokens valid until expiry)",
			slog.String("subject", subject), slog.Any("error", cerr))
	}
	if lerr := s.consent.RevokeLoginSessions(ctx, subject); lerr != nil && s.log != nil {
		s.log.Warn("account delete: hydra login revoke failed",
			slog.String("subject", subject), slog.Any("error", lerr))
	}
	return nil
}
