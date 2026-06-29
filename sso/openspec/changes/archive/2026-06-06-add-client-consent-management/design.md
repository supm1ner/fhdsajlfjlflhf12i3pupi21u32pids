## Context

Change 1 implemented OAuth client CRUD over Hydra's admin API, exposed at `/api/v1/admin/clients` behind the machine `X-Admin-Key`. Change 5 added a session-based RBAC gate (`RequireRole`) and the admin console shell with a Services placeholder. This change connects the two: client management through the role gate, plus edit + consent visibility, plus the Services UI.

## Goals / Non-Goals

**Goals:**
- Manage relying-party clients from the console (list/create/edit/delete) under the session role gate, audited.
- Show per-client consent usage; allow revoking a client's grants.
- Show a confidential client's secret exactly once on create/rotate.

**Non-Goals:**
- Dynamic client registration (RFC 7591) for third parties — this is operator-only.
- Fine-grained per-scope policy beyond what Hydra models.
- Rotating client secrets UI beyond a basic "regenerate" (optional).

## Decisions

### D1 — Session+role-gated routes at /admin/services (distinct from the machine /admin/clients)
Mount the console client management under `/api/v1/admin/services` (RequireRole(admin), CSRF, session) — a DIFFERENT path from the machine `/api/v1/admin/clients` (X-Admin-Key), because chi forbids two route registrations at the same method+path and the two need different auth. "Services" also matches the prototype tab name. The machine `/admin/clients` route stays for automation/seed; the console uses `/admin/services`. Both call the same `oidc.HydraClient` CRUD, so behavior is consistent. (NOTE: this supersedes the proposal/spec's earlier `/admin/clients` console path to avoid the router collision.)

### D2 — Add Update + Get to the Hydra client
Extend `oidc.HydraClient` with `GetClient(id)` and `UpdateClient(id, OAuth2Client)` (Hydra `GET/PUT /admin/clients/{id}`). Edit replaces name/redirect URIs/scopes/grant+response types; client type changes are constrained (changing public↔confidential affects auth method + secret — handle explicitly, document).

### D3 — Per-client consent usage
A count of consent sessions for a client. Hydra's admin API lists consent sessions by subject, not by client, so a direct per-client count needs either: (a) iterating, or (b) Hydra's client-scoped consent endpoints if available. Decision: expose `GET /clients/{id}/consents` returning a best-effort count (and `DELETE` to revoke all of that client's grants via Hydra's `DELETE /admin/oauth2/auth/sessions/consent?client={id}&all=true` if supported); if Hydra lacks an efficient per-client count, return the count it can give and document the limitation. The implementer verifies the available Hydra endpoints.

### D4 — Secret handling
On create (confidential) or regenerate, Hydra returns the secret once; the API returns it once and never stores or re-serves it (the UI shows a copy-once panel). Public (PKCE) clients have no secret.

### D5 — Services tab UI
Replace the Change 5 placeholder: a table of clients (name, type, redirect URIs, scopes, consent count), a create form (name, type, redirect URIs, scopes, grant/response types) that surfaces the one-time secret, an edit form, and delete (confirm). All calls go to the session-gated admin client routes; actions are audited server-side.

## Risks / Trade-offs

- **Hydra per-client consent query gaps** → Mitigation: implement what Hydra supports; degrade to a documented best-effort count; isolate in the client.
- **Editing a client breaking a live RP** → Mitigation: confirmations + the audit trail; redirect-URI validation (absolute, no fragment) reused from Change 1.
- **Secret exposure** → Mitigation: secret returned once, never logged, never re-served; transport is the same-origin authenticated console.
- **public↔confidential type change** → Mitigation: validate + document; default to disallowing a silent type flip (require explicit intent).

## Open Questions

- Whether to support client-secret rotation now or defer — default: include a simple "regenerate secret" for confidential clients if Hydra exposes it; else defer.
- Per-client consent count efficiency at scale — revisit if the deployment grows many clients/subjects.
