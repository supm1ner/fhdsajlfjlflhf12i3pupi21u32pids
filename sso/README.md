# cotton-id

A **production, single-tenant OpenID Connect identity provider**. Other apps in the product
family (Vault, Mailbox, Studio, Cloud, Stream) redirect users here to sign in and receive tokens —
one account, one identity, seamless everywhere.

cotton-id deliberately **does not hand-roll OIDC protocol crypto**. It delegates the OAuth2/OIDC
engine (authorization, token issuance, JWKS, PKCE, refresh-token rotation) to
[**Ory Hydra**](https://www.ory.sh/hydra/), a certified implementation. cotton-id owns *identity* —
users, credentials, sessions, the login and consent UI, and the relying-party (OAuth client)
registry. Hydra owns the *protocol*. This split eliminates the highest-severity class of bugs while
keeping cotton-id the IdP brand and identity owner.

> **Status:** this repository implements the **walking skeleton** (OpenSpec change
> [`bootstrap-platform-and-oidc-login`](openspec/changes/bootstrap-platform-and-oidc-login/)):
> the runtime substrate plus the email/password → consent → token path, end to end, in Docker.
> Passkeys, social login, **account self-service** (see [docs/ACCOUNT.md](docs/ACCOUNT.md)), and the
> **admin console** (see [docs/ADMIN.md](docs/ADMIN.md)) have since landed; TOTP is a later change.

---

## Architecture at a glance

```
                                  ┌──────────────────────────────────────────┐
   Relying Party (RP)             │                cotton-id                  │
   e.g. Vault / Mailbox           │                                           │
        │                         │   ┌───────────────┐   ┌───────────────┐   │
        │ 1. /oauth2/auth         │   │   frontend    │   │    backend    │   │
        ▼                         │   │ React + Vite  │   │   Go 1.25     │   │
   ┌─────────────┐  login/consent │   │  (SPA, nginx) │   │   chi router  │   │
   │  Ory Hydra  │  redirects     │   │ login/consent │◄─►│ auth · oidc   │   │
   │  (OIDC eng.)│◄──────────────►│   │   /signup     │   │ adminapi      │   │
   │ tokens·JWKS │   admin API    │   └───────────────┘   └───────┬───────┘   │
   │  PKCE·refr. │◄───────────────┼───────────────────────────────┤           │
   └──────┬──────┘  accept/reject │                       ┌───────▼───────┐   │
          │ 2. code → token       │                       │  PostgreSQL   │   │
          ▼                       │   ┌───────────────┐   │ users·sessions│   │
     ID + access token to RP      │   │  Prometheus   │◄──┤ reset_tokens  │   │
                                  │   │   /metrics    │   │  (cottonid DB)│   │
                                  │   └───────────────┘   └───────────────┘   │
                                  └──────────────────────────────────────────┘

  Hydra and cotton-id share one PostgreSQL instance but use SEPARATE databases
  (cottonid for the app, hydra for Hydra's own protocol state).
```

cotton-id never sees authorization codes or tokens — Hydra mints them. cotton-id only answers
Hydra's **login challenge** ("who is this user?") and **consent challenge** ("what may this client
access?") by calling Hydra's admin API to accept or reject. See
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the full topology and sequence.

---

## Quickstart

Prerequisites: **Docker** + **Docker Compose**. (For local backend/frontend development you also
need Go 1.25 — obtained automatically via `GOTOOLCHAIN=auto` — and Node 24.)

```bash
# 1. Clone and enter the repo
git clone <repo-url> cotton-id && cd cotton-id

# 2. Create your local env file from the documented template, then edit secrets
cp deploy/.env.example deploy/.env

# 3. Bring the whole stack up
docker compose -f deploy/docker-compose.yml up --build
```

Compose starts, in dependency order: **PostgreSQL → Hydra (migrate + serve) → cotton-id backend
(migrate + serve) → frontend**. Each service has a health check; the backend retries its
dependencies with backoff and only serves HTTP once migrations have applied.

Once healthy:

| URL | What |
|---|---|
| <http://localhost:3000> | cotton-id web app (landing, login, signup, consent) |
| <http://localhost:8080/healthz> | Backend health + dependency report |
| <http://localhost:8080/swagger/> | Interactive API docs (Swagger UI) |
| <http://localhost:8080/metrics> | Prometheus metrics |
| <http://localhost:4444> | Hydra public OIDC issuer (`/.well-known/openid-configuration`) |

To stop: `docker compose -f deploy/docker-compose.yml down` (data lives in named volumes; add `-v`
to wipe it).

---

## The demo OIDC flow

With the stack up and a demo relying-party client registered (the dev profile seeds one; see
[docs/RUNBOOK.md](docs/RUNBOOK.md) to register your own), a full `authorization_code` + PKCE flow
runs like this:

1. **RP → Hydra.** The relying party redirects the browser to Hydra's authorization endpoint:
   `GET http://localhost:4444/oauth2/auth?response_type=code&client_id=...&redirect_uri=...&scope=openid%20profile%20email&state=...&code_challenge=...&code_challenge_method=S256`.
2. **Hydra → cotton-id login.** Hydra has no user store, so it 302-redirects to cotton-id's
   `/oauth/login?login_challenge=…`. If a valid cotton-id session already exists, login is accepted
   silently; otherwise the browser lands on the SPA `/login` screen.
3. **User signs in.** The SPA posts credentials to `/api/v1/auth/login`, then confirms the login
   challenge — Hydra redirects onward.
4. **Hydra → cotton-id consent.** Hydra 302s to `/oauth/consent?consent_challenge=…`. The SPA shows
   the requesting client and the scopes it wants; the user grants (optionally "remember").
5. **Hydra issues the code.** Hydra redirects back to the RP's `redirect_uri` with an authorization
   `code`, which the RP exchanges at `http://localhost:4444/oauth2/token` for an **ID token** and an
   **access token**.

The exact endpoint shapes and the login/consent handshake are pinned in
[docs/dev/build-contract.md](docs/dev/build-contract.md) §3 and walked through in
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md). A copy-pasteable `curl` walkthrough lives in
[docs/RUNBOOK.md](docs/RUNBOOK.md).

