package passkey

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"

	"cotton-id/internal/auth"
	"cotton-id/internal/httpx"
	"cotton-id/internal/notify"
	"cotton-id/internal/observability"
	"cotton-id/internal/oidc"
)

// handlers.go — the passkey HTTP surface, mounted under /api/v1 by main.go:
//
//	POST /api/v1/passkeys/register/begin    (auth) → CredentialCreationOptions + cid_wa
//	POST /api/v1/passkeys/register/finish   (auth) → verify attestation, store, clear cid_wa
//	GET  /api/v1/passkeys                    (auth) → list the user's credentials
//	DELETE /api/v1/passkeys/{id}             (auth) → delete one of the user's credentials
//	POST /api/v1/auth/passkey/login/begin    (pre-auth) → CredentialRequestOptions + cid_wa
//	POST /api/v1/auth/passkey/login/finish   (pre-auth) → verify assertion, establish session
//
// All are JSON POST/GET/DELETE in the /api/v1 CSRF group; the SPA echoes the
// double-submit CSRF token. The register/list/delete routes require a valid
// cotton-id session (resolved via auth.Service.UserForSession); the login routes
// are pre-auth (the assertion itself is the credential).

// maxPasskeyNameLen bounds the user-supplied passkey nickname.
const maxPasskeyNameLen = 64

// SessionAuthenticator is the auth seam the handlers use: resolve the current
// session, mint a session for an authenticated user. *auth.Service satisfies it.
type SessionAuthenticator interface {
	UserForSession(ctx context.Context, token string) (*auth.User, error)
	EstablishSession(ctx context.Context, userID uuid.UUID, remember bool, ua, ip string) (*auth.EstablishedSession, error)
}

// hydraAccepter continues an in-progress OIDC login. *oidc.HydraClient satisfies it.
type hydraAccepter interface {
	AcceptLoginRequest(ctx context.Context, challenge string, body oidc.AcceptLogin) (*oidc.RedirectTo, error)
}

// Deps are the passkey handlers' dependencies; main.go populates this.
type Deps struct {
	Logger      *slog.Logger
	Metrics     *observability.Metrics
	Users       *auth.UserStore
	Credentials *CredentialStore
	Auth        SessionAuthenticator
	Hydra       hydraAccepter

	// RPID / RPDisplayName / RPOrigins are the relying-party configuration.
	RPID          string
	RPDisplayName string
	RPOrigins     []string

	// SessionCookieName / CookieSecure mirror the auth handlers so the session
	// cookie passkey login sets is identical to the password-login one.
	SessionCookieName string
	CookieSecure      bool

	// StateKey signs the cid_wa ceremony cookie (per-process random key from main.go).
	StateKey []byte

	// Notifier sends the best-effort login-notification email; SessionLister backs
	// the new-device heuristic. Both are OPTIONAL (nil-safe): when either is unset
	// the notification is skipped. main.go wires them.
	Notifier      *notify.Notifier
	SessionLister sessionLister
}

// sessionLister lists a user's active sessions for the new-device
// login-notification heuristic. *auth.SessionStore satisfies it via ListByUser.
type sessionLister interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]auth.Session, error)
}

// Handlers serves the passkey API.
type Handlers struct {
	deps       Deps
	wa         *webauthn.WebAuthn
	state      *stateCodec
	challenges *challengeGuard
}

// NewHandlers builds the passkey Handlers, constructing the WebAuthn relying party
// from the configured RP id / display name / origins. It returns an error when the
// RP configuration is invalid (so main.go fails fast at startup).
func NewHandlers(deps Deps) (*Handlers, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          deps.RPID,
		RPDisplayName: deps.RPDisplayName,
		RPOrigins:     deps.RPOrigins,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			// Resident keys preferred → enables usernameless (discoverable) login
			// without forcing it on constrained authenticators (design open Q).
			ResidentKey: protocol.ResidentKeyRequirementPreferred,
			// User verification preferred → broad device support (design open Q).
			UserVerification: protocol.VerificationPreferred,
		},
	})
	if err != nil {
		return nil, err
	}
	return &Handlers{
		deps:       deps,
		wa:         wa,
		state:      newStateCodec(deps.StateKey, deps.CookieSecure),
		challenges: newChallengeGuard(10 * time.Minute),
	}, nil
}

