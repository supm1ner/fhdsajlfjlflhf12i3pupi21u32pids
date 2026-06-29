# Security

cotton-id is a **production identity provider** that handles credentials and drives token issuance,
so security is the primary design constraint, not an afterthought. This change
(`bootstrap-platform-and-oidc-login`) owns the credential-handling and token-issuance paths, and
therefore **sets the security baseline that every later change inherits**.

This document records the threat model, the concrete security decisions (with their rationale), and
— importantly — the **known gaps** in this walking skeleton. It describes intended/contracted
behavior; the authoritative parameter values live in
[docs/dev/build-contract.md](dev/build-contract.md) §5.

To report a vulnerability, contact the maintainers privately (do not open a public issue).

---

## 1. Threat model

**Assets we protect:** user credentials (passwords), session tokens, password-reset tokens, the
relying-party (OAuth client) registry and secrets, the Hydra admin API, users' own account data
(profile, preferences, sessions, consent grants — managed via account self-service, §2.10), the
operator-only administration surface and the integrity of the audit trail (admin console, §2.11), and
the integrity of issued ID/access tokens.

**Trust boundaries:**

- The **browser** is untrusted: it can be hostile, scripted, or running attacker-controlled pages on
  other origins.
- **Relying parties** are semi-trusted: registered, but a compromised RP must not be able to
  escalate beyond its granted scopes or exfiltrate other clients' secrets.
- **Hydra's admin API (`:4445`)** is a high-value internal surface — never exposed beyond the
  backend.
- **PostgreSQL** is trusted infrastructure inside the compose network.

**Adversaries and the mitigations in this change:**

