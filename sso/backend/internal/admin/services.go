package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"cotton-id/internal/adminapi"
	"cotton-id/internal/audit"
	"cotton-id/internal/httpx"
	"cotton-id/internal/oidc"
)

// services.go — the role-gated console handlers for the Services tab: CRUD over
// OAuth relying-party clients plus a best-effort per-client consent count/revoke.
//
// These live under /api/v1/admin/services (design D1), DISTINCT from the machine
// X-Admin-Key /admin/clients route (internal/adminapi). They REUSE adminapi's
// request DTO, redirect-URI validation, public/confidential Hydra mapping, and
// the client-safe Summarize projection so the two surfaces never drift. The
// secret is shown exactly ONCE on create (confidential clients) and never
// re-served on a read. Every mutation is audited with the acting admin
// (auth.UserFromContext) + the target client id.

// defaultConsentScanLimit bounds the per-client consent subject scan when
// Deps.ConsentScanLimit is unset. Hydra v2.2.0 has no per-client consent query,
// so the count/revoke iterate the IdP's subjects (design D3); this cap keeps the
// console route bounded. Beyond the cap the result is reported complete=false.
const defaultConsentScanLimit = 500

// createServiceResponse is the 201 body: the generated clientId plus the secret
// for confidential clients ONLY, shown exactly once (omitempty → absent for
// public clients and never present on any later read).
type createServiceResponse struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret,omitempty"`
}

// servicesListResponse wraps the client summaries in a {services:[...]} envelope.
type servicesListResponse struct {
	Services []adminapi.ClientSummary `json:"services"`
}

// serviceDetailResponse is GET /admin/services/{id}: the client-safe projection
// (no secret). Identical shape to a list row, wrapped in {service}.
type serviceDetailResponse struct {
	Service adminapi.ClientSummary `json:"service"`
}

// updateServiceRequest is the PATCH /admin/services/{id} body. Every field is a
// pointer so an omitted field means "leave unchanged"; a present field replaces
// that attribute. clientType, if present, MUST match the client's existing type
// (a silent public↔confidential flip is rejected — design "Risks", D2).
type updateServiceRequest struct {
	Name          *string   `json:"name,omitempty" example:"Renamed App"`
	RedirectURIs  *[]string `json:"redirectUris,omitempty"`
	Scopes        *[]string `json:"scopes,omitempty"`
	GrantTypes    *[]string `json:"grantTypes,omitempty"`
	ResponseTypes *[]string `json:"responseTypes,omitempty"`
	ClientType    *string   `json:"clientType,omitempty" example:"public"`
}

// serviceConsentsResponse is the best-effort per-client consent usage. count is
// the number of subjects (users) found with an active grant for the client among
// those scanned; complete is false when the IdP has more subjects than the scan
// cap, in which case count is a lower bound (design D3).
type serviceConsentsResponse struct {
	ClientID string `json:"clientId"`
	Count    int    `json:"count"`
	Complete bool   `json:"complete"`
}

// revokeServiceConsentsResponse reports how many subjects' grants were revoked
// for the client (best-effort across the scanned subjects).
type revokeServiceConsentsResponse struct {
	ClientID string `json:"clientId"`
	Revoked  int    `json:"revoked"`
	Complete bool   `json:"complete"`
}

// consentScanLimit returns the configured per-client consent scan cap or the
// default when unset.
func (h *Handlers) consentScanLimit() int {
	if h.deps.ConsentScanLimit > 0 {
		return h.deps.ConsentScanLimit
	}
	return defaultConsentScanLimit
}

// ListServices lists the registered OAuth relying-party clients (no secrets).
//
// @Summary     List services (OAuth clients)
// @Description Lists the registered relying-party clients with id, name, type (public/confidential), redirect URIs, scopes, and grant/response types. Secrets are never returned. Requires an admin/owner session.
// @Tags        admin-console
// @Produce     json
// @Success     200 {object} servicesListResponse
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/services [get]
func (h *Handlers) ListServices(w http.ResponseWriter, r *http.Request) {
	clients, err := h.deps.Clients.ListClients(r.Context())
	if err != nil {
		h.log(r).Error("admin list services failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not list services")
		return
	}
	out := make([]adminapi.ClientSummary, 0, len(clients))
	for _, c := range clients {
		out = append(out, adminapi.Summarize(c))
	}
	httpx.WriteJSON(w, http.StatusOK, servicesListResponse{Services: out})
}

