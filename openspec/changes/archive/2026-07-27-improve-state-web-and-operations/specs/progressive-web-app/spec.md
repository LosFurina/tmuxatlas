## MODIFIED Requirements

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

## ADDED Requirements

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