| Threat | Mitigation |
|---|---|
| Offline password cracking after a DB leak | argon2id (memory-hard), per-password salt, PHC-encoded params (§2.1) |
| Credential stuffing / brute force | per-IP + per-account rate limiting and incremental lockout (§2.5) |
| Session theft via XSS | `HttpOnly` cookies (JS can't read them) + strict CSP (§2.2, §2.6) |
| Session theft via cookie value | opaque random 256-bit id stored hashed server-side; cookie is not the user id (§2.2) |
| Cross-site request forgery | double-submit CSRF token on all browser-posted state changes + `SameSite=Lax` (§2.3) |
| Clickjacking | `X-Frame-Options`/`frame-ancestors` (§2.6) |
| MIME sniffing | `X-Content-Type-Options: nosniff` (§2.6) |
| Account enumeration | uniform "invalid credentials" and non-enumerating password reset (§2.4) |
| Token/PKCE/JWKS protocol bugs | delegated to certified Ory Hydra; cotton-id never mints tokens (§2.7) |
| Authorization-code interception | PKCE required for public clients (enforced by Hydra) (§2.7) |
| Secrets in source/logs | env/compose injection, fail-fast on weak prod defaults, no credentials in logs (§2.8) |
| Replay of reset tokens | single-use, time-limited, hashed-at-rest reset tokens; sessions invalidated on reset (§2.4) |
| Hijacked session changing password / deleting account | re-authentication (current password, or typed confirmation) required for both; deletion double-confirmed + audited (§2.10) |
| Malicious image upload (oversize / wrong type / decompression) | strict type allowlist (content-type + magic bytes) and hard size caps; single overwrite per kind (§2.10) |
| Non-admin reaching the operator console / privilege escalation | server-side role gate (`user`<`admin`<`owner`) on every admin endpoint, owner-only on dangerous actions, last-owner + self-action guards; client gate is UX only (§2.11) |
| Repudiation of admin actions / tampering with the audit trail | persistent, append-only `audit_log` (no update/delete API) recording every security/admin event with actor, target, trusted IP, and request id; no secrets (§2.11) |

**Out of scope for this change** (see "Known gaps", §3): a second authentication factor (TOTP),
distributed rate limiting, and production secret management. (Passkeys — §2.9 — and social-login
account linking are covered by later changes that build on this baseline.)

---

## 2. Security decisions

### 2.1 Password hashing — argon2id

Passwords are hashed with **argon2id** (`golang.org/x/crypto/argon2`), the modern memory-hard KDF,
with these code-default parameters (build-contract §5):

| Parameter | Value |
|---|---|
| time (iterations) | 3 |
| memory | 64 MiB (`64 * 1024` KiB) |
| threads (parallelism) | 4 |
| salt length | 16 bytes (per-password, random) |
| key length | 32 bytes |

The hash is stored as a standard **PHC string** (`$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>`) so
the algorithm parameters travel with the hash and can evolve over time (rehash-on-login as
parameters are raised). Verification decodes the PHC string and compares in **constant time** with
respect to the hash bytes (password-authentication spec: "Hash verification is constant-time"). The
plaintext is never stored or logged.

**Why argon2id** (design D3): bcrypt is weaker against GPU attacks; scrypt is acceptable but
argon2id is the modern default. The `memory` parameter is the one to tune to deployment RAM (open
question in the design — 64 MiB assumed).

**Password policy.** Minimum 8 characters, with a server-enforced minimum strength (bonuses for case
mix, digits, symbols), mirroring the prototype's strength meter. The client-side meter is
**advisory only** — the server is authoritative and rejects weak passwords on signup and reset.

### 2.2 Sessions — opaque, server-side, cookie-delivered

Sessions are **opaque server-side records** in PostgreSQL, not stateless JWTs (design D4):

- On login, a random **256-bit** token is generated. The DB stores `sha256(token)` hex as the
  session id; the **raw** token is delivered to the browser in the `cid_session` cookie. A DB leak
  therefore does not yield usable session tokens.
- The cookie value is an **opaque identifier — never the raw user id** (password-authentication
  spec).
- **Lifetime:** "remember me" → 30-day server expiry; otherwise a browser-session cookie with a
  24-hour server expiry. The server-side `expires_at` is authoritative — an expired record is never
  honored even if the cookie is replayed.
- **Revocation:** logout deletes the session row and clears the cookie; the prior cookie can no
  longer authenticate. (Password reset invalidates all of the account's sessions.)

**Why server-side** (design D4): unlike stateless JWT cookies, server-side sessions are instantly
revocable, and the row model is exactly what the future "active sessions / revoke" surface needs.

**Cookie attributes** (build-contract §5) on both `cid_session` and `cid_csrf`:

| Attribute | Value |
|---|---|
| `HttpOnly` | yes (blocks JS access → mitigates XSS session theft) |
| `Secure` | per `COOKIE_SECURE` (`false` in dev over HTTP, **`true` in production**) |
| `SameSite` | `Lax` (blocks most cross-site sends; defense-in-depth with CSRF) |
| `Path` | `/` |

### 2.3 CSRF — synchronizer-token (double-submit)

All **browser-posted, state-changing** routes — signup, login, logout, password forgot/reset, and
the OAuth login/consent accept/reject POSTs — require a CSRF token (design D5):

- `GET /api/v1/csrf` issues a token, returning it in the body **and** setting it as the `cid_csrf`
  cookie.
- State-changing requests must echo it in the `X-CSRF-Token` header. Middleware verifies
  `X-CSRF-Token == cid_csrf` (double-submit).
- **`SameSite=Lax` is defense-in-depth, not the sole defense** — the consent and login POSTs are
  browser-driven and state-changing, so an explicit token is required.

**Exemption:** pure machine-to-machine admin endpoints (`/api/v1/admin/*`) are CSRF-exempt and
instead require the `X-Admin-Key` header — they aren't driven by an authenticated browser session,
so CSRF doesn't apply.

### 2.4 Account enumeration resistance & password reset

- **Login** returns the same generic "invalid credentials" error whether the email is unknown or the
  password is wrong — never disclosing which field was wrong (password-authentication spec).
- **Signup** rejects a duplicate email with a generic "already in use" message rather than confirming
  the specific email is registered.
- **Password reset request** (`POST /api/v1/auth/password/forgot`) responds with the **same** `202`
  success message regardless of whether an account exists for that email — it never enumerates.
- **Reset tokens** are single-use and time-limited (`PASSWORD_RESET_TTL_MINUTES`, default 30).
  Stored as `sha256(token)` hex in `password_reset_tokens` with `used_at` marking consumption. On a
  successful reset the token is invalidated **and the account's existing sessions are invalidated**
  (password-authentication spec: "Valid token sets a new password").
- A **used or expired** token is rejected and does not change the password.

### 2.5 Rate limiting & lockout

Per-IP **and** per-account token-bucket limits guard `/api/v1/auth/login`, `/api/v1/auth/signup`,
and the password-reset endpoints (design D6, build-contract §5):

- Defaults: `RATE_LIMIT_RPS=5`, `RATE_LIMIT_BURST=10` (config-driven).
- A **dedicated per-account lockout** (separate from the steady-state token bucket) tracks
  *consecutive* login failures: after `LOGIN_LOCKOUT_THRESHOLD` (default 5) failures the account is
  temporarily refused with **incremental backoff** (30s, doubling, capped at 15m), reset on a
  successful login. A `Retry-After` header tells the client when to retry.
- Throttling and lockout events are recorded as **security events** in the structured log.
- The per-IP bucket and audit log key off a **trusted** client IP: `X-Forwarded-For` is honored
  **only** from a configured `TRUSTED_PROXIES` CIDR allowlist (empty by default → the direct peer
  IP is used), so a spoofed header cannot rotate the limiter bucket or poison the audit trail.

The limiter and lockout sit behind interfaces so a Redis/DB backend can replace the in-memory ones
later (see Known gaps).

### 2.6 Security headers

Every browser-facing response carries (platform-foundation spec, "HTTP security middleware
baseline"):

- `Content-Security-Policy` — constrains script/style/connect sources; the primary XSS mitigation
  alongside `HttpOnly` cookies.
- `X-Content-Type-Options: nosniff` — no MIME sniffing.
- `Referrer-Policy` — limits referrer leakage.
- `X-Frame-Options` / CSP `frame-ancestors` — anti-clickjacking.
- `Strict-Transport-Security` (HSTS) — emitted when `COOKIE_SECURE=true` (production over TLS) so
  browsers refuse http downgrade and an active attacker cannot SSL-strip the session/CSRF cookies.
- A unique **request-id** header for correlation; an inbound `X-Request-Id` is reused only when it
  passes a strict charset/length check, otherwise a fresh UUID is generated (no log-forging).

Error responses are `application/problem+json` and **do not leak stack traces or internal details**.

### 2.7 Protocol crypto — delegated to Ory Hydra

cotton-id **never mints tokens, validates PKCE, or serves JWKS** — Ory Hydra (a certified OIDC
engine) does (design D1). This deliberately removes the highest-severity bug class from cotton-id's
surface:

- Authorization codes, ID tokens, access tokens, refresh-token rotation, and JWKS are entirely
  Hydra's.
- **PKCE is required for public clients** — an authorization request from a public client without a
  `code_challenge` is rejected (oidc-provider spec). This is enforced by Hydra.
- cotton-id only supplies *who the user is* (stable `sub` = user UUID) and *what they consented to*,
  via Hydra's admin API. The `sub` is stable across sessions/clients and never reused.

### 2.8 Secret handling

- All secrets (DB password, Hydra system/cookie secrets, session/CSRF keys, `ADMIN_API_KEY`) are
  injected via environment / compose, **never committed**. `.gitignore` excludes `.env` and
  `deploy/.env`; only `.env.example` (placeholders) is tracked.
- Config is **validated at startup**: the process exits non-zero before binding the port if a
  required secret is missing, and **refuses known-insecure defaults outside development**
  (`COTTON_ENV != development`) — design D11, platform-foundation spec "Missing required secret fails
  fast".
- `ADMIN_API_KEY` must be 32+ random bytes and is **required in production**.
- **No credentials in logs:** no log line contains a plaintext password or a full session token
  (platform-foundation spec, "Credentials never appear in logs").

### 2.9 Phishing-resistant passkeys (WebAuthn)

cotton-id supports **passkeys** (WebAuthn / FIDO2) as a passwordless, **phishing-resistant**
sign-in method (`add-passkey-auth` change):

- A passkey is a public-key credential **scoped to cotton-id's relying-party domain** (the
  configured `WEBAUTHN_RP_ID`). The private key never leaves the user's authenticator, and the
  browser refuses to use the credential on any other origin — so a credential **cannot be phished**
  or replayed against a look-alike site, unlike a password. This is the strong-auth path that
  offsets single-factor password accounts (TOTP remains deferred).
- The relying party is configured from environment (`WEBAUTHN_RP_ID`, `WEBAUTHN_RP_DISPLAY_NAME`,
  `WEBAUTHN_RP_ORIGINS`); the RP ID must be a registrable suffix of every allowed origin, and
  ceremonies whose origin/RP ID don't match are rejected. **Production must set the real domain and
  https origin** (`http://localhost` is the only insecure-context exception).
- **Cloned-authenticator detection:** each credential's signature counter is tracked, and a
  non-increasing counter (for authenticators that maintain one) is refused and logged as a security
  event, per WebAuthn guidance — see §3 and [PASSKEYS.md](PASSKEYS.md).
- **Lost-all-passkeys fallback:** passkeys are additive, never the only way in. A user who loses
  every passkey signs in with their **password** or recovers via **password reset** (§2.4); there is
  no separate passkey-recovery flow by design.
- Ceremony challenge state rides in a **short-lived, HMAC-signed, HttpOnly `cid_wa` cookie**
  (10-min TTL), mirroring the social-login state cookie — no server-side ceremony table, and the
  signed cookie resists tampering.

Operator setup (RP config, prod origins, the lost-passkey fallback, clone-detection behavior) is in
[PASSKEYS.md](PASSKEYS.md).

### 2.10 Account self-service — re-auth, revocation, upload limits, deletion

The **account self-service** surface (`add-account-self-service` change) lets a signed-in user manage
their own identity from `/account`. Because it exposes sensitive, destructive actions over an
existing browser session, it carries its own controls. The operator-facing detail is in
[ACCOUNT.md](ACCOUNT.md); the security-relevant rules are:

- **Re-authentication for sensitive actions.** Holding a live session is **not** sufficient to change
  the password or delete the account — the user must prove presence:
  - **Password change** (`PUT /api/v1/account/password`) verifies the **current** password (via the
    same constant-time password authenticator as login, §2.1) before accepting a new
    policy-compliant one. A wrong current password is rejected and nothing changes.
  - **Account deletion** (`DELETE /api/v1/account`) requires re-auth — the current password, or a
    typed confirmation for a **social-only** account (no password set). Without valid re-auth the
    account is not deleted.
  This blunts a hijacked-but-idle session silently taking over or destroying the account.

- **Session revocation.** Sessions are opaque server-side rows (§2.2), so they are **instantly
  revocable**. A user can list their active sessions — the **current** one is identified by hashing
  the request's session cookie and matching the stored `sessions.id` (= `sha256(token)`) — and revoke
  any of them, or "revoke all others". A successful **password change revokes the user's other
  sessions** (keeping the current one), mirroring reset semantics (§2.4) minus the token: changing
  the password kicks out anyone holding a live session.

- **Consent-grant revocation.** Listing "connected apps" reads the user's Hydra **consent sessions**
  by subject; revoking one calls Hydra's admin API to revoke that client's consent for the subject,
  so the relying party must obtain consent again on its next authorization. This is the user-facing
  complement to consent "remember" (§3, "Consent remember semantics"); all Hydra calls stay behind
  the one `oidc.HydraClient`.

- **Profile-image upload limits.** Avatars/banners are **bounded blobs in Postgres**, constrained to
  blunt decompression/abuse and keep rows small:
  - **Type allowlist** `image/png` / `image/jpeg` / `image/webp`, validated by **both** the declared
    content-type **and** the file's magic bytes — a file lying about its type is rejected.
  - **Hard size caps**, env-tunable and non-secret: `ACCOUNT_AVATAR_MAX_KB` (default 512 KB) and
    `ACCOUNT_BANNER_MAX_KB` (default 1024 KB). An over-cap or disallowed-type upload returns
    `problem+json` and stores nothing.
  - **One avatar + one banner per user** — a new upload overwrites the old, so there is no unbounded
    accumulation.

- **Deletion cascade.** A confirmed, re-authenticated `DELETE /api/v1/account` deletes the `users`
  row; foreign-key `ON DELETE CASCADE` removes that user's **sessions, passkeys
  (`webauthn_credentials`), social identities, and profile images** in the same transaction (no
  orphans), then **best-effort** revokes the subject's Hydra login + consent sessions (a Hydra hiccup
  does not block the local delete — the app DB is the source of truth). The action is **irreversible**,
  double-confirmed in the UI, and recorded as a structured **security event** (no secrets) for audit.

All account endpoints are session-protected (an unauthenticated call gets `401`) and the
state-changing ones are in the CSRF group (§2.3); errors are `problem+json` and never leak internals
(§2.6).

### 2.11 Admin console — RBAC enforcement, escalation guards, and the audit log

The **admin console** (`add-admin-console` change) is the operator surface for managing every
account — list/search users, inspect a user, and run lifecycle actions (suspend, role change, force
reset, delete). Because it exposes privileged, cross-account, destructive actions, it carries the
strongest authorization in the system. The operator-facing detail is in [ADMIN.md](ADMIN.md); the
security-relevant rules are:

- **Role-based access, enforced server-side.** Accounts have a ranked role — **`user` < `admin` <
  `owner`** (the `users.role` column, build-contract §4). The human console API at `/api/v1/admin/*`
  is gated by a `RequireRole` middleware that resolves the session user (via
  `auth.Service.UserForSession`) and checks the rank **on every request**: an unauthenticated call
  gets `401`, a signed-in non-admin gets `403`. The SPA also hides `/admin` from non-admins, but **the
  client-side gate is UX only — the server is authoritative**. Hiding the UI is never the control;
  each endpoint independently re-checks the role, and owner-only actions re-check `owner` in the
  handler. This human gate is **separate from the machine `X-Admin-Key`** (§2.3) used by OAuth
  client-registration — the key is for unattended scripts, the role gate for signed-in operators; they
  do not interchange.

- **Privilege-escalation guards on every lifecycle action.** The action rules are enforced in the
  backend regardless of what the UI offers, so the console cannot be turned into an escalation tool:
  - **Only an `owner` may grant or revoke `admin`/`owner`.** An `admin` cannot mint another admin or
    promote anyone — closing the "admin escalates itself by proxy" path.
  - **An `admin` cannot act on an `owner`.** Touching an owner account requires owner rank.
  - **No self-escalation** — a user cannot raise their own role.
  - **No self-action on destructive paths** — you cannot **suspend** or **delete your own** account
    from the console (self-service handles your own account, §2.10).
  - **The last `owner` is protected** — the system refuses to demote or delete the final owner, so
    cotton-id can never be locked out of ownership.

- **Suspension revokes access immediately.** Suspending an account sets `status = suspended` **and
  revokes the target's sessions** (`SessionStore.DeleteByUser`, the same instant-revocation property
  as §2.2/§2.10), so a suspended user is signed out everywhere at once and the login path refuses a
  non-active account (§2.4).