// CreateService registers a new OAuth relying-party client from the console.
//
// @Summary     Create a service (OAuth client)
// @Description Registers a relying-party client. Returns the generated clientId, plus a clientSecret for confidential clients shown EXACTLY ONCE (never re-served). Invalid redirect URIs (non-absolute or containing a fragment) are rejected. Audited. Requires an admin/owner session.
// @Tags        admin-console
// @Accept      json
// @Produce     json
// @Param       body body adminapi.RegisterClientRequest true "Service to create"
// @Success     201 {object} createServiceResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     409 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/services [post]
func (h *Handlers) CreateService(w http.ResponseWriter, r *http.Request) {
	act, ok := actor(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}

	var req adminapi.RegisterClientRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "name", "name is required")
		return
	}
	if len(req.RedirectURIs) == 0 {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "redirectUris", "at least one redirect URI is required")
		return
	}
	if !adminapi.ValidRedirectURIs(req.RedirectURIs) {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "redirectUris", "redirect URIs must be absolute http(s) URLs without a fragment")
		return
	}
	if req.ClientType == "" {
		req.ClientType = adminapi.ClientTypePublic
	}
	if req.ClientType != adminapi.ClientTypePublic && req.ClientType != adminapi.ClientTypeConfidential {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "clientType", "clientType must be 'public' or 'confidential'")
		return
	}
	// Sensible defaults so a minimal request yields a working auth-code client.
	if len(req.GrantTypes) == 0 {
		req.GrantTypes = []string{"authorization_code", "refresh_token"}
	}
	if len(req.ResponseTypes) == 0 {
		req.ResponseTypes = []string{"code"}
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail}
	}

	created, err := h.deps.Clients.CreateClient(r.Context(), req.ToHydraClient())
	if err != nil {
		var he *oidc.HydraError
		if errors.As(err, &he) && he.StatusCode == http.StatusConflict {
			httpx.WriteProblem(w, r, http.StatusConflict, "a client with that id already exists")
			return
		}
		h.log(r).Error("admin create service failed", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not create service")
		return
	}

	h.log(r).Info("admin service created",
		slog.String("actor_id", act.ID.String()),
		slog.String("client_id", created.ClientID),
		slog.String("client_name", req.Name),
		slog.String("client_type", req.ClientType),
	)
	_ = h.deps.Audit.Append(r.Context(), audit.FromRequest(r, ActionServiceCreate).
		WithActor(act.ID, act.Username).
		WithTarget(audit.TargetClient, created.ClientID).
		WithMetadata(map[string]any{"clientName": req.Name, "clientType": req.ClientType}))

	// Secret returned exactly once (omitempty → absent for public clients).
	httpx.WriteJSON(w, http.StatusCreated, createServiceResponse{
		ClientID:     created.ClientID,
		ClientSecret: created.ClientSecret,
	})
}

// ServiceDetail returns one client's client-safe projection (no secret).
//
// @Summary     Service detail
// @Description Returns a single relying-party client's id, name, type, redirect URIs, scopes, and grant/response types. The secret is never returned (shown only once on create). Requires an admin/owner session.
// @Tags        admin-console
// @Produce     json
// @Param       id path string true "client id"
// @Success     200 {object} serviceDetailResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/services/{id} [get]
func (h *Handlers) ServiceDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "client id is required")
		return
	}
	client, err := h.deps.Clients.GetClient(r.Context(), id)
	if err != nil {
		h.writeClientError(w, r, err, "could not load service")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, serviceDetailResponse{Service: adminapi.Summarize(*client)})
}

