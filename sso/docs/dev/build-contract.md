# cotton-id — Build Contract (Change 1: walking skeleton)

This is the **binding interface contract** for implementing `bootstrap-platform-and-oidc-login`.
Every implementer (human or agent) MUST follow these exact names, paths, routes, schemas, env
vars, and ports so the independently-built pieces compose. Behavior/requirements live in
`openspec/changes/bootstrap-platform-and-oidc-login/` (proposal, design, specs, tasks) — read those
for the "why" and the acceptance scenarios. This file pins the "exact shapes".

## 0. Stack & versions

- Backend: Go **1.25** (`GOTOOLCHAIN=auto`), module path **`cotton-id`**.
- Frontend: React + TypeScript via **Vite**, Node 24.
- DB: **PostgreSQL 16**. OIDC engine: **Ory Hydra v2** (image `oryd/hydra:v2.2.0`).
- Everything runs via **Docker Compose**.

## 1. Repository layout

```
backend/
  go.mod                      # module cotton-id, go 1.25
  cmd/cotton-id/main.go       # composition root (owns wiring; single owner)
  internal/
    config/      config.go                 # typed env config + Validate()
    observability/ logging.go metrics.go    # slog JSON, prometheus registry+middleware
    database/    db.go migrate.go           # pgxpool + custom embedded-SQL migrator
    httpx/       router.go middleware.go errors.go csrf.go ratelimit.go render.go
    auth/        argon2.go password.go user.go session.go store.go service.go handlers.go authenticator.go
    oidc/        hydra.go login.go consent.go claims.go handlers.go
    adminapi/    clients.go handlers.go     # OAuth client (RP) registration over Hydra admin
    mailer/      mailer.go                  # interface + dev logger impl
  migrations/    0001_init.up.sql 0001_init.down.sql ...
  docs/          # swaggo-generated (docs.go, swagger.json, swagger.yaml)
frontend/
  package.json vite.config.ts tsconfig.json index.html
  src/ main.tsx App.tsx routes/ components/ lib/ i18n/ styles/
deploy/
  docker-compose.yml .env.example
  hydra/hydra.yml
  prometheus/prometheus.yml
docs/  ARCHITECTURE.md SECURITY.md API.md RUNBOOK.md dev/build-contract.md
```

`main.go` is the ONLY file that wires packages together; package authors expose constructors and
`Routes(r chi.Router)` / handler structs and never edit `main.go`'s siblings.

## 2. Go dependencies (resolved via `go mod tidy`)

- `github.com/go-chi/chi/v5`, `github.com/go-chi/cors`
- `github.com/jackc/pgx/v5` (+ `pgxpool`)
- `github.com/prometheus/client_golang`
- `golang.org/x/crypto` (argon2), `golang.org/x/time/rate`
- `github.com/google/uuid`
- `github.com/swaggo/swag`, `github.com/swaggo/http-swagger/v2`, `github.com/swaggo/files`

Hydra admin calls, env loading, and migrations are **hand-written** (no extra deps).

## 3. HTTP API (exact)

Base path `/api/v1`. JSON bodies camelCase. Errors are `application/problem+json`
(`{type,title,status,detail,instance}`). State-changing browser routes require CSRF
(`X-CSRF-Token` header == `cid_csrf` cookie). Admin routes require `X-Admin-Key: <ADMIN_API_KEY>`.

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| GET | `/api/v1/csrf` | none | → 200 `{token}` (also sets `cid_csrf` cookie) |
| POST | `/api/v1/auth/signup` | CSRF | `{displayName,username,email,password}` → 201 `{user}` + session cookie |
| POST | `/api/v1/auth/login` | CSRF | `{email,password,remember}` → 200 `{user}` + session cookie |
| POST | `/api/v1/auth/logout` | CSRF+session | → 204 |
| GET | `/api/v1/auth/session` | session | → 200 `{user}` / 401 |
| POST | `/api/v1/auth/password/forgot` | CSRF | `{email}` → 202 `{message}` (never enumerates) |
| POST | `/api/v1/auth/password/reset` | CSRF | `{token,password}` → 204 |
| POST | `/api/v1/admin/clients` | admin-key | `{name,redirectUris,scopes,grantTypes,responseTypes,clientType}` → 201 `{clientId,clientSecret?}` |
| GET | `/api/v1/admin/clients` | admin-key | → 200 `{clients:[...]}` |
| DELETE | `/api/v1/admin/clients/{id}` | admin-key | → 204 |
| GET | `/api/v1/oauth/consent?consent_challenge=` | session | → 200 `{client:{id,name},requestedScopes,user}` |
| POST | `/api/v1/oauth/login/accept` | CSRF+session | `{loginChallenge}` → 200 `{redirectTo}` |
| POST | `/api/v1/oauth/consent/accept` | CSRF+session | `{consentChallenge,grantScopes,remember}` → 200 `{redirectTo}` |
| POST | `/api/v1/oauth/consent/reject` | CSRF+session | `{consentChallenge}` → 200 `{redirectTo}` |

