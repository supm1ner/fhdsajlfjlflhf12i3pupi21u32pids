## 1. Repository & toolchain scaffold

- [x] 1.1 Create monorepo layout: `backend/`, `frontend/`, `deploy/`, `docs/`; add root `README.md`, `.gitignore`, `.editorconfig`, `.dockerignore`, `Makefile`
- [x] 1.2 Initialize Go module `cotton-id` targeting `go 1.25`; add `GOTOOLCHAIN=auto` note; create `backend/cmd/cotton-id/main.go` placeholder
- [x] 1.3 Add core Go dependencies (chi, pgx/pgxpool, golang-migrate, x/crypto/argon2, prometheus/client_golang, swaggo/swag + http-swagger, ory hydra admin client or generated client, google/uuid, caarlos0/env or similar)
- [x] 1.4 Commit `.env.example` documenting every config var with safe placeholders

## 2. Platform foundation (backend substrate)

- [x] 2.1 `internal/config`: typed `Config` loaded + validated from env; fail-fast on missing secrets / weak defaults outside dev
- [x] 2.2 `internal/observability`: slog JSON logger, request-id middleware, Prometheus registry + HTTP metrics middleware, `/metrics` handler
- [x] 2.3 `internal/database`: pgxpool connection with retry/backoff; `migrations` embedded FS + golang-migrate runner; health check query
- [x] 2.4 `internal/httpx`: chi router factory, security-headers + CORS middleware, RFC7807 problem+json error helpers, recovery middleware, `/healthz`
- [x] 2.5 `internal/httpx`: CSRF synchronizer-token middleware (cookie + header/form), exempting admin/bearer routes
- [x] 2.6 `internal/httpx`: per-IP/per-account token-bucket rate limiter behind an interface, applied to auth routes
- [x] 2.7 Wire `main.go`: config → logger → db+migrate → metrics → router → graceful shutdown
- [x] 2.8 swaggo annotations + `swag init` generating `docs/`; serve Swagger UI at `/swagger/`; Makefile target `swagger`

## 3. Password authentication & sessions

