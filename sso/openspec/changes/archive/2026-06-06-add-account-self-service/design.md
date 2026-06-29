## Context

Most of the data already exists: `users` (profile, status, role, avatar_url), `sessions` (devices), `webauthn_credentials` (passkeys), `social_identities`, and Hydra's consent grants. This change is mostly a self-service HTTP + UI layer over that data, plus a little new state (banner, preferences, avatar/banner blobs). All endpoints require the current session (reusing `auth.Service.UserForSession`) and are in the CSRF group.

## Goals / Non-Goals

**Goals:**
- A working Account screen matching the prototype: Profile, Security, Sessions, Connected services, Settings, Danger zone.
- Edit profile text + upload avatar/banner; change password with re-auth; list/revoke sessions; list/revoke connected apps; persist preferences; delete account.

**Non-Goals:**
- Email change + re-verification (deferred — no email delivery yet).
- TOTP enrollment (the 2FA UI is a "coming soon" seam; the schema seam already exists).
- An object store / CDN for images (small blobs in Postgres are sufficient at this scale).

## Decisions

### D1 — `internal/account` package over the existing stores
A thin service composing the user store, session store, passkey store (list only, for the security overview), Hydra client (consent grants), and an image store. Handlers mount under `/api/v1/account`.

### D2 — Avatar/banner as bounded blobs in Postgres
Uploads are `multipart/form-data`, validated to `image/png|jpeg|webp`, max ~512 KB (avatar) / ~1 MB (banner), stored in a `profile_images` table (`user_id`, `kind`, `content_type`, `bytes`, `updated_at`) and served from `GET /api/v1/account/images/{kind}` (or a public `/users/{id}/avatar` for cross-app display later). `users.avatar_url`/`banner_url` hold the served URL. No volume/object-store needed; the size cap keeps rows small.
- **Alternative:** local volume or S3 — deferred; Postgres blobs are adequate and keep deploy simple.

### D3 — Password change re-authenticates and revokes other sessions
`PUT /account/password` requires the current password (verified via the password authenticator), enforces the password policy on the new one, rehashes, and **revokes all other sessions** (keeping the current one), mirroring reset semantics minus the token.

### D4 — Sessions: reuse the `sessions` table; mark current; revoke by id
`GET /account/sessions` lists the user's sessions (device/UA, ip, created, last-seen, expiry) with the current one flagged (matched by hashing the request's session cookie). `DELETE /account/sessions/{id}` deletes one (scoped to the user); a "revoke all others" convenience deletes all but the current.

### D5 — Connected apps = Hydra consent grants
`GET /account/connections` lists the user's consent sessions from Hydra's admin API (`/admin/oauth2/auth/sessions/consent?subject=<userID>`), projected to `{client, grantedScopes, grantedAt}`. `DELETE /account/connections/{client}` revokes that client's grants for the subject (Hydra admin revoke endpoint). This is the user-facing half of consent management; the admin/RP-registration half is Change 6.

### D6 — Preferences persisted server-side
Add `pref_theme` (dark|light|system), `pref_lang` (ru|en), `login_notifications` (bool) to `users`. `PATCH /account/preferences` updates them; the SPA reads them on load so theme/lang sync across devices (falling back to localStorage when unauthenticated). Login-notification email is a stored preference; actual emails wait on the mailer (Change 7).

### D7 — Account deletion cascades
`DELETE /account` requires re-auth (current password, or a confirmation for social-only accounts). It deletes the user row (FK cascades remove sessions, passkeys, social identities, profile images) and best-effort revokes the subject's Hydra login/consent sessions. Irreversible; the UI double-confirms.

### D8 — Frontend Account screen
Port the prototype's account layout (`_design_ref/screen-account.jsx`) into a typed React screen at `/account` with the sections above, reusing the glass UI kit, the passkey management (Change 3) for the Passkeys subsection, RU/EN, and the typed api client. Auth-gated (redirect to /login when unauthenticated).

## Risks / Trade-offs

- **Image upload abuse (size/type/decompression)** → Mitigation: strict content-type allowlist, hard size cap, decode-and-re-encode or at least validate magic bytes; per-user single avatar/banner (overwrite).
- **Revoking the current session by accident** → Mitigation: the current session is flagged and "revoke" on it logs the user out intentionally; "revoke others" excludes it.
- **Hydra consent API shape drift** → Mitigation: isolate in the Hydra client; tolerate missing fields; integration-test against the running Hydra.
- **Account deletion is destructive** → Mitigation: re-auth + double confirm + audit log; FK cascade keeps it consistent.

## Open Questions

- Whether to expose a public avatar URL now (for cross-app display) or keep it auth-gated until a later change — default: serve avatar at an auth-gated account route now, public route later.
- Email change flow — deferred to when email delivery lands.
