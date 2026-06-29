package httpx

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

// CSRFHeaderName is the request header carrying the CSRF token, compared against
// the CSRF cookie (double-submit).
const CSRFHeaderName = "X-CSRF-Token"

// csrfTokenBytes is the entropy of a CSRF token before base64 encoding.
const csrfTokenBytes = 32

// CSRFConfig configures the double-submit middleware.
type CSRFConfig struct {
	// CookieName is the CSRF cookie name (cid_csrf).
	CookieName string
	// Secure marks the cookie Secure.
	Secure bool
}

// safeMethods are never CSRF-checked (they must be side-effect free per HTTP).
var safeMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
	http.MethodTrace:   true,
}

// GenerateCSRFToken returns a new random, URL-safe CSRF token.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SetCSRFCookie issues a fresh CSRF token and writes it to the cid_csrf cookie,
// returning the token so the handler can also send it in the response body. The
// cookie is intentionally NOT HttpOnly: the SPA must read it to echo it in the
// X-CSRF-Token header (the "double submit" pattern). SameSite=Lax + the header
// echo together defeat cross-site forgery.
func SetCSRFCookie(w http.ResponseWriter, cfg CSRFConfig) (string, error) {
	token, err := GenerateCSRFToken()
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
	return token, nil
}

// CSRF returns the double-submit CSRF middleware. For unsafe (state-changing)
// methods it requires that the X-CSRF-Token header equals the cid_csrf cookie,
// compared in constant time. Safe methods pass through.
//
// There is deliberately NO in-band exemption based on request headers: a bypass
// keyed on the mere presence of an attacker-controllable header (e.g.
// Authorization or X-Admin-Key) would be a latent CSRF hole. Genuinely
// machine-to-machine routes (the admin API) are mounted on a separate subtree
// that never includes this middleware and authenticate with a *validated* admin
// key instead — they need no exemption here.
func CSRF(cfg CSRFConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if safeMethods[r.Method] {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(cfg.CookieName)
			if err != nil || cookie.Value == "" {
				WriteProblem(w, r, http.StatusForbidden, "missing CSRF cookie")
				return
			}
			header := r.Header.Get(CSRFHeaderName)
			if header == "" {
				WriteProblem(w, r, http.StatusForbidden, "missing CSRF token header")
				return
			}
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
				WriteProblem(w, r, http.StatusForbidden, "CSRF token mismatch")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
