package account

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"cotton-id/internal/auth"
	"cotton-id/internal/httpx"
	"cotton-id/internal/observability"
)

// handlers.go — the account self-service HTTP surface, mounted under /api/v1 by
// main.go. Every route requires an active cotton-id session (resolved via
// auth.Service.UserForSession) and lives in the /api/v1 CSRF group:
//
//	GET    /api/v1/account                      → full profile (user + prefs + counts)
//	PATCH  /api/v1/account                      → update displayName/about/location
//	PATCH  /api/v1/account/preferences          → theme/lang/loginNotifications
//	PUT    /api/v1/account/images/{kind}        → multipart avatar/banner upload
//	GET    /api/v1/account/images/{kind}        → serve the bytes (owner only)
//	PUT    /api/v1/account/password             → change password (re-auth, revoke others)
//	GET    /api/v1/account/sessions             → list sessions (current flagged)
//	DELETE /api/v1/account/sessions/{id}        → revoke one session (scoped)
//	DELETE /api/v1/account/sessions             → revoke all but the current
//	GET    /api/v1/account/connections          → Hydra consent grants
//	DELETE /api/v1/account/connections/{client} → revoke a client's consent
//	DELETE /api/v1/account                       → delete account (re-auth required)

// maxMultipartMemory bounds the in-memory portion of a multipart parse; the hard
// byte cap is enforced separately via MaxBytesReader sized to the kind.
const maxMultipartMemory = 1 << 20 // 1 MiB

// Handlers serves the account self-service API.
type Handlers struct {
	deps Deps
	svc  *Service
}

// NewHandlers builds the account Handlers, assembling the domain service from the
// concrete deps.
func NewHandlers(deps Deps) *Handlers {
	return &Handlers{deps: deps, svc: buildService(deps)}
}

// Mount registers the account routes on r (the /api/v1 CSRF subrouter).
func (h *Handlers) Mount(r chi.Router) {
	r.Route("/account", func(r chi.Router) {
		r.Get("/", h.GetProfile)
		r.Patch("/", h.UpdateProfile)
		r.Delete("/", h.DeleteAccount)
		r.Patch("/preferences", h.UpdatePreferences)
		r.Put("/password", h.ChangePassword)
		r.Route("/images", func(r chi.Router) {
			r.Put("/{kind}", h.UploadImage)
			r.Get("/{kind}", h.ServeImage)
		})
		r.Route("/sessions", func(r chi.Router) {
			r.Get("/", h.ListSessions)
			r.Delete("/", h.RevokeOtherSessions)
			r.Delete("/{id}", h.RevokeSession)
		})
		r.Route("/connections", func(r chi.Router) {
			r.Get("/", h.ListConnections)
			r.Delete("/{client}", h.RevokeConnection)
		})
	})
}

// --- response/request DTOs (camelCase JSON) ---

// userView is the account self-service projection of a user: the public fields
// plus the avatar/banner URLs, status/role, preferences, and timestamps. Richer
// than auth.PublicUser (which the auth/oidc flows depend on and must not change).
type userView struct {
	ID                 string  `json:"id"`
	Email              string  `json:"email"`
	EmailVerified      bool    `json:"emailVerified"`
	Username           string  `json:"username"`
	DisplayName        string  `json:"displayName"`
	Role               string  `json:"role"`
	Status             string  `json:"status"`
	About              string  `json:"about"`
	Location           string  `json:"location"`
	AvatarURL          *string `json:"avatarUrl,omitempty"`
	BannerURL          *string `json:"bannerUrl,omitempty"`
	Theme              string  `json:"theme"`
	Lang               string  `json:"lang"`
	LoginNotifications bool    `json:"loginNotifications"`
	HasPassword        bool    `json:"hasPassword"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
}

func toUserView(u *auth.User) userView {
	return userView{
		ID:                 u.ID.String(),
		Email:              u.Email,
		EmailVerified:      u.EmailVerified,
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		Role:               u.Role,
		Status:             u.Status,
		About:              u.About,
		Location:           u.Location,
		AvatarURL:          u.AvatarURL,
		BannerURL:          u.BannerURL,
		Theme:              u.PrefTheme,
		Lang:               u.PrefLang,
		LoginNotifications: u.LoginNotifications,
		HasPassword:        u.PasswordHash != nil,
		CreatedAt:          u.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:          u.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// countsView is the security-overview tally.
type countsView struct {
	Sessions    int `json:"sessions"`
	Passkeys    int `json:"passkeys"`
	Connections int `json:"connections"`
}

// profileResponse is the GET /account body: the user plus the counts.
type profileResponse struct {
	User   userView   `json:"user"`
	Counts countsView `json:"counts"`
}

// userEnvelope wraps the user for the PATCH responses, matching the codebase's
// single-object response convention (e.g. {user}).
type userEnvelope struct {
	User userView `json:"user"`
}

type updateProfileRequest struct {
	DisplayName string `json:"displayName" example:"Alex Cotton"`
	About       string `json:"about" example:"Building things."`
	Location    string `json:"location" example:"Almaty, KZ"`
}

type updatePreferencesRequest struct {
	Theme              string `json:"theme" example:"system"`
	Lang               string `json:"lang" example:"ru"`
	LoginNotifications bool   `json:"loginNotifications" example:"true"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" example:"old-Pass-1!"`
	NewPassword     string `json:"newPassword" example:"new-Pass-2!"`
}

