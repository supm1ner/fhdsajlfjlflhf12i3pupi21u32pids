package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"cotton-id/internal/mailer"
)

// Config carries the auth-relevant runtime settings the service needs. main.go
// populates it from internal/config so this package stays config-agnostic.
type Config struct {
	SessionTTL         time.Duration
	SessionRememberTTL time.Duration
	PasswordResetTTL   time.Duration
	// FrontendBaseURL is used to build the password-reset link.
	FrontendBaseURL string
	Argon2Params    Argon2Params
}

// Service is the auth domain service: it composes the stores, the password
// authenticator, and the mailer to implement signup/login/logout/session/reset.
// HTTP handlers (handlers.go) call into it; it has no knowledge of HTTP.
type Service struct {
	cfg      Config
	users    *UserStore
	sessions *SessionStore
	resets   *ResetTokenStore
	auth     Authenticator
	mailer   mailer.Mailer
}

// NewService wires the auth service.
func NewService(cfg Config, users *UserStore, sessions *SessionStore, resets *ResetTokenStore, authn Authenticator, m mailer.Mailer) *Service {
	return &Service{
		cfg:      cfg,
		users:    users,
		sessions: sessions,
		resets:   resets,
		auth:     authn,
		mailer:   m,
	}
}

// Mailer returns the underlying mailer (for verification code sending, etc.).
func (s *Service) Mailer() mailer.Mailer { return s.mailer }

// usernameRe restricts usernames to a safe, URL/handle-friendly charset.
var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)

// Validation errors surfaced to handlers (mapped to field problems).
var (
	ErrInvalidEmail       = errors.New("email is not a valid address")
	ErrInvalidUsername    = errors.New("username must be 3-32 chars: letters, digits, _ . -")
	ErrDisplayNameInvalid = errors.New("display name is required")
)

// SignupParams holds validated-on-entry signup input.
type SignupParams struct {
	DisplayName string
	Username    string
	Email       string
	Password    string
	UserAgent   string
	IP          string
}

// SignupResult bundles the created user and the new session token.
type SignupResult struct {
	User         *User
	SessionToken string
	Remember     bool
	ExpiresAt    time.Time
}

// Signup validates input, creates the account with an argon2id hash, and
// establishes a session. Duplicate email/username map to typed errors.
func (s *Service) Signup(ctx context.Context, p SignupParams) (*SignupResult, error) {
	p.DisplayName = strings.TrimSpace(p.DisplayName)
	p.Username = strings.TrimSpace(p.Username)
	p.Email = normalizeEmail(p.Email)

	if p.DisplayName == "" {
		return nil, ErrDisplayNameInvalid
	}
	if !usernameRe.MatchString(p.Username) {
		return nil, ErrInvalidUsername
	}
	if !validEmail(p.Email) {
		return nil, ErrInvalidEmail
	}
	if err := ValidatePassword(p.Password); err != nil {
		return nil, err
	}

	hash, err := HashPassword(p.Password, s.cfg.Argon2Params)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.users.Create(ctx, CreateUserParams{
		Email:        p.Email,
		Username:     p.Username,
		DisplayName:  p.DisplayName,
		PasswordHash: hash,
	})
	if err != nil {
		return nil, err
	}

	// Signup never sets "remember"; uses the standard (24h) TTL.
	sess, token, err := s.createSession(ctx, user.ID, false, p.UserAgent, p.IP)
	if err != nil {
		return nil, err
	}
	return &SignupResult{User: user, SessionToken: token, Remember: false, ExpiresAt: sess.ExpiresAt}, nil
}

// LoginParams holds login input.
type LoginParams struct {
	Email     string
	Password  string
	Remember  bool
	UserAgent string
	IP        string
}

// LoginResult bundles the authenticated user and new session.
type LoginResult struct {
	User         *User
	SessionToken string
	Remember     bool
	ExpiresAt    time.Time
}

// Login authenticates credentials and establishes a session on success.
func (s *Service) Login(ctx context.Context, p LoginParams) (*LoginResult, error) {
	user, err := s.auth.Authenticate(ctx, Credentials{
		Identifier: normalizeEmail(p.Email),
		Secret:     p.Password,
	})
	if err != nil {
		return nil, err
	}

	sess, token, err := s.createSession(ctx, user.ID, p.Remember, p.UserAgent, p.IP)
	if err != nil {
		return nil, err
	}
	return &LoginResult{User: user, SessionToken: token, Remember: p.Remember, ExpiresAt: sess.ExpiresAt}, nil
}

// Logout revokes the session identified by the raw cookie token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.DeleteByToken(ctx, token)
}

// UserForSession resolves a raw cookie token to its (active) user, or an error.
// Used by GET /auth/session and as the SessionVerifier seam for OIDC.
func (s *Service) UserForSession(ctx context.Context, token string) (*User, error) {
	sess, err := s.sessions.GetByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	user, err := s.users.GetByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if user.Status != StatusActive {
		return nil, ErrAccountNotActive
	}
	// Best-effort, throttled last-seen bump: every authenticated request resolves
	// the session here, so this is the single point that records session use. The
	// store throttles the write to ≤1/min/session in SQL; a failed bump is
	// deliberately ignored so it can never fail the request (design D4). Only bump
	// when the cheap in-memory check shows the row is already stale, to skip the
	// UPDATE round-trip for an already-fresh session.
	if time.Since(sess.LastSeenAt) > lastSeenThrottle {
		// Error deliberately ignored: last-seen tracking is best-effort cosmetic
		// data and must never fail the authenticated request.
		_ = s.sessions.BumpLastSeen(ctx, sess.ID)
	}
	return user, nil
}

