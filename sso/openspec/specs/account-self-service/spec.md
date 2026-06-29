# account-self-service Specification

## Purpose
TBD - created by archiving change add-account-self-service. Update Purpose after archive.
## Requirements
### Requirement: View and edit profile
The system SHALL let a signed-in user view and edit their public profile (display name, about, location) and upload an avatar and banner image.

#### Scenario: Edit profile fields
- **WHEN** a signed-in user submits updated display name / about / location
- **THEN** the system validates and saves them and returns the updated profile

#### Scenario: Upload an avatar
- **WHEN** a signed-in user uploads an avatar image of an allowed type within the size limit
- **THEN** the system stores it, updates the user's avatar URL, and serves the image; an over-size or disallowed-type upload is rejected

#### Scenario: Profile requires authentication
- **WHEN** an unauthenticated request calls a profile endpoint
- **THEN** the system responds with an unauthorized error

### Requirement: Change password
The system SHALL let a signed-in user change their password after re-authenticating with the current one, and SHALL revoke their other sessions on success.

#### Scenario: Successful password change
- **WHEN** a user submits the correct current password and a new policy-compliant password
- **THEN** the system updates the stored hash, keeps the current session, and revokes the user's other sessions

#### Scenario: Wrong current password rejected
- **WHEN** a user submits an incorrect current password
- **THEN** the system rejects the change and the password is unchanged

### Requirement: List and revoke active sessions
The system SHALL let a signed-in user see their active sessions (device, location, timestamps) with the current session identified, and revoke any of them.

#### Scenario: List sessions with current flagged
- **WHEN** a signed-in user requests their sessions
- **THEN** the system returns their sessions and marks which one is the current request's session

#### Scenario: Revoke another session
- **WHEN** a user revokes one of their sessions by id
- **THEN** that session is deleted and can no longer authenticate; a user cannot revoke a session belonging to another account

### Requirement: List and revoke connected apps
The system SHALL let a signed-in user see the relying parties they have granted access to and revoke any grant.

#### Scenario: List connected apps
- **WHEN** a signed-in user requests their connected apps
- **THEN** the system returns the consent grants for their subject (client name + granted scopes)

#### Scenario: Revoke a grant
- **WHEN** a user revokes a connected app
- **THEN** the system revokes that client's consent for the user's subject, so the app must obtain consent again on next authorization

### Requirement: Preferences
The system SHALL persist a user's theme, language, and login-notification preferences and apply them across devices.

#### Scenario: Update preferences
- **WHEN** a signed-in user updates their theme/language/login-notification preferences
- **THEN** the system saves them and they are returned on subsequent loads of the account

### Requirement: Delete account
The system SHALL let a signed-in user permanently delete their account after re-authentication, removing their data.

#### Scenario: Successful deletion
- **WHEN** a user confirms deletion and re-authenticates
- **THEN** the system deletes the account and its sessions, passkeys, social identities, and profile images, best-effort revokes the subject's OIDC sessions, and the account can no longer sign in

#### Scenario: Deletion requires re-auth
- **WHEN** a deletion request is made without valid re-authentication
- **THEN** the system refuses and the account is not deleted

