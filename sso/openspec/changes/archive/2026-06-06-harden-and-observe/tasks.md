## 1. Email delivery

- [x] 1.1 `internal/mailer`: SMTPMailer implementing Mailer (host/port/user/pass/from/STARTTLS); main.go selects SMTP when SMTP_HOST set, else LogMailer; config SMTP_* vars + .env.example + compose
- [x] 1.2 Login-notification: a Notifier triggered on successful interactive login (password/social/passkey) when the account's login_notifications pref is true and the device/IP is "new" (coarse fingerprint, recent window); best-effort, non-blocking, detached context
- [x] 1.3 Admin "message user": un-stub — send an email via the Mailer; audited

## 2. Observability

- [x] 2.1 deploy/prometheus/alerts.yml: rules (high 5xx rate, auth-failure spike, target down); load it in prometheus.yml
- [x] 2.2 deploy/grafana: compose service (observability profile) + provisioned Prometheus datasource + dashboards JSON (HTTP latency/error by route, login outcomes + lockouts, signups, consent grants, healthz up); GRAFANA_ADMIN_PASSWORD config
- [x] 2.3 Backend: add any missing metrics (social/passkey login outcome counters; lockout counter) so dashboards are complete

## 3. Deferred hardening

- [x] 3.1 Migration: sessions.last_seen_at; bump on session use (throttled ≤1/min/session); surface in account GET /sessions + admin user detail
- [x] 3.2 Atomic avatar/banner upload: wrap blob upsert + URL set in one pgx transaction
- [x] 3.3 Migration: index audit_log (target_type, target_id); add target filter to audit.Filter + the Journal API + actor-label substring filter; user-detail activity uses the target filter (no 200-row scan)
- [x] 3.4 Shared state key: if OAUTH_STATE_KEY (>=32 bytes) set, derive social cid_oauth + passkey cid_wa HMAC keys via HKDF (distinct labels); else per-process key + a startup warning; config + docs

## 4. Tests

- [x] 4.1 Unit: SMTPMailer message building (no real send); new-device fingerprint decision; HKDF key derivation determinism + distinctness; last_seen throttle; audit target filter query
- [x] 4.2 Integration (//go:build integration): last_seen bump + surface; atomic image upload rollback on failure; audit target-filtered query
- [x] 4.3 Frontend: sessions show last-seen; (no major UI change)

## 5. Docs

- [x] 5.1 docs/OBSERVABILITY.md (dashboards, alerts, how to reach Grafana) ; update SECURITY.md (remove/amend the resolved single-instance + best-effort-revoke + last-seen gaps; note login-notification + email delivery) ; RUNBOOK (SMTP setup, Grafana, alerts) ; README (observability + email)
- [x] 5.2 .env.example + compose: SMTP_*, OAUTH_STATE_KEY, GRAFANA_ADMIN_PASSWORD documented

## 6. Close-out: full-system verification

- [x] 6.1 A scripted e2e smoke (Make target / script): bring up the stack; exercise CSRF→signup→login→reset, a registered RP authorization_code+PKCE flow (login+consent→token), providers list, passkey login/begin shape, account profile/sessions, admin RBAC (admin 200 / user 403) + a lifecycle action visible in the Journal
- [x] 6.2 Final adversarial security review across the whole system (regressions, the now-complete attack surface)
- [x] 6.3 `go build/vet/test` + gofmt clean; frontend tsc/build/test clean; `docker compose up` healthy end-to-end; archive all OpenSpec changes