// UpdateService edits a client's name, redirect URIs, scopes, and grant/response
// types. The client type cannot be flipped (public↔confidential) silently.
//
// @Summary     Edit a service (OAuth client)
// @Description Updates a relying-party client's name, redirect URIs, scopes, and/or grant/response types. Omitted fields are left unchanged. Invalid redirect URIs are rejected. A public↔confidential type change is rejected (clientType, if sent, must match the existing type). Audited. Requires an admin/owner session.
// @Tags        admin-console
// @Accept      json
// @Produce     json
// @Param       id   path string               true "client id"
// @Param       body body updateServiceRequest true "fields to change"
// @Success     200 {object} serviceDetailResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Failure     409 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/services/{id} [patch]
func (h *Handlers) UpdateService(w http.ResponseWriter, r *http.Request) {
	act, ok := actor(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "client id is required")
		return
	}

	var req updateServiceRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read-modify-write: Hydra's PUT is a full replacement, so we start from the
	// current record and overlay only the provided fields. This also lets us derive
	// the existing client type to enforce the no-silent-flip constraint.
	current, err := h.deps.Clients.GetClient(r.Context(), id)
	if err != nil {
		h.writeClientError(w, r, err, "could not load service")
		return
	}
	currentSummary := adminapi.Summarize(*current)

	// Constrain the client type: a present clientType must match the existing
	// type. Changing public↔confidential changes the auth method + secret lifecycle
	// and must be explicit (here: not supported via edit — delete + recreate).
	if req.ClientType != nil {
		ct := strings.TrimSpace(*req.ClientType)
		if ct != adminapi.ClientTypePublic && ct != adminapi.ClientTypeConfidential {
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "clientType", "clientType must be 'public' or 'confidential'")
			return
		}
		if ct != currentSummary.ClientType {
			httpx.WriteFieldProblem(w, r, http.StatusConflict, "clientType",
				"changing a client between public and confidential is not supported via edit; delete and recreate the service")
			return
		}
	}

	// Build the desired full representation from the current record + overlays.
	name := currentSummary.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
		if name == "" {
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "name", "name cannot be empty")
			return
		}
	}
	redirects := currentSummary.RedirectURIs
	if req.RedirectURIs != nil {
		redirects = *req.RedirectURIs
		if len(redirects) == 0 {
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "redirectUris", "at least one redirect URI is required")
			return
		}
		if !adminapi.ValidRedirectURIs(redirects) {
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "redirectUris", "redirect URIs must be absolute http(s) URLs without a fragment")
			return
		}
	}
	scopes := currentSummary.Scopes
	if req.Scopes != nil {
		scopes = *req.Scopes
	}
	grantTypes := currentSummary.GrantTypes
	if req.GrantTypes != nil {
		grantTypes = *req.GrantTypes
	}
	responseTypes := currentSummary.ResponseTypes
	if req.ResponseTypes != nil {
		responseTypes = *req.ResponseTypes
	}

	// Reuse adminapi's mapping so the Hydra representation matches create exactly,
	// preserving the existing (current) client type / auth method.
	desired := (&adminapi.RegisterClientRequest{
		Name:          name,
		RedirectURIs:  redirects,
		Scopes:        scopes,
		GrantTypes:    grantTypes,
		ResponseTypes: responseTypes,
		ClientType:    currentSummary.ClientType,
	}).ToHydraClient()
	desired.ClientID = id // PUT replaces by id; keep the same id.

	updated, err := h.deps.Clients.UpdateClient(r.Context(), id, desired)
	if err != nil {
		h.writeClientError(w, r, err, "could not update service")
		return
	}

	h.log(r).Info("admin service updated",
		slog.String("actor_id", act.ID.String()),
		slog.String("client_id", id),
		slog.String("client_name", name),
	)
	_ = h.deps.Audit.Append(r.Context(), audit.FromRequest(r, ActionServiceUpdate).
		WithActor(act.ID, act.Username).
		WithTarget(audit.TargetClient, id).
		WithMetadata(map[string]any{"clientName": name}))

	httpx.WriteJSON(w, http.StatusOK, serviceDetailResponse{Service: adminapi.Summarize(*updated)})
}

// DeleteService removes a client.
//
// @Summary     Delete a service (OAuth client)
// @Description Removes a relying-party client. Subsequent authorization requests for that clientId are rejected. Idempotent. Audited. Requires an admin/owner session.
// @Tags        admin-console
// @Param       id path string true "client id"
// @Success     204 "No Content"
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/services/{id} [delete]
func (h *Handlers) DeleteService(w http.ResponseWriter, r *http.Request) {
	act, ok := actor(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "client id is required")
		return
	}

	if err := h.deps.Clients.DeleteClient(r.Context(), id); err != nil {
		h.log(r).Error("admin delete service failed", slog.Any("error", err), slog.String("client_id", id))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not delete service")
		return
	}

	h.log(r).Info("admin service deleted",
		slog.String("actor_id", act.ID.String()),
		slog.String("client_id", id),
	)
	_ = h.deps.Audit.Append(r.Context(), audit.FromRequest(r, ActionServiceDelete).
		WithActor(act.ID, act.Username).
		WithTarget(audit.TargetClient, id))

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// ServiceConsents returns a best-effort count of the client's consent usage.
//
// @Summary     Service consent usage (best-effort)
// @Description Returns the number of users who have an active consent grant for the client. BEST-EFFORT: Hydra v2.2.0 exposes no per-client consent query, so the count is derived by scanning the IdP's subjects (capped); complete=false means the count is a lower bound. Requires an admin/owner session.
// @Tags        admin-console
// @Produce     json
// @Param       id path string true "client id"
// @Success     200 {object} serviceConsentsResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/services/{id}/consents [get]
func (h *Handlers) ServiceConsents(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "client id is required")
		return
	}

	count, complete, err := h.scanClientConsents(r, id, false)
	if err != nil {
		h.log(r).Error("admin service consents count failed", slog.Any("error", err), slog.String("client_id", id))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not read service consents")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, serviceConsentsResponse{ClientID: id, Count: count, Complete: complete})
}

