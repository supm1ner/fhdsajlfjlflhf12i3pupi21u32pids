// Package admin implements cotton-id's human-operator admin console API: the
// role-gated endpoints behind /api/v1/admin that back the console's Overview,
// Users (list + detail + lifecycle actions), and Journal screens.
//
// ============================================================================
// INTEGRATION CONTRACT
// ============================================================================
//
//	func Mount(r chi.Router, deps Deps)
//	    Registers, under the /api/v1 subrouter, BEHIND auth.RequireRole(admin)
//	    and inside the CSRF group (state-changing browser routes):
//	        GET    /admin/overview
//	        GET    /admin/users
//	        GET    /admin/users/{id}
//	        POST   /admin/users/{id}/suspend
//	        POST   /admin/users/{id}/reactivate
//	        PATCH  /admin/users/{id}/role         (owner-only)
//	        POST   /admin/users/{id}/reset-password
//	        POST   /admin/users/{id}/message
//	        DELETE /admin/users/{id}              (owner-only)
//	        GET    /admin/audit
//
// The session→role gate is auth.RequireRole; it stashes the acting *auth.User on
// the request context (auth.UserFromContext), which the handlers read for the
// audit actor and the self-action / owner-only guards. Every mutating action
// writes an audit entry with actor + target. This human console API is distinct
// from the machine X-Admin-Key client-registration routes (internal/adminapi),
// which stay mounted outside the CSRF group.
// ============================================================================
package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cotton-id/internal/audit"
	"cotton-id/internal/auth"
	"cotton-id/internal/httpx"
	"cotton-id/internal/mailer"
	"cotton-id/internal/observability"
	"cotton-id/internal/oidc"
)

// sessionLister returns a user's active sessions (auth.SessionStore.ListByUser).
type sessionLister interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]auth.Session, error)
}

// servicesCounter returns the number of registered OAuth2 clients (services) for
// the overview. main.go provides an adapter over oidc.HydraClient.ListClients so
// this package does not depend on the Hydra client surface directly.
type servicesCounter interface {
	CountServices(ctx context.Context) (int, error)
}

// clientManager is the Hydra client surface the Services tab needs: full CRUD
// over OAuth relying-party clients plus the subject-scoped consent revoke Hydra
// supports. *oidc.HydraClient satisfies it; declaring the seam keeps the console
// handlers unit-testable with a fake and free of a hard dependency on the Hydra
// client's whole surface.
type clientManager interface {
	ListClients(ctx context.Context) ([]oidc.OAuth2Client, error)
	GetClient(ctx context.Context, id string) (*oidc.OAuth2Client, error)
	CreateClient(ctx context.Context, client oidc.OAuth2Client) (*oidc.OAuth2Client, error)
	UpdateClient(ctx context.Context, id string, client oidc.OAuth2Client) (*oidc.OAuth2Client, error)
	DeleteClient(ctx context.Context, id string) error
	// ListConsentSessions / RevokeConsentSessions are SUBJECT-scoped (Hydra
	// v2.2.0 exposes no per-client query); the console composes them across the
	// IdP's subjects for the best-effort per-client count/revoke (design D3).
	ListConsentSessions(ctx context.Context, subject string) ([]oidc.ConsentSessionRecord, error)
	RevokeConsentSessions(ctx context.Context, subject, client string) error
}

// subjectLister enumerates the IdP's user ids as Hydra subjects (bounded), for
// the best-effort per-client consent count/revoke. *Store satisfies it.
type subjectLister interface {
	ListSubjectIDs(ctx context.Context, limit int) ([]uuid.UUID, bool, error)
}

// messenger sends a transactional email to a target user (the "message user"
// action). *mailer.SMTPMailer / *mailer.LogMailer satisfy it via Send. Optional:
// a nil messenger disables the message endpoint (501), so the console degrades
// gracefully when no mailer is wired.
type messenger interface {
	Send(ctx context.Context, msg mailer.Message) error
}

