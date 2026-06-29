## Why

The prototype's **Console** (admin) surface is where operators run cotton-id: see growth at a glance, manage every account, audit what happened, and configure the system. Change 1 modeled roles (`user`/`admin`/`owner`) and statuses (`active`/`invited`/`suspended`) but exposed no admin API or UI. This change builds the admin console and the role-gated APIs behind it, and introduces a **persistent audit log** so the Journal tab is real (today's security events only go to stdout).

## What Changes

- **RBAC**: enforce that only `admin`/`owner` accounts may reach `/api/v1/admin/*`; `owner` is required for the most dangerous actions (delete, role changes to admin/owner).
- **Persistent audit log**: an `audit_log` table and an audit sink the existing security-event logging also writes to (login, signup, reset, consent, client reg, admin actions), queryable for the Journal.
- **Overview**: aggregate stats (total accounts, active today, new this week, registered services) + a 30-day sign-up series + recent sign-ups + a recent activity feed.
- **User management**: list users with search + status/role filters + pagination; per-user detail (profile, sessions, recent activity, connected services counts); actions — **suspend/reactivate**, **change role**, **force password reset** (issues a reset token / link), **delete**, and (stubbed) "message".
- **Journal**: a filterable view of the audit log (actor, action, target, time, ip).
- **Settings**: a minimal admin settings surface (read-only system info + the safe toggles available).
- **Frontend**: the admin shell (sidebar Overview/Users/Services/Journal/Settings, search, the dark glass console design) with the Overview, Users (table + detail), and Journal screens wired to the APIs; the **Services** tab is delivered by Change 6.

This change is **additive**: a new capability, a new table, and admin endpoints; existing user flows are unchanged.

## Capabilities

### New Capabilities

- `admin-console`: Operator administration — RBAC enforcement, user listing/search/filter, per-user detail + lifecycle actions (suspend/role/reset/delete), aggregate overview metrics, and a persistent, queryable audit log (Journal).

### Modified Capabilities

<!-- None at the requirement level; reuses users/sessions and adds audit persistence the whole system writes to. -->

## Impact

- **New code**: `internal/admin` (handlers + queries), `internal/audit` (audit log model + writer + reader), the React admin shell + Overview/Users/Journal screens.
- **New endpoints**: `GET /api/v1/admin/overview`, `GET /api/v1/admin/users` (search/filter/paginate), `GET /api/v1/admin/users/{id}`, `POST /api/v1/admin/users/{id}/suspend|reactivate`, `PATCH /api/v1/admin/users/{id}/role`, `POST /api/v1/admin/users/{id}/reset-password`, `DELETE /api/v1/admin/users/{id}`, `GET /api/v1/admin/audit`.
- **Schema**: `audit_log` table; (the existing `/api/v1/admin/clients` from Change 1 stays for the Services tab in Change 6).
- **AuthZ**: a session-based admin/owner gate (distinct from the machine `X-Admin-Key` used by client registration); audit every admin action.
- **Security**: privilege-escalation prevention (only owner grants admin/owner; admins cannot act on owners; self-demotion/self-delete guards).
