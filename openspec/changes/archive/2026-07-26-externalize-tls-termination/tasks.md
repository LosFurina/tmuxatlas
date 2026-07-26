## 1. HTTP Origin Configuration

- [x] 1.1 Add `--listen`/`TMUXATLAS_LISTEN` with default `127.0.0.1:7654`, remove the port-only server configuration, and add validation plus listener tests.
- [x] 1.2 Add `--public-url`/`TMUXATLAS_PUBLIC_URL`, validate absolute HTTP(S) URLs, derive Secure Cookie behavior from the configured scheme, and cover HTTP and HTTPS cookie cases with tests.
- [x] 1.3 Update server options and startup logging to use the listen address and public URL, including a warning for an explicitly configured non-loopback plaintext origin.

## 2. Remove Built-in TLS and Certificate UI

- [x] 2.1 Remove server-side TLS setup, listener wrapping, TLS options, certificate fingerprinting, reload watchers, CLI flags, and environment variables.
- [x] 2.2 Remove `pkg/tlscert` and its test suite after all callers have been eliminated.
- [x] 2.3 Remove `/api/tls/status`, `/api/tls/ca.crt`, `/api/tls/ca.mobileconfig`, and mobile configuration generation from the HTTP server.
- [x] 2.4 Remove the Trust Certificate component, `/trust` routing, login links, and any associated frontend state or copy.
- [x] 2.5 Add regression tests confirming default startup does not generate or read TLS files and removed TLS options are rejected.

## 3. Simplify Peer Transport Trust

- [x] 3.1 Replace custom peer TLS configuration with standard Go HTTP and WebSocket transports so HTTPS/WSS uses hostname verification and system roots.
- [x] 3.2 Remove `--insecure`, private CA loading, leaf-certificate pinning, certificate rotation handling, certificate fingerprint helpers, and their tests.
- [x] 3.3 Remove CA material from pairing responses and remove TLS fingerprint suffixes from generated and consumed pairing codes.
- [x] 3.4 Implement an idempotent peer-store migration that backs up legacy data, strips `ca_cert_pem` and `tls_cert_pem`, and preserves Ed25519 identity fields.
- [x] 3.5 Add migration tests plus peer transport tests for system-trusted HTTPS/WSS rejection behavior and explicit local HTTP/WS URLs.
- [x] 3.6 Exercise pairing, peer control synchronization, and remote PTY relay through a generic HTTP/WebSocket reverse-proxy test fixture.

## 4. Deployment and Upgrade Documentation

- [x] 4.1 Update README configuration, startup, installation, and remote-access examples for the loopback HTTP origin and external public URL.
- [x] 4.2 Rewrite multi-host security and pairing documentation to describe trusted gateway TLS plus Ed25519 peer authentication instead of self-signed TLS or mTLS.
- [x] 4.3 Add Cloudflare Tunnel and Nginx+ACME examples covering Host preservation, WebSocket upgrades, query strings, and long-lived connection timeouts.
- [x] 4.4 Document breaking configuration removals, legacy peer-store migration, rollback backup behavior, and manual cleanup of unused TLS files.
- [x] 4.5 Update systemd/launchd installation output and any development proxy defaults to use local HTTP.

## 5. Verification and Cleanup

- [x] 5.1 Run frontend type checking and production build, then run `go test ./...`.
- [x] 5.2 Scan source, UI, tests, and documentation for obsolete self-signed CA, certificate trust, mTLS, TLS flag, pinning, and insecure-verification references.
- [x] 5.3 Validate the OpenSpec change and confirm all proxy-deployment and peer-transport scenarios are covered by tests or documented deployment verification.
