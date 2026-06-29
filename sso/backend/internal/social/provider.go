// Package social implements cotton-id's social-login (OAuth2/OIDC) connector:
// a provider-agnostic abstraction with concrete adapters for Google (OIDC),
// GitHub, VK ID, and Yandex, the signed-cookie state/PKCE handling, the
// verified-email account resolver (the account-takeover guard), and the three
// browser-facing HTTP endpoints (providers list, start, callback). It establishes
// a normal cotton-id session via internal/auth and, when a Hydra login_challenge
// is in flight, continues the OIDC handshake via internal/oidc.
//
// Provider endpoints were verified against each provider's current official
// documentation (see design.md / the change report). Notable choices:
//   - Google: full OIDC + PKCE (S256); userinfo carries email_verified.
//   - GitHub: OAuth2, state only (no PKCE); email comes from /user/emails, where
//     the primary AND verified address is selected.
//   - VK: the current VK ID (id.vk.ru) OAuth 2.1 + PKCE flow; the token exchange
//     omits client_secret (PKCE replaces it) and the callback carries device_id.
//   - Yandex: OAuth2 + PKCE (S256); userinfo at login.yandex.ru/info.
package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Provider identifiers — these match the {provider} path segment and the env-var
// suffixes (SOCIAL_<P>_CLIENT_ID / _SECRET).
const (
	ProviderGoogle = "google"
	ProviderGitHub = "github"
	ProviderVK     = "vk"
	ProviderYandex = "yandex"
)

// Identity is the normalized profile a provider adapter extracts from a token +
// userinfo. Subject is the provider's stable, unique user id (never the email).
// EmailVerified gates account linking (design D3): an unverified email is never
// auto-linked to an existing account.
type Identity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Username      string
	AvatarURL     string
}

// Provider describes one social provider's OAuth/OIDC integration. The handlers
// are generic over this; per-provider quirks live in the closures below and in
// mapUserInfo. A Provider value is immutable after construction.
type Provider struct {
	// ID is the provider key (google|github|vk|yandex).
	ID string
	// DisplayName is the human label shown in the providers list / UI.
	DisplayName string

	authURL     string
	tokenURL    string
	userInfoURL string
	scopes      []string
	// usesPKCE selects the S256 PKCE extension (Google, VK, Yandex). GitHub OAuth
	// apps rely on state only.
	usesPKCE bool

	// mapUserInfo turns a token response + an HTTP client into a normalized
	// Identity. It owns all provider-specific calls (extra userinfo / emails
	// requests) and JSON shapes. It receives the *Provider it is invoked on so it
	// reads endpoint URLs from the (possibly test-repointed) value rather than a
	// captured pointer.
	mapUserInfo func(ctx context.Context, p *Provider, hc *http.Client, tok *tokenResponse) (*Identity, error)
}

// tokenResponse is the (superset) token-endpoint response. Providers populate
// different subsets; adapters read what they need. VK ID additionally returns the
// user_id/email in this response for some flows, but cotton-id fetches the
// profile from user_info for consistency.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	IDToken     string `json:"id_token"`
	Scope       string `json:"scope"`
	// VK ID / legacy VK return these directly; kept for completeness.
	Email  string          `json:"email"`
	UserID json.RawMessage `json:"user_id"`
	// Raw retains the full decoded body so an adapter can read provider-specific
	// fields without widening this struct.
	Raw map[string]json.RawMessage `json:"-"`
}

// AuthCodeURL builds the provider's authorization-endpoint URL for the start
// redirect. state is the anti-CSRF token; codeChallenge is the S256 PKCE
// challenge (empty for providers that don't use PKCE).
func (p *Provider) AuthCodeURL(clientID, redirectURI, state, codeChallenge string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	if len(p.scopes) > 0 {
		q.Set("scope", strings.Join(p.scopes, p.scopeSeparator()))
	}
	if p.usesPKCE && codeChallenge != "" {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
	}
	return p.authURL + "?" + q.Encode()
}

// scopeSeparator returns the scope delimiter for the provider. Yandex uses a
// comma-separated scope list; the OAuth2/OIDC default is a space.
func (p *Provider) scopeSeparator() string {
	if p.ID == ProviderYandex {
		return ","
	}
	return " "
}

// UsesPKCE reports whether the provider participates in the S256 PKCE extension.
func (p *Provider) UsesPKCE() bool { return p.usesPKCE }

