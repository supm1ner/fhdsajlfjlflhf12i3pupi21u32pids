package passkey

import (
	"sync"
	"time"
)

// challengeGuard enforces single-use of a WebAuthn ceremony challenge SERVER-SIDE,
// closing the replay window for counterless authenticators. The signed cid_wa
// cookie is cleared on finish, but a client-controlled cookie clear is not
// authoritative: an attacker who captured the cookie + assertion within the TTL
// could otherwise replay the finish. This guard records each consumed challenge
// (for its TTL) and refuses a second use.
//
// It is in-memory and single-instance (consistent with the rate limiter / lockout
// / per-process state key); a multi-replica deployment needs a shared store. The
// challenge values are high-entropy and bounded in number by the TTL, so the map
// stays small.
type challengeGuard struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
	now  func() time.Time
}

// newChallengeGuard builds a guard whose entries expire after ttl.
func newChallengeGuard(ttl time.Duration) *challengeGuard {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &challengeGuard{seen: make(map[string]time.Time), ttl: ttl, now: time.Now}
}

// consume records the challenge as used and reports whether it was FRESH. It
// returns false when the challenge was already consumed (a replay) or is empty.
func (g *challengeGuard) consume(challenge string) bool {
	if challenge == "" {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	// Opportunistic eviction of expired entries (bounded, cheap).
	for k, t := range g.seen {
		if now.Sub(t) > g.ttl {
			delete(g.seen, k)
		}
	}
	if _, used := g.seen[challenge]; used {
		return false
	}
	g.seen[challenge] = now
	return true
}