// Deps are the admin console dependencies, populated by main.go.
type Deps struct {
	Logger  *slog.Logger
	Metrics *observability.Metrics

	// Store provides the admin read aggregates + user listing.
	Store *Store
	// Service implements the lifecycle actions + escalation guards.
	Service *Service
	// Users is the auth user store (detail reads + role/status are via Service).
	Users *auth.UserStore
	// Sessions lists a target user's active sessions for the detail view.
	Sessions sessionLister
	// Audit appends admin-action entries; a nil writer is a safe no-op.
	Audit *audit.Writer
	// Journal reads the audit log for the Journal + activity feeds.
	Journal *audit.Reader
	// Services counts registered services for the overview (Hydra). Optional: a
	// nil counter yields a services count of 0 rather than failing the overview.
	Services servicesCounter
	// Clients is the Hydra client surface backing the Services tab CRUD + the
	// best-effort per-client consent count/revoke. main.go wires *oidc.HydraClient.
	Clients clientManager
	// Subjects enumerates the IdP's subjects for the best-effort per-client
	// consent count/revoke. main.go wires the admin *Store.
	Subjects subjectLister
	// Mailer sends the admin "message user" email. OPTIONAL: when nil the message
	// endpoint returns 501 Not Implemented (the dev LogMailer is wired by default,
	// so this is normally present). Sends are best-effort and audited.
	Mailer messenger
	// ConsentScanLimit caps the per-client consent subject scan (the number of
	// subjects probed against Hydra). Zero applies defaultConsentScanLimit. The
	// count/revoke are reported as best-effort + "complete=false" when the IdP has
	// more subjects than this cap (design D3).
	ConsentScanLimit int
}

// Handlers serves the admin console API.
type Handlers struct {
	deps Deps
}

// NewHandlers builds the admin Handlers.
func NewHandlers(deps Deps) *Handlers {
	return &Handlers{deps: deps}
}

// Mount registers the admin console routes. The caller wraps the subtree in
// auth.RequireRole(auth.RoleAdmin, ...) (the role gate) and the CSRF middleware;
// owner-only actions (role change, delete) re-check owner privilege in-handler.
func (h *Handlers) Mount(r chi.Router) {
	r.Route("/admin", h.MountInto)
}

// MountInto registers the console routes on r WITHOUT the "/admin" prefix, for a
// caller (main.go) that owns the single "/admin" subtree and shares it with the
// machine X-Admin-Key client routes (chi forbids two subrouters at one path).
func (h *Handlers) MountInto(r chi.Router) {
	r.Get("/overview", h.Overview)
	r.Get("/audit", h.Audit)
	r.Route("/users", func(r chi.Router) {
		r.Get("/", h.ListUsers)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.UserDetail)
			r.Post("/suspend", h.Suspend)
			r.Post("/reactivate", h.Reactivate)
			r.Patch("/role", h.ChangeRole)
			r.Post("/reset-password", h.ResetPassword)
			r.Post("/message", h.MessageUser)
			r.Delete("/", h.DeleteUser)
		})
	})
	// Services tab: console management of OAuth relying-party clients. These
	// paths are /api/v1/admin/services* (design D1) — DISTINCT from the machine
	// X-Admin-Key /admin/clients route — and inherit RequireRole(admin) + CSRF +
	// audit from the enclosing mount.
	r.Route("/services", func(r chi.Router) {
		r.Get("/", h.ListServices)
		r.Post("/", h.CreateService)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", h.ServiceDetail)
			r.Patch("/", h.UpdateService)
			r.Delete("/", h.DeleteService)
			r.Get("/consents", h.ServiceConsents)
			r.Delete("/consents", h.RevokeServiceConsents)
		})
	})
}

func (h *Handlers) log(r *http.Request) *slog.Logger {
	return observability.LoggerFrom(r.Context(), h.deps.Logger)
}

// actor returns the admin user RequireRole stashed on the context. It is always
// present on these routes (the middleware ran first); the bool guards against a
// misconfigured mount.
func actor(r *http.Request) (*auth.User, bool) {
	return auth.UserFromContext(r.Context())
}

