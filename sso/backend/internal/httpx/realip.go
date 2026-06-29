package httpx

import (
	"context"
	"net"
	"net/http"
	"strings"
)

// clientIPKey is the context key under which the resolved client IP is stored by
// RealIP middleware so handlers/limiters read a single trusted value.
type clientIPCtxKey struct{}

// RealIP resolves the true client IP, honoring the X-Forwarded-For header ONLY
// when the immediate peer (r.RemoteAddr) is a configured trusted proxy. With an
// empty trusted set (the safe default) XFF is ignored entirely and the direct
// peer address is used — so a deployment that forgets to configure its proxy
// fails safe rather than trusting an attacker-supplied header.
//
// This is the security-critical input to both per-IP rate limiting and the
// audit log: trusting a spoofable header would let an attacker rotate the
// limiter bucket on every request and frame arbitrary source IPs.
type RealIP struct {
	trusted []*net.IPNet
}

// NewRealIP parses the trusted-proxy CIDRs (e.g. "10.0.0.0/8", "172.16.0.0/12").
// Bare IPs are accepted and treated as /32 or /128. An invalid entry is an
// error so misconfiguration is caught at startup, not silently ignored.
func NewRealIP(cidrs []string) (*RealIP, error) {
	ri := &RealIP{}
	for _, raw := range cidrs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			if ip := net.ParseIP(s); ip != nil {
				if ip.To4() != nil {
					s += "/32"
				} else {
					s += "/128"
				}
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, err
		}
		ri.trusted = append(ri.trusted, n)
	}
	return ri, nil
}

// Middleware resolves the client IP once per request and stashes it on the
// context for ClientIP to read. It should be installed early in the stack.
func (ri *RealIP) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ri.resolve(r)
		ctx := context.WithValue(r.Context(), clientIPCtxKey{}, ip)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resolve computes the client IP for a request given the trusted-proxy set.
func (ri *RealIP) resolve(r *http.Request) string {
	peer := remoteHost(r.RemoteAddr)
	// Only consult XFF if the direct peer is itself a trusted proxy.
	if len(ri.trusted) == 0 || !ri.isTrusted(peer) {
		return peer
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return peer
	}
	// Walk right-to-left, skipping trusted hops; the first untrusted address is
	// the real client. The right-most entries are the ones our proxy appended and
	// can be relied on; the left-most are the most easily spoofed.
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(parts[i])
		if hop == "" {
			continue
		}
		if ri.isTrusted(hop) {
			continue
		}
		return hop
	}
	return peer
}

// isTrusted reports whether ipStr falls inside any trusted-proxy CIDR.
func (ri *RealIP) isTrusted(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range ri.trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// remoteHost extracts the host portion of a host:port RemoteAddr.
func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// ClientIP returns the client IP resolved by RealIP middleware. When the
// middleware is not installed (e.g. in a unit test), it falls back to the host
// portion of RemoteAddr and never trusts X-Forwarded-For.
func ClientIP(r *http.Request) string {
	if v, ok := r.Context().Value(clientIPCtxKey{}).(string); ok && v != "" {
		return v
	}
	return remoteHost(r.RemoteAddr)
}
