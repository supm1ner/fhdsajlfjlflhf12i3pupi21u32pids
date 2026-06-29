package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"cotton-id/internal/adminapi"
	"cotton-id/internal/auth"
	"cotton-id/internal/observability"
	"cotton-id/internal/oidc"
)

// --- fakes ------------------------------------------------------------------

// fakeClientManager is an in-memory clientManager for the Services handler tests.
// It records the last create/update and the captured Hydra representations so the
// tests can assert the adminapi mapping and secret-once behavior.
type fakeClientManager struct {
	clients map[string]oidc.OAuth2Client
	// consent[subject] = clientIDs the subject has granted.
	consent map[string][]string

	createResp   oidc.OAuth2Client
	createErr    error
	getErr       error
	updateErr    error
	deleteErr    error
	lastCreated  oidc.OAuth2Client
	lastUpdated  oidc.OAuth2Client
	lastUpdateID string
	revoked      []revokePair
}

type revokePair struct{ subject, client string }

func newFakeClientManager() *fakeClientManager {
	return &fakeClientManager{clients: map[string]oidc.OAuth2Client{}, consent: map[string][]string{}}
}

func (f *fakeClientManager) ListClients(context.Context) ([]oidc.OAuth2Client, error) {
	out := make([]oidc.OAuth2Client, 0, len(f.clients))
	for _, c := range f.clients {
		out = append(out, c)
	}
	return out, nil
}

func (f *fakeClientManager) GetClient(_ context.Context, id string) (*oidc.OAuth2Client, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	c, ok := f.clients[id]
	if !ok {
		return nil, &oidc.HydraError{Op: "get client", StatusCode: http.StatusNotFound, Body: "not found"}
	}
	cp := c
	return &cp, nil
}

func (f *fakeClientManager) CreateClient(_ context.Context, client oidc.OAuth2Client) (*oidc.OAuth2Client, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.lastCreated = client
	resp := f.createResp
	if resp.ClientID == "" {
		resp.ClientID = "generated-id"
	}
	// Persist the created record (secret is NOT stored back on the read view).
	stored := client
	stored.ClientID = resp.ClientID
	stored.ClientSecret = ""
	f.clients[resp.ClientID] = stored
	return &resp, nil
}

func (f *fakeClientManager) UpdateClient(_ context.Context, id string, client oidc.OAuth2Client) (*oidc.OAuth2Client, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.lastUpdated = client
	f.lastUpdateID = id
	f.clients[id] = client
	cp := client
	return &cp, nil
}

func (f *fakeClientManager) DeleteClient(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.clients, id)
	return nil
}

func (f *fakeClientManager) ListConsentSessions(_ context.Context, subject string) ([]oidc.ConsentSessionRecord, error) {
	var out []oidc.ConsentSessionRecord
	for _, cid := range f.consent[subject] {
		cid := cid
		out = append(out, oidc.ConsentSessionRecord{
			ConsentRequest: &oidc.ConsentRequest{Subject: subject, Client: &oidc.OAuth2Client{ClientID: cid}},
		})
	}
	return out, nil
}

func (f *fakeClientManager) RevokeConsentSessions(_ context.Context, subject, client string) error {
	f.revoked = append(f.revoked, revokePair{subject, client})
	// Drop the grant from the in-memory model.
	kept := f.consent[subject][:0]
	for _, cid := range f.consent[subject] {
		if cid != client {
			kept = append(kept, cid)
		}
	}
	f.consent[subject] = kept
	return nil
}

// fakeSubjects is an in-memory subjectLister.
type fakeSubjects struct {
	ids      []uuid.UUID
	complete bool
}

func (f fakeSubjects) ListSubjectIDs(_ context.Context, limit int) ([]uuid.UUID, bool, error) {
	if len(f.ids) > limit {
		return f.ids[:limit], false, nil
	}
	return f.ids, f.complete, nil
}

