# password-authentication Specification

## Purpose
TBD - created by archiving change bootstrap-platform-and-oidc-login. Update Purpose after archive.
## Requirements
### Requirement: Account creation with email and password
The system SHALL allow a visitor to create an account with a display name, a unique username, a unique email address, and a password, and SHALL reject duplicates and invalid input.

#### Scenario: Successful signup
- **WHEN** a visitor submits a valid display name, unused username, unused well-formed email, and a password meeting the policy
- **THEN** the system creates the account, stores only an argon2id hash of the password, establishes a session, and responds with the authenticated user's public profile

#### Scenario: Duplicate email rejected
- **WHEN** a visitor submits an email that already belongs to an account
- **THEN** the system rejects the signup with a validation error and does not reveal whether the email specifically is taken beyond a generic "already in use" message

#### Scenario: Duplicate username rejected
- **WHEN** a visitor submits a username that is already taken
- **THEN** the system rejects the signup with a validation error identifying the username field

#### Scenario: Weak password rejected
- **WHEN** a visitor submits a password below the minimum policy (shorter than 8 characters or below the minimum strength)
- **THEN** the system rejects the signup with a validation error and the account is not created

### Requirement: Password hashing with argon2id
The system SHALL hash passwords using argon2id with per-password random salt and SHALL store the algorithm parameters alongside the hash so they can evolve over time.

#### Scenario: Password stored as argon2id PHC string
- **WHEN** an account is created or its password changed
- **THEN** the stored credential is an argon2id PHC-encoded string containing the version, memory, iterations, parallelism, salt, and hash, and never the plaintext

#### Scenario: Hash verification is constant-time
- **WHEN** a login attempt verifies a password against a stored hash
- **THEN** the comparison is performed in constant time with respect to the hash bytes

### Requirement: Email and password login
The system SHALL authenticate a user by email and password and, on success, establish a session.

#### Scenario: Successful login
- **WHEN** a user submits the correct email and password for an active account
- **THEN** the system establishes a session, sets a secure session cookie, and responds with the user's public profile

#### Scenario: Wrong password rejected uniformly
- **WHEN** a user submits an email that exists but the wrong password, or an email that does not exist
- **THEN** the system returns the same generic "invalid credentials" error for both cases and does not disclose which field was wrong

#### Scenario: Suspended account cannot log in
- **WHEN** a user with a suspended account submits correct credentials
- **THEN** the system refuses to establish a session and returns an account-status error

### Requirement: Secure server-side sessions
The system SHALL represent sessions as opaque server-side records delivered via an `HttpOnly`, `Secure`, `SameSite=Lax` cookie, and SHALL support revocation and expiry.

#### Scenario: Session cookie attributes
- **WHEN** a session is established
- **THEN** the session cookie is marked `HttpOnly`, `Secure`, and `SameSite=Lax`, and its value is an opaque identifier that is not the raw user id

#### Scenario: Remember-me extends lifetime
- **WHEN** a user logs in with "remember me" enabled
- **THEN** the session is persisted with an extended expiry (30 days); otherwise the session expires within 24 hours

#### Scenario: Logout revokes the session
- **WHEN** an authenticated user logs out
- **THEN** the server-side session record is deleted and the session cookie is cleared, and the prior cookie can no longer authenticate requests

#### Scenario: Expired session is rejected
- **WHEN** a request presents a session cookie whose server-side record has expired
- **THEN** the request is treated as unauthenticated and the stale record is not honored

### Requirement: Password reset
The system SHALL allow a user to request a password reset by email and to set a new password using a single-use, time-limited reset token, without revealing whether an email is registered.

#### Scenario: Reset request does not enumerate accounts
- **WHEN** a visitor requests a password reset for any email address
- **THEN** the system responds with the same success message regardless of whether an account exists for that email

#### Scenario: Valid token sets a new password
- **WHEN** a user submits a valid, unexpired reset token together with a new policy-compliant password
- **THEN** the system updates the stored hash, invalidates the reset token, and invalidates existing sessions for that account

#### Scenario: Used or expired token rejected
- **WHEN** a reset token that has already been used or has expired is submitted
- **THEN** the system rejects the request and does not change the password

### Requirement: Brute-force protection on credential endpoints
The system SHALL rate-limit and throttle repeated authentication attempts per IP address and per account.

#### Scenario: Repeated failures are throttled
- **WHEN** login attempts for the same account fail repeatedly beyond the configured threshold
- **THEN** further attempts are delayed or temporarily refused and the throttling is recorded as a security event