// exchangeParams carries the per-request values the token exchange needs beyond
// the static provider config.
type exchangeParams struct {
	clientID     string
	clientSecret string
	redirectURI  string
	code         string
	codeVerifier string
	// extra carries provider-specific token-request params (e.g. VK's device_id
	// and state echo). Keys are sent as-is in the form body.
	extra url.Values
}

// Exchange performs the authorization-code → token exchange and then maps the
// userinfo into an Identity. It is the single entry the callback handler uses
// per provider.
func (p *Provider) Exchange(ctx context.Context, hc *http.Client, ep exchangeParams) (*Identity, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", ep.code)
	form.Set("redirect_uri", ep.redirectURI)
	form.Set("client_id", ep.clientID)
	if p.usesPKCE && ep.codeVerifier != "" {
		form.Set("code_verifier", ep.codeVerifier)
	}
	// VK ID's PKCE flow authenticates the client with the verifier and omits the
	// secret. Every other adapter sends the confidential client secret.
	if p.ID != ProviderVK {
		form.Set("client_secret", ep.clientSecret)
	}
	for k, vs := range ep.extra {
		for _, v := range vs {
			form.Set(k, v)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%s: build token request: %w", p.ID, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// GitHub returns form-encoded by default; ask for JSON.
	req.Header.Set("Accept", "application/json")

	tok, err := p.doToken(req, hc)
	if err != nil {
		return nil, err
	}
	return p.mapUserInfo(ctx, p, hc, tok)
}

// doToken sends the token request and decodes the JSON token response. A non-2xx
// status (or an OAuth error field in the body) becomes an error.
func (p *Provider) doToken(req *http.Request, hc *http.Client) (*tokenResponse, error) {
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: token request: %w", p.ID, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: read token response: %w", p.ID, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: token endpoint status %d: %s", p.ID, resp.StatusCode, truncateForLog(raw))
	}

	var tok tokenResponse
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, fmt.Errorf("%s: decode token response: %w", p.ID, err)
	}
	_ = json.Unmarshal(raw, &tok.Raw)
	// Surface in-body OAuth errors (200 with {"error":...} happens with GitHub).
	if errField, ok := tok.Raw["error"]; ok {
		return nil, fmt.Errorf("%s: token endpoint error: %s", p.ID, truncateForLog(errField))
	}
	if tok.AccessToken == "" && tok.IDToken == "" {
		return nil, fmt.Errorf("%s: token response carried no access_token", p.ID)
	}
	return &tok, nil
}

// getJSON performs an authenticated GET and decodes the JSON body into dst. The
// authHeader (full header value, e.g. "Bearer x" or "OAuth x") is applied when
// non-empty. apiHeaders sets any extra request headers (e.g. GitHub's API
// version / Accept).
func getJSON(ctx context.Context, hc *http.Client, urlStr, authHeader string, apiHeaders map[string]string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range apiHeaders {
		req.Header.Set(k, v)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("userinfo status %d: %s", resp.StatusCode, truncateForLog(raw))
	}
	return json.Unmarshal(raw, dst)
}

