package oidc

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"cotton-id/internal/audit"
	"cotton-id/internal/auth"
	"cotton-id/internal/httpx"
)

// consent.go — Hydra consent-challenge flow (build-contract §3, steps 4-5).
//
//   - GET /oauth/consent?consent_challenge= : Hydra entry. If skippable → accept
//     (granting the requested scopes + claims) and 302 back; else 302 →
//     FRONTEND_BASE_URL/consent?consent_challenge=.
//   - GET /api/v1/oauth/consent?consent_challenge= : return {client,requestedScopes,user}
//     for the SPA to render the consent screen.
//   - POST /api/v1/oauth/consent/accept {consentChallenge,grantScopes,remember} → {redirectTo}
//   - POST /api/v1/oauth/consent/reject {consentChallenge} → {redirectTo}

// rememberConsentForSeconds is how long Hydra remembers a "remember" consent
// grant (30 days), matching the design's remembered-decision affordance.
const rememberConsentForSeconds = int64(30 * 24 * 60 * 60)

// clientInfo is the client-safe projection of the requesting relying party.
type clientInfo struct {
	ID   string `json:"id" example:"demo-client"`
	Name string `json:"name" example:"Demo App"`
}

// consentDetailsResponse is the GET /api/v1/oauth/consent payload the SPA renders.
type consentDetailsResponse struct {
	Client          clientInfo      `json:"client"`
	RequestedScopes []string        `json:"requestedScopes"`
	User            auth.PublicUser `json:"user"`
}

// consentAcceptRequest is the body for POST /api/v1/oauth/consent/accept.
type consentAcceptRequest struct {
	ConsentChallenge string   `json:"consentChallenge" example:"def456challenge"`
	GrantScopes      []string `json:"grantScopes" example:"openid,profile,email"`
	Remember         bool     `json:"remember" example:"true"`
}

// consentRejectRequest is the body for POST /api/v1/oauth/consent/reject.
type consentRejectRequest struct {
	ConsentChallenge string `json:"consentChallenge" example:"def456challenge"`
}

// ConsentBrowser handles GET /oauth/consent?consent_challenge= — Hydra's consent
// entry, served at the SERVER ROOT (not under /api/v1). Skippable requests (a
// remembered grant) are accepted immediately; otherwise the browser is
// redirected to the SPA consent page.
//
// It is intentionally NOT given its own swaggo @Router block: under the single
// /api/v1 BasePath swaggo cannot distinguish this root-level GET /oauth/consent
// from the documented JSON endpoint of the same path (ConsentDetails), and its
// redirect behavior mirrors the documented GET /oauth/login entry. Keeping one
// Swagger entry per path avoids a duplicate-route collision in the spec.
func (h *handlers) ConsentBrowser(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)
	challenge := r.URL.Query().Get("consent_challenge")
	if challenge == "" {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "missing consent_challenge")
		return
	}

	cr, err := h.hydra.GetConsentRequest(r.Context(), challenge)
	if err != nil {
		log.Error("hydra get consent request failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not start consent")
		return
	}

	// Skippable consent (Hydra remembered a prior grant): accept silently with the
	// user's claims for the requested scopes.
	if cr.Skip {
		user, err := h.deps.Sessions.UserForSession(r.Context(), h.rawSessionToken(r))
		if err != nil {
			// Skip is true but we can't resolve the user; fall back to the SPA so a
			// fresh decision can be made rather than failing the flow.
			h.redirectToSPAConsent(w, r, challenge)
			return
		}
		redirect, err := h.acceptConsent(r, challenge, cr.RequestedScope, cr.RequestedScope, user, false)
		if err != nil {
			log.Error("hydra accept consent (skip) failed", slog.Any("error", err))
			httpx.WriteProblem(w, r, http.StatusBadGateway, "could not complete consent")
			return
		}
		log.Info("oidc consent auto-accepted (skip)", slog.String("subject", cr.Subject))
		h.deps.Metrics.ConsentDecisions.WithLabelValues("accept").Inc()
		http.Redirect(w, r, redirect.RedirectTo, http.StatusFound)
		return
	}

	h.redirectToSPAConsent(w, r, challenge)
}

// redirectToSPAConsent 302s the browser to the SPA consent route, carrying the
// challenge so the SPA can fetch details and post the decision.
func (h *handlers) redirectToSPAConsent(w http.ResponseWriter, r *http.Request, challenge string) {
	base := strings.TrimRight(h.deps.FrontendBaseURL, "/")
	target := base + "/consent?consent_challenge=" + urlQueryEscape(challenge)
	http.Redirect(w, r, target, http.StatusFound)
}

// ConsentDetails handles GET /api/v1/oauth/consent?consent_challenge= — the SPA
// fetches the requesting client and requested scopes to render the consent UI.
//
// @Summary     Get OIDC consent details
// @Description Returns the requesting client, the requested scopes, and the current user so the SPA can render the consent screen.
// @Tags        oidc
// @Produce     json
// @Param       consent_challenge query string true "Hydra consent challenge"
// @Success     200 {object} consentDetailsResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Router      /oauth/consent [get]
func (h *handlers) ConsentDetails(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)
	challenge := r.URL.Query().Get("consent_challenge")
	if challenge == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "consent_challenge", "consent_challenge is required")
		return
	}

	// The consent screen must be shown to the authenticated user.
	user, ok, err := h.sessionUser(r)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}

	cr, err := h.hydra.GetConsentRequest(r.Context(), challenge)
	if err != nil {
		log.Error("hydra get consent request failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not load consent")
		return
	}

	ci := clientInfo{}
	if cr.Client != nil {
		ci.ID = cr.Client.ClientID
		ci.Name = cr.Client.ClientName
		if ci.Name == "" {
			ci.Name = cr.Client.ClientID
		}
	}

	httpx.WriteJSON(w, http.StatusOK, consentDetailsResponse{
		Client:          ci,
		RequestedScopes: cr.RequestedScope,
		User:            user.Public(),
	})
}