// Overview returns the admin dashboard aggregates.
//
// @Summary     Admin overview metrics
// @Description Returns total accounts, active-today, new-this-week, registered services count, a 30-day daily sign-up series, recent sign-ups, and a recent activity feed. Requires an admin/owner session.
// @Tags        admin-console
// @Produce     json
// @Success     200 {object} overviewResponse
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/overview [get]
func (h *Handlers) Overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := h.log(r)

	stats, err := h.deps.Store.OverviewStats(ctx)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	series, err := h.deps.Store.SignupSeries(ctx, 30)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	recent, err := h.deps.Store.RecentSignups(ctx, 5)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	activity, _, err := h.deps.Journal.Query(ctx, audit.Filter{Limit: 10})
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	// Services count is best-effort: if Hydra is unreachable the overview still
	// renders (count 0) rather than failing the whole dashboard.
	services := 0
	if h.deps.Services != nil {
		n, cerr := h.deps.Services.CountServices(ctx)
		if cerr != nil {
			log.Warn("admin overview: services count failed", slog.Any("error", cerr))
		} else {
			services = n
		}
	}

	signups := make([]signupPointView, 0, len(series))
	for _, p := range series {
		signups = append(signups, signupPointView{Date: p.Date.Format("2006-01-02"), Count: p.Count})
	}
	recentSummaries := make([]adminUserSummary, 0, len(recent))
	for _, u := range recent {
		recentSummaries = append(recentSummaries, toUserSummary(u))
	}

	httpx.WriteJSON(w, http.StatusOK, overviewResponse{
		TotalUsers:     stats.TotalUsers,
		ActiveToday:    stats.ActiveToday,
		NewThisWeek:    stats.NewThisWeek,
		Services:       services,
		Signups:        signups,
		RecentSignups:  recentSummaries,
		RecentActivity: toAuditViews(activity),
	})
}

// ListUsers returns a filtered, paginated page of users.
//
// @Summary     List users
// @Description Lists users with a case-insensitive search over username/displayName/email, optional status/role filters, and offset pagination, including each user's connected-services count. Requires an admin/owner session.
// @Tags        admin-console
// @Produce     json
// @Param       query    query string false "case-insensitive search term"
// @Param       status   query string false "filter by status" Enums(active, invited, suspended)
// @Param       role     query string false "filter by role" Enums(user, admin, owner)
// @Param       page     query int    false "1-based page number"
// @Param       pageSize query int    false "page size (max 100)"
// @Success     200 {object} usersResponse
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/users [get]
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := atoiDefault(q.Get("page"), 1)
	pageSize := atoiDefault(q.Get("pageSize"), defaultPageSize)

	filter := UserFilter{
		Query:    q.Get("query"),
		Status:   q.Get("status"),
		Role:     q.Get("role"),
		Page:     page,
		PageSize: pageSize,
	}
	// Reject unknown filter values fast (parameterized either way, but a clear 400
	// beats silently returning everything for a typo'd status/role).
	if filter.Status != "" && !validStatus(filter.Status) {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "status", "status must be active, invited, or suspended")
		return
	}
	if filter.Role != "" && !validRole(filter.Role) {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "role", "role must be user, admin, or owner")
		return
	}

	items, total, err := h.deps.Store.ListUsers(r.Context(), filter)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	out := make([]adminUserSummary, 0, len(items))
	for _, it := range items {
		out = append(out, toUserSummary(it))
	}
	httpx.WriteJSON(w, http.StatusOK, usersResponse{
		Users:    out,
		Total:    total,
		Page:     normalizePage(page),
		PageSize: normalizePageSize(pageSize),
	})
}