// stubResolver resolves any non-empty token to the configured user — used to
// drive auth.RequireRole so the handlers see a real acting user on the context
// exactly as in production.
type stubResolver struct {
	user *auth.User
	err  error
}

func (s stubResolver) UserForSession(context.Context, string) (*auth.User, error) {
	return s.user, s.err
}

// mountServices mounts the Services handlers behind auth.RequireRole(admin) with
// the given resolver, mirroring main.go's wiring (sans CSRF, which is exercised
// elsewhere). Returns the handler and the fake client manager.
func mountServices(t *testing.T, resolver auth.SessionResolver, fc *fakeClientManager, subs subjectLister) http.Handler {
	t.Helper()
	h := NewHandlers(Deps{
		Logger:   observability.NewLogger("error"),
		Metrics:  observability.NewMetrics(),
		Clients:  fc,
		Subjects: subs,
	})
	r := chi.NewRouter()
	r.Route("/api/v1/admin", func(adm chi.Router) {
		adm.Use(auth.RequireRole(auth.RoleAdmin, resolver, "cid_session", observability.NewLogger("error")))
		h.MountInto(adm)
	})
	return r
}

func adminResolver() auth.SessionResolver {
	return stubResolver{user: &auth.User{ID: uuid.New(), Username: "ada", Role: auth.RoleAdmin}}
}

func withSession(req *http.Request) *http.Request {
	req.AddCookie(&http.Cookie{Name: "cid_session", Value: "tok"})
	return req
}

// --- tests ------------------------------------------------------------------

