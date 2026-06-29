// Command cotton-id is the cotton-id identity provider backend. It is the single
// composition root: it loads config, builds the logger, connects to Postgres and
// runs migrations, sets up metrics, builds the router with the global middleware
// stack, mounts the auth/oidc/admin routes, exposes /metrics, /healthz, and
// /swagger, then serves HTTP with graceful shutdown.
//
// @title                      cotton-id API
// @version                    0.1.0
// @description                cotton-id is a single-tenant OpenID Connect identity provider. This is the change-1 walking skeleton: email/password auth, sessions, and the OIDC login/consent surface (OIDC and admin-client routes are stubs in this change).
// @BasePath                   /api/v1
// @schemes                    http https
// @securityDefinitions.apikey AdminKey
// @in                         header
// @name                       X-Admin-Key
// @securityDefinitions.apikey CSRF
// @in                         header
// @name                       X-CSRF-Token
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	"cotton-id/internal/account"
	"cotton-id/internal/admin"
	"cotton-id/internal/adminapi"
	"cotton-id/internal/audit"
	"cotton-id/internal/auth"
	"cotton-id/internal/config"
	"cotton-id/internal/database"
	"cotton-id/internal/httpx"
	"cotton-id/internal/mailer"
	"cotton-id/internal/notify"
	"cotton-id/internal/observability"
	"cotton-id/internal/oidc"
	"cotton-id/internal/passkey"
	"cotton-id/internal/social"
	"cotton-id/internal/statekey"
	"cotton-id/migrations"

	// docs is the swaggo-generated package; the blank import registers the spec.
	_ "cotton-id/docs"
)

func main() {
	if err := run(); err != nil {
		// run() logs details; this guarantees a non-zero exit on fatal error.
		os.Exit(1)
	}
}

