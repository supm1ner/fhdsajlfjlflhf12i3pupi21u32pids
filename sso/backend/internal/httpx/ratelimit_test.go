package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenBucketAllowsBurstThenBlocks(t *testing.T) {
	t.Parallel()
	// rps very low so the bucket doesn't refill within the test; burst=3.
	l := NewTokenBucketLimiter(0.0001, 3, time.Minute)

	allowed := 0
	for i := 0; i < 10; i++ {
		if l.Allow("ip:1.2.3.4") {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("allowed %d requests, want exactly burst=3", allowed)
	}
}

func TestTokenBucketIsolatesKeys(t *testing.T) {
	t.Parallel()
	l := NewTokenBucketLimiter(0.0001, 1, time.Minute)

	if !l.Allow("ip:a") {
		t.Fatal("first request for key a should pass")
	}
	if l.Allow("ip:a") {
		t.Fatal("second request for key a should be blocked")
	}
	// A different key has its own bucket.
	if !l.Allow("ip:b") {
		t.Fatal("first request for key b should pass independently")
	}
}

func TestTokenBucketEvictsIdleKeys(t *testing.T) {
	t.Parallel()
	l := NewTokenBucketLimiter(1, 1, 10*time.Millisecond)

	base := time.Now()
	l.now = func() time.Time { return base }
	l.Allow("ip:old")
	if len(l.buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(l.buckets))
	}

	// Advance past the TTL; the next Allow sweeps the idle key.
	l.now = func() time.Time { return base.Add(time.Second) }
	l.Allow("ip:new")
	if _, ok := l.buckets["ip:old"]; ok {
		t.Fatal("idle key should have been evicted")
	}
}

func TestRateLimitByIPMiddleware(t *testing.T) {
	t.Parallel()
	l := NewTokenBucketLimiter(0.0001, 1, time.Minute)
	h := RateLimitByIP(l)(okHandler())

	newReq := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		return req
	}

	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, newReq())
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, newReq())
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request should be 429, got %d", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Error("429 response should set Retry-After")
	}
}

// resolvedIP runs req through RealIP middleware built from cidrs and returns the
// IP that ClientIP sees downstream.
func resolvedIP(t *testing.T, cidrs []string, req *http.Request) string {
	t.Helper()
	ri, err := NewRealIP(cidrs)
	if err != nil {
		t.Fatalf("NewRealIP: %v", err)
	}
	var got string
	ri.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = ClientIP(r)
	})).ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestClientIPFallsBackToRemoteAddr(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.4:9999"
	// No middleware installed: ClientIP must use the peer, never XFF.
	if got := ClientIP(req); got != "198.51.100.4" {
		t.Fatalf("ClientIP = %q, want 198.51.100.4", got)
	}
}

func TestClientIPIgnoresXFFFromUntrustedPeer(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.7:5555" // direct, untrusted peer
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	// Empty trusted set AND untrusted peer: the spoofed header must be ignored.
	if got := resolvedIP(t, nil, req); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want the peer 203.0.113.7 (XFF must be ignored)", got)
	}
	if got := resolvedIP(t, []string{"10.0.0.0/8"}, req); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want 203.0.113.7 (peer not in trusted set)", got)
	}
}

func TestClientIPHonorsXFFFromTrustedProxy(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:5555" // trusted proxy peer
	// Two hops: an external proxy (untrusted, the real client edge) then ours.
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.9")
	got := resolvedIP(t, []string{"10.0.0.0/8"}, req)
	if got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, want 203.0.113.7 (right-most untrusted hop)", got)
	}
}

func TestNewRealIPRejectsInvalidCIDR(t *testing.T) {
	t.Parallel()
	if _, err := NewRealIP([]string{"not-a-cidr"}); err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
	// Bare IPs are accepted and normalized to host routes.
	if _, err := NewRealIP([]string{"10.0.0.1", "::1"}); err != nil {
		t.Fatalf("bare IPs should be accepted: %v", err)
	}
}
