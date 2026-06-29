package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// capturedRequest records what the fake Hydra admin server received so the test
// can assert the client built the request correctly.
type capturedRequest struct {
	Method     string
	Path       string
	EscapedURI string // the raw (escaped) request-target path, as sent on the wire
	Query      url.Values
	Body       map[string]any
}

// newFakeHydra returns a test server that records the last request and replies
// with the provided status/body, plus the captured-request pointer.
func newFakeHydra(t *testing.T, status int, respBody string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	cap := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.Method = r.Method
		cap.Path = r.URL.Path
		cap.EscapedURI = r.URL.EscapedPath()
		cap.Query = r.URL.Query()
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			cap.Body = map[string]any{}
			_ = json.Unmarshal(b, &cap.Body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

func TestHydraClient_GetLoginRequest(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK,
		`{"challenge":"abc","skip":true,"subject":"user-1","requested_scope":["openid","email"]}`)
	c := NewHydraClient(srv.URL)

	lr, err := c.GetLoginRequest(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetLoginRequest: %v", err)
	}
	if cap.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", cap.Method)
	}
	if cap.Path != "/admin/oauth2/auth/requests/login" {
		t.Errorf("path = %s", cap.Path)
	}
	if got := cap.Query.Get("login_challenge"); got != "abc" {
		t.Errorf("login_challenge = %q, want abc", got)
	}
	if !lr.Skip || lr.Subject != "user-1" {
		t.Errorf("decoded login request mismatch: %+v", lr)
	}
	if len(lr.RequestedScope) != 2 || lr.RequestedScope[0] != "openid" {
		t.Errorf("requested_scope mismatch: %+v", lr.RequestedScope)
	}
}

func TestHydraClient_AcceptLoginRequest(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK, `{"redirect_to":"http://hydra/continue"}`)
	c := NewHydraClient(srv.URL)

	out, err := c.AcceptLoginRequest(context.Background(), "abc", AcceptLogin{Subject: "user-1"})
	if err != nil {
		t.Fatalf("AcceptLoginRequest: %v", err)
	}
	if cap.Method != http.MethodPut {
		t.Errorf("method = %s, want PUT", cap.Method)
	}
	if cap.Path != "/admin/oauth2/auth/requests/login/accept" {
		t.Errorf("path = %s", cap.Path)
	}
	if cap.Query.Get("login_challenge") != "abc" {
		t.Errorf("missing login_challenge query")
	}
	if cap.Body["subject"] != "user-1" {
		t.Errorf("body subject = %v, want user-1", cap.Body["subject"])
	}
	if out.RedirectTo != "http://hydra/continue" {
		t.Errorf("redirect_to = %q", out.RedirectTo)
	}
}

func TestHydraClient_RejectLoginRequest(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK, `{"redirect_to":"http://hydra/denied"}`)
	c := NewHydraClient(srv.URL)

	_, err := c.RejectLoginRequest(context.Background(), "abc", RejectRequest{Error: "access_denied"})
	if err != nil {
		t.Fatalf("RejectLoginRequest: %v", err)
	}
	if cap.Path != "/admin/oauth2/auth/requests/login/reject" {
		t.Errorf("path = %s", cap.Path)
	}
	if cap.Body["error"] != "access_denied" {
		t.Errorf("body error = %v", cap.Body["error"])
	}
}

func TestHydraClient_GetConsentRequest(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK,
		`{"challenge":"k","requested_scope":["openid","profile"],"client":{"client_id":"demo","client_name":"Demo"}}`)
	c := NewHydraClient(srv.URL)

	cr, err := c.GetConsentRequest(context.Background(), "k")
	if err != nil {
		t.Fatalf("GetConsentRequest: %v", err)
	}
	if cap.Path != "/admin/oauth2/auth/requests/consent" {
		t.Errorf("path = %s", cap.Path)
	}
	if cap.Query.Get("consent_challenge") != "k" {
		t.Errorf("missing consent_challenge query")
	}
	if cr.Client == nil || cr.Client.ClientID != "demo" || cr.Client.ClientName != "Demo" {
		t.Errorf("client decode mismatch: %+v", cr.Client)
	}
	if len(cr.RequestedScope) != 2 {
		t.Errorf("requested_scope mismatch: %+v", cr.RequestedScope)
	}
}

