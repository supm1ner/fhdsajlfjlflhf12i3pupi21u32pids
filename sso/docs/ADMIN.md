# Admin console

cotton-id ships an **operator console** — the place where the people who run cotton-id see growth at
a glance, manage every account, audit what happened, and configure the system. Where
[account self-service](ACCOUNT.md) lets a user manage **their own** identity, the admin console lets
a privileged operator manage **everyone's**. It is a separate, role-gated surface: only `admin` and
`owner` accounts can reach it, and the most dangerous actions are reserved for the `owner`.

The console is the `add-admin-console` change. The behavior, requirements (`SHALL` + WHEN/THEN
scenarios), and design decisions live in
[`openspec/changes/add-admin-console/`](../openspec/changes/add-admin-console/); the exact
request/response shapes ultimately come from the running backend's
[Swagger UI](API.md#viewing-swagger). This doc is the **orientation map**: what the console offers,
the RBAC model, the lifecycle actions and their escalation guards, and the persistent audit log
(Journal) that backs it.

All admin endpoints are **session-protected and role-gated** — an unauthenticated call gets `401`,
and a signed-in non-admin gets `403`. State-changing ones sit in the **CSRF group** (require
`X-CSRF-Token` == the `cid_csrf` cookie). Errors are `application/problem+json` (RFC 7807) like the
rest of the API.

---

## Reaching the console

The SPA renders the console at **`/admin`** (with sub-routes for each tab). The route is **doubly
gated**:

- **Client-side** the SPA fetches `GET /api/v1/auth/session`; a visitor who is not signed in is sent
  to `/login`, and a signed-in user **without** an `admin`/`owner` role is redirected away from
  `/admin` (the console is simply not shown to them). This gate is **UX only** — it keeps the
  console out of the way of ordinary users.
- **Server-side** is authoritative: every `/api/v1/admin/*` endpoint independently re-checks the
  caller's role on each request, so hiding the UI is never the security boundary (see
  [SECURITY.md](SECURITY.md) §2.11). A non-admin who calls an admin endpoint directly still gets
  `403`.

> **There is no second password or separate admin login.** The console reuses the operator's normal
> cotton-id session — they sign in once at `/login`, and if their account carries the `admin`/`owner`
> role the `/admin` surface becomes reachable. To grant someone access, raise their role (below);
> to revoke it, lower their role or suspend the account.

The **dev profile seeds an operator account** so a developer reaches `/admin` immediately —
`admin@cotton.local` / `DemoAdmin!2026`, role **`admin`** (see
[RUNBOOK.md](RUNBOOK.md) §4 and §10). It is a well-known dev credential and **must never be loaded
in production**; in production you create the first operator yourself (below).

---

## The console surface

The console is a dark "glass" shell with a left sidebar (Overview, Users, Services, Journal,
Settings), a top search, and the per-tab screens. It ports the prototype
`_design_ref/screen-admin.jsx` and reuses the shared UI kit and RU/EN i18n (the `adm_*` keys).

| Tab | What it shows / does | Delivered by |
|---|---|---|
| **Overview** | Aggregate stats (total accounts, active today, new this week, registered services), a 30-day daily sign-up chart, recent sign-ups, and a recent-activity feed read from the audit log. | this change |
| **Users** | A searchable, filterable, paginated table of every account (status + role badges, connected-services count, join date) and a per-user **detail** card (profile, sessions, recent activity, connected services) with the **lifecycle action** buttons. | this change |
| **Journal** | A filterable view of the **persistent audit log** — actor, action, target, time, IP — in reverse-chronological order, with filters and pagination. | this change |
| **Services** | The registered relying parties (OAuth clients) — register / edit / delete a client, see each client's consent usage, and revoke a client's grants. | **`add-client-consent-management`** |
| **Settings** | A minimal admin settings surface — read-only system info plus the safe toggles available. | placeholder / later |

