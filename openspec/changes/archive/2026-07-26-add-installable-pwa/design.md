## Context

The React/Vite frontend is embedded into the Go binary and served by a catch-all route. `index.html` already references `/manifest.json`, the repository already contains branded 192 px, 512 px, and Apple touch icons, and `usePushNotifications` already attempts to register `/sw.js`. Neither referenced file currently exists. Because the Go catch-all treats every missing path as an SPA navigation, both URLs return `index.html` with `text/html`, which prevents manifest parsing and Service Worker registration.

TmuxAtlas is inherently online: terminal sessions, authentication, state, and Peer connectivity all depend on a live Hub. The PWA must therefore improve installation and presentation without implying that terminal operations work offline. The deployed public URL remains the Passkey relying-party origin, including when the UI runs in standalone mode behind Cloudflare Tunnel or another trusted HTTPS gateway.

## Goals / Non-Goals

**Goals:**

- Satisfy browser PWA installation requirements with valid, same-origin metadata and branded assets.
- Offer a discoverable install experience on Chromium and useful manual instructions on iOS/iPadOS.
- Complete the existing Web Push flow with robust `push` and `notificationclick` handling.
- Guarantee that authentication, APIs, WebSockets, terminal streams, and navigations are never served from a Service Worker cache.
- Make missing static resources observable as `404` responses rather than disguised SPA documents.
- Preserve the current embedded-frontend build and release model with no required runtime dependency.

**Non-Goals:**

- Offline terminal access, offline authentication, background terminal execution, or offline mutation queues.
- Native application packaging or distribution through an app store.
- Changing the Hub public URL, Passkey RP ID, Session TTL, Peer protocol, or TLS termination.
- Persisting Push subscriptions on the Hub as part of this change.

## Decisions

### Hand-authored manifest and Service Worker

Place `manifest.json`, `sw.js`, and any dedicated PWA icons in `web/public/`, allowing Vite to copy them unchanged into the embedded distribution. The Service Worker will be plain JavaScript because it must be delivered at `/sw.js` with root scope.

This is preferred over `vite-plugin-pwa`/Workbox because TmuxAtlas needs a small Push worker but deliberately does not need offline application caching. Avoiding a generated caching runtime reduces dependency, update, and stale-shell risk.

### Network-only application behavior

The Service Worker will not install a `fetch` handler or create application caches. Consequently, documents, hashed frontend assets, `/api/*`, `/ws/*`, authentication paths, and terminal traffic retain normal browser/network semantics. The manifest and Service Worker still provide installation and Push functionality without presenting a misleading offline shell.

Pre-caching a shell was considered, but rejected because a cached release can mismatch a newly updated Hub binary and an offline shell cannot perform any meaningful TmuxAtlas operation.

### Push notifications reuse the existing worker

The same root-scoped Service Worker registered by `usePushNotifications` will parse the Hub's JSON Push payload and call `showNotification` with branded icons and a same-origin destination stored in notification data. A notification click will focus an existing same-origin client when possible, navigate it to the destination, or open a new window.

Malformed or incomplete payloads will fall back to a generic TmuxAtlas notification and `/`, preventing Push events from failing silently. Notification destinations must remain relative/same-origin so remote payloads cannot turn notification clicks into open redirects.

### Install controls live in the application

A reusable React hook will capture `beforeinstallprompt`, detect standalone display mode, and expose install availability. Settings will present the durable install entry point. On browsers that expose a native prompt, the UI invokes it only after a user gesture. On iOS/iPadOS, where no programmatic prompt exists, the UI presents concise “Share → Add to Home Screen” guidance. The entry is hidden or marked installed in standalone mode.

An automatic prompt on first load was rejected because browsers may suppress it and it interrupts Passkey setup and terminal use.

### Manifest identity remains bound to the Hub origin

The manifest uses `/` for `id`, `start_url`, and `scope`, with `display: standalone`, the existing dark theme/background colors, and `any` plus dedicated `maskable` icon declarations. Relative same-origin values ensure each Hub installation remains associated with its own public origin and therefore with its existing Passkey RP ID.

### Asset misses are not SPA routes

The Go frontend handler will continue serving `index.html` for extensionless application routes such as `/settings`, `/setup`, and `/session/...`. When a requested path looks like a static asset and the embedded file is absent, it will return `404`. `/sw.js` will be served as JavaScript with revalidation-friendly cache headers; `manifest.json` will be served as JSON/manifest content.

This preserves deep-link navigation while exposing broken build references and correct MIME behavior.

## Risks / Trade-offs

- [Browser PWA and install-prompt behavior differs across platforms] → Feature-detect capabilities, keep manual iOS guidance, and avoid treating prompt availability as a requirement for using TmuxAtlas.
- [A Service Worker can outlive a particular frontend release] → Keep it network-neutral, avoid caches, use immediate activation/client claiming, and make the worker backward-compatible with missing Push fields.
- [Push notification clicks may refer to a session that no longer exists] → Navigate to the best-known session path and allow the existing application to fall back to its normal session/overview behavior.
- [Maskable icons can be cropped by launchers] → Generate dedicated safe-zone assets rather than labeling an edge-to-edge icon as maskable without verification.
- [Static-route hardening could break legitimate deep links containing dots] → Define SPA routes explicitly or use asset-extension detection covered by Go tests, including session names containing dots.

## Migration Plan

1. Add and validate the manifest, worker, install UI, icons, and routing behavior.
2. Build the frontend into the embedded Go distribution and run Go, frontend, and Playwright tests.
3. Publish through the normal tagged release workflow; no server-side data migration is needed.
4. After Hub update/restart, browsers discover the manifest and worker on the next load. Existing Passkeys and cookies remain scoped to the same origin, although a normal Hub restart clears in-memory Sessions.
5. Roll back by installing the prior TmuxAtlas release. Because this design creates no application caches, the prior UI is not shadowed by cached assets; an already registered worker remains harmless and is replaced or can be unregistered by the browser.

## Open Questions

None.
