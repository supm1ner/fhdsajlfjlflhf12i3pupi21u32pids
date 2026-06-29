## Context

Everything functional is built; the gaps are operational. Metrics (`/metrics`) and a persistent audit log already exist — they just need dashboards + alerts. The `Mailer` is an interface with a dev logger — it needs an SMTP implementation and a login-notification trigger. Several low-severity review items were consciously deferred to this change.

## Goals / Non-Goals

**Goals:**
- Real email delivery (reset, login-notification, admin message), configurable, with the dev logger as the safe default.
- Grafana dashboards + Prometheus alerts over existing metrics/audit.
- Resolve the deferred hardening items.
- A final full-system review + e2e smoke proving the whole IdP works.

**Non-Goals:**
- A new notification channel beyond email (SMS/push).
- Multi-region/HA topology beyond the shared-key change that *enables* multi-replica.
- Rewriting any existing capability.

## Decisions

### D1 — SMTP Mailer behind the existing interface
Add `SMTPMailer` implementing `mailer.Mailer` (net/smtp or a small lib), configured by `SMTP_HOST/PORT/USERNAME/PASSWORD/FROM/STARTTLS`. `main.go` selects SMTP when `SMTP_HOST` is set, else the dev `LogMailer`. All existing call sites (reset) and the new ones (login-notification, admin message) go through the interface unchanged. Failures are logged, never block the user action (reset already does this).

### D2 — Login-notification emails
On a successful interactive login (password/social/passkey) from a new device/IP, if the account's `login_notifications` preference is true, enqueue a notification email (best-effort, async-ish via a goroutine with a detached context, or synchronous best-effort). "New device" = no prior session with the same (user-agent, ip) coarse fingerprint in the recent window. Keep it simple + non-blocking; document the heuristic.

### D3 — Grafana + Prometheus alerts
Add a `grafana` compose service (observability profile) with a provisioned Prometheus datasource and dashboards (JSON under `deploy/grafana/`): HTTP latency/error rate by route/status, login success/failure + lockouts, signups, consent grants, `/healthz` dependency up/down. Add `deploy/prometheus/alerts.yml` with rules (high 5xx rate, auth-failure spike, target down) and load it in the Prometheus config. Add any missing counters (e.g. social/passkey login outcomes) so the dashboards are complete.

### D4 — Deferred hardening
- **Session last_seen_at**: add the column; bump it when a session authenticates a request (cheap UPDATE, throttled to ≤1/min/session to avoid write amplification); surface in account `GET /sessions` + admin user detail.
- **Atomic image upload**: wrap the blob upsert + URL set in one pgx transaction.
- **Audit target filter + index**: add `(target_type, target_id)` index; add a target filter to `audit.Filter` + the Journal API; make the user-detail activity query use it (instead of scanning a 200-row window). Add an actor-label substring filter so the Journal's free-text actor input works.
- **Shared state signing key**: if `OAUTH_STATE_KEY` (≥32 bytes) is set, derive the social `cid_oauth` and passkey `cid_wa` cookie HMAC keys from it (HKDF with distinct labels) so all replicas agree; else keep the per-process random key (single-instance) and log a warning that multi-replica needs the shared key.

### D5 — Final review + e2e
A scripted end-to-end smoke (a Make target / shell script) brings up the stack and exercises: CSRF, signup, login, password reset, a registered RP's authorization_code+PKCE flow through login+consent to token, the providers list, a passkey login ceremony's begin shape, account profile/sessions, and admin RBAC (admin 200 / user 403) + a lifecycle action appearing in the Journal. A final adversarial review covers the whole system for regressions.

## Risks / Trade-offs

- **SMTP misconfiguration blocking actions** → Mitigation: send is best-effort + logged; the user action never depends on delivery; dev default is the logger.
- **Login-notification noise / false "new device"** → Mitigation: coarse fingerprint + the per-account preference (default on, user-toggleable); document the heuristic; rate-limit notifications.
- **last_seen_at write amplification** → Mitigation: throttle the bump (≤1/min/session).
- **Grafana default credentials** → Mitigation: require `GRAFANA_ADMIN_PASSWORD`; observability profile only; documented.

## Open Questions

- Whether to make login-notification "new device" stateful (a known-devices table) — deferred; the coarse heuristic is enough for v1, documented.
- HKDF vs simple SHA-256 domain separation for the shared state key — HKDF with labels; both keys derived distinctly.
