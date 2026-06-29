package social

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func testCodec(t *testing.T) *stateCodec {
	t.Helper()
	key, err := NewSigningKey()
	if err != nil {
		t.Fatalf("signing key: %v", err)
	}
	return newStateCodec(key, false)
}

func TestStateSignParseRoundTrip(t *testing.T) {
	c := testCodec(t)
	st, err := newState(ProviderGoogle, "chal123", true, true)
	if err != nil {
		t.Fatalf("newState: %v", err)
	}
	if st.PKCEVerifier == "" {
		t.Fatalf("expected a PKCE verifier when withPKCE=true")
	}

	value, err := c.sign(st)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := c.parse(value)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.State != st.State || got.PKCEVerifier != st.PKCEVerifier ||
		got.LoginChallenge != "chal123" || !got.Remember || got.Provider != ProviderGoogle {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, st)
	}
}

func TestStateTamperRejected(t *testing.T) {
	c := testCodec(t)
	st, _ := newState(ProviderVK, "", false, true)
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
	st, _ := newState(ProviderYandex, "", false, false)
	st.IssuedAt = time.Now().Add(-2 * stateCookieTTL).Unix()
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
	// Valid signature over an empty-state payload must still be rejected (state
	// and provider are required).
	payload, _ := json.Marshal(oauthState{IssuedAt: time.Now().Unix()})
	enc := base64.RawURLEncoding.EncodeToString(payload)
	value := enc + "." + base64.RawURLEncoding.EncodeToString(c.mac([]byte(enc)))
	if _, err := c.parse(value); err == nil {
		t.Error("empty state/provider should be rejected")
	}
}

func TestPKCEChallengeDeterministic(t *testing.T) {
	a := pkceChallenge("verifier-xyz")
	b := pkceChallenge("verifier-xyz")
	if a != b {
		t.Fatal("pkceChallenge must be deterministic")
	}
	if a == pkceChallenge("other") {
		t.Fatal("different verifiers must yield different challenges")
	}
	if _, err := base64.RawURLEncoding.DecodeString(a); err != nil {
		t.Fatalf("challenge must be base64url: %v", err)
	}
}
