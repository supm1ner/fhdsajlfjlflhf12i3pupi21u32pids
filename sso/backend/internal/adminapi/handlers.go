// Package adminapi implements cotton-id's admin client-registration endpoints,
// proxying Ory Hydra's admin API for OAuth2 client (relying-party) CRUD
// (build-contract §3, specs/oidc-provider "OAuth client registration"). The
// routes are protected by the X-Admin-Key middleware (constant-time compared,
// fail-closed) and are EXEMPT from CSRF (machine-to-machine).
//
// ============================================================================
// INTEGRATION CONTRACT
// ============================================================================
//
//	func Mount(r chi.Router, deps Deps)
//	    Registers, under the /api/v1 subrouter:
//	        POST   /admin/clients          -> register a client
//	        GET    /admin/clients          -> list clients
//	        DELETE /admin/clients/{id}     -> delete a client
//	    wrapped in RequireAdminKey(deps.AdminAPIKey).
//
// The Hydra HTTP transport is the hand-written oidc.HydraClient (net/http, no
// extra deps per build-contract §2), reused here. Responses use the httpx.Write*
// helpers for uniform problem+json.
// ============================================================================
package adminapi

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"cotton-id/internal/audit"
	"cotton-id/internal/httpx"
	"cotton-id/internal/observability"
	"cotton-id/internal/oidc"
)

// Deps are the admin API dependencies, populated by main.go.
type Deps struct {
	Logger        *slog.Logger
	Metrics       *observability.Metrics
	HydraAdminURL string
	AdminAPIKey   string
	// Audit is an OPTIONAL audit-log sink. Nil is a safe no-op (tests omit it);
	// main.go wires the real writer. Used to record client create/delete alongside
	// the existing structured slog lines. These routes are machine-to-machine
	// (X-Admin-Key), so entries carry the "admin-key" actor label and no actor id.
	Audit *audit.Writer
}

// Mount registers the machine client routes under "/admin" behind the admin-key
// middleware. Kept for the package's own tests; main.go uses MountClients so the
// console and these machine routes can share one "/admin" subtree.
func Mount(r chi.Router, deps Deps) {
	r.Route("/admin", func(r chi.Router) { mountClients(r, deps) })
}

// MountClients registers the machine client routes on r WITHOUT the "/admin"
// prefix (X-Admin-Key, CSRF-exempt), for main.go's combined "/admin" subtree.
func MountClients(r chi.Router, deps Deps) {
	mountClients(r, deps)
}

func mountClients(r chi.Router, deps Deps) {
	h := &handlers{deps: deps, hydra: oidc.NewHydraClient(deps.HydraAdminURL)}
	r.Group(func(r chi.Router) {
		r.Use(RequireAdminKey(deps.AdminAPIKey, deps.Logger))
		r.Post("/clients", h.RegisterClient)
		r.Get("/clients", h.ListClients)
		r.Delete("/clients/{id}", h.DeleteClient)
	})
}

// handlers holds the admin API dependencies plus the Hydra admin client.
type handlers struct {
	deps  Deps
	hydra *oidc.HydraClient
}

// log returns the request-scoped logger.
func (h *handlers) log(r *http.Request) *slog.Logger {
	return observability.LoggerFrom(r.Context(), h.deps.Logger)
}