- **Force password reset reuses the hardened reset path.** The admin "force reset" issues a
  **single-use, time-limited, hashed-at-rest** reset token via the same mechanism as self-service
  forgot-password (§2.4) — it does not set or reveal a password; the user completes the reset
  themselves. (Delivery is stubbed today — §3.)

- **Delete cascades and is owner-gated.** An owner-only `DELETE` removes the `users` row with FK
  `ON DELETE CASCADE` (sessions, passkeys, social identities, profile images) and best-effort revokes
  the subject's Hydra sessions — the same cascade as self-service deletion (§2.10) — guarded by the
  not-self and last-owner rules above.

- **Every admin action is audited.** Each lifecycle action writes an entry to the **persistent
  audit log** (below) attributing it to the calling operator with the target and request context —
  whether or not it changed state — so privileged activity is reviewable after the fact.

- **Persistent, append-only audit log.** Security-relevant and administrative events (login ok/fail,
  signup, password reset, consent decisions, client registration, and the admin lifecycle actions) are
  recorded to a durable `audit_log` table (migration `0005_audit_log`), **in addition to** the
  structured stdout security log (§2.5). Previously these events reached **stdout only**, so they did
  not survive the container; the audit log makes the trail queryable (the console's **Journal**, and
  `GET /api/v1/admin/audit`). It is **append-only by design** — the API exposes read and the backend
  exposes write; there is **no update or delete** path. Each row carries `ts, actor_id, actor_label,
  action, target_type, target_id, ip, request_id, metadata`. The `ip` is the **trusted** client IP
  (`httpx.ClientIP`, honoring `TRUSTED_PROXIES`, §2.5), so a spoofed `X-Forwarded-For` cannot poison
  the trail; rows carry **no secrets** (no passwords, session tokens, or reset tokens). Writes are
  **best-effort and never block** the user's action — an audit-insert failure is logged at error and
  the action still succeeds, so auditing can never mask or break a login or an admin operation.
  Search terms reach the user-listing query as **bound parameters** (parameterized pgx with
  ILIKE/citext), so the console's search is not an injection surface.

