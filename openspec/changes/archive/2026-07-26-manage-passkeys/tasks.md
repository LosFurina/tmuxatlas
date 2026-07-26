## 1. Passkey Store and API

- [x] 1.1 Add safe credential metadata DTOs and URL-safe credential ID encode/decode helpers
- [x] 1.2 Implement locked store operations to list and rename credentials with label validation
- [x] 1.3 Implement atomic credential deletion with unknown-ID and final-credential protection
- [x] 1.4 Add authenticated list, rename, and delete HTTP handlers and register their protected routes
- [x] 1.5 Add store and handler tests covering authorization, metadata filtering, validation, persistence, unknown IDs, and concurrent final-credential protection

## 2. Shared WebAuthn Frontend

- [x] 2.1 Extract creation/request option decoding and credential response serialization from `useAuth.ts` into a shared WebAuthn client module
- [x] 2.2 Refactor first-run registration and Passkey login to use the shared module without changing existing behavior
- [x] 2.3 Add a Passkey management API hook for inventory loading, additional registration, rename, delete, refresh, pending state, and errors

## 3. Security Settings UI

- [x] 3.1 Add a Passkey inventory to Settings → Security with labels, created time, last-used time, and an accessible empty/loading/error state
- [x] 3.2 Add the native “Add passkey” flow with optional labeling and immediate inventory refresh
- [x] 3.3 Add inline rename with non-empty 80-code-point validation and backend error handling
- [x] 3.4 Add explicit deletion confirmation, disable final-credential deletion, and refresh on stale backend conflicts

## 4. Browser E2E Coverage

- [x] 4.1 Add Playwright configuration and an isolated server fixture using temporary configuration and a captured bootstrap token
- [x] 4.2 Add a Chromium CDP virtual authenticator fixture with resident credential and user-verification support
- [x] 4.3 Implement an E2E lifecycle covering first enrollment, logout/login, second enrollment, inventory, rename, deletion, and final-credential protection
- [x] 4.4 Update CI to install Chromium and run the Passkey E2E suite without touching developer or runner Passkey state

## 5. Documentation and Verification

- [x] 5.1 Document adding backup Passkeys, supported browser/provider selection, deletion safety, and recovery limitations
- [x] 5.2 Run backend tests including race coverage, frontend production build, Playwright E2E, full `go test ./...`, and `go vet ./...`
