## 1. Data model

- [x] 1.1 Migration `0004_account_self_service`: add `banner_url text`, `pref_theme text default 'system'`, `pref_lang text default 'ru'`, `login_notifications boolean default true` to users; create `profile_images(user_id, kind, content_type, bytes bytea, updated_at, primary key(user_id, kind))`
- [x] 1.2 Extend `auth.UserStore` / a profile store with: UpdateProfile, UpdatePreferences, GetFull (incl. new fields); image store (upsert/get)

## 2. Backend — profile & preferences

- [x] 2.1 `internal/account`: service over user store + session store + passkey store (list) + Hydra client + image store
- [x] 2.2 `GET /api/v1/account` (full profile incl. prefs, counts) ; `PATCH /api/v1/account` (display name/about/location, validated)
- [x] 2.3 Avatar/banner: `PUT /api/v1/account/images/{kind}` multipart upload (type allowlist png/jpeg/webp, size cap, magic-byte check) ; `GET /api/v1/account/images/{kind}` serve ; set avatar_url/banner_url
- [x] 2.4 `PATCH /api/v1/account/preferences` (theme/lang/login_notifications)

## 3. Backend — security & lifecycle

- [x] 3.1 `PUT /api/v1/account/password` — verify current password, enforce policy, rehash, revoke other sessions
- [x] 3.2 `GET /api/v1/account/sessions` (mark current via hashed cookie) ; `DELETE /api/v1/account/sessions/{id}` (scoped) ; `DELETE /api/v1/account/sessions` (revoke others)
- [x] 3.3 `GET /api/v1/account/connections` (Hydra consent sessions by subject) ; `DELETE /api/v1/account/connections/{client}` (revoke)
- [x] 3.4 `DELETE /api/v1/account` — re-auth (current password / confirmation), cascade delete + best-effort Hydra session revoke + audit log
- [x] 3.5 Hydra client: add ListConsentSessions(subject), RevokeConsentSessions(subject, client), RevokeLoginSessions(subject)
- [x] 3.6 Wire all routes in main.go (CSRF group, auth-gated); swaggo; regenerate docs

## 4. Frontend — Account screen

- [x] 4.1 `routes/Account.tsx` at `/account` (auth-gated; redirect to /login if not), porting `_design_ref/screen-account.jsx` layout with the glass kit, RU/EN
- [x] 4.2 Sections: Profile (edit + avatar/banner upload), Security (password change, 2FA "coming soon" seam, Passkeys via the Change 3 API, Sessions list+revoke), Connected services (list+revoke), Settings (theme/lang/login-notifications), Danger zone (delete with double-confirm)
- [x] 4.3 Typed api.ts methods for all account endpoints; image upload handling; preference sync on load (server prefs override localStorage when signed in)
- [x] 4.4 A header/menu affordance linking to /account and logout

## 5. Tests

- [x] 5.1 Unit: profile validation, password-change (wrong current rejected; others revoked), image type/size validation, preferences update, session current-flagging, delete re-auth
- [x] 5.2 Integration (Postgres testcontainer): profile/prefs/image store; sessions list/revoke scoping; account delete cascade; (Hydra consent list/revoke behind integration tag against running Hydra)
- [x] 5.3 Frontend: Account screen renders sections; edit profile; sessions list; delete double-confirm gating

## 6. Docs & verification

- [x] 6.1 Update README/RUNBOOK (account features) ; SECURITY.md (re-auth for password/delete, session/consent revocation, image upload limits)
- [x] 6.2 `go build/vet/test` + gofmt clean; frontend tsc/build/test clean; compose config clean
- [x] 6.3 Live smoke: sign in → GET /account → edit profile → change password → list sessions → list connections → delete a test account
