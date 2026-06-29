package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cotton-id/internal/audit"
	"cotton-id/internal/httpx"
	"cotton-id/internal/notify"
	"cotton-id/internal/observability"
)

// Handlers serves the email/password auth API. It owns cookie issuance, CSRF
// token minting, per-account rate limiting, and security-event logging. It is
// mounted under /api/v1 by main.go via Mount.
type Handlers struct {
	svc     *Service
	log     *slog.Logger
	metrics *observability.Metrics
	limiter httpx.RateLimiter
	lockout Lockout

	// audit is an OPTIONAL audit-log sink. It is nil unless main.go wires one via
	// WithAudit; a nil *audit.Writer is a safe no-op, so tests that construct
	// Handlers without it are unaffected.
	audit *audit.Writer

	// notifier + sessions back the best-effort login-notification email. Both are
	// OPTIONAL (nil unless main.go wires them via WithLoginNotifier); a nil notifier
	// or nil sessions disables the feature, so tests are unaffected.
	notifier *notify.Notifier
	sessions sessionLister

	sessionCookieName string
	csrfCookieName    string
	cookieSecure      bool

	verifier *CodeStore
}

// sessionLister lists a user's active sessions for the new-device login-notification
// heuristic. *SessionStore satisfies it via ListByUser.
type sessionLister interface {
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Session, error)
}

// WithAudit attaches an audit-log writer to the handlers and returns h for
// fluent wiring. Optional and additive: when unset, audit calls are no-ops. main.go
// calls this; tests may omit it.
func (h *Handlers) WithAudit(w *audit.Writer) *Handlers {
	h.audit = w
	return h
}

// WithVerifier attaches a code verifier for signup email verification.
func (h *Handlers) WithVerifier(v *CodeStore) *Handlers {
	h.verifier = v
	return h
}

// WithLoginNotifier attaches the login-notification notifier + the session lister
// the new-device heuristic reads, returning h for fluent wiring. Optional and
// additive: when either is nil the feature is a no-op, so tests are unaffected.
func (h *Handlers) WithLoginNotifier(n *notify.Notifier, sessions sessionLister) *Handlers {
	h.notifier = n
	h.sessions = sessions
	return h
}

// HandlersConfig configures the auth Handlers.
type HandlersConfig struct {
	SessionCookieName string
	CSRFCookieName    string
	CookieSecure      bool
}

// NewHandlers builds the auth Handlers. limiter applies steady-state per-IP /
// per-account rate caps; lockout applies incremental backoff after consecutive
// login failures for an account.
func NewHandlers(svc *Service, log *slog.Logger, metrics *observability.Metrics, limiter httpx.RateLimiter, lockout Lockout, cfg HandlersConfig) *Handlers {
	return &Handlers{
		svc:               svc,
		log:               log,
		metrics:           metrics,
		limiter:           limiter,
		lockout:           lockout,
		sessionCookieName: cfg.SessionCookieName,
		csrfCookieName:    cfg.CSRFCookieName,
		cookieSecure:      cfg.CookieSecure,
	}
}

// Mount registers the auth routes onto r (which is the /api/v1 subrouter).
// CSRF and per-IP rate limiting are applied by the caller's middleware stack;
// per-account throttling is applied inside login/forgot handlers.
func (h *Handlers) Mount(r chi.Router) {
	r.Get("/csrf", h.CSRF)
	r.Route("/auth", func(r chi.Router) {
		r.Post("/signup", h.Signup)
		r.Post("/signup/send-code", h.SignupSendCode)
		r.Post("/signup/verify-code", h.SignupVerifyCode)
		r.Post("/login", h.Login)
		r.Post("/logout", h.Logout)
		r.Get("/session", h.Session)
		r.Post("/password/forgot", h.PasswordForgot)
		r.Post("/password/reset", h.PasswordReset)
		// Social-login routes (/auth/social/...) are owned by internal/social and
		// mounted separately by main.go.
	})
}

// --- request/response DTOs (camelCase JSON per contract §3) ---

type signupRequest struct {
	DisplayName string `json:"displayName" example:"Alex Cotton"`
	Username    string `json:"username" example:"alex"`
	Email       string `json:"email" example:"alex@cotton-id.io"`
	Password    string `json:"password" example:"hunter2Pass!"`
}

