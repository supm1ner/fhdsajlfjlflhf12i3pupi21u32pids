## ADDED Requirements

### Requirement: Manage OAuth clients from the role-gated console
The system SHALL let an admin list, create, view, edit, and delete OAuth relying-party clients through session+role-gated admin endpoints, and SHALL audit each change.

#### Scenario: List clients
- **WHEN** an admin requests the registered clients
- **THEN** the system returns each client's id, name, type (public/confidential), redirect URIs, and allowed scopes

#### Scenario: Create a client shows the secret once
- **WHEN** an admin creates a confidential client
- **THEN** the system returns the generated `client_id` and `client_secret` exactly once, the action is audited, and the secret is never re-served on subsequent reads

#### Scenario: Edit a client
- **WHEN** an admin updates a client's name, redirect URIs, scopes, or grant/response types with valid values
- **THEN** the system persists the change in Hydra and audits it; invalid redirect URIs (non-absolute or containing a fragment) are rejected

#### Scenario: Delete a client
- **WHEN** an admin deletes a client
- **THEN** the client is removed and subsequent authorization requests using its `client_id` are rejected, and the deletion is audited

#### Scenario: Non-admin cannot manage clients
- **WHEN** a non-admin (or unauthenticated) request hits a console client endpoint
- **THEN** the system refuses with a forbidden/unauthorized error

### Requirement: Per-client consent visibility and revocation
The system SHALL let an admin see how many users have granted a client consent and revoke that client's grants.

#### Scenario: Consent usage count
- **WHEN** an admin requests a client's consent usage
- **THEN** the system returns the number of users who have an active consent grant for that client (best-effort per Hydra's capability)

#### Scenario: Revoke a client's grants
- **WHEN** an admin revokes a client's consents
- **THEN** the system revokes the consent grants for that client so its users must consent again on next authorization, and the action is audited
