package passkey

import (
	"testing"
	"time"
)

func TestChallengeGuardSingleUse(t *testing.T) {
	t.Parallel()
	g := newChallengeGuard(time.Minute)

	if !g.consume("chal-1") {
		t.Fatal("first use of a challenge must be allowed")
	}
	if g.consume("chal-1") {
		t.Fatal("second use of the same challenge must be refused (replay)")
	}
	if !g.consume("chal-2") {
		t.Fatal("a different challenge must be allowed independently")
	}
	if g.consume("") {
		t.Fatal("an empty challenge must be refused")
	}
}

func TestChallengeGuardEvictsExpired(t *testing.T) {
	t.Parallel()
	g := newChallengeGuard(10 * time.Millisecond)
	base := time.Unix(1_700_000_000, 0)
	g.now = func() time.Time { return base }

	g.consume("old")
	// Advance past the TTL: the next consume sweeps "old", so reusing it is allowed
	// again (it has fully expired) and the map does not grow unbounded.
	g.now = func() time.Time { return base.Add(time.Second) }
	if !g.consume("new") {
		t.Fatal("fresh challenge after eviction should be allowed")
	}
	if _, present := g.seen["old"]; present {
		t.Fatal("expired challenge should have been evicted")
	}
}
