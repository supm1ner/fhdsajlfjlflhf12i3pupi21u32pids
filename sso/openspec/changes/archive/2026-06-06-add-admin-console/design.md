## Context

Roles and statuses already exist on `users`. What's missing is (a) a session-based authorization gate for human admins (the existing `X-Admin-Key` is for machine client-registration only), (b) admin queries over users + aggregate stats, and (c) a persistent audit log to back the Journal — security events currently only reach stdout. This change adds all three plus the admin UI shell.

## Goals / Non-Goals

**Goals:**
- Role-gated admin APIs + a working Overview, Users (list + detail + actions), and Journal.
- A persistent audit log the whole backend appends to, queryable by the Journal.
- Privilege-escalation-safe lifecycle actions.

**Non-Goals:**
- The Services tab / OAuth client registration UI (Change 6).
- Real "message user" email (stub; needs the mailer, Change 7).
- Deep analytics/BI — Overview uses simple SQL aggregates.

## Decisions

### D1 — Session-based RBAC middleware
A `RequireRole(min)` middleware resolves the session user (via `auth.Service.UserForSession`) and checks the role rank (`user` < `admin` < `owner`). `/api/v1/admin/*` (the human console API) requires `admin`. Owner-only actions (delete, granting admin/owner, acting on an owner) check `owner` in the handler. This is separate from the machine `X-Admin-Key` group used by `/api/v1/admin/clients` (Change 1) — to avoid confusion, the human console API is mounted at `/api/v1/admin` with the role gate, and the existing key-gated client routes remain (Change 6 may re-expose them through the role gate for the UI).

### D2 — Persistent audit log
A `audit_log` table (`id, ts, actor_id, actor_label, action, target_type, target_id, ip, request_id, metadata jsonb`). An `audit.Writer` appends asynchronously-but-reliably (synchronous insert in-request is fine at this scale; failures are logged, never block the user action). Refactor the existing security-event log points (login ok/fail, signup, reset, consent, client reg, plus new admin actions) to ALSO call the audit writer. The Journal reads with filters (actor, action, time range) + pagination.
- **Trade-off:** synchronous insert adds a tiny latency to security events; acceptable and simplest. A buffered async writer is a later optimization.

### D3 — Overview aggregates
Simple SQL: total users, active-today (sessions/last activity today), new-this-week (created_at), services count (Hydra client count), a 30-day daily sign-up series (`GROUP BY date_trunc('day', created_at)`), recent sign-ups (latest N users), recent activity (latest N audit rows).

### D4 — User listing
`GET /admin/users?query=&status=&role=&page=&pageSize=` — case-insensitive search over username/displayName/email (citext), status/role filters, keyset or offset pagination, returning the projection the table needs (incl. a per-user connected-services count). `GET /admin/users/{id}` returns detail (profile, sessions, recent activity from audit, connected services, counts).

### D5 — Lifecycle actions, escalation-safe
- Suspend/reactivate: set status; suspending revokes the target's sessions. Cannot suspend yourself or an owner (unless you are owner and target isn't the last owner).
- Change role: only `owner` may grant/revoke `admin`/`owner`; cannot demote the last owner; cannot escalate yourself.
- Force password reset: issue a reset token + (stub) email; logs the event.
- Delete: `owner`-only; cannot delete yourself or the last owner; cascades (sessions/passkeys/social/images) + best-effort Hydra revoke.
Every action writes an audit entry with actor + target.

### D6 — Frontend admin shell
Port `_design_ref/screen-admin.jsx`: the dark glass console with the sidebar (Overview, Users, Services, Journal, Settings), top search, and the screens. Route `/admin` (+ subroutes). Auth-gated AND role-gated client-side (fetch `/api/v1/auth/session`; redirect non-admins). Overview (stat cards + sign-up chart + recent lists), Users (filterable table + the per-user detail card with the action buttons + confirmations), Journal (audit table with filters). Services + Settings are placeholders that Change 6 / later fills. Reuse the glass kit + RU/EN.

## Risks / Trade-offs

- **Privilege escalation** → Mitigation: role-rank checks server-side on every action; owner-only guards; last-owner and self-action guards; all audited. The client-side gate is UX only — the server is authoritative.
- **Audit write failure masking actions** → Mitigation: the user action still succeeds; audit failure is logged at error; consider a fallback. Audit is append-only (no update/delete via API).
- **Expensive aggregate queries at scale** → Mitigation: indexes on created_at/status/role; the 30-day series is bounded; revisit with materialized views if needed.
- **Search injection** → Mitigation: parameterized queries; `query` is a bound parameter used with ILIKE/citext.

## Open Questions

- Keyset vs offset pagination — default offset for simplicity now; keyset if the table grows large.
- Whether "active today" is based on session last-seen or audit logins — use audit login events (more precise) with a sessions fallback.