The console adds **no new secrets or required env** (it reuses session/CSRF/trusted-proxy/reset
config, build-contract §5). The only new persistent state is the `audit_log` table. All admin
endpoints are session-protected and role-gated; state-changing ones are in the CSRF group (§2.3);
errors are `problem+json` and never leak internals (§2.6).

### 2.12 Relying-party (OAuth client) management — role-gated, secret-once, audited

The **Services** tab (`add-client-consent-management` change) lets an operator manage the
relying-party (OAuth client) registry — register, edit, delete clients, see per-client consent usage,
and revoke a client's grants — from the role-gated console. Because the client registry and its
secrets are protected assets (§1) and a misconfigured or hostile client can intercept authorization
codes, this surface carries the console's authorization and audit guarantees. The operator-facing
detail is in [ADMIN.md](ADMIN.md); the security-relevant rules are:

- **Role-gated, server-enforced — distinct from the machine key.** Console client management lives at
  `/api/v1/admin/services` behind **session + `RequireRole(admin)` + CSRF** (the same gate as the rest
  of the console, §2.11): an unauthenticated call gets `401`, a signed-in non-admin gets `403`, and the
  client-side gate is UX only. This is a **separate path and separate auth** from the machine
  **`X-Admin-Key`** client-registration route (`/api/v1/admin/clients`, §2.3) — chi forbids two
  registrations at one method+path, and the two need different auth (design D1). Both doors reach the
  **same** Hydra registry through the one `oidc.HydraClient`, but the admin key does **not** grant
  console access and a console session does **not** authorize the key route; they do not interchange.