func TestHydraClient_AcceptConsentRequest(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK, `{"redirect_to":"http://hydra/code"}`)
	c := NewHydraClient(srv.URL)

	body := AcceptConsent{
		GrantScope:  []string{"openid", "email"},
		Remember:    true,
		RememberFor: 3600,
		Session:     &ConsentSession{IDToken: IDTokenClaims{Subject: "user-1", Email: "a@b.c"}},
	}
	out, err := c.AcceptConsentRequest(context.Background(), "k", body)
	if err != nil {
		t.Fatalf("AcceptConsentRequest: %v", err)
	}
	if cap.Path != "/admin/oauth2/auth/requests/consent/accept" {
		t.Errorf("path = %s", cap.Path)
	}
	if cap.Method != http.MethodPut {
		t.Errorf("method = %s, want PUT", cap.Method)
	}
	gs, ok := cap.Body["grant_scope"].([]any)
	if !ok || len(gs) != 2 || gs[0] != "openid" {
		t.Errorf("grant_scope mismatch: %v", cap.Body["grant_scope"])
	}
	if cap.Body["remember"] != true {
		t.Errorf("remember = %v, want true", cap.Body["remember"])
	}
	// session.id_token.sub should round-trip through the JSON body.
	sess, _ := cap.Body["session"].(map[string]any)
	idt, _ := sess["id_token"].(map[string]any)
	if idt["sub"] != "user-1" {
		t.Errorf("session.id_token.sub = %v, want user-1", idt["sub"])
	}
	if out.RedirectTo != "http://hydra/code" {
		t.Errorf("redirect_to = %q", out.RedirectTo)
	}
}

func TestHydraClient_RejectConsentRequest(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK, `{"redirect_to":"http://hydra/denied"}`)
	c := NewHydraClient(srv.URL)

	_, err := c.RejectConsentRequest(context.Background(), "k", RejectRequest{Error: "access_denied"})
	if err != nil {
		t.Fatalf("RejectConsentRequest: %v", err)
	}
	if cap.Path != "/admin/oauth2/auth/requests/consent/reject" {
		t.Errorf("path = %s", cap.Path)
	}
}

func TestHydraClient_LogoutRequest(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK, `{"challenge":"lo","subject":"user-1"}`)
	c := NewHydraClient(srv.URL)

	lr, err := c.GetLogoutRequest(context.Background(), "lo")
	if err != nil {
		t.Fatalf("GetLogoutRequest: %v", err)
	}
	if cap.Path != "/admin/oauth2/auth/requests/logout" || cap.Query.Get("logout_challenge") != "lo" {
		t.Errorf("get logout request mismatch: path=%s q=%v", cap.Path, cap.Query)
	}
	if lr.Subject != "user-1" {
		t.Errorf("subject = %q", lr.Subject)
	}

	srv2, cap2 := newFakeHydra(t, http.StatusOK, `{"redirect_to":"http://app/logged-out"}`)
	c2 := NewHydraClient(srv2.URL)
	out, err := c2.AcceptLogoutRequest(context.Background(), "lo")
	if err != nil {
		t.Fatalf("AcceptLogoutRequest: %v", err)
	}
	if cap2.Method != http.MethodPut || cap2.Path != "/admin/oauth2/auth/requests/logout/accept" {
		t.Errorf("accept logout mismatch: %s %s", cap2.Method, cap2.Path)
	}
	if out.RedirectTo != "http://app/logged-out" {
		t.Errorf("redirect_to = %q", out.RedirectTo)
	}
}

func TestHydraClient_CreateClient(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusCreated,
		`{"client_id":"generated-id","client_secret":"s3cr3t","client_name":"Demo"}`)
	c := NewHydraClient(srv.URL)

	out, err := c.CreateClient(context.Background(), OAuth2Client{
		ClientName:   "Demo",
		RedirectURIs: []string{"http://localhost/cb"},
		Scope:        "openid profile",
	})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if cap.Method != http.MethodPost || cap.Path != "/admin/clients" {
		t.Errorf("create client request mismatch: %s %s", cap.Method, cap.Path)
	}
	if cap.Body["client_name"] != "Demo" {
		t.Errorf("client_name = %v", cap.Body["client_name"])
	}
	if cap.Body["scope"] != "openid profile" {
		t.Errorf("scope = %v", cap.Body["scope"])
	}
	if out.ClientID != "generated-id" || out.ClientSecret != "s3cr3t" {
		t.Errorf("create response mismatch: %+v", out)
	}
}

