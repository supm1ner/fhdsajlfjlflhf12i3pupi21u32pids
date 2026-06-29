// Package statekey derives the per-purpose HMAC signing keys for cotton-id's
// short-lived ceremony-state cookies (the social cid_oauth state cookie and the
// passkey cid_wa ceremony cookie) from one optional shared secret.
//
// Why: each cookie is signed with an HMAC key. With a per-process random key, a
// ceremony begun on one backend replica cannot be finished on another, so the
// per-process default only works single-instance. When OAUTH_STATE_KEY is set, we
// derive BOTH cookie keys from it via HKDF-SHA256 with DISTINCT info labels, so:
//   - every replica derives the same two keys (multi-replica works), and
//   - the social and passkey keys are cryptographically independent (domain
//     separation): leaking or cross-using one cannot forge the other.
//
// HKDF (RFC 5869) with a fixed label is deterministic, so the same master key +
// label always yields the same derived key — the property the replicas rely on.
package statekey

import (
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	// LabelOAuth is the HKDF info label for the social (cid_oauth) state cookie key.
	LabelOAuth = "cotton-id/oauth-state"
	// LabelPasskey is the HKDF info label for the passkey (cid_wa) ceremony cookie key.
	LabelPasskey = "cotton-id/passkey-state"

	// keyLen is the derived key length in bytes (256-bit HMAC-SHA256 key), matching
	// the per-process random keys the social/passkey packages otherwise generate.
	keyLen = 32
)

// Derive returns a deterministic keyLen-byte key derived from master and label via
// HKDF-SHA256 (no salt; the label is the info/context string that separates the
// purposes). The same (master, label) always yields the same key, so replicas
// agree; distinct labels yield independent keys (domain separation).
//
// master MUST be the shared secret bytes (callers validate it is non-empty and
// long enough before calling); label is one of the Label* constants.
func Derive(master []byte, label string) ([]byte, error) {
	r := hkdf.New(sha256.New, master, nil /* salt */, []byte(label))
	key := make([]byte, keyLen)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return key, nil
}