- **Client secret is shown exactly once and never re-served.** For a **confidential** client, Hydra
  generates the `client_secret` and returns it **once** on create (and on an optional regenerate).
  cotton-id passes it through **once** and **never stores or re-serves it**: it is excluded from list
  and detail reads and from every later response, and it is **never logged** (consistent with §2.8,
  "no credentials in logs"). The transport is the **same-origin, authenticated console** over the
  session cookie; the UI shows it in a copy-once panel. A lost secret can only be **regenerated**
  (invalidating the old one) or replaced by re-creating the client. **Public (PKCE) clients have no
  secret**, so there is nothing to expose — they prove possession with a `code_challenge` (§2.7).

- **Edits are validated; type changes are explicit.** Redirect URIs are re-validated on create and
  edit — **absolute `http(s)`, host present, no fragment** (the same `validRedirectURI` helper the
  machine route uses, §2.3) — so a client cannot be pointed at a non-absolute or fragment-bearing
  redirect. Changing a client's **type** (public↔confidential) changes its auth method and secret
  handling and is treated as an explicit, constrained operation, not a silent edit (design D2), since
  a silent flip could strip a public client's PKCE requirement or orphan a confidential secret.

- **Deletion de-authorizes immediately.** Deleting a client removes it from Hydra, after which **any
  authorization request using its `client_id` is rejected** (`invalid_client`) — a delete instantly
  cuts off the relying party.

