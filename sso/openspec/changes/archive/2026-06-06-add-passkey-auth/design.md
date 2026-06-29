## Context

WebAuthn is a two-ceremony protocol (registration, authentication), each a begin (server issues a challenge + options) / finish (browser returns a signed response the server verifies) round-trip. cotton-id is the relying party (RP). Like social login, a successful passkey authentication just establishes a normal cotton-id session and can continue a Hydra `login_challenge`. We use `github.com/go-webauthn/webauthn`, which handles the protocol/crypto; cotton-id owns the user/credential storage and the HTTP surface.

## Goals / Non-Goals

**Goals:**
- Register a passkey for a signed-in user; sign in passwordlessly with it.
- Support discoverable credentials (usernameless) and resident-key UX, plus username-first.
- Continue an in-progress OIDC login via passkey.
- Manage (list/remove) one's passkeys; clone-detection via sign counter.

**Non-Goals:**
- Attestation verification against a metadata service (MDS) / enterprise attestation — accept self/none attestation (typical for consumer passkeys).
- Account *recovery* when all passkeys are lost (password reset already exists as the fallback).
- The full account-security UI (Change 4) — this change ships only a minimal management page.

## Decisions

### D1 — Library + RP configuration
Use `go-webauthn/webauthn`. RP config from env: `WEBAUTHN_RP_ID` (the registrable domain, e.g. `localhost` in dev, `id.example.com` in prod), `WEBAUTHN_RP_DISPLAY_NAME` ("cotton-id"), `WEBAUTHN_RP_ORIGINS` (CSV of allowed origins, e.g. `http://localhost:3000`). The RP ID MUST be a registrable suffix of every origin. Defaults derive from `FRONTEND_BASE_URL`/`PUBLIC_BASE_URL` for local dev.

### D2 — A `webauthn.User` adapter over the cotton-id account
Implement the library's `User` interface backing onto `auth.User` + the credential store: `WebAuthnID()` = the account UUID bytes, `WebAuthnName()` = username, `WebAuthnDisplayName()` = display name, `WebAuthnCredentials()` = the stored credentials. Credentials persist in `webauthn_credentials`.

### D3 — Ceremony state in a signed short-lived cookie
`webauthn.SessionData` (the per-ceremony challenge + expected params) is serialized into a signed, HttpOnly, SameSite=Lax `cid_wa` cookie (10-min TTL), mirroring the social `cid_oauth` pattern — no server-side ceremony table. Registration ceremonies also bind the cookie to the authenticated user id; login ceremonies need no prior session.

### D4 — Endpoints (all JSON, under /api/v1, CSRF-protected)
- `POST /passkeys/register/begin` (auth) → `CredentialCreationOptions` + set `cid_wa`. Excludes already-registered credentials.
- `POST /passkeys/register/finish` (auth) → verify attestation, store credential (with a user-supplied nickname), clear cookie.
- `GET /passkeys` (auth) → list the user's credentials (id, name, created, last used, transports).
- `DELETE /passkeys/{id}` (auth) → remove one of the user's credentials.
- `POST /auth/passkey/login/begin` `{email?, loginChallenge?}` → `CredentialRequestOptions` (allow-list when email given; discoverable when omitted) + set `cid_wa`.
- `POST /auth/passkey/login/finish` → verify assertion, update sign count (reject regressions as cloned), establish a session, and if a `loginChallenge` was carried, accept it via Hydra and return `{redirectTo}`; else `{user}`.

### D5 — Sign-count clone detection
On each authentication, compare the authenticator's returned sign count to the stored one; a non-increasing counter (when the authenticator uses counters) is logged as a security event and the authentication is refused (possible cloned credential), per WebAuthn guidance.

### D6 — Frontend
- Login screen: the passkey button calls `/auth/passkey/login/begin`, runs `navigator.credentials.get()` (WebAuthn JSON → ArrayBuffer encoding handled by a small helper or `@github/webauthn-json`), posts to `/finish`, then continues (loginChallenge → redirectTo, else home). Hidden/disabled when `window.PublicKeyCredential` is unavailable.
- A minimal `/passkeys` page (auth-gated): list, add (`navigator.credentials.create()` via register begin/finish), delete. Reachable from a post-login affordance; Change 4 folds it into account security.

## Risks / Trade-offs

- **RP ID / origin mismatch breaks ceremonies** → Mitigation: explicit env config + sane dev defaults; document the prod values; integration test asserts options carry the configured RP ID.
- **ArrayBuffer ↔ base64url encoding bugs** → Mitigation: use a vetted encoding helper (`@github/webauthn-json`) on the frontend; test the JSON shape.
- **Cookie-stored session data tampering** → Mitigation: HMAC-signed cookie (reuse the social state-codec approach), short TTL, HttpOnly.
- **Lost-all-passkeys lockout** → Mitigation: password + reset remain available; documented.

## Open Questions

- Whether to require user verification (UV) "required" vs "preferred" — default **preferred** (broad device support); make it config-tunable.
- Resident-key requirement — **preferred**, to enable usernameless login without forcing it on constrained authenticators.
