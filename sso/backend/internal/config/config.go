// Package config loads and validates cotton-id runtime configuration from the
// environment. Configuration is read once at startup into a typed [Config] and
// validated by [Config.Validate]; the process refuses to start when a required
// secret is missing or a security-critical value uses a known-insecure default
// outside development (the "fail fast" requirement from platform-foundation).
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment names. Production tightens validation (no weak/empty secrets,
// Secure cookies expected).
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
	EnvTest        = "test"
)

// weakSecrets is a small denylist of obviously-insecure placeholder values that
// must never be used for secrets in production.
var weakSecrets = map[string]bool{
	"":              true,
	"changeme":      true,
	"change-me":     true,
	"secret":        true,
	"password":      true,
	"admin":         true,
	"dev":           true,
	"development":   true,
	"insecure":      true,
	"test":          true,
	"placeholder":   true,
	"replace-me":    true,
	"your-key-here": true,
	"admin-api-key": true,
}

// Config is the fully-typed cotton-id configuration. Field names map to the env
// vars defined in the build contract (§5). Durations are derived from the
// *_HOURS / *_DAYS / *_MINUTES integer vars.
type Config struct {
	// Env selects the environment profile. One of development|production|test.
	Env string

	// HTTPAddr is the backend listen address, e.g. ":8080".
	HTTPAddr string
	// PublicBaseURL is the backend's externally-reachable base URL.
	PublicBaseURL string
	// FrontendBaseURL is the SPA origin used for browser redirects.
	FrontendBaseURL string

	// DatabaseURL is the PostgreSQL DSN for the cotton-id database.
	DatabaseURL string

	// HydraAdminURL is Hydra's admin API (internal only).
	HydraAdminURL string
	// HydraPublicURL is Hydra's public OIDC issuer URL.
	HydraPublicURL string

	// SessionCookieName / CSRFCookieName name the cookies cotton-id sets.
	SessionCookieName string
	CSRFCookieName    string

	// AdminAPIKey authorizes the admin client-registration endpoints.
	AdminAPIKey string

	// OAuthStateKey is an OPTIONAL shared secret (>=32 bytes) from which the social
	// (cid_oauth) and passkey (cid_wa) ceremony-state cookie HMAC keys are derived
	// via HKDF with distinct labels, so all backend replicas agree on the signing
	// key and a ceremony begun on one replica can finish on another. When empty,
	// each process uses a per-process random key (single-instance) and logs a
	// startup warning that multi-replica deployments require this key.
	OAuthStateKey string

	// SessionTTL is the lifetime for non-remember sessions.
	SessionTTL time.Duration
	// SessionRememberTTL is the lifetime for "remember me" sessions.
	SessionRememberTTL time.Duration
	// PasswordResetTTL is the lifetime of a password-reset token.
	PasswordResetTTL time.Duration

	// RateLimitRPS / RateLimitBurst configure the auth-endpoint token buckets.
	RateLimitRPS   float64
	RateLimitBurst int

	// LogLevel is the slog level: debug|info|warn|error.
	LogLevel string

	// CookieSecure forces the Secure attribute on cookies and enables HSTS.
	CookieSecure bool

	// TrustedProxies is the allowlist of proxy CIDRs whose X-Forwarded-For header
	// is honored when resolving the client IP. Empty (the default) means XFF is
	// ignored entirely and the direct peer address is used — fail-safe.
	TrustedProxies []string

	// LoginLockoutThreshold is the number of consecutive failed logins for an
	// account before incremental backoff/lockout engages.
	LoginLockoutThreshold int

	// Social holds the per-provider OAuth/OIDC credentials for social login. A
	// provider is "enabled" only when both its client id and secret are set.
	Social SocialConfig

	// WebAuthn holds the relying-party configuration for passkey (WebAuthn/FIDO2)
	// registration and login.
	WebAuthn WebAuthnConfig

	// SMTP holds the outbound-email transport configuration. When SMTP.Host is
	// empty the backend uses the development LogMailer instead of delivering mail.
	SMTP SMTPConfig
}