func TestServices_CreatePublic_NoSecret(t *testing.T) {
	fc := newFakeClientManager()
	h := mountServices(t, adminResolver(), fc, fakeSubjects{complete: true})

	body := `{"name":"Demo","redirectUris":["http://localhost:5173/callback"],"clientType":"public"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/v1/admin/services", strings.NewReader(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	var out createServiceResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.ClientID == "" {
		t.Errorf("missing clientId")
	}
	if out.ClientSecret != "" {
		t.Errorf("public client must not return a secret, got %q", out.ClientSecret)
	}
	// Reuse of adminapi mapping: public → token_endpoint_auth_method=none.
	if fc.lastCreated.TokenEndpointAuthMethod != "none" {
		t.Errorf("auth method = %q, want none", fc.lastCreated.TokenEndpointAuthMethod)
	}
	if fc.lastCreated.Scope != "openid profile email" {
		t.Errorf("default scope = %q", fc.lastCreated.Scope)
	}
}

func TestServices_CreateConfidential_SecretShownOnce(t *testing.T) {
	fc := newFakeClientManager()
	fc.createResp = oidc.OAuth2Client{ClientID: "srv-1", ClientSecret: "s3cr3t"}
	h := mountServices(t, adminResolver(), fc, fakeSubjects{complete: true})

	body := `{"name":"Server App","redirectUris":["https://app.example/cb"],"scopes":["openid"],"clientType":"confidential"}`
	req := withSession(httptest.NewRequest(http.MethodPost, "/api/v1/admin/services", strings.NewReader(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	var out createServiceResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.ClientSecret != "s3cr3t" {
		t.Fatalf("confidential create should return secret once, got %q", out.ClientSecret)
	}
	if fc.lastCreated.TokenEndpointAuthMethod != "client_secret_basic" {
		t.Errorf("auth method = %q, want client_secret_basic", fc.lastCreated.TokenEndpointAuthMethod)
	}

	// Secret must NEVER be re-served on a subsequent detail read.
	req2 := withSession(httptest.NewRequest(http.MethodGet, "/api/v1/admin/services/srv-1", nil))
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200 (body %s)", rec2.Code, rec2.Body.String())
	}
	if strings.Contains(rec2.Body.String(), "s3cr3t") || strings.Contains(rec2.Body.String(), "clientSecret") {
		t.Errorf("detail must not re-serve the secret: %s", rec2.Body.String())
	}
}

func TestServices_CreateValidation(t *testing.T) {
	fc := newFakeClientManager()
	h := mountServices(t, adminResolver(), fc, fakeSubjects{complete: true})

	cases := []struct{ name, body string }{
		{"missing name", `{"redirectUris":["http://x/cb"]}`},
		{"no redirect uris", `{"name":"X"}`},
		{"relative redirect uri", `{"name":"X","redirectUris":["/cb"]}`},
		{"fragment redirect uri", `{"name":"X","redirectUris":["http://x/cb#frag"]}`},
		{"bad client type", `{"name":"X","redirectUris":["http://x/cb"],"clientType":"weird"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := withSession(httptest.NewRequest(http.MethodPost, "/api/v1/admin/services", strings.NewReader(tc.body)))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestServices_List(t *testing.T) {
	fc := newFakeClientManager()
	fc.clients["a"] = oidc.OAuth2Client{ClientID: "a", ClientName: "A", TokenEndpointAuthMethod: "none", Scope: "openid profile"}
	fc.clients["b"] = oidc.OAuth2Client{ClientID: "b", ClientName: "B", TokenEndpointAuthMethod: "client_secret_basic"}
	h := mountServices(t, adminResolver(), fc, fakeSubjects{complete: true})

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/v1/admin/services", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out servicesListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Services) != 2 {
		t.Fatalf("got %d services, want 2", len(out.Services))
	}
	// Type derivation reused from adminapi.Summarize.
	types := map[string]string{}
	for _, s := range out.Services {
		types[s.ClientID] = s.ClientType
	}
	if types["a"] != adminapi.ClientTypePublic || types["b"] != adminapi.ClientTypeConfidential {
		t.Errorf("client types = %v", types)
	}
}

func TestServices_Edit_PreservesTypeAndMaps(t *testing.T) {
	fc := newFakeClientManager()
	fc.clients["srv"] = oidc.OAuth2Client{
		ClientID: "srv", ClientName: "Old", TokenEndpointAuthMethod: "client_secret_basic",
		RedirectURIs: []string{"https://old/cb"}, GrantTypes: []string{"authorization_code"},
		ResponseTypes: []string{"code"}, Scope: "openid",
	}
	h := mountServices(t, adminResolver(), fc, fakeSubjects{complete: true})

	body := `{"name":"New","redirectUris":["https://new/cb"],"scopes":["openid","email"]}`
	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/services/srv", strings.NewReader(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	// Type preserved (confidential → client_secret_basic) and fields overlaid.
	if fc.lastUpdated.TokenEndpointAuthMethod != "client_secret_basic" {
		t.Errorf("edit flipped auth method to %q", fc.lastUpdated.TokenEndpointAuthMethod)
	}
	if fc.lastUpdated.ClientName != "New" {
		t.Errorf("name not updated: %q", fc.lastUpdated.ClientName)
	}
	if fc.lastUpdated.Scope != "openid email" {
		t.Errorf("scope = %q, want 'openid email'", fc.lastUpdated.Scope)
	}
	if len(fc.lastUpdated.RedirectURIs) != 1 || fc.lastUpdated.RedirectURIs[0] != "https://new/cb" {
		t.Errorf("redirects = %v", fc.lastUpdated.RedirectURIs)
	}
	if fc.lastUpdateID != "srv" {
		t.Errorf("update id = %q", fc.lastUpdateID)
	}
}

func TestServices_Edit_RejectsSilentTypeFlip(t *testing.T) {
	fc := newFakeClientManager()
	fc.clients["srv"] = oidc.OAuth2Client{ClientID: "srv", ClientName: "Pub", TokenEndpointAuthMethod: "none", RedirectURIs: []string{"https://x/cb"}}
	h := mountServices(t, adminResolver(), fc, fakeSubjects{complete: true})

	// Existing is public; ask to make it confidential → must be rejected (409).
	body := `{"clientType":"confidential"}`
	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/services/srv", strings.NewReader(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body %s)", rec.Code, rec.Body.String())
	}
	if fc.lastUpdateID != "" {
		t.Errorf("update must not be called on a rejected type flip")
	}
}

