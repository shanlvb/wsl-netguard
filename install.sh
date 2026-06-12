#!/usr/bin/env bash
set -euo pipefail

GITHUB_REPO_DEFAULT="shanlvb/wsl-netguard"
RAW_INSTALL_URL="https://raw.githubusercontent.com/${GITHUB_REPO_DEFAULT}/main/install.sh"

repo="${WSL_NETGUARD_REPO:-${GITHUB_REPO_DEFAULT}}"
version="${WSL_NETGUARD_VERSION:-latest}"
enable_after_install=0
install_deps=1
skip_wsl_check="${WSL_NETGUARD_SKIP_WSL_CHECK:-0}"

usage() {
	cat <<USAGE
Install wsl-netguard from GitHub Releases.

Usage:
  install.sh [options]

Options:
  --version <tag>       Install a specific release tag, for example v0.1.0.
  --repo <owner/name>   Install from another GitHub repository.
  --enable              Enable network interception after installation.
  --no-deps             Do not install common OS packages.
  --skip-wsl-check      Allow running outside WSL.
  -h, --help            Show this help.

Environment:
  WSL_NETGUARD_VERSION  Release tag to install. Defaults to latest.
  WSL_NETGUARD_REPO     GitHub repository. Defaults to ${GITHUB_REPO_DEFAULT}.
  GH_TOKEN              Optional GitHub token for private repositories.

Online installer:
  curl -fsSL ${RAW_INSTALL_URL} | bash
USAGE
}

log() {
	printf 'wsl-netguard: %s\n' "$*" >&2
}

die() {
	log "error: $*"
	exit 1
}

need_cmd() {
	command -v "$1" >/dev/null 2>&1 || die "$1 is required"
}

run_sudo() {
	if [[ "${EUID}" -eq 0 ]]; then
		"$@"
		return
	fi

	need_cmd sudo
	sudo "$@"
}

parse_args() {
	while [[ "$#" -gt 0 ]]; do
		case "$1" in
		--version)
			[[ "$#" -ge 2 ]] || die "--version requires a value"
			version="$2"
			shift 2
			;;
		--version=*)
			version="${1#*=}"
			shift
			;;
		--repo)
			[[ "$#" -ge 2 ]] || die "--repo requires a value"
			repo="$2"
			shift 2
			;;
		--repo=*)
			repo="${1#*=}"
			shift
			;;
		--enable)
			enable_after_install=1
			shift
			;;
		--no-deps)
			install_deps=0
			shift
			;;
		--skip-wsl-check)
			skip_wsl_check=1
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		*)
			die "unknown option: $1"
			;;
		esac
	done
}

validate_inputs() {
	if [[ ! "${repo}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
		die "invalid GitHub repository ${repo}; expected owner/name"
	fi
	if [[ "${version}" != "latest" && ! "${version}" =~ ^[A-Za-z0-9._-]+$ ]]; then
		die "invalid release tag ${version}; use letters, numbers, dot, underscore, or dash"
	fi
}

is_wsl() {
	[[ -r /proc/sys/kernel/osrelease ]] && grep -qiE '(microsoft|wsl)' /proc/sys/kernel/osrelease
}

require_wsl() {
	if [[ "${skip_wsl_check}" == "1" ]]; then
		return
	fi
	if ! is_wsl; then
		die "this installer is intended for WSL; use --skip-wsl-check only for packaging or tests"
	fi
}

install_common_packages() {
	if [[ "${install_deps}" -eq 0 ]]; then
		log "skipping OS package installation"
		return
	fi

	if command -v apt-get >/dev/null 2>&1; then
		log "installing common runtime packages with apt-get"
		run_sudo apt-get update
		run_sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y \
			ca-certificates curl tar coreutils iproute2 nftables
		return
	fi

	if command -v dnf >/dev/null 2>&1; then
		log "installing common runtime packages with dnf"
		run_sudo dnf install -y ca-certificates curl tar coreutils iproute nftables
		return
	fi

	if command -v yum >/dev/null 2>&1; then
		log "installing common runtime packages with yum"
		run_sudo yum install -y ca-certificates curl tar coreutils iproute nftables
		return
	fi

	if command -v zypper >/dev/null 2>&1; then
		log "installing common runtime packages with zypper"
		run_sudo zypper --non-interactive install ca-certificates curl tar coreutils iproute2 nftables
		return
	fi

	if command -v pacman >/dev/null 2>&1; then
		log "installing common runtime packages with pacman"
		run_sudo pacman -Sy --needed --noconfirm ca-certificates curl tar coreutils iproute2 nftables
		return
	fi

	log "no supported package manager found; continuing without OS package installation"
}

require_download_tools() {
	for cmd in awk curl grep mktemp sha256sum tar uname; do
		need_cmd "${cmd}"
	done
}

warn_for_missing_runtime() {
	local missing=()
	for cmd in sing-box nft ip systemctl; do
		if ! command -v "${cmd}" >/dev/null 2>&1; then
			missing+=("${cmd}")
		fi
	done

	if [[ "${#missing[@]}" -eq 0 ]]; then
		return
	fi

	log "warning: missing runtime commands: ${missing[*]}"
	if [[ "${enable_after_install}" -eq 1 ]]; then
		log "the --enable step is expected to fail until these commands are installed"
	else
		log "install them before running: sudo wsl-netguard enable"
	fi
}

warn_if_systemd_is_not_running() {
	if [[ -d /run/systemd/system ]]; then
		return
	fi

	if [[ "${enable_after_install}" -eq 1 ]]; then
		log "warning: systemd is not running; the --enable step is expected to fail"
	else
		log "warning: systemd is not running; install can complete, but enable will fail until systemd is available"
	fi
}

detect_arch() {
	case "$(uname -m)" in
	x86_64 | amd64)
		printf 'amd64\n'
		;;
	aarch64 | arm64)
		printf 'arm64\n'
		;;
	*)
		die "unsupported architecture: $(uname -m)"
		;;
	esac
}

