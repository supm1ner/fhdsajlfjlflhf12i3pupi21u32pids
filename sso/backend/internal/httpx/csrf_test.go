package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCSRFAllowsSafeMethods(t *testing.T) {
	t.Parallel()
	mw := CSRF(CSRFConfig{CookieName: "cid_csrf"})
	h := mw(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET should pass CSRF, got %d", rec.Code)
	}
}

func TestCSRFBlocksMissingToken(t *testing.T) {
	t.Parallel()
	mw := CSRF(CSRFConfig{CookieName: "cid_csrf"})
	h := mw(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF cookie should be 403, got %d", rec.Code)
	}
}

func TestCSRFBlocksMismatch(t *testing.T) {
	t.Parallel()
	mw := CSRF(CSRFConfig{CookieName: "cid_csrf"})
	h := mw(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: "cid_csrf", Value: "cookie-token"})
	req.Header.Set(CSRFHeaderName, "different-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("CSRF mismatch should be 403, got %d", rec.Code)
	}
}

func TestCSRFAllowsMatchingDoubleSubmit(t *testing.T) {
	t.Parallel()
	mw := CSRF(CSRFConfig{CookieName: "cid_csrf"})
	h := mw(okHandler())

	const tok = "matching-token-value"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.AddCookie(&http.Cookie{Name: "cid_csrf", Value: tok})
	req.Header.Set(CSRFHeaderName, tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("matching double-submit should pass, got %d", rec.Code)
	}
}

// TestCSRFDoesNotExemptOnHeaders locks in the hardening from the security review:
// the CSRF middleware must NOT grant an exemption based on the mere presence of
// an X-Admin-Key or Authorization header (both attacker-controllable). Genuine
// machine-to-machine routes are protected by being mounted outside this
// middleware, not by an in-band header check.
func TestCSRFDoesNotExemptOnHeaders(t *testing.T) {
	t.Parallel()
	mw := CSRF(CSRFConfig{CookieName: "cid_csrf"})
	h := mw(okHandler())

	t.Run("admin key header does not bypass", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.Header.Set("X-Admin-Key", "some-key")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("X-Admin-Key must NOT exempt a browser route from CSRF, got %d", rec.Code)
		}
	})

	t.Run("authorization header does not bypass", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.Header.Set("Authorization", "Bearer abc")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("Authorization header must NOT exempt from CSRF, got %d", rec.Code)
		}
	})
}

func TestSetCSRFCookie(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	tok, err := SetCSRFCookie(rec, CSRFConfig{CookieName: "cid_csrf", Secure: true})
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" {
		t.Fatal("expected a token")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != "cid_csrf" || c.Value != tok {
		t.Fatalf("cookie mismatch: %+v vs token %q", c, tok)
	}
	if c.HttpOnly {
		t.Fatal("CSRF cookie must NOT be HttpOnly (SPA must read it)")
	}
	if !c.Secure {
		t.Fatal("CSRF cookie should be Secure when configured")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Fatal("CSRF cookie should be SameSite=Lax")
	}
}