// Mount registers the passkey routes on r (the /api/v1 CSRF subrouter).
func (h *Handlers) Mount(r chi.Router) {
	r.Route("/passkeys", func(r chi.Router) {
		r.Post("/register/begin", h.RegisterBegin)
		r.Post("/register/finish", h.RegisterFinish)
		r.Get("/", h.List)
		r.Delete("/{id}", h.Delete)
	})
	r.Route("/auth/passkey/login", func(r chi.Router) {
		r.Post("/begin", h.LoginBegin)
		r.Post("/finish", h.LoginFinish)
	})
}

// --- response/request DTOs (camelCase JSON) ---

// credentialView is the client-safe projection of a stored credential.
type credentialView struct {
	ID         string   `json:"id" example:"7c6f0b6e-..."`
	Name       string   `json:"name" example:"MacBook Touch ID"`
	CreatedAt  string   `json:"createdAt" example:"2026-06-06T12:00:00Z"`
	LastUsedAt *string  `json:"lastUsedAt,omitempty"`
	Transports []string `json:"transports,omitempty" example:"internal,hybrid"`
}

type listResponse struct {
	Passkeys []credentialView `json:"passkeys"`
}

// credentialEnvelope wraps a single credential view in a {passkey} object, matching
// the codebase's single-object response convention (e.g. signup's {user}).
type credentialEnvelope struct {
	Passkey credentialView `json:"passkey"`
}

// registerFinishRequest carries the attestation plus the user-chosen nickname.
// Credential is the raw WebAuthn PublicKeyCredential JSON from the browser.
type registerFinishRequest struct {
	Name       string          `json:"name" example:"MacBook Touch ID"`
	Credential json.RawMessage `json:"credential" swaggertype:"object"`
}

// loginBeginRequest optionally narrows to a user (allow-list) and/or continues an
// in-progress OIDC login.
type loginBeginRequest struct {
	Email          string `json:"email,omitempty" example:"alex@cotton-id.io"`
	LoginChallenge string `json:"loginChallenge,omitempty"`
}

// loginFinishRequest carries the assertion (raw WebAuthn PublicKeyCredential JSON).
type loginFinishRequest struct {
	Credential json.RawMessage `json:"credential" swaggertype:"object"`
}

// loginFinishResponse returns either a redirect (OIDC continuation) or the user.
type loginFinishResponse struct {
	RedirectTo string           `json:"redirectTo,omitempty"`
	User       *auth.PublicUser `json:"user,omitempty"`
}

// --- registration ---

// RegisterBegin starts a passkey registration for the authenticated user.
//
// @Summary     Begin passkey registration
// @Description Returns WebAuthn CredentialCreationOptions for the current user, excluding their already-registered credentials, and sets the short-lived signed cid_wa ceremony cookie. Requires an authenticated session.
// @Tags        passkeys
// @Produce     json
// @Success     200 {object} protocol.CredentialCreation
// @Failure     401 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /passkeys/register/begin [post]
func (h *Handlers) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	stored, err := h.deps.Credentials.ListByUser(r.Context(), user.ID)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	waUser := newWebauthnUser(user, stored)

	// Exclude already-registered credentials so the same authenticator is not
	// double-registered.
	exclusions := make([]protocol.CredentialDescriptor, 0, len(stored))
	for _, c := range waUser.WebAuthnCredentials() {
		exclusions = append(exclusions, c.Descriptor())
	}

	creation, session, err := h.wa.BeginRegistration(waUser, webauthn.WithExclusions(exclusions))
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	if err := h.state.write(w, newRegisterState(session, user.ID.String())); err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	log.Info("passkey registration started", slog.String("user_id", user.ID.String()), slog.String("ip", httpx.ClientIP(r)))
	httpx.WriteJSON(w, http.StatusOK, creation)
}