// UserDetail returns a single user's profile, sessions, recent activity, and
// connected-services count.
//
// @Summary     User detail
// @Description Returns a user's profile, active sessions, recent audit activity (events targeting them), and connected-services count. Requires an admin/owner session.
// @Tags        admin-console
// @Produce     json
// @Param       id path string true "user id (uuid)"
// @Success     200 {object} userDetailResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/users/{id} [get]
func (h *Handlers) UserDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	user, err := h.deps.Users.GetByID(ctx, id)
	if err != nil {
		h.writeUserError(w, r, err)
		return
	}

	sessions, err := h.deps.Sessions.ListByUser(ctx, id)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	sessViews := make([]sessionView, 0, len(sessions))
	for _, s := range sessions {
		sessViews = append(sessViews, sessionView{
			UserAgent: s.UserAgent, IP: s.IP, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt, LastSeenAt: s.LastSeenAt,
		})
	}

	// Recent activity targeting this user: actions filter on the audit target.
	activity := h.recentActivityForTarget(ctx, id)

	connections, err := h.deps.Store.ConnectionsCount(ctx, id)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, userDetailResponse{
		User:           toUserDetail(user),
		Sessions:       sessViews,
		RecentActivity: activity,
		Connections:    connections,
	})
}

// Suspend suspends a user and revokes their sessions.
//
// @Summary     Suspend a user
// @Description Sets the user's status to suspended and revokes all their sessions. A user cannot suspend themselves, and an admin cannot suspend an owner (owner privilege required to act on an owner). Audited.
// @Tags        admin-console
// @Produce     json
// @Param       id path string true "user id (uuid)"
// @Success     200 {object} userEnvelope
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/users/{id}/suspend [post]
func (h *Handlers) Suspend(w http.ResponseWriter, r *http.Request) {
	h.statusAction(w, r, true)
}

// Reactivate reactivates a suspended user.
//
// @Summary     Reactivate a user
// @Description Sets a suspended user's status back to active. Audited.
// @Tags        admin-console
// @Produce     json
// @Param       id path string true "user id (uuid)"
// @Success     200 {object} userEnvelope
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/users/{id}/reactivate [post]
func (h *Handlers) Reactivate(w http.ResponseWriter, r *http.Request) {
	h.statusAction(w, r, false)
}

