## Why

TmuxAtlas already presents mobile metadata, install icons, and a Service Worker registration path, but the referenced manifest and Service Worker do not exist, so browsers receive the SPA HTML fallback instead of valid PWA resources. Completing the PWA contract will let users install the Hub as a focused app while also making the existing Web Push path functional and predictable.

## What Changes

- Make the TmuxAtlas Web UI installable as a Progressive Web App on supported desktop and mobile browsers.
- Provide a valid Web App Manifest with standalone display settings, branded icons, theme colors, scope, and start URL.
- Provide a Service Worker that supports Web Push and notification-click navigation without caching API, WebSocket, authentication, or terminal traffic.
- Add an in-app install entry point for browsers with a programmatic install prompt and platform-appropriate guidance where installation must be initiated manually.
- Harden embedded static-file routing so missing asset-like paths return `404` instead of the SPA document.
- Add automated coverage for manifest delivery, Service Worker delivery and registration, install UI behavior, notification navigation, and cache exclusions.

## Capabilities

### New Capabilities

- `progressive-web-app`: Installability, standalone presentation, Service Worker behavior, Web Push handling, install guidance, safe caching boundaries, and static PWA resource delivery.

### Modified Capabilities

None.

## Impact

- Frontend entry document, React UI, public assets, PWA metadata, and Service Worker code under `web/`.
- Embedded frontend routing in `pkg/server/`.
- Frontend build output and Playwright/Go test coverage.
- Cloudflare Tunnel and other trusted HTTPS gateways remain compatible; no Hub API, Peer protocol, Passkey credential, or deployment configuration migration is required.