// RegisterFinish verifies the attestation and stores the new credential.
//
// @Summary     Finish passkey registration
// @Description Verifies the authenticator's attestation against the cid_wa ceremony cookie and the configured relying party, stores the credential with the supplied nickname, and clears the cookie. Requires an authenticated session.
// @Tags        passkeys
// @Accept      json
// @Produce     json
// @Param       body body registerFinishRequest true "Attestation response plus nickname"
// @Success     201 {object} credentialEnvelope
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     409 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /passkeys/register/finish [post]
func (h *Handlers) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}

	st, err := h.state.read(r)
	if err != nil || st.Kind != ceremonyRegister {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid or expired registration state")
		return
	}
	// Single-use cookie; clear regardless of the outcome below.
	h.state.clear(w)

	// Server-side single-use: refuse a replayed ceremony challenge (the cookie
	// clear above is client-controlled and not authoritative).
	if !h.challenges.consume(st.Session.Challenge) {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "registration state already used or expired")
		return
	}

	// Bind: the ceremony cookie must belong to the currently-authenticated user.
	if st.UserID != user.ID.String() {
		log.Warn("passkey register finish rejected: user mismatch", slog.String("user_id", user.ID.String()), slog.String("ip", httpx.ClientIP(r)))
		httpx.WriteProblem(w, r, http.StatusBadRequest, "registration state does not match the current session")
		return
	}

	var req registerFinishRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Credential) == 0 {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "credential", "credential is required")
		return
	}
	// Bound the user-supplied nickname (input validation; avoids oversized rows /
	// API responses). The name is optional.
	name := strings.TrimSpace(req.Name)
	if utf8.RuneCountInString(name) > maxPasskeyNameLen {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "name", "passkey name is too long")
		return
	}

	parsed, err := protocol.ParseCredentialCreationResponseBytes(req.Credential)
	if err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid attestation response")
		return
	}

	stored, err := h.deps.Credentials.ListByUser(r.Context(), user.ID)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	waUser := newWebauthnUser(user, stored)

	credential, err := h.wa.CreateCredential(waUser, st.Session, parsed)
	if err != nil {
		log.Warn("passkey attestation verification failed", slog.String("user_id", user.ID.String()), slog.Any("error", err), slog.String("ip", httpx.ClientIP(r)))
		httpx.WriteProblem(w, r, http.StatusBadRequest, "could not verify the passkey")
		return
	}

	row, err := h.deps.Credentials.Create(r.Context(), CreateParams{
		UserID:          user.ID,
		CredentialID:    credential.ID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          credential.Authenticator.AAGUID,
		SignCount:       int64(credential.Authenticator.SignCount),
		Transports:      transportsToStrings(credential.Transport),
		Name:            name,
	})
	if err != nil {
		if errors.Is(err, ErrCredentialAlreadyRegistered) {
			httpx.WriteProblem(w, r, http.StatusConflict, "this passkey is already registered")
			return
		}
		httpx.WriteServerError(w, r, err)
		return
	}

	log.Info("passkey registered", slog.String("user_id", user.ID.String()), slog.String("credential_db_id", row.ID.String()), slog.String("ip", httpx.ClientIP(r)))
	httpx.WriteJSON(w, http.StatusCreated, credentialEnvelope{Passkey: toView(*row)})
}

// --- management ---

// List returns the authenticated user's passkeys.
//
// @Summary     List passkeys
// @Description Returns the current user's registered passkeys (id, nickname, created/last-used timestamps, transports). Requires an authenticated session.
// @Tags        passkeys
// @Produce     json
// @Success     200 {object} listResponse
// @Failure     401 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /passkeys [get]
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	stored, err := h.deps.Credentials.ListByUser(r.Context(), user.ID)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	out := listResponse{Passkeys: make([]credentialView, 0, len(stored))}
	for _, c := range stored {
		out.Passkeys = append(out.Passkeys, toView(c))
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Delete removes one of the authenticated user's passkeys.
//
// @Summary     Delete a passkey
// @Description Removes one of the current user's passkeys by its id. Returns 404 if the passkey does not exist or belongs to another account. Requires an authenticated session.
// @Tags        passkeys
// @Param       id path string true "Passkey id (UUID)"
// @Success     204 "No Content"
// @Failure     401 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /passkeys/{id} [delete]
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)

	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteProblem(w, r, http.StatusNotFound, "passkey not found")
		return
	}
	if err := h.deps.Credentials.DeleteForUser(r.Context(), user.ID, id); err != nil {
		if errors.Is(err, ErrCredentialNotFound) {
			httpx.WriteProblem(w, r, http.StatusNotFound, "passkey not found")
			return
		}
		httpx.WriteServerError(w, r, err)
		return
	}
	log.Info("passkey deleted", slog.String("user_id", user.ID.String()), slog.String("credential_db_id", id.String()), slog.String("ip", httpx.ClientIP(r)))
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// --- login ---