// statusAction is the shared suspend/reactivate handler.
func (h *Handlers) statusAction(w http.ResponseWriter, r *http.Request, suspend bool) {
	ctx := r.Context()
	act, ok := actor(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var (
		target *auth.User
		err    error
		action string
	)
	if suspend {
		target, err = h.deps.Service.Suspend(ctx, act, id)
		action = ActionUserSuspend
	} else {
		target, err = h.deps.Service.Reactivate(ctx, act, id)
		action = ActionUserReactivate
	}
	if err != nil {
		h.writeGuardError(w, r, err)
		return
	}

	h.log(r).Info("admin status action",
		slog.String("action", action),
		slog.String("actor_id", act.ID.String()),
		slog.String("target_id", target.ID.String()),
	)
	_ = h.deps.Audit.Append(ctx, audit.FromRequest(r, action).
		WithActor(act.ID, act.Username).
		WithTarget(audit.TargetUser, target.ID.String()).
		WithMetadata(map[string]any{"username": target.Username}))

	// Reflect the new status in the returned user without a re-read.
	if suspend {
		target.Status = auth.StatusSuspended
	} else {
		target.Status = auth.StatusActive
	}
	httpx.WriteJSON(w, http.StatusOK, userEnvelope{User: toUserDetail(target)})
}

// ChangeRole grants or revokes a role (owner-only).
//
// @Summary     Change a user's role
// @Description Grants or revokes a role (user/admin/owner). OWNER-ONLY. The last owner cannot be demoted and a user cannot change their own role. Audited.
// @Tags        admin-console
// @Accept      json
// @Produce     json
// @Param       id   path string            true "user id (uuid)"
// @Param       body body changeRoleRequest true "new role"
// @Success     200 {object} userEnvelope
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/users/{id}/role [patch]
func (h *Handlers) ChangeRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	act, ok := actor(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req changeRoleRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	req.Role = strings.TrimSpace(req.Role)

	prevRole, target, err := h.deps.Service.ChangeRole(ctx, act, id, req.Role)
	if err != nil {
		h.writeGuardError(w, r, err)
		return
	}

	h.log(r).Info("admin role change",
		slog.String("actor_id", act.ID.String()),
		slog.String("target_id", target.ID.String()),
		slog.String("from_role", prevRole),
		slog.String("to_role", req.Role),
	)
	_ = h.deps.Audit.Append(ctx, audit.FromRequest(r, ActionUserRole).
		WithActor(act.ID, act.Username).
		WithTarget(audit.TargetUser, target.ID.String()).
		WithMetadata(map[string]any{"fromRole": prevRole, "toRole": req.Role, "username": target.Username}))

	target.Role = req.Role
	httpx.WriteJSON(w, http.StatusOK, userEnvelope{User: toUserDetail(target)})
}

// ResetPassword forces a password reset for a user.
//
// @Summary     Force a password reset
// @Description Issues a single-use password-reset token for the user and (stub-)emails the link. Audited.
// @Tags        admin-console
// @Produce     json
// @Param       id path string true "user id (uuid)"
// @Success     200 {object} messageResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/users/{id}/reset-password [post]
func (h *Handlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	act, ok := actor(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	target, issueErr, err := h.deps.Service.ResetPassword(ctx, act, id)
	if err != nil {
		// A lookup / DB error (target unresolved): map not-found to 404, else 500.
		h.writeUserError(w, r, err)
		return
	}
	if issueErr != nil {
		// Best-effort: the mailer (a dev stub) failed but the token may be
		// persisted. Log it and still report success + audit; the admin can
		// re-issue or share the link rather than the action hard-failing.
		h.log(r).Warn("admin force reset: issuance/delivery failed",
			slog.Any("error", issueErr), slog.String("target_id", id.String()))
	}

	h.log(r).Info("admin forced password reset",
		slog.String("actor_id", act.ID.String()),
		slog.String("target_id", target.ID.String()),
	)
	_ = h.deps.Audit.Append(ctx, audit.FromRequest(r, ActionUserReset).
		WithActor(act.ID, act.Username).
		WithTarget(audit.TargetUser, target.ID.String()).
		WithMetadata(map[string]any{"username": target.Username}))

	httpx.WriteJSON(w, http.StatusOK, messageResponse{Message: "A password reset link has been issued."})
}

// maxMessageSubjectLen / maxMessageBodyLen bound the admin "message user" inputs
// so an oversized request can neither bloat the audit row nor the email.
const (
	maxMessageSubjectLen = 200
	maxMessageBodyLen    = 10000
)

// defaultMessageSubject is used when the admin supplies no subject.
const defaultMessageSubject = "A message from the cotton-id team"

// MessageUser emails an admin-composed message to a target user.
//
// @Summary     Message a user
// @Description Sends an admin-composed email (optional subject, required body) to the target user's address via the configured mailer. Delivery is best-effort: a mailer failure is logged but the action still succeeds and is audited. Requires an admin/owner session.
// @Tags        admin-console
// @Accept      json
// @Produce     json
// @Param       id   path string             true "user id (uuid)"
// @Param       body body messageUserRequest true "subject (optional) and body"
// @Success     200 {object} messageResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Failure     501 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/users/{id}/message [post]
func (h *Handlers) MessageUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	act, ok := actor(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req messageUserRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	subject := strings.TrimSpace(req.Subject)
	body := strings.TrimSpace(req.Body)
	if body == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "body", "message body is required")
		return
	}
	if len([]rune(subject)) > maxMessageSubjectLen {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "subject", "subject is too long")
		return
	}
	if len([]rune(body)) > maxMessageBodyLen {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "body", "message body is too long")
		return
	}
	if subject == "" {
		subject = defaultMessageSubject
	}

	// The mailer is optional; without one there is no way to deliver the message.
	if h.deps.Mailer == nil {
		httpx.WriteProblem(w, r, http.StatusNotImplemented, "messaging is not configured")
		return
	}

	target, err := h.deps.Users.GetByID(ctx, id)
	if err != nil {
		h.writeUserError(w, r, err)
		return
	}

	// Best-effort send: a delivery failure is logged but does NOT fail the action,
	// matching the force-reset semantics — the operator intent is recorded and the
	// admin can retry. The send is bounded by its own context so a slow mail server
	// cannot pin the request goroutine indefinitely.
	sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	sendErr := h.deps.Mailer.Send(sendCtx, mailer.Message{To: target.Email, Subject: subject, Body: body})
	if sendErr != nil {
		h.log(r).Warn("admin message: delivery failed",
			slog.Any("error", sendErr), slog.String("target_id", id.String()))
	}

	h.log(r).Info("admin messaged user",
		slog.String("actor_id", act.ID.String()),
		slog.String("target_id", target.ID.String()),
	)
	_ = h.deps.Audit.Append(ctx, audit.FromRequest(r, ActionUserMessage).
		WithActor(act.ID, act.Username).
		WithTarget(audit.TargetUser, target.ID.String()).
		WithMetadata(map[string]any{"username": target.Username, "subject": subject, "delivered": sendErr == nil}))

	httpx.WriteJSON(w, http.StatusOK, messageResponse{Message: "The message has been sent."})
}

