#!/bin/sh
set -eu

image="${TMUXATLAS_TEST_IMAGE:-tmuxatlas:ci}"
project="${TMUXATLAS_TEST_PROJECT:-tmuxatlas-ci}"
volume="${TMUXATLAS_TEST_VOLUME:-${project}-data}"
port="${TMUXATLAS_TEST_PORT:-17655}"

export TMUXATLAS_IMAGE="${image%:*}"
export TMUXATLAS_IMAGE_TAG="${image##*:}"
export TMUXATLAS_PULL_POLICY=never
export TMUXATLAS_PUBLIC_URL=https://staging.example.com
export TMUXATLAS_ORIGIN_PORT="$port"
export TMUXATLAS_VOLUME="$volume"

cleanup() {
	docker compose -p "$project" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

docker compose -p "$project" config >/dev/null

metadata="$(docker image inspect "$image" --format '{{.Config.User}} {{json .Config.Entrypoint}} {{json .Config.Cmd}}')"
case "$metadata" in
	'65532:65532 ["/usr/local/bin/tmuxatlas"] ["hub"]') ;;
	*) echo "unexpected image metadata: $metadata" >&2; exit 1 ;;
esac

docker compose -p "$project" up -d --wait
container="$(docker compose -p "$project" ps -q hub)"

test "$(docker inspect "$container" --format '{{.HostConfig.ReadonlyRootfs}}')" = "true"
test "$(docker inspect "$container" --format '{{json .HostConfig.CapDrop}}')" = '["ALL"]'
curl -fsS -H 'Host: staging.example.com' "http://127.0.0.1:${port}/api/version" >/dev/null
docker compose -p "$project" exec -T hub tmuxatlas healthcheck --role hub --deployment docker
docker compose -p "$project" exec -T hub tmuxatlas pair >/dev/null

if docker exec "$container" /bin/sh -c true >/dev/null 2>&1; then
	echo "runtime image unexpectedly contains a shell" >&2
	exit 1
fi
if docker exec "$container" tmux -V >/dev/null 2>&1; then
	echo "runtime image unexpectedly contains tmux" >&2
	exit 1
fi

before="$(docker logs "$container" 2>&1 | sed -n 's/.*"fingerprint":"\([^"]*\)".*/\1/p' | tail -1)"
test -n "$before"
docker compose -p "$project" up -d --force-recreate --wait
container="$(docker compose -p "$project" ps -q hub)"
after="$(docker logs "$container" 2>&1 | sed -n 's/.*"fingerprint":"\([^"]*\)".*/\1/p' | tail -1)"
test "$before" = "$after"

docker stop --time 15 "$container" >/dev/null
test "$(docker inspect "$container" --format '{{.State.ExitCode}}')" = "0"

echo "Container smoke test passed (persistent fingerprint: $after)"
