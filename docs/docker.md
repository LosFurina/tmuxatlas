# Docker Hub deployment

The official container runs the remote-only TmuxAtlas Hub. It serves the Web
UI, Passkey authentication, Peer coordination, and remote terminal proxying,
but it deliberately contains no tmux or shell. Run `tmuxatlas agent` on every
machine that contributes tmux sessions.

## Start

Copy the example and set the final browser-facing HTTPS origin before enrolling
the first Passkey:

```sh
cp .env.docker.example .env
sed -i.bak 's#https://tmuxatlas.example.com#https://tmuxatlas.your-domain.example#' .env
docker compose pull
docker compose up -d
docker compose ps
```

The Compose service publishes the origin only on `127.0.0.1` and stores all
durable state in one named volume. Keep exactly one Hub container attached to
that volume: the JSON stores use a single-writer model.

Read the one-time bootstrap token from the initial log, open the configured
public URL, and enroll the administrator Passkey:

```sh
docker compose logs tmuxatlas
```

Pair each host from the Hub UI or pairing command, then run the native Agent on
that host. Do not install tmux or mount its socket into the Hub container.

## Trusted gateway

Cloudflare Tunnel can target the loopback origin:

```yaml
ingress:
  - hostname: tmuxatlas.example.com
    service: http://127.0.0.1:7654
  - service: http_status:404
```

No Cloudflare Access layer is required. Cloudflare terminates trusted TLS and
proxies HTTP/WebSocket traffic to the loopback origin.

An Nginx+ACME gateway needs WebSocket upgrade forwarding:

```nginx
location / {
    proxy_pass http://127.0.0.1:7654;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto https;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

`TMUXATLAS_PUBLIC_URL` must exactly match the final browser origin. Changing its
hostname after enrollment invalidates the WebAuthn relying-party binding and
requires a new Passkey bootstrap.

## Health and diagnostics

Compose uses the private Unix socket health endpoint rather than exposing a
public unauthenticated probe:

```sh
docker compose ps
docker compose exec hub tmuxatlas healthcheck --role hub --deployment docker
docker compose logs --tail=100 hub
```

The runtime is non-root, read-only, drops all Linux capabilities, has no shell,
and writes only to its XDG state/config/runtime directories in the volume or
tmpfs.

## Backup and restore

Stop the writer before taking a consistent archive:

```sh
docker compose stop tmuxatlas
docker run --rm \
  -v tmuxatlas-data:/source:ro \
  -v "$PWD":/backup \
  alpine tar -C /source -czf /backup/tmuxatlas-data.tgz .
docker compose start tmuxatlas
```

Restore only into an empty volume while the Hub is stopped:

```sh
docker volume create tmuxatlas-data-restored
docker run --rm \
  -v tmuxatlas-data-restored:/target \
  -v "$PWD":/backup:ro \
  alpine tar -C /target -xzf /backup/tmuxatlas-data.tgz
```

Set `TMUXATLAS_VOLUME=tmuxatlas-data-restored` before starting the restored Hub.
Never run `docker compose down -v` unless permanent deletion of Passkeys,
peer identities, push keys, and all Hub state is intentional.

## Native-to-Docker migration

1. Stop the native Hub service so there is only one writer.
2. Back up `~/.config/tmuxatlas` and `~/.local/share/tmuxatlas`.
3. Create the target named volume.
4. Copy the existing config and data contents into the matching XDG directories
   under `/var/lib/tmuxatlas` in the volume.
5. Start Compose with the same `TMUXATLAS_PUBLIC_URL`.
6. Verify `tmuxatlas healthcheck`, Passkey login, Agent presence, and a remote
   PTY before disabling the old service permanently.

Keep the backup and native service definition until the Docker deployment has
been verified. Do not run native and Docker Hubs against the same state.

## Update and rollback

Container updates replace the immutable image; they never patch the binary
inside the container:

```sh
docker compose pull
docker compose up -d
docker compose ps
```

Pin `TMUXATLAS_IMAGE_TAG` to an exact release such as `v0.10.0` for predictable
production rollouts. For stronger supply-chain pinning, replace the image tag
with a published `@sha256:` digest.

Rollback by restoring the previous tag or digest and recreating the service:

```sh
TMUXATLAS_IMAGE_TAG=v0.9.2 docker compose up -d
```

The persistent volume is retained across image recreation. Take a backup before
cross-version changes and confirm release notes before rolling state back to an
older binary.
