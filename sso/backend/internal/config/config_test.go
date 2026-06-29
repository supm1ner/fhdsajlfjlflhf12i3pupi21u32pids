package config

import (
	"strings"
	"testing"
	"time"
)

// baseValidConfig returns a config that passes Validate in development.
func baseValidConfig() *Config {
	return &Config{
		Env:                   EnvDevelopment,
		HTTPAddr:              ":8080",
		PublicBaseURL:         "http://localhost:8080",
		FrontendBaseURL:       "http://localhost:3000",
		DatabaseURL:           "postgres://u:p@localhost:5432/db?sslmode=disable",
		HydraAdminURL:         "http://localhost:4445",
		HydraPublicURL:        "http://localhost:4444",
		SessionCookieName:     "cid_session",
		CSRFCookieName:        "cid_csrf",
		AdminAPIKey:           "dev",
		SessionTTL:            24 * time.Hour,
		SessionRememberTTL:    30 * 24 * time.Hour,
		PasswordResetTTL:      30 * time.Minute,
		RateLimitRPS:          5,
		RateLimitBurst:        10,
		LogLevel:              "info",
		CookieSecure:          false,
		LoginLockoutThreshold: 5,
		WebAuthn: WebAuthnConfig{
			RPID:          "localhost",
			RPDisplayName: "cotton-id",
			RPOrigins:     []string{"http://localhost:3000"},
		},
	}
}

func TestValidateDevPasses(t *testing.T) {
	t.Parallel()
	if err := baseValidConfig().Validate(); err != nil {
		t.Fatalf("dev config should validate, got: %v", err)
	}
}

func TestValidateRequiresDatabaseURL(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.DatabaseURL = ""
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected DATABASE_URL error, got: %v", err)
	}
}

func TestValidateSMTPDisabledByDefault(t *testing.T) {
	t.Parallel()
	c := baseValidConfig() // SMTP zero-value (Host empty) → disabled
	if c.SMTP.Enabled() {
		t.Fatal("SMTP should be disabled when Host is empty")
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("config with SMTP disabled should validate, got: %v", err)
	}
}

func TestValidateSMTPRequiresFromWhenEnabled(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.SMTP = SMTPConfig{Host: "smtp.example.com", Port: 587, STARTTLS: true} // no From
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "SMTP_FROM") {
		t.Fatalf("expected SMTP_FROM error, got: %v", err)
	}
}

func TestValidateSMTPRejectsBadPort(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.SMTP = SMTPConfig{Host: "smtp.example.com", Port: 0, From: "no-reply@example.com"}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "SMTP_PORT") {
		t.Fatalf("expected SMTP_PORT range error, got: %v", err)
	}
}

func TestValidateSMTPEnabledValid(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.SMTP = SMTPConfig{Host: "smtp.example.com", Port: 587, From: "no-reply@example.com", STARTTLS: true}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid SMTP config should validate, got: %v", err)
	}
}

func TestValidateProductionRejectsWeakAdminKey(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.Env = EnvProduction
	c.CookieSecure = true
	c.PublicBaseURL = "https://id.example.com"
	c.AdminAPIKey = "changeme" // known-weak
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "ADMIN_API_KEY") {
		t.Fatalf("expected ADMIN_API_KEY weakness error, got: %v", err)
	}
}

func TestValidateProductionRequiresStrongAdminKeyLength(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.Env = EnvProduction
	c.CookieSecure = true
	c.PublicBaseURL = "https://id.example.com"
	c.AdminAPIKey = "short-but-not-denylisted" // < 32 chars
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "at least 32") {
		t.Fatalf("expected 32-char requirement error, got: %v", err)
	}
}

func TestValidateProductionRequiresSecureCookies(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.Env = EnvProduction
	c.PublicBaseURL = "https://id.example.com"
	c.AdminAPIKey = strings.Repeat("a", 40)
	c.CookieSecure = false
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "COOKIE_SECURE") {
		t.Fatalf("expected COOKIE_SECURE error, got: %v", err)
	}
}

func TestValidateProductionRejectsHTTPPublicURL(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.Env = EnvProduction
	c.CookieSecure = true
	c.AdminAPIKey = strings.Repeat("a", 40)
	c.PublicBaseURL = "http://id.example.com" // not https
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected https error, got: %v", err)
	}
}

