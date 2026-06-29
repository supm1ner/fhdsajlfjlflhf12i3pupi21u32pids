## Why

The v1 feature set is built; this final change makes cotton-id **production-operable**: real email delivery (so password-reset, login-notification, and admin "message" stop being stubs), an observability stack (Grafana dashboards + Prometheus alerts over the metrics + audit log already emitted), and the deferred low-severity hardening items accumulated across the adversarial reviews. It closes with a full-system security review and an end-to-end verification of every capability.

## What Changes

- **Email delivery**: a real SMTP `Mailer` (configurable; the dev `LogMailer` stays the default for local) wired to password reset, **login-notification emails** (honoring the `login_notifications` preference — email on sign-in from a new device/IP), and the admin **"message user"** action (un-stub).
- **Observability**: a Grafana service + provisioned dashboards (auth success/failure rates, request latency by route, signups, consent grants, lockouts) and Prometheus **alert rules** (error-rate, auth-failure spike, dependency down); enrich app metrics where gaps exist.
- **Deferred hardening** (from prior reviews): session `last_seen_at` (bumped on use, surfaced in account + admin); atomic avatar/banner upload (single transaction); audit `target` index + a target/actor-label filter for the Journal; an optional **shared signing key** (config) for the OAuth/passkey state cookies so multi-replica deployments work; document the single-instance limits removed.
- **Close-out**: a final full-system adversarial security review and a scripted end-to-end smoke covering every capability (password, social, passkey, OIDC RP flow, account, admin).

This change is **additive + hardening**: new optional config + a Grafana service + small schema touches; no capability is removed.

## Capabilities

### New Capabilities

- `observability-ops`: Operational observability + delivery — SMTP email delivery (reset/login-notification/message), Grafana dashboards, Prometheus alert rules, and the production-readiness hardening of prior deferred items.

### Modified Capabilities

<!-- Touches account-self-service (last_seen_at, atomic image), admin-console (audit target filter), platform-foundation (alerts/metrics) at the implementation level; no spec requirement is rewritten — captured as new observability-ops requirements. -->

## Impact

- **New code**: `internal/mailer` SMTP implementation + login-notification trigger; Prometheus alert rules + Grafana provisioning under `deploy/`; small store/handler edits for last_seen_at + atomic image + audit target filter.
- **New config**: `SMTP_*` (host/port/user/pass/from/tls), `OAUTH_STATE_KEY` (optional shared key), Grafana admin password.
- **New infra**: a `grafana` service (observability profile) + provisioned datasource/dashboards; Prometheus alert rules file.
- **Schema**: `sessions.last_seen_at`; an index on `audit_log (target_type, target_id)`.
- **Security**: closes the deferred review items; the final review confirms no regressions across the now-complete system.