// RegisterClient registers a new OAuth2 client (relying party) in Hydra.
//
// @Summary     Register an OAuth2 client
// @Description Creates a relying-party client in Hydra. Returns the generated clientId, plus a clientSecret for confidential clients (shown once). Requires the X-Admin-Key header.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       body body registerClientRequest true "Client to register"
// @Success     201 {object} registerClientResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    AdminKey
// @Router      /admin/clients [post]
func (h *handlers) RegisterClient(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)

	var req registerClientRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// --- validation ---
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "name", "name is required")
		return
	}
	if len(req.RedirectURIs) == 0 {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "redirectUris", "at least one redirect URI is required")
		return
	}
	if !validRedirectURIs(req.RedirectURIs) {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "redirectUris", "redirect URIs must be absolute http(s) URLs without a fragment")
		return
	}
	if req.ClientType == "" {
		req.ClientType = ClientTypePublic
	}
	if req.ClientType != ClientTypePublic && req.ClientType != ClientTypeConfidential {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "clientType", "clientType must be 'public' or 'confidential'")
		return
	}
	// Sensible defaults so a minimal request still yields a working auth-code client.
	if len(req.GrantTypes) == 0 {
		req.GrantTypes = []string{"authorization_code", "refresh_token"}
	}
	if len(req.ResponseTypes) == 0 {
		req.ResponseTypes = []string{"code"}
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail}
	}

	created, err := h.hydra.CreateClient(r.Context(), req.toHydraClient())
	if err != nil {
		var he *oidc.HydraError
		if errors.As(err, &he) && he.StatusCode == http.StatusConflict {
			httpx.WriteProblem(w, r, http.StatusConflict, "a client with that id already exists")
			return
		}
		log.Error("hydra create client failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not register client")
		return
	}

	log.Info("oauth client created",
		slog.String("client_id", created.ClientID),
		slog.String("client_name", req.Name),
		slog.String("client_type", req.ClientType),
		slog.String("ip", httpx.ClientIP(r)),
	)
	e := audit.FromRequest(r, audit.ActionClientCreate)
	e.ActorLabel = "admin-key"
	_ = h.deps.Audit.Append(r.Context(), e.
		WithTarget(audit.TargetClient, created.ClientID).
		WithMetadata(map[string]any{"clientName": req.Name, "clientType": req.ClientType}))

	httpx.WriteJSON(w, http.StatusCreated, registerClientResponse{
		ClientID:     created.ClientID,
		ClientSecret: created.ClientSecret, // omitempty: only present for confidential
	})
}

// ListClients returns the registered OAuth2 clients (no secrets).
//
// @Summary     List OAuth2 clients
// @Description Lists the registered relying-party clients. Secrets are never returned. Requires the X-Admin-Key header.
// @Tags        admin
// @Produce     json
// @Success     200 {object} listClientsResponse
// @Failure     401 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    AdminKey
// @Router      /admin/clients [get]
func (h *handlers) ListClients(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)

	clients, err := h.hydra.ListClients(r.Context())
	if err != nil {
		log.Error("hydra list clients failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not list clients")
		return
	}

	out := make([]clientSummary, 0, len(clients))
	for _, c := range clients {
		out = append(out, summarize(c))
	}
	httpx.WriteJSON(w, http.StatusOK, listClientsResponse{Clients: out})
}

// DeleteClient removes a registered OAuth2 client.
//
// @Summary     Delete an OAuth2 client
// @Description Removes a relying-party client from Hydra. Subsequent authorization requests for that clientId are rejected. Idempotent. Requires the X-Admin-Key header.
// @Tags        admin
// @Param       id path string true "Client id"
// @Success     204 "No Content"
// @Failure     401 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    AdminKey
// @Router      /admin/clients/{id} [delete]
func (h *handlers) DeleteClient(w http.ResponseWriter, r *http.Request) {
	log := h.log(r)
	id := chi.URLParam(r, "id")
	if id == "" {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "client id is required")
		return
	}

	if err := h.hydra.DeleteClient(r.Context(), id); err != nil {
		log.Error("hydra delete client failed", slog.Any("error", err), slog.String("client_id", id))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not delete client")
		return
	}

	log.Info("oauth client deleted",
		slog.String("client_id", id),
		slog.String("ip", httpx.ClientIP(r)),
	)
	e := audit.FromRequest(r, audit.ActionClientDelete)
	e.ActorLabel = "admin-key"
	_ = h.deps.Audit.Append(r.Context(), e.WithTarget(audit.TargetClient, id))
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// validRedirectURIs returns true only when every entry is an absolute http(s)
// URL with no fragment (a fragment in a redirect URI is forbidden by OAuth2).
func validRedirectURIs(uris []string) bool {
	for _, raw := range uris {
		if !validRedirectURI(raw) {
			return false
		}
	}
	return true
}

// RequireAdminKey returns middleware enforcing the X-Admin-Key header against the
// configured admin key, compared in constant time. A missing/blank configured
// key denies all access (fail-closed). Part of the substrate contract.
func RequireAdminKey(adminKey string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := r.Header.Get("X-Admin-Key")
			if adminKey == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(adminKey)) != 1 {
				if log != nil {
					observability.LoggerFrom(r.Context(), log).Warn("admin auth failed",
						slog.String("ip", httpx.ClientIP(r)),
						slog.String("path", r.URL.Path),
					)
				}
				httpx.WriteProblem(w, r, http.StatusUnauthorized, "admin authorization required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