---

## Account self-service

A signed-in user manages their own identity from one place — the **Account** screen at `/account`:

- **Profile** — display name, about, location, and an avatar + banner image (uploaded as bounded,
  type-checked blobs; email change is deferred until verification ships).
- **Security** — change password (re-authenticates with the current one, then revokes other
  sessions), manage [passkeys](docs/PASSKEYS.md), and review/revoke **active sessions** with the
  current device flagged.
- **Connected apps** — see the relying parties you've granted access to (your Hydra consent grants)
  and revoke any of them.
- **Preferences** — theme, language, and a login-notification toggle, persisted server-side so they
  sync across devices.
- **Danger zone** — permanently delete the account (re-authenticated and double-confirmed), cascading
  your sessions, passkeys, social identities, and profile images.

Full surface, endpoints, security rules (re-auth, revocation, upload limits, deletion cascade), and
config are in [docs/ACCOUNT.md](docs/ACCOUNT.md).

---

## Admin console

Operators run cotton-id from the **Console** at `/admin` — a role-gated surface for managing every
account, not just one's own:

- **Overview** — aggregate stats (total accounts, active today, new this week, registered services),
  a 30-day sign-up chart, recent sign-ups, and a recent-activity feed.
- **Users** — search/filter/paginate every account, then open a user to see their profile, sessions,
  activity, and connected services, and run **lifecycle actions**: suspend/reactivate, change role,
  force a password reset, or delete.
- **Journal** — a filterable view of the **persistent, append-only audit log** (actor, action,
  target, time, IP) that every security event and admin action is written to.
- **Services / Settings** — RP (OAuth client) management lands in a later change; Settings is a
  minimal system-info surface for now.

