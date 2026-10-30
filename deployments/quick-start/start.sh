#!/usr/bin/env bash
# start.sh — one-command entry point for the Agent Manager quick-start dev container.
#
# Downloaded/attached as its own release asset (like deployments/vm/bootstrap.sh),
# so it can be fetched and run directly on the host:
#   curl -fsSL <URL>/start.sh -o start.sh && chmod +x start.sh && ./start.sh
#
# It only checks host prerequisites and launches the dev container; install.sh
# is not run automatically — run it yourself from the container shell once it
# starts.
set -euo pipefail

# Stamped to the release version at build time (see .github/scripts/update-install-helpers.sh
# for the equivalent 2026.11.0 -> version substitution pattern applied to this file).
DEFAULT_VERSION="2026.11.0"
IMAGE="${QUICK_START_IMAGE:-${AMP_IMAGE_REGISTRY:-ghcr.io/wso2}/amp-quick-start}"

log() { printf '\033[0;34m[start]\033[0m %s\n' "$*"; }
warn() { printf '\033[0;33m[start] WARNING:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[0;31m[start] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage: ./start.sh [--version vX.Y.Z]

Checks local prerequisites, then launches the Agent Manager quick-start dev
container. Run ./install.sh from the container shell to install the platform.

  --version   Quick-start image tag to run (default: the version this script
              shipped with, or $QUICK_START_VERSION if set)
EOF
}

VERSION="${QUICK_START_VERSION:-$DEFAULT_VERSION}"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="${2:?--version requires a value}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done
VERSION="${VERSION#v}"

log "Checking prerequisites..."

command -v docker >/dev/null 2>&1 || \
  die "Docker is required but was not found on PATH. Install Docker before continuing: https://docs.docker.com/get-docker/"

if ! docker info >/dev/null 2>&1; then
  msg="Docker is installed but the daemon is not reachable (docker info failed)."
  if [[ "$(uname -s)" == "Darwin" ]]; then
    msg+=$'\n'"On macOS, start Colima with a dedicated profile:"
    msg+=$'\n'"  colima start --profile agent-manager --vm-type=vz --vz-rosetta --network-address --cpu 4 --memory 8"
  else
    msg+=$'\n'"Make sure the Docker daemon is running (e.g. 'sudo systemctl start docker')."
  fi
  die "$msg"
fi
log "Docker is installed and reachable"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64|arm64|aarch64) log "Detected architecture: ${ARCH}" ;;
  *) warn "Unrecognized architecture '${ARCH}' — the quick-start image is published for amd64/arm64 only" ;;
esac

# This image is pulled by the HOST's Docker daemon, so it needs credentials of
# its own: a cluster-side pull secret does not apply, and the cryptic
# "denied: requested access to the resource is denied" is what a missing login
# looks like. Kept self-contained rather than sourcing
# deployments/scripts/lib-registry-auth.sh, because start.sh is fetched and run
# as a standalone release asset with no siblings on disk.
REGISTRY_PREFIX="${AMP_IMAGE_REGISTRY:-ghcr.io/wso2}"
if [[ "$REGISTRY_PREFIX" != "ghcr.io/wso2" ]]; then
  REGISTRY_HOST="${REGISTRY_PREFIX%%/*}"
  if [[ -n "${AMP_REGISTRY_USERNAME:-}" && -n "${AMP_REGISTRY_PASSWORD:-}" ]]; then
    log "Authenticating to ${REGISTRY_HOST}"
    printf '%s' "$AMP_REGISTRY_PASSWORD" \
      | docker login "$REGISTRY_HOST" -u "$AMP_REGISTRY_USERNAME" --password-stdin >/dev/null \
      || die "docker login to ${REGISTRY_HOST} failed. Check AMP_REGISTRY_USERNAME and AMP_REGISTRY_PASSWORD."
  else
    log "Using the Docker credentials already on this host for ${REGISTRY_HOST}"
    log "If the pull is denied, run: docker login ${REGISTRY_HOST}"
  fi
fi

log "Starting quick-start dev container (${IMAGE}:v${VERSION})"
log "Run ./install.sh from the container shell to install the platform (~15-20 minutes)."
# install.sh runs inside the container and reads these, so a private-registry
# install has to carry them across. Without this the container defaulted back to
# the public registry and installed charts with no pull secret at all.
#
# A credentials file is mounted read-only rather than passed as environment,
# because `docker inspect` shows a container's environment to anyone who can
# reach the daemon.
DOCKER_ARGS=()
if [[ "$REGISTRY_PREFIX" != "ghcr.io/wso2" ]]; then
  DOCKER_ARGS+=(-e "AMP_IMAGE_REGISTRY=${REGISTRY_PREFIX}")
  if [[ -n "${AMP_REGISTRY_CREDENTIALS_FILE:-}" && -f "${AMP_REGISTRY_CREDENTIALS_FILE}" ]]; then
    DOCKER_ARGS+=(-v "${AMP_REGISTRY_CREDENTIALS_FILE}:/run/amp-registry.creds:ro"
                  -e "AMP_REGISTRY_CREDENTIALS_FILE=/run/amp-registry.creds")
  elif [[ -n "${AMP_REGISTRY_USERNAME:-}" && -n "${AMP_REGISTRY_PASSWORD:-}" ]]; then
    warn "Passing registry credentials as container environment variables; 'docker inspect' will show them. Set AMP_REGISTRY_CREDENTIALS_FILE to mount a file instead."
    DOCKER_ARGS+=(-e "AMP_REGISTRY_USERNAME=${AMP_REGISTRY_USERNAME}"
                  -e "AMP_REGISTRY_PASSWORD=${AMP_REGISTRY_PASSWORD}")
  fi
  if [[ "${AMP_REGISTRY_ALLOW_INSECURE:-false}" == "true" ]]; then
    DOCKER_ARGS+=(-e "AMP_REGISTRY_ALLOW_INSECURE=true")
  fi
fi

exec docker run --rm -it --name amp-quick-start \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --network=host \
  ${DOCKER_ARGS[@]+"${DOCKER_ARGS[@]}"} \
  "${IMAGE}:v${VERSION}"