- **Every mutation is audited; no secrets in the trail.** Create, edit, delete, and consent-revoke
  each write an entry to the persistent, append-only **audit log** (§2.11) with the **signed-in
  operator** as the actor and the client id as the target — unlike the machine route, whose rows carry
  the `admin-key` actor label and no actor id. Audit rows carry **no secrets** (no `client_secret`),
  consistent with the rest of the trail.

- **Consent visibility/revocation reuses the consent-session path.** The per-client usage count and
  the revoke read/act on **Hydra's consent sessions** — the same mechanism behind a user's own
  "connected apps" revocation (§2.10), but aggregated/acted **per client**. Revoking a client's grants
  forces its users to consent again on next authorization. The per-client **count is best-effort**
  (Hydra lists consent sessions by subject, not by client — §3), a documented limitation rather than a
  security control.

This change adds **no new secrets or required env** and **no new persistent state** of its own — it
reuses the session/CSRF/role gate and the existing `audit_log` (§2.11), and stores client state only in
Hydra. All Services endpoints are session-protected and role-gated; mutations are in the CSRF group
(§2.3); errors are `problem+json` and never leak internals (§2.6).

---

## 3. Known gaps

These are **deliberate, documented limitations** of the walking skeleton. They are tracked for
later changes and called out so operators and reviewers are not misled into thinking they are
covered.

