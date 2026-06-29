package httpx

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cotton-id/internal/observability"
)

// RouterDeps carries the dependencies the base router needs.
type RouterDeps struct {
	Logger          *slog.Logger
	Metrics         *observability.Metrics
	FrontendBaseURL string
	// RealIP resolves the trusted client IP from the request (see realip.go).
	RealIP *RealIP
	// Secure enables HSTS (production over TLS).
	Secure bool
}

// NewRouter builds the base chi router with the global middleware stack applied
// in order: real-IP → request-id → security headers → CORS → recovery → metrics.
// Domain packages mount their routes onto the returned router. Per-route concerns
// (CSRF, rate limiting, auth) are applied by those packages on their own subtrees.
func NewRouter(deps RouterDeps) *chi.Mux {
	SetLogger(deps.Logger)

	r := chi.NewRouter()
	// Resolve the real client IP first so rate limiting and audit logging key off
	// a trusted value, not a spoofable header.
	if deps.RealIP != nil {
		r.Use(deps.RealIP.Middleware)
	}
	r.Use(observability.RequestIDMiddleware)
	r.Use(SecurityHeaders(deps.Secure))
	r.Use(CORS(deps.FrontendBaseURL))
	r.Use(Recoverer(deps.Logger))
	if deps.Metrics != nil {
		r.Use(deps.Metrics.Middleware)
	}
	return r
}

// HealthChecker reports the health of a named dependency.
type HealthChecker interface {
	// Name identifies the dependency in the health report.
	Name() string
	// Check returns nil when the dependency is reachable.
	Check(ctx context.Context) error
}

// healthCheckerFunc adapts a function to HealthChecker.
type healthCheckerFunc struct {
	name string
	fn   func(ctx context.Context) error
}

func (h healthCheckerFunc) Name() string                    { return h.name }
func (h healthCheckerFunc) Check(ctx context.Context) error { return h.fn(ctx) }

// NewHealthChecker wraps a name and check function as a HealthChecker.
func NewHealthChecker(name string, fn func(ctx context.Context) error) HealthChecker {
	return healthCheckerFunc{name: name, fn: fn}
}

// healthResponse is the /healthz body.
type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// HealthzHandler returns the /healthz handler. It reports 200 when every
// dependency is reachable and 503 otherwise, with a per-dependency report.
//
// @Summary     Liveness/readiness probe
// @Description Reports whether the backend can reach its critical dependencies.
// @Tags        ops
// @Produce     json
// @Success     200 {object} healthResponse
// @Failure     503 {object} healthResponse
// @Router      /healthz [get]
func HealthzHandler(checkers ...HealthChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := healthResponse{Status: "ok", Checks: make(map[string]string)}
		healthy := true
		for _, c := range checkers {
			if err := c.Check(r.Context()); err != nil {
				healthy = false
				resp.Checks[c.Name()] = "unhealthy: " + err.Error()
			} else {
				resp.Checks[c.Name()] = "ok"
			}
		}
		if !healthy {
			resp.Status = "degraded"
			WriteJSON(w, http.StatusServiceUnavailable, resp)
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}