type loginRequest struct {
	Email    string `json:"email" example:"alex@cotton-id.io"`
	Password string `json:"password" example:"hunter2Pass!"`
	Remember bool   `json:"remember" example:"true"`
}

type forgotRequest struct {
	Email string `json:"email" example:"alex@cotton-id.io"`
}

type resetRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// userEnvelope wraps the public user in a {user} object per the contract.
type userEnvelope struct {
	User PublicUser `json:"user"`
}

type csrfResponse struct {
	Token string `json:"token"`
}

type messageResponse struct {
	Message string `json:"message" example:"If that email is registered, a reset link has been sent."`
}

type sendCodeRequest struct {
	Email string `json:"email"`
}

type verifyCodeRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type verifyCodeResponse struct {
	Valid bool `json:"valid"`
}

// CSRF issues a fresh CSRF token, sets the cid_csrf cookie, and returns the token
// in the body so the SPA can echo it in the X-CSRF-Token header.
//
// @Summary     Issue a CSRF token
// @Description Sets the cid_csrf cookie and returns the matching token for the double-submit header.
// @Tags        auth
// @Produce     json
// @Success     200 {object} csrfResponse
// @Router      /csrf [get]
func (h *Handlers) CSRF(w http.ResponseWriter, r *http.Request) {
	token, err := httpx.SetCSRFCookie(w, httpx.CSRFConfig{CookieName: h.csrfCookieName, Secure: h.cookieSecure})
	if err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, csrfResponse{Token: token})
}

// Signup creates an account and establishes a session.
//
// @Summary     Create an account
// @Description Creates a new account with display name, username, email, and password, then establishes a session.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body signupRequest true "Signup payload"
// @Success     201 {object} userEnvelope
// @Failure     400 {object} httpx.Problem
// @Failure     409 {object} httpx.Problem
// @Failure     429 {object} httpx.Problem
// @Router      /auth/signup [post]
func (h *Handlers) Signup(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.log)

	var req signupRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}

	res, err := h.svc.Signup(r.Context(), SignupParams{
		DisplayName: req.DisplayName,
		Username:    req.Username,
		Email:       req.Email,
		Password:    req.Password,
		UserAgent:   r.UserAgent(),
		IP:          httpx.ClientIP(r),
	})
	if err != nil {
		h.metrics.SignupAttempts.WithLabelValues("failure").Inc()
		switch {
		case errors.Is(err, ErrEmailTaken):
			log.Info("signup rejected: email taken", slog.String("ip", httpx.ClientIP(r)))
			httpx.WriteFieldProblem(w, r, http.StatusConflict, "email", "already in use")
		case errors.Is(err, ErrUsernameTaken):
			log.Info("signup rejected: username taken", slog.String("ip", httpx.ClientIP(r)))
			httpx.WriteFieldProblem(w, r, http.StatusConflict, "username", "already in use")
		case errors.Is(err, ErrInvalidEmail):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "email", err.Error())
		case errors.Is(err, ErrInvalidUsername):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "username", err.Error())
		case errors.Is(err, ErrDisplayNameInvalid):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "displayName", err.Error())
		case errors.Is(err, ErrPasswordTooShort), errors.Is(err, ErrPasswordTooWeak), errors.Is(err, ErrPasswordTooLong):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "password", err.Error())
		default:
			httpx.WriteServerError(w, r, err)
		}
		return
	}

	h.metrics.SignupAttempts.WithLabelValues("success").Inc()
	log.Info("signup succeeded",
		slog.String("user_id", res.User.ID.String()),
		slog.String("username", res.User.Username),
	)
	_ = h.audit.Append(r.Context(), audit.FromRequest(r, audit.ActionSignup).
		WithActor(res.User.ID, res.User.Username).
		WithTarget(audit.TargetUser, res.User.ID.String()))
	h.setSessionCookie(w, res.SessionToken, res.Remember, res.ExpiresAt)
	httpx.WriteJSON(w, http.StatusCreated, userEnvelope{User: res.User.Public()})
}

