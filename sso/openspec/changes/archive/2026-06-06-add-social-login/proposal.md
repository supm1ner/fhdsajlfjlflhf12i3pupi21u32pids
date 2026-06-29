## Why

The cotton-id auth screens advertise social sign-in, and users expect to sign in with the identity providers they already have. Change 1 left `/api/v1/auth/social/{provider}/start` as a 501 stub. This change makes social login real for **Google, GitHub, VK (vk.com), and Yandex (ya.ru)** and removes the **Apple** button (Apple Sign-In needs a paid Apple Developer account and a client-secret-as-JWT flow — deferred, per the product decision).

Social login is a new authentication *method* that establishes a cotton-id session exactly like password login, so it slots into the existing Hydra login/consent handshake without changing it.

## What Changes

- Introduce a provider-agnostic **OAuth2 social connector** with concrete configs for Google (OIDC), GitHub, VK, and Yandex, each mapping the provider's userinfo to `{subject, email, email_verified, name, username, avatar}`.
- Implement `GET /api/v1/auth/social/{provider}/start` (replacing the 501 stub): create CSRF `state` (+ PKCE where supported), remember any in-progress `login_challenge`, and redirect to the provider's authorization URL.
- Implement `GET /api/v1/auth/social/{provider}/callback`: validate `state`, exchange the code for a token, fetch userinfo, **find-or-link** a cotton-id account by *verified* email (never link on an unverified email — account-takeover guard), establish a session, then continue the OIDC handshake (accept the carried `login_challenge`) or land on the SPA.
- Add a **`social_identities`** table (provider + provider subject ↔ cotton-id user) so a returning social user maps to the same account; allow a user to have multiple linked providers.
- Provider credentials (client id/secret/redirect) are configuration; when a provider is unconfigured the start endpoint returns a clear "provider not enabled" error and its button is hidden.
- Frontend: remove the Apple button; render Google, GitHub, VK, Yandex with provider branding; only show buttons for configured providers; wire them to the start endpoint. Fix the signup "Имя пользователя" column overflow.

This change is **additive**: it adds a new capability and a new auth method; no existing requirement is removed.

## Capabilities

### New Capabilities

- `auth-social`: Sign in / sign up with an external identity provider (Google, GitHub, VK, Yandex) via OAuth2/OIDC — the start + callback flow, verified-email account linking, the `social_identities` model, per-provider configuration, and the social-button UI.

### Modified Capabilities

<!-- None. Existing capabilities (password-authentication, oidc-provider) are unchanged; social login reuses the session + Hydra handshake. -->

## Impact

- **New code**: `internal/social` (connector + provider configs + handlers), a `social_identities` migration, frontend social UI; touches `internal/auth` only via the existing user store (find/create/link).
- **New endpoints**: `/api/v1/auth/social/{provider}/start` (was 501) and `/api/v1/auth/social/{provider}/callback`.
- **New dependencies / config**: per-provider `*_CLIENT_ID` / `*_CLIENT_SECRET` env vars; the providers' OAuth endpoints (no new Go modules required — net/http).
- **Security surface**: owns external-IdP token handling and account linking; the verified-email linking rule is the key control.
- **External setup**: real sign-in requires OAuth apps registered with each provider (documented); unconfigured providers degrade gracefully.
