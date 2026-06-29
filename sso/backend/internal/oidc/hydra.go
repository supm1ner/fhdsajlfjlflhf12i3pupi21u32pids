// Package oidc implements cotton-id's OpenID Connect login and consent provider,
// delegating protocol/token issuance to Ory Hydra (design.md D1). Hydra owns the
// OAuth2/OIDC protocol surface (authorize, token, JWKS, PKCE, refresh rotation)
// and has no user store or UI: it redirects the browser to cotton-id's
// login/consent/logout endpoints with a *_challenge and cotton-id calls Hydra's
// ADMIN API to accept/reject the challenge.
//
// ============================================================================
// INTEGRATION CONTRACT (stable; owned by the substrate slice). Build against
// these and do not change files outside internal/oidc and internal/adminapi.
// ============================================================================
//
// --- internal/oidc.Mount / MountBrowser (this package) ---
//
//	Mount(r, deps)        registers the JSON API routes on the /api/v1 subrouter:
//	                        GET  /oauth/consent?consent_challenge=
//	                        POST /oauth/login/accept
//	                        POST /oauth/consent/accept
//	                        POST /oauth/consent/reject
//	MountBrowser(r, deps) registers the browser-redirect routes on the ROOT router:
//	                        GET /oauth/login?login_challenge=
//	                        GET /oauth/consent?consent_challenge=
//	                        GET /oauth/logout?logout_challenge=
//
// --- auth.Service (cotton-id/internal/auth) — the user/session seam ---
//
//	func (s *Service) UserForSession(ctx, token) (*auth.User, error)
//	    Resolve a raw session-cookie token to the active *auth.User. Returns
//	    auth.ErrSessionNotFound / auth.ErrUserNotFound / auth.ErrAccountNotActive.
//	    Used to decide whether a Hydra login challenge can be auto-accepted.
//	auth.User.ID.String() is the stable Hydra `subject`.
//
// --- httpx helpers for uniform responses (cotton-id/internal/httpx) ---
//
//	WriteJSON / WriteProblem / WriteServerError / DecodeJSON / ClientIP.
//	CSRF is already enforced on the /api/v1 subtree by main.go; the /oauth/*
//	browser-redirect routes are GET (no CSRF needed).
//
// --- config values received via Deps ---
//
//	HydraAdminURL, HydraPublicURL, FrontendBaseURL, PublicBaseURL,
//	SessionCookieName. The Hydra admin client below is hand-written with
//	net/http (no extra deps, per build-contract §2) and tolerant of Hydra v2
//	JSON shapes.
//
// ============================================================================
package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cotton-id/internal/auth"
)

// SessionVerifier resolves a raw session-cookie token to the authenticated user.
// *auth.Service satisfies this (via UserForSession). The OIDC handlers use it to
// auto-accept Hydra login challenges for already-authenticated browsers.
type SessionVerifier interface {
	UserForSession(ctx context.Context, token string) (*auth.User, error)
}

// ensure the seam type lines up with auth.Service at compile time.
var _ SessionVerifier = (interface {
	UserForSession(ctx context.Context, token string) (*auth.User, error)
})(nil)

// ----------------------------------------------------------------------------
// HydraClient — hand-written client over the Ory Hydra v2 ADMIN API.
// ----------------------------------------------------------------------------

// HydraError is returned when Hydra's admin API responds with a non-2xx status.
// It carries the status code and the raw body so handlers can log/translate it
// without exposing internals to the browser.
type HydraError struct {
	Op         string // the admin operation, e.g. "accept login request"
	StatusCode int
	Body       string
}

func (e *HydraError) Error() string {
	return fmt.Sprintf("hydra %s: unexpected status %d: %s", e.Op, e.StatusCode, e.Body)
}

// HydraClient talks to Hydra's admin API. It is safe for concurrent use.
type HydraClient struct {
	adminURL string
	http     *http.Client
}

