package oidc

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cotton-id/internal/audit"
	"cotton-id/internal/auth"
	"cotton-id/internal/httpx"
	"cotton-id/internal/observability"
)

// Deps are the dependencies the OIDC handlers need. main.go populates this; the
// handlers read from it and must not add required fields main.go can't supply.
type Deps struct {
	Logger          *slog.Logger
	Metrics         *observability.Metrics
	Sessions        SessionVerifier
	HydraAdminURL   string
	HydraPublicURL  string
	FrontendBaseURL string
	PublicBaseURL   string
	// SessionCookieName is the cookie carrying the raw session token.
	SessionCookieName string
	// CookieSecure marks cleared cookies Secure (consistent with auth cookies).
	CookieSecure bool
	// Audit is an OPTIONAL audit-log sink. Nil is a safe no-op (tests omit it);
	// main.go wires the real writer. Used to record consent grant/deny alongside
	// the existing structured slog lines.
	Audit *audit.Writer
}

// Mount registers the OIDC JSON API routes on the /api/v1 subrouter
// (build-contract §3). These are browser-driven and CSRF-protected by the
// caller's middleware (the POSTs are state-changing).
//
//	GET  /api/v1/oauth/consent?consent_challenge=
//	POST /api/v1/oauth/login/accept
//	POST /api/v1/oauth/consent/accept
//	POST /api/v1/oauth/consent/reject
func Mount(r chi.Router, deps Deps) {
	h := newHandlers(deps)
	r.Route("/oauth", func(r chi.Router) {
		r.Get("/consent", h.ConsentDetails)
		r.Post("/login/accept", h.LoginAccept)
		r.Post("/login/reject", h.LoginReject)
		r.Post("/consent/accept", h.ConsentAccept)
		r.Post("/consent/reject", h.ConsentReject)
	})
}

// MountBrowser registers the browser-redirect OIDC routes on the ROOT router
// (not under /api). Hydra 302s the browser here with the *_challenge.
//
//	GET /oauth/login?login_challenge=
//	GET /oauth/consent?consent_challenge=
//	GET /oauth/logout?logout_challenge=
func MountBrowser(r chi.Router, deps Deps) {
	h := newHandlers(deps)
	r.Route("/oauth", func(r chi.Router) {
		r.Get("/login", h.LoginBrowser)
		r.Get("/consent", h.ConsentBrowser)
		r.Get("/logout", h.LogoutBrowser)
	})
}

// handlers holds the OIDC handler dependencies plus the Hydra admin client.
type handlers struct {
	deps  Deps
	hydra *HydraClient
}

// newHandlers constructs the handler set, building the Hydra admin client from
// the configured admin URL.
func newHandlers(deps Deps) *handlers {
	return &handlers{
		deps:  deps,
		hydra: NewHydraClient(deps.HydraAdminURL),
	}
}

// sessionUser resolves the current browser's cotton-id user from its session
// cookie, or returns (nil, false) when there is no valid active session. Errors
// other than the expected not-authenticated cases are returned for logging.
func (h *handlers) sessionUser(r *http.Request) (*auth.User, bool, error) {
	c, err := r.Cookie(h.deps.SessionCookieName)
	if err != nil || c.Value == "" {
		return nil, false, nil
	}
	user, err := h.deps.Sessions.UserForSession(r.Context(), c.Value)
	if err != nil {
		// The "no live session" cases are expected and not errors to surface.
		if errors.Is(err, auth.ErrSessionNotFound) ||
			errors.Is(err, auth.ErrUserNotFound) ||
			errors.Is(err, auth.ErrAccountNotActive) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return user, true, nil
}

// log returns the request-scoped logger.
func (h *handlers) log(r *http.Request) *slog.Logger {
	return observability.LoggerFrom(r.Context(), h.deps.Logger)
}

// LogoutBrowser handles GET /oauth/logout?logout_challenge= (build-contract §3).
// It accepts the Hydra logout challenge (best-effort), clears the cotton-id
// session, and 302s the browser to Hydra's post-logout redirect.
//
// @Summary     OIDC front-channel logout
// @Description Accepts Hydra's logout challenge, clears the cotton-id session, and redirects to the post-logout URL.
// @Tags        oidc
// @Param       logout_challenge query string true "Hydra logout challenge"
// @Success     302 "Redirect to Hydra post-logout URL"
// @Failure     400 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Router      /oauth/logout [get]
func (h *handlers) LogoutBrowser(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)
	challenge := r.URL.Query().Get("logout_challenge")
	if challenge == "" {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "missing logout_challenge")
		return
	}

	// Best-effort: read the logout request (for the subject, for logging).
	if lr, err := h.hydra.GetLogoutRequest(r.Context(), challenge); err == nil && lr != nil {
		log.Info("oidc logout accepted", slog.String("subject", lr.Subject))
	}

	redirect, err := h.hydra.AcceptLogoutRequest(r.Context(), challenge)
	if err != nil {
		log.Error("hydra accept logout failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not complete logout")
		return
	}

	// Clear the cotton-id session cookie so the browser is logged out locally too.
	// We cannot revoke the server-side session row without the raw token, but the
	// auth /logout endpoint covers that path; here we at least drop the cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     h.deps.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.deps.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	http.Redirect(w, r, redirect.RedirectTo, http.StatusFound)
}
