## Why

cotton-id is a production, single-tenant **OpenID Connect identity provider**: other apps in the product family (Vault, Mailbox, Studio, Cloud, Stream) redirect users here to sign in and receive tokens. Before any of the richer surfaces from the design prototype (passkeys, social login, account self-service, the admin console) can be built, the system needs a **walking skeleton** that proves the hardest, most security-critical path end-to-end: a relying party performs an OAuth2/OIDC authorization-code flow, the user authenticates with email + password, grants consent, and receives valid tokens — all running in Docker, observable, documented, and tested.

We deliberately delegate the OIDC protocol engine (token issuance, JWKS, PKCE, refresh rotation) to **Ory Hydra**, a certified implementation, so cotton-id never hand-rolls protocol crypto. cotton-id owns identity (users, login UI, consent UI, client registry) and Hydra owns the protocol. This first change establishes that split and the project substrate every later change extends.

## What Changes

- Introduce the cotton-id monorepo layout: Go 1.25 backend (`cmd/`, `internal/`), React + TypeScript frontend (Vite), and infrastructure (Docker Compose, migrations, Hydra config).
- Stand up the runtime substrate: typed configuration, PostgreSQL connection pool, versioned SQL migrations, an HTTP server with security middleware (security headers, CSRF protection on browser-posted forms, per-route rate limiting, CORS), structured JSON logging, a Prometheus `/metrics` endpoint, and an auto-generated OpenAPI/Swagger UI for every endpoint.
- Implement **email/password authentication**: account creation (signup), login, logout, password reset, argon2id password hashing, and server-side sessions backed by Secure/HttpOnly/SameSite cookies.
- Integrate **Ory Hydra** as the OIDC engine: implement the login-challenge and consent-challenge handlers Hydra delegates to, OAuth client (relying-party) registration via Hydra's admin API, and a working authorization-code + PKCE flow that issues ID/access tokens.
- Build the React frontend for this slice: the landing page, sign-in form, sign-up form, and consent screen, faithful to the prototype's glass/hue design, with RU/EN i18n and dark/light theming, talking to the backend over a typed API client.
- Package everything to run with a single `docker compose up`: PostgreSQL + Hydra + backend + frontend, with health checks and seedable demo data.
- Cover the slice with automated tests (Go unit + integration against a real Postgres/Hydra, frontend component tests) and ship developer + operator documentation.

This change is **additive only** — it is the first change in the repository; no existing capabilities are modified or removed.

## Capabilities

### New Capabilities

- `platform-foundation`: The runtime substrate — project layout, typed config, PostgreSQL access + migrations, the HTTP server and its security middleware, structured logging, Prometheus metrics, OpenAPI/Swagger documentation, the Docker Compose topology, health checks, and the web application shell (SPA delivery, RU/EN i18n, dark/light theming).
- `password-authentication`: User accounts and the email/password credential — signup, login, logout, password reset, argon2id hashing with strength enforcement, and cookie-backed server-side sessions.
- `oidc-provider`: cotton-id as an OpenID Connect provider via Ory Hydra — the login and consent challenge handlers, OAuth client (relying-party) registration, the authorization-code + PKCE flow, and the claims contained in issued tokens.

### Modified Capabilities

<!-- None. This is the first change; there are no existing specs to modify. -->

## Impact

- **New systems / dependencies**: PostgreSQL (data store), Ory Hydra (OIDC engine, run as a container), Go modules (chi router, pgx, argon2, prometheus client, swag/swaggo or oapi-codegen), Node/Vite/React/TypeScript toolchain.
- **New code**: entire `backend/`, `frontend/`, and `deploy/` trees plus `openspec/` artifacts; no existing code is touched.
- **APIs**: introduces the public auth API (`/api/v1/auth/*`), the Hydra-facing login/consent flow endpoints, the admin client-registration API, the OpenAPI document + Swagger UI, and `/metrics` + `/healthz`.
- **Security surface**: this change owns the credential-handling and token-issuance paths, so it sets the security baseline (hashing parameters, cookie attributes, CSRF, rate limiting, secret handling) that every later change inherits.
- **Operations**: introduces the `docker compose` runtime, migration workflow, and observability endpoints operators will depend on.
