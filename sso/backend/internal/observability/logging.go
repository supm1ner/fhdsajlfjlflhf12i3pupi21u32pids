// Package observability provides cotton-id's structured logging and Prometheus
// metrics. It exposes a JSON slog logger, a request-id middleware that
// correlates log lines with a per-request id, an HTTP duration histogram
// middleware, application security counters, and the /metrics handler.
package observability

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/google/uuid"
)

// requestIDKey is the context key under which the per-request id is stored.
type ctxKey int

const requestIDKey ctxKey = iota

// RequestIDHeader is the response header carrying the correlation id.
const RequestIDHeader = "X-Request-Id"

// NewLogger builds a JSON slog.Logger at the given level (debug|info|warn|error)
// writing to stdout. Unknown levels default to info.
func NewLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}

// RequestID extracts the correlation id stored on the request context. It
// returns "" when none is present.
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// withRequestID returns a copy of ctx carrying the request id.
func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// validRequestID bounds a reused inbound correlation id to a safe charset and
// length so an attacker cannot pollute or forge the audit trail with oversized
// or control-character values. Trusted callers passing a UUID-like id still work.
func validRequestID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.':
		default:
			return false
		}
	}
	return true
}

// RequestIDMiddleware assigns each request a correlation id, reusing an inbound
// X-Request-Id only when it passes [validRequestID] (otherwise a fresh UUID is
// generated), stores it on the context, and echoes it on the response so clients
// and logs can correlate.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if !validRequestID(id) {
			id = uuid.NewString()
		}
		ctx := withRequestID(r.Context(), id)
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggerFrom returns base with the request's correlation id attached, so handler
// log lines are automatically correlated.
func LoggerFrom(ctx context.Context, base *slog.Logger) *slog.Logger {
	if id := RequestID(ctx); id != "" {
		return base.With(slog.String("request_id", id))
	}
	return base
}
