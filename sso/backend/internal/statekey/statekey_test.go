package statekey

import (
	"bytes"
	"testing"
)

// master is a 32-byte test secret (the validated minimum length).
var master = []byte("0123456789abcdef0123456789abcdef")

// TestDeriveDeterministic: the same (master, label) always yields the same key —
// the property multiple replicas rely on to agree on the signing key.
func TestDeriveDeterministic(t *testing.T) {
	for _, label := range []string{LabelOAuth, LabelPasskey} {
		k1, err := Derive(master, label)
		if err != nil {
			t.Fatalf("Derive(%q) #1: %v", label, err)
		}
		k2, err := Derive(master, label)
		if err != nil {
			t.Fatalf("Derive(%q) #2: %v", label, err)
		}
		if !bytes.Equal(k1, k2) {
			t.Errorf("label %q: derivation not deterministic: %x != %x", label, k1, k2)
		}
		if len(k1) != keyLen {
			t.Errorf("label %q: key length = %d, want %d", label, len(k1), keyLen)
		}
	}
}

// TestDeriveLabelsDistinct: the OAuth and passkey labels yield independent keys
// from the same master (domain separation), so one cannot forge the other.
func TestDeriveLabelsDistinct(t *testing.T) {
	oauth, err := Derive(master, LabelOAuth)
	if err != nil {
		t.Fatalf("Derive oauth: %v", err)
	}
	pk, err := Derive(master, LabelPasskey)
	if err != nil {
		t.Fatalf("Derive passkey: %v", err)
	}
	if bytes.Equal(oauth, pk) {
		t.Errorf("SECURITY: distinct labels produced the same key: %x", oauth)
	}
}

// TestDeriveDiffersByMaster: a different master yields different keys for the same
// label (so rotating OAUTH_STATE_KEY actually changes the derived keys).
func TestDeriveDiffersByMaster(t *testing.T) {
	other := []byte("FEDCBA9876543210FEDCBA9876543210")
	a, err := Derive(master, LabelOAuth)
	if err != nil {
		t.Fatalf("Derive(master): %v", err)
	}
	b, err := Derive(other, LabelOAuth)
	if err != nil {
		t.Fatalf("Derive(other): %v", err)
	}
	if bytes.Equal(a, b) {
		t.Errorf("different masters produced the same key for label %q", LabelOAuth)
	}
}