Browser-redirect endpoints (served by backend, not under `/api`):
| GET | `/oauth/login?login_challenge=` | Hydra entry: accept if session can skip, else 302 → `FRONTEND_BASE_URL/login?login_challenge=` |
| GET | `/oauth/consent?consent_challenge=` | accept if skippable, else 302 → `FRONTEND_BASE_URL/consent?consent_challenge=` |
| GET | `/oauth/logout?logout_challenge=` | accept logout via Hydra, clear session, 302 → post-logout URL |

Ops: `GET /healthz` (200/503 + dependency report), `GET /metrics`, `GET /swagger/*`.

### Login/consent handshake (pin this flow)
1. RP → `GET HYDRA_PUBLIC/oauth2/auth?...` → Hydra 302 → `GET /oauth/login?login_challenge=C`.
2. Backend GETs login request from Hydra admin. If `skip` or valid cotton-id session → accept login (subject = user id) → 302 to Hydra `redirect_to`. Else 302 → SPA `/login?login_challenge=C`.
3. SPA collects credentials → `POST /api/v1/auth/login` (session established) → then `POST /api/v1/oauth/login/accept {loginChallenge:C}` → `{redirectTo}` → SPA `window.location = redirectTo` (back to Hydra).
4. Hydra 302 → `GET /oauth/consent?consent_challenge=K`. If skippable → accept → 302 back. Else 302 → SPA `/consent?consent_challenge=K`.
5. SPA `GET /api/v1/oauth/consent?consent_challenge=K` → shows client+scopes → user accepts → `POST /api/v1/oauth/consent/accept {consentChallenge:K,grantScopes,remember}` → `{redirectTo}` → SPA navigates → Hydra issues code → RP exchanges at `/oauth2/token`.

## 4. Database schema (migration 0001)

```sql
CREATE TABLE users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email         CITEXT UNIQUE NOT NULL,
  email_verified BOOLEAN NOT NULL DEFAULT FALSE,
  username      CITEXT UNIQUE NOT NULL,
  display_name  TEXT NOT NULL,
  password_hash TEXT,                         -- nullable: future social-only accounts
  status        TEXT NOT NULL DEFAULT 'active', -- active|invited|suspended
  role          TEXT NOT NULL DEFAULT 'user',   -- user|admin|owner (seam; admin UI later)
  about         TEXT NOT NULL DEFAULT '',
  location      TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
  id          TEXT PRIMARY KEY,               -- sha256(opaque token) hex
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  remember    BOOLEAN NOT NULL DEFAULT FALSE,
  user_agent  TEXT NOT NULL DEFAULT '',
  ip          TEXT NOT NULL DEFAULT '',
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at  TIMESTAMPTZ NOT NULL
);
CREATE TABLE password_reset_tokens (
  token_hash  TEXT PRIMARY KEY,               -- sha256(token) hex
  user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at  TIMESTAMPTZ NOT NULL,
  used_at     TIMESTAMPTZ
);
-- requires extensions: citext, pgcrypto (gen_random_uuid)
```
The cotton-id app uses its own database/schema; **Hydra uses a separate database** in the same Postgres instance (`hydra` DB), migrated by `hydra migrate sql`.

## 5. Configuration (env vars — exact names)