The Overview "services count" comes from Hydra's client list (`oidc.HydraClient.ListClients`); the
full Services **management** UI is described in [Services (relying-party clients)](#services-relying-party-clients)
below.

---

## RBAC — the role model

cotton-id has three account roles with a strict **rank**:

```
user  <  admin  <  owner
```

The role lives on the `users` row (`role` column, default `user` — build-contract §4) and is the
single source of authority for the console. A higher rank includes everything a lower rank can do.

| Role | Can reach `/admin` | Can do |
|---|---|---|
| **`user`** | No | Nothing in the console. Manages only their own account at `/account`. This is every self-signed-up account. |
| **`admin`** | Yes | The full **read** surface (Overview, Users list + detail, Journal) and the **non-escalating lifecycle actions**: suspend / reactivate a non-owner, force a password reset, send a (stubbed) message. Cannot grant or revoke `admin`/`owner`, cannot delete a user, and cannot act on an `owner`. |
| **`owner`** | Yes | Everything an `admin` can, **plus** the dangerous actions: **change roles** (grant/revoke `admin`/`owner`), **delete** a user, and act on accounts that are themselves `owner`. The owner is the top of the chain. |

The **rank gate is enforced on the server for every endpoint and every action** — a `RequireRole`
middleware resolves the session user (via `auth.Service.UserForSession`) and checks the rank before
the handler runs; owner-only actions re-check `owner` inside the handler. The client-side redirect is
a convenience, not the control (see [SECURITY.md](SECURITY.md) §2.11).

This human-operator gate is **distinct from the machine `X-Admin-Key`** used by the OAuth
client-registration endpoints (`/api/v1/admin/clients`, build-contract §3 / [RUNBOOK.md](RUNBOOK.md)
§4). The key is for unattended scripts and CI; the console's role gate is for signed-in humans. They
do not interchange.

---

## Operator lifecycle actions

From a user's **detail** card the operator runs the account lifecycle. Every action is
**privilege-escalation-guarded** and **audited** (an entry is written to the Journal with the actor,
the target, and the request context). The guards are enforced server-side regardless of what the UI
offers.

| Action | Endpoint | Min role | Effect & guards |
|---|---|---|---|
| **Suspend** | `POST /api/v1/admin/users/{id}/suspend` | `admin` | Sets `status = suspended` **and revokes the target's sessions** (`SessionStore.DeleteByUser`), so they are signed out everywhere and can no longer sign in. Cannot suspend **yourself**; cannot suspend an **owner** (only an owner may act on an owner). |
| **Reactivate** | `POST /api/v1/admin/users/{id}/reactivate` | `admin` | Sets `status = active`; the account can sign in again. |
| **Change role** | `PATCH /api/v1/admin/users/{id}/role` | `owner` | Grants or revokes `admin`/`owner`. **Owner-only.** Cannot **demote the last owner** (the system always keeps at least one), and cannot **escalate your own** role. |
| **Force password reset** | `POST /api/v1/admin/users/{id}/reset-password` | `admin` | Issues a single-use, time-limited **reset token** (the same mechanism as the self-service "forgot password" flow) and delivers it via the mailer — **stubbed today**, so the token is logged by the dev mailer rather than emailed (see [SECURITY.md](SECURITY.md) §3). The user sets a new password from the reset link. |
| **Delete** | `DELETE /api/v1/admin/users/{id}` | `owner` | **Owner-only**, destructive, irreversible. Removes the `users` row; FK `ON DELETE CASCADE` removes the account's **sessions, passkeys, social identities, and profile images**; then **best-effort** revokes the subject's Hydra login/consent sessions. Cannot **delete yourself**; cannot **delete the last owner**. |
| **Message** | (stub) | `admin` | A "message user" affordance that is **stubbed** until the mailer lands (Change 7). |

### The escalation guards in one place

These are the rules the server enforces so the console cannot be used to escalate privilege or break
the system (see [SECURITY.md](SECURITY.md) §2.11):

- **Only an `owner` may grant or revoke `admin`/`owner`.** An `admin` cannot mint another admin or
  promote anyone — that would let an admin escalate themselves by proxy.
- **An `admin` cannot act on an `owner`.** Suspending, resetting, or otherwise touching an owner
  requires owner rank.
- **No self-escalation.** A user cannot raise their own role (an owner cannot, e.g., re-confirm
  themselves up — there is nothing above owner — and an admin cannot promote themselves at all).
- **No self-destruction.** You cannot **suspend** or **delete your own** account from the console
  (use account self-service for your own account, [ACCOUNT.md](ACCOUNT.md)).
- **The last owner is protected.** The system refuses to demote or delete the final `owner`, so
  there is always at least one account that can administer cotton-id — you can never lock the system
  out of ownership.
- **Every action is audited.** Whether or not it succeeds in changing state, each lifecycle action
  is recorded as an audit entry attributing it to the calling operator.

### Endpoint summary (read surface)

| Method | Path | Min role | Purpose |
|---|---|---|---|
| `GET` | `/api/v1/admin/overview` | `admin` | Aggregate metrics: totals, active-today, new-this-week, services count, 30-day sign-up series, recent sign-ups, recent activity. |
| `GET` | `/api/v1/admin/users` | `admin` | List users — `?query=&status=&role=&page=&pageSize=`; case-insensitive search over username/displayName/email, status/role filters, pagination, per-user connected-services count. |
| `GET` | `/api/v1/admin/users/{id}` | `admin` | One user's detail: profile, sessions, recent activity (from the audit log), connected services. |
| `GET` | `/api/v1/admin/audit` | `admin` | The Journal — `?actor=&action=&from=&to=&page=&pageSize=`; reverse-chronological audit entries. |

The authoritative request/response shapes are in [Swagger](API.md#viewing-swagger); every admin
endpoint carries swaggo annotations.

---

## Services (relying-party clients)

The **Services** tab is where operators manage the **relying parties** — the OAuth/OIDC clients that
authenticate their users through cotton-id (Vault, Mailbox, Studio, Cloud, Stream…). A "service" here
is exactly an **OAuth client** registered in Hydra: an id, a name, the redirect URIs Hydra will send
auth codes back to, the scopes it may request, its grant/response types, and its **type**
(public vs confidential). Where the Overview only **counts** services, this tab is the full
**register → edit → delete** lifecycle, plus each client's live **consent usage** so the registry is
observable. The behavior, requirements, and design decisions live in
[`openspec/changes/add-client-consent-management/`](../openspec/changes/add-client-consent-management/).

> **Two routes, one Hydra registry — do not confuse them.** Client management exists in **two
> places** with **different auth**, by design:
> - the **console** (this tab) under **`/api/v1/admin/services`** — **session + `RequireRole(admin)` +
>   CSRF**, for signed-in human operators; and
> - the **machine API** under **`/api/v1/admin/clients`** — **`X-Admin-Key`**, CSRF-exempt, for
>   unattended scripts / CI / seed (build-contract §3, [RUNBOOK.md](RUNBOOK.md) §4).
>
> They are **distinct paths** (`/services` vs `/clients`) because chi forbids two route registrations
> at the same method+path and the two need different auth (design D1). Both call the **same**
> `oidc.HydraClient` CRUD, so a client created via one route is visible and editable from the other —
> it is one registry, two doors. The console route does **not** accept the admin key, and the machine
> route does **not** accept a session; they do not interchange.

### Public vs confidential clients

The client **type** decides how it authenticates to the token endpoint, and whether it has a secret:

| Type | Who it's for | Auth method | Secret |
|---|---|---|---|
| **`public`** | Browser SPAs and native/mobile apps that **cannot keep a secret**. | `none` — proves possession with **PKCE** (`code_challenge`) instead. | **None.** Public clients have no secret. |
| **`confidential`** | Server-side apps that **can** keep a secret safe. | `client_secret_basic` — Hydra issues a `client_secret`. | **One**, returned **once** on create/regenerate (below). |

The mapping (auth method ← type) and the redirect-URI validation are the **same helpers** used by the
machine route (`adminapi.toHydraClient` / `validRedirectURI`), so a client behaves identically however
it was created. **Redirect URIs must be absolute `http(s)` URLs with a host and no fragment** —
anything else is rejected with a `problem+json` field error. Changing a client's type
(public↔confidential) changes its auth method and secret handling and so is **not a silent edit** —
it requires explicit intent and is documented as a constrained operation (design D2).

### The console lifecycle

From the Services tab an operator runs the full client lifecycle; every **mutation** is **audited**
server-side (actor = the signed-in operator, target = the client id):

- **Register** — a create form (name, type, redirect URIs, scopes, grant/response types). On success
  the new `clientId` is returned; for a **confidential** client the `clientSecret` is shown in a
  **copy-once panel** (see secret handling below). Minimal requests get sensible defaults
  (`authorization_code`+`refresh_token` grants, `code` response, `openid profile email` scopes).
- **Edit** — change a client's name, redirect URIs, allowed scopes, or grant/response types. Edits are
  validated (redirect URIs as above) and persisted in Hydra. Editing does **not** rotate or reveal the
  secret.
- **Delete** — remove a client (UI confirms first). After deletion, **any authorization request using
  that `client_id` is rejected** by Hydra (`invalid_client`), so a delete instantly de-authorizes the
  relying party.
- **Consent usage / revoke** — see below.

### The one-time client secret

For a **confidential** client, Hydra generates the `client_secret` and returns it **exactly once**, on
create (and on an optional regenerate). cotton-id passes it through **once** and **never stores or
re-serves it**: it is **not** included in the list, the detail read, or any later response, and it is
**never logged**. The UI shows it in a one-time copy panel with a clear "store this now — it cannot be
retrieved again" warning. If a secret is lost, the only recovery is to **regenerate** it (which
invalidates the old one) or delete and re-create the client. **Public clients have no secret**, so
there is nothing to copy — they rely on PKCE.

### Per-client consent usage and revocation

Each client's row/detail surfaces how many users have an **active consent grant** for it — i.e. how
many people have signed into that relying party and agreed to its scopes. This is read from **Hydra's
consent sessions** (the same data the per-user "connected apps" surface reads by subject — see
[ACCOUNT.md](ACCOUNT.md)), but aggregated **per client**. Because Hydra lists consent sessions by
**subject**, not by client, the per-client count is **best-effort** and may be a known-limited figure
at scale — this is documented behavior, not a bug (design D3).

An operator can **revoke a client's grants** from the tab: this revokes the consent sessions for that
client so **every user must consent again** on their next authorization to it (the relying party still
exists; it just loses remembered consent). The revoke is **audited**. This is the operator-side,
**per-client** complement to a user revoking a single connection in account self-service.

### Endpoint summary (Services — console, role-gated)

All routes are under the **session + `RequireRole(admin)` + CSRF** group at `/api/v1/admin/services`
(distinct from the machine `/api/v1/admin/clients`). A non-admin gets `403`; an unauthenticated caller
gets `401`. Mutations are audited.

| Method | Path | Min role | Purpose |
|---|---|---|---|
| `GET` | `/api/v1/admin/services` | `admin` | List registered clients (id, name, type, redirect URIs, scopes, grant/response types). **No secrets.** |
| `POST` | `/api/v1/admin/services` | `admin` | Register a client. Returns `clientId`, plus `clientSecret` **once** for a confidential client. Audited. |
| `GET` | `/api/v1/admin/services/{id}` | `admin` | One client's detail (same fields as the list; never the secret). |
| `PATCH` | `/api/v1/admin/services/{id}` | `admin` | Edit name / redirect URIs / scopes / grant+response types. Audited. |
| `DELETE` | `/api/v1/admin/services/{id}` | `admin` | Delete the client (subsequent auth requests with its `client_id` are rejected). Audited. |
| `GET` | `/api/v1/admin/services/{id}/consents` | `admin` | Best-effort count of users with an active consent grant for the client. |
| `DELETE` | `/api/v1/admin/services/{id}/consents` | `admin` | Revoke all of the client's consent grants (users must re-consent). Audited. |

The authoritative request/response shapes are in [Swagger](API.md#viewing-swagger); every endpoint
carries swaggo annotations. The audit actions are `admin.client.create` / `admin.client.delete` (and
the edit/consent-revoke equivalents), with the **operator** as the actor — unlike the machine route,
whose audit rows carry the `admin-key` actor label and no actor id.

---

## The audit log (Journal)

The **Journal** is a real, queryable view of a **persistent, append-only audit log** — not the
ephemeral stdout log. Before this change, security events only reached stdout (structured slog), so
they vanished with the container and could not be browsed. This change adds an `audit_log` table and
an audit writer that the existing security-event log points **also** write to, plus a reader the
Journal queries.

### What is recorded

The audit writer captures the security-relevant and administrative events across the backend — they
are written **in addition to** the existing structured stdout log (the stdout lines are unchanged; the
audit row is additive):

- **Authentication & account:** login succeeded / failed, signup, password reset.
- **OIDC:** consent decisions (accept/reject).
- **Registry:** OAuth client (relying-party) registration via the admin API.
- **Admin lifecycle:** every console action above — suspend, reactivate, role change, force reset,
  delete — attributed to the operator who performed it.

### The audit row

Each entry carries the timestamp, who did it, what they did, to whom, and the request context. The
`audit_log` table (migration `0005_audit_log`) is:

| Column | Meaning |
|---|---|
| `id` | Primary key (UUID). |
| `ts` | When it happened (`timestamptz`, defaults to now). |
| `actor_id` | The account that performed the action (nullable — e.g. a failed login has no resolved actor). |
| `actor_label` | A human-readable label for the actor (e.g. username/email) for display. |
| `action` | The event name (e.g. `login.succeeded`, `user.suspended`, `client.registered`). |
| `target_type` / `target_id` | What the action was about (e.g. a user, a client). |
| `ip` | The **trusted** client IP (`httpx.ClientIP`, honoring `TRUSTED_PROXIES` — a spoofed `X-Forwarded-For` cannot poison the trail; [SECURITY.md](SECURITY.md) §2.5). |
| `request_id` | Correlates the audit row with the stdout request log. |
| `metadata` | A `jsonb` bag for action-specific detail (no secrets). |

The table is indexed on `ts`, `actor_id`, and `action` for the Journal's filters.

### Properties

- **Append-only.** The API exposes **read** (the Journal) and the internal **write** (the audit
  writer); there is **no update or delete** path. The audit trail is a record, not editable state.
- **Best-effort, never blocking.** The writer inserts synchronously in-request (simplest at this
  scale). If the insert fails, the failure is **logged at error and the user's action still
  succeeds** — an audit hiccup must not break login or an admin action. (A buffered async writer is a
  later optimization; see the change's design D2.)
- **No secrets.** Like the stdout security log, audit rows never contain passwords, full session
  tokens, or reset tokens.

### Reading the Journal

Operators read it in the console's **Journal** tab (filter by action / actor / time range, paginate).
The same data is available over the API at `GET /api/v1/admin/audit`, and — because the rows live in
the `cottonid` database — directly in SQL for ad-hoc operator queries (see [RUNBOOK.md](RUNBOOK.md)
§10).

---

## Overview metrics

The Overview is intentionally **simple SQL aggregates** (no BI/analytics engine):

- **Total accounts** — count of `users`.
- **Active today** — accounts with a login event today (audit login events, with a sessions
  last-seen fallback).
- **New this week** — `users` created in the last 7 days (`created_at`).
- **Services** — the Hydra client count (`oidc.HydraClient.ListClients`).
- **30-day sign-up series** — daily `users` created over the last 30 days
  (`GROUP BY date_trunc('day', created_at)`), rendered as the bar chart.
- **Recent sign-ups** — the latest N accounts.
- **Recent activity** — the latest N audit rows.

These are bounded queries backed by indexes on `created_at`/`status`/`role`; if the user table grows
very large, the series and aggregates are the candidates for materialized views (change design,
Risks).

---

## Configuration

The console adds **no new operator configuration** — it reuses the existing session, CSRF, trusted-proxy,
and password-reset settings (build-contract §5). The only new persistent state is the `audit_log`
table, created by migration `0005_audit_log` on backend start (applied automatically — see
[RUNBOOK.md](RUNBOOK.md) §2). The reset-token TTL the "force password reset" action uses is the
existing `PASSWORD_RESET_TTL_MINUTES` (default 30).

To create the **first operator in production** (where the dev seed is never loaded), promote an
existing account to `owner` directly in the database, then use that account to manage roles from the
console thereafter — see [RUNBOOK.md](RUNBOOK.md) §10.

---

## Deferred / known gaps

These are deliberate, documented limits of this surface (tracked for later changes):

- **Per-client consent count is best-effort.** Because Hydra lists consent sessions by **subject**,
  not by client, the Services tab's per-client usage count is a best-effort figure that may be limited
  at scale (design D3). The register/edit/delete lifecycle and consent **revoke** are fully wired (see
  [Services (relying-party clients)](#services-relying-party-clients)); the machine `X-Admin-Key` API
  ([RUNBOOK.md](RUNBOOK.md) §4) remains for automation.
- **"Message user" is stubbed.** Real email to a user waits on the mailer (Change 7); the affordance
  is present but inert.
- **Force-reset email is stubbed.** The reset token is issued and **logged by the dev mailer**, not
  emailed ([SECURITY.md](SECURITY.md) §3, "Email delivery is stubbed"). Treat reset-token log lines
  as sensitive wherever the dev mailer is active.
- **Audit writer is synchronous.** A buffered/async writer is a later optimization; today the insert
  is in-request and best-effort (above).
- **Settings is minimal.** Read-only system info plus the safe toggles available; deeper system
  configuration is out of scope for this change.