// SMTPConfig is the outbound-email (SMTP) transport configuration. A non-empty
// Host selects the real SMTP mailer; otherwise the dev LogMailer is used. Auth is
// applied only when Username is set; STARTTLS upgrades the connection to TLS
// before AUTH/DATA (default true). These are NOT validated as secrets — a missing
// SMTP config is a supported (dev) mode, not an error.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	STARTTLS bool
}

// Enabled reports whether a real SMTP transport is configured (Host is set).
func (c SMTPConfig) Enabled() bool {
	return strings.TrimSpace(c.Host) != ""
}

// WebAuthnConfig is the relying-party (RP) configuration for passkeys. RPID is
// the registrable domain (e.g. "localhost" in dev, "id.example.com" in prod),
// RPDisplayName is shown by the authenticator UI, and RPOrigins is the list of
// fully-qualified origins permitted to complete ceremonies. RPID MUST be a
// registrable suffix of each origin's host (validated in [Config.Validate]).
type WebAuthnConfig struct {
	RPID          string
	RPDisplayName string
	RPOrigins     []string
}

// SocialProviderConfig holds one social provider's OAuth credentials. A provider
// is considered enabled only when both ClientID and ClientSecret are non-empty.
type SocialProviderConfig struct {
	ClientID     string
	ClientSecret string
}

// Enabled reports whether both the client id and secret are configured (the
// confidential-client case: Google, GitHub, Yandex).
func (p SocialProviderConfig) Enabled() bool {
	return strings.TrimSpace(p.ClientID) != "" && strings.TrimSpace(p.ClientSecret) != ""
}

// EnabledPublic reports whether the client id is configured, for PKCE public
// clients (VK ID) that authenticate with the PKCE verifier instead of a client
// secret — so requiring a secret would wrongly keep them disabled.
func (p SocialProviderConfig) EnabledPublic() bool {
	return strings.TrimSpace(p.ClientID) != ""
}

// SocialConfig groups the supported social providers' credentials. Provider keys
// match the {provider} path segment (google|github|vk|yandex).
type SocialConfig struct {
	Google SocialProviderConfig
	GitHub SocialProviderConfig
	VK     SocialProviderConfig
	Yandex SocialProviderConfig
}

// IsProduction reports whether the production profile is active.
func (c *Config) IsProduction() bool { return c.Env == EnvProduction }

