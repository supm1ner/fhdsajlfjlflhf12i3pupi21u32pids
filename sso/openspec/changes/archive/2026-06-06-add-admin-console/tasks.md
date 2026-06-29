## 1. Audit log

- [x] 1.1 Migration `0005_audit_log`: table `audit_log(id uuid pk, ts timestamptz default now(), actor_id uuid null, actor_label text, action text, target_type text, target_id text, ip text, request_id text, metadata jsonb)` + indexes on ts, actor_id, action
- [x] 1.2 `internal/audit`: Writer.Append(ctx, Entry) (synchronous insert; log-on-failure, never block) + Reader.Query(filters, page) ; an Entry helper from request context (ip, request_id)
- [x] 1.3 Wire the writer into existing security-event points (auth login ok/fail, signup, reset, oidc consent, adminapi client reg) via a small audit dependency — additive, do not change their behavior

## 2. RBAC + admin service

- [x] 2.1 `internal/httpx` or `internal/admin`: role rank (user<admin<owner) + `RequireRole(min, sessionResolver)` middleware (resolve session user, 401/403)
- [x] 2.2 `internal/admin`: service + Deps + Mount under `/api/v1/admin` (role-gated, CSRF group); reuse auth user store + session store + oidc Hydra client (services count) + audit reader

## 3. Admin endpoints

- [x] 3.1 `GET /api/v1/admin/overview` (aggregates: totals, active-today, new-this-week, services count, 30-day signup series, recent signups, recent activity)
- [x] 3.2 `GET /api/v1/admin/users` (query/status/role filters + pagination + per-user services count) ; `GET /api/v1/admin/users/{id}` (detail: profile, sessions, recent activity, connections)
- [x] 3.3 `POST /admin/users/{id}/suspend` + `/reactivate` (suspend revokes sessions; guards: not self, not owner unless permitted) ; `PATCH /admin/users/{id}/role` (owner-only grant/revoke admin/owner; last-owner + self-escalation guards)
- [x] 3.4 `POST /admin/users/{id}/reset-password` (issue reset token + stub email) ; `DELETE /admin/users/{id}` (owner-only; not self, not last owner; cascade + Hydra revoke)
- [x] 3.5 `GET /api/v1/admin/audit` (filters: actor/action/time + pagination) ; audit every admin action with actor+target ; swaggo; wire main.go; regenerate docs

## 4. Frontend admin shell

- [x] 4.1 `routes/Admin.tsx` shell at `/admin` (+ subroutes), porting `_design_ref/screen-admin.jsx`: sidebar (Overview/Users/Services/Journal/Settings), top search, dark glass console. Auth + role gated (fetch /auth/session; redirect non-admins).
- [x] 4.2 Overview screen: stat cards, 30-day sign-up chart (SVG/CSS bars), recent sign-ups, activity feed
- [x] 4.3 Users screen: filterable/searchable/paginated table (status/role badges, services count, joined) + per-user detail card (summary, action buttons suspend/role/reset/delete with confirmations, sessions, activity, connected services)
- [x] 4.4 Journal screen: audit table with filters (action/actor/time) + pagination
- [x] 4.5 Services + Settings: placeholders (Services filled by Change 6) ; typed api.ts admin methods ; RU/EN

## 5. Tests

- [x] 5.1 Unit: role-rank + RequireRole middleware (user→403, admin→ok, owner-only gating); escalation guards (last-owner, self-action); audit entry construction; overview/user-list query building
- [x] 5.2 Integration (Postgres testcontainer): audit write+query; user list filters/search/pagination; suspend revokes sessions; role-change + delete guards; overview aggregates
- [x] 5.3 Frontend: admin shell redirects non-admins; users table renders + filters; a lifecycle action confirms before calling; journal renders

## 6. Docs & verification

- [x] 6.1 docs: ADMIN.md (console, RBAC, audit) ; SECURITY.md (RBAC, escalation guards, audit log, admin action auditing) ; RUNBOOK (operator tasks)
- [x] 6.2 `go build/vet/test` + gofmt clean; frontend tsc/build/test clean; compose config clean
- [x] 6.3 Live smoke: seed/owner account reaches /admin; non-admin gets 403; list users; suspend a test user (sessions revoked); audit shows the action
