package passkey

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func testCodec(t *testing.T) *stateCodec {
	t.Helper()
	key, err := NewSigningKey()
	if err != nil {
		t.Fatalf("signing key: %v", err)
	}
	return newStateCodec(key, false)
}

func sampleSession() *webauthn.SessionData {
	return &webauthn.SessionData{
		Challenge:      "Y2hhbGxlbmdlLXZhbHVl",
		RelyingPartyID: "localhost",
		UserID:         []byte{0x01, 0x02, 0x03},
	}
}

func TestRegisterStateRoundTrip(t *testing.T) {
	c := testCodec(t)
	st := newRegisterState(sampleSession(), "user-123")

	value, err := c.sign(st)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := c.parse(value)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Kind != ceremonyRegister {
		t.Errorf("kind = %q, want register", got.Kind)
	}
	if got.UserID != "user-123" {
		t.Errorf("userID = %q, want user-123", got.UserID)
	}
	if got.Session.Challenge != st.Session.Challenge {
		t.Errorf("challenge = %q, want %q", got.Session.Challenge, st.Session.Challenge)
	}
}

func TestLoginStateRoundTrip(t *testing.T) {
	c := testCodec(t)
	st := newLoginState(sampleSession(), "chal-xyz")

	value, _ := c.sign(st)
	got, err := c.parse(value)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Kind != ceremonyLogin {
		t.Errorf("kind = %q, want login", got.Kind)
	}
	if got.LoginChallenge != "chal-xyz" {
		t.Errorf("loginChallenge = %q, want chal-xyz", got.LoginChallenge)
	}
}

func TestStateTamperRejected(t *testing.T) {
	c := testCodec(t)
	st := newLoginState(sampleSession(), "")
	value, _ := c.sign(st)

	// Flip a byte in the payload portion.
	tampered := "A" + value[1:]
	if _, err := c.parse(tampered); err == nil {
		t.Fatal("expected tampered cookie to be rejected")
	}

	// A different key must not validate the signature.
	other := newStateCodec([]byte("a-totally-different-key-0123456789"), false)
	if _, err := other.parse(value); err == nil {
		t.Fatal("expected signature mismatch under a different key")
	}
}

func TestStateExpired(t *testing.T) {
	c := testCodec(t)
	st := newLoginState(sampleSession(), "")
	st.IssuedAt = time.Now().Add(-2 * cookieTTL).Unix()
	value, _ := c.sign(st)
	if _, err := c.parse(value); err == nil {
		t.Fatal("expected expired state to be rejected")
	}
}

func TestStateMalformed(t *testing.T) {
	c := testCodec(t)
	for _, v := range []string{"", "no-dot", ".", "a.b", "onlyone."} {
		if _, err := c.parse(v); err == nil {
			t.Errorf("parse(%q) should fail", v)
		}
	}
	// A validly-signed payload with no challenge / unknown kind must still fail.
	payload, _ := json.Marshal(ceremonyState{Kind: ceremonyLogin, IssuedAt: time.Now().Unix()})
	enc := base64.RawURLEncoding.EncodeToString(payload)
	value := enc + "." + base64.RawURLEncoding.EncodeToString(c.mac([]byte(enc)))
	if _, err := c.parse(value); err == nil {
		t.Error("empty challenge should be rejected")
	}

	payload2, _ := json.Marshal(ceremonyState{Kind: "bogus", Session: *sampleSession(), IssuedAt: time.Now().Unix()})
	enc2 := base64.RawURLEncoding.EncodeToString(payload2)
	value2 := enc2 + "." + base64.RawURLEncoding.EncodeToString(c.mac([]byte(enc2)))
	if _, err := c.parse(value2); err == nil {
		t.Error("unknown ceremony kind should be rejected")
	}
}