// truncateForLog renders a byte slice for an error message, bounded so a large
// provider error body can't bloat logs.
func truncateForLog(b []byte) string {
	const max = 256
	s := strings.TrimSpace(string(b))
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

// ----------------------------------------------------------------------------
// Provider adapters
// ----------------------------------------------------------------------------

// Google — full OIDC with PKCE (S256). The userinfo endpoint returns the OIDC
// standard claims including email_verified.
func googleProvider() *Provider {
	p := &Provider{
		ID:          ProviderGoogle,
		DisplayName: "Google",
		authURL:     "https://accounts.google.com/o/oauth2/v2/auth",
		tokenURL:    "https://oauth2.googleapis.com/token",
		userInfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
		scopes:      []string{"openid", "email", "profile"},
		usesPKCE:    true,
	}
	p.mapUserInfo = func(ctx context.Context, p *Provider, hc *http.Client, tok *tokenResponse) (*Identity, error) {
		var ui struct {
			Sub           string `json:"sub"`
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Name          string `json:"name"`
			GivenName     string `json:"given_name"`
			Picture       string `json:"picture"`
		}
		if err := getJSON(ctx, hc, p.userInfoURL, "Bearer "+tok.AccessToken, nil, &ui); err != nil {
			return nil, fmt.Errorf("google userinfo: %w", err)
		}
		if ui.Sub == "" {
			return nil, fmt.Errorf("google userinfo: missing sub")
		}
		return &Identity{
			Subject:       ui.Sub,
			Email:         ui.Email,
			EmailVerified: ui.EmailVerified,
			Name:          ui.Name,
			Username:      ui.GivenName,
			AvatarURL:     ui.Picture,
		}, nil
	}
	return p
}

// GitHub — OAuth2, state only (no PKCE). The profile comes from /user; the email
// is selected from /user/emails as the primary AND verified address (account-
// takeover guard: an unverified GitHub email must never be treated as verified).
func githubProvider() *Provider {
	return githubProviderAt("https://api.github.com")
}

// githubProviderAt builds the GitHub adapter against the given API base URL. The
// base is a seam so tests can repoint /user and /user/emails at an httptest
// server; production uses https://api.github.com.
func githubProviderAt(apiBase string) *Provider {
	const apiVer = "2022-11-28"
	userURL := apiBase + "/user"
	emailsURL := apiBase + "/user/emails"
	p := &Provider{
		ID:          ProviderGitHub,
		DisplayName: "GitHub",
		authURL:     "https://github.com/login/oauth/authorize",
		tokenURL:    "https://github.com/login/oauth/access_token",
		userInfoURL: userURL,
		scopes:      []string{"read:user", "user:email"},
		usesPKCE:    false,
	}
	ghHeaders := map[string]string{
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": apiVer,
	}
	p.mapUserInfo = func(ctx context.Context, _ *Provider, hc *http.Client, tok *tokenResponse) (*Identity, error) {
		var u struct {
			ID        int64  `json:"id"`
			Login     string `json:"login"`
			Name      string `json:"name"`
			Email     string `json:"email"`
			AvatarURL string `json:"avatar_url"`
		}
		if err := getJSON(ctx, hc, userURL, "Bearer "+tok.AccessToken, ghHeaders, &u); err != nil {
			return nil, fmt.Errorf("github user: %w", err)
		}
		if u.ID == 0 {
			return nil, fmt.Errorf("github user: missing id")
		}

		// Select the primary, verified email. GitHub's /user email may be null
		// (private) or unverified; the /user/emails list is authoritative.
		email, verified := selectGitHubEmail(ctx, hc, emailsURL, "Bearer "+tok.AccessToken, ghHeaders)
		if email == "" && u.Email != "" {
			// Fall back to the profile email, but only as UNVERIFIED — we cannot
			// assert verification for it.
			email, verified = u.Email, false
		}

		return &Identity{
			Subject:       strconv.FormatInt(u.ID, 10),
			Email:         email,
			EmailVerified: verified,
			Name:          u.Name,
			Username:      u.Login,
			AvatarURL:     u.AvatarURL,
		}, nil
	}
	return p
}

// githubEmail is one entry of GitHub's /user/emails list.
type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// selectGitHubEmail picks the primary verified email; failing that, the first
// verified email. It returns ("", false) when no verified email is available, so
// the resolver never links on an unverified GitHub address.
func selectGitHubEmail(ctx context.Context, hc *http.Client, emailsURL, authHeader string, headers map[string]string) (string, bool) {
	var emails []githubEmail
	if err := getJSON(ctx, hc, emailsURL, authHeader, headers, &emails); err != nil {
		// If the emails call fails we degrade to "no verified email" so the
		// resolver never links on an unverified address.
		return "", false
	}
	return pickGitHubEmail(emails)
}

// pickGitHubEmail is the pure selection logic (unit-tested): primary+verified
// first, else the first verified address, else empty.
func pickGitHubEmail(emails []githubEmail) (string, bool) {
	var firstVerified string
	for _, e := range emails {
		if !e.Verified {
			continue
		}
		if e.Primary {
			return e.Email, true
		}
		if firstVerified == "" {
			firstVerified = e.Email
		}
	}
	if firstVerified != "" {
		return firstVerified, true
	}
	return "", false
}

// VK — the current VK ID (id.vk.ru) OAuth 2.1 + PKCE (S256) flow. The token
// exchange omits client_secret (PKCE replaces it) and requires the device_id the
// provider returns on the callback (carried via exchangeParams.extra). The
// profile (including email) comes from oauth2/user_info under a "user" object.
func vkProvider() *Provider {
	const userInfoURL = "https://id.vk.ru/oauth2/user_info"
	p := &Provider{
		ID:          ProviderVK,
		DisplayName: "VK",
		authURL:     "https://id.vk.ru/authorize",
		tokenURL:    "https://id.vk.ru/oauth2/auth",
		userInfoURL: userInfoURL,
		scopes:      []string{"email"},
		usesPKCE:    true,
	}
	p.mapUserInfo = func(ctx context.Context, p *Provider, hc *http.Client, tok *tokenResponse) (*Identity, error) {
		// VK ID's user_info is a POST with the access token in the form body, but
		// it also honors a Bearer GET; cotton-id uses the documented POST form.
		form := url.Values{}
		form.Set("access_token", tok.AccessToken)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.userInfoURL, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, fmt.Errorf("vk user_info: build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		resp, err := hc.Do(req)
		if err != nil {
			return nil, fmt.Errorf("vk user_info: %w", err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return nil, fmt.Errorf("vk user_info: read: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("vk user_info: status %d: %s", resp.StatusCode, truncateForLog(raw))
		}
		return mapVKUserInfo(raw, tok.Email)
	}
	return p
}

// mapVKUserInfo parses VK ID's user_info body (a {"user": {...}} envelope) into
// an Identity. tokenEmail is the email the token response carried (a fallback if
// user_info omits it). VK ID returns NO email-verification signal, so the email
// is treated as UNVERIFIED (see EmailVerified below).
func mapVKUserInfo(raw []byte, tokenEmail string) (*Identity, error) {
	var env struct {
		User struct {
			UserID    json.RawMessage `json:"user_id"`
			FirstName string          `json:"first_name"`
			LastName  string          `json:"last_name"`
			Email     string          `json:"email"`
			Avatar    string          `json:"avatar"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("vk user_info: decode: %w", err)
	}
	subject := jsonNumberOrString(env.User.UserID)
	if subject == "" {
		return nil, fmt.Errorf("vk user_info: missing user_id")
	}
	email := env.User.Email
	if email == "" {
		email = tokenEmail
	}
	name := strings.TrimSpace(env.User.FirstName + " " + env.User.LastName)
	return &Identity{
		Subject: subject,
		Email:   email,
		// VK ID's user_info returns NO email-verification flag, and VK accounts are
		// phone-first (the address may be unconfirmed). Treat a VK email as
		// UNVERIFIED so it can never auto-link to a pre-existing cotton-id account
		// (design D3 takeover guard) — a VK user gets their own account; an explicit
		// authenticated link can be offered later via account settings.
		EmailVerified: false,
		Name:          name,
		Username:      "",
		AvatarURL:     env.User.Avatar,
	}, nil
}

// Yandex — OAuth2 + PKCE (S256). Userinfo at login.yandex.ru/info with an
// "Authorization: OAuth <token>" header.
func yandexProvider() *Provider {
	const userInfoURL = "https://login.yandex.ru/info?format=json"
	p := &Provider{
		ID:          ProviderYandex,
		DisplayName: "Yandex",
		authURL:     "https://oauth.yandex.ru/authorize",
		tokenURL:    "https://oauth.yandex.ru/token",
		userInfoURL: userInfoURL,
		scopes:      []string{"login:email", "login:info", "login:avatar"},
		usesPKCE:    true,
	}
	p.mapUserInfo = func(ctx context.Context, p *Provider, hc *http.Client, tok *tokenResponse) (*Identity, error) {
		var ui struct {
			ID            string `json:"id"`
			Login         string `json:"login"`
			DefaultEmail  string `json:"default_email"`
			RealName      string `json:"real_name"`
			DisplayName   string `json:"display_name"`
			DefaultAvatar string `json:"default_avatar_id"`
			IsAvatarEmpty bool   `json:"is_avatar_empty"`
		}
		if err := getJSON(ctx, hc, p.userInfoURL, "OAuth "+tok.AccessToken, nil, &ui); err != nil {
			return nil, fmt.Errorf("yandex userinfo: %w", err)
		}
		if ui.ID == "" {
			return nil, fmt.Errorf("yandex userinfo: missing id")
		}
		name := ui.RealName
		if name == "" {
			name = ui.DisplayName
		}
		avatar := ""
		if !ui.IsAvatarEmpty && ui.DefaultAvatar != "" {
			avatar = "https://avatars.yandex.net/get-yapic/" + ui.DefaultAvatar + "/islands-200"
		}
		return &Identity{
			Subject: ui.ID,
			Email:   ui.DefaultEmail,
			// Yandex only returns default_email for confirmed account addresses.
			EmailVerified: ui.DefaultEmail != "",
			Name:          name,
			Username:      ui.Login,
			AvatarURL:     avatar,
		}, nil
	}
	return p
}

// jsonNumberOrString renders a JSON value that may be a number or a quoted string
// (VK has returned user_id as both across API versions) as a plain string.
func jsonNumberOrString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return strings.Trim(string(raw), `"`)
}
