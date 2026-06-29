package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"cotton-id/internal/auth"
	"cotton-id/internal/observability"
)

// stubSessions is a SessionVerifier returning a fixed user for a known token.
type stubSessions struct {
	token string
	user  *auth.User
	err   error
}

func (s *stubSessions) UserForSession(_ context.Context, token string) (*auth.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	if token != s.token {
		return nil, auth.ErrSessionNotFound
	}
	return s.user, nil
}

func testDeps(hydraURL string, sessions SessionVerifier) Deps {
	return Deps{
		Logger:            observability.NewLogger("error"),
		Metrics:           observability.NewMetrics(),
		Sessions:          sessions,
		HydraAdminURL:     hydraURL,
		HydraPublicURL:    "http://hydra:4444",
		FrontendBaseURL:   "http://localhost:3000",
		PublicBaseURL:     "http://localhost:8080",
		SessionCookieName: "cid_session",
	}
}

func mountAPI(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) {
		Mount(api, deps)
	})
	return r
}

func mountBrowser(deps Deps) http.Handler {
	r := chi.NewRouter()
	MountBrowser(r, deps)
	return r
}

func TestLoginAccept_RequiresSession(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusOK, `{"redirect_to":"x"}`)
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: testUser()})
	h := mountAPI(deps)

	// No session cookie → 401.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/login/accept",
		strings.NewReader(`{"loginChallenge":"abc"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLoginAccept_WithSession(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK, `{"redirect_to":"http://hydra/continue"}`)
	user := testUser()
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: user})
	h := mountAPI(deps)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/login/accept",
		strings.NewReader(`{"loginChallenge":"abc"}`))
	req.AddCookie(&http.Cookie{Name: "cid_session", Value: "good"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var out redirectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.RedirectTo != "http://hydra/continue" {
		t.Errorf("redirectTo = %q", out.RedirectTo)
	}
	// The subject sent to Hydra must be the stable account id.
	if cap.Body["subject"] != user.ID.String() {
		t.Errorf("subject = %v, want %s", cap.Body["subject"], user.ID.String())
	}
}

func TestLoginAccept_MissingChallenge(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusOK, `{}`)
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: testUser()})
	h := mountAPI(deps)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/login/accept", strings.NewReader(`{}`))
	req.AddCookie(&http.Cookie{Name: "cid_session", Value: "good"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestConsentDetails(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusOK,
		`{"challenge":"k","requested_scope":["openid","email"],"client":{"client_id":"demo","client_name":"Demo App"}}`)
	user := testUser()
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: user})
	h := mountAPI(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/consent?consent_challenge=k", nil)
	req.AddCookie(&http.Cookie{Name: "cid_session", Value: "good"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var out consentDetailsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Client.ID != "demo" || out.Client.Name != "Demo App" {
		t.Errorf("client mismatch: %+v", out.Client)
	}
	if len(out.RequestedScopes) != 2 {
		t.Errorf("requested scopes mismatch: %+v", out.RequestedScopes)
	}
	if out.User.ID != user.ID.String() {
		t.Errorf("user id mismatch: %s", out.User.ID)
	}
}

func TestConsentDetails_RequiresSession(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusOK, `{}`)
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: testUser()})
	h := mountAPI(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/consent?consent_challenge=k", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// TestConsentAccept_ClampsScopes verifies the accept never grants a scope the
// client did not request, even if the SPA posts a wider set.
func TestConsentAccept_ClampsScopes(t *testing.T) {
	// Hydra reports requested_scope = [openid, email]; the client did not request
	// "admin". The fake returns the consent request for GET and a redirect on PUT.
	cap := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"challenge":"k","requested_scope":["openid","email"],"client":{"client_id":"demo"}}`))
			return
		}
		// PUT accept: capture the granted scopes.
		cap.Method = r.Method
		cap.Path = r.URL.Path
		cap.Body = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&cap.Body)
		_, _ = w.Write([]byte(`{"redirect_to":"http://hydra/code"}`))
	}))
	defer srv.Close()

	user := testUser()
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: user})
	h := mountAPI(deps)

	// SPA maliciously/erroneously posts "admin" which was never requested.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/consent/accept",
		strings.NewReader(`{"consentChallenge":"k","grantScopes":["openid","email","admin"],"remember":true}`))
	req.AddCookie(&http.Cookie{Name: "cid_session", Value: "good"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	gs, _ := cap.Body["grant_scope"].([]any)
	got := make([]string, 0, len(gs))
	for _, s := range gs {
		got = append(got, s.(string))
	}
	if len(got) != 2 || got[0] != "openid" || got[1] != "email" {
		t.Fatalf("granted scopes = %v, want [openid email] (admin must be dropped)", got)
	}
	// remember should propagate with a remember_for.
	if cap.Body["remember"] != true {
		t.Errorf("remember not propagated")
	}
	// ID token claims must carry email (granted) but not be widened.
	sess, _ := cap.Body["session"].(map[string]any)
	idt, _ := sess["id_token"].(map[string]any)
	if idt["sub"] != user.ID.String() {
		t.Errorf("id_token.sub = %v, want %s", idt["sub"], user.ID.String())
	}
	if idt["email"] != user.Email {
		t.Errorf("id_token.email = %v, want %s", idt["email"], user.Email)
	}
}

// TestLoginReject verifies a user-cancelled sign-in rejects the Hydra login
// challenge with access_denied (oidc-provider "failed authentication rejects the
// challenge" scenario).
func TestLoginReject(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK, `{"redirect_to":"http://rp/error?error=access_denied"}`)
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: testUser()})
	h := mountAPI(deps)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/login/reject",
		strings.NewReader(`{"loginChallenge":"abc"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if cap.Body["error"] != "access_denied" {
		t.Errorf("reject error = %v, want access_denied", cap.Body["error"])
	}
	var out redirectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(out.RedirectTo, "access_denied") {
		t.Errorf("redirectTo = %q, want RP error URL", out.RedirectTo)
	}
}

func TestLoginReject_MissingChallenge(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusOK, `{}`)
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: testUser()})
	h := mountAPI(deps)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/login/reject", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestConsentReject(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK, `{"redirect_to":"http://hydra/denied"}`)
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: testUser()})
	h := mountAPI(deps)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/consent/reject",
		strings.NewReader(`{"consentChallenge":"k"}`))
	req.AddCookie(&http.Cookie{Name: "cid_session", Value: "good"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if cap.Body["error"] != "access_denied" {
		t.Errorf("reject error = %v, want access_denied", cap.Body["error"])
	}
}

// TestLoginBrowser_NoSessionRedirectsToSPA verifies the unauthenticated browser
// entry redirects to the SPA login carrying the challenge.
func TestLoginBrowser_NoSessionRedirectsToSPA(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusOK, `{"challenge":"c","skip":false}`)
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: testUser()})
	h := mountBrowser(deps)

	req := httptest.NewRequest(http.MethodGet, "/oauth/login?login_challenge=c", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "http://localhost:3000/login?login_challenge=c") {
		t.Errorf("Location = %q, want SPA login with challenge", loc)
	}
}

// TestLoginBrowser_WithSessionAutoAccepts verifies an already-authenticated
// browser is accepted immediately with the stable subject and 302'd to Hydra.
func TestLoginBrowser_WithSessionAutoAccepts(t *testing.T) {
	cap := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"challenge":"c","skip":false}`))
			return
		}
		cap.Body = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&cap.Body)
		_, _ = w.Write([]byte(`{"redirect_to":"http://hydra/continue"}`))
	}))
	defer srv.Close()

	user := testUser()
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: user})
	h := mountBrowser(deps)

	req := httptest.NewRequest(http.MethodGet, "/oauth/login?login_challenge=c", nil)
	req.AddCookie(&http.Cookie{Name: "cid_session", Value: "good"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (body %s)", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "http://hydra/continue" {
		t.Errorf("Location = %q, want hydra continue", rec.Header().Get("Location"))
	}
	if cap.Body["subject"] != user.ID.String() {
		t.Errorf("subject = %v, want %s", cap.Body["subject"], user.ID.String())
	}
}

func TestLoginBrowser_MissingChallenge(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusOK, `{}`)
	deps := testDeps(srv.URL, &stubSessions{token: "good", user: testUser()})
	h := mountBrowser(deps)

	req := httptest.NewRequest(http.MethodGet, "/oauth/login", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
