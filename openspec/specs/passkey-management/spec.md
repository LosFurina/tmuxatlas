# Passkey Management Specification

## Purpose

Define secure administrator Passkey inventory, enrollment, labeling, deletion,
user experience, and end-to-end verification behavior.

## Requirements

### Requirement: Authenticated Passkey inventory
The system SHALL allow only an authenticated administrator session to retrieve the registered Passkey inventory. Each inventory item SHALL include a URL-safe credential identifier, display label, creation timestamp, and optional last-used timestamp, and MUST NOT expose credential public keys or attestation data.

#### Scenario: Administrator views registered Passkeys
- **WHEN** an authenticated administrator opens Settings → Security
- **THEN** the system returns and displays every registered Passkey with its metadata

#### Scenario: Unauthenticated inventory request
- **WHEN** a request without a valid administrator session requests the Passkey inventory
- **THEN** the system returns an unauthorized response without credential metadata

### Requirement: Additional Passkey enrollment
The system SHALL allow an authenticated administrator to register an additional discoverable Passkey through the browser's native WebAuthn flow using the existing administrator user handle and relying-party ID. The registration options MUST require user verification and MUST NOT restrict the authenticator attachment or provider.

#### Scenario: Add a second Passkey
- **WHEN** an authenticated administrator selects “Add passkey,” supplies an optional label, and completes WebAuthn verification
- **THEN** the system persists the new credential and displays it in the Passkey inventory

#### Scenario: Browser offers a compatible provider
- **WHEN** additional registration begins on a browser with a platform, hybrid phone, or password-manager authenticator available
- **THEN** the WebAuthn request permits the browser to offer any compatible provider

#### Scenario: Unauthenticated additional enrollment
- **WHEN** credentials already exist and a request without a valid administrator session begins another registration
- **THEN** the system rejects the request as unauthorized

#### Scenario: Duplicate credential enrollment
- **WHEN** an authenticator returns a credential identifier that is already registered
- **THEN** the system rejects the duplicate and leaves the inventory unchanged

### Requirement: Passkey label management
The system SHALL allow an authenticated administrator to rename a registered Passkey. A new label MUST be trimmed, non-empty, and no longer than 80 Unicode code points.

#### Scenario: Rename a Passkey
- **WHEN** an authenticated administrator submits a valid new label for an existing credential identifier
- **THEN** the system atomically persists the label and displays the updated value

#### Scenario: Reject an invalid label
- **WHEN** an administrator submits an empty or over-length label
- **THEN** the system returns a validation error and preserves the previous label

#### Scenario: Rename an unknown credential
- **WHEN** an administrator submits a rename for a credential identifier that is not registered
- **THEN** the system returns not found and does not modify the store

### Requirement: Safe Passkey deletion
The system SHALL allow an authenticated administrator to delete a registered Passkey only when at least one other credential will remain. The count check, removal, and persistence MUST execute atomically.

#### Scenario: Delete one of multiple Passkeys
- **WHEN** an authenticated administrator confirms deletion while two or more credentials exist
- **THEN** the system removes the selected credential and retains the remaining credentials

#### Scenario: Prevent deleting the final Passkey
- **WHEN** an administrator attempts to delete the only registered credential
- **THEN** the system returns a conflict, preserves the credential, and explains that the final Passkey cannot be deleted

#### Scenario: Concurrent deletion protection
- **WHEN** concurrent deletion requests would otherwise remove every credential
- **THEN** at most the requests that leave one credential succeed and one credential remains persisted

#### Scenario: Delete an unknown credential
- **WHEN** an administrator requests deletion of a credential identifier that is not registered
- **THEN** the system returns not found and leaves the inventory unchanged

### Requirement: Passkey management user experience
The Security settings interface SHALL expose Passkey inventory and lifecycle actions with clear pending, success, error, and destructive-confirmation states. The interface SHALL disable deletion of the final credential while treating the backend as authoritative.

#### Scenario: Registration succeeds in Settings
- **WHEN** the browser completes an additional Passkey ceremony
- **THEN** the interface refreshes the inventory and identifies the new credential without requiring a page reload

#### Scenario: Registration is cancelled
- **WHEN** the administrator cancels the browser WebAuthn prompt
- **THEN** the interface reports cancellation without changing the inventory

#### Scenario: Delete confirmation
- **WHEN** an administrator selects delete for a removable credential
- **THEN** the interface requires explicit confirmation before sending the deletion request

#### Scenario: Backend rejects stale deletion state
- **WHEN** the interface believes a credential is removable but the backend rejects deletion because it has become the final credential
- **THEN** the interface shows the backend error and refreshes the inventory

### Requirement: End-to-end WebAuthn verification
The project SHALL include an automated browser E2E test using a Chromium virtual authenticator with resident credential and user-verification support. The test MUST exercise real WebAuthn creation and assertion responses against an isolated TmuxAtlas server.

#### Scenario: Complete Passkey lifecycle E2E
- **WHEN** CI runs the Passkey management E2E suite
- **THEN** it verifies first enrollment, logout and login, additional enrollment, rename, removable-credential deletion, and final-credential deletion protection

#### Scenario: Isolated credential state
- **WHEN** the E2E suite starts
- **THEN** it uses a temporary configuration directory and does not read or modify developer or CI-user Passkeys