type sessionView struct {
	ID         string `json:"id"`
	UserAgent  string `json:"userAgent"`
	IP         string `json:"ip"`
	CreatedAt  string `json:"createdAt"`
	ExpiresAt  string `json:"expiresAt"`
	LastSeenAt string `json:"lastSeenAt"`
	Current    bool   `json:"current"`
}

type sessionsResponse struct {
	Sessions []sessionView `json:"sessions"`
}

type connectionView struct {
	Client        connectionClient `json:"client"`
	GrantedScopes []string         `json:"grantedScopes"`
	GrantedAt     string           `json:"grantedAt,omitempty"`
}

type connectionClient struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type connectionsResponse struct {
	Connections []connectionView `json:"connections"`
}

type deleteAccountRequest struct {
	CurrentPassword string `json:"currentPassword,omitempty" example:"old-Pass-1!"`
	Confirm         bool   `json:"confirm,omitempty" example:"true"`
}

// --- profile ---

// GetProfile returns the authenticated user's full profile and security counts.
//
// @Summary     Get account profile
// @Description Returns the current user's full profile (including avatar/banner URLs and preferences) plus session/passkey/connection counts for the security overview. Requires an authenticated session.
// @Tags        account
// @Produce     json
// @Success     200 {object} profileResponse
// @Failure     401 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /account [get]
func (h *Handlers) GetProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	profile, err := h.svc.GetProfile(r.Context(), user)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, profileResponse{
		User: toUserView(profile.User),
		Counts: countsView{
			Sessions:    profile.Counts.Sessions,
			Passkeys:    profile.Counts.Passkeys,
			Connections: profile.Counts.Connections,
		},
	})
}

// UpdateProfile updates the user's editable profile fields.
//
// @Summary     Update account profile
// @Description Updates the current user's display name, about, and location (validated and length-bounded). Requires an authenticated session.
// @Tags        account
// @Accept      json
// @Produce     json
// @Param       body body updateProfileRequest true "Profile fields"
// @Success     200 {object} userEnvelope
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /account [patch]
func (h *Handlers) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req updateProfileRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.svc.UpdateProfile(r.Context(), user, UpdateProfileInput{
		DisplayName: req.DisplayName,
		About:       req.About,
		Location:    req.Location,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrDisplayNameRequired), errors.Is(err, ErrDisplayNameTooLong):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "displayName", err.Error())
		case errors.Is(err, ErrAboutTooLong):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "about", err.Error())
		case errors.Is(err, ErrLocationTooLong):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "location", err.Error())
		default:
			httpx.WriteServerError(w, r, err)
		}
		return
	}
	log.Info("account profile updated", slog.String("user_id", user.ID.String()), slog.String("ip", httpx.ClientIP(r)))
	httpx.WriteJSON(w, http.StatusOK, userEnvelope{User: toUserView(updated)})
}

// UpdatePreferences updates the user's theme/language/login-notification settings.
//
// @Summary     Update preferences
// @Description Updates the current user's theme (dark|light|system), language (ru|en), and login-notification toggle. Persisted server-side so they sync across devices. Requires an authenticated session.
// @Tags        account
// @Accept      json
// @Produce     json
// @Param       body body updatePreferencesRequest true "Preferences"
// @Success     200 {object} userEnvelope
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /account/preferences [patch]
func (h *Handlers) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req updatePreferencesRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.svc.UpdatePreferences(r.Context(), user, UpdatePreferencesInput{
		Theme:              req.Theme,
		Lang:               req.Lang,
		LoginNotifications: req.LoginNotifications,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidTheme):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "theme", err.Error())
		case errors.Is(err, ErrInvalidLang):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "lang", err.Error())
		default:
			httpx.WriteServerError(w, r, err)
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, userEnvelope{User: toUserView(updated)})
}

