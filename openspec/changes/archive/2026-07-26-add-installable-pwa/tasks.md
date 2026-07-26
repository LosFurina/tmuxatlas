## 1. PWA Metadata and Assets

- [x] 1.1 Add a valid root-scoped `web/public/manifest.json` with TmuxAtlas identity, standalone presentation, colors, and same-origin start/scope URLs, and keep `web/index.html` metadata aligned with it.
- [x] 1.2 Create dedicated safe-zone maskable PWA icons, reference both `any` and `maskable` icons from the manifest, and verify every declared asset is copied by the Vite build.

## 2. Service Worker and Push Behavior

- [x] 2.1 Add a root-scoped `web/public/sw.js` with immediate lifecycle activation and no fetch handler, application cache, offline shell, or queued network operations.
- [x] 2.2 Implement defensive Push payload parsing, branded notification presentation, safe same-origin destination construction, and notification-click focus/navigate/open behavior in the Service Worker.
- [x] 2.3 Align `usePushNotifications` registration and readiness handling with the root-scoped worker while preserving subscription, re-subscription, and unsubscribe behavior.

## 3. Installation Experience

- [x] 3.1 Add a reusable frontend installation hook that captures `beforeinstallprompt`, records prompt outcomes, detects standalone mode, reacts to `appinstalled`, and detects Apple mobile manual-install environments.
- [x] 3.2 Add an accessible installation section to Settings that invokes the native prompt only on user action, presents Share → Add to Home Screen guidance on iPhone/iPad, and suppresses redundant actions in standalone mode.

## 4. Embedded Resource Delivery

- [x] 4.1 Refactor the embedded frontend handler to serve existing PWA/static resources with correct MIME and Service Worker cache headers, return `404` for missing asset-like paths, and retain SPA fallback for supported deep links.
- [x] 4.2 Add Go tests covering manifest and Service Worker delivery, missing asset responses, standard SPA routes, and session deep links whose names contain dots.

## 5. Browser Coverage and Documentation

- [x] 5.1 Add Playwright coverage for manifest installability fields, Service Worker registration/root scope, absence of application caches or fetch interception, and network delivery after a Hub/frontend version change.
- [x] 5.2 Add Playwright coverage for Chromium install-prompt interaction, dismissal/acceptance state, Apple mobile guidance, standalone suppression, and safe notification-click navigation.
- [x] 5.3 Document PWA installation for Chromium and iPhone/iPad, HTTPS/public-origin requirements, online-only behavior, and compatibility with existing Passkeys and trusted gateways.

## 6. Verification

- [x] 6.1 Run the frontend build and validate that the embedded output contains the manifest, Service Worker, and every declared icon with expected content types.
- [x] 6.2 Run Go tests, frontend E2E tests, `go vet`, installer validation, OpenSpec validation, and repository whitespace checks; resolve all failures before completion.