func TestHydraClient_GetClient(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK,
		`{"client_id":"demo id","client_name":"Demo","redirect_uris":["http://localhost/cb"],"scope":"openid profile","token_endpoint_auth_method":"none"}`)
	c := NewHydraClient(srv.URL)

	out, err := c.GetClient(context.Background(), "demo id")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if cap.Method != http.MethodGet {
		t.Errorf("method = %s, want GET", cap.Method)
	}
	// The id must be path-escaped on the wire.
	if cap.EscapedURI != "/admin/clients/demo%20id" {
		t.Errorf("get path not escaped: %s", cap.EscapedURI)
	}
	if out.ClientID != "demo id" || out.ClientName != "Demo" {
		t.Errorf("get response mismatch: %+v", out)
	}
	if out.TokenEndpointAuthMethod != "none" {
		t.Errorf("auth method = %q, want none", out.TokenEndpointAuthMethod)
	}
}

func TestHydraClient_GetClient_NotFound(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusNotFound, `{"error":"not found"}`)
	c := NewHydraClient(srv.URL)
	_, err := c.GetClient(context.Background(), "missing")
	var he *HydraError
	if !errors.As(err, &he) || he.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 HydraError, got %v", err)
	}
}

func TestHydraClient_UpdateClient(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK,
		`{"client_id":"demo","client_name":"Renamed","token_endpoint_auth_method":"client_secret_basic"}`)
	c := NewHydraClient(srv.URL)

	out, err := c.UpdateClient(context.Background(), "demo", OAuth2Client{
		ClientID:                "demo",
		ClientName:              "Renamed",
		RedirectURIs:            []string{"https://app.example/cb"},
		GrantTypes:              []string{"authorization_code"},
		ResponseTypes:           []string{"code"},
		Scope:                   "openid email",
		TokenEndpointAuthMethod: "client_secret_basic",
	})
	if err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}
	if cap.Method != http.MethodPut || cap.Path != "/admin/clients/demo" {
		t.Errorf("update request mismatch: %s %s", cap.Method, cap.Path)
	}
	if cap.Body["client_name"] != "Renamed" {
		t.Errorf("body client_name = %v, want Renamed", cap.Body["client_name"])
	}
	if cap.Body["scope"] != "openid email" {
		t.Errorf("body scope = %v", cap.Body["scope"])
	}
	if out.ClientName != "Renamed" {
		t.Errorf("update response mismatch: %+v", out)
	}
}

func TestHydraClient_ListClients(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK,
		`[{"client_id":"a","client_name":"A"},{"client_id":"b","client_name":"B"}]`)
	c := NewHydraClient(srv.URL)

	clients, err := c.ListClients(context.Background())
	if err != nil {
		t.Fatalf("ListClients: %v", err)
	}
	if cap.Method != http.MethodGet || cap.Path != "/admin/clients" {
		t.Errorf("list clients request mismatch: %s %s", cap.Method, cap.Path)
	}
	if len(clients) != 2 || clients[0].ClientID != "a" || clients[1].ClientID != "b" {
		t.Errorf("list decode mismatch: %+v", clients)
	}
}

func TestHydraClient_DeleteClient(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusNoContent, ``)
	c := NewHydraClient(srv.URL)

	if err := c.DeleteClient(context.Background(), "my id/with space"); err != nil {
		t.Fatalf("DeleteClient: %v", err)
	}
	if cap.Method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", cap.Method)
	}
	// The id must be path-escaped on the wire (EscapedPath reflects the raw
	// request-target; r.URL.Path is already decoded by the server).
	if cap.EscapedURI != "/admin/clients/my%20id%2Fwith%20space" {
		t.Errorf("delete path not escaped: %s", cap.EscapedURI)
	}
}

func TestHydraClient_DeleteClient_NotFoundIsIdempotent(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusNotFound, `{"error":"not found"}`)
	c := NewHydraClient(srv.URL)
	if err := c.DeleteClient(context.Background(), "missing"); err != nil {
		t.Fatalf("DeleteClient on 404 should be nil, got %v", err)
	}
}

func TestHydraClient_Non2xxBecomesHydraError(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusConflict, `{"error":"conflict"}`)
	c := NewHydraClient(srv.URL)

	_, err := c.CreateClient(context.Background(), OAuth2Client{ClientName: "x"})
	var he *HydraError
	if !errors.As(err, &he) {
		t.Fatalf("expected *HydraError, got %v", err)
	}
	if he.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409", he.StatusCode)
	}
	if he.Body == "" {
		t.Errorf("HydraError should carry the response body")
	}
}