func run() error {
	// --- config ---
	cfg, err := config.Load()
	if err != nil {
		// No logger yet; write to stderr via the default slog logger.
		observability.NewLogger("error").Error("config load failed", "error", err)
		return err
	}

	// --- logger ---
	log := observability.NewLogger(cfg.LogLevel)
	log.Info("starting cotton-id", "env", cfg.Env)

	if err := cfg.Validate(); err != nil {
		log.Error("config validation failed", "error", err)
		return err
	}

	// Root context cancelled on SIGINT/SIGTERM for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- database + migrations ---
	db, err := database.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Error("database connect failed", "error", err)
		return err
	}
	defer db.Close()

	if err := db.RunMigrations(ctx, migrations.FS, log); err != nil {
		log.Error("migrations failed", "error", err)
		return err
	}

	// --- metrics ---
	metrics := observability.NewMetrics()

	// --- audit log wiring ---
	// The Writer is appended to at every existing security-event point (login,
	// signup, reset, consent, client reg) and by the admin lifecycle actions; it
	// is synchronous and best-effort (a failed insert never blocks the user
	// action). The Reader backs the admin Journal. auditWriter is threaded into
	// the auth/oidc/adminapi handlers below; auditReader and sessionResolver are
	// the seams the admin console (next slice) consumes.
	auditWriter := audit.NewWriter(db.Pool, log)
	auditReader := audit.NewReader(db.Pool)

	// --- auth wiring ---
	userStore := auth.NewUserStore(db.Pool)
	sessionStore := auth.NewSessionStore(db.Pool)
	resetStore := auth.NewResetTokenStore(db.Pool)
	socialIdentityStore := auth.NewSocialIdentityStore(db.Pool)
	argonParams := auth.DefaultArgon2Params()
	authn := auth.NewPasswordAuthenticator(userStore, argonParams)

	// --- mailer selection ---
	// SMTPMailer is used when SMTP_HOST is configured; otherwise the dev LogMailer
	// (logs messages instead of delivering) is the safe default. All transactional
	// sends (reset, login-notification, admin message) go through this interface;
	// sends are best-effort and never block/fail the user action.
	var appMailer mailer.Mailer
	if cfg.SMTP.Enabled() {
		appMailer = mailer.NewSMTPMailer(mailer.SMTPConfig{
			Host:     cfg.SMTP.Host,
			Port:     cfg.SMTP.Port,
			Username: cfg.SMTP.Username,
			Password: cfg.SMTP.Password,
			From:     cfg.SMTP.From,
			STARTTLS: cfg.SMTP.STARTTLS,
		})
		log.Info("mailer: SMTP transport",
			"host", cfg.SMTP.Host, "port", cfg.SMTP.Port,
			"starttls", cfg.SMTP.STARTTLS, "auth", cfg.SMTP.Username != "")
	} else {
		appMailer = mailer.NewLogMailer(log)
		log.Info("mailer: development LogMailer (SMTP_HOST not set; emails are logged, not delivered)")
	}

	// Login-notification notifier over the selected mailer (best-effort, async).
	loginNotifier := notify.NewNotifier(appMailer, log, "cotton-id")

	authService := auth.NewService(auth.Config{
		SessionTTL:         cfg.SessionTTL,
		SessionRememberTTL: cfg.SessionRememberTTL,
		PasswordResetTTL:   cfg.PasswordResetTTL,
		FrontendBaseURL:    cfg.FrontendBaseURL,
		Argon2Params:       argonParams,
	}, userStore, sessionStore, resetStore, authn, appMailer)

	// Shared token-bucket limiter for auth routes (per-IP + per-account keys).
	limiter := httpx.NewTokenBucketLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst, 10*time.Minute)
	// Incremental-backoff lockout for repeated failed logins per account.
	lockout := auth.NewMemoryLockout(cfg.LoginLockoutThreshold)

	codeStore := auth.NewCodeStore(5)
	authHandlers := auth.NewHandlers(authService, log, metrics, limiter, lockout, auth.HandlersConfig{
		SessionCookieName: cfg.SessionCookieName,
		CSRFCookieName:    cfg.CSRFCookieName,
		CookieSecure:      cfg.CookieSecure,
	}).WithAudit(auditWriter).WithLoginNotifier(loginNotifier, sessionStore).WithVerifier(codeStore)

	// sessionResolver is the seam auth.RequireRole uses to gate the admin console
	// API on a minimum role; authService satisfies it via UserForSession. The
	// admin console mounts its routes behind
	// auth.RequireRole(auth.RoleAdmin, sessionResolver, cfg.SessionCookieName, log).
	var sessionResolver auth.SessionResolver = authService

	// --- admin console wiring ---
	// The human-operator console API (/api/v1/admin, role-gated) reuses the auth
	// user/session stores, the audit reader/writer, and a Hydra client (for the
	// registered-services count + best-effort revoke on delete). It is distinct
	// from the machine X-Admin-Key client-registration routes (adminapi).
	adminStore := admin.NewStore(db.Pool)
	adminHydra := oidc.NewHydraClient(cfg.HydraAdminURL)
	adminService := admin.BuildService(userStore, adminStore, sessionStore, authService, adminHydra)
	adminDeps := admin.Deps{
		Logger:   log,
		Metrics:  metrics,
		Store:    adminStore,
		Service:  adminService,
		Users:    userStore,
		Sessions: sessionStore,
		Audit:    auditWriter,
		Journal:  auditReader,
		Services: admin.HydraServicesCounter{Client: adminHydra},
		// Services tab: the same Hydra admin client backs the console client CRUD
		// + the best-effort per-client consent count/revoke; the admin Store
		// enumerates the IdP's subjects for that best-effort scan (design D3).
		Clients:  adminHydra,
		Subjects: adminStore,
		// Mailer backs the console "message user" action; the dev LogMailer (or the
		// configured SMTPMailer) is reused so messages go through the same transport.
		Mailer: appMailer,
	}

	// --- ceremony-state signing keys (social cid_oauth + passkey cid_wa) ---
	// When OAUTH_STATE_KEY is configured, BOTH cookie keys are derived from it via
	// HKDF with distinct labels so all replicas agree on the signing keys (a
	// ceremony begun on one replica can finish on another) while staying
	// cryptographically independent (domain separation). When it is unset, each
	// process falls back to a per-process random key — correct single-instance, but
	// multi-replica deployments need the shared key (warned below).
	socialStateKey, passkeyStateKey, err := deriveStateKeys(cfg.OAuthStateKey, log)
	if err != nil {
		log.Error("state signing key setup failed", "error", err)
		return err
	}

	// --- social-login wiring ---
	socialHandlers := social.NewHandlers(social.Deps{
		Logger:     log,
		Metrics:    metrics,
		Users:      userStore,
		Identities: socialIdentityStore,
		Sessions:   authService,
		Hydra:      oidc.NewHydraClient(cfg.HydraAdminURL),
		Providers: social.ProvidersConfig{
			Google: socialCreds(cfg.Social.Google),
			GitHub: socialCreds(cfg.Social.GitHub),
			// VK ID is a PKCE public client — enabled on client id alone (no secret).
			VK: social.ProviderCredentials{
				ClientID:     cfg.Social.VK.ClientID,
				ClientSecret: cfg.Social.VK.ClientSecret,
				Enabled:      cfg.Social.VK.EnabledPublic(),
			},
			Yandex: socialCreds(cfg.Social.Yandex),
		},
		PublicBaseURL:     cfg.PublicBaseURL,
		FrontendBaseURL:   cfg.FrontendBaseURL,
		SessionCookieName: cfg.SessionCookieName,
		CookieSecure:      cfg.CookieSecure,
		StateKey:          socialStateKey,
		Notifier:          loginNotifier,
		SessionLister:     sessionStore,
	})

	// --- passkey (WebAuthn) wiring ---
	// The cid_wa ceremony-state cookie key (passkeyStateKey) is set up above
	// alongside the social key: derived from OAUTH_STATE_KEY via HKDF when shared,
	// else a per-process random key.
	credentialStore := passkey.NewCredentialStore(db.Pool)
	passkeyHandlers, err := passkey.NewHandlers(passkey.Deps{
		Logger:            log,
		Metrics:           metrics,
		Users:             userStore,
		Credentials:       credentialStore,
		Auth:              authService,
		Hydra:             oidc.NewHydraClient(cfg.HydraAdminURL),
		RPID:              cfg.WebAuthn.RPID,
		RPDisplayName:     cfg.WebAuthn.RPDisplayName,
		RPOrigins:         cfg.WebAuthn.RPOrigins,
		SessionCookieName: cfg.SessionCookieName,
		CookieSecure:      cfg.CookieSecure,
		StateKey:          passkeyStateKey,
		Notifier:          loginNotifier,
		SessionLister:     sessionStore,
	})
	if err != nil {
		log.Error("passkey handlers init failed", "error", err)
		return err
	}

	// --- account self-service wiring ---
	accountHandlers := account.NewHandlers(account.Deps{
		Logger:            log,
		Metrics:           metrics,
		Users:             userStore,
		Sessions:          sessionStore,
		Credentials:       credentialStore,
		Hydra:             oidc.NewHydraClient(cfg.HydraAdminURL),
		Images:            account.NewImageStore(db.Pool),
		Auth:              authService,
		Authn:             authn,
		Lockout:           lockout,
		Params:            argonParams,
		PublicBaseURL:     cfg.PublicBaseURL,
		SessionCookieName: cfg.SessionCookieName,
		CookieSecure:      cfg.CookieSecure,
	})

	// Trusted-proxy-aware client-IP resolver (drives rate limiting + audit IPs).
	realIP, err := httpx.NewRealIP(cfg.TrustedProxies)
	if err != nil {
		log.Error("invalid TRUSTED_PROXIES", "error", err)
		return err
	}

	// --- router + global middleware ---
	r := httpx.NewRouter(httpx.RouterDeps{
		Logger:          log,
		Metrics:         metrics,
		FrontendBaseURL: cfg.FrontendBaseURL,
		RealIP:          realIP,
		Secure:          cfg.CookieSecure,
	})

	csrf := httpx.CSRF(httpx.CSRFConfig{CookieName: cfg.CSRFCookieName, Secure: cfg.CookieSecure})
	rateLimit := httpx.RateLimitByIP(limiter)

	// /api/v1 subtree. Per-IP rate limiting applies to the whole subtree. The
	// browser-facing routes (auth + OIDC JSON) additionally require CSRF; the
	// admin routes are machine-to-machine (admin-key) and are mounted OUTSIDE the
	// CSRF group so they never depend on a CSRF cookie (contract §3, design D5).
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(rateLimit)

		// Browser-facing, CSRF-protected group.
		api.Group(func(browser chi.Router) {
			browser.Use(csrf)
			authHandlers.Mount(browser)
			passkeyHandlers.Mount(browser)
			accountHandlers.Mount(browser)
			oidc.Mount(browser, oidc.Deps{
				Logger:            log,
				Metrics:           metrics,
				Sessions:          authService,
				HydraAdminURL:     cfg.HydraAdminURL,
				HydraPublicURL:    cfg.HydraPublicURL,
				FrontendBaseURL:   cfg.FrontendBaseURL,
				PublicBaseURL:     cfg.PublicBaseURL,
				SessionCookieName: cfg.SessionCookieName,
				CookieSecure:      cfg.CookieSecure,
				Audit:             auditWriter,
			})
		})

		// Social login (GET browser routes: providers list, start, callback). They
		// are CSRF-exempt by design — the OAuth round-trip's anti-CSRF control is
		// the signed cid_oauth state cookie validated on callback — so they are
		// mounted OUTSIDE the CSRF group.
		socialHandlers.Mount(api)

		// /api/v1/admin is a SINGLE subtree (chi forbids two subrouters at one
		// path) shared by two distinct auth models:
		//   - the machine client-registration API (X-Admin-Key, CSRF-exempt), and
		//   - the human operator console (session + CSRF + RequireRole(admin)).
		// Their paths are disjoint (/admin/clients vs /admin/{overview,users,audit}).
		api.Route("/admin", func(adm chi.Router) {
			// Machine-to-machine client routes — admin-key authorized, CSRF-exempt.
			adminapi.MountClients(adm, adminapi.Deps{
				Logger:        log,
				Metrics:       metrics,
				HydraAdminURL: cfg.HydraAdminURL,
				AdminAPIKey:   cfg.AdminAPIKey,
				Audit:         auditWriter,
			})
			// Human console — CSRF-protected and role-gated. RequireRole(admin)
			// resolves the session user (401 anonymous, 403 non-admin) and stashes
			// the acting user for the handlers' audit + escalation guards; owner-only
			// actions (role change, delete) re-check owner in-handler.
			adm.Group(func(c chi.Router) {
				c.Use(csrf)
				c.Use(auth.RequireRole(auth.RoleAdmin, sessionResolver, cfg.SessionCookieName, log))
				admin.MountConsole(c, adminDeps)
			})
		})
	})

	// Browser-redirect OIDC routes live at the root (not under /api).
	oidc.MountBrowser(r, oidc.Deps{
		Logger:            log,
		Metrics:           metrics,
		Sessions:          authService,
		HydraAdminURL:     cfg.HydraAdminURL,
		HydraPublicURL:    cfg.HydraPublicURL,
		FrontendBaseURL:   cfg.FrontendBaseURL,
		PublicBaseURL:     cfg.PublicBaseURL,
		SessionCookieName: cfg.SessionCookieName,
		CookieSecure:      cfg.CookieSecure,
	})

	// --- ops endpoints ---
	// Hydra admin client used solely for the readiness probe in /healthz.
	hydraHealth := oidc.NewHydraClient(cfg.HydraAdminURL)
	r.Handle("/metrics", metrics.Handler())
	r.Get("/healthz", httpx.HealthzHandler(
		httpx.NewHealthChecker("database", db.Health),
		httpx.NewHealthChecker("hydra", hydraHealth.Health),
	))
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL(cfg.PublicBaseURL+"/swagger/doc.json"),
	))

	// --- HTTP server with graceful shutdown ---
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		log.Error("http server error", "error", err)
		return err
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		return err
	}
	log.Info("server stopped cleanly")
	return nil
}