func TestServices_Edit_RejectsBadRedirect(t *testing.T) {
	fc := newFakeClientManager()
	fc.clients["srv"] = oidc.OAuth2Client{ClientID: "srv", TokenEndpointAuthMethod: "none", RedirectURIs: []string{"https://x/cb"}}
	h := mountServices(t, adminResolver(), fc, fakeSubjects{complete: true})

	body := `{"redirectUris":["/relative"]}`
	req := withSession(httptest.NewRequest(http.MethodPatch, "/api/v1/admin/services/srv", strings.NewReader(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestServices_Detail_NotFound(t *testing.T) {
	fc := newFakeClientManager()
	h := mountServices(t, adminResolver(), fc, fakeSubjects{complete: true})
	req := withSession(httptest.NewRequest(http.MethodGet, "/api/v1/admin/services/ghost", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestServices_Delete(t *testing.T) {
	fc := newFakeClientManager()
	fc.clients["srv"] = oidc.OAuth2Client{ClientID: "srv"}
	h := mountServices(t, adminResolver(), fc, fakeSubjects{complete: true})

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/services/srv", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}
	if _, ok := fc.clients["srv"]; ok {
		t.Errorf("client not deleted")
	}
}

func TestServices_ConsentsCount_BestEffort(t *testing.T) {
	s1, s2, s3 := uuid.New(), uuid.New(), uuid.New()
	fc := newFakeClientManager()
	fc.consent[s1.String()] = []string{"srv", "other"}
	fc.consent[s2.String()] = []string{"other"}
	fc.consent[s3.String()] = []string{"srv"}
	subs := fakeSubjects{ids: []uuid.UUID{s1, s2, s3}, complete: true}
	h := mountServices(t, adminResolver(), fc, subs)

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/v1/admin/services/srv/consents", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var out serviceConsentsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Count != 2 {
		t.Errorf("count = %d, want 2 (subjects granting srv)", out.Count)
	}
	if !out.Complete {
		t.Errorf("complete should be true when all subjects scanned")
	}
}

func TestServices_RevokeConsents_BestEffort(t *testing.T) {
	s1, s2 := uuid.New(), uuid.New()
	fc := newFakeClientManager()
	fc.consent[s1.String()] = []string{"srv"}
	fc.consent[s2.String()] = []string{"srv", "other"}
	subs := fakeSubjects{ids: []uuid.UUID{s1, s2}, complete: true}
	h := mountServices(t, adminResolver(), fc, subs)

	req := withSession(httptest.NewRequest(http.MethodDelete, "/api/v1/admin/services/srv/consents", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var out revokeServiceConsentsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Revoked != 2 {
		t.Errorf("revoked = %d, want 2", out.Revoked)
	}
	// Each revoke was issued with subject+client (the only Hydra-supported call).
	if len(fc.revoked) != 2 {
		t.Fatalf("revoke calls = %d, want 2", len(fc.revoked))
	}
	for _, rp := range fc.revoked {
		if rp.client != "srv" {
			t.Errorf("revoke client = %q, want srv", rp.client)
		}
	}
}

func TestServices_NonAdminForbidden(t *testing.T) {
	fc := newFakeClientManager()
	resolver := stubResolver{user: &auth.User{ID: uuid.New(), Username: "joe", Role: auth.RoleUser}}
	h := mountServices(t, resolver, fc, fakeSubjects{complete: true})

	req := withSession(httptest.NewRequest(http.MethodGet, "/api/v1/admin/services", nil))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want 403", rec.Code)
	}
}

func TestServices_UnauthenticatedRejected(t *testing.T) {
	fc := newFakeClientManager()
	resolver := stubResolver{err: auth.ErrSessionNotFound}
	h := mountServices(t, resolver, fc, fakeSubjects{complete: true})

	// No session cookie → 401 from RequireRole.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", rec.Code)
	}
}
