package adminapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"cotton-id/internal/observability"
)

const testAdminKey = "test-admin-key-0123456789abcdef"

// fakeHydra returns a test server emulating the subset of Hydra's admin client
// endpoints used by the admin API, plus a pointer to the last request body.
func fakeHydra(t *testing.T, createResp string) (*httptest.Server, *map[string]any, *string) {
	t.Helper()
	var lastBody map[string]any
	var deletedID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/clients":
			lastBody = map[string]any{}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &lastBody)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, createResp)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/clients":
			_, _ = io.WriteString(w, `[{"client_id":"a","client_name":"A","token_endpoint_auth_method":"none","scope":"openid profile"},{"client_id":"b","client_name":"B","token_endpoint_auth_method":"client_secret_basic"}]`)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/admin/clients/"):
			deletedID = strings.TrimPrefix(r.URL.Path, "/admin/clients/")
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &lastBody, &deletedID
}

func mount(hydraURL string) http.Handler {
	r := chi.NewRouter()
	r.Route("/api/v1", func(api chi.Router) {
		Mount(api, Deps{
			Logger:        observability.NewLogger("error"),
			Metrics:       observability.NewMetrics(),
			HydraAdminURL: hydraURL,
			AdminAPIKey:   testAdminKey,
		})
	})
	return r
}

func TestAdmin_RequiresAdminKey(t *testing.T) {
	srv, _, _ := fakeHydra(t, `{}`)
	h := mount(srv.URL)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/clients", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing key: status = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/admin/clients", nil)
	req.Header.Set("X-Admin-Key", "wrong")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: status = %d, want 401", rec.Code)
	}
}

func TestAdmin_RegisterPublicClient(t *testing.T) {
	srv, lastBody, _ := fakeHydra(t, `{"client_id":"generated","client_name":"Demo"}`)
	h := mount(srv.URL)

	body := `{"name":"Demo","redirectUris":["http://localhost:5173/callback"],"clientType":"public"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/clients", strings.NewReader(body))
	req.Header.Set("X-Admin-Key", testAdminKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	var out registerClientResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.ClientID != "generated" {
		t.Errorf("clientId = %q", out.ClientID)
	}
	if out.ClientSecret != "" {
		t.Errorf("public client must not return a secret, got %q", out.ClientSecret)
	}
	// Public client must be sent to Hydra with token_endpoint_auth_method=none and
	// default scopes/grant types.
	if (*lastBody)["token_endpoint_auth_method"] != "none" {
		t.Errorf("auth method = %v, want none", (*lastBody)["token_endpoint_auth_method"])
	}
	gt, _ := (*lastBody)["grant_types"].([]any)
	if len(gt) == 0 {
		t.Errorf("grant_types should default to non-empty")
	}
	if (*lastBody)["scope"] != "openid profile email" {
		t.Errorf("scope default = %v", (*lastBody)["scope"])
	}
}

func TestAdmin_RegisterConfidentialClientReturnsSecret(t *testing.T) {
	srv, lastBody, _ := fakeHydra(t, `{"client_id":"generated","client_secret":"s3cr3t"}`)
	h := mount(srv.URL)

	body := `{"name":"Server App","redirectUris":["https://app.example/cb"],"scopes":["openid"],"clientType":"confidential"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/clients", strings.NewReader(body))
	req.Header.Set("X-Admin-Key", testAdminKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	var out registerClientResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.ClientSecret != "s3cr3t" {
		t.Errorf("confidential client should return secret, got %q", out.ClientSecret)
	}
	if (*lastBody)["token_endpoint_auth_method"] != "client_secret_basic" {
		t.Errorf("auth method = %v, want client_secret_basic", (*lastBody)["token_endpoint_auth_method"])
	}
}

func TestAdmin_RegisterValidation(t *testing.T) {
	srv, _, _ := fakeHydra(t, `{}`)
	h := mount(srv.URL)

	cases := []struct {
		name string
		body string
	}{
		{"missing name", `{"redirectUris":["http://x/cb"]}`},
		{"no redirect uris", `{"name":"X"}`},
		{"relative redirect uri", `{"name":"X","redirectUris":["/cb"]}`},
		{"redirect uri with fragment", `{"name":"X","redirectUris":["http://x/cb#frag"]}`},
		{"bad client type", `{"name":"X","redirectUris":["http://x/cb"],"clientType":"weird"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/clients", strings.NewReader(tc.body))
			req.Header.Set("X-Admin-Key", testAdminKey)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAdmin_ListClients(t *testing.T) {
	srv, _, _ := fakeHydra(t, `{}`)
	h := mount(srv.URL)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/clients", nil)
	req.Header.Set("X-Admin-Key", testAdminKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out listClientsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Clients) != 2 {
		t.Fatalf("got %d clients, want 2", len(out.Clients))
	}
	if out.Clients[0].ClientType != ClientTypePublic {
		t.Errorf("client a should be public (auth method none), got %s", out.Clients[0].ClientType)
	}
	if out.Clients[1].ClientType != ClientTypeConfidential {
		t.Errorf("client b should be confidential, got %s", out.Clients[1].ClientType)
	}
	// scope string should split into a slice.
	if len(out.Clients[0].Scopes) != 2 {
		t.Errorf("client a scopes = %v, want 2", out.Clients[0].Scopes)
	}
}

func TestAdmin_DeleteClient(t *testing.T) {
	srv, _, deletedID := fakeHydra(t, `{}`)
	h := mount(srv.URL)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/clients/demo-id", nil)
	req.Header.Set("X-Admin-Key", testAdminKey)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if *deletedID != "demo-id" {
		t.Errorf("deleted id = %q, want demo-id", *deletedID)
	}
}

func TestValidRedirectURI(t *testing.T) {
	cases := []struct {
		uri  string
		want bool
	}{
		{"http://localhost:3000/callback", true},
		{"https://app.example.com/cb", true},
		{"/relative", false},
		{"app://callback", false},
		{"http://host/cb#fragment", false},
		{"not a url at all", false},
		{"https://", false},
	}
	for _, tc := range cases {
		if got := validRedirectURI(tc.uri); got != tc.want {
			t.Errorf("validRedirectURI(%q) = %v, want %v", tc.uri, got, tc.want)
		}
	}
}
