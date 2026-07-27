# progressive-web-app Specification

## Purpose
Define the installable Progressive Web App behavior for TmuxAtlas, including installation metadata and guidance, Service Worker lifecycle, Push notification handling, network freshness, static resource routing, and origin security.

## Requirements

### Requirement: Installable application metadata
The Hub SHALL serve a valid same-origin Web App Manifest describing TmuxAtlas with a stable application identity, root start URL and scope, standalone display mode, theme and background colors, and branded `any` and `maskable` icons.

#### Scenario: Browser retrieves the manifest
- **WHEN** a client requests the manifest from a running Hub
- **THEN** the Hub returns the manifest with a successful status, a manifest-compatible content type, and installable metadata whose URLs remain within that Hub origin

#### Scenario: Installed application launches
- **WHEN** a user launches an installed TmuxAtlas application
- **THEN** the browser opens the Hub root within a standalone application window

### Requirement: User-controlled installation experience
The Web UI SHALL expose an installation entry point when installation is relevant, invoke a native installation prompt only from a user gesture when the browser supports it, and provide manual installation guidance on supported Apple mobile platforms that do not expose a prompt.

#### Scenario: Chromium install prompt is available
- **WHEN** the browser emits an install prompt and the user activates the TmuxAtlas install control
- **THEN** the Web UI invokes the captured native prompt and reflects whether installation was accepted or dismissed

#### Scenario: Apple mobile browser requires manual installation
- **WHEN** an iPhone or iPad user opens the install entry while TmuxAtlas is not running standalone
- **THEN** the Web UI explains how to use Share and Add to Home Screen without claiming that installation was completed

#### Scenario: Application is already installed
- **WHEN** TmuxAtlas is running in standalone display mode
- **THEN** the Web UI does not offer another native installation action

### Requirement: Service Worker delivery and lifecycle
The Hub SHALL serve a root-scoped Service Worker as JavaScript, and the Web UI SHALL register and reconcile it through the same origin without requiring an offline cache. The UI SHALL treat Service Worker and Push registration as confirmed only after the browser and Hub operations required for the current state have succeeded.

#### Scenario: Service Worker registration
- **WHEN** a supported authenticated browser loads the TmuxAtlas application or enables Push notifications
- **THEN** `/sw.js` is returned with a JavaScript-compatible content type and registration succeeds with root scope

#### Scenario: Existing browser subscription is reconciled
- **WHEN** the application starts with an existing `PushManager` subscription
- **THEN** the Web UI submits that subscription to the Hub and reports `subscribed` only after the Hub confirms persistence

#### Scenario: Subscription reconciliation fails
- **WHEN** the browser has a local subscription but the Hub registration request fails
- **THEN** the Web UI exposes a retryable state and does not suppress fallback notifications by falsely reporting `subscribed`

#### Scenario: New worker version activates
- **WHEN** a browser discovers an updated TmuxAtlas Service Worker
- **THEN** the new worker becomes active without leaving application requests dependent on an obsolete cached shell

### Requirement: Push notification presentation
The Service Worker SHALL handle valid and malformed Push events, display a branded notification, and associate each notification with a safe same-origin navigation destination. A valid session notification SHALL preserve the stable host, session, window/pane, tool, and status identity supplied by the Hub so notifications from different hosts do not collide or navigate to the wrong session.

#### Scenario: Valid Hub Push payload arrives
- **WHEN** the Service Worker receives a valid TmuxAtlas Push payload
- **THEN** it displays the supplied title and body with TmuxAtlas branding and stores the corresponding same-origin host/session destination

#### Scenario: Remote session Push payload arrives
- **WHEN** a valid payload identifies host `host-a` and session `project`
- **THEN** the notification destination is `/session/host-a/project` with each path segment safely encoded, and its tag distinguishes it from `project` on any other host

#### Scenario: Malformed Push payload arrives
- **WHEN** the Service Worker cannot parse a Push payload or required fields are absent
- **THEN** it displays a generic TmuxAtlas notification whose destination is the Hub root

#### Scenario: Notification is clicked
- **WHEN** the user clicks a TmuxAtlas Push notification
- **THEN** the Service Worker focuses and navigates an existing same-origin application window when possible, or opens a new same-origin window at the notification destination

### Requirement: Live traffic is never served from a Service Worker cache
The PWA SHALL preserve network-only semantics for documents, frontend assets, authentication, API requests, WebSockets, terminal streams, and Peer-related browser traffic.

#### Scenario: Terminal and API traffic is active
- **WHEN** the installed application performs an API request or establishes a WebSocket connection
- **THEN** the request reaches the current Hub and is not fulfilled from or queued by a Service Worker cache

