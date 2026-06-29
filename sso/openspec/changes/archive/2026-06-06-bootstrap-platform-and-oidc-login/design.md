## Context

cotton-id is a greenfield, single-tenant OpenID Connect identity provider. The design prototype (a static React mock under `_design_ref/`) defines the visual language — glass-morphism, an `--h` accent-hue token system, Instrument Serif + Hanken Grotesk type, dark/light themes, RU/EN — and the full feature surface (auth, account self-service, admin console). This change implements only the **walking skeleton**: the runtime substrate plus the email/password → consent → token path, end to end, in Docker.

The defining architectural constraint, settled during product brainstorming: cotton-id is a **production** IdP, so it must not hand-roll OIDC protocol crypto. We therefore split the system at the protocol boundary and delegate token issuance to **Ory Hydra**.

Constraints:
- Backend Go 1.25; frontend React + TypeScript; store PostgreSQL; all endpoints in Swagger; Prometheus metrics; runs in Docker; tested; documented; built spec-first via OpenSpec.
- Local Go toolchain is 1.23, so builds rely on `GOTOOLCHAIN=auto` (and the `golang:1.25` Docker image) to obtain 1.25.
- Single tenant: one identity domain, no `tenant_id` partitioning.

## Goals / Non-Goals

**Goals:**
- A `docker compose up` that brings up PostgreSQL + Hydra + backend + frontend with health checks.
- A relying party can complete `authorization_code` + PKCE: redirect → cotton-id login → consent → ID/access tokens from Hydra.
- Email/password signup, login, logout, password reset with argon2id and secure server-side sessions.
- OAuth client (relying-party) registration via an admin endpoint that proxies Hydra's admin API.
- Security baseline (hashing params, cookie attributes, CSRF, rate limiting, security headers, secret handling) set here for all later changes to inherit.
- Every HTTP endpoint visible in Swagger UI; `/metrics` and `/healthz` exposed; structured JSON logs.
- A clean, extensible layout: auth methods behind an interface; social/passkey/admin slot in without rework.

**Non-Goals:**
- Passkeys, TOTP, social login, account self-service UI, the admin console (later changes 2–7).
- Multi-tenancy, email *delivery* infrastructure (reset tokens are issued and logged/stubbed via a pluggable `Mailer`; SMTP wiring is a later concern).
- Production secret management (Vault/KMS); this change uses env/compose secrets with a documented path to a manager.
- High-availability Hydra/Postgres topology; single-node compose is the target.

## Decisions

### D1 — Delegate OIDC to Ory Hydra; cotton-id is the login + consent provider
Hydra owns `/oauth2/auth`, `/oauth2/token`, JWKS, PKCE, and refresh-token rotation. It has no user DB and no UI — it redirects to cotton-id with a `login_challenge` / `consent_challenge`, and cotton-id calls Hydra's admin API to *accept/reject*. cotton-id owns users, credentials, sessions, the login/consent UI, and the client registry.
- **Why:** a certified protocol engine eliminates the highest-severity class of bugs (token/PKCE/JWKS mistakes) while cotton-id remains the IdP brand and identity owner.
- **Alternatives:** (a) hand-roll with `ory/fosite` — maximum control, maximum security ownership, rejected for a production v1; (b) full Keycloak/Zitadel — they own the user store and UI, conflicting with the bespoke cotton-id design.

### D2 — Backend layout: `cmd/` + `internal/` layered by domain
`backend/cmd/cotton-id/main.go` wires config → logger → db → Hydra client → services → router. `internal/` holds `config`, `database` (pgx pool + migrations), `httpx` (router, middleware, problem+json errors), `auth` (password, session, user store), `oidc` (Hydra client, login/consent handlers), `client` (RP registry), `observability` (metrics, logging), `openapi`. Each domain exposes interfaces so later changes add `passkey`, `social`, `admin` packages without touching existing ones.
- **Why:** standard idiomatic Go; the interface seams are the "extensibility" requirement made concrete.

### D3 — Credentials: argon2id via `golang.org/x/crypto/argon2`
Parameters: `time=3`, `memory=64MiB`, `threads=4`, 16-byte salt, 32-byte key, encoded in the standard `$argon2id$v=19$...` PHC string so parameters can evolve and rehash-on-login is possible. Password policy mirrors the prototype's strength meter (min 8 chars; bonuses for case mix, digits, symbols) with a minimum acceptable strength enforced server-side — the client meter is advisory only.
- **Alternatives:** bcrypt (rejected: weaker against GPU), scrypt (acceptable; argon2id is the modern default).

### D4 — Sessions: opaque server-side tokens in Postgres, cookie-delivered
On login, create a `sessions` row (random 256-bit id, user id, created/expires, user-agent, ip) and set cookie `cid_session` = opaque id. Attributes: `HttpOnly`, `Secure`, `SameSite=Lax`, `Path=/`. "Remember me" sets a 30-day expiry; otherwise a session cookie (browser-session) with a 24-hour server expiry. Logout deletes the row. This same table backs the later "active sessions / revoke" feature with no schema change.
- **Why:** server-side sessions are revocable (unlike stateless JWT cookies) and the row model is exactly what the design's "Sessions" surface needs. Opaque ids avoid putting anything sensitive in the cookie.