- **Password-only accounts are single-factor.** There is no *TOTP* second factor yet. An attacker
  with a valid password fully authenticates a password-only account. **Passkeys now provide a
  phishing-resistant, passwordless path** (§2.9, [PASSKEYS.md](PASSKEYS.md)), but **TOTP remains
  deferred** and passkeys are an *alternative* sign-in method rather than a mandatory second factor.
  The schema reserves an `mfa_*` seam. For accounts without a passkey, security rests on password
  strength + rate limiting + reset hygiene. (Design risk note, SECURITY-relevant spec gap.)

- **In-memory rate limiter — single-node only.** The token-bucket limiter lives in process memory,
  so limits and lockout state **do not survive a restart and are not shared across replicas**. A
  multi-replica deployment would let an attacker spread attempts across instances. Accepted for the
  single-node v1; the interface allows a Redis backend later (design D6 / risk note). *(Note: the
  separate **ceremony-state-cookie** single-instance limit — below — is now resolved by
  `OAUTH_STATE_KEY`; the rate limiter itself remains in-memory.)*

- **Multi-replica ceremony state — now supported via a shared key (per-process default).** *(Resolved
  in `harden-and-observe`.)* The OAuth/social `cid_oauth` and passkey `cid_wa` ceremony-state cookies
  are HMAC-signed. Previously each backend process generated a **random per-process** signing key, so a
  ceremony begun on one replica could not be finished on another — the cookie validated only on a single
  instance. Setting **`OAUTH_STATE_KEY`** (≥32 bytes) now derives both cookie keys from it via **HKDF**
  with distinct labels, so all replicas agree and a begin→finish ceremony survives load-balancing across
  instances. When `OAUTH_STATE_KEY` is **unset**, the safe **per-process random key remains the default**
  (correct for single-instance) and the backend logs a startup warning that multi-replica requires the
  shared key. The key is a secret like any other (env/compose, never committed, rotation logs ceremonies
  in flight out).

- **Dev secrets and no secret manager.** This change uses env/compose secrets with a **documented
  path to a manager** (Vault/KMS), but does not integrate one. Dev defaults are intentionally weak
  and **must not be used in production** — `COOKIE_SECURE=false`, example keys, and the demo seed
  client/admin are development-only. Production secret management is explicitly a non-goal here
  (design Non-Goals).

- **Email delivery — now real (SMTP), best-effort.** *(Resolved in `harden-and-observe`.)* cotton-id
  now delivers transactional email through a configurable **SMTP** transport
  (`SMTP_HOST/PORT/USERNAME/PASSWORD/FROM/STARTTLS`): password-reset tokens, the admin "force reset"
  link, the admin **"message user"** action, and **login-notification** emails. The dev `LogMailer`
  remains the **default** when `SMTP_HOST` is unset — and in that mode reset-token log lines are still
  sensitive, so treat them accordingly. Delivery is **best-effort**: a send failure is logged and the
  originating user action still succeeds — email delivery never blocks or fails a login, signup, reset,
  or admin operation. Configure real SMTP (and verify the `FROM` address) before relying on email in
  production; see [RUNBOOK.md](RUNBOOK.md) §12.

- **Login-notification emails — new-device heuristic is coarse and stateless.** *(New in
  `harden-and-observe`.)* On a successful interactive sign-in (password / social / passkey) from a
  device/IP not seen recently, if the account's `login_notifications` preference is on, cotton-id sends
  a best-effort notification email. "New device" is a **coarse fingerprint** — the (user-agent, IP) pair
  against the account's recent sessions — and is intentionally **stateless** (no known-devices table), so
  it can both miss (same coarse fingerprint from a genuinely new device) and over-notify (a rotating IP).
  This is a usability signal, **not** a security control, and the per-account preference lets a user turn
  it off. A stateful known-devices model is deferred.