curl_args() {
	local token="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
	printf '%s\0' -fsSL --retry 3 --connect-timeout 15
	if [[ -n "${token}" ]]; then
		printf '%s\0' -H "Authorization: Bearer ${token}"
	fi
}

github_get() {
	local url="$1"
	local args=()
	while IFS= read -r -d '' arg; do
		args+=("${arg}")
	done < <(curl_args)

	curl "${args[@]}" "${url}"
}

download_to() {
	local url="$1"
	local output="$2"
	local args=()
	while IFS= read -r -d '' arg; do
		args+=("${arg}")
	done < <(curl_args)

	curl "${args[@]}" -o "${output}" "${url}"
}

resolve_version() {
	if [[ "${version}" != "latest" ]]; then
		printf '%s\n' "${version}"
		return
	fi

	local api_url="https://api.github.com/repos/${repo}/releases/latest"
	local tag
	tag="$(github_get "${api_url}" | sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
	[[ -n "${tag}" ]] || die "could not resolve latest release for ${repo}"
	printf '%s\n' "${tag}"
}

verify_checksum() {
	local checksums_path="$1"
	local asset_name="$2"
	local work_dir="$3"
	local checksum_line

	checksum_line="$(awk -v file="${asset_name}" '$2 == file { print; found = 1 } END { if (!found) exit 1 }' "${checksums_path}")" ||
		die "checksums.txt does not contain ${asset_name}"

	printf '%s\n' "${checksum_line}" | (cd "${work_dir}" && sha256sum -c -)
}

validate_archive_paths() {
	local archive_path="$1"
	local unsafe_entry

	if unsafe_entry="$(tar -tzf "${archive_path}" | awk 'BEGIN { unsafe = 0 } (/^\/|(^|\/)\.\.(\/|$)/) { if (!unsafe) print; unsafe = 1 } END { exit unsafe ? 0 : 1 }')"; then
		die "release archive contains unsafe path: ${unsafe_entry}"
	fi
}

install_release() {
	local resolved_version="$1"
	local arch="$2"
	local tmp_dir="$3"
	local asset_name="wsl-netguard_${resolved_version}_linux_${arch}.tar.gz"
	local release_url="https://github.com/${repo}/releases/download/${resolved_version}/${asset_name}"
	local checksums_url="https://github.com/${repo}/releases/download/${resolved_version}/checksums.txt"
	local archive_path="${tmp_dir}/${asset_name}"
	local checksums_path="${tmp_dir}/checksums.txt"
	local extracted_dir="${tmp_dir}/wsl-netguard_${resolved_version}_linux_${arch}"
	local binary_path="${extracted_dir}/wsl-netguard"

	log "downloading ${asset_name}"
	download_to "${release_url}" "${archive_path}"
	download_to "${checksums_url}" "${checksums_path}"

	log "verifying ${asset_name} with checksums.txt"
	verify_checksum "${checksums_path}" "${asset_name}" "${tmp_dir}"

	validate_archive_paths "${archive_path}"
	tar -xzf "${archive_path}" -C "${tmp_dir}"
	[[ -x "${binary_path}" || -f "${binary_path}" ]] || die "release archive does not contain wsl-netguard binary"
	chmod +x "${binary_path}"

	if [[ "${enable_after_install}" -eq 1 ]]; then
		log "running wsl-netguard install --enable"
		log "--enable applies the managed nftables kill switch and starts systemd services"
		run_sudo "${binary_path}" install --enable
	else
		log "running wsl-netguard install"
		run_sudo "${binary_path}" install
		log "network interception is not enabled; run: sudo wsl-netguard enable"
	fi
}

main() {
	parse_args "$@"
	validate_inputs
	require_wsl
	install_common_packages
	require_download_tools
	warn_if_systemd_is_not_running
	warn_for_missing_runtime

	local arch
	local resolved_version
	local tmp_dir

	arch="$(detect_arch)"
	resolved_version="$(resolve_version)"
	if [[ ! "${resolved_version}" =~ ^[A-Za-z0-9._-]+$ ]]; then
		die "resolved release tag ${resolved_version} is not supported by this installer"
	fi

	tmp_dir="$(mktemp -d)"
	trap 'rm -rf "${tmp_dir}"' EXIT

	install_release "${resolved_version}" "${arch}" "${tmp_dir}"
	log "done"
}

main "$@"