### D5 — CSRF: synchronizer-token (double-submit) on browser-posted forms
Login, signup, password-reset, logout, and the consent POST require a CSRF token (`cid_csrf` cookie + `X-CSRF-Token` header / form field), validated by middleware. Pure machine-to-machine admin endpoints authenticated by a bearer/admin key are exempt and instead require that key.
- **Why:** the consent and login POSTs are browser-driven and state-changing; SameSite=Lax is defense-in-depth, not sufficient alone.

### D6 — Rate limiting + lockout
Per-IP and per-account token-bucket limits on `/auth/login`, `/auth/signup`, `/auth/password/reset` (in-memory limiter behind an interface so it can move to Redis later). Repeated login failures apply incremental backoff. Limits and lockout thresholds are config-driven.

### D7 — HTTP stack: chi router, RFC 7807 problem+json errors, OpenAPI via swaggo annotations
`go-chi/chi` for routing/middleware; errors returned as `application/problem+json`. OpenAPI: annotate handlers with `swaggo/swag` to generate `docs/swagger.json` served at `/swagger/`. A CI check fails if generated docs drift from annotations, guaranteeing "all endpoints in Swagger."
- **Alternatives:** spec-first `oapi-codegen` (stronger contract, more upfront ceremony) — deferred; annotation-first keeps the skeleton moving and still produces the required Swagger.

### D8 — Observability: prometheus/client_golang + slog
`/metrics` exposes Go runtime metrics plus app counters/histograms (HTTP request duration by route/status, login success/failure, signups, consent grants, Hydra admin call latency). Logging via stdlib `log/slog` as JSON with a request-id middleware; every security-relevant event (login ok/fail, signup, reset, consent, client registration) is logged with structured fields, forming the seed of the later audit log.

### D9 — Frontend: Vite + React + TypeScript, port the prototype's token system verbatim
The prototype's CSS custom properties (`index.html` `:root`/theme blocks), glass primitives, blob background, fonts, and the `Field`/`Button`/`Toggle`/`Icon`/`Logo`/`LangSwitch`/`ThemeSwitch` components are reimplemented as typed React components in `frontend/src`. i18n uses the existing RU/EN dictionary shape (`i18n.ts`). A typed `apiClient` wraps `fetch` with credentials, CSRF header injection, and problem+json parsing. React Router handles `/`, `/login`, `/signup`, `/consent`. Apple social button is rendered but **disabled with a "soon" affordance** (stub per product decision); Google/GitHub buttons route to the backend social-start endpoint (implemented in a later change — here they are present but inert/stubbed).
- **Why:** fidelity to the approved design while gaining type safety and a real build pipeline (the prototype used CDN React + Babel-in-browser, unsuitable for production).

### D10 — Migrations: SQL files via `golang-migrate`, run on startup + as a one-shot
Plain `.sql` up/down files in `backend/migrations`, embedded with `embed.FS`, applied by a migrate step. Compose runs migrations before the backend serves. Hydra runs its own `hydra migrate sql` against a separate database/schema in the same Postgres instance.

### D11 — Configuration: env-driven, 12-factor, typed and validated at boot
A `Config` struct loaded from env (with a `.env.example`), validated at startup (fail fast on missing secrets). Secrets (DB password, Hydra system secret, session/CSRF keys, cookie encryption key) are injected via compose env / secrets, never committed. Cookie/session signing keys are required in non-dev and refuse weak defaults.

## Risks / Trade-offs

- **Hydra operational complexity** → Mitigation: pin a known Hydra image tag, encapsulate all Hydra calls behind one `oidc.HydraClient`, document the login/consent contract, and cover the flow with an integration test that runs Hydra in compose.
- **Local toolchain is Go 1.23, target 1.25** → Mitigation: `GOTOOLCHAIN=auto` for local, `golang:1.25` image for the canonical build; CI builds in Docker so the version is authoritative.
- **Account-linking foot-gun (future social login)** → Mitigation now: the `users` schema stores `email_verified`; identity-linking will be allowed only on verified email. The seam is in the data model from day one.
- **Password-only accounts are single-factor** (TOTP deferred) → Mitigation: keep a `mfa_*` seam in the schema and document the gap; passkeys (change 2) provide the strong-auth path.
- **CSRF + SPA + cross-origin in dev** → Mitigation: serve frontend and API same-origin via the compose reverse proxy / Vite proxy; SameSite=Lax + explicit CSRF token avoids cross-site posts.
- **In-memory rate limiter doesn't survive multi-replica** → Accepted for single-node v1; interface allows a Redis backend later.
- **Annotation-drift in Swagger** → Mitigation: a CI step regenerates docs and fails on diff.

## Migration Plan

Greenfield; nothing to migrate from. Deploy path: `docker compose up` runs Postgres → Hydra migrate → cotton-id migrate → backend → frontend. Rollback = `docker compose down` (data in named volumes). Each later change adds migrations forward-only; `down` files exist for local dev only.

## Open Questions

- Email delivery provider for password-reset and login-notification emails (deferred; `Mailer` interface with a logging/dev implementation now).
- Whether to adopt spec-first `oapi-codegen` before the API surface grows large (revisit at change 4–5).
- Final argon2id memory parameter on the target host (64 MiB assumed; tune to deployment RAM).
