package auth

import (
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()
	const pw = "Correct-Horse-9!"

	hash, err := HashPassword(pw, DefaultArgon2Params())
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("hash is not a PHC argon2id string: %q", hash)
	}
	if strings.Contains(hash, pw) {
		t.Fatal("hash must not contain the plaintext")
	}

	ok, err := VerifyPassword(pw, hash)
	if err != nil {
		t.Fatalf("VerifyPassword(correct): %v", err)
	}
	if !ok {
		t.Fatal("VerifyPassword(correct) = false, want true")
	}

	ok, err = VerifyPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword(wrong): %v", err)
	}
	if ok {
		t.Fatal("VerifyPassword(wrong) = true, want false")
	}
}

func TestHashUniquePerCall(t *testing.T) {
	t.Parallel()
	const pw = "Repeat-Me-1!"
	h1, err := HashPassword(pw, DefaultArgon2Params())
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashPassword(pw, DefaultArgon2Params())
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("two hashes of the same password are identical; salt is not random")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=65536,t=3$onlyfourparts$x",
		"$bcrypt$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
	}
	for _, c := range cases {
		if _, err := VerifyPassword("x", c); err == nil {
			t.Errorf("VerifyPassword(%q) expected error, got nil", c)
		}
	}
}

func TestDecodeHashRoundTrip(t *testing.T) {
	t.Parallel()
	p := DefaultArgon2Params()
	hash, err := HashPassword("round-trip-1A!", p)
	if err != nil {
		t.Fatal(err)
	}
	got, salt, key, err := decodeHash(hash)
	if err != nil {
		t.Fatalf("decodeHash: %v", err)
	}
	if got.Time != p.Time || got.Memory != p.Memory || got.Threads != p.Threads {
		t.Errorf("decoded params %+v, want %+v", got, p)
	}
	if uint32(len(salt)) != p.SaltLen {
		t.Errorf("salt len = %d, want %d", len(salt), p.SaltLen)
	}
	if uint32(len(key)) != p.KeyLen {
		t.Errorf("key len = %d, want %d", len(key), p.KeyLen)
	}
}

func TestNeedsRehash(t *testing.T) {
	t.Parallel()
	cur := DefaultArgon2Params()
	hash, err := HashPassword("rehash-Me-1!", cur)
	if err != nil {
		t.Fatal(err)
	}
	if NeedsRehash(hash, cur) {
		t.Error("NeedsRehash = true for matching params, want false")
	}

	stronger := cur
	stronger.Time = cur.Time + 1
	if !NeedsRehash(hash, stronger) {
		t.Error("NeedsRehash = false for upgraded params, want true")
	}

	if !NeedsRehash("garbage", cur) {
		t.Error("NeedsRehash = false for malformed hash, want true")
	}
}

// The dummyHash used for timing-equalization must be a valid PHC string so the
// unknown-account verify path doesn't error.
func TestDummyHashIsValid(t *testing.T) {
	t.Parallel()
	if _, err := VerifyPassword("anything", dummyHash); err != nil {
		t.Fatalf("dummyHash must decode cleanly: %v", err)
	}
}