- **PKCE enforcement requires a non-dev (TLS) Hydra.** Hydra is configured with
  `oauth2.pkce.enforced_for_public_clients: true`, but the local Compose runs Hydra in **dev mode**
  (required to serve an `http://` issuer — Hydra refuses http otherwise), and dev mode relaxes PKCE
  enforcement. The happy-path authorization-code + PKCE flow is verified end-to-end locally; the
  *rejection* of a public client that omits PKCE is guaranteed by config and takes effect in
  **production** (https issuer, dev mode off). The integration test asserts it only against an https
  issuer and skips under the dev stack. **Production must terminate TLS for Hydra and disable dev mode.**

- **Social-login account linking not yet present.** `email_verified` is recorded now precisely so
  that future identity linking can be restricted to verified emails — but social login itself
  (change 3) and its linking rules are not in this change.

- **`email_verified` is `false` for password signups (no verification flow yet).** ID tokens
  faithfully report `email_verified` from the account, which defaults to `false` until an
  email-verification flow ships (a later change; the seed admin is verified). **Relying parties that
  gate on `email_verified=true` will reject freshly self-signed-up users** — a conscious, documented
  behavior, not a bug.

- **Consent "remember" scope semantics.** Remembered consent skips re-prompting for the
  same-or-narrower scopes until revoked; broad scope grants can be reviewed and reset from the **admin
  console's Services tab**, which now exposes RP management and per-client consent **revoke** (§2.12).
  The remaining limit is that the per-client consent **count** is **best-effort** — Hydra lists consent
  sessions by **subject**, not by client, so the aggregate may be limited at scale (a documented
  figure, not a security control).

- **Best-effort Hydra session revoke — a small revocation window.** When an account is **deleted** or
  **suspended** (self-service §2.10, admin §2.11) — and when a user revokes a **connected app** — cotton-id
  best-effort revokes the subject's (or client's) Hydra login/consent sessions. The **local** state change
  is authoritative and atomic (the row/status change and the local session revocation), but the Hydra call
  is **best-effort and does not block** the action: if Hydra is briefly unreachable, the local change still
  succeeds and the Hydra-side login/consent session may persist until cotton-id's next successful call or
  the session's own expiry. Net effect: a deleted/suspended user is immediately locked out of **cotton-id**
  (the login path refuses a non-active/absent account, and their cotton-id sessions are gone), but an
  **already-issued, unexpired access token** at a relying party remains valid until it expires — token
  lifetime is Hydra's, and cotton-id does not (and by design cannot) reach into an RP to invalidate a live
  access token. Keep RP access-token lifetimes short if this window matters for your deployment.

- **Session last-seen — coarse, throttled (informational).** *(New in `harden-and-observe`.)* Each session
  now records a `last_seen_at` that is bumped when the session authenticates a request and is surfaced in
  the account "active sessions" view and the admin user detail. The bump is **throttled to ≤1/min/session**
  to avoid write amplification, so the displayed time is **approximate** (it can lag actual use by up to a
  minute) and is an **informational** signal for the user/operator — useful for spotting an unfamiliar
  active session — not a precise activity audit. The per-request **audit log** (§2.11) remains the
  authoritative event trail.

When deploying to production, the **minimum** hardening checklist is: set `COTTON_ENV=production`,
set `COOKIE_SECURE=true`, supply a 32+ byte `ADMIN_API_KEY` and strong Hydra/session/CSRF secrets,
serve everything over HTTPS, do **not** load the dev demo seed, and **create your first operator by
promoting a real account to `owner`** (the dev seed's `admin` operator must not exist in production —
see [RUNBOOK.md](RUNBOOK.md) §10). Additionally, for the capabilities added in `harden-and-observe`:
configure **SMTP** (`SMTP_*`) so transactional and login-notification email actually sends (§3, RUNBOOK
§12); if you run **more than one backend replica**, set a shared **`OAUTH_STATE_KEY`** (≥32 bytes) so
OAuth/passkey ceremonies validate across instances (§3); and if you enable the **observability** profile,
set a strong **`GRAFANA_ADMIN_PASSWORD`** and keep Prometheus/Grafana/`/metrics` off the public network.
See [RUNBOOK.md](RUNBOOK.md) and [OBSERVABILITY.md](OBSERVABILITY.md).