func TestHydraClient_ListConsentSessions(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK,
		`[{"consent_request":{"subject":"user-1","client":{"client_id":"demo","client_name":"Demo App"}},`+
			`"grant_scope":["openid","email"],"handled_at":"2026-01-01T00:00:00Z","remember":true}]`)
	c := NewHydraClient(srv.URL)

	records, err := c.ListConsentSessions(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListConsentSessions: %v", err)
	}
	if cap.Method != http.MethodGet || cap.Path != "/admin/oauth2/auth/sessions/consent" {
		t.Errorf("request mismatch: %s %s", cap.Method, cap.Path)
	}
	if cap.Query.Get("subject") != "user-1" {
		t.Errorf("subject query = %q, want user-1", cap.Query.Get("subject"))
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	rec := records[0]
	if rec.ConsentRequest == nil || rec.ConsentRequest.Client == nil ||
		rec.ConsentRequest.Client.ClientID != "demo" || rec.ConsentRequest.Client.ClientName != "Demo App" {
		t.Errorf("consent_request.client decode mismatch: %+v", rec.ConsentRequest)
	}
	if len(rec.GrantScope) != 2 || rec.GrantScope[0] != "openid" {
		t.Errorf("grant_scope mismatch: %+v", rec.GrantScope)
	}
	if rec.HandledAt != "2026-01-01T00:00:00Z" {
		t.Errorf("handled_at = %q", rec.HandledAt)
	}
}

func TestHydraClient_ListConsentSessions_EmptyArray(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusOK, `[]`)
	c := NewHydraClient(srv.URL)
	records, err := c.ListConsentSessions(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("empty list err = %v, want nil", err)
	}
	if len(records) != 0 {
		t.Errorf("records = %d, want 0", len(records))
	}
}

func TestHydraClient_RevokeConsentSessions(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusNoContent, ``)
	c := NewHydraClient(srv.URL)

	if err := c.RevokeConsentSessions(context.Background(), "user-1", "demo"); err != nil {
		t.Fatalf("RevokeConsentSessions: %v", err)
	}
	if cap.Method != http.MethodDelete || cap.Path != "/admin/oauth2/auth/sessions/consent" {
		t.Errorf("request mismatch: %s %s", cap.Method, cap.Path)
	}
	if cap.Query.Get("subject") != "user-1" || cap.Query.Get("client") != "demo" {
		t.Errorf("query mismatch: subject=%q client=%q", cap.Query.Get("subject"), cap.Query.Get("client"))
	}
}

func TestHydraClient_RevokeConsentSessions_NotFoundIdempotent(t *testing.T) {
	srv, _ := newFakeHydra(t, http.StatusNotFound, `{"error":"not found"}`)
	c := NewHydraClient(srv.URL)
	if err := c.RevokeConsentSessions(context.Background(), "user-1", "demo"); err != nil {
		t.Fatalf("404 revoke should be nil, got %v", err)
	}
}

func TestHydraClient_RevokeAllConsentSessions(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusNoContent, ``)
	c := NewHydraClient(srv.URL)

	if err := c.RevokeAllConsentSessions(context.Background(), "user-1"); err != nil {
		t.Fatalf("RevokeAllConsentSessions: %v", err)
	}
	if cap.Method != http.MethodDelete || cap.Path != "/admin/oauth2/auth/sessions/consent" {
		t.Errorf("request mismatch: %s %s", cap.Method, cap.Path)
	}
	if cap.Query.Get("subject") != "user-1" || cap.Query.Get("all") != "true" {
		t.Errorf("query mismatch: subject=%q all=%q", cap.Query.Get("subject"), cap.Query.Get("all"))
	}
}

func TestHydraClient_RevokeLoginSessions(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusNoContent, ``)
	c := NewHydraClient(srv.URL)

	if err := c.RevokeLoginSessions(context.Background(), "user-1"); err != nil {
		t.Fatalf("RevokeLoginSessions: %v", err)
	}
	if cap.Method != http.MethodDelete || cap.Path != "/admin/oauth2/auth/sessions/login" {
		t.Errorf("request mismatch: %s %s", cap.Method, cap.Path)
	}
	if cap.Query.Get("subject") != "user-1" {
		t.Errorf("subject query = %q, want user-1", cap.Query.Get("subject"))
	}
}

func TestHydraClient_TrimsTrailingSlash(t *testing.T) {
	srv, cap := newFakeHydra(t, http.StatusOK, `{"challenge":"x"}`)
	c := NewHydraClient(srv.URL + "/")
	if _, err := c.GetLoginRequest(context.Background(), "x"); err != nil {
		t.Fatalf("GetLoginRequest: %v", err)
	}
	// No double slash in the path.
	if cap.Path != "/admin/oauth2/auth/requests/login" {
		t.Errorf("path = %s (trailing slash not trimmed?)", cap.Path)
	}
}