// DeleteUser deletes a user (owner-only).
//
// @Summary     Delete a user
// @Description Permanently deletes a user and cascades their sessions/passkeys/social identities, and best-effort revokes their Hydra grants. OWNER-ONLY. A user cannot delete themselves and the last owner cannot be deleted. Audited.
// @Tags        admin-console
// @Param       id path string true "user id (uuid)"
// @Success     204 "No Content"
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/users/{id} [delete]
func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	act, ok := actor(r)
	if !ok {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	target, hydraErr, err := h.deps.Service.Delete(ctx, act, id)
	if err != nil {
		h.writeGuardError(w, r, err)
		return
	}
	if hydraErr != nil {
		// The account row is already gone; a Hydra revoke failure is logged, not
		// surfaced (best-effort cleanup, design D5).
		h.log(r).Warn("admin delete: hydra revoke failed",
			slog.Any("error", hydraErr), slog.String("target_id", id.String()))
	}

	h.log(r).Info("admin deleted user",
		slog.String("actor_id", act.ID.String()),
		slog.String("target_id", target.ID.String()),
	)
	_ = h.deps.Audit.Append(ctx, audit.FromRequest(r, ActionUserDelete).
		WithActor(act.ID, act.Username).
		WithTarget(audit.TargetUser, target.ID.String()).
		WithMetadata(map[string]any{"username": target.Username, "email": target.Email}))

	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// Audit returns a filtered, paginated page of the audit log (the Journal).
//
// @Summary     Query the audit log (Journal)
// @Description Returns audit entries newest-first, with optional actor/action/time-range filters and pagination. Requires an admin/owner session.
// @Tags        admin-console
// @Produce     json
// @Param       actor      query string false "filter by actor: an exact actor id (uuid) or a case-insensitive actor-label substring"
// @Param       action     query string false "filter by action (e.g. auth.login.ok)"
// @Param       targetType query string false "filter by target type (e.g. user, client, session)"
// @Param       targetId   query string false "filter by target id (the affected entity's id)"
// @Param       from       query string false "lower time bound (RFC3339, inclusive)"
// @Param       to         query string false "upper time bound (RFC3339, exclusive)"
// @Param       page       query int    false "1-based page number"
// @Param       pageSize   query int    false "page size (max 200)"
// @Success     200 {object} auditResponse
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Security    CSRF
// @Router      /admin/audit [get]
func (h *Handlers) Audit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page := atoiDefault(q.Get("page"), 1)
	// Default + clamp the page size BEFORE computing the offset: an omitted/zero
	// pageSize previously left the offset at 0 for every page, so paging could
	// never reach older entries. Mirror the users handler's clamping.
	pageSize := atoiDefault(q.Get("pageSize"), 50)
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	f := audit.Filter{
		Action:     q.Get("action"),
		TargetType: q.Get("targetType"),
		TargetID:   q.Get("targetId"),
		Limit:      pageSize,
		Offset:     (normalizePage(page) - 1) * pageSize,
	}
	// The actor input is overloaded: a valid UUID filters on the exact actor_id;
	// any other free text is a case-insensitive actor_label substring search (so
	// the Journal's free-text actor box works without the operator knowing ids).
	if a := strings.TrimSpace(q.Get("actor")); a != "" {
		if actorID, err := uuid.Parse(a); err == nil {
			f.Actor = &actorID
		} else {
			f.ActorLabel = a
		}
	}
	if from := q.Get("from"); from != "" {
		ts, err := parseRFC3339(from)
		if err != nil {
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "from", "from must be an RFC3339 timestamp")
			return
		}
		f.From = &ts
	}
	if to := q.Get("to"); to != "" {
		ts, err := parseRFC3339(to)
		if err != nil {
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "to", "to must be an RFC3339 timestamp")
			return
		}
		f.To = &ts
	}

	entries, total, err := h.deps.Journal.Query(r.Context(), f)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, auditResponse{
		Entries:  toAuditViews(entries),
		Total:    total,
		Page:     normalizePage(page),
		PageSize: pageSize,
	})
}

