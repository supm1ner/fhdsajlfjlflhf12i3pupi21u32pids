## 1. Data model & config

- [x] 1.1 Migration `0002_social_identities`: table `social_identities` (id, user_id FK, provider, provider_subject, email, created_at; unique (provider, provider_subject)); add nullable `avatar_url` to users
- [x] 1.2 `internal/config`: per-provider `SOCIAL_<P>_CLIENT_ID` / `SOCIAL_<P>_CLIENT_SECRET` (google, github, vk, yandex); derive redirect URIs from `PUBLIC_BASE_URL`; a provider is "enabled" iff id+secret are set
- [x] 1.3 Update `.env.example` (backend + deploy) + docker-compose with the new vars (commented, empty by default)

## 2. Social connector (backend)

- [x] 2.1 `internal/social`: `Provider` type (authURL, tokenURL, userInfoURL, scopes, mapUserInfo) + `Identity{Subject,Email,EmailVerified,Name,Username,AvatarURL}`
- [x] 2.2 Provider adapters — Google (OIDC userinfo), GitHub (`/user` + `/user/emails` primary verified), VK (email from token response + `users.get`), Yandex (`login.yandex.ru/info`); RESEARCH exact current endpoints/params from each provider's docs before coding
- [x] 2.3 State/PKCE: signed short-lived `cid_oauth` cookie carrying state, PKCE verifier, optional login_challenge, remember; HMAC with config key
- [x] 2.4 Token exchange + userinfo fetch over net/http with bounded timeouts and provider-specific quirks

## 3. Account resolution & handlers

- [x] 3.1 `internal/social`: account resolver — find by (provider,subject) → link by verified email → create new (uniquified username) → never link on unverified email (D3)
- [x] 3.2 Extend `internal/auth` user store with `GetByEmail`/`Create`/link helpers as needed (no behavior change to existing flows)
- [x] 3.3 Handler `GET /api/v1/auth/social/{provider}/start` (replace the 501 stub in internal/auth): build state, redirect; reject unconfigured providers
- [x] 3.4 Handler `GET /api/v1/auth/social/{provider}/callback`: validate state, exchange, resolve account, establish session, accept login_challenge or redirect to SPA; security-event logging
- [x] 3.5 Handler `GET /api/v1/auth/social/providers` → enabled provider list; wire all routes in main.go; swaggo annotations; regenerate docs

## 4. Frontend

- [x] 4.1 `SocialRow`: remove Apple; fetch `/auth/social/providers` and render only enabled ones (Google/GitHub/VK/Yandex) with brand marks; navigate to `start`
- [x] 4.2 Confirm the signup "Имя пользователя" column overflow is fixed (minWidth:0 + flex-wrap) on Login & Signup
- [x] 4.3 Update i18n if any new copy; keep RU default

## 5. Tests

- [x] 5.1 Unit: each provider's `mapUserInfo` (incl. GitHub primary-verified email, VK email-in-token, unverified-email path)
- [x] 5.2 Unit/handler: state validation, unconfigured-provider rejection, account resolver (link/create/no-link-on-unverified) with httptest fake providers
- [x] 5.3 Frontend: SocialRow renders enabled providers, omits Apple, omits disabled

## 6. Docs & verification

- [x] 6.1 `docs/SOCIAL_LOGIN.md` (or extend RUNBOOK): how to register OAuth apps with Google/GitHub/VK/Yandex and set credentials; redirect URIs
- [x] 6.2 `go build/vet/test` + `gofmt` clean; frontend `tsc/build/test` clean; `docker compose config` clean
- [x] 6.3 Live smoke: `providers` endpoint reflects configured set; an enabled provider's `start` 302s to the provider; adversarial review of linking/state