// RevokeServiceConsents revokes the client's grants across the scanned subjects.
//
// @Summary     Revoke a service's consents (best-effort)
// @Description Revokes the client's consent grants so its users must consent again on next authorization. BEST-EFFORT: Hydra v2.2.0 has no client-only revoke, so this revokes per subject across the IdP's subjects (capped); complete=false means some subjects beyond the cap were not processed. Audited. Requires an admin/owner session.
// @Tags        admin-console
// @Produce     json
// @Param       id path string true "client id"
// @Success     200 {object} revokeServiceConsentsResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/services/{id}/consents [delete]
func (h *Handlers) RevokeServiceConsents(w http.ResponseWriter, r *http.Request) {
	act, ok := actor(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "client id is required")
		return
	}

	revoked, complete, err := h.scanClientConsents(r, id, true)
	if err != nil {
		h.log(r).Error("admin service consents revoke failed", slog.Any("error", err), slog.String("client_id", id))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not revoke service consents")
		return
	}

	h.log(r).Info("admin service consents revoked",
		slog.String("actor_id", act.ID.String()),
		slog.String("client_id", id),
		slog.Int("revoked", revoked),
		slog.Bool("complete", complete),
	)
	_ = h.deps.Audit.Append(r.Context(), audit.FromRequest(r, ActionServiceConsentsRevoke).
		WithActor(act.ID, act.Username).
		WithTarget(audit.TargetClient, id).
		WithMetadata(map[string]any{"revoked": revoked, "complete": complete}))

	httpx.WriteJSON(w, http.StatusOK, revokeServiceConsentsResponse{ClientID: id, Revoked: revoked, Complete: complete})
}

// scanClientConsents is the shared best-effort per-client consent worker. It
// enumerates the IdP's subjects (capped) and, for each, inspects Hydra's
// subject-scoped consent sessions for a grant to the target client. When revoke
// is true it also issues the subject+client revoke Hydra supports. It returns the
// number of subjects matched (or revoked) and whether the full subject set was
// scanned. Hydra v2.2.0 exposes no per-client consent query, so this is the
// documented best-effort path (design D3, oidc/hydra.go capability note).
func (h *Handlers) scanClientConsents(r *http.Request, clientID string, revoke bool) (int, bool, error) {
	// Bound the best-effort scan: up to consentScanLimit (default 500) sequential
	// Hydra calls could otherwise approach the HTTP server's write timeout. On
	// deadline we stop early and report the result as incomplete.
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	subjects, complete, err := h.deps.Subjects.ListSubjectIDs(ctx, h.consentScanLimit())
	if err != nil {
		return 0, false, err
	}
	matched := 0
	for _, sid := range subjects {
		// Stop early (incomplete) if we run out of the scan budget.
		if ctx.Err() != nil {
			complete = false
			break
		}
		subject := sid.String()
		records, err := h.deps.Clients.ListConsentSessions(ctx, subject)
		if err != nil {
			return 0, false, err
		}
		granted := false
		for _, rec := range records {
			if rec.ConsentRequest != nil && rec.ConsentRequest.Client != nil &&
				rec.ConsentRequest.Client.ClientID == clientID {
				granted = true
				break
			}
		}
		if !granted {
			continue
		}
		matched++
		if revoke {
			if err := h.deps.Clients.RevokeConsentSessions(ctx, subject, clientID); err != nil {
				return 0, false, err
			}
		}
	}
	return matched, complete, nil
}

// writeClientError maps a Hydra client error to a problem response: a 404 from
// Hydra (unknown client) becomes a 404; anything else is a 502 with the supplied
// generic detail (the underlying error is logged, never leaked).
func (h *Handlers) writeClientError(w http.ResponseWriter, r *http.Request, err error, detail string) {
	var he *oidc.HydraError
	if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
		httpx.WriteProblem(w, r, http.StatusNotFound, "service not found")
		return
	}
	h.log(r).Error("admin service hydra error", slog.Any("error", err))
	httpx.WriteProblem(w, r, http.StatusBadGateway, detail)
}
