package auth

import (
	"sync"
	"time"
)

// Lockout throttles repeated failed authentication for a key (typically an
// account) with INCREMENTAL BACKOFF, satisfying the password-authentication
// spec's "incremental backoff / temporarily refused" requirement — which a
// plain refilling token bucket cannot meet (it never escalates). The interface
// lets the in-memory implementation move to Redis/DB later so lockout survives
// restarts and works across replicas (design D6).
type Lockout interface {
	// Locked reports whether key is currently locked and, if so, the remaining
	// duration. It does not mutate state.
	Locked(key string) (bool, time.Duration)
	// Fail records a failed attempt; it returns whether the key is now locked and
	// for how long.
	Fail(key string) (locked bool, retryAfter time.Duration)
	// Reset clears the failure state for key (call after a successful auth).
	Reset(key string)
}

type lockoutEntry struct {
	failures    int
	lockedUntil time.Time
	lastSeen    time.Time
}

// MemoryLockout is an in-memory [Lockout]. After `threshold` consecutive
// failures, each further failure locks the key for base·2ⁿ (capped at max),
// where n grows with each failure past the threshold. A successful auth resets
// the counter. Idle entries are swept so memory does not grow unbounded.
type MemoryLockout struct {
	mu        sync.Mutex
	entries   map[string]*lockoutEntry
	threshold int
	base      time.Duration
	max       time.Duration
	now       func() time.Time
}

// Default backoff parameters: the threshold is configurable; the timing is
// fixed at sensible values (first lock 30s, doubling, capped at 15m).
const (
	lockoutBase = 30 * time.Second
	lockoutMax  = 15 * time.Minute
	lockoutIdle = time.Hour // evict entries untouched this long
)

// NewMemoryLockout builds an in-memory lockout. threshold<=0 disables locking
// (Fail/Locked become no-ops) so the limiter can be turned off via config.
func NewMemoryLockout(threshold int) *MemoryLockout {
	return &MemoryLockout{
		entries:   make(map[string]*lockoutEntry),
		threshold: threshold,
		base:      lockoutBase,
		max:       lockoutMax,
		now:       time.Now,
	}
}

// Locked implements Lockout.
func (l *MemoryLockout) Locked(key string) (bool, time.Duration) {
	if l.threshold <= 0 {
		return false, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return false, 0
	}
	now := l.now()
	if now.Before(e.lockedUntil) {
		return true, e.lockedUntil.Sub(now)
	}
	return false, 0
}

// Fail implements Lockout.
func (l *MemoryLockout) Fail(key string) (bool, time.Duration) {
	if l.threshold <= 0 {
		return false, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweepLocked(now)

	e, ok := l.entries[key]
	if !ok {
		e = &lockoutEntry{}
		l.entries[key] = e
	}
	e.failures++
	e.lastSeen = now

	if e.failures >= l.threshold {
		over := e.failures - l.threshold // 0,1,2,...
		d := l.base
		for i := 0; i < over && d < l.max; i++ {
			d *= 2
		}
		if d > l.max || d <= 0 {
			d = l.max
		}
		e.lockedUntil = now.Add(d)
		return true, d
	}
	return false, 0
}

// Reset implements Lockout.
func (l *MemoryLockout) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// sweepLocked evicts entries untouched beyond lockoutIdle. Caller holds l.mu.
func (l *MemoryLockout) sweepLocked(now time.Time) {
	for k, e := range l.entries {
		if now.Sub(e.lastSeen) > lockoutIdle {
			delete(l.entries, k)
		}
	}
}
