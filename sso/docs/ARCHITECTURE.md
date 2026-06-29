# Architecture

This document describes how cotton-id is put together: the runtime topology, the split between
cotton-id and Ory Hydra, the login/consent sequence Hydra delegates to us, the backend package map,
and the extensibility seams that let later changes (passkeys, social login, admin console) slot in
without rework.

It describes the **intended/contracted** behavior, pinned by
[docs/dev/build-contract.md](dev/build-contract.md) and the design decisions in
[`openspec/changes/bootstrap-platform-and-oidc-login/design.md`](../openspec/changes/bootstrap-platform-and-oidc-login/design.md).
For the live, always-current request/response shapes, see Swagger at `/swagger/`.

---

## 1. Topology

cotton-id runs as four containers orchestrated by a single Docker Compose file, plus an optional
Prometheus service.

```
                            ┌────────────────────────────────────────────────────────┐
   Browser                  │  Docker Compose network                                 │
   (user agent)            │                                                          │
      │                     │   ┌──────────────┐         ┌──────────────────────────┐ │
      │  :3000              │   │   frontend   │  /api    │        backend           │ │
      ├────────────────────►│   │ nginx + SPA  │  /oauth  │   Go 1.25 · chi router   │ │
      │                     │   │ React + Vite │─────────►│                          │ │
      │  :8080 (direct,     │   └──────────────┘ proxied  │  httpx → auth            │ │
      │   dev/ops only)     │                             │        → oidc            │ │
      ├─────────────────────┼────────────────────────────►│        → adminapi        │ │
      │                     │                             │        → observability   │ │
      │  :4444 (RP starts   │   ┌──────────────┐  admin   │        → config/database │ │
      │   OIDC flow here)   │   │  Ory Hydra   │◄─────────┤                          │ │
      └─────────────────────┼──►│   v2.2.0     │  :4445   └────────────┬─────────────┘ │
                            │   │ public :4444 │                       │ pgxpool        │
                            │   │ admin  :4445 │          ┌────────────▼─────────────┐ │
                            │   └──────┬───────┘          │       PostgreSQL 16       │ │
                            │          │ hydra DB         │  cottonid DB │  hydra DB  │ │
                            │          └─────────────────►│ (app schema) │ (engine)   │ │
                            │                             └──────────────────────────┘ │
                            │   ┌──────────────┐                                        │
                            │   │  Prometheus  │── scrapes ─► backend /metrics          │
                            │   │    :9090     │                                        │
                            │   └──────────────┘                                        │
                            └────────────────────────────────────────────────────────┘
```

Key topology facts:

- **Single tenant.** One identity domain, no `tenant_id` partitioning anywhere.
- **One Postgres instance, two databases.** The app uses `cottonid`; Hydra uses a separate `hydra`
  database in the same instance, migrated by `hydra migrate sql`. They never share tables.
- **Hydra's admin API (`:4445`) is internal only.** Only the backend calls it; it is never exposed
  to browsers or relying parties.
- **Same-origin in the browser.** In production, nginx serves the SPA and reverse-proxies `/api`,
  `/oauth`, `/healthz`, and `/swagger` to the backend, so the browser only ever talks to one origin
  (port 3000). In dev, Vite's proxy does the same. This keeps cookies first-party and CSRF simple.
- **Startup ordering.** Postgres → Hydra migrate → Hydra serve → cotton-id migrate → backend serve →
  frontend. The backend retries dependency connections with backoff and only binds HTTP after
  migrations apply (platform-foundation spec: "Backend waits for its dependencies").

---

## 2. The cotton-id / Hydra split

The defining architectural decision (design **D1**): cotton-id is a **production** IdP, so it must
not hand-roll OIDC protocol crypto. The system is split at the protocol boundary.

| Concern | Owner |
|---|---|
| `/oauth2/auth`, `/oauth2/token`, JWKS, discovery | **Hydra** |
| PKCE verification, refresh-token rotation, token signing | **Hydra** |
| Authorization codes, access tokens, ID tokens | **Hydra** (minted; cotton-id never sees them) |
| Users, credentials (argon2id), email verification | **cotton-id** |
| Server-side sessions, cookies | **cotton-id** |
| Login UI + consent UI | **cotton-id** |
| Login/consent challenge handling (accept/reject) | **cotton-id** (via Hydra admin API) |
| ID-token claims (sub, email, name, …) | **cotton-id** (supplied at consent-accept) |
| OAuth client (relying-party) registry | **cotton-id** admin endpoint → proxies Hydra |