// --- helpers ---

// recentActivityForTarget returns up to 10 recent audit entries that target the
// given user. It filters server-side on (target_type=user, target_id=id) using the
// audit target filter (backed by the audit_log_target_idx index from migration
// 0006), so the query selects exactly the user's entries instead of scanning a
// recent 200-row window and discarding non-matches. A query error yields an empty
// slice so the detail view still renders.
func (h *Handlers) recentActivityForTarget(ctx context.Context, id uuid.UUID) []auditView {
	entries, _, err := h.deps.Journal.Query(ctx, audit.Filter{
		TargetType: audit.TargetUser,
		TargetID:   id.String(),
		Limit:      10,
	})
	if err != nil {
		return []auditView{}
	}
	return toAuditViews(entries)
}

// writeGuardError maps the admin Service's typed guard errors to problem
// responses. Self-action / last-owner / invalid-transition are 409 (the request
// conflicts with an invariant); owner-only is 403; invalid role is 400;
// not-found is 404.
func (h *Handlers) writeGuardError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrUserNotFound):
		httpx.WriteProblem(w, r, http.StatusNotFound, "user not found")
	case errors.Is(err, ErrOwnerOnly):
		httpx.WriteProblem(w, r, http.StatusForbidden, "only an owner may perform this action")
	case errors.Is(err, ErrSelfAction):
		httpx.WriteProblem(w, r, http.StatusConflict, "cannot perform this action on your own account")
	case errors.Is(err, ErrLastOwner):
		httpx.WriteProblem(w, r, http.StatusConflict, "cannot remove the last owner")
	case errors.Is(err, ErrInvalidStatusTransition):
		httpx.WriteProblem(w, r, http.StatusConflict, "the account is already in the requested state")
	case errors.Is(err, ErrInvalidRole):
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "role", "role must be user, admin, or owner")
	default:
		httpx.WriteServerError(w, r, err)
	}
}

// writeUserError maps a user lookup error to a 404 (not found) or 500.
func (h *Handlers) writeUserError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, auth.ErrUserNotFound) {
		httpx.WriteProblem(w, r, http.StatusNotFound, "user not found")
		return
	}
	httpx.WriteServerError(w, r, err)
}

// parseID reads and validates the {id} path param as a uuid, writing a 400 and
// returning ok=false on failure.
func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := chi.URLParam(r, "id")
	id, err := uuid.Parse(raw)
	if err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, "id must be a valid uuid")
		return uuid.Nil, false
	}
	return id, true
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func normalizePage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func normalizePageSize(size int) int {
	if size <= 0 {
		return defaultPageSize
	}
	if size > maxPageSize {
		return maxPageSize
	}
	return size
}

func validStatus(s string) bool {
	switch s {
	case auth.StatusActive, auth.StatusInvited, auth.StatusSuspended:
		return true
	default:
		return false
	}
}

// parseRFC3339 parses an RFC3339 timestamp (the Journal's from/to bounds).
func parseRFC3339(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}
