package social

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cotton-id/internal/auth"
	"cotton-id/internal/httpx"
	"cotton-id/internal/notify"
	"cotton-id/internal/observability"
	"cotton-id/internal/oidc"
)

// handlers.go — the social-login HTTP surface, mounted under /api/v1 by main.go:
//
//	GET /api/v1/auth/social/providers          → list enabled providers (no CSRF; GET)
//	GET /api/v1/auth/social/{provider}/start    → 302 to the provider authorize URL
//	GET /api/v1/auth/social/{provider}/callback → exchange, resolve, sign in, continue
//
// All three are GET browser routes (the OAuth redirect is a top-level GET), so no
// CSRF token applies; the anti-CSRF control for the OAuth round-trip is the signed
// `cid_oauth` state cookie (state.go), validated on callback.

// SessionEstablisher mints a cotton-id session for an already-authenticated user.
// *auth.Service satisfies it via EstablishSession.
type SessionEstablisher interface {
	EstablishSession(ctx context.Context, userID uuid.UUID, remember bool, ua, ip string) (*auth.EstablishedSession, error)
}

// hydraAccepter is the subset of *oidc.HydraClient the callback uses to continue
// an in-progress OIDC login. It is satisfied by *oidc.HydraClient.
type hydraAccepter interface {
	AcceptLoginRequest(ctx context.Context, challenge string, body oidc.AcceptLogin) (*oidc.RedirectTo, error)
}

