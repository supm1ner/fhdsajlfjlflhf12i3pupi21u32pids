# API reference

cotton-id's HTTP API. The **authoritative, always-current** reference is the auto-generated Swagger
UI served by the running backend — this document is a stable orientation map plus copy-pasteable
examples. The exact request/response shapes are pinned in
[docs/dev/build-contract.md](dev/build-contract.md) §3.

- **Base path:** `/api/v1`
- **JSON:** request and response bodies are JSON, **camelCase**.
- **Errors:** `application/problem+json` (RFC 7807) — `{type, title, status, detail, instance}`.
  Errors never leak stack traces or internal details.
- **CSRF:** state-changing browser routes require `X-CSRF-Token` header == the `cid_csrf` cookie
  (obtain both from `GET /api/v1/csrf`).
- **Admin auth:** admin routes require the `X-Admin-Key: <ADMIN_API_KEY>` header.
- **Sessions:** session-protected routes require the `cid_session` cookie (set on login/signup).

---

## Viewing Swagger

With the stack running:

```
http://localhost:8080/swagger/
```

(or via the frontend's same-origin proxy: `http://localhost:3000/swagger/`).

Every public endpoint is annotated with `swaggo/swag` and appears in Swagger UI with its method,
path, request schema, and response schemas (platform-foundation spec: "Swagger UI lists all
endpoints"). The raw documents are also generated to `backend/docs/swagger.json` and
`backend/docs/swagger.yaml`. A CI drift check fails if the generated docs fall out of sync with the
handler annotations, so Swagger is guaranteed complete.

---

## Endpoint reference

### Auth & session — `/api/v1/auth/*`

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| `GET` | `/api/v1/csrf` | none | → `200 {token}` (also sets `cid_csrf` cookie) |
| `POST` | `/api/v1/auth/signup` | CSRF | `{displayName,username,email,password}` → `201 {user}` + session cookie |
| `POST` | `/api/v1/auth/login` | CSRF | `{email,password,remember}` → `200 {user}` + session cookie |
| `POST` | `/api/v1/auth/logout` | CSRF + session | → `204` |
| `GET` | `/api/v1/auth/session` | session | → `200 {user}` / `401` |
| `POST` | `/api/v1/auth/password/forgot` | CSRF | `{email}` → `202 {message}` (never enumerates) |
| `POST` | `/api/v1/auth/password/reset` | CSRF | `{token,password}` → `204` |

### Account self-service (signed-in user) — `/api/v1/account/*`

Session-protected; state-changing calls also require CSRF. Full surface and security rules in
[ACCOUNT.md](ACCOUNT.md).

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| `GET` | `/api/v1/account` | session | → `200 {profile, preferences, counts}` |
| `PATCH` | `/api/v1/account` | CSRF + session | `{displayName?,about?,location?}` → `200 {profile}` |
| `PUT` | `/api/v1/account/images/{kind}` | CSRF + session | `multipart/form-data` (avatar\|banner; type + size capped) → `200 {url}` |
| `GET` | `/api/v1/account/images/{kind}` | session | → `200` image bytes |
| `PATCH` | `/api/v1/account/preferences` | CSRF + session | `{theme?,lang?,loginNotifications?}` → `200 {preferences}` |
| `PUT` | `/api/v1/account/password` | CSRF + session | `{currentPassword,newPassword}` → `204` (re-auth; revokes other sessions) |
| `GET` | `/api/v1/account/sessions` | session | → `200 {sessions:[...]}` (current flagged) |
| `DELETE` | `/api/v1/account/sessions/{id}` | CSRF + session | → `204` (scoped to the user) |
| `DELETE` | `/api/v1/account/sessions` | CSRF + session | → `204` (revoke all others) |
| `GET` | `/api/v1/account/connections` | session | → `200 {connections:[{client,grantedScopes,grantedAt}]}` |
| `DELETE` | `/api/v1/account/connections/{client}` | CSRF + session | → `204` (revoke that client's consent) |
| `DELETE` | `/api/v1/account` | CSRF + session | `{currentPassword?\|confirm?}` → `204` (re-auth; cascade delete) |

### Admin — OAuth client (relying-party) registry — `/api/v1/admin/*`

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| `POST` | `/api/v1/admin/clients` | admin-key | `{name,redirectUris,scopes,grantTypes,responseTypes,clientType}` → `201 {clientId,clientSecret?}` |
| `GET` | `/api/v1/admin/clients` | admin-key | → `200 {clients:[...]}` |
| `DELETE` | `/api/v1/admin/clients/{id}` | admin-key | → `204` |

`clientSecret` is returned only for **confidential** clients (it is not returned for public clients,
which use PKCE).

### OIDC login/consent (SPA-facing JSON) — `/api/v1/oauth/*`

| Method | Path | Auth | Body → Response |
|---|---|---|---|
| `GET` | `/api/v1/oauth/consent?consent_challenge=` | session | → `200 {client:{id,name},requestedScopes,user}` |
| `POST` | `/api/v1/oauth/login/accept` | CSRF + session | `{loginChallenge}` → `200 {redirectTo}` |
| `POST` | `/api/v1/oauth/consent/accept` | CSRF + session | `{consentChallenge,grantScopes,remember}` → `200 {redirectTo}` |
| `POST` | `/api/v1/oauth/consent/reject` | CSRF + session | `{consentChallenge}` → `200 {redirectTo}` |

### OIDC browser-redirect endpoints (served by the backend, **not** under `/api`)

These are the entry points Hydra 302-redirects the browser to. They are not JSON APIs; they redirect.

| Method | Path | Behavior |
|---|---|---|
| `GET` | `/oauth/login?login_challenge=` | Hydra entry: accept if the session can skip, else `302 → FRONTEND_BASE_URL/login?login_challenge=` |
| `GET` | `/oauth/consent?consent_challenge=` | accept if skippable, else `302 → FRONTEND_BASE_URL/consent?consent_challenge=` |
| `GET` | `/oauth/logout?logout_challenge=` | accept logout via Hydra, clear session, `302 → post-logout URL` |

### Operations

| Method | Path | Behavior |
|---|---|---|
| `GET` | `/healthz` | `200` (or `503`) with a dependency report (Postgres, Hydra) |
| `GET` | `/metrics` | Prometheus text exposition |
| `GET` | `/swagger/*` | Swagger UI + generated OpenAPI document |

The OAuth2/OIDC **protocol** endpoints themselves (`/oauth2/auth`, `/oauth2/token`, JWKS,
`/.well-known/openid-configuration`) are served by **Hydra** on `:4444`, not by cotton-id.

---

## Examples

> The examples below run against a local stack (`http://localhost:8080`). Adjust the host for your
> environment. They use a cookie jar (`-c`/`-b cookies.txt`) so the session and CSRF cookies persist
> across calls.

### 1. Get a CSRF token

State-changing browser calls need a CSRF token. Fetch one (it is returned in the body and set as the
`cid_csrf` cookie):

```bash
curl -s -c cookies.txt http://localhost:8080/api/v1/csrf
# → {"token":"<csrf-token>"}
```

Capture the token into a shell variable for reuse:

```bash
CSRF=$(curl -s -c cookies.txt http://localhost:8080/api/v1/csrf | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
```

### 2. Sign up

```bash
curl -s -b cookies.txt -c cookies.txt \
  -X POST http://localhost:8080/api/v1/auth/signup \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d '{
        "displayName": "Ada Lovelace",
        "username": "ada",
        "email": "ada@example.com",
        "password": "correct horse battery staple"
      }'
# → 201 { "user": { "id":"...", "email":"ada@example.com", "username":"ada", "displayName":"Ada Lovelace", ... } }
# A cid_session cookie is set in cookies.txt.
```

### 3. Log in

```bash
curl -s -b cookies.txt -c cookies.txt \
  -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d '{ "email": "ada@example.com", "password": "correct horse battery staple", "remember": true }'
# → 200 { "user": { ... } }  + refreshed cid_session cookie
```

Check the current session, then log out:

```bash
curl -s -b cookies.txt http://localhost:8080/api/v1/auth/session       # → 200 {user} or 401

curl -s -b cookies.txt -X POST http://localhost:8080/api/v1/auth/logout \
  -H "X-CSRF-Token: $CSRF"                                             # → 204
```

### 4. Request a password reset (non-enumerating)

```bash
curl -s -b cookies.txt -X POST http://localhost:8080/api/v1/auth/password/forgot \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d '{ "email": "ada@example.com" }'
# → 202 { "message": "If that account exists, a reset link has been sent." }
```

In development the reset **token is logged** by the dev `Mailer` (not emailed). Grab it from the
backend logs, then set a new password:

```bash
curl -s -b cookies.txt -X POST http://localhost:8080/api/v1/auth/password/reset \
  -H "Content-Type: application/json" \
  -H "X-CSRF-Token: $CSRF" \
  -d '{ "token": "<token-from-logs>", "password": "a different strong passphrase" }'
# → 204  (token consumed; existing sessions for the account invalidated)
```

### 5. Register an OAuth client (relying party)

Admin endpoints use `X-Admin-Key` (not CSRF/session). Register a **public** client that uses PKCE:

```bash
curl -s -X POST http://localhost:8080/api/v1/admin/clients \
  -H "Content-Type: application/json" \
  -H "X-Admin-Key: $ADMIN_API_KEY" \
  -d '{
        "name": "Vault Web",
        "redirectUris": ["http://localhost:5173/callback"],
        "scopes": ["openid", "profile", "email"],
        "grantTypes": ["authorization_code", "refresh_token"],
        "responseTypes": ["code"],
        "clientType": "public"
      }'
# → 201 { "clientId": "..." }            (no clientSecret for public clients — they use PKCE)
```

For a **confidential** client, set `"clientType": "confidential"`; the response includes a one-time
`clientSecret` — store it securely, it is not retrievable later.

List and delete clients:

```bash
curl -s -H "X-Admin-Key: $ADMIN_API_KEY" http://localhost:8080/api/v1/admin/clients
# → 200 { "clients": [ ... ] }

curl -s -X DELETE -H "X-Admin-Key: $ADMIN_API_KEY" \
  http://localhost:8080/api/v1/admin/clients/<clientId>
# → 204
```

### 6. Run the full OIDC flow

With a client registered, point a browser (or an OIDC client library) at Hydra's authorization
endpoint to start the `authorization_code` + PKCE flow:

```
http://localhost:4444/oauth2/auth?response_type=code
  &client_id=<clientId>
  &redirect_uri=http://localhost:5173/callback
  &scope=openid%20profile%20email
  &state=<random>
  &code_challenge=<S256-challenge>
  &code_challenge_method=S256
```

The browser is redirected through cotton-id's login and consent screens (see the sequence in
[ARCHITECTURE.md](ARCHITECTURE.md) §3), then back to your `redirect_uri` with a `code`. Exchange it
at `http://localhost:4444/oauth2/token` (with the PKCE `code_verifier`) for an ID token and access
token. End-to-end ops steps are in [RUNBOOK.md](RUNBOOK.md).

---

## Status codes & errors

| Status | Meaning in cotton-id |
|---|---|
| `200` / `201` / `202` / `204` | Success (see per-endpoint table) |
| `400` | Malformed request / validation error (problem+json with field detail) |
| `401` | Not authenticated (missing/expired session) |
| `403` | CSRF failure, missing/invalid admin key, or consent denied |
| `404` | Unknown resource (e.g. client id) |
| `429` | Rate-limited / throttled (credential endpoints) |
| `5xx` | Server / dependency error (generic problem+json; details in logs, not the body) |

Login and password-reset deliberately return **uniform** errors to resist account enumeration (see
[SECURITY.md](SECURITY.md) §2.4) — do not rely on distinguishing "unknown email" from "wrong
password" via the API.
