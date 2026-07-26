## Context

TmuxAtlas has a single administrator identity backed by one or more discoverable WebAuthn credentials in `~/.config/tmuxatlas/passkeys.json`. The store already records a label, creation time, last-used time, and the complete public credential record, and the registration handler already accepts an authenticated session after initial enrollment. However, there is no read/update/delete API or frontend management surface, WebAuthn browser serialization is embedded in `useAuth.ts`, and no browser test currently exercises a complete ceremony.

This change crosses the authentication store, HTTP routing, Settings UI, and CI. It must preserve the single-user model, keep private key material on the authenticator, remain compatible with platform/cross-device/password-manager authenticators, and prevent an administrator from leaving the installation with no usable credential.

## Goals / Non-Goals

**Goals:**

- Let an authenticated administrator list, add, rename, and delete Passkeys from Settings → Security.
- Expose only credential metadata required by the UI; never expose stored public-key or attestation records through the management API.
- Enforce authorization and the last-credential invariant in the backend, including under concurrent requests.
- Reuse one WebAuthn serialization implementation for first enrollment and additional enrollment.
- Prove frontend/backend compatibility with a real Chromium WebAuthn ceremony driven by a virtual authenticator.

**Non-Goals:**

- Multi-user accounts, roles, invitations, or independent WebAuthn user handles.
- Passkey recovery after every credential has been lost.
- Provider-specific APIs for Apple, Proton Pass, Bitwarden, or 1Password.
- Importing, exporting, or synchronizing private keys.
- Revoking existing in-memory sessions when a non-final credential is deleted.

## Decisions

### 1. Keep one WebAuthn user and attach multiple credentials

Additional Passkeys SHALL use the existing stable WebAuthn user handle and RP ID. This preserves discoverable login behavior and the current single-administrator trust model. Introducing user accounts was rejected because it would change authorization semantics throughout the project.

### 2. Separate metadata management from ceremony endpoints

The existing registration begin/finish endpoints SHALL continue to serve both initial bootstrap registration and authenticated additional registration. New authenticated management endpoints SHALL be:

- `GET /api/auth/passkeys`
- `PATCH /api/auth/passkeys/{credentialID}`
- `DELETE /api/auth/passkeys/{credentialID}`

These routes SHALL be protected by the normal session middleware. Credential IDs SHALL be URL-safe base64 without padding. List responses SHALL contain only ID, label, creation time, and last-used time.

Keeping registration at the existing URLs avoids duplicating ceremony state and preserves first-run compatibility. Putting metadata routes behind the router middleware provides defense in depth in addition to handler-level registration authorization.

### 3. Enforce mutations atomically in PasskeyStore

Rename and delete operations SHALL acquire the store write lock, locate the credential by decoded ID, validate the requested mutation, update memory, and persist through the existing temporary-file-plus-rename path before releasing the lock. Deleting when only one credential remains SHALL return a conflict and leave both memory and disk unchanged.

Labels SHALL be trimmed and limited to 80 Unicode code points. Registration may use an empty label and the UI will display a neutral fallback; an explicit rename requires a non-empty label. This keeps existing first-run records compatible while preventing unbounded or visually empty metadata.

### 4. Extract browser WebAuthn encoding into a shared module

Creation/request option conversion and credential response serialization SHALL move out of `useAuth.ts` into a reusable WebAuthn client module. First-run setup and Settings registration SHALL call the same function, preventing drift between two browser implementations.

A dedicated Passkey management hook/component SHALL own list loading, add, rename, delete, pending state, and error display. The Security section SHALL show credential metadata, an “Add passkey” action, and a destructive confirmation for deletion. The last credential's delete action SHALL be disabled in the UI, while the backend remains authoritative.

### 5. Exercise ceremonies with Playwright and Chromium CDP

E2E tests SHALL start an isolated TmuxAtlas server with a temporary home/config directory and localhost public URL, capture the one-time setup token from server logs, and attach a Chromium virtual authenticator through the DevTools `WebAuthn` domain. The authenticator SHALL support resident credentials and user verification.

The scenario SHALL cover first enrollment, logout/login, additional enrollment, metadata listing, rename, deletion, and rejection of deleting the final credential. CI SHALL install Chromium and run the E2E suite after building the application. Mocking `navigator.credentials` was rejected because it would not validate the JSON/base64 contract or server cryptographic verification.

## Risks / Trade-offs

- **[Risk] Browser virtual-authenticator behavior differs from physical iPhone/password-manager UX** → Use standards-compliant WebAuthn options without attachment restrictions and retain manual provider testing; E2E validates the protocol contract rather than provider chrome.
- **[Risk] Concurrent deletes could remove all credentials** → Check credential count and persist while holding one store write lock.
- **[Risk] A frontend bug could expose destructive actions too easily** → Require explicit confirmation, disable final deletion in the UI, and enforce the invariant again in the backend.
- **[Risk] E2E increases CI duration and dependency size** → Install and run Chromium only, not the full Playwright browser set.
- **[Trade-off] Deleting a credential does not revoke active browser sessions** → Preserve current session semantics; session inventory and revocation remain a separate capability.

## Migration Plan

1. Add store methods and authenticated APIs without changing the existing file schema.
2. Refactor the shared WebAuthn frontend helper and add the Settings surface.
3. Add backend and Playwright tests, then enable the E2E job in CI.
4. Deploy as a backward-compatible release; existing `passkeys.json` files require no migration.

Rollback uses the previous binary and frontend. Credentials added by the new version remain valid because the persisted schema is unchanged. Labels and timestamps unknown to older UI versions are already part of the current store.

## Open Questions

None. Multi-user access, session revocation, and recovery reset are intentionally deferred.