// deriveStateKeys returns the HMAC signing keys for the social (cid_oauth) and
// passkey (cid_wa) ceremony-state cookies. When sharedKey is configured it derives
// BOTH from it via HKDF with distinct labels (so all replicas agree on the keys
// and the two keys stay independent). When sharedKey is empty it falls back to a
// fresh per-process random key for each and logs a warning that multi-replica
// deployments require OAUTH_STATE_KEY (a ceremony begun on one replica would
// otherwise fail to finish on another).
func deriveStateKeys(sharedKey string, log *slog.Logger) (socialKey, passkeyKey []byte, err error) {
	if sharedKey != "" {
		socialKey, err = statekey.Derive([]byte(sharedKey), statekey.LabelOAuth)
		if err != nil {
			return nil, nil, err
		}
		passkeyKey, err = statekey.Derive([]byte(sharedKey), statekey.LabelPasskey)
		if err != nil {
			return nil, nil, err
		}
		log.Info("ceremony-state signing keys derived from OAUTH_STATE_KEY (shared across replicas via HKDF)")
		return socialKey, passkeyKey, nil
	}

	socialKey, err = social.NewSigningKey()
	if err != nil {
		return nil, nil, err
	}
	passkeyKey, err = passkey.NewSigningKey()
	if err != nil {
		return nil, nil, err
	}
	log.Warn("OAUTH_STATE_KEY is not set: using per-process random ceremony-state signing keys " +
		"(single-instance only). Multi-replica deployments MUST set OAUTH_STATE_KEY so the " +
		"social cid_oauth and passkey cid_wa cookies validate across replicas.")
	return socialKey, passkeyKey, nil
}

// socialCreds maps a config.SocialProviderConfig to the social package's
// credentials view, computing the enabled flag (both id and secret present).
func socialCreds(p config.SocialProviderConfig) social.ProviderCredentials {
	return social.ProviderCredentials{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		Enabled:      p.Enabled(),
	}
}
