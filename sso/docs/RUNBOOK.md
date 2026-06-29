# Runbook

Operational guide for running cotton-id: starting and stopping the stack, migrations, health and
metrics, registering relying parties, rotating secrets, and troubleshooting Hydra.

Commands assume you run Compose from the repo root with the deploy file:
`docker compose -f deploy/docker-compose.yml …`. The exact env-var names and ports referenced here
are defined in [docs/dev/build-contract.md](dev/build-contract.md) §5–§6.

---

## 1. Start / stop

### First run

```bash
cp deploy/.env.example deploy/.env          # then edit secrets (see §5)
docker compose -f deploy/docker-compose.yml up --build
```

Startup order (enforced by health checks and the backend's dependency retry/backoff):
**PostgreSQL → Hydra migrate (one-shot) → Hydra serve → cotton-id migrate → backend serve →
frontend.** The backend only binds its HTTP port after migrations have applied and dependencies are
reachable.

### Day-to-day

```bash
# Start in the background
docker compose -f deploy/docker-compose.yml up -d

# Tail logs (all services, or one)
docker compose -f deploy/docker-compose.yml logs -f
docker compose -f deploy/docker-compose.yml logs -f backend

# Stop (keep data in named volumes)
docker compose -f deploy/docker-compose.yml down

# Stop AND wipe data volumes (destructive — clears users, sessions, Hydra state)
docker compose -f deploy/docker-compose.yml down -v

# Restart a single service after a config change
docker compose -f deploy/docker-compose.yml restart backend
```

### Verify it came up

```bash
curl -s http://localhost:8080/healthz                         # backend + dependency report
curl -s http://localhost:4444/.well-known/openid-configuration # Hydra discovery
open http://localhost:3000                                     # the web app
open http://localhost:8080/swagger/                            # API docs
```

---

## 2. Migrations

cotton-id uses **forward-only versioned SQL migrations** (`.up.sql` / `.down.sql`) in
`backend/migrations/`, embedded via `embed.FS` and applied by the backend's own migrator on startup
(design D10). Hydra runs its **own** migrations against its **separate** `hydra` database via
`hydra migrate sql`.

- **Normal operation:** migrations run automatically before the backend serves traffic. Applying to
  an already-current database is idempotent — nothing is re-applied (platform-foundation spec).
- **Fresh database:** all migrations apply in order to bring the schema to the current version.

Inspect or operate migrations:

```bash
# What migration files exist
ls backend/migrations/

# Connect to the app database to inspect schema / migration bookkeeping
docker compose -f deploy/docker-compose.yml exec postgres \
  psql -U cotton -d cottonid -c '\dt'

# Re-run the Hydra SQL migration explicitly (if the one-shot needs re-running)
docker compose -f deploy/docker-compose.yml run --rm hydra-migrate
```

> `down` migrations exist for **local development only**. Production is forward-only; never roll a
> schema back in production — roll forward with a new migration.

**Adding a migration** (see [CONTRIBUTING.md](../CONTRIBUTING.md)): add the next-numbered
`NNNN_name.up.sql` and `NNNN_name.down.sql` pair; it applies on the next backend start.

---

## 3. Health & metrics

### Health

`GET /healthz` reports the backend's ability to reach its critical dependencies (Postgres, Hydra):

```bash
curl -i http://localhost:8080/healthz
# 200 + body when healthy; non-200 (503) when a dependency is down
```

Compose uses this as the backend's health check. If `/healthz` is non-200, check the dependency it
flags before restarting the backend itself (§6).

### Metrics

`GET /metrics` exposes Prometheus text exposition — Go runtime metrics plus app metrics including
`http_request_duration_seconds` (labeled by route, method, and status), login success/failure/locked
(`cotton_login_attempts_total`), social and passkey login outcomes (`cotton_social_login_total`,
`cotton_passkey_login_total`), signups, consent decisions, and account lockouts
(`cotton_account_lockouts_total`).

```bash
curl -s http://localhost:8080/metrics | grep http_request_duration_seconds
```

The optional **observability profile** (Prometheus + Grafana) scrapes this and ships provisioned
dashboards and alert rules — see **§11** below and **[OBSERVABILITY.md](OBSERVABILITY.md)** for the full
metric catalog, dashboards, and alerts. Scrape config lives in `deploy/prometheus/prometheus.yml`, alert
rules in `deploy/prometheus/alerts.yml`, and Grafana provisioning under `deploy/grafana/`.

### Logs / audit

Logs are structured JSON to stdout, each line carrying the request id. Every security-relevant event
(login ok/fail, signup, password reset, consent decision, client registration, throttling) is logged
with structured fields and **no secrets**. To find security events:

```bash
docker compose -f deploy/docker-compose.yml logs backend | grep -i '"event"'
```

In addition to stdout, those same security events **and every admin-console action** are appended to
a **persistent, append-only audit log** (the `audit_log` table) that survives container restarts. It
backs the admin console's **Journal** tab and is queryable over the API and directly in SQL — see §10
("Read the Journal"). The stdout log is ephemeral and best for live tailing; the audit log is the
durable trail.

---

## 4. Registering a relying party (OAuth client)

A **relying party** is an OAuth client registered in Hydra — the apps that authenticate their users
through cotton-id. There are **two ways** to manage them, against the **same** registry:

- **The admin console (humans).** Operators register/edit/delete clients and see consent usage from
  the **Services** tab at `/admin` — **session + role-gated** (`/api/v1/admin/services`). This is the
  day-to-day path; see [§4a](#4a-from-the-admin-console-services-tab) below and **[ADMIN.md](ADMIN.md)**.
- **The machine API (automation).** Unattended scripts, CI, and the dev seed register clients with the
  **`X-Admin-Key`** header (`/api/v1/admin/clients`); see [§4b](#4b-from-the-machine-api-automation).

Both call the same Hydra admin API through the one `oidc.HydraClient`, so a client created either way
is visible and editable from the other. They differ only in **auth** (a signed-in admin session vs the
admin key) and **path** (`/services` vs `/clients`) — the key does **not** grant console access and the
console session does **not** work on the key route. Use the console for interactive operator work; use
the key for automation.

### 4a. From the admin console (Services tab)

Sign in at `/login` as an `admin`/`owner`, open `/admin`, and go to **Services**:

- **Register** — fill the create form (name, type **public**/**confidential**, redirect URIs, scopes,
  grant/response types) and submit. For a **confidential** client the generated **`clientSecret` is
  shown once** in a copy panel — **store it immediately**, it is never re-served. **Public** (SPA /
  native) clients have **no secret** and use PKCE.
- **Edit** — change a client's name, redirect URIs, scopes, or grant/response types. Redirect URIs must
  be absolute `http(s)` URLs without a fragment (same validation as the machine route).
- **Delete** — confirm to remove the client; afterwards any authorization request with that `client_id`
  is rejected by Hydra.
- **Consent usage / revoke** — see each client's (best-effort) active-grant count, and **revoke all**
  of a client's grants so its users must consent again on next sign-in.

Every console mutation is **audited** to the Journal with the operator as the actor (see §10 and
[ADMIN.md](ADMIN.md)). You can drive the same routes over the API with the operator's session cookie
(`/api/v1/admin/services…`, CSRF token on mutations) — the shapes are in [Swagger](API.md).

### 4b. From the machine API (automation)

For unattended use, register through cotton-id's `X-Admin-Key` admin endpoint, which proxies Hydra's
admin API. These calls require the `X-Admin-Key` header (`ADMIN_API_KEY`) and are CSRF-exempt.

```bash
ADMIN_API_KEY=$(grep '^ADMIN_API_KEY=' deploy/.env | cut -d= -f2)

# Register a public client (browser/SPA — uses PKCE, no secret)
curl -s -X POST http://localhost:8080/api/v1/admin/clients \
  -H "Content-Type: application/json" -H "X-Admin-Key: $ADMIN_API_KEY" \
  -d '{
        "name": "Vault Web",
        "redirectUris": ["https://vault.example.com/callback"],
        "scopes": ["openid","profile","email"],
        "grantTypes": ["authorization_code","refresh_token"],
        "responseTypes": ["code"],
        "clientType": "public"
      }'
# → 201 { "clientId": "..." }

# List clients
curl -s -H "X-Admin-Key: $ADMIN_API_KEY" http://localhost:8080/api/v1/admin/clients

# Remove a client (subsequent auth requests with that client_id are rejected)
curl -s -X DELETE -H "X-Admin-Key: $ADMIN_API_KEY" \
  http://localhost:8080/api/v1/admin/clients/<clientId>
```

For a **confidential** client (server-side app that can keep a secret), set
`"clientType": "confidential"`; the `201` response includes a one-time `clientSecret` — **store it
immediately**, it cannot be retrieved again.

The **dev profile seeds** a demo relying-party client and a demo admin user so you can run the flow
immediately without registering anything. Do **not** load the dev seed in production.

Smoke-test the full flow (see [API.md](API.md) §6 and [ARCHITECTURE.md](ARCHITECTURE.md) §3): point
an OIDC client at `http://localhost:4444/oauth2/auth?response_type=code&client_id=…&…&code_challenge=…`,
sign in, consent, and exchange the returned `code` at `http://localhost:4444/oauth2/token`.

---

## 5. Rotating secrets

All secrets are injected via env / compose and validated at startup; outside `development`,
known-insecure defaults are **refused** (the process exits non-zero before binding the port). The
secrets that matter:

| Secret (env var) | Used for | Rotation effect |
|---|---|---|
| `ADMIN_API_KEY` | Authorizes `/api/v1/admin/*` | Old key stops working immediately; update admin callers. |
| Hydra **system secret** | Encrypts Hydra's data at rest | Rotate via Hydra's documented key-rotation (supports old+new during transition). A naive swap can invalidate existing grants/tokens. |
| Hydra **cookie secret** | Hydra's CSRF/login-session cookies | Invalidates in-flight login/consent flows; users restart the flow. |
| Session / CSRF keys | cotton-id cookie integrity | Existing `cid_session` / `cid_csrf` cookies become invalid; users re-authenticate. |
| `DATABASE_URL` password | Postgres auth | Change in Postgres and in env together, then restart the backend. |

General procedure:

```bash
# 1. Generate a strong value (32+ bytes)
openssl rand -base64 32

# 2. Update deploy/.env (never commit it — .env is gitignored)

# 3. Recreate the affected service so it picks up the new env
docker compose -f deploy/docker-compose.yml up -d --force-recreate backend
```

Notes:
- Rotating cotton-id session/CSRF keys logs everyone out (cookies no longer validate) — expected.
- Rotating Hydra's **system secret** in production requires Hydra's **rotation** mechanism (keep the
  old secret listed alongside the new one until re-encryption completes) — do **not** simply replace
  it, or you risk locking out existing encrypted state. Consult the Hydra docs for the running
  version (`oryd/hydra:v2.2.0`).
- For production, secrets should ultimately come from a manager (Vault/KMS), not `.env` — that
  integration is a documented later concern (see [SECURITY.md](SECURITY.md) §3).

---

## 6. Troubleshooting Hydra

Hydra is the OIDC engine; most "OIDC isn't working" issues are configuration or connectivity between
Hydra and cotton-id. Encapsulation note: all Hydra calls go through one `oidc.HydraClient`, and the
login/consent contract is fixed (build-contract §3) — so symptoms usually point to one of these:

| Symptom | Likely cause & fix |
|---|---|
| Authorization request → blank page or 500 at `/oauth/login` | Backend can't reach Hydra **admin** API. Check `HYDRA_ADMIN_URL` (`http://hydra:4445` inside compose) and `docker compose logs hydra`. |
| Browser loops between login and Hydra | Login challenge isn't being accepted (session not established, or `login_challenge` lost). Verify `POST /api/v1/auth/login` set `cid_session`, then `POST /api/v1/oauth/login/accept` succeeded. |
| `redirect_uri` mismatch error from Hydra | The RP's `redirect_uri` is not in the registered client's `redirectUris`. Re-register or update the client (§4). |
| `invalid_client` | Wrong `client_id`, or the client was deleted. List clients (§4). |
| Public client rejected | Missing PKCE `code_challenge` — public clients **must** use PKCE (Hydra enforces this). |
| Consent never remembers | "remember" not sent or scopes differ. Remembered consent only skips for same-or-narrower scopes. |
| `hydra-migrate` fails on startup | Hydra's `hydra` database isn't ready or DSN is wrong. Confirm Postgres is healthy and Hydra's DSN points at the **`hydra`** database (separate from `cottonid`). |
| Tokens won't validate at the RP | RP is using the wrong issuer/JWKS. Use Hydra's discovery: `http://localhost:4444/.well-known/openid-configuration`. |

Useful checks:

```bash
# Is Hydra healthy?
curl -s http://localhost:4444/health/ready
curl -s http://localhost:4445/health/ready          # admin (internal)

# Hydra's view of the world
curl -s http://localhost:4444/.well-known/openid-configuration | head

# Hydra logs
docker compose -f deploy/docker-compose.yml logs -f hydra

# Confirm the two databases exist and are separate
docker compose -f deploy/docker-compose.yml exec postgres psql -U cotton -c '\l'
```

Hydra config lives in `deploy/hydra/hydra.yml`; its login/consent URLs must point at cotton-id's
`/oauth/login` and `/oauth/consent`, and its public URL must match `HYDRA_PUBLIC_URL` so issued
tokens carry the correct issuer.

---

## 7. Common operational tasks (cheat sheet)

```bash
# Open a psql shell on the app database
docker compose -f deploy/docker-compose.yml exec postgres psql -U cotton -d cottonid

# Count users / active sessions
docker compose -f deploy/docker-compose.yml exec postgres \
  psql -U cotton -d cottonid -c 'SELECT count(*) FROM users;'
docker compose -f deploy/docker-compose.yml exec postgres \
  psql -U cotton -d cottonid -c 'SELECT count(*) FROM sessions WHERE expires_at > now();'

# Purge expired sessions manually (the app also purges on a schedule)
docker compose -f deploy/docker-compose.yml exec postgres \
  psql -U cotton -d cottonid -c 'DELETE FROM sessions WHERE expires_at < now();'

# Rebuild after a code change
docker compose -f deploy/docker-compose.yml up -d --build backend
```

---

## 8. Enabling social login

Users can optionally sign in with **Google, GitHub, VK, or Yandex** (Apple is not supported). Each
provider is independently configurable and **disabled until you set its credentials** — an
unconfigured provider's button is hidden and its endpoints return "provider not enabled".

To enable a provider you register an OAuth app with it and set two env vars
(`SOCIAL_<PROVIDER>_CLIENT_ID` / `SOCIAL_<PROVIDER>_CLIENT_SECRET`) in `deploy/.env`. The redirect
URI to register is `PUBLIC_BASE_URL` + `/api/v1/auth/social/<provider>/callback`.

Full step-by-step instructions for each provider — redirect URIs, scopes, and the exact env vars —
are in **[docs/SOCIAL_LOGIN.md](SOCIAL_LOGIN.md)**.

```bash
# After setting a provider's two vars in deploy/.env, recreate the backend:
docker compose -f deploy/docker-compose.yml up -d --force-recreate backend

# Confirm which providers are enabled (only configured ones are returned):
curl -s http://localhost:8080/api/v1/auth/social/providers
```

---

## 9. Account self-service (operations)

Signed-in users manage their own account from the SPA `/account` screen — profile, password, active
sessions, connected apps (consent grants), preferences, and account deletion. The full surface,
endpoints, and security rules are in **[docs/ACCOUNT.md](ACCOUNT.md)**; this section is the
operator's view. All account endpoints require a valid `cid_session` cookie (so the live smoke below
signs in first); state-changing ones also require the CSRF token.

### Profile-image upload limits

Avatars and banners are stored as **bounded blobs in Postgres** (no object store). The hard size caps
are env-tunable (non-secret, safe defaults):

| Env var | Caps | Default |
|---|---|---|
| `ACCOUNT_AVATAR_MAX_KB` | Max avatar upload (KB) | `512` |
| `ACCOUNT_BANNER_MAX_KB` | Max banner upload (KB) | `1024` |

Uploads are also restricted to `image/png`/`jpeg`/`webp` by content-type **and** magic bytes (a code
constant, not env). To change a cap, edit `deploy/.env` and recreate the backend:

```bash
docker compose -f deploy/docker-compose.yml up -d --force-recreate backend
```

Raise caps cautiously — bytes are stored in Postgres and read into memory on upload.

### Live smoke (sign in → manage → delete a test account)

Run against a local stack with a cookie jar (re-use the `$CSRF` capture from [API.md](API.md) §1).
This exercises the surface end-to-end on a **throwaway** account:

```bash
# 0. CSRF + a test account (signup logs you in: cid_session is set)
curl -s -c cookies.txt http://localhost:8080/api/v1/csrf
CSRF=$(curl -s -c cookies.txt http://localhost:8080/api/v1/csrf | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
curl -s -b cookies.txt -c cookies.txt -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" \
  -d '{"displayName":"Test User","username":"testuser","email":"test@example.com","password":"correct horse battery staple"}'

# 1. Read the full profile (prefs + counts)
curl -s -b cookies.txt http://localhost:8080/api/v1/account

# 2. Edit profile fields
curl -s -b cookies.txt -X PATCH http://localhost:8080/api/v1/account \
  -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" \
  -d '{"displayName":"Test User 2","about":"hello","location":"Almaty"}'

# 3. Change password (re-auths with current; revokes OTHER sessions)
curl -s -b cookies.txt -X PUT http://localhost:8080/api/v1/account/password \
  -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" \
  -d '{"currentPassword":"correct horse battery staple","newPassword":"a different strong passphrase"}'

# 4. List active sessions (current one flagged) and connected apps
curl -s -b cookies.txt http://localhost:8080/api/v1/account/sessions
curl -s -b cookies.txt http://localhost:8080/api/v1/account/connections

# 5. Delete the test account (re-auth required) — IRREVERSIBLE, cascades everything
curl -s -b cookies.txt -X DELETE http://localhost:8080/api/v1/account \
  -H "Content-Type: application/json" -H "X-CSRF-Token: $CSRF" \
  -d '{"currentPassword":"a different strong passphrase"}'
# → account + its sessions/passkeys/social identities/profile images removed;
#   the subject's Hydra login/consent sessions best-effort revoked.
```

### Account deletion (what it removes)

`DELETE /api/v1/account` is **destructive and irreversible**. It requires re-authentication (the
current password, or a typed confirmation for a social-only account) and the UI double-confirms. On
success it:

1. Deletes the `users` row; FK `ON DELETE CASCADE` removes that user's **sessions**, **passkeys**
   (`webauthn_credentials`), **social identities**, and **profile images** in the same transaction.
2. **Best-effort** revokes the subject's Hydra login + consent sessions (a Hydra hiccup does not
   block the local delete — the app DB is the source of truth for account existence).
3. Logs a structured **security event** (no secrets) for the audit trail.

Inspect account state operationally (e.g. confirm a delete cascaded):

```bash
# A user's sessions, passkeys, social identities, and profile images by email
docker compose -f deploy/docker-compose.yml exec postgres psql -U cotton -d cottonid -c \
  "SELECT u.email,
          (SELECT count(*) FROM sessions s WHERE s.user_id=u.id)            AS sessions,
          (SELECT count(*) FROM webauthn_credentials w WHERE w.user_id=u.id) AS passkeys,
          (SELECT count(*) FROM social_identities si WHERE si.user_id=u.id)  AS socials,
          (SELECT count(*) FROM profile_images pi WHERE pi.user_id=u.id)     AS images
   FROM users u WHERE u.email='test@example.com';"
# After a delete, the user row is gone and every dependent count is 0 (cascade).
```

Find account-management security events in the structured log:

```bash
docker compose -f deploy/docker-compose.yml logs backend | grep -iE '"event".*(password_change|session_revoke|consent_revoke|account_delete)'
```

---

## 10. Admin console (operations)

The **admin console** is the operator surface at the SPA route `/admin` — manage every account,
review aggregate stats, and read the audit log (Journal). It is **role-gated**: only accounts with
the `admin` or `owner` role can reach it; `owner` is required for the dangerous actions (role
changes, delete). The full surface, RBAC model, and security rules are in **[ADMIN.md](ADMIN.md)**
and **[SECURITY.md](SECURITY.md) §2.11**; this section is the operator's view.

There is **no separate admin login** — an operator signs in normally at `/login`, and if their
account carries the `admin`/`owner` role the `/admin` surface becomes reachable. To grant or revoke
access, change a role (below) or suspend the account. All admin endpoints require the operator's
`cid_session` cookie (so the API examples below sign in first); state-changing ones also require the
CSRF token. A non-admin caller gets `403`; an unauthenticated one gets `401`.

> The console reuses the operator's browser session. The machine `X-Admin-Key` (§4b) is a **separate**
> mechanism for unattended OAuth-client registration — it does **not** grant console access, and the
> console role gate does **not** replace it. Operators manage relying-party clients from the console's
> **Services** tab (§4a, role-gated `/api/v1/admin/services`); the key route is for automation.

### Create the first operator (production)

The **dev profile seeds** an operator account so a developer reaches `/admin` immediately:

| Field | Value |
|---|---|
| Email | `admin@cotton.local` |
| Username | `admin` |
| Password | `DemoAdmin!2026` |
| Role | `admin` |

This is a **well-known dev credential** and **must never be loaded in production** (the seed one-shot
runs only under the `dev` Compose profile). In production you create the **first operator yourself**
by promoting a real, signed-up account to `owner` directly in the database (there is no bootstrap
endpoint — by design, role changes require an existing owner):

```bash
# Promote an existing account to owner (run once, after that account has signed up)
docker compose -f deploy/docker-compose.yml exec postgres psql -U cotton -d cottonid -c \
  "UPDATE users SET role='owner', updated_at=now() WHERE email='you@example.com';"
```

That `owner` then manages every other role from the console — you should not need raw SQL again.

### Find a user

In the console, the **Users** tab searches across username, display name, and email and filters by
status/role. Operationally you can do the same in SQL:

```bash
# Find an account by email / username / display name (case-insensitive)
docker compose -f deploy/docker-compose.yml exec postgres psql -U cotton -d cottonid -c \
  "SELECT id, email, username, display_name, status, role, created_at
   FROM users
   WHERE email ILIKE '%alex%' OR username ILIKE '%alex%' OR display_name ILIKE '%alex%'
   ORDER BY created_at DESC LIMIT 20;"
```

### Suspend / reactivate a user

Suspending sets `status='suspended'` **and revokes the user's sessions** — they are signed out
everywhere and can no longer sign in. From the console it is the **Suspend** button on the user's
detail card (`POST /api/v1/admin/users/{id}/suspend`); **Reactivate** restores `status='active'`
(`POST …/reactivate`). Guards: you cannot suspend yourself, and only an `owner` may act on an
`owner`. Confirm operationally:

```bash
# Confirm a suspend took effect and revoked sessions
docker compose -f deploy/docker-compose.yml exec postgres psql -U cotton -d cottonid -c \
  "SELECT u.status, (SELECT count(*) FROM sessions s WHERE s.user_id=u.id) AS sessions
   FROM users u WHERE u.email='someone@example.com';"
# → status 'suspended' and sessions 0 after a suspend
```

### Change a role

Role changes are **owner-only** and escalation-guarded — only an `owner` may grant/revoke
`admin`/`owner`, the **last owner** cannot be demoted, and no one can escalate their **own** role.
From the console this is the role control on the user's detail card
(`PATCH /api/v1/admin/users/{id}/role`). Day-to-day, do this from the console; the raw-SQL promotion
above is only the production bootstrap for the **first** owner.

### Force a password reset

The **Reset password** action (`POST /api/v1/admin/users/{id}/reset-password`) issues a single-use,
time-limited reset token for the target (the same mechanism as self-service forgot-password) and
delivers it via the mailer. With **SMTP configured** (§12) the link is **emailed** to the user; with no
SMTP set the dev `LogMailer` is used and the token is **logged** rather than emailed (see
[SECURITY.md](SECURITY.md) §3). When the dev mailer is active, find the issued reset token in the logs:

```bash
docker compose -f deploy/docker-compose.yml logs backend | grep -i 'reset'
```

### Delete a user

Deletion is **owner-only**, destructive, and irreversible (`DELETE /api/v1/admin/users/{id}`). It
removes the `users` row; FK `ON DELETE CASCADE` removes the account's sessions, passkeys, social
identities, and profile images, then best-effort revokes the subject's Hydra sessions. Guards: you
cannot delete yourself, and the **last owner** cannot be deleted. (This is the same cascade as
account self-service deletion — [ACCOUNT.md](ACCOUNT.md) — but performed by an operator.) Confirm a
delete cascaded with the per-user inventory query in §9.

### Read the Journal (audit log)

Every security event and admin action is appended to the **persistent `audit_log` table**. In the
console, the **Journal** tab shows it with filters (action, actor, time range) and pagination, newest
first; the same data is at `GET /api/v1/admin/audit`. Operationally you can read it directly in SQL:

```bash
# Latest 50 audit entries (who did what, to whom, from where, when)
docker compose -f deploy/docker-compose.yml exec postgres psql -U cotton -d cottonid -c \
  "SELECT ts, actor_label, action, target_type, target_id, ip
   FROM audit_log ORDER BY ts DESC LIMIT 50;"

# Just the admin lifecycle actions (suspend/reactivate/role/reset/delete)
docker compose -f deploy/docker-compose.yml exec postgres psql -U cotton -d cottonid -c \
  "SELECT ts, actor_label, action, target_id, ip
   FROM audit_log WHERE action LIKE 'user.%' ORDER BY ts DESC LIMIT 50;"

# Everything a specific operator did
docker compose -f deploy/docker-compose.yml exec postgres psql -U cotton -d cottonid -c \
  "SELECT ts, action, target_type, target_id FROM audit_log
   WHERE actor_id = (SELECT id FROM users WHERE email='operator@example.com')
   ORDER BY ts DESC LIMIT 50;"
```

The audit log is **append-only** — there is no operator path (API or intended SQL) to edit or delete
entries; it is the durable trail that outlives the stdout logs (§3).

### Live smoke (reach the console, gate a non-admin, suspend a test user)

```bash
# 0. CSRF + sign in as the seeded dev operator (dev profile only)
curl -s -c admin.txt http://localhost:8080/api/v1/csrf
ACSRF=$(curl -s -c admin.txt http://localhost:8080/api/v1/csrf | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
curl -s -b admin.txt -c admin.txt -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" -H "X-CSRF-Token: $ACSRF" \
  -d '{"email":"admin@cotton.local","password":"DemoAdmin!2026"}'

# 1. The operator reaches the admin API (overview + user list)
curl -s -b admin.txt http://localhost:8080/api/v1/admin/overview
curl -s -b admin.txt "http://localhost:8080/api/v1/admin/users?query=test&page=1&pageSize=20"

# 2. A non-admin is refused (sign in as an ordinary user, then call an admin endpoint → 403)
#    (use any non-admin account's cookie jar)
# curl -s -o /dev/null -w '%{http_code}\n' -b user.txt http://localhost:8080/api/v1/admin/overview   # → 403

# 3. Suspend a test user (sessions get revoked) and see it in the Journal
TID=<test-user-id>
curl -s -b admin.txt -X POST "http://localhost:8080/api/v1/admin/users/$TID/suspend" \
  -H "X-CSRF-Token: $ACSRF"
curl -s -b admin.txt "http://localhost:8080/api/v1/admin/audit?action=user.suspended&page=1&pageSize=10"
```

---

## 11. Observability stack & the e2e smoke

### Start Prometheus + Grafana (observability profile)

The metrics scraper and dashboards live behind the `observability` Compose profile so the
default stack stays lean. Bring them up alongside a running stack:

```bash
make up                 # the app stack
make observability      # adds Prometheus + Grafana (profile: observability)

# …or directly:
docker compose -f deploy/docker-compose.yml --profile observability up -d
```

| Service | URL | Notes |
|---|---|---|
| Prometheus | <http://localhost:9090> | Scrapes `backend:8080/metrics`; evaluates the alert rules. |
| Grafana | <http://localhost:3001> | Sign in as `admin` / `GRAFANA_ADMIN_PASSWORD`; datasource + dashboards auto-provisioned. |

**`GRAFANA_ADMIN_PASSWORD` is required** — Grafana has no default admin password. Set it in
`deploy/.env` before starting the profile (and before exposing Grafana anywhere reachable).

### Dashboards & alerts

Grafana auto-provisions the Prometheus datasource and the dashboards (HTTP latency/error rate by
route, login outcomes + lockouts, social/passkey logins, sign-ups, consent grants, dependency up/down)
from `deploy/grafana/`. Prometheus loads the alert rules from `deploy/prometheus/alerts.yml`:

- **HighHTTP5xxRate** (critical) — 5xx ratio over the API > 5% for 5m.
- **AuthFailureSpike** (warning) — > 50% of login attempts fail/lock for 10m (brute-force signal).
- **LoginLockoutSurge** (warning) — sustained account lockouts (> 0.2/s for 10m).
- **TargetDown** (critical) — the backend scrape target is `up == 0` for 2m.

Review rules and firing alerts in Prometheus → **Status → Rules** / **Alerts**
(<http://localhost:9090/alerts>). After editing `alerts.yml`, reload without a restart:

```bash
curl -X POST http://localhost:9090/-/reload
```

The shipped stack evaluates alerts in Prometheus but ships **no Alertmanager** — wire one (and its
receivers) via Prometheus's `alerting:` block to actually deliver pages. Full metric catalog and
dashboard/alert detail are in **[OBSERVABILITY.md](OBSERVABILITY.md)**.

### End-to-end smoke (`make e2e`)

`scripts/e2e-smoke.sh` exercises every top-level capability against a **running** stack over HTTP with
curl and a cookie jar — no build, no jq. It is the production-readiness gate; run it after `make up`:

```bash
make up        # bring the stack up (incl. the dev seed — the smoke uses the seed admin)
make e2e       # or: sh scripts/e2e-smoke.sh
```

It prints `PASS`/`FAIL` per step and **exits non-zero** if any step fails. The steps:

1. `/healthz` reachable (aborts early if the backend is down).
2. **CSRF** — `GET /api/v1/csrf` issues a token + sets `cid_csrf`.
3. **Signup** → `201` + `cid_session`.
4. **Session / logout / login** — `GET /auth/session` 200, logout 204, login 200 (password round-trip).
5. **Password forgot** → `202`, and the **same** `202` for an unknown email (non-enumerating).
6. **Social providers** — `GET /auth/social/providers` 200 (empty array is valid with no creds).
7. **Passkey login begin** — `POST /auth/passkey/login/begin` 200 with request options + `cid_wa`.
8. **OIDC RP flow** — register a public client via the **machine API** (`X-Admin-Key`), then drive an
   `authorization_code` + **PKCE** `/oauth2/auth` and assert the **302 chain** reaches the cotton-id
   login (`FRONTEND_BASE_URL/login?login_challenge=…`); the throwaway client is deleted afterward.
9. **Account gate** — `GET /account` 401 without a session, 200 with one (and `/account/sessions` 200).
10. **Admin RBAC** — the seed admin reaches `/admin/overview` (200); the ordinary user gets 403 and an
    anonymous caller gets 401.
11. **Audit Journal** — suspend the smoke user, then assert `user.suspended` appears in `/admin/audit`.

Override the endpoints / credentials via env when the stack is not on the defaults:

```bash
BASE_URL=http://localhost:8080 \
HYDRA_PUBLIC_URL=http://localhost:4444 \
FRONTEND_BASE_URL=http://localhost:3000 \
ADMIN_API_KEY=... \
ADMIN_EMAIL=admin@cotton.local ADMIN_PASSWORD='DemoAdmin!2026' \
sh scripts/e2e-smoke.sh
```

`ADMIN_API_KEY` defaults to the value in `deploy/.env` when unset. The smoke targets a **dev** stack
(`COTTON_ENV=development`, `COOKIE_SECURE=false`, the dev seed loaded — it signs in as the seed
operator and the OIDC step relies on the `http` issuer).

---

## 12. Email delivery (SMTP)

cotton-id sends transactional email — **password-reset** links, the admin **force-reset** link, the
admin **"message user"** action, and **login-notification** emails — through a configurable SMTP
transport. With **no SMTP configured** the dev `LogMailer` is used: messages are written to the
structured log instead of sent (handy for local dev; treat reset-token log lines as sensitive). All
sends are **best-effort** — a delivery failure is logged and the originating action still succeeds, so
email never blocks a login, signup, reset, or admin operation.

### Configure SMTP

Set these in `deploy/.env` (then recreate the backend). SMTP is **enabled when `SMTP_HOST` is set**;
leaving it empty keeps the dev logger.

| Env var | Example | Notes |
|---|---|---|
| `SMTP_HOST` | `smtp.example.com` | SMTP server host. Empty = dev `LogMailer` (no real email). |
| `SMTP_PORT` | `587` | Submission port (587 STARTTLS, 465 implicit TLS, 25 plain). |
| `SMTP_USERNAME` | `cotton-id@example.com` | SMTP auth user (omit for an unauthenticated relay). |
| `SMTP_PASSWORD` | (secret) | SMTP auth password — a **secret**; never commit it. |
| `SMTP_FROM` | `cotton-id <no-reply@example.com>` | The envelope/From address. Must be one your provider allows. |
| `SMTP_STARTTLS` | `true` | Upgrade the connection to TLS via STARTTLS (recommended for 587). |

```bash
# After setting the SMTP_* vars in deploy/.env, recreate the backend:
docker compose -f deploy/docker-compose.yml up -d --force-recreate backend
```

### Verify delivery

Trigger a password-reset and watch the backend logs for the send result (success or a logged,
non-blocking failure):

```bash
docker compose -f deploy/docker-compose.yml logs backend | grep -iE 'mail|smtp|reset'
```

A successful send logs the delivery; a failure logs the error **without** the reset token or any
secret, and the user's `202` still returns. If email isn't arriving: confirm `SMTP_HOST` is set (else
the dev logger is active), check the `FROM` address is authorized by your provider, verify the
port/STARTTLS match (587+STARTTLS vs 465 implicit TLS), and inspect the logged SMTP error.

### Login-notification emails

On a successful interactive sign-in from a **new device/IP**, if the account's `login_notifications`
preference is on, cotton-id sends a best-effort notification email. "New device" is a coarse,
**stateless** (user-agent, IP) heuristic against recent sessions — a usability signal, not a security
control — so it can both miss and over-notify; users toggle it from `/account` preferences. See
[SECURITY.md](SECURITY.md) §3.
