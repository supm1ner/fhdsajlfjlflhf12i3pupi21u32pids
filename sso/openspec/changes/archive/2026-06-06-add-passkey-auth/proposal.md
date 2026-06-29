## Why

The cotton-id design and security promise feature **passkeys** ("вход без пароля по биометрии") as a first-class, phishing-resistant sign-in method, and the login screen already shows a (disabled) passkey button. This change makes passkeys real: users can register a WebAuthn credential and sign in passwordlessly. Passkeys are the strong-authentication story that offsets the deliberately-deferred TOTP (Change 1 left password-only accounts single-factor).

## What Changes

- Add WebAuthn (FIDO2) support via the maintained `github.com/go-webauthn/webauthn` library, configured as the cotton-id relying party.
- **Registration** (authenticated): begin/finish ceremony endpoints that create a credential for the current user; store credentials in a `webauthn_credentials` table.
- **Passwordless login**: begin/finish ceremony endpoints that verify an assertion and establish a session, supporting both username-first and discoverable (usernameless) credentials, and continuing an in-progress Hydra `login_challenge` like password/social login.
- **Management**: list and delete the current user's passkeys.
- Ceremony challenge state is carried in a short-lived signed `cid_wa` cookie (no server-side ceremony table needed).
- Frontend: the login screen's passkey button becomes functional; a minimal **Passkeys** management page lets a signed-in user register/remove credentials (the full account-security UI lands in account self-service, Change 4, reusing this API).

This change is **additive**: a new authentication method and capability; nothing existing is removed.

## Capabilities

### New Capabilities

- `auth-passkey`: Register and use WebAuthn passkeys — the registration and passwordless-login ceremonies, credential storage and management, relying-party configuration, sign-count clone-detection, and the passkey UI.

### Modified Capabilities

<!-- None. Reuses the session + Hydra handshake from earlier changes unchanged. -->

## Impact

- **New dependency**: `github.com/go-webauthn/webauthn`.
- **New code**: `internal/passkey` (ceremonies + store + handlers), a `webauthn_credentials` migration, frontend passkey login + a management page.
- **New endpoints**: `/api/v1/passkeys/register/{begin,finish}`, `/api/v1/passkeys` (GET), `/api/v1/passkeys/{id}` (DELETE), `/api/v1/auth/passkey/login/{begin,finish}`.
- **New config**: `WEBAUTHN_RP_ID`, `WEBAUTHN_RP_DISPLAY_NAME`, `WEBAUTHN_RP_ORIGINS` (defaults derived for local dev).
- **Security**: phishing-resistant auth; sign-count regression is treated as a cloned-authenticator signal.