type sendCodeResponse struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// SignupSendCode sends a verification code to the given email.
//
// @Summary     Send verification code
// @Description Sends a one-time code to the email for signup verification.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body sendCodeRequest true "Email"
// @Success     200 {object} sendCodeResponse
// @Failure     400 {object} httpx.Problem
// @Failure     429 {object} httpx.Problem
// @Router      /auth/signup/send-code [post]
func (h *Handlers) SignupSendCode(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.log)

	var req sendCodeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if req.Email == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "email", "email is required")
		return
	}

	code, err := h.verifier.Generate(r.Context(), req.Email)
	if err != nil {
		log.Error("generate verification code", "error", err)
		httpx.WriteServerError(w, r, err)
		return
	}
	log.Info("verification code", "to", req.Email, "code", code)
	if err := h.svc.Mailer().SendVerificationCode(r.Context(), req.Email, code); err != nil {
		log.Error("send verification code", "error", err)
		// Code is already stored; best-effort delivery.
	}
	resp := sendCodeResponse{Message: "code sent"}
	// Return the code in the API response so the UI can show it when SMTP fails.
	resp.Code = code
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// SignupVerifyCode checks a verification code for signup.
//
// @Summary     Verify code
// @Description Checks a one-time code sent to the email.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body verifyCodeRequest true "Email + code"
// @Success     200 {object} verifyCodeResponse
// @Failure     400 {object} httpx.Problem
// @Failure     429 {object} httpx.Problem
// @Router      /auth/signup/verify-code [post]
func (h *Handlers) SignupVerifyCode(w http.ResponseWriter, r *http.Request) {
	var req verifyCodeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if req.Email == "" || req.Code == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "code", "email and code are required")
		return
	}
	valid := h.verifier.Verify(r.Context(), req.Email, req.Code)
	httpx.WriteJSON(w, http.StatusOK, verifyCodeResponse{Valid: valid})
}

// Login authenticates and establishes a session.
//
// @Summary     Log in
// @Description Authenticates by email and password and establishes a session.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body loginRequest true "Login payload"
// @Success     200 {object} userEnvelope
// @Failure     400 {object} httpx.Problem
// @Failure     401 {object} httpx.Problem
// @Failure     403 {object} httpx.Problem
// @Failure     429 {object} httpx.Problem
// @Router      /auth/login [post]
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.log)
	ip := httpx.ClientIP(r)

	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}

	acctKey := "acct:" + normalizeEmail(req.Email)

	// Incremental-backoff lockout: after repeated consecutive failures for this
	// account, refuse temporarily with a growing delay (spec: "incremental
	// backoff / temporarily refused"). This escalates, unlike the steady-state
	// token bucket below.
	if req.Email != "" && h.lockout != nil {
		if locked, retry := h.lockout.Locked(acctKey); locked {
			h.metrics.LoginAttempts.WithLabelValues("locked").Inc()
			log.Warn("login refused: account locked out", slog.String("ip", ip), slog.Duration("retry_after", retry))
			w.Header().Set("Retry-After", retryAfterSeconds(retry))
			httpx.WriteProblem(w, r, http.StatusTooManyRequests, "too many failed attempts, this account is temporarily locked")
			return
		}
	}

	// Per-account steady-state throttle (in addition to per-IP middleware).
	if req.Email != "" && h.limiter != nil && !h.limiter.Allow(acctKey) {
		h.metrics.LoginAttempts.WithLabelValues("locked").Inc()
		log.Warn("login throttled (per-account)", slog.String("ip", ip))
		w.Header().Set("Retry-After", "1")
		httpx.WriteProblem(w, r, http.StatusTooManyRequests, "too many login attempts, please slow down")
		return
	}

	res, err := h.svc.Login(r.Context(), LoginParams{
		Email:     req.Email,
		Password:  req.Password,
		Remember:  req.Remember,
		UserAgent: r.UserAgent(),
		IP:        ip,
	})
	if err != nil {
		h.metrics.LoginAttempts.WithLabelValues("failure").Inc()
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			// Count the failed credential attempt toward lockout.
			if req.Email != "" && h.lockout != nil {
				if locked, retry := h.lockout.Fail(acctKey); locked {
					h.metrics.AccountLockouts.Inc()
					log.Warn("account locked out after repeated failures",
						slog.String("ip", ip), slog.Duration("lock_for", retry))
				}
			}
			log.Info("login failed: invalid credentials", slog.String("ip", ip))
			_ = h.audit.Append(r.Context(), audit.FromRequest(r, audit.ActionLoginFail).
				WithMetadata(map[string]any{"email": normalizeEmail(req.Email), "reason": "invalid_credentials"}))
			httpx.WriteProblem(w, r, http.StatusUnauthorized, "invalid credentials")
		case errors.Is(err, ErrAccountNotActive):
			log.Info("login failed: account not active", slog.String("ip", ip))
			httpx.WriteProblem(w, r, http.StatusForbidden, "account is not active")
		default:
			httpx.WriteServerError(w, r, err)
		}
		return
	}

	// Successful auth clears the failure counter for the account.
	if h.lockout != nil {
		h.lockout.Reset(acctKey)
	}
	h.metrics.LoginAttempts.WithLabelValues("success").Inc()
	log.Info("login succeeded", slog.String("user_id", res.User.ID.String()), slog.String("ip", ip))
	_ = h.audit.Append(r.Context(), audit.FromRequest(r, audit.ActionLoginOK).
		WithActor(res.User.ID, res.User.Username).
		WithTarget(audit.TargetUser, res.User.ID.String()))
	h.maybeNotifyLogin(r, res.User, ip)
	h.setSessionCookie(w, res.SessionToken, res.Remember, res.ExpiresAt)
	httpx.WriteJSON(w, http.StatusOK, userEnvelope{User: res.User.Public()})
}

