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

validate_public_url() {
	candidate="$1"
	case "$candidate" in
		http://* | https://*) ;;
		*) return 1 ;;
	esac
	case "$candidate" in
		*" "* | *"	"* | *"?"* | *"#"* | *"@"*) return 1 ;;
	esac
	authority="${candidate#*://}"
	[ -n "$authority" ] || return 1
	case "$authority" in
		*/*) return 1 ;;
	esac
	case "$candidate" in
		https://*) return 0 ;;
		http://localhost | http://localhost:* | http://127.0.0.1 | http://127.0.0.1:* | http://\[::1\] | http://\[::1\]:*)
			return 0
			;;
	esac
	return 1
}

prompt_role() {
	role="${TMUXATLAS_ROLE:-}"
	if [ -z "$role" ]; then
		if ! (: </dev/tty) 2>/dev/null; then
			role="binary"
			log "No interactive terminal; installing the binary only."
			return
		fi
		while :; do
			printf '%s\n' 'How will this machine run TmuxAtlas?' >/dev/tty
			printf '%s\n' '  1) Hub    — Web UI and inbound peer connections' >/dev/tty
			printf '%s\n' '  2) Agent  — outbound-only tmux agent' >/dev/tty
			printf '%s\n' '  3) Binary — install without configuration or service' >/dev/tty
			printf 'Select [1-3]: ' >/dev/tty
			IFS= read -r answer </dev/tty || answer=""
			case "$answer" in
				1 | hub | Hub) role="hub"; break ;;
				2 | agent | Agent) role="agent"; break ;;
				3 | binary | Binary) role="binary"; break ;;
				*) printf 'Enter 1, 2, or 3.\n' >/dev/tty ;;
			esac
		done
	fi
	case "$role" in
		hub | server) role="hub" ;;
		agent) ;;
		binary | none) role="binary" ;;
		*) fail "TMUXATLAS_ROLE must be hub, agent, or binary" ;;
	esac
}

prompt_public_url() {
	public_url="${TMUXATLAS_PUBLIC_URL:-}"

	if [ -z "$public_url" ]; then
		if ! (: </dev/tty) 2>/dev/null; then
			fail "TMUXATLAS_PUBLIC_URL is required in non-interactive mode"
		fi
		while :; do
			printf 'Public URL for browser access (for example https://tmuxatlas.example.com): ' >/dev/tty
			IFS= read -r public_url </dev/tty || public_url=""
			if validate_public_url "$public_url"; then
				break
			fi
			printf '%s\n' 'Enter an HTTPS URL, or an HTTP localhost URL for local-only use.' >/dev/tty
		done
	elif ! validate_public_url "$public_url"; then
		fail "TMUXATLAS_PUBLIC_URL must be HTTPS, or HTTP on localhost"
	fi
}

prompt_hub_url() {
	hub_url="${TMUXATLAS_HUB:-}"
	if [ -z "$hub_url" ]; then
		if ! (: </dev/tty) 2>/dev/null; then
			fail "TMUXATLAS_HUB is required for agent installation"
		fi
		while :; do
			printf 'Trusted Hub URL (for example https://tmuxatlas.example.com): ' >/dev/tty
			IFS= read -r hub_url </dev/tty || hub_url=""
			if validate_public_url "$hub_url"; then
				break
			fi
			printf '%s\n' 'Enter an HTTPS URL, or an HTTP localhost URL for local testing.' >/dev/tty
		done
	elif ! validate_public_url "$hub_url"; then
		fail "TMUXATLAS_HUB must be HTTPS, or HTTP on localhost"
	fi
}

prompt_pair_code() {
	pair_code="${TMUXATLAS_PAIR_CODE:-}"
	if [ -z "$pair_code" ]; then
		if ! (: </dev/tty) 2>/dev/null; then
			fail "TMUXATLAS_PAIR_CODE is required for agent installation"
		fi
		printf 'One-time pairing code from the Hub: ' >/dev/tty
		IFS= read -r pair_code </dev/tty || pair_code=""
	fi
	[ -n "$pair_code" ] || fail "a pairing code is required for agent installation"
}

save_environment() {
	config_dir="${HOME}/.config/tmuxatlas"
	env_file="${config_dir}/.env"
	mkdir -p "$config_dir"
	temp_env="${temp_dir}/tmuxatlas.env"

	if [ -f "$env_file" ]; then
		case "$role" in
			hub) awk '$0 !~ /^[[:space:]]*TMUXATLAS_PUBLIC_URL=/' "$env_file" >"$temp_env" ;;
			agent) awk '$0 !~ /^[[:space:]]*TMUXATLAS_HUB=/' "$env_file" >"$temp_env" ;;
		esac
	else
		: >"$temp_env"
	fi
	case "$role" in
		hub) printf 'TMUXATLAS_PUBLIC_URL=%s\n' "$public_url" >>"$temp_env" ;;
		agent) printf 'TMUXATLAS_HUB=%s\n' "$hub_url" >>"$temp_env" ;;
	esac
	umask 077
	install -m 0600 "$temp_env" "$env_file"
	log "Saved ${role} configuration in ${env_file}"
}

configure_tmux() {
	tmux_config="${TMUXATLAS_TMUX_CONF:-${HOME}/.tmux.conf}"

	if grep -Eq '^[[:space:]]*set(-option)?[[:space:]]+-g[[:space:]]+mouse[[:space:]]+on([[:space:]]|$)' "$tmux_config" 2>/dev/null; then
		log "tmux mouse support is already enabled in ${tmux_config}"
	else
		{
			printf '\n'
			printf '%s\n' '# >>> TmuxAtlas >>>'
			printf '%s\n' '# Enable mouse scrolling and selection in the web terminal.'
			printf '%s\n' 'set -g mouse on'
			printf '%s\n' '# <<< TmuxAtlas <<<'
		} >>"$tmux_config"
		log "Enabled tmux mouse support in ${tmux_config}"
	fi

	if command -v tmux >/dev/null 2>&1 && tmux source-file "$tmux_config" >/dev/null 2>&1; then
		log "Reloaded the active tmux configuration."
	else
		log "The setting will apply when tmux next starts."
	fi
}

prompt_tmux_configuration() {
	answer="${TMUXATLAS_CONFIGURE_TMUX:-}"

	if [ -z "$answer" ]; then
		if ! (: </dev/tty) 2>/dev/null; then
			log "Skipping tmux configuration in non-interactive mode."
			log "Set TMUXATLAS_CONFIGURE_TMUX=yes to enable it automatically."
			return
		fi

		while :; do
			printf 'Enable tmux mouse scrolling in %s? [Y/n] ' \
				"${TMUXATLAS_TMUX_CONF:-${HOME}/.tmux.conf}" >/dev/tty
			IFS= read -r answer </dev/tty || answer=""
			case "$answer" in
				"" | y | Y | yes | YES | Yes)
					answer="yes"
					break
					;;
				n | N | no | NO | No)
					answer="no"
					break
					;;
				*) printf 'Please answer yes or no.\n' >/dev/tty ;;
			esac
		done
	fi

	case "$answer" in
		1 | true | TRUE | True | y | Y | yes | YES | Yes) configure_tmux ;;
		0 | false | FALSE | False | n | N | no | NO | No)
			log "Skipped tmux configuration."
			;;
		*) fail "TMUXATLAS_CONFIGURE_TMUX must be yes or no" ;;
	esac
}

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"
prompt_role
case "$role" in
	hub) prompt_public_url ;;
	agent)
		prompt_hub_url
		prompt_pair_code
		;;
esac

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
if [ "$role" != "binary" ]; then
	save_environment
fi

log "Installed ${install_dir}/tmuxatlas"
case ":${PATH}:" in
	*":${install_dir}:"*) ;;
	*) log "Add ${install_dir} to PATH before running tmuxatlas." ;;
esac
prompt_tmux_configuration

case "$role" in
	hub)
		"${install_dir}/tmuxatlas" install --mode server --public-url "$public_url"
		;;
	agent)
		"${install_dir}/tmuxatlas" pair --hub "$hub_url" --code "$pair_code"
		"${install_dir}/tmuxatlas" install --mode agent --hub "$hub_url"
		;;
	binary)
		log "Binary-only installation complete; no configuration or service was created."
		;;
esac