- [x] 3.1 Migration: `users` table (id, email, email_verified, username, display_name, password_hash, status, created_at, updated_at) with unique indexes; `mfa`/profile seam columns documented
- [x] 3.2 Migration: `sessions` table (id, user_id, expires_at, remember, user_agent, ip, created_at) and `password_reset_tokens` table (token_hash, user_id, expires_at, used_at)
- [x] 3.3 `internal/auth`: argon2id hash + verify (PHC encode/decode, configurable params, constant-time compare); password policy + strength check mirroring the prototype
- [x] 3.4 `internal/auth`: user store (create, get-by-email/username/id, set status, update password) over pgx
- [x] 3.5 `internal/auth`: session store (create, get, delete, delete-by-user, purge-expired) + opaque token generation
- [x] 3.6 `internal/auth`: `Authenticator` interface (method seam for passkey/social later) with a password implementation
- [x] 3.7 HTTP handlers: `POST /api/v1/auth/signup`, `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, `GET /api/v1/auth/session` (current user)
- [x] 3.8 HTTP handlers: `POST /api/v1/auth/password/forgot`, `POST /api/v1/auth/password/reset` (single-use token, non-enumerating) + `Mailer` interface with dev logger impl
- [x] 3.9 Apply rate limiting + uniform error responses + security-event logging to all auth handlers

## 4. OIDC provider (Hydra integration)

- [x] 4.1 `internal/oidc`: `HydraClient` wrapper over Hydra admin API (get/accept/reject login, get/accept/reject consent, client CRUD)
- [x] 4.2 Login flow: `GET/POST /oauth/login` — read `login_challenge`, reuse session or render sign-in, accept/reject via Hydra with stable subject
- [x] 4.3 Consent flow: `GET/POST /oauth/consent` — read `consent_challenge`, render client + scopes, accept/reject with granted scopes + ID-token claims, support "remember"
- [x] 4.4 ID-token claims mapper (sub stable, email/email_verified/name/preferred_username per scope)
- [x] 4.5 Admin client registration: `POST/GET/DELETE /api/v1/admin/clients` (admin-key authorized) proxying Hydra; redirect URIs, scopes, grant/response types, client type
- [x] 4.6 Logout/back-channel: accept Hydra logout challenge (best-effort) and clear cotton-id session

## 5. Frontend (React + TypeScript)

- [x] 5.1 Vite + React + TS scaffold under `frontend/`; ESLint + Prettier + tsconfig strict
- [x] 5.2 Port design tokens: global CSS (`:root`/dark/light vars), glass primitives, blob background, fonts, reduce-motion handling
- [x] 5.3 Typed UI kit: `Field`, `Button`, `Toggle`, `Icon`, `Logo`, `LangSwitch`, `ThemeSwitch` from the prototype
- [x] 5.4 `i18n.ts` RU/EN dictionary (typed keys) + theme/lang context with persistence
- [x] 5.5 Typed `apiClient` (fetch with credentials, CSRF header, problem+json parsing, typed responses)
- [x] 5.6 Screens: Landing (`/`), Sign in (`/login`), Sign up (`/signup` with strength meter), Consent (`/consent`)
- [x] 5.7 Wire forms to backend: signup/login/logout; Apple button rendered disabled ("soon" stub); Google/GitHub buttons present, routing to social-start stub
- [x] 5.8 Production build served by nginx; same-origin API proxy config for dev + prod

## 6. Infrastructure & runtime

- [x] 6.1 `backend/Dockerfile` multi-stage (golang:1.25 build → distroless/alpine run), non-root
- [x] 6.2 `frontend/Dockerfile` multi-stage (node:24 build → nginx run) with SPA + API proxy config
- [x] 6.3 `deploy/docker-compose.yml`: postgres, hydra (+ hydra-migrate one-shot), backend (+ migrate), frontend, healthchecks, named volumes, networks
- [x] 6.4 Hydra config (`deploy/hydra/hydra.yml`) + DSN + system/cookie secrets via env; URLs pointing at cotton-id login/consent
- [x] 6.5 Compose env wiring + `deploy/.env.example`; optional Prometheus service + scrape config under `deploy/prometheus/`
- [x] 6.6 Seed script/migration for a demo admin user and a demo relying-party client (dev profile only)

## 7. Tests

- [x] 7.1 Unit tests: argon2id hash/verify, password policy/strength, config validation, CSRF + rate-limit middleware, claims mapper
- [x] 7.2 Integration tests (Postgres via testcontainers or compose): user store, session store, signup/login/logout/reset handlers
- [x] 7.3 Integration test: full OIDC authorization-code + PKCE flow against Hydra in compose (login → consent → token)
- [x] 7.4 Frontend tests: UI kit + form components (Vitest + Testing Library); i18n/theme toggle
- [x] 7.5 `make test` aggregates backend+frontend; coverage reported; CI workflow (build in Docker, run tests, swagger drift check)

## 8. Documentation

- [x] 8.1 Root `README.md`: what cotton-id is, quickstart (`docker compose up`), demo flow, ports
- [x] 8.2 `docs/ARCHITECTURE.md`: topology diagram, Hydra split, request/login/consent sequence, package map
- [x] 8.3 `docs/SECURITY.md`: threat model, hashing/cookie/CSRF/rate-limit decisions, secret handling, known gaps (single-factor, TOTP deferred)
- [x] 8.4 `docs/API.md` + Swagger pointer; `docs/RUNBOOK.md` (ops: migrations, health, metrics, registering a client)
- [x] 8.5 `CONTRIBUTING.md` describing the OpenSpec SDD workflow used in this repo

## 9. Verification & sign-off

- [x] 9.1 `docker compose up` smoke: stack healthy, signup→login→consent→token works end-to-end
- [x] 9.2 Run full test suite + swagger drift check; confirm `/metrics` and `/swagger/` populated
- [x] 9.3 Adversarial review (security + spec-compliance) and resolve findings
- [x] 9.4 Update task checkboxes; `openspec validate` clean
