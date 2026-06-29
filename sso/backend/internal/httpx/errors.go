// Package httpx holds cotton-id's HTTP plumbing: the chi router factory,
// security/CORS/recovery middleware, RFC 7807 problem+json errors and JSON
// render helpers, the CSRF double-submit middleware, the auth-route rate
// limiter, and the /healthz handler. Domain packages (auth, oidc, adminapi)
// mount their routes onto the router this package builds and use these helpers
// for uniform responses.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"cotton-id/internal/observability"
)

// Problem is an RFC 7807 problem+json document. It is the single error shape for
// every cotton-id API response.
type Problem struct {
	// Type is a URI reference identifying the problem type. "about:blank" is
	// used when only the status conveys meaning.
	Type string `json:"type" example:"about:blank"`
	// Title is a short, human-readable summary of the problem type.
	Title string `json:"title" example:"Bad Request"`
	// Status is the HTTP status code.
	Status int `json:"status" example:"400"`
	// Detail is a human-readable explanation specific to this occurrence.
	Detail string `json:"detail,omitempty" example:"email is required"`
	// Instance optionally identifies the specific occurrence (the request path).
	Instance string `json:"instance,omitempty"`
	// Field optionally names the offending request field for validation errors.
	Field string `json:"field,omitempty" example:"email"`
}

// ProblemContentType is the media type for problem responses.
const ProblemContentType = "application/problem+json"

// problemLogger is set by the router factory so error helpers can log 5xx
// responses with the request id. It is package-level and read-only after setup.
var problemLogger *slog.Logger = slog.Default()

// SetLogger configures the logger used to record server errors. Called once by
// NewRouter.
func SetLogger(l *slog.Logger) {
	if l != nil {
		problemLogger = l
	}
}

// WriteProblem renders an RFC 7807 problem+json response. Detail is sent to the
// client as-is, so callers must never put internal/sensitive text in it for 5xx
// errors — use WriteServerError for those.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	writeProblem(w, r, Problem{
		Type:     "about:blank",
		Title:    http.StatusText(status),
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
	})
}

// WriteFieldProblem renders a validation problem naming the offending field.
func WriteFieldProblem(w http.ResponseWriter, r *http.Request, status int, field, detail string) {
	writeProblem(w, r, Problem{
		Type:     "about:blank",
		Title:    http.StatusText(status),
		Status:   status,
		Detail:   detail,
		Instance: r.URL.Path,
		Field:    field,
	})
}

// WriteServerError logs the underlying error (with the request id) and returns a
// generic 500 problem to the client, never leaking internal detail.
func WriteServerError(w http.ResponseWriter, r *http.Request, err error) {
	log := observability.LoggerFrom(r.Context(), problemLogger)
	log.Error("internal server error",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Any("error", err),
	)
	writeProblem(w, r, Problem{
		Type:     "about:blank",
		Title:    http.StatusText(http.StatusInternalServerError),
		Status:   http.StatusInternalServerError,
		Detail:   "an unexpected error occurred",
		Instance: r.URL.Path,
	})
}

func writeProblem(w http.ResponseWriter, _ *http.Request, p Problem) {
	w.Header().Set("Content-Type", ProblemContentType)
	w.WriteHeader(p.Status)
	// Best-effort: if encoding fails the status line has already been written.
	_ = json.NewEncoder(w).Encode(p)
}
