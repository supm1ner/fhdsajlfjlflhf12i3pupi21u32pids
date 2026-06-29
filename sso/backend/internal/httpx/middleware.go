package httpx

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/cors"

	"cotton-id/internal/observability"
)

// SecurityHeaders applies the cotton-id browser-facing security header baseline
// to every response: a strict Content-Security-Policy, nosniff, a privacy
// Referrer-Policy, and clickjacking protection via X-Frame-Options +
// frame-ancestors. The CSP intentionally allows the inline styles/fonts the
// design system relies on while forbidding object/base and framing.
//
// When secure is true (production over TLS), it also emits HSTS so browsers
// refuse to downgrade to http and an active network attacker cannot strip TLS
// before the Secure cookie attribute can protect the session/CSRF cookies. HSTS
// is intentionally NOT sent in dev (where the IdP is served over http) since a
// stray max-age would pin localhost to https.
func SecurityHeaders(secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			// frame-ancestors 'none' + X-Frame-Options DENY: cotton-id is never framed.
			h.Set("Content-Security-Policy",
				"default-src 'self'; "+
					"img-src 'self' data: https:; "+
					"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
					"font-src 'self' https://fonts.gstatic.com data:; "+
					"script-src 'self'; "+
					"connect-src 'self'; "+
					"object-src 'none'; "+
					"base-uri 'none'; "+
					"frame-ancestors 'none'")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			if secure {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS builds a CORS middleware permitting the SPA origin to make
// credentialed requests and to read/send the CSRF header. allowedOrigin is the
// FRONTEND_BASE_URL.
func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	return cors.Handler(cors.Options{
		AllowedOrigins:   []string{allowedOrigin},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Content-Type", CSRFHeaderName, "X-Admin-Key", observability.RequestIDHeader},
		ExposedHeaders:   []string{observability.RequestIDHeader},
		AllowCredentials: true,
		MaxAge:           300,
	})
}

// Recoverer converts a panic in a downstream handler into a logged 500
// problem+json response instead of dropping the connection. The stack trace
// stays server-side.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// http.ErrAbortHandler is the documented sentinel for an
					// intentional abort; re-panic so the server handles it.
					if rec == http.ErrAbortHandler {
						panic(rec)
					}
					l := observability.LoggerFrom(r.Context(), log)
					l.Error("panic recovered",
						slog.Any("panic", rec),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
					)
					WriteProblem(w, r, http.StatusInternalServerError, "an unexpected error occurred")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