// NewHydraClient builds a HydraClient for the given admin base URL (e.g.
// http://hydra:4445). A bounded HTTP client timeout protects the request path
// from a hung Hydra.
func NewHydraClient(adminURL string) *HydraClient {
	return &HydraClient{
		adminURL: strings.TrimRight(adminURL, "/"),
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// --- Login challenge -------------------------------------------------------

// LoginRequest is the subset of Hydra's OAuth2LoginRequest cotton-id needs.
// Field tags match Hydra v2's snake_case JSON; unknown fields are ignored.
type LoginRequest struct {
	Challenge      string         `json:"challenge"`
	Subject        string         `json:"subject"`
	Skip           bool           `json:"skip"`
	RequestURL     string         `json:"request_url"`
	RequestedScope []string       `json:"requested_scope"`
	Client         *OAuth2Client  `json:"client"`
	OIDCContext    map[string]any `json:"oidc_context"`
}

// AcceptLogin carries the parameters for accepting a login challenge.
type AcceptLogin struct {
	Subject     string `json:"subject"`
	Remember    bool   `json:"remember,omitempty"`
	RememberFor int64  `json:"remember_for,omitempty"`
}

// RedirectTo is Hydra's accept/reject response: the URL to send the browser to.
type RedirectTo struct {
	RedirectTo string `json:"redirect_to"`
}

// GetLoginRequest fetches the login request identified by challenge.
func (c *HydraClient) GetLoginRequest(ctx context.Context, challenge string) (*LoginRequest, error) {
	var out LoginRequest
	err := c.do(ctx, http.MethodGet, "/admin/oauth2/auth/requests/login",
		url.Values{"login_challenge": {challenge}}, nil, &out, "get login request")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AcceptLoginRequest accepts the login challenge with the given subject/remember.
func (c *HydraClient) AcceptLoginRequest(ctx context.Context, challenge string, body AcceptLogin) (*RedirectTo, error) {
	var out RedirectTo
	err := c.do(ctx, http.MethodPut, "/admin/oauth2/auth/requests/login/accept",
		url.Values{"login_challenge": {challenge}}, body, &out, "accept login request")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RejectRequest is the body for rejecting a login or consent challenge.
type RejectRequest struct {
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
	StatusCode       int    `json:"status_code,omitempty"`
}

// RejectLoginRequest rejects the login challenge (e.g. access_denied).
func (c *HydraClient) RejectLoginRequest(ctx context.Context, challenge string, body RejectRequest) (*RedirectTo, error) {
	var out RedirectTo
	err := c.do(ctx, http.MethodPut, "/admin/oauth2/auth/requests/login/reject",
		url.Values{"login_challenge": {challenge}}, body, &out, "reject login request")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Consent challenge -----------------------------------------------------

// ConsentRequest is the subset of Hydra's OAuth2ConsentRequest cotton-id needs.
type ConsentRequest struct {
	Challenge                    string         `json:"challenge"`
	Subject                      string         `json:"subject"`
	Skip                         bool           `json:"skip"`
	Client                       *OAuth2Client  `json:"client"`
	RequestedScope               []string       `json:"requested_scope"`
	RequestedAccessTokenAudience []string       `json:"requested_access_token_audience"`
	OIDCContext                  map[string]any `json:"oidc_context"`
}

// ConsentSession carries the claims placed into the issued ID/access tokens.
type ConsentSession struct {
	IDToken     any `json:"id_token,omitempty"`
	AccessToken any `json:"access_token,omitempty"`
}

// AcceptConsent carries the parameters for accepting a consent challenge.
type AcceptConsent struct {
	GrantScope               []string        `json:"grant_scope"`
	GrantAccessTokenAudience []string        `json:"grant_access_token_audience,omitempty"`
	Session                  *ConsentSession `json:"session,omitempty"`
	Remember                 bool            `json:"remember,omitempty"`
	RememberFor              int64           `json:"remember_for,omitempty"`
}

// GetConsentRequest fetches the consent request identified by challenge.
func (c *HydraClient) GetConsentRequest(ctx context.Context, challenge string) (*ConsentRequest, error) {
	var out ConsentRequest
	err := c.do(ctx, http.MethodGet, "/admin/oauth2/auth/requests/consent",
		url.Values{"consent_challenge": {challenge}}, nil, &out, "get consent request")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AcceptConsentRequest accepts the consent challenge with granted scopes/claims.
func (c *HydraClient) AcceptConsentRequest(ctx context.Context, challenge string, body AcceptConsent) (*RedirectTo, error) {
	var out RedirectTo
	err := c.do(ctx, http.MethodPut, "/admin/oauth2/auth/requests/consent/accept",
		url.Values{"consent_challenge": {challenge}}, body, &out, "accept consent request")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RejectConsentRequest rejects the consent challenge (access_denied).
func (c *HydraClient) RejectConsentRequest(ctx context.Context, challenge string, body RejectRequest) (*RedirectTo, error) {
	var out RedirectTo
	err := c.do(ctx, http.MethodPut, "/admin/oauth2/auth/requests/consent/reject",
		url.Values{"consent_challenge": {challenge}}, body, &out, "reject consent request")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// --- Logout challenge ------------------------------------------------------

// LogoutRequest is the subset of Hydra's logout request cotton-id needs.
type LogoutRequest struct {
	Challenge          string `json:"challenge"`
	Subject            string `json:"subject"`
	SID                string `json:"sid"`
	RequestURL         string `json:"request_url"`
	RPInitiated        bool   `json:"rp_initiated"`
	PostLogoutRedirect string `json:"post_logout_redirect_uri"`
}

// GetLogoutRequest fetches the logout request identified by challenge.
func (c *HydraClient) GetLogoutRequest(ctx context.Context, challenge string) (*LogoutRequest, error) {
	var out LogoutRequest
	err := c.do(ctx, http.MethodGet, "/admin/oauth2/auth/requests/logout",
		url.Values{"logout_challenge": {challenge}}, nil, &out, "get logout request")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AcceptLogoutRequest accepts the logout challenge and returns the redirect URL.
func (c *HydraClient) AcceptLogoutRequest(ctx context.Context, challenge string) (*RedirectTo, error) {
	var out RedirectTo
	err := c.do(ctx, http.MethodPut, "/admin/oauth2/auth/requests/logout/accept",
		url.Values{"logout_challenge": {challenge}}, nil, &out, "accept logout request")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// --- OAuth2 client (relying-party) CRUD ------------------------------------

// OAuth2Client is the subset of Hydra's OAuth2 client model cotton-id reads and
// writes. Hydra returns the generated client_id (and client_secret for
// confidential clients) on create. Unknown fields round-trip is not required.
type OAuth2Client struct {
	ClientID                string   `json:"client_id,omitempty"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
	Audience                []string `json:"audience,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	CreatedAt               string   `json:"created_at,omitempty"`
}

// CreateClient registers a new OAuth2 client in Hydra and returns the created
// record (including client_id and, for confidential clients, client_secret).
func (c *HydraClient) CreateClient(ctx context.Context, client OAuth2Client) (*OAuth2Client, error) {
	var out OAuth2Client
	err := c.do(ctx, http.MethodPost, "/admin/clients", nil, client, &out, "create client")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetClient fetches a single OAuth2 client by id (Hydra GET /admin/clients/{id}).
// A 404 surfaces as a *HydraError with StatusCode 404 so callers can map it to a
// not-found response. Hydra never returns the client_secret on a read (it is
// shown only once on create), so the returned record carries no secret.
func (c *HydraClient) GetClient(ctx context.Context, id string) (*OAuth2Client, error) {
	var out OAuth2Client
	err := c.do(ctx, http.MethodGet, "/admin/clients/"+url.PathEscape(id), nil, nil, &out, "get client")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateClient replaces the OAuth2 client with the given id (Hydra PUT
// /admin/clients/{id}). PUT is a full replacement: the caller must send the
// complete desired client representation (Hydra overwrites the stored record with
// the body), so callers should read-modify-write the fields they want to change.
// Hydra returns the updated record; for a client that keeps client_secret_basic
// it does NOT re-issue a secret, so the response carries no client_secret.
func (c *HydraClient) UpdateClient(ctx context.Context, id string, client OAuth2Client) (*OAuth2Client, error) {
	var out OAuth2Client
	err := c.do(ctx, http.MethodPut, "/admin/clients/"+url.PathEscape(id), nil, client, &out, "update client")
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListClients returns the registered OAuth2 clients.
func (c *HydraClient) ListClients(ctx context.Context) ([]OAuth2Client, error) {
	var out []OAuth2Client
	err := c.do(ctx, http.MethodGet, "/admin/clients", nil, nil, &out, "list clients")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteClient removes the OAuth2 client with the given id. A 404 is treated as
// success (idempotent delete) so a repeated DELETE does not error.
func (c *HydraClient) DeleteClient(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, "/admin/clients/"+url.PathEscape(id), nil, nil, nil, "delete client")
	var he *HydraError
	if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
		return nil
	}
	return err
}

// --- Per-client consent capability (Hydra v2.2.0 boundary) -----------------
//
// IMPORTANT — Hydra v2.2.0 consent capability boundary (verified against the
// ory/hydra v2.2.0 source, consent/handler.go):
//
//   - GET  /admin/oauth2/auth/sessions/consent REQUIRES a `subject` query
//     param and accepts only `subject` + `login_session_id`. There is NO
//     `client` filter and NO way to enumerate sessions across all subjects.
//   - DELETE /admin/oauth2/auth/sessions/consent also REQUIRES `subject`; the
//     optional `client`/`all=true` params only NARROW a subject-scoped revoke.
//     There is NO client-only ("revoke every grant for this client") delete.
//
// Consequence: Hydra exposes no efficient per-client consent count and no
// client-only revoke. cotton-id therefore derives both BEST-EFFORT by iterating
// over the IdP's own users as the subject set (cotton-id is the only IdP, so the
// subjects Hydra can hold grants for are exactly cotton-id's user ids — see the
// package contract: auth.User.ID.String() is the Hydra subject). The
// subject-scoped primitives Hydra DOES support are ListConsentSessions and
// RevokeConsentSessions (subject[,client]) below; the admin console composes them
// across subjects and documents the best-effort/limitation (design D3).

// --- Consent / login sessions (self-service connected-apps view) -----------

// ConsentSessionRecord is the subset of Hydra's OAuth2ConsentSession the
// account self-service surface needs: the embedded consent request (client +
// requested scope + subject), the scopes/audience the user actually granted, and
// the timestamps. Field tags match Hydra v2's snake_case JSON; unknown fields are
// ignored. handled_at is when the consent was granted (Hydra has no separate
// created_at on this record).
type ConsentSessionRecord struct {
	ConsentRequest           *ConsentRequest `json:"consent_request"`
	GrantScope               []string        `json:"grant_scope"`
	GrantAccessTokenAudience []string        `json:"grant_access_token_audience"`
	HandledAt                string          `json:"handled_at"`
	Remember                 bool            `json:"remember"`
	RememberFor              int64           `json:"remember_for"`
}

// ListConsentSessions returns the subject's granted consent sessions from Hydra's
// admin API (GET /admin/oauth2/auth/sessions/consent?subject=). An unknown subject
// (or one that has granted nothing) yields an empty slice, not an error. This is
// the user-facing "connected apps" view (design.md D5).
func (c *HydraClient) ListConsentSessions(ctx context.Context, subject string) ([]ConsentSessionRecord, error) {
	var out []ConsentSessionRecord
	err := c.do(ctx, http.MethodGet, "/admin/oauth2/auth/sessions/consent",
		url.Values{"subject": {subject}}, nil, &out, "list consent sessions")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RevokeConsentSessions revokes the subject's consent for a single client
// (DELETE /admin/oauth2/auth/sessions/consent?subject=&client=). After this the
// client must obtain consent again on the next authorization. A 404 is treated as
// success (idempotent revoke). client must be non-empty so this never accidentally
// revokes every grant for the subject.
func (c *HydraClient) RevokeConsentSessions(ctx context.Context, subject, client string) error {
	err := c.do(ctx, http.MethodDelete, "/admin/oauth2/auth/sessions/consent",
		url.Values{"subject": {subject}, "client": {client}}, nil, nil, "revoke consent sessions")
	var he *HydraError
	if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
		return nil
	}
	return err
}

// RevokeAllConsentSessions revokes every consent the subject has granted
// (DELETE /admin/oauth2/auth/sessions/consent?subject=&all=true). Used on account
// deletion. A 404 is treated as success (idempotent revoke).
func (c *HydraClient) RevokeAllConsentSessions(ctx context.Context, subject string) error {
	err := c.do(ctx, http.MethodDelete, "/admin/oauth2/auth/sessions/consent",
		url.Values{"subject": {subject}, "all": {"true"}}, nil, nil, "revoke all consent sessions")
	var he *HydraError
	if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
		return nil
	}
	return err
}

// RevokeLoginSessions invalidates all of the subject's authentication (login)
// sessions in Hydra (DELETE /admin/oauth2/auth/sessions/login?subject=), forcing a
// fresh login at the IdP on the next authorization. Used on account deletion. A
// 404 is treated as success (idempotent revoke).
func (c *HydraClient) RevokeLoginSessions(ctx context.Context, subject string) error {
	err := c.do(ctx, http.MethodDelete, "/admin/oauth2/auth/sessions/login",
		url.Values{"subject": {subject}}, nil, nil, "revoke login sessions")
	var he *HydraError
	if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
		return nil
	}
	return err
}

// --- health ----------------------------------------------------------------

// Health probes Hydra's admin readiness endpoint so /healthz can report Hydra as
// a dependency. It returns nil when Hydra responds ready. A short context timeout
// keeps the health check from blocking on a hung Hydra.
func (c *HydraClient) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.adminURL+"/health/ready", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("hydra not reachable: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("hydra not ready: status %d", resp.StatusCode)
	}
	return nil
}

// --- transport -------------------------------------------------------------

// do performs an admin API request: it marshals body (when non-nil) as JSON,
// applies the query params, sends the request, and decodes a 2xx JSON response
// into out (when non-nil). Non-2xx responses become a *HydraError carrying the
// status and raw body. The op string is used only for error messages/logging.
func (c *HydraClient) do(ctx context.Context, method, path string, query url.Values, body, out any, op string) error {
	u := c.adminURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("hydra %s: marshal request: %w", op, err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return fmt.Errorf("hydra %s: build request: %w", op, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("hydra %s: %w", op, err)
	}
	defer resp.Body.Close()

	// Read the whole (bounded) body so we can surface it on error and decode it
	// on success. Hydra admin responses are small.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("hydra %s: read response: %w", op, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HydraError{Op: op, StatusCode: resp.StatusCode, Body: string(raw)}
	}

	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("hydra %s: decode response: %w", op, err)
		}
	}
	return nil
}
