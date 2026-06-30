package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"

	"sunrise/chat/server/logs"
)

// TestMain initializes the shared logger so handler code paths that log don't panic.
func TestMain(m *testing.M) {
	logs.Init(io.Discard, "stdFlags")
	os.Exit(m.Run())
}

// idpFixture is a fake OIDC provider: it serves a discovery document and a JWKS
// for a freshly generated RSA key, and can sign ID tokens with that key.
type idpFixture struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	kid    string
}

func newIDPFixture(t *testing.T) *idpFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	f := &idpFixture{key: key, kid: "test-key-1"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":   f.issuer(),
			"jwks_uri": f.server.URL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		jwk := jose.JSONWebKey{Key: &key.PublicKey, KeyID: f.kid, Algorithm: "RS256", Use: "sig"}
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}})
	})
	f.server = httptest.NewServer(mux)
	return f
}

// issuer returns the issuer URL with a trailing slash, matching typical IdP behavior.
func (f *idpFixture) issuer() string { return f.server.URL + "/" }

func (f *idpFixture) close() { f.server.Close() }

// sign mints an ID token with the given claims, signed by the fixture's key.
func (f *idpFixture) sign(t *testing.T, c claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	token.Header["kid"] = f.kid
	s, err := token.SignedString(f.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return s
}

// newHandler builds an initialized authenticator pointed at the fixture.
func newHandler(t *testing.T, f *idpFixture, clientID string) *authenticator {
	t.Helper()
	conf, _ := json.Marshal(map[string]any{
		"issuer":             f.issuer(),
		"client_id":          clientID,
		"allow_new_accounts": true,
		"add_to_tags":        true,
	})
	a := &authenticator{}
	if err := a.Init(conf, "oidc"); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return a
}

func goodClaims(f *idpFixture, aud string) claims {
	now := time.Now()
	return claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    f.issuer(),
			Subject:   "user-sub-123",
			Audience:  jwt.ClaimStrings{aud},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		Email:         "alice@example.com",
		EmailVerified: true,
		Name:          "Alice",
	}
}

func TestValidate_Valid(t *testing.T) {
	f := newIDPFixture(t)
	defer f.close()
	a := newHandler(t, f, "sunrise-messenger")

	tok := f.sign(t, goodClaims(f, "sunrise-messenger"))
	cl, err := a.validate([]byte(tok))
	if err != nil {
		t.Fatalf("validate returned error for a valid token: %v", err)
	}
	if cl.Subject != "user-sub-123" {
		t.Errorf("subject = %q, want %q", cl.Subject, "user-sub-123")
	}
	if cl.Email != "alice@example.com" {
		t.Errorf("email = %q, want %q", cl.Email, "alice@example.com")
	}
	if !cl.EmailVerified {
		t.Errorf("email_verified = false, want true")
	}
}

func TestValidate_WrongIssuer(t *testing.T) {
	f := newIDPFixture(t)
	defer f.close()
	a := newHandler(t, f, "sunrise-messenger")

	c := goodClaims(f, "sunrise-messenger")
	c.Issuer = "https://evil.example.com/"
	tok := f.sign(t, c)
	if _, err := a.validate([]byte(tok)); err == nil {
		t.Fatal("validate accepted a token with the wrong issuer")
	}
}

func TestValidate_WrongAudience(t *testing.T) {
	f := newIDPFixture(t)
	defer f.close()
	a := newHandler(t, f, "sunrise-messenger")

	tok := f.sign(t, goodClaims(f, "some-other-client"))
	if _, err := a.validate([]byte(tok)); err == nil {
		t.Fatal("validate accepted a token with the wrong audience")
	}
}

func TestValidate_Expired(t *testing.T) {
	f := newIDPFixture(t)
	defer f.close()
	a := newHandler(t, f, "sunrise-messenger")

	c := goodClaims(f, "sunrise-messenger")
	now := time.Now()
	c.ExpiresAt = jwt.NewNumericDate(now.Add(-time.Hour))
	c.IssuedAt = jwt.NewNumericDate(now.Add(-2 * time.Hour))
	tok := f.sign(t, c)
	if _, err := a.validate([]byte(tok)); err == nil {
		t.Fatal("validate accepted an expired token")
	}
}

func TestValidate_WrongKey(t *testing.T) {
	f := newIDPFixture(t)
	defer f.close()
	a := newHandler(t, f, "sunrise-messenger")

	// Sign with a different key than the one published in the JWKS.
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, goodClaims(f, "sunrise-messenger"))
	token.Header["kid"] = f.kid
	tok, err := token.SignedString(other)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := a.validate([]byte(tok)); err == nil {
		t.Fatal("validate accepted a token signed by an unknown key")
	}
}

func TestValidate_Malformed(t *testing.T) {
	f := newIDPFixture(t)
	defer f.close()
	a := newHandler(t, f, "sunrise-messenger")

	if _, err := a.validate([]byte("   ")); err == nil {
		t.Fatal("validate accepted an empty token")
	}
	if _, err := a.validate([]byte("not.a.jwt")); err == nil {
		t.Fatal("validate accepted a malformed token")
	}
}

func TestInit_DisabledWithoutIssuer(t *testing.T) {
	a := &authenticator{}
	conf, _ := json.Marshal(map[string]any{"client_id": "x"})
	if err := a.Init(conf, "oidc"); err != nil {
		t.Fatalf("Init should succeed (disabled) without an issuer, got: %v", err)
	}
	// A disabled scheme rejects auth rather than panicking on the nil parser.
	if _, _, err := a.Authenticate([]byte("anything"), ""); err == nil {
		t.Fatal("disabled scheme should reject Authenticate")
	}
}

func TestInit_RequiresAudienceSource(t *testing.T) {
	a := &authenticator{}
	conf, _ := json.Marshal(map[string]any{"issuer": "http://localhost:4444/"})
	if err := a.Init(conf, "oidc"); err == nil {
		t.Fatal("Init succeeded without client_id or audiences")
	}
}

func TestGetRealName(t *testing.T) {
	a := &authenticator{}
	if a.GetRealName() != "oidc" {
		t.Errorf("GetRealName = %q, want oidc", a.GetRealName())
	}
}
