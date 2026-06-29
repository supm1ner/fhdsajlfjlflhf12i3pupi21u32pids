## ADDED Requirements

### Requirement: Containerized runtime topology
The system SHALL run as a set of Docker containers — PostgreSQL, Ory Hydra, the cotton-id backend, and the cotton-id frontend — orchestrated by a single Docker Compose file, such that the whole stack starts with one command.

#### Scenario: Stack starts with one command
- **WHEN** an operator runs `docker compose up` in a clean checkout with the required environment variables set
- **THEN** PostgreSQL, Hydra, the backend, and the frontend all start, database migrations are applied before the backend serves traffic, and each service reports healthy via its health check

#### Scenario: Backend waits for its dependencies
- **WHEN** the backend container starts before PostgreSQL or Hydra are ready
- **THEN** the backend retries its dependency connections with backoff and only begins serving HTTP once migrations have applied and dependencies are reachable

### Requirement: Typed, validated configuration
The system SHALL load all configuration from environment variables into a typed structure and SHALL validate it at startup, refusing to start when a required secret is missing or a security-critical key uses a known-insecure default outside development.

#### Scenario: Missing required secret fails fast
- **WHEN** the backend starts without a required secret (for example the session signing key) in a non-development environment
- **THEN** the process exits non-zero before binding the HTTP port and logs which configuration value is missing

#### Scenario: Example configuration is provided
- **WHEN** a developer inspects the repository
- **THEN** a committed `.env.example` documents every configuration variable and no real secret values are committed

### Requirement: Database access and versioned migrations
The system SHALL access PostgreSQL through a pooled connection and SHALL manage schema with forward-only versioned SQL migrations applied automatically on deploy.

#### Scenario: Migrations apply idempotently
- **WHEN** the migration step runs against a database that is already at the latest version
- **THEN** no migration is re-applied and the step succeeds without error

#### Scenario: Fresh database is brought to current schema
- **WHEN** the stack starts against an empty database
- **THEN** all migrations are applied in order and the resulting schema matches the current version

### Requirement: HTTP security middleware baseline
The system SHALL apply, to all browser-facing HTTP responses, security headers including `Content-Security-Policy`, `X-Content-Type-Options: nosniff`, `Referrer-Policy`, and `X-Frame-Options`/frame-ancestors, and SHALL attach a unique request id to every request.

#### Scenario: Security headers present
- **WHEN** a client requests any browser-facing route
- **THEN** the response includes the configured security headers and a request-id header

#### Scenario: Errors use problem+json
- **WHEN** a request fails with a client or server error
- **THEN** the response body is `application/problem+json` with a stable type, title, and status, and does not leak stack traces or internal details

### Requirement: Prometheus metrics endpoint
The system SHALL expose a Prometheus-format metrics endpoint at `/metrics` covering Go runtime metrics and application metrics including HTTP request duration by route and status.

#### Scenario: Metrics are scrapeable
- **WHEN** Prometheus (or any client) scrapes `/metrics`
- **THEN** the endpoint returns HTTP 200 with Prometheus text exposition including `http_request_duration_seconds` labeled by route and status code

### Requirement: Structured logging
The system SHALL emit structured JSON logs that include the request id and SHALL log every security-relevant event (authentication attempts, signups, password resets, consent decisions, client registrations) with structured fields and without logging secrets or raw credentials.

#### Scenario: Request is logged with correlation id
- **WHEN** any HTTP request is handled
- **THEN** a structured log line is emitted containing the request id, method, route, status, and duration

#### Scenario: Credentials never appear in logs
- **WHEN** a login or signup request is processed
- **THEN** no log line contains the plaintext password or full session token

### Requirement: OpenAPI documentation for every endpoint
The system SHALL publish an OpenAPI document describing every public HTTP endpoint and SHALL serve interactive Swagger UI for it.

#### Scenario: Swagger UI lists all endpoints
- **WHEN** a developer opens the Swagger UI route
- **THEN** every public endpoint is present with its method, path, request schema, and response schemas

#### Scenario: Documentation stays in sync
- **WHEN** the API surface changes without the OpenAPI document being regenerated
- **THEN** the documentation generation check fails in CI

### Requirement: Health checks
The system SHALL expose a liveness/readiness endpoint that reports the backend's ability to reach its critical dependencies.

#### Scenario: Healthy when dependencies reachable
- **WHEN** `/healthz` is requested while PostgreSQL and Hydra are reachable
- **THEN** the endpoint returns HTTP 200 with a body indicating the service and its dependencies are healthy

#### Scenario: Unhealthy when a dependency is down
- **WHEN** `/healthz` is requested while PostgreSQL is unreachable
- **THEN** the endpoint returns a non-200 status indicating degraded health

### Requirement: Web application shell with theming and localization
The frontend SHALL deliver a single-page application reproducing the cotton-id visual design (glass surfaces, accent-hue token system, serif/sans type) and SHALL support a dark and a light theme and Russian and English locales, with the user's choice persisted.

#### Scenario: Theme toggle persists
- **WHEN** a user switches between dark and light theme
- **THEN** the interface updates immediately and the choice persists across reloads

#### Scenario: Locale toggle switches all copy
- **WHEN** a user switches the language between RU and EN
- **THEN** all visible interface copy renders in the selected language and the choice persists across reloads