// --- images ---

// UploadImage stores an avatar or banner from a multipart upload.
//
// @Summary     Upload avatar or banner
// @Description Uploads the current user's avatar or banner as multipart/form-data (field "file"). The content type is validated by magic bytes (png/jpeg/webp only) and the size is capped (avatar ≤ 512 KiB, banner ≤ 1 MiB). The image is stored and the user's avatar/banner URL is set. Requires an authenticated session.
// @Tags        account
// @Accept      multipart/form-data
// @Produce     json
// @Param       kind path string true "Image kind" Enums(avatar, banner)
// @Param       file formData file true "Image file (png, jpeg, or webp)"
// @Success     200 {object} userEnvelope
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     413 {object} httpx.Problem
// @Failure     415 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /account/images/{kind} [put]
func (h *Handlers) UploadImage(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	kind := chi.URLParam(r, "kind")
	if !ValidKind(kind) {
		httpx.WriteProblem(w, r, http.StatusNotFound, "unknown image kind")
		return
	}

	// Hard cap the request body to the kind's size limit (plus a small multipart
	// envelope allowance) before parsing, so an oversized upload is rejected
	// without buffering the whole thing.
	sizeCap := MaxBytesForKind(kind)
	r.Body = http.MaxBytesReader(w, r.Body, int64(sizeCap)+4096)
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.WriteFieldProblem(w, r, http.StatusRequestEntityTooLarge, "file", ErrImageTooLarge.Error())
			return
		}
		httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid multipart upload")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "file", "a file field is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, int64(sizeCap)+1))
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	contentType, verr := ValidateImage(kind, data)
	if verr != nil {
		switch {
		case errors.Is(verr, ErrImageTooLarge):
			httpx.WriteFieldProblem(w, r, http.StatusRequestEntityTooLarge, "file", verr.Error())
		case errors.Is(verr, ErrImageUnsupportedType):
			httpx.WriteFieldProblem(w, r, http.StatusUnsupportedMediaType, "file", verr.Error())
		case errors.Is(verr, ErrImageEmpty):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "file", verr.Error())
		default:
			httpx.WriteProblem(w, r, http.StatusBadRequest, verr.Error())
		}
		return
	}

	// Store the blob AND point the user's avatar/banner URL at the served route in
	// ONE transaction, so the bytes and the URL commit together (atomic upload): a
	// failure rolls both back rather than leaving a stored blob with no URL set (or
	// a URL pointing at bytes that never persisted). The served bytes change in
	// place, so the URL itself is stable across re-uploads.
	servedURL := h.deps.PublicBaseURL + "/api/v1/account/images/" + kind
	if err := h.deps.Images.UpsertWithURL(r.Context(), user.ID, kind, contentType, data, servedURL); err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}

	refreshed, err := h.deps.Users.GetByID(r.Context(), user.ID)
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	log.Info("account image uploaded",
		slog.String("user_id", user.ID.String()),
		slog.String("kind", kind),
		slog.String("content_type", contentType),
		slog.Int("bytes", len(data)),
		slog.String("ip", httpx.ClientIP(r)),
	)
	httpx.WriteJSON(w, http.StatusOK, userEnvelope{User: toUserView(refreshed)})
}

// ServeImage serves the authenticated user's avatar or banner bytes.
//
// @Summary     Serve avatar or banner
// @Description Serves the current user's avatar or banner bytes with the stored content type. Auth-gated to the owner for now (a public cross-app URL is a later change). Requires an authenticated session.
// @Tags        account
// @Produce     image/png
// @Param       kind path string true "Image kind" Enums(avatar, banner)
// @Success     200 {file} binary
// @Failure     401 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /account/images/{kind} [get]
func (h *Handlers) ServeImage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	kind := chi.URLParam(r, "kind")
	if !ValidKind(kind) {
		httpx.WriteProblem(w, r, http.StatusNotFound, "unknown image kind")
		return
	}
	img, err := h.deps.Images.Get(r.Context(), user.ID, kind)
	if err != nil {
		if errors.Is(err, ErrImageNotFound) {
			httpx.WriteProblem(w, r, http.StatusNotFound, "image not found")
			return
		}
		httpx.WriteServerError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", img.ContentType)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(img.Bytes)
}

// --- password ---

