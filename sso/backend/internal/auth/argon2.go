package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params are the cost parameters for argon2id hashing. The defaults match
// the build contract §5: time=3, memory=64 MiB, threads=4, salt=16, key=32.
// They are encoded into every PHC string so they can evolve and old hashes can
// be re-hashed on the next successful login.
type Argon2Params struct {
	Time    uint32 // number of iterations
	Memory  uint32 // memory in KiB
	Threads uint8  // parallelism
	SaltLen uint32 // salt length in bytes
	KeyLen  uint32 // derived key length in bytes
}

// DefaultArgon2Params returns the contract defaults.
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		Time:    3,
		Memory:  64 * 1024, // 64 MiB
		Threads: 4,
		SaltLen: 16,
		KeyLen:  32,
	}
}

// argon2Version is the algorithm version encoded in the PHC string.
const argon2Version = argon2.Version

var (
	// ErrInvalidHash is returned when a stored PHC string is malformed.
	ErrInvalidHash = errors.New("invalid argon2 hash format")
	// ErrIncompatibleVersion is returned for an unsupported argon2 version.
	ErrIncompatibleVersion = errors.New("incompatible argon2 version")
)

// HashPassword hashes password with argon2id using p and returns a standard PHC
// encoded string: $argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>.
func HashPassword(password string, p Argon2Params) (string, error) {
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Key := base64.RawStdEncoding.EncodeToString(key)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version, p.Memory, p.Time, p.Threads, b64Salt, b64Key), nil
}

// VerifyPassword reports whether password matches the argon2id PHC-encoded hash,
// using a constant-time comparison over the derived key. A malformed hash
// returns an error (distinct from a non-match, which returns false, nil).
func VerifyPassword(password, encodedHash string) (bool, error) {
	p, salt, key, err := decodeHash(encodedHash)
	if err != nil {
		return false, err
	}
	other := argon2.IDKey([]byte(password), salt, p.Time, p.Memory, p.Threads, p.KeyLen)
	// ConstantTimeCompare is constant-time with respect to the hash bytes.
	if subtle.ConstantTimeCompare(key, other) == 1 {
		return true, nil
	}
	return false, nil
}

// NeedsRehash reports whether a stored hash uses parameters weaker than the
// current target p, indicating it should be re-hashed on next login.
func NeedsRehash(encodedHash string, p Argon2Params) bool {
	cur, _, key, err := decodeHash(encodedHash)
	if err != nil {
		return true
	}
	return cur.Time != p.Time ||
		cur.Memory != p.Memory ||
		cur.Threads != p.Threads ||
		uint32(len(key)) != p.KeyLen
}

// decodeHash parses a PHC argon2id string back into its params, salt, and key.
func decodeHash(encodedHash string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	// "" / argon2id / v=19 / m=..,t=..,p=.. / salt / key
	if len(parts) != 6 {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	if parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	if version != argon2Version {
		return Argon2Params{}, nil, nil, ErrIncompatibleVersion
	}

	var p Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}

	p.SaltLen = uint32(len(salt))
	p.KeyLen = uint32(len(key))
	return p, salt, key, nil
}
