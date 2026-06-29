## 1. Hydra client extensions

- [x] 1.1 `oidc.HydraClient`: add `GetClient(id)`, `UpdateClient(id, OAuth2Client)` (Hydra GET/PUT /admin/clients/{id}); verify the per-client consent endpoints Hydra v2 exposes (count + revoke-by-client) and add the supported calls
- [x] 1.2 Reuse the Change 1 redirect-URI validation + public/confidential auth-method mapping; constrain client-type changes (explicit, documented)

## 2. Role-gated console client endpoints (internal/admin)

- [x] 2.1 `GET /api/v1/admin/clients` + `POST` (RequireRole admin) — list + create (return secret once for confidential); audited
- [x] 2.2 `GET /api/v1/admin/clients/{id}` (detail) + `PATCH` (edit name/redirects/scopes/grants) + `DELETE` (confirm) — audited
- [x] 2.3 `GET /api/v1/admin/clients/{id}/consents` (best-effort usage count) + `DELETE /api/v1/admin/clients/{id}/consents` (revoke all) — audited
- [x] 2.4 swaggo annotate; wire in main.go behind the session role gate (keep the legacy X-Admin-Key route from Change 1 intact); regenerate docs

## 3. Frontend — Services tab

- [x] 3.1 Replace the Change 5 Services placeholder: a clients table (name, type, redirect URIs, scopes, consent count) from GET /admin/clients
- [x] 3.2 Create form (name, type, redirect URIs[], scopes[], grant/response types) → POST; show the one-time secret in a copy panel for confidential clients
- [x] 3.3 Edit form (PATCH) + delete (confirm) + revoke-consents action; per-client consent count display; RU/EN; glass styling
- [x] 3.4 Typed api.ts methods + types

## 4. Tests

- [x] 4.1 Unit: client request validation (redirect URIs, type), secret-shown-once projection, role-gate on the routes, Hydra client request building (httptest)
- [x] 4.2 Integration (//go:build integration, running Hydra): create → list → get → edit → delete round-trip; consent count/revoke best-effort
- [x] 4.3 Frontend: Services tab renders clients; create shows secret once; non-admin blocked

## 5. Docs & verification

- [x] 5.1 docs: extend ADMIN.md (Services tab, client lifecycle, secret handling) + RUNBOOK (register/edit/delete a relying party from the console) + SECURITY.md (role-gated client mgmt, secret-once, audit)
- [x] 5.2 `go build/vet/test` + gofmt clean; frontend tsc/build/test clean; compose config clean
- [x] 5.3 Live smoke: admin creates a client via the console route (secret returned once); list shows it; an authorization_code flow with that client still works; delete removes it