// ChangePassword changes the user's password after re-authenticating.
//
// @Summary     Change password
// @Description Changes the current user's password: verifies the current password, enforces the password policy on the new one, rehashes, and revokes the user's other sessions (the current device stays signed in). Requires an authenticated session.
// @Tags        account
// @Accept      json
// @Success     204 "No Content"
// @Param       body body changePasswordRequest true "Current and new password"
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /account/password [put]
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req changePasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	currentSessionID := h.currentSessionID(r)
	revoked, err := h.svc.ChangePassword(r.Context(), user, req.CurrentPassword, req.NewPassword, currentSessionID)
	if err != nil {
		switch {
		case errors.Is(err, ErrAccountLocked):
			log.Warn("password change refused: account locked", slog.String("user_id", user.ID.String()), slog.String("ip", httpx.ClientIP(r)))
			w.Header().Set("Retry-After", "60")
			httpx.WriteProblem(w, r, http.StatusTooManyRequests, err.Error())
		case errors.Is(err, ErrWrongPassword):
			log.Info("password change rejected: wrong current password", slog.String("user_id", user.ID.String()), slog.String("ip", httpx.ClientIP(r)))
			httpx.WriteFieldProblem(w, r, http.StatusUnauthorized, "currentPassword", err.Error())
		case errors.Is(err, auth.ErrPasswordTooShort), errors.Is(err, auth.ErrPasswordTooWeak), errors.Is(err, auth.ErrPasswordTooLong):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "newPassword", err.Error())
		default:
			httpx.WriteServerError(w, r, err)
		}
		return
	}
	log.Info("password changed",
		slog.String("user_id", user.ID.String()),
		slog.Int64("other_sessions_revoked", revoked),
		slog.String("ip", httpx.ClientIP(r)),
	)
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// --- sessions ---

