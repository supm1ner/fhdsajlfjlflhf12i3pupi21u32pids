package social

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"cotton-id/internal/observability"
)

// newTestHandlers builds Handlers with the given enabled providers. Stores are
// left nil: the tested paths (providers list, disabled rejection, start redirect,
// state validation) never reach the DB.
func newTestHandlers(enabled map[string]bool) *Handlers {
	key, _ := NewSigningKey()
	cred := func(id string) ProviderCredentials {
		return ProviderCredentials{ClientID: id + "-id", ClientSecret: id + "-secret", Enabled: enabled[id]}
	}
	return NewHandlers(Deps{
		Logger:  observability.NewLogger("error"),
		Metrics: observability.NewMetrics(),
		Providers: ProvidersConfig{
			Google: cred(ProviderGoogle),
			GitHub: cred(ProviderGitHub),
			VK:     cred(ProviderVK),
			Yandex: cred(ProviderYandex),
		},
		PublicBaseURL:     "https://id.example.com",
		FrontendBaseURL:   "https://app.example.com",
		SessionCookieName: "cid_session",
		StateKey:          key,
	})
}

func mountRouter(h *Handlers) http.Handler {
	r := chi.NewRouter()
	h.Mount(r)
	return r
}

func TestProvidersListsOnlyEnabled(t *testing.T) {
	h := newTestHandlers(map[string]bool{ProviderGoogle: true, ProviderYandex: true})
	req := httptest.NewRequest(http.MethodGet, "/auth/social/providers", nil)
	rec := httptest.NewRecorder()
	mountRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out providersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Providers) != 2 {
		t.Fatalf("got %d providers, want 2: %+v", len(out.Providers), out.Providers)
	}
	if out.Providers[0].ID != ProviderGoogle || out.Providers[1].ID != ProviderYandex {
		t.Errorf("order/ids = %+v, want [google, yandex]", out.Providers)
	}
	if out.Providers[0].Name != "Google" {
		t.Errorf("google name = %q, want Google", out.Providers[0].Name)
	}
}

func TestProvidersEmptyWhenNoneEnabled(t *testing.T) {
	h := newTestHandlers(map[string]bool{})
	req := httptest.NewRequest(http.MethodGet, "/auth/social/providers", nil)
	rec := httptest.NewRecorder()
	mountRouter(h).ServeHTTP(rec, req)

	// Must be a present-but-empty array, not null (the SPA iterates it).
	if got := strings.TrimSpace(rec.Body.String()); got != `{"providers":[]}` {
		t.Errorf("body = %s, want empty providers array", got)
	}
}

func TestStartRejectsDisabledProvider(t *testing.T) {
	h := newTestHandlers(map[string]bool{ProviderGoogle: true})
	req := httptest.NewRequest(http.MethodGet, "/auth/social/github/start", nil)
	rec := httptest.NewRecorder()
	mountRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type = %q, want problem+json", ct)
	}
}

func TestStartRedirectsAndSetsStateCookie(t *testing.T) {
	h := newTestHandlers(map[string]bool{ProviderGoogle: true})
	req := httptest.NewRequest(http.MethodGet, "/auth/social/google/start?login_challenge=chal&remember=true", nil)
	rec := httptest.NewRecorder()
	mountRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	for _, want := range []string{
		"https://accounts.google.com/o/oauth2/v2/auth",
		"client_id=google-id",
		"redirect_uri=https%3A%2F%2Fid.example.com%2Fapi%2Fv1%2Fauth%2Fsocial%2Fgoogle%2Fcallback",
		"code_challenge_method=S256",
		"state=",
	} {
		if !strings.Contains(loc, want) {
			t.Errorf("redirect %s missing %q", loc, want)
		}
	}

	// The cid_oauth cookie must be set, HttpOnly, and carry the challenge/remember.
	var oauthCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == OAuthCookieName {
			oauthCookie = c
		}
	}
	if oauthCookie == nil {
		t.Fatal("cid_oauth cookie not set")
	}
	if !oauthCookie.HttpOnly {
		t.Error("cid_oauth must be HttpOnly")
	}
	if oauthCookie.SameSite != http.SameSiteLaxMode {
		t.Error("cid_oauth must be SameSite=Lax")
	}
	codec := newStateCodec(h.deps.StateKey, false)
	st, err := codec.parse(oauthCookie.Value)
	if err != nil {
		t.Fatalf("cookie should be a valid signed state: %v", err)
	}
	if st.LoginChallenge != "chal" || !st.Remember || st.Provider != ProviderGoogle {
		t.Errorf("state payload = %+v, want chal/remember/google", st)
	}
}

func TestCallbackRejectsMissingStateCookie(t *testing.T) {
	h := newTestHandlers(map[string]bool{ProviderGoogle: true})
	req := httptest.NewRequest(http.MethodGet, "/auth/social/google/callback?code=abc&state=xyz", nil)
	rec := httptest.NewRecorder()
	mountRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no state cookie)", rec.Code)
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	h := newTestHandlers(map[string]bool{ProviderGoogle: true})
	codec := newStateCodec(h.deps.StateKey, false)
	st, _ := newState(ProviderGoogle, "", false, true)
	value, _ := codec.sign(st)

	// Query state does NOT match the cookie's state.
	req := httptest.NewRequest(http.MethodGet, "/auth/social/google/callback?code=abc&state=WRONG", nil)
	req.AddCookie(&http.Cookie{Name: OAuthCookieName, Value: value})
	rec := httptest.NewRecorder()
	mountRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (state mismatch)", rec.Code)
	}
	// The cookie must be cleared on a failed callback.
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == OAuthCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("cid_oauth cookie should be cleared on callback")
	}
}

func TestCallbackRejectsDisabledProvider(t *testing.T) {
	h := newTestHandlers(map[string]bool{ProviderGoogle: true})
	req := httptest.NewRequest(http.MethodGet, "/auth/social/vk/callback?code=abc&state=xyz", nil)
	rec := httptest.NewRecorder()
	mountRouter(h).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