#### Scenario: Hub is unreachable
- **WHEN** the installed application cannot reach its Hub
- **THEN** it exposes the normal connection failure behavior and does not present cached terminal or authentication state as live

#### Scenario: Hub is upgraded
- **WHEN** a user reloads after the Hub binary has been updated
- **THEN** the browser obtains the frontend served by the current Hub release rather than an older Service Worker-cached application shell

### Requirement: Static resource misses are explicit
The embedded frontend server SHALL distinguish SPA navigation routes from missing static resources so that absent PWA and asset files do not receive the SPA HTML document.

#### Scenario: Existing PWA resource is requested
- **WHEN** a client requests the manifest, Service Worker, or declared icon
- **THEN** the Hub returns the exact embedded resource with its correct content type

#### Scenario: Missing asset-like path is requested
- **WHEN** a client requests a nonexistent JavaScript, JSON, image, stylesheet, or other asset-like path
- **THEN** the Hub returns `404` instead of `index.html`

#### Scenario: SPA deep link is requested
- **WHEN** a client directly requests a supported TmuxAtlas application route
- **THEN** the Hub returns the SPA document so the React application can resolve the route

### Requirement: Installed mode preserves origin security
Installing TmuxAtlas SHALL NOT change the Hub origin used for Passkeys, cookies, APIs, WebSockets, or Push subscriptions.

#### Scenario: Installed user authenticates
- **WHEN** a user opens installed TmuxAtlas and signs in with an existing Passkey
- **THEN** WebAuthn uses the same public Hub origin and RP ID as the browser version

#### Scenario: Hub is served through a trusted HTTPS gateway
- **WHEN** TmuxAtlas is installed from a Hub exposed through Cloudflare Tunnel or an HTTPS reverse proxy
- **THEN** PWA, Passkey, and Push browser APIs operate against that HTTPS public origin without requiring a separate application origin

### Requirement: Durable Push subscription store
The Hub SHALL persist valid Push subscriptions in its user configuration directory with user-private permissions and atomic replacement. Subscriptions SHALL survive normal Hub restarts, be deduplicated by endpoint, and be removed when the browser unsubscribes or the Push service permanently rejects the endpoint.

#### Scenario: Hub restarts with stored subscriptions
- **WHEN** the Hub starts after one or more subscriptions were successfully persisted
- **THEN** those subscriptions are available to the Push sender before any browser is reopened

#### Scenario: Browser registers the same endpoint again
- **WHEN** reconciliation submits an endpoint already present in the store
- **THEN** the Hub atomically updates/deduplicates the record without creating duplicate delivery

#### Scenario: Push service expires an endpoint
- **WHEN** delivery returns a permanent not-found or gone response
- **THEN** the Hub atomically removes that endpoint from the durable store

### Requirement: Notification preference consistency
The Hub Push sender SHALL evaluate the current `preferences.notifications.statuses` for every candidate event, and browser fallback notifications SHALL use the same enabled-status semantics. Waiting、Error 和 Completed SHALL be delivered only when enabled.

#### Scenario: Completed notifications are enabled
- **WHEN** a tool transitions to Completed and `completed` is enabled
- **THEN** each valid subscribed endpoint receives one Completed Push notification, or the active page uses one fallback notification when Push is not active

#### Scenario: Waiting notifications are disabled
- **WHEN** a tool transitions to Waiting while `waiting` is absent from preferences
- **THEN** neither the server Push path nor the page fallback creates a Waiting notification

#### Scenario: Preferences change while Hub is running
- **WHEN** the administrator changes enabled statuses before a later event
- **THEN** the later event uses the updated preferences without requiring Hub restart or resubscription

### Requirement: Host-aware Push payload contract
The Hub SHALL include stable host ID、host display name、session、window、pane、tool 和 status in Push payloads when those fields are known. Session-targeted payloads missing required stable identity SHALL fall back to the Hub root rather than guessing a local session.

#### Scenario: Same session name exists on two hosts
- **WHEN** both hosts produce notification-worthy events for session `work`
- **THEN** the generated payloads, tags, and click destinations remain distinct by stable host ID

#### Scenario: Required target identity is missing
- **WHEN** a payload cannot identify a safe host/session destination
- **THEN** the Service Worker uses `/` and does not navigate to a potentially unrelated local session

### Requirement: Persistent Push and PWA lifecycle verification
The project SHALL include automated coverage for real application Service Worker registration, browser-to-Hub reconciliation, durable store restart, preference filtering, host-aware navigation, malformed payload fallback, subscription expiry, and continued network-only behavior.

#### Scenario: PWA and Push suites run in CI
- **WHEN** CI runs Go and Playwright PWA tests
- **THEN** tests exercise the application registration path rather than only registering the worker from test code, and verify that no Service Worker application cache or fetch interception is introduced
