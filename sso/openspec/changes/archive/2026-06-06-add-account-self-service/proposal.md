## Why

The prototype's **Account** surface lets a signed-in user manage their own identity — profile, security, sessions, connected apps, preferences, and account deletion. Change 1 shipped the data (users, sessions) and later changes added passkeys and social identities; this change exposes the self-service UI + APIs so users can actually manage all of it. It also realizes the design's "видьте каждый вход, каждое устройство и каждое разрешение в одном месте" (see every login, device and permission in one place).

## What Changes

- **Profile**: view and edit display name, about, location, avatar and banner images; username and email are shown (email change deferred — needs verification). Avatar/banner are uploaded (size + type bounded) and served by the backend.
- **Security**: change password (re-auth with current password, revokes other sessions); manage **passkeys** (reuse the Change 3 API); **active sessions** list with the current device marked and per-session revoke; a TOTP **2FA seam** rendered as "coming soon" (TOTP itself stays deferred).
- **Connected apps**: list the OAuth consent grants the user has given relying parties (via Hydra) and **revoke** any of them.
- **Preferences**: theme, language, and login-notification toggle, persisted server-side so they sync across devices.
- **Danger zone**: delete account (re-auth required), cascading the user's sessions, passkeys, social identities, and Hydra consent/login sessions.

This change is **additive**: a new self-service capability + endpoints + a few user columns; existing auth flows are unchanged.

## Capabilities

### New Capabilities

- `account-self-service`: A signed-in user managing their own account — profile (incl. avatar/banner), password change, active-session listing/revocation, connected-app (consent-grant) listing/revocation, preferences, and account deletion.

### Modified Capabilities

<!-- None at the requirement level; reuses sessions (Change 1), passkeys (Change 3), social identities (Change 2), and the Hydra consent API. -->

## Impact

- **New code**: `internal/account` (profile/preferences/sessions/connections/deletion handlers + store), avatar/banner storage, the React Account screen and its sections.
- **New endpoints**: `GET/PATCH /api/v1/account`, `PUT /api/v1/account/password`, `GET/DELETE /api/v1/account/sessions[/{id}]`, `GET/DELETE /api/v1/account/connections[/{client}]`, `PATCH /api/v1/account/preferences`, avatar/banner upload + serve, `DELETE /api/v1/account`.
- **Schema**: add `banner_url` and preference columns (`pref_theme`, `pref_lang`, `login_notifications`) to users (or a `user_preferences` row); an avatar/banner blob store.
- **Hydra**: read + revoke the user's consent sessions via the admin API.
- **Security**: re-authentication for password change + account deletion; session revocation; consent revocation.