// ConsentAccept handles POST /api/v1/oauth/consent/accept — the user grants the
// consent. Only the requested scopes that the user grants are passed to Hydra,
// along with the ID-token claims for those scopes and an optional remember.
//
// @Summary     Accept an OIDC consent challenge
// @Description Grants consent for the chosen scopes, attaches the user's identity claims, optionally remembers the decision, and returns the URL to navigate back to Hydra.
// @Tags        oidc
// @Accept      json
// @Produce     json
// @Param       body body consentAcceptRequest true "Consent decision"
// @Success     200 {object} redirectResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /oauth/consent/accept [post]
func (h *handlers) ConsentAccept(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)

	var req consentAcceptRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if req.ConsentChallenge == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "consentChallenge", "consentChallenge is required")
		return
	}

	user, ok, err := h.sessionUser(r)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}

	// Re-fetch the consent request so we never grant a scope the client did not
	// request, even if the SPA posts a wider set (defense against a tampered body).
	cr, err := h.hydra.GetConsentRequest(r.Context(), req.ConsentChallenge)
	if err != nil {
		log.Error("hydra get consent request failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not load consent")
		return
	}

	// Granted = (scopes the user agreed to) ∩ (scopes the client requested).
	granted := scopeIntersection(req.GrantScopes, cr.RequestedScope)

	redirect, err := h.acceptConsent(r, req.ConsentChallenge, granted, cr.RequestedScope, user, req.Remember)
	if err != nil {
		log.Error("hydra accept consent failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not complete consent")
		return
	}

	log.Info("oidc consent granted",
		slog.String("user_id", user.ID.String()),
		slog.String("client_id", clientID(cr)),
		slog.Any("granted_scopes", granted),
		slog.Bool("remember", req.Remember),
		slog.String("ip", httpx.ClientIP(r)),
	)
	_ = h.deps.Audit.Append(r.Context(), audit.FromRequest(r, audit.ActionConsentGrant).
		WithActor(user.ID, user.Username).
		WithTarget(audit.TargetClient, clientID(cr)).
		WithMetadata(map[string]any{"grantedScopes": granted, "remember": req.Remember}))
	h.deps.Metrics.ConsentDecisions.WithLabelValues("accept").Inc()
	httpx.WriteJSON(w, http.StatusOK, redirectResponse{RedirectTo: redirect.RedirectTo})
}

// ConsentReject handles POST /api/v1/oauth/consent/reject — the user denies
// consent; Hydra returns the relying party an access_denied error.
//
// @Summary     Reject an OIDC consent challenge
// @Description Denies the consent request; the relying party receives an access_denied error. Returns the URL to navigate back to Hydra.
// @Tags        oidc
// @Accept      json
// @Produce     json
// @Param       body body consentRejectRequest true "Consent challenge to reject"
// @Success     200 {object} redirectResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /oauth/consent/reject [post]
func (h *handlers) ConsentReject(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)

	var req consentRejectRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if req.ConsentChallenge == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "consentChallenge", "consentChallenge is required")
		return
	}

	// Require an authenticated session so an unauthenticated caller cannot drive
	// the consent decision for someone else's challenge.
	user, ok, err := h.sessionUser(r)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}

	redirect, err := h.hydra.RejectConsentRequest(r.Context(), req.ConsentChallenge, RejectRequest{
		Error:            "access_denied",
		ErrorDescription: "The resource owner denied the request",
		StatusCode:       http.StatusForbidden,
	})
	if err != nil {
		log.Error("hydra reject consent failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not complete consent")
		return
	}

	log.Info("oidc consent denied", slog.String("ip", httpx.ClientIP(r)))
	_ = h.deps.Audit.Append(r.Context(), audit.FromRequest(r, audit.ActionConsentDeny).
		WithActor(user.ID, user.Username))
	h.deps.Metrics.ConsentDecisions.WithLabelValues("reject").Inc()
	httpx.WriteJSON(w, http.StatusOK, redirectResponse{RedirectTo: redirect.RedirectTo})
}

// acceptConsent builds the Hydra accept-consent body from the granted scopes and
// the user's claims (scope-gated), then accepts the challenge. grantScopes is the
// set actually granted; claimsScopes gates which claims are attached (normally
// the same set). It does NOT write any HTTP response.
func (h *handlers) acceptConsent(r *http.Request, challenge string, grantScopes, claimsScopes []string, user *auth.User, remember bool) (*RedirectTo, error) {
	claims := ClaimsForUser(user, claimsScopes)
	body := AcceptConsent{
		GrantScope: grantScopes,
		Session: &ConsentSession{
			IDToken: claims,
		},
		Remember: remember,
	}
	if remember {
		body.RememberFor = rememberConsentForSeconds
	}
	return h.hydra.AcceptConsentRequest(r.Context(), challenge, body)
}

// rawSessionToken returns the raw session cookie value, or "" when absent.
func (h *handlers) rawSessionToken(r *http.Request) string {
	c, err := r.Cookie(h.deps.SessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// clientID returns the consent request's client id (or "" when unknown), for logs.
func clientID(cr *ConsentRequest) string {
	if cr != nil && cr.Client != nil {
		return cr.Client.ClientID
	}
	return ""
}

// urlQueryEscape escapes a value for safe inclusion in a URL query string.
func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}