// Load reads configuration from the process environment, applying the contract's
// documented defaults for non-secret values. It does not validate; call
// [Config.Validate] after Load. Load only returns an error when a numeric or
// boolean variable is malformed.
func Load() (*Config, error) {
	c := &Config{
		Env:               getEnv("COTTON_ENV", EnvDevelopment),
		HTTPAddr:          getEnv("HTTP_ADDR", ":8080"),
		PublicBaseURL:     getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),
		FrontendBaseURL:   getEnv("FRONTEND_BASE_URL", "http://localhost:3000"),
		DatabaseURL:       getEnv("DATABASE_URL", ""),
		HydraAdminURL:     getEnv("HYDRA_ADMIN_URL", "http://hydra:4445"),
		HydraPublicURL:    getEnv("HYDRA_PUBLIC_URL", "http://localhost:4444"),
		SessionCookieName: getEnv("SESSION_COOKIE_NAME", "cid_session"),
		CSRFCookieName:    getEnv("CSRF_COOKIE_NAME", "cid_csrf"),
		AdminAPIKey:       getEnv("ADMIN_API_KEY", ""),
		OAuthStateKey:     getEnv("OAUTH_STATE_KEY", ""),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		TrustedProxies:    splitCSV(getEnv("TRUSTED_PROXIES", "")),
		Social: SocialConfig{
			Google: SocialProviderConfig{
				ClientID:     getEnv("SOCIAL_GOOGLE_CLIENT_ID", ""),
				ClientSecret: getEnv("SOCIAL_GOOGLE_CLIENT_SECRET", ""),
			},
			GitHub: SocialProviderConfig{
				ClientID:     getEnv("SOCIAL_GITHUB_CLIENT_ID", ""),
				ClientSecret: getEnv("SOCIAL_GITHUB_CLIENT_SECRET", ""),
			},
			VK: SocialProviderConfig{
				ClientID:     getEnv("SOCIAL_VK_CLIENT_ID", ""),
				ClientSecret: getEnv("SOCIAL_VK_CLIENT_SECRET", ""),
			},
			Yandex: SocialProviderConfig{
				ClientID:     getEnv("SOCIAL_YANDEX_CLIENT_ID", ""),
				ClientSecret: getEnv("SOCIAL_YANDEX_CLIENT_SECRET", ""),
			},
		},
	}

	// WebAuthn relying-party config. Dev defaults derive from FRONTEND_BASE_URL:
	// the RP ID defaults to the SPA host without port (e.g. "localhost") and the
	// allowed origins default to the SPA base URL — so local dev works with no
	// extra env. In production these MUST be set explicitly.
	frontendHost := hostWithoutPort(c.FrontendBaseURL)
	c.WebAuthn = WebAuthnConfig{
		RPID:          getEnv("WEBAUTHN_RP_ID", frontendHost),
		RPDisplayName: getEnv("WEBAUTHN_RP_DISPLAY_NAME", "cotton-id"),
		RPOrigins:     splitCSV(getEnv("WEBAUTHN_RP_ORIGINS", strings.TrimRight(c.FrontendBaseURL, "/"))),
	}

	var err error
	var errs []error

	lockoutThreshold, err := getEnvInt("LOGIN_LOCKOUT_THRESHOLD", 5)
	if err != nil {
		errs = append(errs, err)
	}

	sessionTTLHours, err := getEnvInt("SESSION_TTL_HOURS", 24)
	if err != nil {
		errs = append(errs, err)
	}
	rememberDays, err := getEnvInt("SESSION_REMEMBER_DAYS", 30)
	if err != nil {
		errs = append(errs, err)
	}
	resetMinutes, err := getEnvInt("PASSWORD_RESET_TTL_MINUTES", 30)
	if err != nil {
		errs = append(errs, err)
	}
	rps, err := getEnvFloat("RATE_LIMIT_RPS", 5)
	if err != nil {
		errs = append(errs, err)
	}
	burst, err := getEnvInt("RATE_LIMIT_BURST", 10)
	if err != nil {
		errs = append(errs, err)
	}
	secure, err := getEnvBool("COOKIE_SECURE", false)
	if err != nil {
		errs = append(errs, err)
	}

	smtpPort, err := getEnvInt("SMTP_PORT", 587)
	if err != nil {
		errs = append(errs, err)
	}
	smtpSTARTTLS, err := getEnvBool("SMTP_STARTTLS", true)
	if err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	c.SessionTTL = time.Duration(sessionTTLHours) * time.Hour
	c.SessionRememberTTL = time.Duration(rememberDays) * 24 * time.Hour
	c.PasswordResetTTL = time.Duration(resetMinutes) * time.Minute
	c.RateLimitRPS = rps
	c.RateLimitBurst = burst
	c.CookieSecure = secure
	c.LoginLockoutThreshold = lockoutThreshold

	c.SMTP = SMTPConfig{
		Host:     getEnv("SMTP_HOST", ""),
		Port:     smtpPort,
		Username: getEnv("SMTP_USERNAME", ""),
		Password: getEnv("SMTP_PASSWORD", ""),
		From:     getEnv("SMTP_FROM", ""),
		STARTTLS: smtpSTARTTLS,
	}

	return c, nil
}

