## 1. Data model & config

- [x] 1.1 Migration `0003_webauthn_credentials`: table (id uuid pk, user_id FK cascade, credential_id bytea unique, public_key bytea, attestation_type text, aaguid bytea, sign_count bigint, transports text[], name text, created_at, last_used_at) + index on user_id
- [x] 1.2 `internal/config`: `WEBAUTHN_RP_ID`, `WEBAUTHN_RP_DISPLAY_NAME`, `WEBAUTHN_RP_ORIGINS` (CSV); derive dev defaults from FRONTEND_BASE_URL host; validate RP id is a registrable suffix of each origin
- [x] 1.3 `go get github.com/go-webauthn/webauthn`; update `.env.example` (backend + deploy) + docker-compose with the new vars

## 2. Backend ceremonies (internal/passkey)

- [x] 2.1 `webauthnUser` adapter implementing the library's User interface over auth.User + the credential store
- [x] 2.2 Credential store (pgx): create, list-by-user, get-by-credential-id, delete (scoped to user), update sign count + last_used_at
- [x] 2.3 Signed `cid_wa` ceremony-state cookie codec (reuse the social HMAC state pattern; 10-min TTL, HttpOnly, Lax)
- [x] 2.4 Registration begin/finish (authenticated): exclude existing credentials; store credential with a nickname
- [x] 2.5 Login begin/finish: allow-list when email given, discoverable when omitted; verify assertion; **sign-count regression → refuse + security event**; establish session; continue login_challenge via Hydra
- [x] 2.6 List + delete handlers (scoped to the authenticated user)

## 3. HTTP wiring

- [x] 3.1 Routes under /api/v1 (CSRF group): `/passkeys/register/{begin,finish}`, `GET /passkeys`, `DELETE /passkeys/{id}`, `/auth/passkey/login/{begin,finish}`; swaggo annotations; wire in main.go; regenerate docs
- [x] 3.2 Auth-gate the register/list/delete routes (require an active session); login routes are pre-auth
- [x] 3.3 Structured security-event logging (register, login ok/fail, clone-detected, delete)

## 4. Frontend

- [x] 4.1 `@github/webauthn-json` (or a small base64url helper) for the navigator.credentials JSON encoding
- [x] 4.2 Login screen: make the passkey button functional (begin → navigator.credentials.get → finish → continue login_challenge or home); hide when `window.PublicKeyCredential` is unavailable
- [x] 4.3 Minimal `/passkeys` page (auth-gated): list, add (create ceremony), delete, with the cotton-id glass styling; a post-login affordance links to it
- [x] 4.4 Typed api.ts methods for all six endpoints; i18n strings (RU default)

## 5. Tests

- [x] 5.1 Unit: cookie codec, sign-count regression logic, RP-config validation, credential store scoping (a user cannot see/delete another's)
- [x] 5.2 Integration (Postgres testcontainer, //go:build integration): full register then login ceremony using the library's test/virtual authenticator helpers where feasible; cross-user delete refused
- [x] 5.3 Frontend: passkey button hidden without PublicKeyCredential; api.ts encoding shape

## 6. Docs & verification

- [x] 6.1 Extend docs/SECURITY.md (phishing-resistant auth, clone detection, lost-passkey fallback) + a short docs/PASSKEYS.md (RP config, prod origins)
- [x] 6.2 `go build/vet/test` + `gofmt` clean; frontend `tsc/build/test` clean; `docker compose config` clean
- [x] 6.3 Live smoke: register/begin returns options carrying the configured RP id; the passkey button renders on the login screen
