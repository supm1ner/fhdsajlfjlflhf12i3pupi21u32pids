# Observability

cotton-id is built to be **operated**, not just run. Every request is timed, every
security-relevant event is counted **and** persisted to a durable audit trail, and the
stack ships an optional **observability profile** — Prometheus + Grafana with
provisioned dashboards and alert rules — so an operator can watch the service's health
and be paged when it degrades.

This document covers:

- [The metrics the backend exposes](#1-metrics-exposed) (`/metrics`).
- [Starting the observability stack](#2-starting-the-observability-stack) (`--profile observability`).
- [Reaching Grafana](#3-grafana--dashboards) and the provisioned dashboards.
- [The Prometheus alert rules](#4-alerts).
- [The audit log](#5-the-audit-log) — the persistent, queryable security trail.

The scrape config lives in `deploy/prometheus/prometheus.yml`, the alert rules in
`deploy/prometheus/alerts.yml`, and the Grafana provisioning under `deploy/grafana/`.
Env vars referenced here are defined in [docs/dev/build-contract.md](dev/build-contract.md) §5
and `deploy/.env.example`.

---

## 1. Metrics exposed

`GET /metrics` serves Prometheus text-exposition over a **dedicated registry** (the surface
is explicit, not the global default registry). It is unauthenticated and meant for the
internal scraper; do not expose it publicly. The bundle is built in
`backend/internal/observability/metrics.go`.

### Runtime + process

| Metric (prefix) | What |
|---|---|
| `go_*` | Go runtime: goroutines, GC pauses, heap, threads (the standard `GoCollector`). |
| `process_*` | Process: CPU seconds, open FDs, resident memory (the `ProcessCollector`). |

These give you the saturation picture (memory growth, FD leaks, GC pressure) for the
backend container.

### Application metrics

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `http_request_duration_seconds` | histogram | `route`, `method`, `status` | Per-request server latency. `route` is the **matched chi route pattern** (e.g. `/api/v1/auth/login`), not the raw path, so path parameters don't explode cardinality. The `_count` series doubles as the request-rate signal; `status` carries the error rate. |
| `cotton_login_attempts_total` | counter | `result` = `success`\|`failure`\|`locked` | Password-login outcomes (and the OIDC auto-accept success path). `locked` increments when the per-account lockout refuses an attempt. |
| `cotton_signup_attempts_total` | counter | `result` = `success`\|`failure` | Account sign-ups. |
| `cotton_consent_decisions_total` | counter | `decision` = `accept`\|`reject` | OIDC consent decisions. |
| `cotton_social_logins_total` | counter | `provider`, `result` = `success`\|`failure` | Social sign-in outcomes per provider (Google/GitHub/VK/Yandex). |
| `cotton_passkey_logins_total` | counter | `result` = `success`\|`failure` | Passkey (WebAuthn) login-ceremony outcomes. |
| `cotton_account_lockouts_total` | counter | — | Times an account crossed the lockout threshold and was throttled. |

> The `cotton_social_logins_total`, `cotton_passkey_logins_total`, and
> `cotton_account_lockouts_total` counters were added in the **harden-and-observe** change
> so the dashboards cover every authentication path, not just password login. Older
> exports may not carry them.

A few useful raw queries (run them in Prometheus, §3):

```promql
# Request rate by route (req/s over 5m)
sum by (route) (rate(http_request_duration_seconds_count[5m]))

# 5xx error ratio across the API
sum(rate(http_request_duration_seconds_count{status=~"5.."}[5m]))
  / sum(rate(http_request_duration_seconds_count[5m]))

# p95 latency by route
histogram_quantile(0.95,
  sum by (le, route) (rate(http_request_duration_seconds_bucket[5m])))

# Login failure rate
sum(rate(cotton_login_attempts_total{result="failure"}[5m]))
```

---

## 2. Starting the observability stack

The observability services live behind a Compose **profile** so the default `make up`
stays lean. Bring them up with the `observability` profile (Prometheus + Grafana):

```bash
# Alongside an already-running stack (recommended):
make up                 # the app stack (incl. the dev seed)
make observability      # adds Prometheus + Grafana (profile: observability)

# …or directly with Compose:
docker compose -f deploy/docker-compose.yml --profile observability up -d
```

`make observability` is a thin wrapper over `docker compose --profile observability up -d`.
Prometheus scrapes the backend's `/metrics` every 15s (`deploy/prometheus/prometheus.yml`),
loads the alert rules (`deploy/prometheus/alerts.yml`), and Grafana auto-provisions the
Prometheus datasource and the dashboards from `deploy/grafana/`.

| Service | Host URL | Notes |
|---|---|---|
| Prometheus | <http://localhost:9090> | TSDB + alert evaluation; scrape config in `deploy/prometheus/`. |
| Grafana | <http://localhost:3001> | Dashboards (see §3). Admin password from `GRAFANA_ADMIN_PASSWORD`. |

> **Required env:** Grafana will not start without `GRAFANA_ADMIN_PASSWORD` set in
> `deploy/.env` — there is **no default admin password** (a default would be a known
> credential). The dev `.env.example` ships a placeholder; change it before exposing
> Grafana anywhere reachable.

To stop just the observability services without touching the app stack:

```bash
docker compose -f deploy/docker-compose.yml --profile observability stop prometheus grafana
```

---

## 3. Grafana + dashboards

Open Grafana at <http://localhost:3001> and sign in as `admin` with the
`GRAFANA_ADMIN_PASSWORD` you set. The **Prometheus datasource** is provisioned
automatically (pointed at the in-network `prometheus:9090`), so dashboards work on first
load — no manual datasource setup.

The provisioned **cotton-id** dashboard (`deploy/grafana/dashboards/cotton-id.json`, loaded into
a `cotton-id` Grafana folder via the file provider in `deploy/grafana/provisioning/`) carries
panels covering:

- **HTTP overview** — request rate, 5xx **error rate**, and **p50/p95/p99 latency** by
  route and status, off `http_request_duration_seconds`.
- **Authentication** — login success/failure rates, **social** and **passkey** login
  outcomes, and **account lockouts** (`cotton_login_attempts_total`,
  `cotton_social_logins_total`, `cotton_passkey_logins_total`,
  `cotton_account_lockouts_total`).
- **Sign-ups & consent** — sign-up rate (`cotton_signup_attempts_total`) and OIDC consent
  accept/reject (`cotton_consent_decisions_total`).
- **Dependency health** — backend `up`/down (the scrape target's `up` series), the visible
  signal behind the "target down" alert.

Dashboards are **provisioned from files**, so edits you make in the Grafana UI are not
persisted across a recreate — change the JSON under `deploy/grafana/dashboards/` and
restart Grafana to make a change stick.

---

## 4. Alerts

Prometheus evaluates the rules in `deploy/prometheus/alerts.yml` (loaded via the
`rule_files` entry in `prometheus.yml`). The shipped rules are the production-readiness
"page me when it's actually broken" set:

| Alert | Severity | Fires when | Why it matters |
|---|---|---|---|
| **HighHTTP5xxRate** | critical | the 5xx ratio across the API exceeds **5%** for **5m** | the service is failing requests — a real outage (DB/Hydra down, panic, bad deploy), not a blip. |
| **AuthFailureSpike** | warning | over **50%** of login attempts fail or lock out for **10m** | a credential-stuffing / brute-force attempt, or a broken auth path. |
| **LoginLockoutSurge** | warning | the **lockout** rate stays above **0.2/s** for **10m** | sustained lockouts even amid heavy legit traffic — a throughput-based abuse signal complementing the ratio alert. |
| **TargetDown** | critical | the backend scrape target's `up` is `0` for **2m** (Prometheus can't reach `/metrics`) | the backend is down or unreachable — the highest-severity signal. |

Inspect them live in Prometheus → **Status → Rules** and **Alerts**
(<http://localhost:9090/alerts>), or query the rule expressions directly. Each alert
carries a `severity` label, a `service: cotton-id-backend` label, and a human-readable
`summary`/`description` annotation. Thresholds and `for:` windows are conservative
starting points — tune them to your real traffic.

> **Routing.** The shipped stack evaluates alerts in Prometheus but does **not** include
> an Alertmanager (no paging/Slack/email by default) — wiring Alertmanager (and its
> receivers) is a deployment concern. Point Prometheus's `alerting:` block at your
> Alertmanager to actually deliver the pages.

Tune thresholds and `for:` windows in `deploy/prometheus/alerts.yml`, then reload
Prometheus without a restart (the service runs with `--web.enable-lifecycle`):

```bash
curl -X POST http://localhost:9090/-/reload
```

---

## 5. The audit log

Metrics tell you **rates and shapes**; the **audit log** tells you **who did what**. It is
the durable, queryable security trail — distinct from, and complementary to, the
ephemeral structured stdout log.

- **What it is.** A persistent, **append-only** `audit_log` table (migration
  `0005_audit_log`) that every security-relevant and administrative event is written to:
  login ok/fail, signup, password reset, consent decisions, OAuth-client registration, and
  every admin-console lifecycle action. Each row carries
  `ts, actor_id, actor_label, action, target_type, target_id, ip, request_id, metadata`.
- **Trusted IP, no secrets.** The `ip` is the **trusted** client IP (honoring
  `TRUSTED_PROXIES`), so a spoofed `X-Forwarded-For` can't poison the trail; rows carry
  **no** passwords, session tokens, or reset tokens.
- **Append-only by design.** The API exposes read and the backend exposes write — there is
  **no** update or delete path. It outlives container restarts (unlike stdout).
- **Where to read it.** The admin console's **Journal** tab, the API at
  `GET /api/v1/admin/audit` (role-gated), or directly in SQL (RUNBOOK §10). The Journal
  filters by **action**, **actor** (id or an actor-label substring), **target**
  (`target_type` + `target_id`, backed by an index so a user-detail activity view is an
  indexed lookup, not a scan), and a **time range**, newest first.

```bash
# Latest 50 audit entries, directly in SQL
docker compose -f deploy/docker-compose.yml exec postgres psql -U cotton -d cottonid -c \
  "SELECT ts, actor_label, action, target_type, target_id, ip
   FROM audit_log ORDER BY ts DESC LIMIT 50;"
```

```bash
# Over the API (as a signed-in operator) — filter by action
curl -s -b admin.txt \
  "http://localhost:8080/api/v1/admin/audit?action=user.suspended&page=1&pageSize=10"
```

For the operator-facing Journal workflow (filters, the lifecycle actions, the SQL
cheat-sheet) see [RUNBOOK.md](RUNBOOK.md) §10; for the audit log's place in the threat
model (append-only integrity, trusted IP, no secrets) see [SECURITY.md](SECURITY.md) §2.11.

---

## See also

- [RUNBOOK.md](RUNBOOK.md) — §3 health/metrics, §11 the observability stack + e2e smoke,
  §10 reading the Journal.
- [SECURITY.md](SECURITY.md) — §2.5 (security events), §2.11 (the audit log).
- `deploy/prometheus/` — scrape config + alert rules · `deploy/grafana/` — provisioning +
  dashboards.
