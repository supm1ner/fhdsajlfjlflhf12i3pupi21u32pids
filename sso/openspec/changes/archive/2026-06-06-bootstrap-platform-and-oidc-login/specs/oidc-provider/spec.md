## ADDED Requirements

### Requirement: OpenID Connect authorization-code flow via Hydra
The system SHALL act as an OpenID Connect provider in which the OAuth2/OIDC protocol endpoints (authorization, token, JWKS) are served by Ory Hydra, and cotton-id implements the login and consent user interaction that Hydra delegates to.

#### Scenario: Relying party completes authorization-code flow
- **WHEN** a registered relying party redirects a browser to Hydra's authorization endpoint with `response_type=code`, a valid `client_id`, `redirect_uri`, `scope`, `state`, and PKCE `code_challenge`
- **THEN** after the user authenticates and consents, Hydra redirects back to the relying party's `redirect_uri` with an authorization code, which the relying party exchanges at the token endpoint for an ID token and access token

#### Scenario: PKCE is required for public clients
- **WHEN** a public client begins an authorization request without a PKCE `code_challenge`
- **THEN** the authorization request is rejected

### Requirement: Login challenge handling
The system SHALL handle Hydra's login challenge by authenticating the user against cotton-id credentials or an existing cotton-id session and accepting or rejecting the challenge through Hydra's admin API.

#### Scenario: Already-authenticated user skips re-login
- **WHEN** Hydra redirects to the login endpoint with a `login_challenge` and the browser already has a valid cotton-id session
- **THEN** cotton-id accepts the login challenge with the session's subject without prompting for credentials again

#### Scenario: Unauthenticated user is prompted then accepted
- **WHEN** Hydra redirects with a `login_challenge` and there is no valid session
- **THEN** cotton-id presents the sign-in form, and upon successful authentication accepts the login challenge with the user's stable subject identifier

#### Scenario: Failed authentication rejects the challenge
- **WHEN** the user fails authentication during a login challenge beyond allowed attempts
- **THEN** cotton-id rejects the login challenge through Hydra and the relying party receives an access-denied error

### Requirement: Consent challenge handling
The system SHALL handle Hydra's consent challenge by presenting the requesting client and requested scopes to the user and accepting or rejecting consent through Hydra's admin API, including the granted scopes and ID-token claims.

#### Scenario: User grants consent
- **WHEN** Hydra redirects to the consent endpoint with a `consent_challenge` for a client requesting scopes
- **THEN** cotton-id displays the client name and requested scopes, and upon the user granting consent accepts the consent challenge with the granted scopes and the user's identity claims

#### Scenario: Consent can be remembered
- **WHEN** a user grants consent and chooses to remember the decision
- **THEN** subsequent authorization requests from the same client for the same-or-narrower scopes are accepted without prompting again, until the grant is revoked

#### Scenario: User denies consent
- **WHEN** a user denies the consent request
- **THEN** cotton-id rejects the consent challenge and the relying party receives an access-denied error

### Requirement: ID token claims
The system SHALL provide identity claims for issued ID tokens consistent with the authenticated cotton-id account, using a stable subject identifier.

#### Scenario: Standard claims populated
- **WHEN** an ID token is issued for an authenticated user who granted the `openid` and `profile`/`email` scopes
- **THEN** the token contains a stable `sub` for that account, and (per granted scope) `email`, `email_verified`, `name`, and `preferred_username` claims sourced from the account

#### Scenario: Subject is stable and non-reassignable
- **WHEN** the same account authenticates across multiple sessions or clients
- **THEN** the `sub` claim is identical each time and is never reused for a different account

### Requirement: OAuth client (relying-party) registration
The system SHALL allow an administrator to register, list, and remove OAuth2 client applications, persisting them via Hydra, including redirect URIs, allowed scopes, grant types, and client type.

#### Scenario: Register a relying party
- **WHEN** an administrator submits a new client with a name, redirect URIs, allowed scopes, and grant/response types to the admin client-registration endpoint
- **THEN** the client is created in Hydra, a `client_id` (and a `client_secret` for confidential clients) is returned, and the client can immediately begin authorization flows

#### Scenario: Registration endpoint requires admin authorization
- **WHEN** a caller without administrator authorization attempts to register a client
- **THEN** the request is rejected with an authorization error and no client is created

#### Scenario: Remove a relying party
- **WHEN** an administrator deletes a registered client
- **THEN** the client is removed from Hydra and subsequent authorization requests using that `client_id` are rejected