Hydra has **no user database and no UI**. When it needs to know *who* a user is or *what* they
consent to, it 302-redirects the browser to cotton-id with a `login_challenge` or
`consent_challenge`, and cotton-id resolves it by calling Hydra's admin API to accept or reject.

**Why this split** (design D1): a certified protocol engine eliminates the highest-severity class of
bugs (token/PKCE/JWKS mistakes) while cotton-id remains the IdP brand and identity owner. Rejected
alternatives: hand-rolling with `ory/fosite` (maximum security ownership, too risky for v1) and full
Keycloak/Zitadel (they own the user store and UI, conflicting with the bespoke cotton-id design).

---

## 3. The login / consent sequence

This is the authoritative step list from build-contract §3, "Login/consent handshake". The
relying-party (RP) endpoints are Hydra's; cotton-id serves the `/oauth/*` browser-redirect endpoints
and the `/api/v1/oauth/*` JSON endpoints the SPA calls.

```
 RP        Hydra (public :4444)      cotton-id backend           cotton-id SPA (:3000)
  │                │                       │                            │
  │ 1. GET /oauth2/auth?response_type=code&client_id&redirect_uri      │
  │    &scope&state&code_challenge (PKCE)  │                            │
  ├───────────────►│                       │                            │
  │                │ 302 /oauth/login?login_challenge=C                 │
  │                ├──────────────────────►│                            │
  │                │            2. GET login request from Hydra admin.  │
  │                │            If `skip` OR valid cid_session →         │
  │                │            accept login (subject = user id) →       │
  │                │            302 to Hydra redirect_to. ───────┐       │
  │                │◄─────────────────────────────────────────────┘     │
  │                │            Else 302 → SPA /login?login_challenge=C  │
  │                │                       ├───────────────────────────►│
  │                │                       │  3. POST /api/v1/auth/login │
  │                │                       │◄───────────────────────────┤ (session established)
  │                │                       │  POST /api/v1/oauth/login/accept {loginChallenge:C}
  │                │                       │◄───────────────────────────┤
  │                │                       │  → {redirectTo} ───────────►│ window.location = redirectTo
  │                │◄──────────────────────────────────────────────────┤
  │                │ 4. 302 /oauth/consent?consent_challenge=K          │
  │                ├──────────────────────►│                            │
  │                │            If skippable (remembered) → accept →     │
  │                │            302 back. Else 302 → SPA /consent?…=K    │
  │                │                       ├───────────────────────────►│
  │                │                       │  5. GET /api/v1/oauth/consent?consent_challenge=K
  │                │                       │◄───────────────────────────┤ (shows client + scopes)
  │                │                       │  POST /api/v1/oauth/consent/accept
  │                │                       │     {consentChallenge:K,grantScopes,remember}
  │                │                       │◄───────────────────────────┤
  │                │                       │  → {redirectTo} ───────────►│ window.location = redirectTo
  │                │◄──────────────────────────────────────────────────┤
  │ 6. 302 redirect_uri?code=…&state=…     │                            │
  │◄───────────────┤                       │                            │
  │ 7. POST /oauth2/token (code + PKCE verifier)                        │
  ├───────────────►│  → { id_token, access_token, refresh_token? }      │
  │◄───────────────┤                       │                            │
```

Step-by-step (matches build-contract §3):

1. **RP → Hydra.** The RP redirects to `GET HYDRA_PUBLIC/oauth2/auth?...`. Hydra 302s to
   `GET /oauth/login?login_challenge=C` on the backend.
2. **Login challenge.** The backend GETs the login request from Hydra's admin API. If Hydra says
   `skip` (already authenticated to Hydra) **or** the browser carries a valid cotton-id session, the
   backend accepts the login (subject = user id) and 302s to Hydra's `redirect_to`. Otherwise it
   302s to the SPA `/login?login_challenge=C`.
3. **Credentials.** The SPA collects credentials and `POST`s `/api/v1/auth/login` (establishing a
   session), then `POST`s `/api/v1/oauth/login/accept {loginChallenge:C}` and receives
   `{redirectTo}`; the SPA sets `window.location = redirectTo` (back to Hydra).