// RequestPasswordReset issues a single-use reset token for the email if an
// account exists, e-mailing the link. It NEVER reveals whether the email is
// registered: the caller responds with the same message regardless of outcome.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			// Non-enumerating: succeed silently.
			return nil
		}
		return err
	}

	token, hash, err := GenerateResetToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(s.cfg.PasswordResetTTL)
	if err := s.resets.Create(ctx, user.ID, hash, expiresAt); err != nil {
		return err
	}

	link := s.buildResetLink(token)
	// Mail failures are logged by the caller but must not reveal account
	// existence; we still return the error so the caller can log it server-side.
	if err := s.mailer.SendPasswordReset(ctx, user.Email, link); err != nil {
		return err
	}
	return nil
}

// IssueResetForUser issues a single-use password-reset token for a known user id
// and (stub-)emails the link. It is the admin-console "force password reset"
// seam: unlike RequestPasswordReset (which takes an email and is deliberately
// non-enumerating), the admin already knows the target account, so this resolves
// by id and surfaces ErrUserNotFound. It reuses the same token store, TTL, and
// mailer as the self-service flow. A mailer failure is returned so the caller can
// log it; the token is still persisted (the admin can re-send / share the link).
func (s *Service) IssueResetForUser(ctx context.Context, userID uuid.UUID) error {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	token, hash, err := GenerateResetToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(s.cfg.PasswordResetTTL)
	if err := s.resets.Create(ctx, user.ID, hash, expiresAt); err != nil {
		return err
	}
	link := s.buildResetLink(token)
	return s.mailer.SendPasswordReset(ctx, user.Email, link)
}

// ResetPassword consumes a valid single-use token, sets the new (policy-checked)
// password, and revokes all existing sessions and outstanding reset tokens for
// the account.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	userID, err := s.resets.Consume(ctx, token)
	if err != nil {
		return err
	}

	hash, err := HashPassword(newPassword, s.cfg.Argon2Params)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := s.users.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}
	// Invalidate all sessions (force re-login) and any other outstanding tokens.
	if err := s.sessions.DeleteByUser(ctx, userID); err != nil {
		return err
	}
	if err := s.resets.DeleteByUser(ctx, userID); err != nil {
		return err
	}
	return nil
}

// EstablishedSession is the result of minting a session for an
// already-authenticated user (e.g. via social login): the raw cookie token plus
// the server-side expiry the caller writes into the cookie.
type EstablishedSession struct {
	Token     string
	Remember  bool
	ExpiresAt time.Time
}

// EstablishSession mints a new cotton-id session for userID without verifying a
// credential. It is the seam social login uses after a provider has authenticated
// the user: the provider flow proves identity, then this binds a cotton-id
// session exactly like password login does (same store, same TTLs). Callers MUST
// only invoke this after they have themselves authenticated the user.
func (s *Service) EstablishSession(ctx context.Context, userID uuid.UUID, remember bool, ua, ip string) (*EstablishedSession, error) {
	sess, token, err := s.createSession(ctx, userID, remember, ua, ip)
	if err != nil {
		return nil, err
	}
	return &EstablishedSession{Token: token, Remember: remember, ExpiresAt: sess.ExpiresAt}, nil
}

// createSession persists a session and returns it plus the raw token for the
// cookie.
func (s *Service) createSession(ctx context.Context, userID uuid.UUID, remember bool, ua, ip string) (*Session, string, error) {
	token, id, err := GenerateSessionToken()
	if err != nil {
		return nil, "", err
	}
	ttl := s.cfg.SessionTTL
	if remember {
		ttl = s.cfg.SessionRememberTTL
	}
	sess := &Session{
		ID:        id,
		UserID:    userID,
		Remember:  remember,
		UserAgent: truncate(ua, 512),
		IP:        ip,
		ExpiresAt: time.Now().Add(ttl),
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, "", err
	}
	return sess, token, nil
}

// SessionTTLFor returns the cookie Max-Age duration for a session: the configured
// remember/standard TTL. For non-remember sessions the cookie is a
// browser-session cookie (no Max-Age) but the server still expires it.
func (s *Service) SessionTTLFor(remember bool) time.Duration {
	if remember {
		return s.cfg.SessionRememberTTL
	}
	return s.cfg.SessionTTL
}

// buildResetLink builds the password-reset URL the user follows from their
// email. The path MUST match the SPA route registered in frontend/src/App.tsx
// (`/reset`); a mismatch would land the user on the SPA 404 and silently break
// the reset flow. Covered by TestBuildResetLinkMatchesSPARoute.
func (s *Service) buildResetLink(token string) string {
	base := strings.TrimRight(s.cfg.FrontendBaseURL, "/")
	return fmt.Sprintf("%s/reset?token=%s", base, url.QueryEscape(token))
}

// --- helpers ---

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	if email == "" || len(email) > 320 {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	// ParseAddress accepts display names; require the bare address form.
	return addr.Address == email
}

// ValidEmail reports whether email is a well-formed bare address (no display
// name, no control characters). Exported so other packages (e.g. social login)
// can apply the same validity bar the password path uses before persisting a
// provider-asserted email to the users table.
func ValidEmail(email string) bool { return validEmail(email) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
