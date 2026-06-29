## ADDED Requirements

### Requirement: Register a passkey
The system SHALL allow an authenticated user to register a WebAuthn credential through a begin/finish ceremony and SHALL store it bound to their account.

#### Scenario: Successful registration
- **WHEN** an authenticated user starts registration and their authenticator returns a valid attestation to the finish endpoint
- **THEN** the system verifies the attestation against the issued challenge and the configured relying-party id/origin, stores the credential (public key, id, sign count, transports, a nickname) for that account, and reports success

#### Scenario: Registration requires authentication
- **WHEN** an unauthenticated request calls a passkey registration endpoint
- **THEN** the system rejects it with an unauthorized error

#### Scenario: Already-registered credential is excluded
- **WHEN** a user who already has passkeys begins another registration
- **THEN** the issued options exclude their existing credentials so the same authenticator is not double-registered

### Requirement: Passwordless sign-in with a passkey
The system SHALL allow a user to authenticate with a registered passkey through a begin/finish ceremony and SHALL establish a session on success, supporting both username-first and discoverable (usernameless) credentials.

#### Scenario: Successful passkey login
- **WHEN** a user completes the authentication ceremony with a registered credential producing a valid assertion
- **THEN** the system verifies the assertion against the issued challenge and RP id/origin, establishes a cotton-id session, and signs the user in without a password

#### Scenario: Usernameless login via discoverable credential
- **WHEN** a user begins login without supplying an identifier and authenticates with a discoverable credential
- **THEN** the system resolves the account from the credential and signs the user in

#### Scenario: Passkey login completes an OIDC flow
- **WHEN** a relying party's OIDC flow is in progress (a login challenge is carried) and passkey authentication succeeds
- **THEN** the system accepts the Hydra login challenge with the account's stable subject and returns the browser to the relying party

#### Scenario: Unknown or non-matching credential is rejected
- **WHEN** the finish step presents an assertion from a credential not registered to any account, or failing signature/challenge verification
- **THEN** the system refuses to establish a session

### Requirement: Cloned-authenticator detection via sign count
The system SHALL track each credential's signature counter and SHALL treat a non-increasing counter (for authenticators that maintain one) as a possible cloned credential.

#### Scenario: Sign-count regression is refused
- **WHEN** an authentication presents a signature counter less than or equal to the stored value while the authenticator uses counters
- **THEN** the system refuses the authentication and records a security event

### Requirement: Manage passkeys
The system SHALL allow an authenticated user to list and remove their own passkeys.

#### Scenario: List own passkeys
- **WHEN** an authenticated user requests their passkeys
- **THEN** the system returns their credentials' metadata (nickname, created, last used, transports) and never another user's

#### Scenario: Remove a passkey
- **WHEN** an authenticated user deletes one of their registered passkeys
- **THEN** the system removes it and it can no longer be used to sign in

#### Scenario: Cannot remove another user's passkey
- **WHEN** a user attempts to delete a credential that belongs to a different account
- **THEN** the system refuses and the credential is unaffected

### Requirement: Relying-party configuration
The system SHALL derive WebAuthn relying-party parameters (id, display name, allowed origins) from configuration and SHALL reject ceremonies whose origin or RP id does not match.

#### Scenario: Mismatched origin is rejected
- **WHEN** a ceremony is completed from an origin not in the configured allowed origins
- **THEN** the system rejects it