// LoginBegin starts a passwordless login ceremony.
//
// @Summary     Begin passkey login
// @Description Returns WebAuthn CredentialRequestOptions and sets the cid_wa ceremony cookie. When an email is supplied the options carry an allow-list of that account's credentials; when omitted a discoverable (usernameless) ceremony is issued. An optional loginChallenge is carried to continue an in-progress OIDC login. Pre-auth.
// @Tags        passkeys
// @Accept      json
// @Produce     json
// @Param       body body loginBeginRequest false "Optional email (allow-list) and loginChallenge"
// @Success     200 {object} protocol.CredentialAssertion
// @Failure     400 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /auth/passkey/login/begin [post]
func (h *Handlers) LoginBegin(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)

	// Body is optional; an empty body yields a discoverable ceremony.
	var req loginBeginRequest
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}

	var (
		assertion *protocol.CredentialAssertion
		session   *webauthn.SessionData
		err       error
	)

	// Username-first: narrow the ceremony to the account's credentials via an
	// allow-list HINT, but keep it a DISCOVERABLE ceremony (empty session.UserID)
	// so LoginFinish's single ValidatePasskeyLogin path verifies it. (Using
	// BeginLogin would set session.UserID, which ValidatePasskeyLogin rejects.) To
	// avoid account enumeration, an unknown email or one with no passkeys falls
	// through to a plain discoverable ceremony, indistinguishable from usernameless.
	var allowList []protocol.CredentialDescriptor
	if email := strings.TrimSpace(req.Email); email != "" {
		user, lookupErr := h.deps.Users.GetByEmail(r.Context(), email)
		if lookupErr == nil && user.Status == auth.StatusActive {
			stored, listErr := h.deps.Credentials.ListByUser(r.Context(), user.ID)
			if listErr != nil {
				httpx.WriteServerError(w, r, listErr)
				return
			}
			waUser := newWebauthnUser(user, stored)
			for _, c := range waUser.WebAuthnCredentials() {
				allowList = append(allowList, c.Descriptor())
			}
		}
	}

	if len(allowList) > 0 {
		assertion, session, err = h.wa.BeginDiscoverableLogin(webauthn.WithAllowedCredentials(allowList))
	} else {
		// Discoverable (usernameless) ceremony.
		assertion, session, err = h.wa.BeginDiscoverableLogin()
	}
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	if err := h.state.write(w, newLoginState(session, strings.TrimSpace(req.LoginChallenge))); err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	log.Info("passkey login started",
		slog.Bool("allow_list", len(allowList) > 0),
		slog.Bool("login_challenge", strings.TrimSpace(req.LoginChallenge) != ""),
		slog.String("ip", httpx.ClientIP(r)),
	)
	httpx.WriteJSON(w, http.StatusOK, assertion)
}