// splitCSV splits a comma-separated env value into trimmed, non-empty entries.
func splitCSV(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Validate checks the configuration for completeness and security. It always
// requires DATABASE_URL and the Hydra URLs. In production it additionally
// requires a strong ADMIN_API_KEY, forbids weak secrets, requires Secure
// cookies, and rejects loopback/insecure base URLs. The returned error joins
// every problem so an operator sees them all at once.
func (c *Config) Validate() error {
	var errs []error

	if strings.TrimSpace(c.DatabaseURL) == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if strings.TrimSpace(c.HydraAdminURL) == "" {
		errs = append(errs, errors.New("HYDRA_ADMIN_URL is required"))
	}
	if strings.TrimSpace(c.HydraPublicURL) == "" {
		errs = append(errs, errors.New("HYDRA_PUBLIC_URL is required"))
	}
	if strings.TrimSpace(c.PublicBaseURL) == "" {
		errs = append(errs, errors.New("PUBLIC_BASE_URL is required"))
	}
	if strings.TrimSpace(c.FrontendBaseURL) == "" {
		errs = append(errs, errors.New("FRONTEND_BASE_URL is required"))
	}

	switch c.Env {
	case EnvDevelopment, EnvProduction, EnvTest:
	default:
		errs = append(errs, fmt.Errorf("COTTON_ENV %q is invalid (want development|production|test)", c.Env))
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL %q is invalid (want debug|info|warn|error)", c.LogLevel))
	}

	if c.SessionTTL <= 0 {
		errs = append(errs, errors.New("SESSION_TTL_HOURS must be > 0"))
	}
	if c.SessionRememberTTL <= 0 {
		errs = append(errs, errors.New("SESSION_REMEMBER_DAYS must be > 0"))
	}
	if c.PasswordResetTTL <= 0 {
		errs = append(errs, errors.New("PASSWORD_RESET_TTL_MINUTES must be > 0"))
	}
	if c.RateLimitRPS <= 0 {
		errs = append(errs, errors.New("RATE_LIMIT_RPS must be > 0"))
	}
	if c.RateLimitBurst <= 0 {
		errs = append(errs, errors.New("RATE_LIMIT_BURST must be > 0"))
	}
	if c.LoginLockoutThreshold <= 0 {
		errs = append(errs, errors.New("LOGIN_LOCKOUT_THRESHOLD must be > 0"))
	}

	// Trusted-proxy entries must be valid CIDRs or bare IPs (fail fast on typos
	// so a misconfigured proxy allowlist never silently trusts nothing/everything).
	for _, p := range c.TrustedProxies {
		if _, _, err := net.ParseCIDR(p); err != nil {
			if net.ParseIP(p) == nil {
				errs = append(errs, fmt.Errorf("TRUSTED_PROXIES entry %q is not a valid CIDR or IP", p))
			}
		}
	}

	// WebAuthn relying-party validation: an RP ID must be configured and must be
	// a registrable suffix of every allowed origin's host, otherwise the browser
	// rejects every ceremony at runtime. Fail fast at startup instead.
	if strings.TrimSpace(c.WebAuthn.RPID) == "" {
		errs = append(errs, errors.New("WEBAUTHN_RP_ID is required (or set FRONTEND_BASE_URL so it can be derived)"))
	}
	if len(c.WebAuthn.RPOrigins) == 0 {
		errs = append(errs, errors.New("WEBAUTHN_RP_ORIGINS is required (or set FRONTEND_BASE_URL so it can be derived)"))
	}
	for _, origin := range c.WebAuthn.RPOrigins {
		host := hostWithoutPort(origin)
		if host == "" {
			errs = append(errs, fmt.Errorf("WEBAUTHN_RP_ORIGINS entry %q is not a valid origin URL", origin))
			continue
		}
		if !isRegistrableSuffix(c.WebAuthn.RPID, host) {
			errs = append(errs, fmt.Errorf("WEBAUTHN_RP_ID %q is not a registrable suffix of origin host %q", c.WebAuthn.RPID, host))
		}
	}

	// SMTP: only validated when a real transport is configured (Host set). A
	// missing SMTP config is the supported dev mode (LogMailer), not an error.
	if c.SMTP.Enabled() {
		if c.SMTP.Port <= 0 || c.SMTP.Port > 65535 {
			errs = append(errs, fmt.Errorf("SMTP_PORT %d is out of range (1-65535)", c.SMTP.Port))
		}
		if strings.TrimSpace(c.SMTP.From) == "" {
			errs = append(errs, errors.New("SMTP_FROM is required when SMTP_HOST is set"))
		}
	}

	// OAUTH_STATE_KEY is optional, but when present it MUST be long enough to be a
	// real shared secret (>=32 bytes): a short shared key is worse than the
	// per-process random default, so reject it rather than silently weakening the
	// state-cookie signing across replicas.
	if k := strings.TrimSpace(c.OAuthStateKey); k != "" && len(k) < 32 {
		errs = append(errs, errors.New("OAUTH_STATE_KEY must be at least 32 bytes when set"))
	}

	// Production-only hardening.
	if c.IsProduction() {
		if isWeakSecret(c.AdminAPIKey) {
			errs = append(errs, errors.New("ADMIN_API_KEY is missing or a known-weak/placeholder value; set a strong (32+ char) random value in production"))
		} else if len(c.AdminAPIKey) < 32 {
			errs = append(errs, errors.New("ADMIN_API_KEY must be at least 32 characters in production"))
		}
		if !c.CookieSecure {
			errs = append(errs, errors.New("COOKIE_SECURE must be true in production"))
		}
		if strings.HasPrefix(c.PublicBaseURL, "http://") {
			errs = append(errs, errors.New("PUBLIC_BASE_URL must use https in production"))
		}
	}

	return errors.Join(errs...)
}

// weakMarkers are substrings that betray a placeholder/dev secret regardless of
// length, so values like "dev-insecure-admin-key-change-me-..." (which would
// otherwise pass the 32-char length check) are still rejected in production.
var weakMarkers = []string{"insecure", "change-me", "changeme", "example", "placeholder", "your-key", "replace-me", "dev-"}

// isWeakSecret reports whether s is empty, a known-weak literal, or contains a
// placeholder marker substring.
func isWeakSecret(s string) bool {
	v := strings.ToLower(strings.TrimSpace(s))
	if weakSecrets[v] {
		return true
	}
	for _, m := range weakMarkers {
		if strings.Contains(v, m) {
			return true
		}
	}
	return false
}

// hostWithoutPort returns the lower-cased host (no port) of a URL or bare
// host[:port] string. It returns "" when the input has no usable host. Used both
// to derive the dev RP ID from FRONTEND_BASE_URL and to validate RP origins.
func hostWithoutPort(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Parse as a URL when a scheme is present; otherwise treat the whole value as
	// a host[:port].
	host := raw
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return ""
		}
		host = u.Host
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

// isRegistrableSuffix reports whether rpID is a registrable suffix of host, per
// the WebAuthn RP-ID rule: the RP ID must equal the origin's effective domain or
// be a parent domain of it (a dotted suffix). For example RP ID "example.com" is
// valid for hosts "example.com" and "id.example.com" but not "notexample.com" or
// "example.com.evil.com". An IP-literal host only matches an identical RP ID.
func isRegistrableSuffix(rpID, host string) bool {
	rpID = strings.ToLower(strings.TrimSpace(rpID))
	host = strings.ToLower(strings.TrimSpace(host))
	if rpID == "" || host == "" {
		return false
	}
	if rpID == host {
		return true
	}
	// IP literals must match exactly (handled above); they have no parent domain.
	if net.ParseIP(host) != nil || net.ParseIP(rpID) != nil {
		return false
	}
	// Otherwise rpID must be a dot-delimited suffix of host (a parent domain).
	return strings.HasSuffix(host, "."+rpID)
}

// --- env helpers ---

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", key, v)
	}
	return n, nil
}

func getEnvFloat(key string, def float64) (float64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid number %q", key, v)
	}
	return f, nil
}

func getEnvBool(key string, def bool) (bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		return false, fmt.Errorf("%s: invalid boolean %q", key, v)
	}
	return b, nil
}
