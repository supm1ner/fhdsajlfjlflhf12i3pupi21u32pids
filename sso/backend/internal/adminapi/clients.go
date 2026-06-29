package adminapi

import (
	"net/url"
	"strings"

	"cotton-id/internal/oidc"
)

// clients.go — the admin client-registration DTOs and the translation between
// cotton-id's camelCase admin API shapes and Hydra's OAuth2 client model. The
// actual Hydra HTTP transport is the hand-written oidc.HydraClient (net/http,
// no extra deps per build-contract §2), reused here so there is a single Hydra
// admin client in the codebase.

// Client types accepted by the admin API. A "public" client (SPA / native app)
// uses PKCE and receives no secret; a "confidential" client (server-side) is
// issued a client_secret.
const (
	ClientTypePublic       = "public"
	ClientTypeConfidential = "confidential"
)

// RegisterClientRequest is the POST /api/v1/admin/clients body (camelCase per
// contract §3). Exported so the role-gated console (internal/admin) reuses the
// same request shape + validation + Hydra mapping rather than drifting.
type RegisterClientRequest struct {
	Name          string   `json:"name" example:"Demo App"`
	RedirectURIs  []string `json:"redirectUris" example:"http://localhost:5173/callback"`
	Scopes        []string `json:"scopes" example:"openid,profile,email"`
	GrantTypes    []string `json:"grantTypes" example:"authorization_code,refresh_token"`
	ResponseTypes []string `json:"responseTypes" example:"code"`
	ClientType    string   `json:"clientType" example:"public"`
}

// registerClientRequest is the unexported alias the existing machine handlers use.
type registerClientRequest = RegisterClientRequest

// registerClientResponse is the 201 response: the generated client id, plus the
// secret for confidential clients only (it is shown exactly once).
type registerClientResponse struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

// ClientSummary is the client-safe projection returned by the list endpoint. The
// secret is NEVER included in a list response. Exported for reuse by the console.
type ClientSummary struct {
	ClientID      string   `json:"clientId"`
	Name          string   `json:"name"`
	RedirectURIs  []string `json:"redirectUris"`
	Scopes        []string `json:"scopes"`
	GrantTypes    []string `json:"grantTypes"`
	ResponseTypes []string `json:"responseTypes"`
	ClientType    string   `json:"clientType"`
	CreatedAt     string   `json:"createdAt,omitempty"`
}

// clientSummary is the unexported alias the existing machine handlers use.
type clientSummary = ClientSummary

// listClientsResponse wraps the clients in a {clients:[...]} envelope.
type listClientsResponse struct {
	Clients []clientSummary `json:"clients"`
}

// ToHydraClient maps a validated admin request to Hydra's OAuth2 client model.
// Public clients are configured for PKCE with token_endpoint_auth_method=none
// (no secret); confidential clients use client_secret_basic so Hydra issues a
// secret. Exported so the console builds the SAME Hydra representation.
func (req *RegisterClientRequest) ToHydraClient() oidc.OAuth2Client {
	authMethod := "client_secret_basic"
	if req.ClientType == ClientTypePublic {
		authMethod = "none"
	}
	return oidc.OAuth2Client{
		ClientName:              req.Name,
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              req.GrantTypes,
		ResponseTypes:           req.ResponseTypes,
		Scope:                   strings.Join(req.Scopes, " "),
		TokenEndpointAuthMethod: authMethod,
	}
}

// toHydraClient is the unexported method the existing machine handlers call.
func (req *registerClientRequest) toHydraClient() oidc.OAuth2Client { return req.ToHydraClient() }

// ValidRedirectURI returns true when raw is an absolute http(s) URL with a host
// and no fragment (a fragment in a redirect URI is forbidden by OAuth2 / OIDC).
// Exported for reuse by the console's edit/create validation.
func ValidRedirectURI(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}
	if u.Fragment != "" {
		return false
	}
	return true
}

// validRedirectURI is the unexported alias the existing machine handlers/tests use.
func validRedirectURI(raw string) bool { return ValidRedirectURI(raw) }

// ValidRedirectURIs returns true only when every entry is an absolute http(s)
// URL with no fragment. Exported for reuse by the console's create/edit handlers.
func ValidRedirectURIs(uris []string) bool {
	for _, raw := range uris {
		if !ValidRedirectURI(raw) {
			return false
		}
	}
	return true
}

// Summarize maps a Hydra OAuth2 client to the client-safe list projection
// (id, name, redirect URIs, scopes, grant/response types, and the derived
// public/confidential type). The secret is never part of this projection.
// Exported so the console returns the SAME shape as the machine list route.
func Summarize(c oidc.OAuth2Client) ClientSummary {
	clientType := ClientTypeConfidential
	if c.TokenEndpointAuthMethod == "none" {
		clientType = ClientTypePublic
	}
	var scopes []string
	if s := strings.TrimSpace(c.Scope); s != "" {
		scopes = strings.Fields(s)
	}
	return ClientSummary{
		ClientID:      c.ClientID,
		Name:          c.ClientName,
		RedirectURIs:  c.RedirectURIs,
		Scopes:        scopes,
		GrantTypes:    c.GrantTypes,
		ResponseTypes: c.ResponseTypes,
		ClientType:    clientType,
		CreatedAt:     c.CreatedAt,
	}
}

// summarize is the unexported alias the existing machine handlers use.
func summarize(c oidc.OAuth2Client) clientSummary { return Summarize(c) }
