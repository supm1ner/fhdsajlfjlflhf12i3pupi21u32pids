## Context

Change 1 shipped password auth + the Hydra login/consent handshake and left social login as a wired-but-501 stub. Social login is just another way to authenticate a cotton-id account; once authenticated, the existing session + `POST /api/v1/oauth/login/accept` path drives the OIDC flow unchanged. Providers differ in protocol details (Google is full OIDC; GitHub/VK/Yandex are OAuth2 + a proprietary userinfo endpoint; VK returns the email in the token response, not userinfo), so the design centers on a small connector abstraction with per-provider adapters.

## Goals / Non-Goals

**Goals:**
- Real sign-in/sign-up via Google, GitHub, VK, Yandex.
- Safe account linking: a social identity links to an existing cotton-id account **only** when the provider asserts a *verified* email that matches; otherwise a new account is created (or, when no email is available, a synthetic account keyed on provider+subject).
- Continue an in-progress OIDC `login_challenge` after social auth.
- Graceful degradation: unconfigured providers are hidden in the UI and rejected by the API with a clear error.
- Remove Apple; fix the signup column overflow.

**Non-Goals:**
- Apple Sign-In (deferred — paid account + JWT client secret).
- Account *unlinking* UI and managing multiple linked providers from the account screen (that lands with account self-service, change 4); the data model supports it now.
- Token refresh / calling provider APIs after login (we only need identity at sign-in).

## Decisions

### D1 — Provider-agnostic connector with per-provider adapters
A `Provider` describes `authURL`, `tokenURL`, `userInfoURL`, `scopes`, and a `mapUserInfo(token, raw) -> Identity` function. `Identity = {Subject, Email, EmailVerified, Name, Username, AvatarURL}`. Google/GitHub/Yandex map from userinfo JSON; VK reads `email` from the token response and profile from `users.get`. Adding a provider later = one adapter + config.
- **Why:** isolates provider quirks; keeps the handler generic and testable with `httptest` fakes.

### D2 — State + PKCE, signed and short-lived
`start` generates a random `state` and (for providers that support it) a PKCE verifier, stores them plus the optional `login_challenge` and `remember` in a short-lived, HttpOnly, signed `cid_oauth` cookie (or server-side row), and redirects. `callback` validates `state` against the cookie (CSRF for the OAuth redirect), then clears it. State TTL ~10 min.
- **Alternative:** server-side state table — heavier; the signed cookie is sufficient and stateless. Use an HMAC with the existing session/CSRF key material.

### D3 — Verified-email account linking (the security crux)
On callback: look up `social_identities(provider, subject)`. If found → that user. Else, if the provider asserts `email_verified=true` and a user with that email exists → **link** (insert `social_identities`) and sign in. Else if a verified email with no existing user → create a new account (username derived + uniquified) and link. If the email is **unverified** → never auto-link to an existing account; create a separate account keyed on provider+subject (email stored but not trusted) to avoid takeover. GitHub: fetch `/user/emails` and use the primary *verified* address. VK: email only present if the `email` scope was granted and the user consented.
- **Why:** linking on an unverified email is the classic social-login account-takeover vector.

### D4 — Session establishment + OIDC continuation
After resolving the user, mint a normal cotton-id session (reuse `auth` session store + cookie helper) and:
- if `login_challenge` was carried → accept it via Hydra and 302 the browser to Hydra's `redirect_to` (completes the RP flow);
- else → 302 to the SPA (`FRONTEND_BASE_URL`), landed and authenticated.

### D5 — Configuration & graceful degradation
Per provider: `SOCIAL_<P>_CLIENT_ID`, `SOCIAL_<P>_CLIENT_SECRET` (+ Yandex/VK any extra). The backend exposes `GET /api/v1/auth/social/providers` → the list of *enabled* providers (those with credentials). The SPA renders only enabled providers. `start`/`callback` for a disabled provider return `400 provider not enabled`. Redirect URI is derived from `PUBLIC_BASE_URL` + `/api/v1/auth/social/{provider}/callback`.

### D6 — Frontend
`SocialRow` fetches enabled providers, drops Apple, and renders Google/GitHub/VK/Yandex with brand marks; clicking navigates to `start`. Signup column overflow fixed with `minWidth:0` + flex-wrap (already applied).

## Risks / Trade-offs

- **Provider API drift (esp. VK ID vs legacy VK OAuth)** → Mitigation: implement against the documented current endpoints, isolate per-provider code, cover mapping with unit tests; the implementer verifies exact endpoints from provider docs.
- **Unverified-email takeover** → Mitigation: D3 (never link unverified).
- **Open redirect via `login_challenge`/return URL** → Mitigation: `redirect_to` comes only from Hydra; the SPA return is a fixed config URL; `state` cookie is signed.
- **Cannot live-test without real provider apps** → Mitigation: full unit/integration coverage with fake provider servers; documented credential setup; unconfigured providers degrade.

## Open Questions

- Username collision strategy when deriving from provider profile (use provider username, else email local-part, append a short suffix on collision) — settled: suffix on collision.
- Whether to store provider avatar now (yes, on the user row / identity, surfaced by account self-service later).
