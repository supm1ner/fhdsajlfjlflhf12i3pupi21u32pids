package social

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newFakeProvider returns a copy of the real provider with its endpoint URLs
// repointed at the test server base, so mapUserInfo exercises the real JSON
// shapes against canned responses.
func repoint(p *Provider, base string) *Provider {
	cp := *p
	cp.tokenURL = base + "/token"
	cp.userInfoURL = base + "/userinfo"
	return &cp
}

// TestMapUserInfoGoogle verifies the OIDC userinfo mapping, including the
// email_verified passthrough.
func TestMapUserInfoGoogle(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantSubject  string
		wantEmail    string
		wantVerified bool
		wantName     string
		wantAvatar   string
		wantErr      bool
	}{
		{
			name:         "verified",
			body:         `{"sub":"108","email":"a@gmail.com","email_verified":true,"name":"Ann Lee","given_name":"Ann","picture":"http://img/a.png"}`,
			wantSubject:  "108",
			wantEmail:    "a@gmail.com",
			wantVerified: true,
			wantName:     "Ann Lee",
			wantAvatar:   "http://img/a.png",
		},
		{
			name:         "unverified",
			body:         `{"sub":"109","email":"u@gmail.com","email_verified":false,"name":"U"}`,
			wantSubject:  "109",
			wantEmail:    "u@gmail.com",
			wantVerified: false,
			wantName:     "U",
		},
		{
			name:    "missing sub is an error",
			body:    `{"email":"x@gmail.com","email_verified":true}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer tok" {
					t.Errorf("missing bearer auth: %q", r.Header.Get("Authorization"))
				}
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			p := repoint(googleProvider(), srv.URL)
			id, err := p.mapUserInfo(context.Background(), p, srv.Client(), &tokenResponse{AccessToken: "tok"})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("mapUserInfo: %v", err)
			}
			assertIdentity(t, id, tt.wantSubject, tt.wantEmail, tt.wantVerified, tt.wantName, tt.wantAvatar)
		})
	}
}

// TestMapUserInfoGitHub verifies the /user + /user/emails flow, especially that
// the primary VERIFIED email is selected and an unverified-only set yields no
// trusted email.
func TestMapUserInfoGitHub(t *testing.T) {
	tests := []struct {
		name         string
		userBody     string
		emailsBody   string
		wantSubject  string
		wantEmail    string
		wantVerified bool
		wantUsername string
	}{
		{
			name:         "primary verified selected",
			userBody:     `{"id":42,"login":"octocat","name":"The Octocat","avatar_url":"http://gh/a.png"}`,
			emailsBody:   `[{"email":"secondary@x.com","primary":false,"verified":true},{"email":"octo@x.com","primary":true,"verified":true}]`,
			wantSubject:  "42",
			wantEmail:    "octo@x.com",
			wantVerified: true,
			wantUsername: "octocat",
		},
		{
			name:         "primary unverified falls to first verified",
			userBody:     `{"id":7,"login":"u7"}`,
			emailsBody:   `[{"email":"primary@x.com","primary":true,"verified":false},{"email":"verified@x.com","primary":false,"verified":true}]`,
			wantSubject:  "7",
			wantEmail:    "verified@x.com",
			wantVerified: true,
			wantUsername: "u7",
		},
		{
			name:         "no verified email yields untrusted empty",
			userBody:     `{"id":9,"login":"u9","email":"profile@x.com"}`,
			emailsBody:   `[{"email":"primary@x.com","primary":true,"verified":false}]`,
			wantSubject:  "9",
			wantEmail:    "profile@x.com", // falls back to profile email, but UNVERIFIED
			wantVerified: false,
			wantUsername: "u9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.userBody))
			})
			mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.emailsBody))
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			// Repoint the absolute api.github.com URLs at the test server.
			p := githubProviderAt(srv.URL)
			id, err := p.mapUserInfo(context.Background(), p, srv.Client(), &tokenResponse{AccessToken: "tok"})
			if err != nil {
				t.Fatalf("mapUserInfo: %v", err)
			}
			if id.Subject != tt.wantSubject {
				t.Errorf("subject = %q, want %q", id.Subject, tt.wantSubject)
			}
			if id.Email != tt.wantEmail {
				t.Errorf("email = %q, want %q", id.Email, tt.wantEmail)
			}
			if id.EmailVerified != tt.wantVerified {
				t.Errorf("verified = %v, want %v", id.EmailVerified, tt.wantVerified)
			}
			if id.Username != tt.wantUsername {
				t.Errorf("username = %q, want %q", id.Username, tt.wantUsername)
			}
		})
	}
}

// TestPickGitHubEmail unit-tests the pure email-selection logic.
func TestPickGitHubEmail(t *testing.T) {
	tests := []struct {
		name      string
		in        []githubEmail
		wantEmail string
		wantOK    bool
	}{
		{"empty", nil, "", false},
		{"primary verified", []githubEmail{{"a@x", false, true}, {"b@x", true, true}}, "b@x", true},
		{"first verified when primary unverified", []githubEmail{{"a@x", true, false}, {"b@x", false, true}}, "b@x", true},
		{"none verified", []githubEmail{{"a@x", true, false}, {"b@x", false, false}}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, ok := pickGitHubEmail(tt.in)
			if email != tt.wantEmail || ok != tt.wantOK {
				t.Errorf("pickGitHubEmail = (%q, %v), want (%q, %v)", email, ok, tt.wantEmail, tt.wantOK)
			}
		})
	}
}

// TestMapVKUserInfo verifies VK ID's {"user":{...}} envelope mapping and the
// email-from-token fallback.
func TestMapVKUserInfo(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		tokenEmail   string
		wantSubject  string
		wantEmail    string
		wantVerified bool
		wantName     string
		wantErr      bool
	}{
		{
			name:         "email in user_info, numeric user_id (VK email is never verified)",
			body:         `{"user":{"user_id":12345,"first_name":"Ivan","last_name":"Petrov","email":"ivan@vk.com","avatar":"http://vk/a.jpg"}}`,
			wantSubject:  "12345",
			wantEmail:    "ivan@vk.com",
			wantVerified: false,
			wantName:     "Ivan Petrov",
		},
		{
			name:         "email only in token response (still unverified)",
			body:         `{"user":{"user_id":"678","first_name":"No","last_name":"Mail"}}`,
			tokenEmail:   "token@vk.com",
			wantSubject:  "678",
			wantEmail:    "token@vk.com",
			wantVerified: false,
			wantName:     "No Mail",
		},
		{
			name:         "no email at all is unverified",
			body:         `{"user":{"user_id":"9","first_name":"Anon"}}`,
			wantSubject:  "9",
			wantEmail:    "",
			wantVerified: false,
			wantName:     "Anon",
		},
		{
			name:    "missing user_id is an error",
			body:    `{"user":{"first_name":"X"}}`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := mapVKUserInfo([]byte(tt.body), tt.tokenEmail)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("mapVKUserInfo: %v", err)
			}
			assertIdentity(t, id, tt.wantSubject, tt.wantEmail, tt.wantVerified, tt.wantName, id.AvatarURL)
		})
	}
}

// TestMapUserInfoYandex verifies the login.yandex.ru/info mapping, the OAuth auth
// header, the real_name→display_name fallback, and avatar URL construction.
func TestMapUserInfoYandex(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantSubject string
		wantEmail   string
		wantName    string
		wantUser    string
		wantAvatar  string
	}{
		{
			name:        "full profile with avatar",
			body:        `{"id":"4242","login":"yndx","default_email":"y@ya.ru","real_name":"Yan Dex","default_avatar_id":"abc","is_avatar_empty":false}`,
			wantSubject: "4242",
			wantEmail:   "y@ya.ru",
			wantName:    "Yan Dex",
			wantUser:    "yndx",
			wantAvatar:  "https://avatars.yandex.net/get-yapic/abc/islands-200",
		},
		{
			name:        "display_name fallback, empty avatar",
			body:        `{"id":"5","login":"u5","default_email":"u5@ya.ru","display_name":"U Five","is_avatar_empty":true}`,
			wantSubject: "5",
			wantEmail:   "u5@ya.ru",
			wantName:    "U Five",
			wantUser:    "u5",
			wantAvatar:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != "OAuth tok" {
					t.Errorf("auth header = %q, want %q", got, "OAuth tok")
				}
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			p := repoint(yandexProvider(), srv.URL)
			id, err := p.mapUserInfo(context.Background(), p, srv.Client(), &tokenResponse{AccessToken: "tok"})
			if err != nil {
				t.Fatalf("mapUserInfo: %v", err)
			}
			if id.Subject != tt.wantSubject || id.Email != tt.wantEmail || id.Name != tt.wantName ||
				id.Username != tt.wantUser || id.AvatarURL != tt.wantAvatar {
				t.Errorf("identity = %+v, want subject=%q email=%q name=%q user=%q avatar=%q",
					id, tt.wantSubject, tt.wantEmail, tt.wantName, tt.wantUser, tt.wantAvatar)
			}
			if !id.EmailVerified {
				t.Errorf("yandex default_email should be treated as verified")
			}
		})
	}
}

// TestAuthCodeURL checks scope joining (space vs comma), PKCE params, and the
// fixed query parameters.
func TestAuthCodeURL(t *testing.T) {
	g := googleProvider()
	u := g.AuthCodeURL("cid", "https://app/cb", "STATE", "CHAL")
	for _, want := range []string{"client_id=cid", "state=STATE", "code_challenge=CHAL", "code_challenge_method=S256", "scope=openid+email+profile", "response_type=code"} {
		if !strings.Contains(u, want) {
			t.Errorf("google authURL missing %q in %s", want, u)
		}
	}

	y := yandexProvider()
	uy := y.AuthCodeURL("cid", "https://app/cb", "S", "C")
	if !strings.Contains(uy, "scope=login%3Aemail%2Clogin%3Ainfo%2Clogin%3Aavatar") {
		t.Errorf("yandex scope should be comma-joined, got %s", uy)
	}

	gh := githubProvider()
	ugh := gh.AuthCodeURL("cid", "https://app/cb", "S", "")
	if strings.Contains(ugh, "code_challenge") {
		t.Errorf("github must not send PKCE params: %s", ugh)
	}
}

func assertIdentity(t *testing.T, id *Identity, subject, email string, verified bool, name, avatar string) {
	t.Helper()
	if id.Subject != subject {
		t.Errorf("subject = %q, want %q", id.Subject, subject)
	}
	if id.Email != email {
		t.Errorf("email = %q, want %q", id.Email, email)
	}
	if id.EmailVerified != verified {
		t.Errorf("verified = %v, want %v", id.EmailVerified, verified)
	}
	if id.Name != name {
		t.Errorf("name = %q, want %q", id.Name, name)
	}
	if avatar != "" && id.AvatarURL != avatar {
		t.Errorf("avatar = %q, want %q", id.AvatarURL, avatar)
	}
}
