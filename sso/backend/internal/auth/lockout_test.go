package auth

import (
	"testing"
	"time"
)

func TestLockoutEngagesAfterThreshold(t *testing.T) {
	t.Parallel()
	l := NewMemoryLockout(3)
	base := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return base }

	// Below threshold: failures accumulate but do not lock.
	for i := 0; i < 2; i++ {
		if locked, _ := l.Fail("acct:a"); locked {
			t.Fatalf("should not lock before threshold (attempt %d)", i+1)
		}
	}
	if locked, _ := l.Locked("acct:a"); locked {
		t.Fatal("must not be locked below threshold")
	}

	// The threshold-th failure locks for the base duration.
	locked, d := l.Fail("acct:a")
	if !locked || d != lockoutBase {
		t.Fatalf("expected lock for %v at threshold, got locked=%v d=%v", lockoutBase, locked, d)
	}
	if isLocked, _ := l.Locked("acct:a"); !isLocked {
		t.Fatal("Locked should report true while within the lock window")
	}
}

func TestLockoutBackoffGrowsAndCaps(t *testing.T) {
	t.Parallel()
	l := NewMemoryLockout(1) // every failure locks
	base := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return base }

	_, d0 := l.Fail("k") // over=0 → base
	_, d1 := l.Fail("k") // over=1 → base*2
	_, d2 := l.Fail("k") // over=2 → base*4
	if d0 != lockoutBase || d1 != lockoutBase*2 || d2 != lockoutBase*4 {
		t.Fatalf("backoff not doubling: %v %v %v", d0, d1, d2)
	}
	// Drive far past the cap and ensure it never exceeds the max.
	var last time.Duration
	for i := 0; i < 20; i++ {
		_, last = l.Fail("k")
	}
	if last != lockoutMax {
		t.Fatalf("backoff should cap at %v, got %v", lockoutMax, last)
	}
}

func TestLockoutExpiresAndResets(t *testing.T) {
	t.Parallel()
	l := NewMemoryLockout(1)
	base := time.Unix(1_700_000_000, 0)
	now := base
	l.now = func() time.Time { return now }

	_, d := l.Fail("k")
	if locked, _ := l.Locked("k"); !locked {
		t.Fatal("should be locked immediately after failure")
	}
	// Advance past the lock window: no longer locked.
	now = base.Add(d + time.Second)
	if locked, _ := l.Locked("k"); locked {
		t.Fatal("lock should have expired")
	}

	// A successful auth (Reset) clears the counter so backoff restarts from base.
	now = base
	l.Fail("k")
	l.Reset("k")
	_, d2 := l.Fail("k")
	if d2 != lockoutBase {
		t.Fatalf("after Reset, backoff should restart at base, got %v", d2)
	}
}

func TestLockoutThresholdZeroDisables(t *testing.T) {
	t.Parallel()
	l := NewMemoryLockout(0)
	for i := 0; i < 100; i++ {
		if locked, _ := l.Fail("k"); locked {
			t.Fatal("threshold<=0 must disable locking")
		}
	}
	if locked, _ := l.Locked("k"); locked {
		t.Fatal("disabled lockout must never report locked")
	}
}

func TestLockoutIsolatesKeys(t *testing.T) {
	t.Parallel()
	l := NewMemoryLockout(1)
	l.Fail("acct:a")
	if locked, _ := l.Locked("acct:b"); locked {
		t.Fatal("a failure for one account must not lock another")
	}
}
