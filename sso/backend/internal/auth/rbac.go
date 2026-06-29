package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"cotton-id/internal/httpx"
	"cotton-id/internal/observability"
)

// Account role values, ordered by privilege (user < admin < owner). These are
// the role strings stored in users.role.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
	RoleOwner = "owner"
)

// roleRank maps a role to its privilege rank. Unknown roles rank as 0 (below
// user) so an unexpected value fails closed rather than granting access.
func roleRank(role string) int {
	switch role {
	case RoleOwner:
		return 3
	case RoleAdmin:
		return 2
	case RoleUser:
		return 1
	default:
		return 0
	}
}

// RoleAtLeast reports whether have is at least as privileged as min on the
// user < admin < owner ladder. It is the single source of truth for role
// comparisons so handlers and middleware agree.
func RoleAtLeast(have, min string) bool {
	return roleRank(have) >= roleRank(min)
}

// SessionResolver resolves a raw session-cookie token to its active user. It is
// the seam RequireRole uses; *Service satisfies it via UserForSession, so the
// middleware depends on this narrow interface rather than the whole service.
type SessionResolver interface {
	UserForSession(ctx context.Context, token string) (*User, error)
}

// ensure *Service implements the seam.
var _ SessionResolver = (*Service)(nil)

// userCtxKey is the context key under which RequireRole stashes the resolved
// admin user for downstream handlers.
type userCtxKey struct{}

// UserFromContext returns the user RequireRole resolved and stashed on the
// request context, or (nil, false) when none is present. Admin handlers use it
// to identify the acting user (e.g. for audit + self-action guards) without
// re-resolving the session.
func UserFromContext(ctx context.Context) (*User, bool) {
	u, ok := ctx.Value(userCtxKey{}).(*User)
	return u, ok
}

// withUser returns a copy of ctx carrying the resolved user.
func withUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userCtxKey{}, u)
}

// RequireRole returns middleware that gates a route on a minimum role. It reads
// the session cookie, resolves the active user via resolver, and:
//   - responds 401 problem+json when there is no valid active session, and
//   - responds 403 problem+json when the user's role is below min.
//
// On success it stashes the resolved user on the request context (retrievable
// via UserFromContext) and calls the next handler. The client-side gate is UX
// only; this server-side check is authoritative (design "Risks").
func RequireRole(min string, resolver SessionResolver, sessionCookieName string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ""
			if c, err := r.Cookie(sessionCookieName); err == nil {
				token = c.Value
			}
			if token == "" {
				httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
				return
			}

			user, err := resolver.UserForSession(r.Context(), token)
			if err != nil {
				// The expected "no live active session" cases are a clean 401.
				if errors.Is(err, ErrSessionNotFound) ||
					errors.Is(err, ErrUserNotFound) ||
					errors.Is(err, ErrAccountNotActive) {
					httpx.WriteProblem(w, r, http.StatusUnauthorized, "not authenticated")
					return
				}
				httpx.WriteServerError(w, r, err)
				return
			}

			if !RoleAtLeast(user.Role, min) {
				if log != nil {
					observability.LoggerFrom(r.Context(), log).Warn("admin authorization denied",
						slog.String("user_id", user.ID.String()),
						slog.String("role", user.Role),
						slog.String("required", min),
						slog.String("ip", httpx.ClientIP(r)),
						slog.String("path", r.URL.Path),
					)
				}
				httpx.WriteProblem(w, r, http.StatusForbidden, "insufficient privileges")
				return
			}

			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
		})
	}
}