**RBAC.** Accounts carry a ranked role — **`user` < `admin` < `owner`**. Only `admin`/`owner` reach
the console; `owner` is required for the most dangerous actions (role changes, delete). The role gate
is **enforced server-side on every endpoint** — the client-side redirect is convenience, not the
control — with escalation guards (only an owner grants admin/owner, admins can't act on owners, no
self-suspend/self-delete, the last owner is protected), and **every admin action is audited**.

**Reaching it.** Sign in normally at `/login`; if your account has the `admin`/`owner` role, `/admin`
becomes reachable (there's no separate admin login). The **dev profile seeds an operator account** —
`admin@cotton.local` / `DemoAdmin!2026` (role `admin`) — so you can open the console immediately. It
is a **dev-only** credential; in production you promote a real account to `owner` instead (and never
load the seed).

The full surface, RBAC model, lifecycle guards, and the audit log are in
[docs/ADMIN.md](docs/ADMIN.md); operator tasks (find/suspend/delete a user, read the Journal) are in
[docs/RUNBOOK.md](docs/RUNBOOK.md) §10.

---

## Observability & email

cotton-id is built to be **operated**. Beyond the always-on structured logs and the durable,
append-only **audit log** (the admin Journal, §Admin console), an optional **observability profile**
brings up **Prometheus + Grafana** with provisioned dashboards (request latency/error rate by route,
login outcomes + lockouts, social/passkey logins, sign-ups, consent grants, dependency up/down) and
Prometheus **alert rules** (elevated error rate, auth-failure spike, dependency down):

```bash
make up                 # the app stack
make observability      # adds Prometheus (:9090) + Grafana (:3001)
```

Grafana auto-provisions the datasource and dashboards; sign in as `admin` with
`GRAFANA_ADMIN_PASSWORD` (no default — set it in `deploy/.env`). The full metric catalog, dashboards,
and alerts are in [docs/OBSERVABILITY.md](docs/OBSERVABILITY.md).

**Email delivery** (password reset, admin force-reset, admin "message user", new-device
**login-notification** emails, and **signup verification codes**) goes through a configurable **SMTP**
transport (`SMTP_*`); with no SMTP configured the dev logger is used. Every send is **best-effort** —
a delivery failure never blocks the user action. Setup is in [docs/RUNBOOK.md](docs/RUNBOOK.md) §12.

> **Email verification codes** for signup are implemented server-side (in-memory, 6-digit, 10 min TTL,
> `POST /auth/signup/send-code` + `/verify-code`) but **disabled in the frontend UI for development**.
> The form creates the account immediately and skips the code step. To enable, re-insert the code
> verification step between form submit and the passkey-offer screen in `Signup.tsx`.

**Multi-replica.** The OAuth/passkey ceremony-state cookies use a per-process key by default
(single-instance). To run more than one backend replica, set a shared `OAUTH_STATE_KEY` (≥32 bytes) so
ceremonies validate across instances — see [docs/SECURITY.md](docs/SECURITY.md) §3.

**End-to-end smoke.** `make e2e` (`scripts/e2e-smoke.sh`) drives the whole IdP against a running stack —
CSRF → signup → login → reset, the OIDC RP flow (PKCE), providers, passkey begin, the account gate,
admin RBAC, and the audit Journal — printing PASS/FAIL per step (RUNBOOK §11).

---

## Ports

| Service | Container port | Host port | Notes |
|---|---|---|---|
| cotton-id backend | 8080 | 8080 | API (`/api/v1`), Hydra-facing `/oauth/*`, `/healthz`, `/metrics`, `/swagger` |
| cotton-id frontend | 80 (nginx) | 3000 | SPA; reverse-proxies `/api`, `/oauth`, `/healthz`, `/swagger` to the backend |
| Hydra — public | 4444 | 4444 | OIDC issuer: `/oauth2/auth`, `/oauth2/token`, JWKS, discovery |
| Hydra — admin | 4445 | 4445 | Admin API (internal only — login/consent accept, client CRUD) |
| PostgreSQL | 5432 | 5432 | Two databases: `cottonid` (app) and `hydra` (engine) |
| Prometheus | 9090 | 9090 | Scrapes the backend `/metrics` (optional `observability` profile) |
| Grafana | 3000 | 3001 | Dashboards over Prometheus (optional `observability` profile) |

---

## Repository layout

```
backend/      Go 1.25 service — cmd/cotton-id (composition root) + internal/ (config, database,
              httpx, auth, oidc, adminapi, observability, mailer) + migrations/ + generated docs/
frontend/     React + TypeScript (Vite) SPA — UI kit, i18n (RU/EN), theming, typed API client
deploy/       docker-compose.yml, .env.example, Hydra config, Prometheus config
docs/         ARCHITECTURE.md · SECURITY.md · API.md · RUNBOOK.md · dev/build-contract.md
openspec/     Spec-driven-development artifacts: the change proposal, design, specs, and tasks
_design_ref/  The approved visual prototype (design tokens + component reference)
```

The single source of truth for exact names, routes, schema, env vars, and ports is
[docs/dev/build-contract.md](docs/dev/build-contract.md).

---

## Documentation

| Doc | What it covers |
|---|---|
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Topology, the cotton-id/Hydra split, the login/consent sequence, the backend package map, extensibility seams |
| [docs/SECURITY.md](docs/SECURITY.md) | Threat model, security decisions (argon2id, sessions, CSRF, rate limiting, headers, secrets), and known gaps |
| [docs/API.md](docs/API.md) | Endpoint reference, how to view Swagger, example `curl` for signup/login and registering an OAuth client |
| [docs/RUNBOOK.md](docs/RUNBOOK.md) | Operations: start/stop, migrations, health/metrics, registering a relying party, rotating secrets, Hydra troubleshooting |
| [docs/SOCIAL_LOGIN.md](docs/SOCIAL_LOGIN.md) | Enabling social login: registering OAuth apps with Google/GitHub/VK/Yandex, redirect URIs, scopes, and the env vars to set |
| [docs/PASSKEYS.md](docs/PASSKEYS.md) | Passkeys (WebAuthn): the relying-party config (RP id/display-name/origins), prod setup, the lost-passkey fallback, and clone detection |
| [docs/ACCOUNT.md](docs/ACCOUNT.md) | Account self-service: the `/account` surface and its endpoints (profile, password, sessions, connected apps, preferences, deletion), the security rules, and the image-upload limits |
| [docs/ADMIN.md](docs/ADMIN.md) | Admin console: the `/admin` surface (Overview, Users, Journal), the RBAC model (`user`<`admin`<`owner`), the operator lifecycle actions and escalation guards, and the persistent audit log |
| [docs/OBSERVABILITY.md](docs/OBSERVABILITY.md) | The metrics exposed (`/metrics`), starting the observability stack (Prometheus + Grafana), the provisioned dashboards and alert rules, and the audit log |
| [CONTRIBUTING.md](CONTRIBUTING.md) | The OpenSpec spec-driven-development workflow, build/test commands, coding conventions |
| [docs/dev/build-contract.md](docs/dev/build-contract.md) | The binding interface contract — authoritative for exact shapes |

---

## How this was built

cotton-id is built **spec-first** with [OpenSpec](https://github.com/Fission-AI/OpenSpec): every
capability is proposed, designed, specified as testable `SHALL` requirements with WHEN/THEN
scenarios, and broken into tasks **before** code is written. The active change lives under
[`openspec/changes/bootstrap-platform-and-oidc-login/`](openspec/changes/bootstrap-platform-and-oidc-login/).
See [CONTRIBUTING.md](CONTRIBUTING.md) to learn the workflow before opening a change.

## Tech stack

Go 1.25 · chi · pgx · PostgreSQL 16 · Ory Hydra v2 · argon2id · React + TypeScript · Vite · nginx ·
Prometheus · Docker Compose.
