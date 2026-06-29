package admin

import (
	"context"

	"github.com/go-chi/chi/v5"

	"cotton-id/internal/auth"
	"cotton-id/internal/oidc"
)

// Mount builds the admin Handlers from deps and registers the console routes on
// r (the /api/v1 subrouter). The caller is responsible for wrapping r in the
// role gate (auth.RequireRole(auth.RoleAdmin, ...)) and the CSRF middleware
// before calling Mount; this keeps the composition (which middleware, in what
// order) in main.go, matching the other domain packages.
func Mount(r chi.Router, deps Deps) {
	NewHandlers(deps).Mount(r)
}

// MountConsole registers the console routes on r WITHOUT the "/admin" prefix, so
// main.go can own a single "/admin" subtree shared with the machine client API.
// The caller wraps r in RequireRole(admin) + CSRF.
func MountConsole(r chi.Router, deps Deps) {
	NewHandlers(deps).MountInto(r)
}

// NewService wiring convenience: BuildService assembles the lifecycle Service
// from the concrete stores main.go already constructs, so the composition root
// does not need to know the Service's internal seams.
func BuildService(users *auth.UserStore, store *Store, sessions *auth.SessionStore, resets resetIssuer, hydra hydraRevoker) *Service {
	return NewService(ServiceDeps{
		Users:    users,
		Owners:   store,
		Sessions: sessions,
		Resets:   resets,
		Hydra:    hydra,
	})
}

// HydraServicesCounter adapts an *oidc.HydraClient to the servicesCounter seam:
// it counts registered OAuth2 clients for the overview's "services" stat.
type HydraServicesCounter struct {
	Client *oidc.HydraClient
}

// CountServices returns the number of registered OAuth2 clients.
func (c HydraServicesCounter) CountServices(ctx context.Context) (int, error) {
	clients, err := c.Client.ListClients(ctx)
	if err != nil {
		return 0, err
	}
	return len(clients), nil
}

// ensure the concrete stores satisfy the Service seams (compile-time check).
var (
	_ userStore      = (*auth.UserStore)(nil)
	_ ownerCounter   = (*Store)(nil)
	_ sessionRevoker = (*auth.SessionStore)(nil)
	_ resetIssuer    = (*auth.Service)(nil)
	_ hydraRevoker   = (*oidc.HydraClient)(nil)
)
