# admin-console Specification

## Purpose
TBD - created by archiving change add-admin-console. Update Purpose after archive.
## Requirements
### Requirement: Role-based access to the admin console
The system SHALL restrict the human admin API to accounts with an `admin` or `owner` role, and SHALL require `owner` for the most dangerous actions.

#### Scenario: Non-admin is refused
- **WHEN** a signed-in user without an admin/owner role calls an admin endpoint
- **THEN** the system responds with a forbidden error and performs no action

#### Scenario: Unauthenticated is refused
- **WHEN** an unauthenticated request calls an admin endpoint
- **THEN** the system responds with an unauthorized error

#### Scenario: Owner-only action gated
- **WHEN** an `admin` (not `owner`) attempts an owner-only action (grant admin/owner, delete a user, or act on an owner)
- **THEN** the system refuses

### Requirement: Persistent audit log
The system SHALL record security-relevant and administrative events to a durable, append-only audit log, and SHALL let admins query it.

#### Scenario: Events are recorded
- **WHEN** a security or admin event occurs (login, signup, password reset, consent, client registration, or an admin lifecycle action)
- **THEN** an audit entry is written with the timestamp, actor, action, target, and request context

#### Scenario: Journal is queryable
- **WHEN** an admin requests the audit log with optional filters (actor, action, time range) and pagination
- **THEN** the system returns the matching entries in reverse-chronological order

### Requirement: Overview metrics
The system SHALL provide aggregate metrics for the admin overview.

#### Scenario: Overview returns metrics
- **WHEN** an admin requests the overview
- **THEN** the system returns total accounts, active-today, new-this-week, services count, a 30-day daily sign-up series, recent sign-ups, and a recent activity feed

### Requirement: List and inspect users
The system SHALL let admins list users with search, status/role filters, and pagination, and inspect a single user's detail.

#### Scenario: Filtered, searched, paginated listing
- **WHEN** an admin lists users with a search term and/or status/role filter and a page
- **THEN** the system returns the matching page of users with their status, role, joined date, and connected-services count

#### Scenario: User detail
- **WHEN** an admin requests a specific user
- **THEN** the system returns that user's profile, sessions, recent activity, and connected services

### Requirement: User lifecycle actions
The system SHALL let admins suspend/reactivate, change the role of, force a password reset for, and delete users, with privilege-escalation guards, and SHALL audit every action.

#### Scenario: Suspend revokes access
- **WHEN** an admin suspends an active user
- **THEN** the user's status becomes suspended, their sessions are revoked, they can no longer sign in, and the action is audited

#### Scenario: Reactivate
- **WHEN** an admin reactivates a suspended user
- **THEN** the user's status becomes active and they can sign in again

#### Scenario: Role change is owner-gated and escalation-safe
- **WHEN** a role change is requested
- **THEN** only an `owner` may grant or revoke `admin`/`owner`, the last `owner` cannot be demoted, and a user cannot escalate their own role

#### Scenario: Delete is guarded and cascades
- **WHEN** an `owner` deletes a user
- **THEN** the account and its sessions/passkeys/social identities are removed and the action is audited; a user cannot delete themselves and the last owner cannot be deleted

#### Scenario: Force password reset
- **WHEN** an admin forces a password reset for a user
- **THEN** a single-use reset token is issued (and delivered/stubbed) and the action is audited