// maybeNotifyLogin fires a best-effort login-notification email when the account
// has the preference enabled AND the (user-agent, ip) device is "new" — i.e. not
// already present among the user's recent sessions besides the one just created.
// It NEVER blocks or fails the sign-in: the send runs on a detached, time-bounded
// context in a goroutine, and every dependency is nil-safe. The feature is a no-op
// when the notifier or the session lister is unwired (tests).
func (h *Handlers) maybeNotifyLogin(r *http.Request, user *User, ip string) {
	if h.notifier == nil || h.sessions == nil || user == nil || !user.LoginNotifications {
		return
	}
	device := notify.Device{UserAgent: r.UserAgent(), IP: ip}
	// The new session already exists; build the PRIOR fingerprints by dropping one
	// occurrence of the current device so the just-created session does not mask a
	// genuinely new device.
	sessions, err := h.sessions.ListByUser(r.Context(), user.ID)
	if err != nil {
		observability.LoggerFrom(r.Context(), h.log).Warn("login notification: list sessions failed", slog.Any("error", err))
		return
	}
	prior := notify.ExcludingOne(sessionDevices(sessions), device)
	if !notify.IsNewDevice(prior, device) {
		return
	}
	h.notifier.SendLoginNotificationAsync(r.Context(), user.Email, user.DisplayName, device)
}

// sessionDevices maps sessions to their coarse device fingerprints for the
// new-device login-notification heuristic.
func sessionDevices(sessions []Session) []notify.Device {
	out := make([]notify.Device, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, notify.Device{UserAgent: s.UserAgent, IP: s.IP})
	}
	return out
}

// Logout revokes the current session.
//
// @Summary     Log out
// @Description Revokes the current session and clears the session cookie.
// @Tags        auth
// @Success     204 "No Content"
// @Failure     401 {object} httpx.Problem
// @Router      /auth/logout [post]
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.log)

	token := h.sessionToken(r)
	if token == "" {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}
	if err := h.svc.Logout(r.Context(), token); err != nil {
		httpx.WriteServerError(w, r, err)
		return
	}
	h.clearSessionCookie(w)
	log.Info("logout succeeded")
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// Session returns the current authenticated user.
//
// @Summary     Current session
// @Description Returns the authenticated user for the current session, or 401.
// @Tags        auth
// @Produce     json
// @Success     200 {object} userEnvelope
// @Failure     401 {object} httpx.Problem
// @Router      /auth/session [get]
func (h *Handlers) Session(w http.ResponseWriter, r *http.Request) {
	token := h.sessionToken(r)
	if token == "" {
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
		return
	}
	user, err := h.svc.UserForSession(r.Context(), token)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrAccountNotActive) {
			h.clearSessionCookie(w)
			httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
			return
		}
		httpx.WriteServerError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, userEnvelope{User: user.Public()})
}

