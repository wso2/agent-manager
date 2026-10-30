#!/usr/bin/env bash
# k3d-install.sh — one-command entry point for installing Agent Manager on a
# local k3d cluster, directly on the host (no dev container).
#
# Attached as its own release asset (like bootstrap.sh and start.sh) and piped
# to bash:
#   curl -fsSL <URL>/k3d-install.sh | bash
#
# It checks host prerequisites, downloads ONE versioned install bundle (no git
# clone, no per-file raw.githubusercontent fetches, which GitHub rate-limits per
# IP), unpacks it under ~/.wso2-agent-manager/<version> and runs install.sh from
# it. The unpacked bundle is kept so uninstall.sh is available afterwards.
# Override the bundle location with AMP_BUNDLE_URL (and the repo with AMP_REPO)
# for testing against a fork/pre-release.
set -euo pipefail

# Stamped to the release version at build time (see .github/scripts/update-install-helpers.sh).
DEFAULT_VERSION="2026.11.0"
REPO="${AMP_REPO:-wso2/agent-manager}"
AMP_HOME="${AMP_HOME:-${HOME}/.wso2-agent-manager}"

log() { printf '\033[0;34m[k3d-install]\033[0m %s\n' "$*"; }
warn() { printf '\033[0;33m[k3d-install] WARNING:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[0;31m[k3d-install] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage: curl -fsSL <url>/k3d-install.sh | bash [-s -- --version X.Y.Z]

Creates a k3d cluster named amp-local and installs OpenChoreo and Agent Manager
on it. Needs Docker (running), k3d, kubectl, helm, curl and lsof on the host.

  --version   Release to install (default: the release this script shipped with)
EOF
}

VERSION="$DEFAULT_VERSION"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="${2:?--version requires a value}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done
VERSION="${VERSION#v}"
# Matched by suffix, not the literal placeholder, which the release stamping rewrites.
[[ "$VERSION" != *-dev ]] || \
  die "this copy of the script is not tied to a release — pass --version <release>"

log "Checking prerequisites..."

command -v docker >/dev/null 2>&1 || \
  die "Docker is required but was not found on PATH: https://docs.docker.com/get-docker/"
if ! docker info >/dev/null 2>&1; then
  msg="Docker is installed but the daemon is not reachable (docker info failed)."
  if [[ "$(uname -s)" == "Darwin" ]]; then
    msg+=$'\n'"On macOS, start Colima with a dedicated profile:"
    msg+=$'\n'"  colima start --profile agent-manager --vm-type=vz --vz-rosetta --network-address --cpu 4 --memory 8"
  fi
  die "$msg"
fi

missing=()
for tool in k3d kubectl helm curl tar lsof; do
  command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
done
[[ ${#missing[@]} -eq 0 ]] || \
  die "missing required tools: ${missing[*]} — see https://wso2.com/agent-platform/docs/v${VERSION}/guides/on-k3d/#prerequisites"

# k3d's DNS fix rewrites the node's resolv.conf to the Docker host gateway, which
# does not resolve anything on Colima; the guided install asks Colima users to
# turn it off, so do the same here.
if [[ -z "${K3D_FIX_DNS:-}" ]] && docker info --format '{{.Name}}' 2>/dev/null | grep -qi colima; then
  log "Colima detected — setting K3D_FIX_DNS=0"
  export K3D_FIX_DNS=0
fi

BUNDLE_URL="${AMP_BUNDLE_URL:-https://github.com/${REPO}/releases/download/amp/v${VERSION}/wso2-agent-manager-${VERSION}.tar.gz}"
DIST_DIR="${AMP_HOME}/${VERSION}"
mkdir -p "$DIST_DIR"
BUNDLE="${DIST_DIR}/wso2-agent-manager-${VERSION}.tar.gz"

log "Downloading install bundle (release ${VERSION})"
log "  ${BUNDLE_URL}"
curl -fsSL --retry 5 --retry-all-errors -o "$BUNDLE" "$BUNDLE_URL" \
  || die "failed to download the install bundle — check the release tag amp/v${VERSION} exists (or set AMP_BUNDLE_URL)"
tar -xzf "$BUNDLE" -C "$DIST_DIR" || die "failed to unpack the install bundle"

QUICK_START_DIR="${DIST_DIR}/deployments/quick-start"
[[ -f "${QUICK_START_DIR}/install.sh" ]] || die "install bundle is missing deployments/quick-start/install.sh"

log "Installing Agent Manager ${VERSION} on k3d (~15-20 minutes)"
cd "$QUICK_START_DIR"
bash ./install.sh