// Deps are the dependencies the social handlers need. main.go populates this.
type Deps struct {
	Logger     *slog.Logger
	Metrics    *observability.Metrics
	Users      *auth.UserStore
	Identities *auth.SocialIdentityStore
	Sessions   *auth.Service
	Hydra      hydraAccepter

	// Social holds the per-provider credentials and enabled flags.
	Providers ProvidersConfig

	// PublicBaseURL is the backend's external URL; the redirect URI is derived as
	// PublicBaseURL + /api/v1/auth/social/{provider}/callback.
	PublicBaseURL string
	// FrontendBaseURL is where the browser lands after a non-OIDC social login.
	FrontendBaseURL string

	// SessionCookieName / CookieSecure mirror the auth handlers so the session
	// cookie social login sets is identical to the password-login one.
	SessionCookieName string
	CookieSecure      bool

	// StateKey signs the cid_oauth cookie (per-process random key from main.go).
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

// ProvidersConfig carries one entry per provider: its credentials and whether it
// is enabled. main.go builds this from config.SocialConfig so this package stays
// config-agnostic.
type ProvidersConfig struct {
	Google ProviderCredentials
	GitHub ProviderCredentials
	VK     ProviderCredentials
	Yandex ProviderCredentials
}

// ProviderCredentials is one provider's client id/secret and enabled flag.
type ProviderCredentials struct {
	ClientID     string
	ClientSecret string
	Enabled      bool
}

// Handlers serves the social-login API.
type Handlers struct {
	deps     Deps
	state    *stateCodec
	resolver *resolver
	registry map[string]*Provider
	creds    map[string]ProviderCredentials
	http     *http.Client
}

// NewHandlers builds the social Handlers. The HTTP client carries a bounded
// timeout so a hung provider can't pin a request goroutine.
func NewHandlers(deps Deps) *Handlers {
	registry := map[string]*Provider{
		ProviderGoogle: googleProvider(),
		ProviderGitHub: githubProvider(),
		ProviderVK:     vkProvider(),
		ProviderYandex: yandexProvider(),
	}
	creds := map[string]ProviderCredentials{
		ProviderGoogle: deps.Providers.Google,
		ProviderGitHub: deps.Providers.GitHub,
		ProviderVK:     deps.Providers.VK,
		ProviderYandex: deps.Providers.Yandex,
	}
	return &Handlers{
		deps:     deps,
		state:    newStateCodec(deps.StateKey, deps.CookieSecure),
		resolver: newResolver(deps.Users, deps.Identities),
		registry: registry,
		creds:    creds,
		http:     &http.Client{Timeout: 15 * time.Second},
	}
}

// Mount registers the social routes on r (the /api/v1 subrouter). These are GET
// browser routes; they need no CSRF token.
func (h *Handlers) Mount(r chi.Router) {
	r.Route("/auth/social", func(r chi.Router) {
		r.Get("/providers", h.Providers)
		r.Get("/{provider}/start", h.Start)
		r.Get("/{provider}/callback", h.Callback)
	})
}

// --- response DTOs (camelCase JSON) ---

type providerInfo struct {
	ID   string `json:"id" example:"google"`
	Name string `json:"name" example:"Google"`
}

type providersResponse struct {
	Providers []providerInfo `json:"providers"`
}

// providerOrder fixes the advertised ordering of providers.
var providerOrder = []string{ProviderGoogle, ProviderGitHub, ProviderVK, ProviderYandex}

// Providers lists the enabled social providers for the SPA to render.
//
// @Summary     List enabled social providers
// @Description Returns the social providers that have credentials configured, so the SPA renders only usable buttons.
// @Tags        social
// @Produce     json
// @Success     200 {object} providersResponse
// @Router      /auth/social/providers [get]
func (h *Handlers) Providers(w http.ResponseWriter, r *http.Request) {
	out := providersResponse{Providers: []providerInfo{}}
	for _, id := range providerOrder {
		if h.creds[id].Enabled {
			out.Providers = append(out.Providers, providerInfo{ID: id, Name: h.registry[id].DisplayName})
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// Start begins a social-login flow: it mints state (+ PKCE), stores it in the
// signed cid_oauth cookie, and 302s the browser to the provider's authorize URL.
//
// @Summary     Start social login
// @Description Redirects the browser to the chosen provider's authorization endpoint. A short-lived signed cookie carries the anti-CSRF state, the PKCE verifier (where supported), and any in-progress login_challenge.
// @Tags        social
// @Param       provider       path  string true  "Social provider" Enums(google, github, vk, yandex)
// @Param       login_challenge query string false "In-progress Hydra login challenge to continue after auth"
// @Success     302 "Redirect to the provider authorization endpoint"
// @Failure     400 {object} httpx.Problem
// @Router      /auth/social/{provider}/start [get]
func (h *Handlers) Start(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	providerID := chi.URLParam(r, "provider")

	provider, cred, ok := h.lookup(providerID)
	if !ok {
		log.Info("social start rejected: provider not enabled",
			slog.String("provider", providerID), slog.String("ip", httpx.ClientIP(r)))
		httpx.WriteProblem(w, r, http.StatusBadRequest, "social provider is not enabled")
		return
	}

	loginChallenge := r.URL.Query().Get("login_challenge")
	remember := r.URL.Query().Get("remember") == "true"

	st, err := newState(providerID, loginChallenge, remember, provider.UsesPKCE())
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	if err := h.state.write(w, st); err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	challenge := ""
	if provider.UsesPKCE() {
		challenge = pkceChallenge(st.PKCEVerifier)
	}
	authURL := provider.AuthCodeURL(cred.ClientID, h.redirectURI(providerID), st.State, challenge)

	log.Info("social login started",
		slog.String("provider", providerID),
		slog.Bool("login_challenge", loginChallenge != ""),
		slog.String("ip", httpx.ClientIP(r)),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback handles the provider's redirect: it validates state, exchanges the
// code, fetches the identity, resolves the account, establishes a session, and
// continues the OIDC handshake (or lands on the SPA).
//
// @Summary     Social login callback
// @Description Validates the OAuth state cookie, exchanges the authorization code, resolves (or creates/links) the cotton-id account by verified email, establishes a session, then continues the OIDC login challenge or redirects to the SPA.
// @Tags        social
// @Param       provider path  string true  "Social provider" Enums(google, github, vk, yandex)
// @Param       code     query string true  "Authorization code from the provider"
// @Param       state    query string true  "Anti-CSRF state echoed by the provider"
// @Success     302 "Redirect back into the OIDC flow or to the SPA"
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Router      /auth/social/{provider}/callback [get]
func (h *Handlers) Callback(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	ip := httpx.ClientIP(r)
	providerID := chi.URLParam(r, "provider")

	provider, cred, ok := h.lookup(providerID)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "social provider is not enabled")
		return
	}

	// Validate the signed state cookie and the echoed state — the OAuth anti-CSRF
	// control. Any failure rejects the request WITHOUT exchanging the code.
	st, err := h.state.read(r)
	if err != nil {
		log.Warn("social callback rejected: invalid state cookie",
			slog.String("provider", providerID), slog.String("ip", ip))
		httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid or expired login state")
		return
	}
	// The cookie is single-use; clear it now regardless of the rest.
	h.state.clear(w)

	queryState := r.URL.Query().Get("state")
	if queryState == "" || st.State != queryState || st.Provider != providerID {
		log.Warn("social callback rejected: state mismatch",
			slog.String("provider", providerID), slog.String("ip", ip))
		httpx.WriteProblem(w, r, http.StatusBadRequest, "login state mismatch")
		return
	}

	// Surface a provider-side error (user denied, etc.) before requiring a code.
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		log.Info("social callback: provider returned error",
			slog.String("provider", providerID), slog.String("error", errParam), slog.String("ip", ip))
		httpx.WriteProblem(w, r, http.StatusBadRequest, "the provider did not grant access")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "missing authorization code")
		return
	}

	// VK ID returns a device_id on the callback that the token exchange requires.
	extra := provider.callbackExtra(r)

	identity, err := provider.Exchange(r.Context(), h.http, exchangeParams{
		clientID:     cred.ClientID,
		clientSecret: cred.ClientSecret,
		redirectURI:  h.redirectURI(providerID),
		code:         code,
		codeVerifier: st.PKCEVerifier,
		extra:        extra,
	})
	if err != nil {
		log.Error("social token exchange failed",
			slog.String("provider", providerID), slog.String("ip", ip), slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not complete social sign-in")
		return
	}
	if identity.Subject == "" {
		httpx.WriteProblem(w, r, http.StatusBadGateway, "the provider did not return an account identifier")
		return
	}

	// Resolve (find / link / create) the cotton-id account — the D3 security crux.
	res, err := h.resolver.Resolve(r.Context(), providerID, identity)
	if err != nil {
		log.Error("social account resolution failed",
			slog.String("provider", providerID), slog.String("ip", ip), slog.Any("error", err))
		httpx.WriteServerError(w, r, err)
		return
	}

	// Status gate (parity with password login / UserForSession): a suspended or
	// otherwise non-active account must not establish a session or complete an
	// OIDC login, even with a linked provider.
	if res.User.Status != auth.StatusActive {
		log.Info("social login refused: account not active",
			slog.String("provider", providerID),
			slog.String("user_id", res.User.ID.String()),
			slog.String("status", res.User.Status),
			slog.String("ip", ip),
		)
		h.deps.Metrics.LoginAttempts.WithLabelValues("failure").Inc()
		h.deps.Metrics.SocialLogins.WithLabelValues(providerID, "failure").Inc()
		httpx.WriteProblem(w, r, http.StatusForbidden, "account is not active")
		return
	}

	// Establish a cotton-id session exactly like password login (same store/TTLs).
	sess, err := h.deps.Sessions.EstablishSession(r.Context(), res.User.ID, st.Remember, r.UserAgent(), ip)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	h.setSessionCookie(w, sess.Token, sess.Remember, sess.ExpiresAt)

	log.Info("social login succeeded",
		slog.String("provider", providerID),
		slog.String("user_id", res.User.ID.String()),
		slog.String("outcome", string(res.Outcome)),
		slog.Bool("email_verified", identity.EmailVerified),
		slog.String("ip", ip),
	)
	h.deps.Metrics.LoginAttempts.WithLabelValues("success").Inc()
	h.deps.Metrics.SocialLogins.WithLabelValues(providerID, "success").Inc()

	// Best-effort login-notification email (new device + preference on). Captures
	// the user-agent/IP of THIS sign-in; never blocks the redirect below.
	h.maybeNotifyLogin(r, res.User, ip)

	// Continue an in-progress OIDC login, else land on the SPA.
	if st.LoginChallenge != "" {
		redirect, err := h.deps.Hydra.AcceptLoginRequest(r.Context(), st.LoginChallenge, oidc.AcceptLogin{
			Subject: res.User.ID.String(),
		})
		if err != nil {
			log.Error("social login: hydra accept failed",
				slog.String("provider", providerID), slog.Any("error", err))
			httpx.WriteProblem(w, r, http.StatusBadGateway, "could not complete login")
			return
		}
		http.Redirect(w, r, redirect.RedirectTo, http.StatusFound)
		return
	}

	http.Redirect(w, r, strings.TrimRight(h.deps.FrontendBaseURL, "/")+"/", http.StatusFound)
}

// --- helpers ---

// lookup resolves an enabled provider + its credentials, or ok=false when the
// provider is unknown or not configured.
func (h *Handlers) lookup(providerID string) (*Provider, ProviderCredentials, bool) {
	provider, known := h.registry[providerID]
	cred, hasCred := h.creds[providerID]
	if !known || !hasCred || !cred.Enabled {
		return nil, ProviderCredentials{}, false
	}
	return provider, cred, true
}

// redirectURI derives the per-provider callback URL from PUBLIC_BASE_URL.
func (h *Handlers) redirectURI(providerID string) string {
	return strings.TrimRight(h.deps.PublicBaseURL, "/") + "/api/v1/auth/social/" + providerID + "/callback"
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

// callbackExtra returns the provider-specific token-request params read from the
// callback query. VK ID returns a device_id that the token exchange requires.
func (p *Provider) callbackExtra(r *http.Request) url.Values {
	if p.ID != ProviderVK {
		return nil
	}
	extra := url.Values{}
	if dev := r.URL.Query().Get("device_id"); dev != "" {
		extra.Set("device_id", dev)
	}
	if st := r.URL.Query().Get("state"); st != "" {
		extra.Set("state", st)
	}
	return extra
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

// ensure the auth.Service satisfies SessionEstablisher at compile time.
var (
	_ SessionEstablisher = (*auth.Service)(nil)
	_ sessionLister      = (*auth.SessionStore)(nil)
)