| Var | Example | Notes |
|---|---|---|
| `COTTON_ENV` | `development` | `development`\|`production`; prod forbids weak secrets |
| `HTTP_ADDR` | `:8080` | backend listen addr |
| `PUBLIC_BASE_URL` | `http://localhost:8080` | backend's external URL |
| `FRONTEND_BASE_URL` | `http://localhost:3000` | SPA origin for redirects |
| `DATABASE_URL` | `postgres://cotton:cotton@postgres:5432/cottonid?sslmode=disable` | |
| `HYDRA_ADMIN_URL` | `http://hydra:4445` | admin API (internal only) |
| `HYDRA_PUBLIC_URL` | `http://localhost:4444` | public OIDC issuer |
| `SESSION_COOKIE_NAME` | `cid_session` | |
| `CSRF_COOKIE_NAME` | `cid_csrf` | |
| `ADMIN_API_KEY` | (32+ random) | required in prod |
| `SESSION_TTL_HOURS` | `24` | non-remember TTL |
| `SESSION_REMEMBER_DAYS` | `30` | remember TTL |
| `PASSWORD_RESET_TTL_MINUTES` | `30` | |
| `RATE_LIMIT_RPS` / `RATE_LIMIT_BURST` | `5` / `10` | auth endpoints |
| `LOGIN_LOCKOUT_THRESHOLD` | `5` | consecutive login failures → incremental-backoff lockout |
| `TRUSTED_PROXIES` | (empty) | CSV of proxy CIDRs whose `X-Forwarded-For` is trusted; empty = use direct peer IP (fail-safe) |
| `LOG_LEVEL` | `info` | |
| `COOKIE_SECURE` | `false` in dev | force Secure attr + enable HSTS |

Secret vars (`ADMIN_API_KEY`, `POSTGRES_PASSWORD`, `HYDRA_SYSTEM_SECRET`, `HYDRA_COOKIE_SECRET`)
have **no working default** in compose (`${VAR:?...}`) — copy `deploy/.env.example` → `deploy/.env`.
`Config.Validate` rejects placeholder/`*-insecure-*`/`change-me` secrets in production.

Cookie attributes: `HttpOnly`, `SameSite=Lax`, `Secure` per `COOKIE_SECURE`, `Path=/`.
argon2id params (code defaults): time=3, memory=64*1024 KiB, threads=4, saltLen=16, keyLen=32.

## 6. Ports

backend `8080`, frontend(nginx) `3000`→80, hydra public `4444`, hydra admin `4445`,
postgres `5432`, prometheus `9090`.

## 7. Frontend specifics

- Port the design tokens **verbatim** from `_design_ref/index.html` `<style>` (`:root`, dark/light,
  glass, blobs, fonts Instrument Serif + Hanken Grotesk) into `src/styles/tokens.css`.
- Reimplement components from `_design_ref/ui.jsx` + `_design_ref/screen-auth.jsx`:
  `Logo, Field, Button, Toggle, Icon, LangSwitch, ThemeSwitch, SocialRow` (typed).
- i18n: typed dictionary from `_design_ref/i18n.jsx` (RU default + EN) in `src/i18n/`.
- Routes (react-router): `/` landing, `/login`, `/signup`, `/consent`. Read `login_challenge` /
  `consent_challenge` query params and drive the handshake in §3.
- `src/lib/api.ts`: typed fetch client — `credentials:'include'`, injects `X-CSRF-Token` from the
  `/api/v1/csrf` call, parses problem+json into typed errors.
- Apple button: rendered **disabled** with a "soon" tooltip (stub). Google/GitHub buttons present;
  they call `/api/v1/auth/social/{provider}/start` which returns 501 Not Implemented for now (stub
  endpoint), so the UI is wired but the providers land in change 3.
- Dev: Vite proxies `/api`, `/oauth`, `/healthz`, `/swagger` → `http://backend:8080`.
  Prod: nginx serves the SPA and reverse-proxies the same prefixes to the backend (same origin).

## 8. Quality gates (every implementer)

- Backend MUST `go build ./...`, `go vet ./...`, and `gofmt` clean. New logic carries tests.
- Frontend MUST `tsc --noEmit` and `vite build` clean.
- Every new endpoint MUST carry swaggo annotations so it appears in Swagger.
- No secret values committed; `.env.example` documents every var.
- Security events (login ok/fail, signup, reset, consent, client reg) logged structured.
