# Account self-service

cotton-id gives every signed-in user a single **Account** surface to manage their own identity —
profile, security, active sessions, connected apps, preferences, and account deletion. The design
goal is *"see every login, every device, and every permission in one place"*: the data already
exists (users, sessions, passkeys, social identities, Hydra consent grants); this surface exposes it
for self-service.

Account self-service is the `add-account-self-service` change. The behavior, requirements
(`SHALL` + WHEN/THEN scenarios), and design decisions live in
[`openspec/changes/add-account-self-service/`](../openspec/changes/add-account-self-service/); the
exact request/response shapes ultimately come from the running backend's
[Swagger UI](API.md#viewing-swagger). This doc is the **orientation map**: what the surface offers,
the endpoints behind it, the security rules (re-authentication, revocation, upload limits, deletion
cascade), and the configuration knobs.

All account endpoints are **session-protected** (they require a valid `cid_session`; an
unauthenticated call gets `401`) and, where they change state, sit in the **CSRF group** (require
`X-CSRF-Token` == the `cid_csrf` cookie). Errors are `application/problem+json` (RFC 7807) like the
rest of the API.

---

## The Account screen

The SPA renders the Account screen at **`/account`** (auth-gated — an unauthenticated visit
redirects to `/login`). It ports the prototype layout (`_design_ref/screen-account.jsx`) into four
tabbed sections plus a profile header with avatar/banner:

| Section | What the user manages |
|---|---|
| **Profile** | Display name, about, location; avatar and banner image. Username and email are shown but **not editable here** (email change needs verification — deferred). |
| **Security** | Change password; manage **passkeys** (the [PASSKEYS.md](PASSKEYS.md) surface); review and revoke **active sessions** (current device marked). A **2FA / TOTP** row is rendered as a "coming soon" seam — TOTP itself is deferred. |
| **Connected services** | The relying parties (OAuth apps) the user has granted access to, via Hydra consent grants — with per-app revoke. |
| **Preferences** | Theme, language, and a login-notification toggle, persisted server-side so they sync across devices. |
| **Danger zone** | Permanently delete the account (double-confirmed, re-authenticated). |

---

## Endpoints

Base path `/api/v1`. JSON is camelCase. Every endpoint requires the session cookie; state-changing
ones also require the CSRF token. (The authoritative shapes are in Swagger; this table is the map.)

### Profile & images

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/api/v1/account` | session | Full profile incl. preferences and security counts (sessions, passkeys, connected apps). |
| `PATCH` | `/api/v1/account` | CSRF + session | Update display name / about / location (validated). |
| `PUT` | `/api/v1/account/images/{kind}` | CSRF + session | Upload avatar/banner (`kind` = `avatar` \| `banner`), `multipart/form-data`. Validated by content-type + magic bytes and size cap (below); sets the served image URL. |
| `GET` | `/api/v1/account/images/{kind}` | session | Serve the stored image. |

### Preferences

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `PATCH` | `/api/v1/account/preferences` | CSRF + session | Update theme (`system`\|`light`\|`dark`), language (`ru`\|`en`), and the login-notification toggle. |

### Security & sessions

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `PUT` | `/api/v1/account/password` | CSRF + session | Change password: **re-authenticate** with the current password, enforce the policy, rehash, and **revoke the user's other sessions** (keeping the current one). |
| `GET` | `/api/v1/account/sessions` | session | List the user's active sessions (device/UA, IP, created, last-seen, expiry) with the **current** request's session flagged. |
| `DELETE` | `/api/v1/account/sessions/{id}` | CSRF + session | Revoke one session (scoped to the user — you cannot revoke another account's session). Revoking the **current** session signs you out. |
| `DELETE` | `/api/v1/account/sessions` | CSRF + session | "Revoke all others" — delete every session except the current one. |

### Connected apps (consent grants)

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/api/v1/account/connections` | session | List the user's consent grants from Hydra (`{client, grantedScopes, grantedAt}`). |
| `DELETE` | `/api/v1/account/connections/{client}` | CSRF + session | Revoke a client's consent for the user's subject — the app must obtain consent again on its next authorization. |

### Account deletion

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `DELETE` | `/api/v1/account` | CSRF + session | Permanently delete the account after **re-authentication** (current password, or a typed confirmation for social-only accounts). Cascades and best-effort revokes Hydra sessions (below). Irreversible. |

> The **public, cross-app** half of connected-app/consent management (an operator browsing every
> grant, RP registration) is the admin console's job (a later change). This surface is strictly the
> **user's own** account.

---

## Security rules

These are summarized in [SECURITY.md](SECURITY.md) §2.10 and enforced by the backend.

### Re-authentication

Two actions are **sensitive** and require the user to prove they are present, not just that a session
cookie exists:

- **Password change** (`PUT /account/password`) verifies the **current** password (via the password
  authenticator) before accepting a new one. A wrong current password is rejected and nothing
  changes.
- **Account deletion** (`DELETE /account`) requires re-authentication — the current password, or for
  a **social-only** account (no password set) a typed confirmation. Without valid re-auth the account
  is **not** deleted.

This blunts a "hijacked live session" from silently changing the password or destroying the account.

### Session revocation

- A successful **password change revokes the user's other sessions** (keeping the current one),
  mirroring password-reset semantics minus the token — a changed password kicks out anyone who had a
  live session.
- The user can **list and revoke** sessions individually, or "revoke all others", from the Security
  tab. The current session is identified by hashing the request's session cookie and matching the
  stored `sessions.id` (which is `sha256(token)`); it is flagged in the list so the user knows which
  device they are on.
- Revocation **deletes the session row** — the prior cookie can no longer authenticate (server-side
  sessions are instantly revocable; see [SECURITY.md](SECURITY.md) §2.2).

### Connected-app (consent-grant) revocation

Revoking a connected app calls Hydra's admin API to **revoke that client's consent** for the user's
subject. The next time that relying party sends the user through authorization, Hydra re-prompts for
consent (the remembered grant is gone). This is the user-facing complement to consent "remember".

### Profile-image upload limits

Profile images are stored as **bounded blobs in Postgres** (no object store / CDN — small images at
this scale don't warrant one). Uploads are constrained to keep rows small and to blunt
decompression/abuse:

- **Type allowlist:** `image/png`, `image/jpeg`, `image/webp` only — validated by both the declared
  content-type **and** the file's magic bytes (a `.png` that isn't really a PNG is rejected).
- **Size caps:** an avatar over `ACCOUNT_AVATAR_MAX_KB` (default **512 KB**) or a banner over
  `ACCOUNT_BANNER_MAX_KB` (default **1024 KB / 1 MB**) is rejected.
- **One per kind:** a user has a single avatar and a single banner; a new upload **overwrites** the
  old (no unbounded accumulation).

An over-size or disallowed-type upload returns a `problem+json` error and stores nothing.

### Deletion cascade

Account deletion is destructive and the UI double-confirms. On a confirmed, re-authenticated delete
the backend:

1. Deletes the **user row**. Foreign-key `ON DELETE CASCADE` removes the user's **sessions**,
   **passkeys** (`webauthn_credentials`), **social identities**, and **profile images** in the same
   transaction — no orphans.
2. **Best-effort** revokes the subject's Hydra **login and consent sessions** so any in-flight OIDC
   state for that subject is cleared. (Best-effort: a Hydra hiccup does not block the local delete,
   which is the source of truth for the account's existence.)
3. Records a **security event** in the structured log (no secrets) for the audit trail.

After deletion the account can no longer sign in by any method.

---

## Configuration

The only operator knobs this surface adds are the image-size caps. They are **not secrets** — safe
dev defaults ship in `.env.example` and Compose passes them to the backend with the same defaults.

| Variable | What it is | Default |
|---|---|---|
| `ACCOUNT_AVATAR_MAX_KB` | Hard upper bound on an avatar upload, in KB. | `512` |
| `ACCOUNT_BANNER_MAX_KB` | Hard upper bound on a banner upload, in KB. | `1024` |

The **type allowlist** (`png`/`jpeg`/`webp`) is a code constant, not env — it is a security control,
not a deployment tuning knob.

To change a cap for a Compose deployment, set it in `deploy/.env` and recreate the backend:

```bash
docker compose -f deploy/docker-compose.yml up -d --force-recreate backend
```

Raise the caps cautiously: every byte is stored in Postgres and read into memory on upload, so larger
caps cost storage and per-request memory. The defaults are sized for typical avatars/banners.

---

## Schema

This surface adds a little state to the `cottonid` database (migration `0004_account_self_service`):

- **`users`** gains `banner_url`, plus the preference columns `pref_theme` (default `system`),
  `pref_lang` (default `ru`), and `login_notifications` (default `true`). (`avatar_url`, `about`,
  `location` already existed.)
- **`profile_images`** — bounded blob store keyed by `(user_id, kind)` with `content_type`, `bytes`,
  and `updated_at`. `user_id` references `users(id) ON DELETE CASCADE` so images vanish with the
  account.

See [RUNBOOK.md](RUNBOOK.md) §2 for how migrations are applied (automatically, on backend start).

---

## Deferred / known gaps

These are deliberate, documented limits of this surface (tracked for later changes):

- **Email change is deferred** — email is shown but not editable here, because changing it needs a
  verification flow and email delivery isn't wired yet ([SECURITY.md](SECURITY.md) §3, "Email
  delivery is stubbed").
- **TOTP is deferred** — the Security tab shows a 2FA "coming soon" seam; passkeys
  ([PASSKEYS.md](PASSKEYS.md)) are the strong-auth path today.
- **Login-notification emails are not sent yet** — the preference is stored, but the actual email on
  a new-device sign-in waits on the mailer (a later change). The toggle persists the user's intent.
- **Profile images live in Postgres, not an object store/CDN** — adequate at this scale; an object
  store is a later optimization, not a correctness gap.
