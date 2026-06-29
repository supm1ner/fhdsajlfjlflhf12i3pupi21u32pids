## ADDED Requirements

### Requirement: Email delivery
The system SHALL deliver transactional emails through a configurable SMTP transport, falling back to a development logger when SMTP is not configured, and email delivery SHALL never block or fail the user action.

#### Scenario: SMTP used when configured
- **WHEN** SMTP is configured and a password-reset (or other transactional) email is triggered
- **THEN** the system sends it via SMTP; if delivery fails, the failure is logged and the originating user action still succeeds

#### Scenario: Dev fallback when unconfigured
- **WHEN** SMTP is not configured
- **THEN** the system uses the development logger transport and the application still functions

### Requirement: Login-notification emails
The system SHALL send a login-notification email when a user signs in from a new device/IP, if that user's login-notification preference is enabled.

#### Scenario: Notify on new-device sign-in
- **WHEN** a user with login notifications enabled authenticates from a device/IP not seen recently
- **THEN** the system sends them a notification email (best-effort) and the sign-in completes regardless of delivery

#### Scenario: Respect the preference
- **WHEN** a user has disabled login notifications
- **THEN** no login-notification email is sent on their sign-ins

### Requirement: Operational dashboards and alerts
The system SHALL provide Grafana dashboards and Prometheus alert rules over the exposed metrics so operators can observe and be alerted on the service's health.

#### Scenario: Dashboards provisioned
- **WHEN** the observability stack is started
- **THEN** Grafana is provisioned with the Prometheus datasource and dashboards covering request latency/error rate, authentication outcomes, sign-ups, and consent grants

#### Scenario: Alerts defined
- **WHEN** Prometheus loads its configuration
- **THEN** alert rules for an elevated error rate, an authentication-failure spike, and a dependency being down are active

### Requirement: Session last-seen tracking
The system SHALL record when each session was last used and surface it to the user and to admins.

#### Scenario: Last-seen surfaced
- **WHEN** a user or admin views the user's active sessions
- **THEN** each session shows a last-seen time that reflects recent use (updated on session use, throttled to avoid write amplification)

### Requirement: Multi-replica-capable ceremony state
The system SHALL support a shared signing key so the OAuth and passkey ceremony-state cookies validate across multiple backend instances when configured.

#### Scenario: Shared key honored
- **WHEN** a shared state key is configured
- **THEN** the OAuth and passkey state cookies are signed with keys derived from it, so a ceremony begun on one instance can finish on another

#### Scenario: Safe single-instance default
- **WHEN** no shared key is configured
- **THEN** the system uses a per-process key (single-instance) and records that multi-replica requires the shared key
