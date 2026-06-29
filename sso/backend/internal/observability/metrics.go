package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics bundles cotton-id's Prometheus registry and the application metrics.
// A dedicated (non-default) registry keeps the surface explicit and testable.
type Metrics struct {
	Registry *prometheus.Registry

	// RequestDuration is the HTTP server latency histogram, labeled by route
	// (the chi route pattern, to bound cardinality) method and status code.
	RequestDuration *prometheus.HistogramVec

	// Security event counters seeding the later audit dashboards.
	LoginAttempts    *prometheus.CounterVec // labels: result=success|failure|locked
	SignupAttempts   *prometheus.CounterVec // labels: result=success|failure
	ConsentDecisions *prometheus.CounterVec // labels: decision=accept|reject

	// SocialLogins counts social (OAuth/OIDC) callback outcomes by provider and
	// result, so the dashboards can break login success/failure down per provider.
	SocialLogins *prometheus.CounterVec // labels: provider, result=success|failure
	// PasskeyLogins counts passkey (WebAuthn) login-finish outcomes by result.
	PasskeyLogins *prometheus.CounterVec // labels: result=success|failure
	// AccountLockouts counts the times an account lockout is ENGAGED (a failure
	// crosses the threshold and the account is locked), distinct from the
	// LoginAttempts{result="locked"} counter that also fires on each refused
	// attempt while already locked.
	AccountLockouts prometheus.Counter
}

// NewMetrics creates the registry, registers Go runtime + process collectors and
// the application metrics, and returns the bundle.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		Registry: reg,
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "method", "status"}),
		LoginAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cotton_login_attempts_total",
			Help: "Login attempts by result.",
		}, []string{"result"}),
		SignupAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cotton_signup_attempts_total",
			Help: "Signup attempts by result.",
		}, []string{"result"}),
		ConsentDecisions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cotton_consent_decisions_total",
			Help: "OIDC consent decisions by outcome.",
		}, []string{"decision"}),
		SocialLogins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cotton_social_logins_total",
			Help: "Social (OAuth/OIDC) login outcomes by provider and result.",
		}, []string{"provider", "result"}),
		PasskeyLogins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "cotton_passkey_logins_total",
			Help: "Passkey (WebAuthn) login outcomes by result.",
		}, []string{"result"}),
		AccountLockouts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cotton_account_lockouts_total",
			Help: "Number of times an account lockout was engaged (threshold crossed).",
		}),
	}

	reg.MustRegister(
		m.RequestDuration, m.LoginAttempts, m.SignupAttempts, m.ConsentDecisions,
		m.SocialLogins, m.PasskeyLogins, m.AccountLockouts,
	)
	return m
}

// Handler returns the /metrics HTTP handler bound to this registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{Registry: m.Registry})
}

// statusRecorder captures the response status for the duration histogram.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Write ensures status defaults to 200 when a handler writes a body without an
// explicit WriteHeader.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Middleware records each request's latency into RequestDuration. The route
// label uses the matched chi route pattern (falling back to the raw path) to
// avoid unbounded label cardinality from path parameters.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 0}

		next.ServeHTTP(rec, r)

		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = r.URL.Path
		}
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		m.RequestDuration.WithLabelValues(route, r.Method, strconv.Itoa(status)).
			Observe(time.Since(start).Seconds())
	})
}
