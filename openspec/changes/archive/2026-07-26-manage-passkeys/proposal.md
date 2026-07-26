## Why

TmuxAtlas currently lets the administrator enroll a Passkey only during first-run setup, leaving no supported way to add a backup authenticator or replace a credential before it is lost. A complete Passkey-only authentication model needs authenticated credential lifecycle management and end-to-end proof that frontend WebAuthn ceremonies match the backend API.

## What Changes

- Add an authenticated Passkey management view under Settings → Security.
- List registered credentials with label, creation time, and last-used time.
- Allow the signed-in administrator to register additional Passkeys using the browser's native WebAuthn provider selection.
- Allow credentials to be renamed and deleted while preventing deletion of the final remaining Passkey.
- Add aligned backend endpoints and persistent-store operations for listing, registering, renaming, and deleting credentials.
- Add browser E2E coverage using a virtual WebAuthn authenticator for first enrollment, login, additional enrollment, rename, deletion, and last-credential protection.

## Capabilities

### New Capabilities

- `passkey-management`: Authenticated lifecycle management for multiple administrator Passkeys, including frontend behavior, API contracts, safety invariants, and E2E verification.

### Modified Capabilities

None.

## Impact

- Backend authentication handlers and `passkeys.json` persistence in `pkg/auth`.
- HTTP route registration in `pkg/server`.
- Settings navigation, Passkey UI, and WebAuthn serialization in `web/src`.
- Playwright E2E setup and CI execution.
- Authentication and deployment documentation.