4. **Consent challenge.** Hydra 302s to `GET /oauth/consent?consent_challenge=K`. If skippable
   (a prior "remember" grant covers the scopes), the backend accepts and 302s back. Otherwise it
   302s to the SPA `/consent?consent_challenge=K`.
5. **Consent.** The SPA `GET`s `/api/v1/oauth/consent?consent_challenge=K` to show the client and
   requested scopes, the user accepts, and the SPA `POST`s
   `/api/v1/oauth/consent/accept {consentChallenge:K,grantScopes,remember}`, receives `{redirectTo}`,
   and navigates. Hydra issues the code.
6. **Token exchange.** The RP exchanges the `code` (with its PKCE verifier) at `/oauth2/token` for an
   ID token and access token.

**Subject stability** (oidc-provider spec): the `sub` claim is the cotton-id user's UUID — stable
across sessions and clients, and never reused for a different account. The claims mapper
(`internal/oidc/claims.go`) populates `email`, `email_verified`, `name`, and `preferred_username`
per granted scope when consent is accepted.

**Logout / back-channel.** `GET /oauth/logout?logout_challenge=…` accepts the Hydra logout
challenge (best-effort), clears the cotton-id session, and 302s to the post-logout URL.

---

## 4. Backend package map

The backend follows the standard idiomatic Go layout: a thin `cmd/` composition root plus
`internal/` packages layered by domain (design **D2**). `cmd/cotton-id/main.go` is the **only** file
that wires packages together — package authors expose constructors and `Routes(r chi.Router)` /
handler structs and never edit `main.go`'s siblings.

```
backend/
  cmd/cotton-id/main.go        Composition root: config → logger → db+migrate → metrics →
                               Hydra client → services → router → graceful shutdown. Single owner.
  internal/
    config/                    config.go — typed Config loaded from env + Validate() (fail-fast on
                               missing secrets / weak defaults outside development).
    observability/             logging.go — slog JSON logger + request-id middleware.
                               metrics.go — Prometheus registry + HTTP metrics middleware.
    database/                  db.go — pgxpool with retry/backoff + health-check query.
                               migrate.go — custom embedded-SQL migrator over migrations/.
    httpx/                     router.go — chi router factory + route mounting.
                               middleware.go — security headers, CORS, recovery.
                               errors.go — RFC 7807 application/problem+json helpers.
                               csrf.go — double-submit CSRF middleware (cookie + header).
                               ratelimit.go — per-IP/per-account token-bucket limiter (interface).
                               render.go — JSON response helpers.
    auth/                      argon2.go — argon2id hash/verify, PHC encode/decode.
                               password.go — password policy + strength check.
                               user.go — user model + user store (create, get-by-*, update).
                               session.go — opaque session token + session model.
                               store.go — session store (create, get, delete, purge-expired).
                               service.go — signup/login/logout/reset orchestration.
                               handlers.go — /api/v1/auth/* HTTP handlers (swaggo-annotated).
                               authenticator.go — Authenticator interface + password impl (seam).
    oidc/                      hydra.go — HydraClient wrapper over Hydra admin API.
                               login.go — login challenge: get/accept/reject.
                               consent.go — consent challenge: get/accept/reject + remember.
                               claims.go — ID-token claims mapper (per-scope).
                               handlers.go — /oauth/* redirect + /api/v1/oauth/* JSON handlers.
    adminapi/                  clients.go — OAuth client (RP) registry over Hydra admin.
                               handlers.go — /api/v1/admin/clients CRUD (admin-key authorized).
    mailer/                    mailer.go — Mailer interface + dev logger implementation.
  migrations/                  0001_init.up.sql / 0001_init.down.sql, … (forward-only).
  docs/                        swaggo-generated: docs.go, swagger.json, swagger.yaml.
```

Request flow through the stack: chi router → security-headers/CORS/request-id/recovery middleware →
(for browser POSTs) CSRF middleware → (for auth routes) rate-limit middleware → domain handler →
service → store → pgxpool. Errors bubble up as `application/problem+json`
(`{type,title,status,detail,instance}`) and never leak stack traces.

### Data model

Three tables in the `cottonid` database (migration `0001_init`), per build-contract §4:

