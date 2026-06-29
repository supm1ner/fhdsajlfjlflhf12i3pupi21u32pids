## Why

The admin console's **Services** tab is where operators register and manage the relying-party apps (Vault, Mailbox, Studio, Cloud, Stream…) that authenticate through cotton-id. Change 1 shipped OAuth client CRUD, but only behind the machine `X-Admin-Key`; the human console (Change 5) is session+role gated. This change exposes client management through the **role-gated** admin API so operators can run it from the UI, builds the Services tab, and surfaces each client's live consent/usage so the registry is observable.

## What Changes

- **Role-gated client management API**: `GET/POST/GET{id}/PATCH/DELETE /api/v1/admin/clients*` behind `RequireRole(admin)` (session-based), wrapping the existing Hydra client CRUD. The legacy `X-Admin-Key` route stays for machine/automation use; the console uses the session-gated routes.
- **Edit** a client (name, redirect URIs, allowed scopes, grant/response types) — not just create/delete.
- **Per-client overview**: how many users have granted it consent (via Hydra consent sessions), so an operator can see usage and (optionally) revoke all grants for a client.
- **Services tab UI**: list registered clients with type (public/confidential) and redirect URIs; create (with the generated secret shown once for confidential clients), edit, delete (with confirm), and view per-client consent counts — replacing the Change 5 placeholder.

This change is **additive**: new role-gated endpoints + the Services UI; the Change 1 key-gated routes are unchanged.

## Capabilities

### New Capabilities

- `client-consent-management`: Operator management of OAuth relying-party clients through the role-gated console — list/create/edit/delete clients, view per-client consent usage, and revoke a client's grants.

### Modified Capabilities

<!-- None at the requirement level. Reuses the Change 1 Hydra client CRUD and the Change 5 RBAC + audit. -->

## Impact

- **New code**: session+role-gated client handlers in `internal/admin` (reusing `oidc.HydraClient` client CRUD + adding Update + a consent-count query), the React Services tab.
- **New endpoints**: `GET/POST /api/v1/admin/clients` (session+role), `GET/PATCH/DELETE /api/v1/admin/clients/{id}`, `GET /api/v1/admin/clients/{id}/consents` (count), `DELETE /api/v1/admin/clients/{id}/consents` (revoke all).
- **Hydra**: add `GetClient`, `UpdateClient`, and a consent-count read to the hand-written client.
- **Security**: client registration/edit/delete are owner/admin-gated and audited; the one-time client secret is shown only on create/rotate and never re-served.
