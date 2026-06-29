package passkey

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

	"github.com/go-webauthn/webauthn/webauthn"
)

// state.go — the short-lived, HttpOnly, SameSite=Lax `cid_wa` cookie that carries
// the in-flight WebAuthn ceremony state (the per-ceremony challenge + expected
// parameters) across the begin→finish round-trip, mirroring the social
// `cid_oauth` state codec.
//
// Design (D3): instead of a server-side ceremony table, the library's
// webauthn.SessionData is serialized into ONE signed cookie. Registration
// ceremonies also bind the cookie to the authenticated user id (UserID); login
// ceremonies carry the optional Hydra login_challenge to continue. Integrity is
// an HMAC-SHA256 over the payload with a per-process key minted in main.go (no
// new committed secret/env var/table — the cookie only needs to round-trip within
// a single ~10-minute ceremony). SameSite=Lax matches the rest of cotton-id's
// cookies; the SPA's fetch POSTs are same-site so it is sent.

// CookieName is the WebAuthn ceremony-state cookie name.
const CookieName = "cid_wa"

// cookieTTL bounds how long a begin→finish ceremony may take.
const cookieTTL = 10 * time.Minute

// errStateInvalid is returned when the cookie is missing, malformed, tampered, or
// expired. Callers treat all cases uniformly (reject the ceremony).
var errStateInvalid = errors.New("webauthn ceremony state is missing, invalid, or expired")

// ceremonyKind distinguishes the two ceremonies so a registration cookie can
// never be replayed into a login finish or vice-versa.
type ceremonyKind string

const (
	ceremonyRegister ceremonyKind = "register"
	ceremonyLogin    ceremonyKind = "login"
)

// ceremonyState is the payload carried (signed) in the cid_wa cookie. JSON tags
// are camelCase per project convention, though the cookie is server-internal.
type ceremonyState struct {
	Kind ceremonyKind `json:"kind"`
	// Session is the library's per-ceremony challenge + expected parameters.
	Session webauthn.SessionData `json:"session"`
	// UserID binds a registration ceremony to the authenticated account (hex of
	// the UUID); empty for login ceremonies.
	UserID string `json:"userId,omitempty"`
	// LoginChallenge is the optional in-progress Hydra login challenge to continue
	// after a successful login; empty otherwise.
	LoginChallenge string `json:"loginChallenge,omitempty"`
	IssuedAt       int64  `json:"issuedAt"`
}

// stateCodec signs and verifies cid_wa cookies with an HMAC-SHA256 key.
type stateCodec struct {
	key          []byte
	cookieSecure bool
}

// newStateCodec builds a codec over the given signing key.
func newStateCodec(key []byte, cookieSecure bool) *stateCodec {
	return &stateCodec{key: key, cookieSecure: cookieSecure}
}

// sign returns the cookie value: base64url(payload).base64url(hmac(payload)).
func (c *stateCodec) sign(st *ceremonyState) (string, error) {
	payload, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(c.mac([]byte(enc))), nil
}

// parse verifies the signature and TTL and returns the decoded state.
func (c *stateCodec) parse(value string) (*ceremonyState, error) {
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
	var st ceremonyState
	if err := json.Unmarshal(payload, &st); err != nil {
		return nil, errStateInvalid
	}
	if st.Session.Challenge == "" {
		return nil, errStateInvalid
	}
	if st.Kind != ceremonyRegister && st.Kind != ceremonyLogin {
		return nil, errStateInvalid
	}
	if time.Since(time.Unix(st.IssuedAt, 0)) > cookieTTL {
		return nil, errStateInvalid
	}
	return &st, nil
}

func (c *stateCodec) mac(b []byte) []byte {
	m := hmac.New(sha256.New, c.key)
	m.Write(b)
	return m.Sum(nil)
}

// write sets the signed cid_wa cookie.
func (c *stateCodec) write(w http.ResponseWriter, st *ceremonyState) error {
	value, err := c.sign(st)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(cookieTTL.Seconds()),
	})
	return nil
}

// read parses and verifies the cid_wa cookie from the request.
func (c *stateCodec) read(r *http.Request) (*ceremonyState, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" {
		return nil, errStateInvalid
	}
	return c.parse(cookie.Value)
}

// clear expires the cid_wa cookie after the ceremony completes (or fails).
func (c *stateCodec) clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   c.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// newRegisterState wraps a registration SessionData bound to userID.
func newRegisterState(session *webauthn.SessionData, userID string) *ceremonyState {
	return &ceremonyState{
		Kind:     ceremonyRegister,
		Session:  *session,
		UserID:   userID,
		IssuedAt: time.Now().Unix(),
	}
}

// newLoginState wraps a login SessionData with the optional login_challenge.
func newLoginState(session *webauthn.SessionData, loginChallenge string) *ceremonyState {
	return &ceremonyState{
		Kind:           ceremonyLogin,
		Session:        *session,
		LoginChallenge: loginChallenge,
		IssuedAt:       time.Now().Unix(),
	}
}

// NewSigningKey returns a fresh random HMAC key for the cid_wa state codec.
// main.go calls this once at startup so the cookie is integrity-protected without
// a committed secret (a per-process key suffices: the cookie only round-trips
// within a single short ceremony).
func NewSigningKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}
