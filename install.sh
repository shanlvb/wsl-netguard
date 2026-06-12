#!/usr/bin/env bash
set -euo pipefail

GITHUB_REPO_DEFAULT="shanlvb/wsl-netguard"
RAW_INSTALL_URL="https://raw.githubusercontent.com/${GITHUB_REPO_DEFAULT}/main/install.sh"
SING_BOX_REPO="SagerNet/sing-box"
SING_BOX_VERSION="v1.13.13"
MANAGED_LIB_DIR="/usr/local/lib/wsl-netguard"
MANAGED_SING_BOX_PATH="/usr/local/lib/wsl-netguard/sing-box"

repo="${WSL_NETGUARD_REPO:-${GITHUB_REPO_DEFAULT}}"
version="${WSL_NETGUARD_VERSION:-latest}"
enable_after_install=0
install_deps=1
install_sing_box="${WSL_NETGUARD_INSTALL_SING_BOX:-1}"
skip_wsl_check="${WSL_NETGUARD_SKIP_WSL_CHECK:-0}"
github_proxy="${WSL_NETGUARD_GITHUB_PROXY:-}"

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
  --no-sing-box         Do not install the managed sing-box runtime.
  --github-proxy <url>  Prefix GitHub URLs with a trusted mirror, for example https://gh-proxy.com/.
  --skip-wsl-check      Allow running outside WSL.
  -h, --help            Show this help.

Environment:
  WSL_NETGUARD_VERSION       Release tag to install. Defaults to latest.
  WSL_NETGUARD_REPO          GitHub repository. Defaults to ${GITHUB_REPO_DEFAULT}.
  WSL_NETGUARD_INSTALL_SING_BOX
                              Install managed sing-box ${SING_BOX_VERSION} to ${MANAGED_SING_BOX_PATH}.
                              Set to 0 to skip.
  WSL_NETGUARD_GITHUB_PROXY  Optional trusted GitHub URL prefix.
  GH_TOKEN                   Optional GitHub token for private repositories.

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
		--no-sing-box)
			install_sing_box=0
			shift
			;;
		--github-proxy)
			[[ "$#" -ge 2 ]] || die "--github-proxy requires a value"
			github_proxy="$2"
			shift 2
			;;
		--github-proxy=*)
			github_proxy="${1#*=}"
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
	if [[ "${install_sing_box}" != "0" && "${install_sing_box}" != "1" ]]; then
		die "invalid WSL_NETGUARD_INSTALL_SING_BOX=${install_sing_box}; expected 0 or 1"
	fi
	validate_github_proxy
	warn_if_proxy_receives_token
}

validate_github_proxy() {
	if [[ -z "${github_proxy}" ]]; then
		return
	fi
	if [[ ! "${github_proxy}" =~ ^https?://[^[:space:]]+$ ]]; then
		die "invalid GitHub proxy ${github_proxy}; expected an http(s) URL prefix"
	fi
	github_proxy="${github_proxy%/}/"
}

warn_if_proxy_receives_token() {
	if [[ -z "${github_proxy}" ]]; then
		return
	fi
	if [[ -n "${GH_TOKEN:-${GITHUB_TOKEN:-}}" ]]; then
		log "warning: GH_TOKEN/GITHUB_TOKEN will be sent to the configured GitHub proxy"
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
	for cmd in awk curl grep install mktemp sha256sum tar uname; do
		need_cmd "${cmd}"
	done
}

warn_for_missing_runtime() {
	local missing=()
	for cmd in nft ip systemctl; do
		if ! command -v "${cmd}" >/dev/null 2>&1; then
			missing+=("${cmd}")
		fi
	done
	if [[ "${install_sing_box}" -eq 0 && ! -x "${MANAGED_SING_BOX_PATH}" ]]; then
		missing+=("${MANAGED_SING_BOX_PATH}")
	fi

	if [[ "${#missing[@]}" -eq 0 ]]; then
		return
	fi

	log "warning: missing runtime requirements: ${missing[*]}"
	if [[ "${enable_after_install}" -eq 1 ]]; then
		log "the --enable step is expected to fail until these requirements are available"
	else
		log "make them available before running: sudo wsl-netguard enable"
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

proxied_url() {
	local url="$1"
	if [[ -z "${github_proxy}" ]]; then
		printf '%s\n' "${url}"
		return
	fi
	printf '%s%s\n' "${github_proxy}" "${url}"
}

github_get() {
	local url="$1"
	local args=()
	while IFS= read -r -d '' arg; do
		args+=("${arg}")
	done < <(curl_args)

	curl "${args[@]}" "$(proxied_url "${url}")"
}

download_to() {
	local url="$1"
	local output="$2"
	local args=()
	while IFS= read -r -d '' arg; do
		args+=("${arg}")
	done < <(curl_args)

	curl "${args[@]}" -o "${output}" "$(proxied_url "${url}")"
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

sing_box_checksum() {
	local arch="$1"
	case "${arch}" in
	amd64)
		printf 'bb99cabf47694625db421ee17898f36cdc1f9c2cb5decf65b12bac8d8437e842\n'
		;;
	arm64)
		printf 'd7fab87b921933eb281d8ee7bd5377cdd8228089f1f7c807c9363a6a2329286c\n'
		;;
	*)
		die "unsupported sing-box architecture: ${arch}"
		;;
	esac
}

install_sing_box_release() {
	local arch="$1"
	local tmp_dir="$2"

	if [[ "${install_sing_box}" -eq 0 ]]; then
		log "skipping managed sing-box installation"
		return
	fi

	local sing_box_version_number="${SING_BOX_VERSION#v}"
	local asset_name="sing-box-${sing_box_version_number}-linux-${arch}.tar.gz"
	local release_url="https://github.com/${SING_BOX_REPO}/releases/download/${SING_BOX_VERSION}/${asset_name}"
	local archive_path="${tmp_dir}/${asset_name}"
	local extracted_dir="${tmp_dir}/sing-box-${sing_box_version_number}-linux-${arch}"
	local binary_path="${extracted_dir}/sing-box"
	local expected_checksum

	expected_checksum="$(sing_box_checksum "${arch}")"

	log "downloading managed sing-box ${SING_BOX_VERSION}"
	download_to "${release_url}" "${archive_path}"

	log "verifying ${asset_name} with pinned SHA256"
	printf '%s  %s\n' "${expected_checksum}" "${asset_name}" | (cd "${tmp_dir}" && sha256sum -c -)

	validate_archive_paths "${archive_path}"
	tar -xzf "${archive_path}" -C "${tmp_dir}"
	[[ -x "${binary_path}" || -f "${binary_path}" ]] || die "sing-box archive does not contain sing-box binary"
	chmod +x "${binary_path}"

	run_sudo install -d -m 0755 "${MANAGED_LIB_DIR}"
	run_sudo install -m 0755 "${binary_path}" "${MANAGED_SING_BOX_PATH}"
	log "installed managed sing-box ${SING_BOX_VERSION} to ${MANAGED_SING_BOX_PATH}"
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
		log "running wsl-netguard install"
		run_sudo "${binary_path}" install
		install_sing_box_release "${arch}" "${tmp_dir}"
		log "running wsl-netguard enable"
		log "--enable applies the managed nftables kill switch and starts systemd services"
		run_sudo "${binary_path}" enable
	else
		log "running wsl-netguard install"
		run_sudo "${binary_path}" install
		install_sing_box_release "${arch}" "${tmp_dir}"
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