// LoginFinish verifies the assertion, runs clone detection, establishes a session,
// and continues an OIDC login if a challenge was carried.
//
// @Summary     Finish passkey login
// @Description Verifies the assertion against the cid_wa ceremony cookie and the configured relying party, rejects a non-increasing sign counter as a possible cloned authenticator, resolves the account, refuses non-active accounts, establishes a cotton-id session (same cookie as password login), and either accepts the carried Hydra login challenge (returning {redirectTo}) or returns {user}. Pre-auth.
// @Tags        passkeys
// @Accept      json
// @Produce     json
// @Param       body body loginFinishRequest true "Assertion response"
// @Success     200 {object} loginFinishResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /auth/passkey/login/finish [post]
func (h *Handlers) LoginFinish(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	ip := httpx.ClientIP(r)

	st, err := h.state.read(r)
	if err != nil || st.Kind != ceremonyLogin {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid or expired login state")
		return
	}
	// Single-use cookie; clear regardless of the outcome below.
	h.state.clear(w)

	// Server-side single-use: refuse a replayed ceremony challenge.
	if !h.challenges.consume(st.Session.Challenge) {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "login state already used or expired")
		return
	}

	var req loginFinishRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Credential) == 0 {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "credential", "credential is required")
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(req.Credential)
	if err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid assertion response")
		return
	}

	// Resolve the account + the asserting stored credential, and verify the
	// assertion. The discoverable handler looks up the credential by its raw id and
	// loads the owning user; the same path serves both allow-list and usernameless
	// ceremonies (the library passes session.UserID empty for discoverable).
	var assertingCred *StoredCredential
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		cred, lookupErr := h.deps.Credentials.GetByCredentialID(r.Context(), rawID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		assertingCred = cred
		user, userErr := h.deps.Users.GetByID(r.Context(), cred.UserID)
		if userErr != nil {
			return nil, userErr
		}
		stored, listErr := h.deps.Credentials.ListByUser(r.Context(), user.ID)
		if listErr != nil {
			return nil, listErr
		}
		return newWebauthnUser(user, stored), nil
	}

	waUser, validated, err := h.wa.ValidatePasskeyLogin(handler, st.Session, parsed)
	if err != nil || assertingCred == nil {
		h.deps.Metrics.LoginAttempts.WithLabelValues("failure").Inc()
		h.deps.Metrics.PasskeyLogins.WithLabelValues("failure").Inc()
		log.Info("passkey login failed: assertion not verified", slog.Any("error", err), slog.String("ip", ip))
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "passkey authentication failed")
		return
	}

	// Clone detection (D5): a non-increasing counter (when counters are in use) is
	// a possible cloned authenticator. Refuse and record a security event.
	if signCountRegressed(uint32(assertingCred.SignCount), validated.Authenticator.SignCount) {
		h.deps.Metrics.LoginAttempts.WithLabelValues("failure").Inc()
		h.deps.Metrics.PasskeyLogins.WithLabelValues("failure").Inc()
		log.Warn("passkey login refused: sign-count regression (possible cloned authenticator)",
			slog.String("user_id", assertingCred.UserID.String()),
			slog.String("credential_db_id", assertingCred.ID.String()),
			slog.Uint64("stored_sign_count", uint64(assertingCred.SignCount)),
			slog.Uint64("asserted_sign_count", uint64(validated.Authenticator.SignCount)),
			slog.String("ip", ip),
		)
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "passkey authentication failed")
		return
	}

	// Resolve the cotton-id account from the verified user adapter.
	account := waUser.(*webauthnUser).user

	// Status gate (parity with password/social login): a non-active account must
	// not establish a session or complete an OIDC login.
	if account.Status != auth.StatusActive {
		h.deps.Metrics.LoginAttempts.WithLabelValues("failure").Inc()
		h.deps.Metrics.PasskeyLogins.WithLabelValues("failure").Inc()
		log.Info("passkey login refused: account not active",
			slog.String("user_id", account.ID.String()), slog.String("status", account.Status), slog.String("ip", ip))
		httpx.WriteProblem(w, r, http.StatusForbidden, "account is not active")
		return
	}

	// Persist the advanced sign counter + last_used_at.
	if err := h.deps.Credentials.UpdateSignCount(r.Context(), assertingCred.CredentialID, int64(validated.Authenticator.SignCount)); err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	// Establish a cotton-id session exactly like password login (same store/TTLs).
	sess, err := h.deps.Auth.EstablishSession(r.Context(), account.ID, false, r.UserAgent(), ip)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	h.setSessionCookie(w, sess.Token, sess.Remember, sess.ExpiresAt)

	h.deps.Metrics.LoginAttempts.WithLabelValues("success").Inc()
	h.deps.Metrics.PasskeyLogins.WithLabelValues("success").Inc()
	log.Info("passkey login succeeded", slog.String("user_id", account.ID.String()), slog.String("ip", ip))

	// Best-effort login-notification email (new device + preference on). Captures
	// the user-agent/IP of THIS sign-in; never blocks the response below.
	h.maybeNotifyLogin(r, account, ip)

	// Continue an in-progress OIDC login, else return the user.
	if st.LoginChallenge != "" {
		redirect, err := h.deps.Hydra.AcceptLoginRequest(r.Context(), st.LoginChallenge, oidc.AcceptLogin{
			Subject: account.ID.String(),
		})
		if err != nil {
			log.Error("passkey login: hydra accept failed", slog.Any("error", err))
			httpx.WriteProblem(w, r, http.StatusBadGateway, "could not complete login")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, loginFinishResponse{RedirectTo: redirect.RedirectTo})
		return
	}

	pub := account.Public()
	httpx.WriteJSON(w, http.StatusOK, loginFinishResponse{User: &pub})
}

