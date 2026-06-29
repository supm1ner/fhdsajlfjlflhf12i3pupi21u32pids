package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestGenerateSessionTokenAndHash(t *testing.T) {
	t.Parallel()
	token, id, err := GenerateSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || id == "" {
		t.Fatal("empty token or id")
	}
	if token == id {
		t.Fatal("the stored id must not equal the cookie token")
	}
	// id must be sha256(token) hex.
	sum := sha256.Sum256([]byte(token))
	if id != hex.EncodeToString(sum[:]) {
		t.Fatal("id is not sha256(token) hex")
	}
	if len(id) != 64 {
		t.Fatalf("sha256 hex id len = %d, want 64", len(id))
	}
}

func TestSessionTokensAreUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		tok, _, err := GenerateSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		if seen[tok] {
			t.Fatal("duplicate session token generated")
		}
		seen[tok] = true
	}
}

func TestGenerateResetToken(t *testing.T) {
	t.Parallel()
	token, hash, err := GenerateResetToken()
	if err != nil {
		t.Fatal(err)
	}
	if hash != HashToken(token) {
		t.Fatal("reset hash != sha256(token)")
	}
	if token == hash {
		t.Fatal("token and stored hash must differ")
	}
}

func TestHashTokenDeterministic(t *testing.T) {
	t.Parallel()
	// Hashing the same input twice must yield the same digest.
	first, second := HashToken("abc"), HashToken("abc")
	if first != second {
		t.Fatal("HashToken not deterministic")
	}
	if HashToken("abc") == HashToken("abd") {
		t.Fatal("HashToken collision for different inputs")
	}
}
