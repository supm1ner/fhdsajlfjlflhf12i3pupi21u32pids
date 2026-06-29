package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cotton-id/internal/observability"
)

func TestRoleAtLeast(t *testing.T) {
	cases := []struct {
		have, min string
		want      bool
	}{
		{RoleUser, RoleUser, true},
		{RoleUser, RoleAdmin, false},
		{RoleUser, RoleOwner, false},
		{RoleAdmin, RoleUser, true},
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleOwner, false},
		{RoleOwner, RoleUser, true},
		{RoleOwner, RoleAdmin, true},
		{RoleOwner, RoleOwner, true},
		// Unknown roles fail closed (rank below user).
		{"", RoleUser, false},
		{"superuser", RoleAdmin, false},
	}
	for _, c := range cases {
		if got := RoleAtLeast(c.have, c.min); got != c.want {
			t.Errorf("RoleAtLeast(%q, %q) = %v, want %v", c.have, c.min, got, c.want)
		}
	}
}

// stubResolver is a SessionResolver returning a fixed user/error for a token.
type stubResolver struct {
	user *User
	err  error
}

func (s stubResolver) UserForSession(_ context.Context, _ string) (*User, error) {
	return s.user, s.err
}

func newReq(token string) *http.Request {
	r := httptest.NewRequest("GET", "/api/v1/admin/overview", nil)
	if token != "" {
		r.AddCookie(&http.Cookie{Name: "cid_session", Value: token})
	}
	return r
}

// guardedHandler wraps a 200-OK handler with RequireRole, capturing the user the
// middleware stashed on the context.
func guardedHandler(min string, resolver SessionResolver) (http.Handler, *captured) {
	cap := &captured{}
	mw := RequireRole(min, resolver, "cid_session", observability.NewLogger("error"))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := UserFromContext(r.Context()); ok {
			cap.user = u
		}
		w.WriteHeader(http.StatusOK)
	}))
	// Mount on chi so request context is realistic.
	r := chi.NewRouter()
	r.Handle("/api/v1/admin/overview", h)
	return r, cap
}

type captured struct{ user *User }

func TestRequireRoleUnauthenticatedNoCookie(t *testing.T) {
	h, _ := guardedHandler(RoleAdmin, stubResolver{user: &User{Role: RoleAdmin}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("")) // no session cookie
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireRoleUnauthenticatedBadSession(t *testing.T) {
	h, _ := guardedHandler(RoleAdmin, stubResolver{err: ErrSessionNotFound})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("tok"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireRoleForbiddenBelowMin(t *testing.T) {
	h, _ := guardedHandler(RoleAdmin, stubResolver{user: &User{ID: uuid.New(), Role: RoleUser}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("tok"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireRoleAllowsAtMinAndStashesUser(t *testing.T) {
	id := uuid.New()
	h, cap := guardedHandler(RoleAdmin, stubResolver{user: &User{ID: id, Role: RoleAdmin}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("tok"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if cap.user == nil || cap.user.ID != id {
		t.Fatalf("expected resolved user %v stashed on context, got %v", id, cap.user)
	}
}

func TestRequireRoleOwnerOnlyGate(t *testing.T) {
	// An admin is below an owner-only gate → 403.
	h, _ := guardedHandler(RoleOwner, stubResolver{user: &User{ID: uuid.New(), Role: RoleAdmin}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq("tok"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin under owner gate: status = %d, want 403", rec.Code)
	}

	// An owner passes.
	h2, _ := guardedHandler(RoleOwner, stubResolver{user: &User{ID: uuid.New(), Role: RoleOwner}})
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, newReq("tok"))
	if rec2.Code != http.StatusOK {
		t.Fatalf("owner under owner gate: status = %d, want 200", rec2.Code)
	}
}

func TestUserFromContextAbsent(t *testing.T) {
	if u, ok := UserFromContext(context.Background()); ok || u != nil {
		t.Fatalf("UserFromContext on empty ctx = (%v, %v), want (nil, false)", u, ok)
	}
}