// --- helpers ---

// requireUser resolves the authenticated user from the session cookie or writes a
// 401 problem and returns ok=false. It is the auth gate for register/list/delete.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	c, err := r.Cookie(h.deps.SessionCookieName)
	if err != nil || c.Value == "" {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return nil, false
	}
	user, err := h.deps.Auth.UserForSession(r.Context(), c.Value)
	if err != nil {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return nil, false
	}
	return user, true
}

// setSessionCookie writes the session cookie with the SAME attributes the auth
// handlers use (HttpOnly, SameSite=Lax, Secure per config, persistent only when
// remember is set).
func (h *Handlers) setSessionCookie(w http.ResponseWriter, token string, remember bool, expiresAt time.Time) {
	c := &http.Cookie{
		Name:     h.deps.SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.deps.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	if remember {
		c.Expires = expiresAt
		c.MaxAge = int(time.Until(expiresAt).Seconds())
	}
	http.SetCookie(w, c)
}

// maybeNotifyLogin fires a best-effort login-notification email when the account
// has the preference enabled AND the (user-agent, ip) device is new relative to
// the user's recent sessions (excluding the one just created). Never blocks/fails
// the sign-in; nil-safe when the notifier or lister is unwired.
func (h *Handlers) maybeNotifyLogin(r *http.Request, user *auth.User, ip string) {
	if h.deps.Notifier == nil || h.deps.SessionLister == nil || user == nil || !user.LoginNotifications {
		return
	}
	device := notify.Device{UserAgent: r.UserAgent(), IP: ip}
	sessions, err := h.deps.SessionLister.ListByUser(r.Context(), user.ID)
	if err != nil {
		observability.LoggerFrom(r.Context(), h.deps.Logger).Warn("login notification: list sessions failed", slog.Any("error", err))
		return
	}
	prior := notify.ExcludingOne(sessionDevices(sessions), device)
	if !notify.IsNewDevice(prior, device) {
		return
	}
	h.deps.Notifier.SendLoginNotificationAsync(r.Context(), user.Email, user.DisplayName, device)
}

// sessionDevices maps sessions to their coarse device fingerprints for the
// new-device login-notification heuristic.
func sessionDevices(sessions []auth.Session) []notify.Device {
	out := make([]notify.Device, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, notify.Device{UserAgent: s.UserAgent, IP: s.IP})
	}
	return out
}

// toView projects a stored credential to its client-safe view.
func toView(c StoredCredential) credentialView {
	v := credentialView{
		ID:         c.ID.String(),
		Name:       c.Name,
		CreatedAt:  c.CreatedAt.UTC().Format(time.RFC3339),
		Transports: c.Transports,
	}
	if c.LastUsedAt != nil {
		s := c.LastUsedAt.UTC().Format(time.RFC3339)
		v.LastUsedAt = &s
	}
	return v
}

// compile-time seam checks.
var (
	_ SessionAuthenticator = (*auth.Service)(nil)
	_ hydraAccepter        = (*oidc.HydraClient)(nil)
	_ sessionLister        = (*auth.SessionStore)(nil)
)