- **`users`** — `id` (UUID, the OIDC `sub`), `email` (CITEXT unique), `email_verified`, `username`
  (CITEXT unique), `display_name`, `password_hash` (nullable — for future social-only accounts),
  `status` (`active|invited|suspended`), `role` (`user|admin|owner` — a seam for the future admin
  UI), `about`, `location`, timestamps.
- **`sessions`** — `id` = `sha256(opaque token)` hex, `user_id`, `remember`, `user_agent`, `ip`,
  `created_at`, `expires_at`. This same table backs the future "active sessions / revoke" surface
  with no schema change.
- **`password_reset_tokens`** — `token_hash` = `sha256(token)` hex, `user_id`, `created_at`,
  `expires_at`, `used_at` (single-use). Requires the `citext` and `pgcrypto` extensions.

---

## 5. Extensibility seams

The walking skeleton is deliberately shaped so changes 2–7 (passkeys, social login, account
self-service, admin console) slot in without touching existing packages. The concrete seams:

- **`Authenticator` interface (`internal/auth/authenticator.go`).** The login path depends on an
  `Authenticator` abstraction, not directly on password verification. The change-1 implementation is
  password-only. Change 2 (passkeys) and change 3 (social login) add new implementations behind the
  same interface; the login handler and the Hydra login-accept logic don't change — they just gain
  another method that resolves to the same stable `sub`.
- **Nullable `password_hash` + `email_verified`.** The `users` schema already allows social-only
  accounts (no password) and records `email_verified` so that future **identity linking** can be
  restricted to verified emails — closing the account-takeover foot-gun before it exists (design
  risk note). The data-model seam is present from day one.
- **`role` column (`user|admin|owner`).** Present now, unused now. The future admin console reads it
  for authorization; no migration needed to light it up.
- **Sessions table as the "active sessions" backend.** Each session row already carries
  `user_agent`, `ip`, `created_at`, and `expires_at` — exactly the fields the design's "Active
  sessions / revoke" surface needs. Revocation = delete the row (already supported by the store).
- **Rate limiter behind an interface (`internal/httpx/ratelimit.go`).** In-memory token bucket now;
  the interface lets a Redis-backed limiter drop in for multi-replica deployments later.
- **`Mailer` interface (`internal/mailer/mailer.go`).** A dev logger implementation now (reset tokens
  are logged, not emailed). A real SMTP/provider implementation drops in behind the interface when
  email delivery is wired.
- **New domain packages, not edits.** Because `main.go` is the sole wiring point and every package
  exposes constructors + `Routes(r chi.Router)`, adding `passkey`, `social`, or `admin` packages
  means new files plus a few lines in `main.go` — existing packages stay closed for modification.
- **`mfa_*` schema seam (documented gap).** TOTP is deferred (change ≥2). The schema leaves room for
  `mfa_*` columns; password-only accounts are single-factor until then. See
  [SECURITY.md](SECURITY.md) "Known gaps".

---

## 6. Observability

- **Logging** (`internal/observability/logging.go`): `log/slog` JSON to stdout, with a request-id
  middleware that correlates every log line. Every security-relevant event — login ok/fail, signup,
  password reset, consent decision, client registration, throttling — is logged with structured
  fields. Plaintext passwords and full session tokens are **never** logged. These logs are the seed
  of the future audit log (design D8).
- **Metrics** (`internal/observability/metrics.go`): `/metrics` exposes Go runtime metrics plus app
  counters/histograms — `http_request_duration_seconds` labeled by route and status, login
  success/failure, signups, consent grants, and Hydra admin-call latency.
- **Health** (`/healthz`): returns 200 with a dependency report when Postgres and Hydra are
  reachable, and a non-200 when a critical dependency is down — driving the Compose health checks.

---

## 7. Configuration

All configuration is loaded from environment variables into a typed `Config` and validated at
startup (design **D11**, 12-factor). The process exits non-zero **before** binding the HTTP port if
a required secret is missing or a security-critical key uses a known-insecure default outside
development. A committed `deploy/.env.example` documents every variable with safe placeholders; no
real secrets are committed. See [docs/dev/build-contract.md](dev/build-contract.md) §5 for the exact
variable names and [docs/RUNBOOK.md](RUNBOOK.md) for operating them.
