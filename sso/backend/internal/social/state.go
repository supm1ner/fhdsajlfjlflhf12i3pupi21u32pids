package social

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// state.go — the short-lived, HttpOnly, SameSite=Lax `cid_oauth` cookie that
// carries the OAuth flow state across the provider round-trip.
//
// Design (D2): on start we mint a random `state`, a PKCE verifier (for providers
// that support it), and remember the optional Hydra login_challenge + the
// "remember" preference. These travel in ONE signed cookie. On callback the
// query `state` MUST equal the cookie's `state` (the OAuth anti-CSRF control),
// after which the cookie is cleared.
//
// Integrity: the cookie value is HMAC-signed with a key supplied at construction.
// main.go generates a per-process random key (no new committed secret / env var /
// table, per the slice constraints); the cookie only needs to round-trip within a
// single ~10-minute OAuth flow, so a per-process key is sufficient. SameSite=Lax
// is required so the provider's top-level GET redirect back to the callback still
// sends the cookie.

// OAuthCookieName is the name of the social-login state cookie.
const OAuthCookieName = "cid_oauth"

// stateCookieTTL bounds how long a start→callback flow may take.
const stateCookieTTL = 10 * time.Minute

// stateRandomBytes is the entropy of the anti-CSRF state and the PKCE verifier.
const stateRandomBytes = 32

// oauthState is the payload carried (signed) in the cid_oauth cookie. JSON tags
// are camelCase per project convention, though the cookie is server-internal.
type oauthState struct {
	State          string `json:"state"`
	PKCEVerifier   string `json:"pkceVerifier,omitempty"`
	LoginChallenge string `json:"loginChallenge,omitempty"`
	Remember       bool   `json:"remember,omitempty"`
	Provider       string `json:"provider"`
	IssuedAt       int64  `json:"issuedAt"`
}

// errStateInvalid is returned when the cookie is missing, malformed, tampered, or
// expired. The callers treat all of these uniformly (reject without exchanging).
var errStateInvalid = errors.New("oauth state is missing, invalid, or expired")

// stateCodec signs and verifies oauthState cookies with an HMAC-SHA256 key.
type stateCodec struct {
	key          []byte
	cookieSecure bool
}

// newStateCodec builds a codec over the given signing key.
func newStateCodec(key []byte, cookieSecure bool) *stateCodec {
	return &stateCodec{key: key, cookieSecure: cookieSecure}
}

// newState creates a fresh oauthState with a random state token and (when the
// provider uses PKCE) a random verifier.
func newState(provider, loginChallenge string, remember, withPKCE bool) (*oauthState, error) {
	state, err := randToken()
	if err != nil {
		return nil, err
	}
	st := &oauthState{
		State:          state,
		LoginChallenge: loginChallenge,
		Remember:       remember,
		Provider:       provider,
		IssuedAt:       time.Now().Unix(),
	}
	if withPKCE {
		v, err := randToken()
		if err != nil {
			return nil, err
		}
		st.PKCEVerifier = v
	}
	return st, nil
}

// sign returns the cookie value: base64url(payload).base64url(hmac(payload)).
func (c *stateCodec) sign(st *oauthState) (string, error) {
	payload, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(c.mac([]byte(enc))), nil
}

// parse verifies the signature and TTL and returns the decoded state.
func (c *stateCodec) parse(value string) (*oauthState, error) {
	enc, sig, ok := strings.Cut(value, ".")
	if !ok || enc == "" || sig == "" {
		return nil, errStateInvalid
	}
	wantMAC, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return nil, errStateInvalid
	}
	if subtle.ConstantTimeCompare(wantMAC, c.mac([]byte(enc))) != 1 {
		return nil, errStateInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return nil, errStateInvalid
	}
	var st oauthState
	if err := json.Unmarshal(payload, &st); err != nil {
		return nil, errStateInvalid
	}
	if st.State == "" || st.Provider == "" {
		return nil, errStateInvalid
	}
	if time.Since(time.Unix(st.IssuedAt, 0)) > stateCookieTTL {
		return nil, errStateInvalid
	}
	return &st, nil
}

func (c *stateCodec) mac(b []byte) []byte {
	m := hmac.New(sha256.New, c.key)
	m.Write(b)
	return m.Sum(nil)
}

// write sets the signed cid_oauth cookie. SameSite=Lax (not Strict) so the
// provider's top-level redirect back to the callback still carries it.
func (c *stateCodec) write(w http.ResponseWriter, st *oauthState) error {
	value, err := c.sign(st)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     OAuthCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(stateCookieTTL.Seconds()),
	})
	return nil
}

// read parses and verifies the cid_oauth cookie from the request.
func (c *stateCodec) read(r *http.Request) (*oauthState, error) {
	cookie, err := r.Cookie(OAuthCookieName)
	if err != nil || cookie.Value == "" {
		return nil, errStateInvalid
	}
	return c.parse(cookie.Value)
}

// clear expires the cid_oauth cookie after the flow completes (or fails).
func (c *stateCodec) clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     OAuthCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// pkceChallenge returns the S256 code_challenge for a verifier:
// base64url(sha256(verifier)).
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// randToken returns a URL-safe random token (state / PKCE verifier).
func randToken() (string, error) {
	b := make([]byte, stateRandomBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewSigningKey returns a fresh random HMAC key for the state codec. main.go
// calls this once at startup so cid_oauth cookies are integrity-protected without
// a committed secret.
func NewSigningKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}