func TestValidateProductionFullyValid(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.Env = EnvProduction
	c.CookieSecure = true
	c.PublicBaseURL = "https://id.example.com"
	c.AdminAPIKey = strings.Repeat("k", 40)
	if err := c.Validate(); err != nil {
		t.Fatalf("fully-valid production config should pass, got: %v", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	// Ensure a clean env for the vars we assert on.
	for _, k := range []string{
		"COTTON_ENV", "HTTP_ADDR", "SESSION_TTL_HOURS", "SESSION_REMEMBER_DAYS",
		"PASSWORD_RESET_TTL_MINUTES", "RATE_LIMIT_RPS", "RATE_LIMIT_BURST", "COOKIE_SECURE",
	} {
		t.Setenv(k, "")
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Env != EnvDevelopment {
		t.Errorf("default Env = %q, want development", c.Env)
	}
	if c.SessionTTL != 24*time.Hour {
		t.Errorf("default SessionTTL = %v, want 24h", c.SessionTTL)
	}
	if c.SessionRememberTTL != 30*24*time.Hour {
		t.Errorf("default SessionRememberTTL = %v, want 720h", c.SessionRememberTTL)
	}
	if c.RateLimitRPS != 5 || c.RateLimitBurst != 10 {
		t.Errorf("default rate limit = %v/%d, want 5/10", c.RateLimitRPS, c.RateLimitBurst)
	}
}

func TestLoadRejectsBadInt(t *testing.T) {
	t.Setenv("SESSION_TTL_HOURS", "not-a-number")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for bad SESSION_TTL_HOURS")
	}
}

func TestLoadSMTPDefaults(t *testing.T) {
	for _, k := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_STARTTLS", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM"} {
		t.Setenv(k, "")
	}
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SMTP.Enabled() {
		t.Error("SMTP should be disabled by default (no SMTP_HOST)")
	}
	if c.SMTP.Port != 587 {
		t.Errorf("default SMTP_PORT = %d, want 587", c.SMTP.Port)
	}
	if !c.SMTP.STARTTLS {
		t.Error("default SMTP_STARTTLS should be true")
	}
}

func TestLoadSMTPFromEnv(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_STARTTLS", "false")
	t.Setenv("SMTP_USERNAME", "user")
	t.Setenv("SMTP_PASSWORD", "pass")
	t.Setenv("SMTP_FROM", "no-reply@example.com")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.SMTP.Enabled() {
		t.Error("SMTP should be enabled when SMTP_HOST is set")
	}
	if c.SMTP.Port != 2525 || c.SMTP.STARTTLS || c.SMTP.Username != "user" || c.SMTP.From != "no-reply@example.com" {
		t.Errorf("unexpected SMTP config: %+v", c.SMTP)
	}
}

// --- WebAuthn relying-party config ---

func TestValidateWebAuthnRequiresRPID(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.WebAuthn.RPID = ""
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "WEBAUTHN_RP_ID is required") {
		t.Fatalf("expected RP id required error, got: %v", err)
	}
}

func TestValidateWebAuthnRequiresOrigins(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.WebAuthn.RPOrigins = nil
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "WEBAUTHN_RP_ORIGINS is required") {
		t.Fatalf("expected RP origins required error, got: %v", err)
	}
}

func TestValidateWebAuthnRPIDMustBeRegistrableSuffix(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.WebAuthn.RPID = "example.com"
	c.WebAuthn.RPOrigins = []string{"https://notexample.com"}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "registrable suffix") {
		t.Fatalf("expected registrable-suffix error, got: %v", err)
	}
}

func TestValidateWebAuthnRPIDSuffixAccepted(t *testing.T) {
	t.Parallel()
	c := baseValidConfig()
	c.WebAuthn.RPID = "example.com"
	c.WebAuthn.RPOrigins = []string{"https://example.com", "https://id.example.com:8443"}
	if err := c.Validate(); err != nil {
		t.Fatalf("RP id as parent domain of origins should pass, got: %v", err)
	}
}

func TestIsRegistrableSuffix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		rpID, host string
		want       bool
	}{
		{"localhost", "localhost", true},
		{"example.com", "example.com", true},
		{"example.com", "id.example.com", true},
		{"example.com", "a.b.example.com", true},
		{"example.com", "notexample.com", false},
		{"example.com", "example.com.evil.com", false},
		{"id.example.com", "example.com", false}, // child cannot be a suffix of parent
		{"127.0.0.1", "127.0.0.1", true},
		{"127.0.0.1", "10.0.0.1", false},
	}
	for _, c := range cases {
		if got := isRegistrableSuffix(c.rpID, c.host); got != c.want {
			t.Errorf("isRegistrableSuffix(%q, %q) = %v, want %v", c.rpID, c.host, got, c.want)
		}
	}
}

func TestHostWithoutPort(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"http://localhost:3000", "localhost"},
		{"https://id.example.com", "id.example.com"},
		{"https://id.example.com:8443", "id.example.com"},
		{"localhost:3000", "localhost"},
		{"localhost", "localhost"},
		{"", ""},
	}
	for _, c := range cases {
		if got := hostWithoutPort(c.in); got != c.want {
			t.Errorf("hostWithoutPort(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLoadDerivesWebAuthnDefaultsFromFrontend(t *testing.T) {
	for _, k := range []string{"WEBAUTHN_RP_ID", "WEBAUTHN_RP_DISPLAY_NAME", "WEBAUTHN_RP_ORIGINS"} {
		t.Setenv(k, "")
	}
	t.Setenv("FRONTEND_BASE_URL", "http://localhost:3000")
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.WebAuthn.RPID != "localhost" {
		t.Errorf("derived RP id = %q, want localhost", c.WebAuthn.RPID)
	}
	if c.WebAuthn.RPDisplayName != "cotton-id" {
		t.Errorf("default RP display name = %q, want cotton-id", c.WebAuthn.RPDisplayName)
	}
	if len(c.WebAuthn.RPOrigins) != 1 || c.WebAuthn.RPOrigins[0] != "http://localhost:3000" {
		t.Errorf("derived RP origins = %v, want [http://localhost:3000]", c.WebAuthn.RPOrigins)
	}
}
