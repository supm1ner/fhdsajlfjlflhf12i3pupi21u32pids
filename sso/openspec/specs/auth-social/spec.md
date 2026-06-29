# auth-social Specification

## Purpose
TBD - created by archiving change add-social-login. Update Purpose after archive.
## Requirements
### Requirement: Sign in with a supported external provider
The system SHALL allow a user to authenticate with Google, GitHub, VK, or Yandex via OAuth2/OIDC, and SHALL establish a cotton-id session on success exactly as password login does.

#### Scenario: Start redirects to the provider
- **WHEN** a user invokes `GET /api/v1/auth/social/{provider}/start` for an enabled provider
- **THEN** the system stores an anti-CSRF `state` (and a PKCE verifier where the provider supports it) in a short-lived signed cookie and redirects the browser to that provider's authorization endpoint with the configured client id, redirect URI, and scopes

#### Scenario: Callback completes authentication
- **WHEN** the provider redirects back to `GET /api/v1/auth/social/{provider}/callback` with a valid code and a `state` matching the stored one
- **THEN** the system exchanges the code for a token, fetches the user's profile, resolves the cotton-id account, establishes a session, and redirects the browser onward (to the SPA, or back into the OIDC flow when a login challenge was in progress)

#### Scenario: Mismatched or missing state is rejected
- **WHEN** the callback `state` is absent or does not match the signed-cookie value
- **THEN** the system rejects the request without exchanging the code or creating a session

### Requirement: Verified-email account linking
The system SHALL link a social identity to an existing cotton-id account only when the provider asserts a verified email matching that account; it SHALL NOT link on an unverified email.

#### Scenario: Returning social user maps to the same account
- **WHEN** a user signs in with a provider whose (provider, subject) pair is already recorded
- **THEN** the system signs them into the previously-linked cotton-id account without creating a duplicate

#### Scenario: Verified email links to an existing account
- **WHEN** a first-time social sign-in presents a verified email that matches an existing cotton-id account
- **THEN** the system links the social identity to that account and signs the user in

#### Scenario: Unverified email never auto-links
- **WHEN** a social sign-in presents an email that the provider has not verified, and an account with that email already exists
- **THEN** the system does NOT link to the existing account (it creates a separate account or refuses), preventing account takeover

#### Scenario: New verified user gets an account
- **WHEN** a verified-email social sign-in matches no existing account
- **THEN** the system creates a new cotton-id account (deriving and uniquifying a username), links the identity, and signs the user in

### Requirement: Continue an in-progress OIDC login via social auth
The system SHALL preserve a Hydra `login_challenge` across the social-login redirect and accept it after authentication.

#### Scenario: Social login completes a relying-party flow
- **WHEN** a relying party's OIDC flow redirected the user to the SPA login, the user chooses a social provider, and authentication succeeds
- **THEN** the system accepts the carried login challenge with the account's stable subject and returns the browser to Hydra to complete token issuance

### Requirement: Per-provider configuration and graceful degradation
The system SHALL treat each provider as independently configurable and SHALL expose only configured providers, rejecting requests for unconfigured ones.

#### Scenario: Enabled providers are advertised
- **WHEN** the SPA requests the list of social providers
- **THEN** the system returns only providers that have credentials configured, and the SPA renders a button for each

#### Scenario: Unconfigured provider is refused
- **WHEN** `start` or `callback` is invoked for a provider with no configured credentials
- **THEN** the system responds with a client error indicating the provider is not enabled, and no redirect or session is produced

#### Scenario: Apple is not offered
- **WHEN** the social sign-in options are presented
- **THEN** Apple is not among them

### Requirement: A cotton-id account may have multiple linked providers
The system SHALL allow one cotton-id account to be linked to several distinct provider identities, keyed by (provider, provider-subject).

#### Scenario: Linking a second provider to the same account
- **WHEN** a user already linked to one provider signs in with a different provider that asserts the same verified email
- **THEN** the system links the second identity to the same account rather than creating a new one

