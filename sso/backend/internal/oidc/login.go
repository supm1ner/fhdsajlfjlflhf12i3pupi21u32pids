package oidc

import (
	"log/slog"
	"net/http"
	"strings"

	"cotton-id/internal/httpx"
)

// login.go — Hydra login-challenge flow (build-contract §3, steps 1-3).
//
//   - GET /oauth/login?login_challenge= : Hydra entry. If `skip` or a valid
//     cotton-id session exists → accept the challenge with subject = user.ID and
//     302 to Hydra's redirect_to. Else 302 → FRONTEND_BASE_URL/login?login_challenge=.
//   - POST /api/v1/oauth/login/accept {loginChallenge} : after the SPA login
//     establishes a session, accept the challenge and return {redirectTo}.

// loginAcceptRequest is the JSON body for POST /api/v1/oauth/login/accept.
type loginAcceptRequest struct {
	LoginChallenge string `json:"loginChallenge" example:"abc123challenge"`
}

// loginRejectRequest is the JSON body for POST /api/v1/oauth/login/reject.
type loginRejectRequest struct {
	LoginChallenge string `json:"loginChallenge" example:"abc123challenge"`
}

// redirectResponse is the uniform {redirectTo} response for the JSON OIDC
// endpoints; the SPA navigates the browser to it to continue the handshake.
type redirectResponse struct {
	RedirectTo string `json:"redirectTo" example:"http://localhost:4444/oauth2/auth?..."`
}

// LoginBrowser handles GET /oauth/login?login_challenge= — Hydra's login entry.
//
// @Summary     OIDC login entry (Hydra redirect target)
// @Description Hydra redirects the browser here with a login_challenge. If the user already has a valid cotton-id session (or Hydra allows skip), the challenge is accepted immediately and the browser is sent back to Hydra. Otherwise the browser is redirected to the SPA sign-in page.
// @Tags        oidc
// @Param       login_challenge query string true "Hydra login challenge"
// @Success     302 "Redirect to Hydra (accepted) or to the SPA sign-in page"
// @Failure     400 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Router      /oauth/login [get]
func (h *handlers) LoginBrowser(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)
	challenge := r.URL.Query().Get("login_challenge")
	if challenge == "" {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "missing login_challenge")
		return
	}

	lr, err := h.hydra.GetLoginRequest(r.Context(), challenge)
	if err != nil {
		log.Error("hydra get login request failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not start login")
		return
	}

	// Resolve the current cotton-id session (if any).
	user, ok, err := h.sessionUser(r)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	// Auto-accept when Hydra says we can skip (subject already authenticated for
	// this client) OR the browser carries a valid cotton-id session. In both
	// cases we re-assert the stable subject (the account id).
	if lr.Skip || ok {
		subject := lr.Subject
		if ok {
			subject = user.ID.String()
		}
		if subject == "" {
			// skip=true but no session and no subject: fall through to SPA login.
			h.redirectToSPALogin(w, r, challenge)
			return
		}
		redirect, err := h.hydra.AcceptLoginRequest(r.Context(), challenge, AcceptLogin{
			Subject: subject,
		})
		if err != nil {
			log.Error("hydra accept login failed", slog.Any("error", err))
			httpx.WriteProblem(w, r, http.StatusBadGateway, "could not complete login")
			return
		}
		log.Info("oidc login auto-accepted",
			slog.String("subject", subject),
			slog.Bool("skip", lr.Skip),
		)
		h.deps.Metrics.LoginAttempts.WithLabelValues("success").Inc()
		http.Redirect(w, r, redirect.RedirectTo, http.StatusFound)
		return
	}

	// No session: send the browser to the SPA sign-in page, carrying the challenge
	// so the SPA can finish the handshake after authentication.
	h.redirectToSPALogin(w, r, challenge)
}

// redirectToSPALogin 302s the browser to the SPA's sign-in route, preserving the
// login_challenge so the SPA can call POST /api/v1/oauth/login/accept afterward.
func (h *handlers) redirectToSPALogin(w http.ResponseWriter, r *http.Request, challenge string) {
	base := strings.TrimRight(h.deps.FrontendBaseURL, "/")
	target := base + "/login?login_challenge=" + urlQueryEscape(challenge)
	http.Redirect(w, r, target, http.StatusFound)
}

// LoginAccept handles POST /api/v1/oauth/login/accept — called by the SPA after
// it has established a cotton-id session via POST /api/v1/auth/login.
//
// @Summary     Accept an OIDC login challenge
// @Description After the SPA has logged the user in, accepts the Hydra login challenge with the session's stable subject and returns the URL to navigate back to Hydra.
// @Tags        oidc
// @Accept      json
// @Produce     json
// @Param       body body loginAcceptRequest true "Login challenge to accept"
// @Success     200 {object} redirectResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /oauth/login/accept [post]
func (h *handlers) LoginAccept(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)

	var req loginAcceptRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if req.LoginChallenge == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "loginChallenge", "loginChallenge is required")
		return
	}

	// Require an authenticated cotton-id session: the accept must bind to a real,
	// active account so we never hand Hydra an unauthenticated subject.
	user, ok, err := h.sessionUser(r)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}

	redirect, err := h.hydra.AcceptLoginRequest(r.Context(), req.LoginChallenge, AcceptLogin{
		Subject: user.ID.String(),
	})
	if err != nil {
		log.Error("hydra accept login failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not complete login")
		return
	}

	log.Info("oidc login accepted",
		slog.String("user_id", user.ID.String()),
		slog.String("ip", httpx.ClientIP(r)),
	)
	h.deps.Metrics.LoginAttempts.WithLabelValues("success").Inc()
	httpx.WriteJSON(w, http.StatusOK, redirectResponse{RedirectTo: redirect.RedirectTo})
}

// LoginReject handles POST /api/v1/oauth/login/reject — the SPA calls it when the
// user cancels sign-in (or after too many failed attempts), so the relying party
// receives an OAuth `access_denied` error from the login step instead of the flow
// hanging open. This satisfies the oidc-provider "failed authentication rejects
// the challenge" scenario.
//
// @Summary     Reject an OIDC login challenge
// @Description Cancels an in-progress sign-in, rejecting the Hydra login challenge with access_denied and returning the URL to send the user back to the relying party.
// @Tags        oidc
// @Accept      json
// @Produce     json
// @Param       body body loginRejectRequest true "Login challenge to reject"
// @Success     200 {object} redirectResponse
// @Failure     400 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /oauth/login/reject [post]
func (h *handlers) LoginReject(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)

	var req loginRejectRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if req.LoginChallenge == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "loginChallenge", "loginChallenge is required")
		return
	}

	redirect, err := h.hydra.RejectLoginRequest(r.Context(), req.LoginChallenge, RejectRequest{
		Error:            "access_denied",
		ErrorDescription: "The user cancelled the sign-in request.",
	})
	if err != nil {
		log.Error("hydra reject login failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not cancel login")
		return
	}

	log.Info("oidc login rejected by user", slog.String("ip", httpx.ClientIP(r)))
	httpx.WriteJSON(w, http.StatusOK, redirectResponse{RedirectTo: redirect.RedirectTo})
}