// ListSessions lists the user's active sessions with the current one flagged.
//
// @Summary     List active sessions
// @Description Returns the current user's active sessions (id, user agent, ip, created/expires timestamps), flagging the request's own session as current. Requires an authenticated session.
// @Tags        account
// @Produce     json
// @Success     200 {object} sessionsResponse
// @Failure     401 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /account/sessions [get]
func (h *Handlers) ListSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	sessions, err := h.svc.ListSessions(r.Context(), user, h.currentSessionID(r))
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	out := sessionsResponse{Sessions: make([]sessionView, 0, len(sessions))}
	for _, s := range sessions {
		out.Sessions = append(out.Sessions, sessionView{
			ID:         s.ID,
			UserAgent:  s.UserAgent,
			IP:         s.IP,
			CreatedAt:  s.CreatedAt.UTC().Format(time.RFC3339),
			ExpiresAt:  s.ExpiresAt.UTC().Format(time.RFC3339),
			LastSeenAt: s.LastSeenAt.UTC().Format(time.RFC3339),
			Current:    s.Current,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// RevokeSession revokes one of the user's sessions by id.
//
// @Summary     Revoke a session
// @Description Revokes one of the current user's sessions by its id (scoped to the user). Returns 404 if the session does not exist or belongs to another account. Requires an authenticated session.
// @Tags        account
// @Param       id path string true "Session id (sha256 hex)"
// @Success     204 "No Content"
// @Failure     401 {object} httpx.Problem
// @Failure     404 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /account/sessions/{id} [delete]
func (h *Handlers) RevokeSession(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.svc.RevokeSession(r.Context(), user, id); err != nil {
		if errors.Is(err, auth.ErrSessionNotFound) {
			httpx.WriteProblem(w, r, http.StatusNotFound, "session not found")
			return
		}
		httpx.WriteServerError(w, r, err)
		return
	}
	log.Info("session revoked", slog.String("user_id", user.ID.String()), slog.String("ip", httpx.ClientIP(r)))
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// RevokeOtherSessions revokes all of the user's sessions except the current one.
//
// @Summary     Revoke other sessions
// @Description Revokes all of the current user's sessions except the request's own (so the current device stays signed in). Requires an authenticated session.
// @Tags        account
// @Success     204 "No Content"
// @Failure     401 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /account/sessions [delete]
func (h *Handlers) RevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	revoked, err := h.svc.RevokeOtherSessions(r.Context(), user, h.currentSessionID(r))
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	log.Info("other sessions revoked",
		slog.String("user_id", user.ID.String()),
		slog.Int64("revoked", revoked),
		slog.String("ip", httpx.ClientIP(r)),
	)
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// --- connections ---

// ListConnections lists the user's connected apps (Hydra consent grants).
//
// @Summary     List connected apps
// @Description Returns the relying parties the current user has granted access to (client id/name + granted scopes), read from Hydra's consent sessions for the user's subject. Requires an authenticated session.
// @Tags        account
// @Produce     json
// @Success     200 {object} connectionsResponse
// @Failure     401 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /account/connections [get]
func (h *Handlers) ListConnections(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	conns, err := h.svc.ListConnections(r.Context(), user)
	if err != nil {
		log.Error("list connections: hydra error", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not load connected apps")
		return
	}
	out := connectionsResponse{Connections: make([]connectionView, 0, len(conns))}
	for _, c := range conns {
		out.Connections = append(out.Connections, connectionView{
			Client:        connectionClient{ID: c.ClientID, Name: c.ClientName},
			GrantedScopes: c.GrantedScopes,
			GrantedAt:     c.GrantedAt,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// RevokeConnection revokes the user's consent for a client.
//
// @Summary     Revoke a connected app
// @Description Revokes the current user's consent for the given client, so the app must obtain consent again on its next authorization. Requires an authenticated session.
// @Tags        account
// @Param       client path string true "OAuth2 client id"
// @Success     204 "No Content"
// @Failure     401 {object} httpx.Problem
// @Failure     502 {object} httpx.Problem
// @Security    CSRF
// @Router      /account/connections/{client} [delete]
func (h *Handlers) RevokeConnection(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	clientID := chi.URLParam(r, "client")
	if clientID == "" {
		httpx.WriteProblem(w, r, http.StatusNotFound, "client not found")
		return
	}
	if err := h.svc.RevokeConnection(r.Context(), user, clientID); err != nil {
		log.Error("revoke connection: hydra error", slog.Any("error", err))
		httpx.WriteProblem(w, r, http.StatusBadGateway, "could not revoke connected app")
		return
	}
	log.Info("connection revoked",
		slog.String("user_id", user.ID.String()),
		slog.String("client_id", clientID),
		slog.String("ip", httpx.ClientIP(r)),
	)
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// --- deletion ---

// DeleteAccount permanently deletes the user's account after re-authentication.
//
// @Summary     Delete account
// @Description Permanently deletes the current user's account after re-authentication (the current password for password accounts, or confirm=true for social/passkey-only accounts). FK cascade removes sessions, passkeys, social identities, and profile images; the subject's Hydra login/consent sessions are best-effort revoked; the session cookie is cleared. Irreversible. Requires an authenticated session.
// @Tags        account
// @Accept      json
// @Success     204 "No Content"
// @Param       body body deleteAccountRequest true "Re-auth material"
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     500 {object} httpx.Problem
// @Security    CSRF
// @Router      /account [delete]
func (h *Handlers) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.deps.Logger)
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req deleteAccountRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), user, DeleteAccountInput{
		CurrentPassword: req.CurrentPassword,
		Confirm:         req.Confirm,
	}); err != nil {
		switch {
		case errors.Is(err, ErrAccountLocked):
			log.Warn("account deletion refused: account locked", slog.String("user_id", user.ID.String()), slog.String("ip", httpx.ClientIP(r)))
			w.Header().Set("Retry-After", "60")
			httpx.WriteProblem(w, r, http.StatusTooManyRequests, err.Error())
		case errors.Is(err, ErrWrongPassword):
			log.Warn("account deletion rejected: wrong current password", slog.String("user_id", user.ID.String()), slog.String("ip", httpx.ClientIP(r)))
			httpx.WriteFieldProblem(w, r, http.StatusUnauthorized, "currentPassword", err.Error())
		case errors.Is(err, ErrReauthRequired):
			httpx.WriteProblem(w, r, http.StatusUnauthorized, err.Error())
		default:
			httpx.WriteServerError(w, r, err)
		}
		return
	}
	h.clearSessionCookie(w)
	log.Warn("account deleted", slog.String("user_id", user.ID.String()), slog.String("ip", httpx.ClientIP(r)))
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// --- helpers ---

// requireUser resolves the authenticated user from the session cookie or writes a
// 401 problem and returns ok=false. It is the auth gate for every account route
// (mirrors internal/passkey's requireUser).
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	c, err := r.Cookie(h.deps.SessionCookieName)
	if err != nil || c.Value == "" {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return nil, false
	}
	user, err := h.deps.Auth.UserForSession(r.Context(), c.Value)
	if err != nil {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return nil, false
	}
	return user, true
}

// currentSessionID returns the sha256 hex id of the request's session cookie, or
// "" when absent. The session row's primary key is HashToken(cookie), so this is
// how the current device is matched for the "current" flag and for keeping the
// in-flight session on revoke-others / password-change.
func (h *Handlers) currentSessionID(r *http.Request) string {
	c, err := r.Cookie(h.deps.SessionCookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	return auth.HashToken(c.Value)
}

// clearSessionCookie expires the session cookie with the same attributes the auth
// handlers use, so a deleted account's browser is logged out.
func (h *Handlers) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.deps.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.deps.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
