package httpx

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter is the interface auth handlers depend on so the implementation can
// move from in-memory to Redis later without touching call sites (design D6).
type RateLimiter interface {
	// Allow reports whether a request keyed by the given identifier (typically
	// an IP or an account email) may proceed now.
	Allow(key string) bool
}

// bucket pairs a token-bucket limiter with a last-seen timestamp for eviction.
type bucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// TokenBucketLimiter is an in-memory per-key token-bucket rate limiter built on
// golang.org/x/time/rate. A background sweeper evicts idle keys so memory does
// not grow unbounded under a wide IP/account fan-out.
type TokenBucketLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rps     rate.Limit
	burst   int
	ttl     time.Duration
	now     func() time.Time
}

// NewTokenBucketLimiter creates a limiter allowing rps requests/second with the
// given burst. Idle keys are evicted after ttl (default 10m when zero).
func NewTokenBucketLimiter(rps float64, burst int, ttl time.Duration) *TokenBucketLimiter {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	l := &TokenBucketLimiter{
		buckets: make(map[string]*bucket),
		rps:     rate.Limit(rps),
		burst:   burst,
		ttl:     ttl,
		now:     time.Now,
	}
	return l
}

// Allow implements RateLimiter.
func (l *TokenBucketLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.buckets[key] = b
	}
	b.lastSeen = now

	// Opportunistic sweep: cheap, bounded, avoids a separate goroutine.
	l.sweepLocked(now)

	return b.limiter.Allow()
}

// sweepLocked evicts buckets idle beyond ttl. Caller must hold l.mu.
func (l *TokenBucketLimiter) sweepLocked(now time.Time) {
	for k, b := range l.buckets {
		if now.Sub(b.lastSeen) > l.ttl {
			delete(l.buckets, k)
		}
	}
}

// RateLimitByIP returns middleware that rejects requests exceeding the per-IP
// limit with a 429 problem. Per-account limiting is applied inside the auth
// handlers (where the account identifier is known after body parsing).
func RateLimitByIP(limiter RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow("ip:" + ClientIP(r)) {
				w.Header().Set("Retry-After", "1")
				WriteProblem(w, r, http.StatusTooManyRequests, "too many requests, please slow down")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