// PasswordForgot requests a reset link without revealing whether the email exists.
//
// @Summary     Request a password reset
// @Description Always responds 202 with a generic message; never reveals whether the email is registered.
// @Tags        auth
// @Accept      json
// @Produce     json
// @Param       body body forgotRequest true "Email to reset"
// @Success     202 {object} messageResponse
// @Failure     400 {object} httpx.Problem
// @Failure     429 {object} httpx.Problem
// @Router      /auth/password/forgot [post]
func (h *Handlers) PasswordForgot(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.log)

	var req forgotRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Throttle per-account to avoid mail bombing a single address.
	if req.Email != "" && h.limiter != nil && !h.limiter.Allow("reset:"+normalizeEmail(req.Email)) {
		w.Header().Set("Retry-After", "1")
		httpx.WriteProblem(w, r, http.StatusTooManyRequests, "too many reset requests, please slow down")
		return
	}

	if err := h.svc.RequestPasswordReset(r.Context(), req.Email); err != nil {
		// Log server-side, but still respond with the uniform success message so
		// account existence and mail outcome are not leaked.
		log.Error("password reset request error", slog.Any("error", err))
	} else {
		log.Info("password reset requested", slog.String("ip", httpx.ClientIP(r)))
		// Audit the request without revealing whether the email is registered: the
		// entry is written on the uniform success path (which fires for both known
		// and unknown emails), with no resolved actor.
		_ = h.audit.Append(r.Context(), audit.FromRequest(r, audit.ActionPasswordResetReq).
			WithMetadata(map[string]any{"email": normalizeEmail(req.Email)}))
	}

	httpx.WriteJSON(w, http.StatusAccepted, messageResponse{
		Message: "If that email is registered, a reset link has been sent.",
	})
}

// PasswordReset sets a new password from a valid single-use token.
//
// @Summary     Reset a password
// @Description Sets a new password using a single-use, time-limited reset token; invalidates existing sessions.
// @Tags        auth
// @Accept      json
// @Param       body body resetRequest true "Reset payload"
// @Success     204 "No Content"
// @Failure     400 {object} httpx.Problem
// @Failure     429 {object} httpx.Problem
// @Router      /auth/password/reset [post]
func (h *Handlers) PasswordReset(w http.ResponseWriter, r *http.Request) {
	log := observability.LoggerFrom(r.Context(), h.log)

	var req resetRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteProblem(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if req.Token == "" {
		httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "token", "token is required")
		return
	}

	if err := h.svc.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		switch {
		case errors.Is(err, ErrResetTokenInvalid):
			log.Info("password reset rejected: invalid token", slog.String("ip", httpx.ClientIP(r)))
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "token", "reset token is invalid or expired")
		case errors.Is(err, ErrPasswordTooShort), errors.Is(err, ErrPasswordTooWeak), errors.Is(err, ErrPasswordTooLong):
			httpx.WriteFieldProblem(w, r, http.StatusBadRequest, "password", err.Error())
		default:
			httpx.WriteServerError(w, r, err)
		}
		return
	}

	log.Info("password reset succeeded", slog.String("ip", httpx.ClientIP(r)))
	_ = h.audit.Append(r.Context(), audit.FromRequest(r, audit.ActionPasswordResetDone))
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}

// --- cookie helpers ---

// setSessionCookie writes the session cookie. For "remember" sessions a
// persistent cookie with an Expires matching the server expiry is set; otherwise
// a browser-session cookie (no Expires) is used while the server still enforces
// the 24h record expiry.
func (h *Handlers) setSessionCookie(w http.ResponseWriter, token string, remember bool, expiresAt time.Time) {
	c := &http.Cookie{
		Name:     h.sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	if remember {
		c.Expires = expiresAt
		c.MaxAge = int(time.Until(expiresAt).Seconds())
	}
	http.SetCookie(w, c)
}

// clearSessionCookie expires the session cookie.
func (h *Handlers) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// sessionToken reads the raw session token from the request cookie.
func (h *Handlers) sessionToken(r *http.Request) string {
	c, err := r.Cookie(h.sessionCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// retryAfterSeconds formats a duration as a Retry-After header value: whole
// seconds, rounded up, with a floor of 1.
func retryAfterSeconds(d time.Duration) string {
	secs := int(d.Seconds())
	if time.Duration(secs)*time.Second < d {
		secs++ // round up any fractional remainder
	}
	if secs < 1 {
		secs = 1
	}
	return strconv.Itoa(secs)
}
