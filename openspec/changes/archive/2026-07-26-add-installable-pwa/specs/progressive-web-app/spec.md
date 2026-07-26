## ADDED Requirements

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
The Hub SHALL serve a root-scoped Service Worker as JavaScript, and the Web UI SHALL register it through the same origin without requiring an offline cache.

#### Scenario: Service Worker registration
- **WHEN** a supported browser enables Push notifications
- **THEN** `/sw.js` is returned with a JavaScript-compatible content type and registration succeeds with root scope

#### Scenario: New worker version activates
- **WHEN** a browser discovers an updated TmuxAtlas Service Worker
- **THEN** the new worker becomes active without leaving application requests dependent on an obsolete cached shell

### Requirement: Push notification presentation
The Service Worker SHALL handle valid and malformed Push events, display a branded notification, and associate each notification with a safe same-origin navigation destination.

#### Scenario: Valid Hub Push payload arrives
- **WHEN** the Service Worker receives a valid TmuxAtlas Push payload
- **THEN** it displays the supplied title and body with TmuxAtlas branding and stores the corresponding same-origin session destination

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
