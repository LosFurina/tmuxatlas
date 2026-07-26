#!/bin/sh

set -eu

repository="${TMUXATLAS_REPOSITORY:-LosFurina/tmuxatlas}"
install_dir="${TMUXATLAS_INSTALL_DIR:-${HOME}/.local/bin}"
version="${TMUXATLAS_VERSION:-}"

log() {
	printf '%s\n' "$*"
}

fail() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"

case "$(uname -s)" in
	Darwin) os="darwin" ;;
	Linux) os="linux" ;;
	*) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
	x86_64 | amd64) arch="amd64" ;;
	arm64 | aarch64) arch="arm64" ;;
	*) fail "unsupported architecture: $(uname -m)" ;;
esac

if [ -z "$version" ]; then
	version="$(
		curl -fsSL \
			-H "Accept: application/vnd.github+json" \
			"https://api.github.com/repos/${repository}/releases?per_page=20" |
			sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' |
			head -n 1
	)"
fi

[ -n "$version" ] || fail "no GitHub release was found for ${repository}"

release_version="${version#v}"
archive="tmuxatlas-v${release_version}-${os}-${arch}.tar.gz"
base_url="https://github.com/${repository}/releases/download/${version}"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT HUP INT TERM

log "Installing tmuxatlas ${version} for ${os}/${arch}..."
curl -fL --retry 3 -o "${temp_dir}/${archive}" "${base_url}/${archive}"
curl -fL --retry 3 -o "${temp_dir}/checksums.txt" "${base_url}/checksums.txt"

expected="$(
		awk -v filename="$archive" '$2 == filename { print $1 }' \
			"${temp_dir}/checksums.txt"
)"
[ -n "$expected" ] || fail "release checksum does not contain ${archive}"

if command -v sha256sum >/dev/null 2>&1; then
	actual="$(sha256sum "${temp_dir}/${archive}" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
	actual="$(shasum -a 256 "${temp_dir}/${archive}" | awk '{ print $1 }')"
else
	fail "sha256sum or shasum is required to verify the download"
fi

[ "$actual" = "$expected" ] || fail "checksum verification failed for ${archive}"

tar -xzf "${temp_dir}/${archive}" -C "$temp_dir"
[ -f "${temp_dir}/tmuxatlas" ] || fail "release archive does not contain the tmuxatlas binary"

mkdir -p "$install_dir"
install -m 0755 "${temp_dir}/tmuxatlas" "${install_dir}/tmuxatlas"

log "Installed ${install_dir}/tmuxatlas"
case ":${PATH}:" in
	*":${install_dir}:"*) ;;
	*) log "Add ${install_dir} to PATH before running tmuxatlas." ;;
esac
log "Run 'tmuxatlas install' if you also want a systemd/launchd user service."
