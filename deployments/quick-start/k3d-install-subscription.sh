#!/usr/bin/env bash
# k3d-install-subscription.sh — one-command install of the WSO2-supported build
# of Agent Manager on a local k3d cluster, for WSO2 subscription customers.
#
# Attached as its own release asset (like k3d-install.sh) and piped to bash:
#   curl -fsSL <URL>/k3d-install-subscription.sh | bash -s -- --project <project> [--version <version>]
#
# <project> is your registry project: wso2-agent-manager for a subscription, or
# wso2-agent-manager-evaluations for an evaluation. The installer itself comes
# from this release's public bundle on GitHub, as with k3d-install.sh; only the
# Helm charts and Agent Manager images come from registry.wso2.com/<project>/sts,
# pulled by helm and by the cluster with your registry credentials.
#
# Each copy is tied to the release it shipped with, and installs that release or
# one of its updates: 2026.11.0 is the release itself, and 2026.11.0-u<N> its
# update N (images 2026.11.0.<N>). Without --version it installs the newest
# one published to the project, so new updates need no new copy of this script.
#
# Credentials come from AMP_REGISTRY_USERNAME and AMP_REGISTRY_PASSWORD, from a
# file named by AMP_REGISTRY_CREDENTIALS_FILE holding both, or from a prompt on
# the terminal. They are never taken as arguments: argv is readable by every user
# through ps, and lands in shell history.
set -euo pipefail

# Stamped to the release version at build time (see .github/scripts/update-install-helpers.sh).
DEFAULT_VERSION="2026.11.0"
REGISTRY_HOST="${AMP_SUBSCRIPTION_REGISTRY:-registry.wso2.com}"
PROJECTS=(wso2-agent-manager wso2-agent-manager-evaluations)
# This short-term support release sits under sts/ in each project, with its
# charts under helm-charts/ there, apart from the images.
IMAGES_SUBPATH="sts"
CHARTS_SUBPATH="${IMAGES_SUBPATH}/helm-charts"
REPO="${AMP_REPO:-wso2/agent-manager}"
AMP_HOME="${AMP_HOME:-${HOME}/.wso2-agent-manager}"

log() { printf '\033[0;34m[k3d-install]\033[0m %s\n' "$*"; }
warn() { printf '\033[0;33m[k3d-install] WARNING:\033[0m %s\n' "$*" >&2; }
die() { printf '\033[0;31m[k3d-install] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage: curl -fsSL <url>/k3d-install-subscription.sh | bash -s -- --project <project> [options]

Creates a k3d cluster named amp-local and installs the WSO2-supported build of
OpenChoreo and Agent Manager on it, from your WSO2 registry project. Needs Docker
(running), k3d, kubectl, helm, curl and lsof on the host.

  --project        wso2-agent-manager (subscription) or
                   wso2-agent-manager-evaluations (evaluation)
  --version        Version to install: the release this script shipped with
                   (e.g. 2026.11.0) or one of its updates (e.g. 2026.11.0-u2).
                   Default: the newest published to the project
  --download-only  Check the credentials, find the version, download the
                   installer, print where it is, and stop without installing

Registry credentials: AMP_REGISTRY_USERNAME and AMP_REGISTRY_PASSWORD, or
AMP_REGISTRY_CREDENTIALS_FILE, or a prompt on the terminal.
EOF
}

# --- Arguments ----------------------------------------------------------------
PROJECT="" REQUESTED="" LEVEL="" DOWNLOAD_ONLY=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --project) PROJECT="${2:?--project requires a value}"; shift 2 ;;
    --version) REQUESTED="${2:?--version requires a value}"; shift 2 ;;
    --update-level) die "--update-level was replaced by --version (e.g. --version 2026.11.0-u2)" ;;
    --download-only) DOWNLOAD_ONLY=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done

# The release this copy installs. AMP_SUBSCRIPTION_RELEASE is for testing an
# unstamped copy; a customer's copy is already tied to its release.
VERSION="${AMP_SUBSCRIPTION_RELEASE:-$DEFAULT_VERSION}"
VERSION="${VERSION#v}"
# Matched by suffix, not the literal placeholder, which the release stamping rewrites.
[[ "$VERSION" != *-dev ]] || \
  die "this copy of the script is not tied to a release — set AMP_SUBSCRIPTION_RELEASE=<release>"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "AMP_SUBSCRIPTION_RELEASE must look like 2026.11.0 (got '${VERSION}')"

# --version names the release (level 0) or an update of it (2026.11.0-u2). The
# image form of an update (2026.11.0.2) is accepted too, since that is what
# pods show.
if [[ -n "$REQUESTED" ]]; then
  REQUESTED="${REQUESTED#v}"
  RELEASE_RE="${VERSION//./\\.}"
  if [[ "$REQUESTED" =~ ^${RELEASE_RE}$ ]]; then
    LEVEL=0
  elif [[ "$REQUESTED" =~ ^${RELEASE_RE}(-u|\.)([1-9][0-9]*)$ ]]; then
    LEVEL="${BASH_REMATCH[2]}"
  elif [[ "$REQUESTED" =~ ^([0-9]+\.[0-9]+\.[0-9]+)((-u|\.)[1-9][0-9]*)?$ ]]; then
    die "${REQUESTED} belongs to release ${BASH_REMATCH[1]}, and this installer installs ${VERSION} and its updates. Use https://github.com/${REPO}/releases/download/amp/v${BASH_REMATCH[1]}/k3d-install-subscription.sh"
  else
    die "--version must be ${VERSION} or one of its updates, such as ${VERSION}-u1 (got '${REQUESTED}')"
  fi
fi

# A piped script has the script itself on stdin, so prompts read the terminal.
have_tty() { [[ -r /dev/tty ]] && { : </dev/tty; } 2>/dev/null; }

if [[ -z "$PROJECT" ]]; then
  have_tty || die "--project is required: one of ${PROJECTS[*]}"
  printf 'Which registry project did WSO2 give you?\n  1) wso2-agent-manager              (subscription)\n  2) wso2-agent-manager-evaluations  (evaluation)\nChoose 1 or 2: ' >&2
  read -r choice </dev/tty
  case "$choice" in
    1) PROJECT="${PROJECTS[0]}" ;;
    2) PROJECT="${PROJECTS[1]}" ;;
    *) die "expected 1 or 2" ;;
  esac
fi
case " ${PROJECTS[*]} " in
  *" ${PROJECT} "*) ;;
  *) die "unknown project '${PROJECT}': expected one of ${PROJECTS[*]}" ;;
esac

# --- Prerequisites (the same checks as k3d-install.sh) ------------------------
log "Checking prerequisites..."
if [[ "$DOWNLOAD_ONLY" == false ]]; then
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
  required=(k3d kubectl helm curl tar lsof)
else
  required=(helm curl tar)
fi
missing=()
for tool in "${required[@]}"; do
  command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
done
[[ ${#missing[@]} -eq 0 ]] || \
  die "missing required tools: ${missing[*]} — see https://wso2.com/agent-platform/docs/v${VERSION}/guides/on-k3d/#prerequisites"

# --- Credentials --------------------------------------------------------------
# The file is parsed, not sourced, so a stray line in it cannot run anything.
if [[ -n "${AMP_REGISTRY_CREDENTIALS_FILE:-}" ]] && [[ -z "${AMP_REGISTRY_USERNAME:-}" || -z "${AMP_REGISTRY_PASSWORD:-}" ]]; then
  [[ -f "$AMP_REGISTRY_CREDENTIALS_FILE" ]] || die "credentials file not found: ${AMP_REGISTRY_CREDENTIALS_FILE}"
  while IFS='=' read -r key value || [[ -n "$key" ]]; do
    value="${value%$'\r'}"; value="${value#\'}"; value="${value%\'}"; value="${value#\"}"; value="${value%\"}"
    case "$key" in
      AMP_REGISTRY_USERNAME) [[ -n "${AMP_REGISTRY_USERNAME:-}" ]] || AMP_REGISTRY_USERNAME="$value" ;;
      AMP_REGISTRY_PASSWORD) [[ -n "${AMP_REGISTRY_PASSWORD:-}" ]] || AMP_REGISTRY_PASSWORD="$value" ;;
    esac
  done < "$AMP_REGISTRY_CREDENTIALS_FILE"
fi
if [[ -z "${AMP_REGISTRY_USERNAME:-}" || -z "${AMP_REGISTRY_PASSWORD:-}" ]]; then
  have_tty || die "no registry credentials: set AMP_REGISTRY_USERNAME and AMP_REGISTRY_PASSWORD, or AMP_REGISTRY_CREDENTIALS_FILE"
  if [[ -z "${AMP_REGISTRY_USERNAME:-}" ]]; then
    printf 'Registry username for %s: ' "$REGISTRY_HOST" >&2
    IFS= read -r AMP_REGISTRY_USERNAME </dev/tty
  fi
  if [[ -z "${AMP_REGISTRY_PASSWORD:-}" ]]; then
    printf 'Registry token for %s: ' "$REGISTRY_HOST" >&2
    IFS= read -rs AMP_REGISTRY_PASSWORD </dev/tty
    printf '\n' >&2
  fi
fi
[[ -n "${AMP_REGISTRY_USERNAME:-}" && -n "${AMP_REGISTRY_PASSWORD:-}" ]] || die "registry username and token are both required"

# --- Registry access ----------------------------------------------------------
# Every request to the registry goes through helm, with the token on stdin and a
# throwaway registry config, so the caller's own helm logins are left untouched.
# The bundled installer signs helm in again for the install itself.
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
export HELM_REGISTRY_CONFIG="${WORK}/helm-registry.json"
HELM_REGISTRY_FLAGS=()
if [[ "${AMP_REGISTRY_ALLOW_INSECURE:-false}" == true ]]; then
  HELM_REGISTRY_FLAGS+=(--plain-http)   # test registries only
fi

log "Signing in to ${REGISTRY_HOST}"
if ! printf '%s' "$AMP_REGISTRY_PASSWORD" | helm registry login "$REGISTRY_HOST" \
    --username "$AMP_REGISTRY_USERNAME" --password-stdin ${HELM_REGISTRY_FLAGS[@]+"${HELM_REGISTRY_FLAGS[@]}"} >/dev/null 2>"$WORK/err"; then
  die "${REGISTRY_HOST} rejected the registry credentials: $(tail -1 "$WORK/err")"
fi

# --- Pick the version ---------------------------------------------------------
# The release is chart version <release> and its update N is <release>-uN. To
# helm, -uN is a prerelease, so every update sorts before the release itself, and
# "u10" is compared as text, so it sorts before "u9". The newest update is
# therefore the highest version below the release, counted up from there one
# level at a time; the publish workflow never skips a level, so the first one
# missing ends the count. --devel is what lets helm consider prereleases at all.
CHART_REF="oci://${REGISTRY_HOST}/${PROJECT}/${CHARTS_SUBPATH}/wso2-agent-manager"

# Succeeds if a chart matches $1 (a version or a range) and leaves its
# Chart.yaml in $WORK/chart.yaml; fails if none does; stops on any other error.
show_chart() {
  if helm show chart "$CHART_REF" --devel --version "$1" \
      ${HELM_REGISTRY_FLAGS[@]+"${HELM_REGISTRY_FLAGS[@]}"} >"$WORK/chart.yaml" 2>"$WORK/err"; then
    return 0
  fi
  if grep -qiE 'not found|could not locate|no matching|name unknown|not known to registry' "$WORK/err"; then
    return 1
  fi
  if grep -qiE 'unauthori|denied|forbidden|401|403' "$WORK/err"; then
    die "the credentials have no access to ${PROJECT}: check the registry project and the token"
  fi
  die "could not read ${CHART_REF}: $(tail -1 "$WORK/err")"
}

# Chart version for level $1.
chart_version_for() {
  if [[ "$1" -eq 0 ]]; then echo "$VERSION"; else echo "${VERSION}-u$1"; fi
}

# Prints the newest level published, or nothing if none is.
newest_level() {
  local found n
  if show_chart ">=${VERSION}-u <${VERSION}"; then
    found="$(sed -n 's/^version:[[:space:]]*//p' "$WORK/chart.yaml" | head -1)"
    [[ "$found" =~ ^${VERSION//./\\.}-u([1-9][0-9]*)$ ]] || \
      die "unexpected chart version '${found}' for release ${VERSION} in ${PROJECT}"
    n="${BASH_REMATCH[1]}"
    while show_chart "$(chart_version_for $((n + 1)))"; do n=$((n + 1)); done
    echo "$n"
  elif show_chart "$VERSION"; then
    echo 0
  fi
}

# A named version is looked up directly. Finding the newest needs the registry
# to list tags, so for a named version it only adds to the messages and is never
# allowed to stop the install.
if [[ -z "$LEVEL" ]]; then
  LEVEL="$(newest_level)"
  [[ -n "$LEVEL" ]] || die "nothing of release ${VERSION} is published to ${PROJECT} yet"
  NEWEST_LEVEL="$LEVEL"
else
  if ! show_chart "$(chart_version_for "$LEVEL")"; then
    NEWEST_LEVEL="$(newest_level 2>/dev/null)" || NEWEST_LEVEL=""
    die "$(chart_version_for "$LEVEL") is not published to ${PROJECT}${NEWEST_LEVEL:+; the newest is $(chart_version_for "$NEWEST_LEVEL")}"
  fi
  NEWEST_LEVEL="$(newest_level 2>/dev/null)" || NEWEST_LEVEL=""
fi
CHART_VERSION="$(chart_version_for "$LEVEL")"
if [[ "$LEVEL" -eq 0 ]]; then TAG="$VERSION"; else TAG="${VERSION}.${LEVEL}"; fi
log "Using ${PROJECT} ${CHART_VERSION} (images ${TAG})"
if [[ -n "$NEWEST_LEVEL" && "$LEVEL" -ne "$NEWEST_LEVEL" ]]; then
  log "  ${PROJECT} also has a newer version: $(chart_version_for "$NEWEST_LEVEL")"
fi

# --- Download the installer ---------------------------------------------------
# The same public release bundle k3d-install.sh uses; it holds no credentials and
# is the same for every update level of the release.
BUNDLE_URL="${AMP_BUNDLE_URL:-https://github.com/${REPO}/releases/download/amp/v${VERSION}/wso2-agent-manager-${VERSION}.tar.gz}"
DIST_DIR="${AMP_HOME}/subscription/${PROJECT}/${TAG}"
# Start from an empty directory, so files left by an earlier download cannot
# mix with this bundle or satisfy the checks below on its behalf.
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"
BUNDLE="${DIST_DIR}/wso2-agent-manager-${VERSION}.tar.gz"
log "Downloading install bundle (release ${VERSION})"
log "  ${BUNDLE_URL}"
curl -fsSL --retry 5 --retry-all-errors -o "$BUNDLE" "$BUNDLE_URL" \
  || die "failed to download the install bundle — check the release tag amp/v${VERSION} exists (or set AMP_BUNDLE_URL)"
tar -xzf "$BUNDLE" -C "$DIST_DIR" || die "failed to unpack the install bundle"
QUICK_START_DIR="${DIST_DIR}/deployments/quick-start"
[[ -f "${QUICK_START_DIR}/install.sh" ]] || die "the install bundle is missing deployments/quick-start/install.sh"
# Installing from a private registry needs the installer's registry support,
# including the helm sign-in for private charts. A release bundle without it
# would fail part way, so stop here instead.
grep -q 'amp_registry_helm_login' "${DIST_DIR}/deployments/scripts/lib-registry-auth.sh" 2>/dev/null || \
  die "the ${VERSION} install bundle cannot install from a private registry; it needs a newer release of the installer"
log "Installer unpacked in ${DIST_DIR}"

if [[ "$DOWNLOAD_ONLY" == true ]]; then
  log "Download only: to install, run"
  log "  cd ${QUICK_START_DIR} && AMP_IMAGE_REGISTRY=${REGISTRY_HOST}/${PROJECT}/${IMAGES_SUBPATH} GATEWAY_REGISTRY=${REGISTRY_HOST}/${PROJECT}/${IMAGES_SUBPATH} HELM_CHART_REGISTRY=${REGISTRY_HOST}/${PROJECT}/${CHARTS_SUBPATH} VERSION=${CHART_VERSION} ./install.sh"
  log "with AMP_REGISTRY_USERNAME and AMP_REGISTRY_PASSWORD (or AMP_REGISTRY_CREDENTIALS_FILE) set."
  exit 0
fi

# --- Install ------------------------------------------------------------------
# k3d's DNS fix rewrites the node's resolv.conf to the Docker host gateway, which
# does not resolve anything on Colima; the guided install asks Colima users to
# turn it off, so do the same here.
if [[ -z "${K3D_FIX_DNS:-}" ]] && docker info --format '{{.Name}}' 2>/dev/null | grep -qi colima; then
  log "Colima detected — setting K3D_FIX_DNS=0"
  export K3D_FIX_DNS=0
fi

# Point the installer at the project for this update. These are set explicitly,
# so values left in the shell from an earlier guided install cannot redirect it.
# The installer takes the credentials from here for the node's registry
# configuration, the image pull secrets and its own helm sign-in.
export AMP_IMAGE_REGISTRY="${REGISTRY_HOST}/${PROJECT}/${IMAGES_SUBPATH}"
export HELM_CHART_REGISTRY="${REGISTRY_HOST}/${PROJECT}/${CHARTS_SUBPATH}"
# Each project carries the gateway beside AMP, so one robot pulls both.
export GATEWAY_REGISTRY="${REGISTRY_HOST}/${PROJECT}/${IMAGES_SUBPATH}"
export VERSION="$CHART_VERSION"
export AMP_REGISTRY_USERNAME AMP_REGISTRY_PASSWORD
unset AMP_REGISTRY_CREDENTIALS_FILE HELM_REGISTRY_CONFIG GATEWAY_REGISTRY_USERNAME GATEWAY_REGISTRY_PASSWORD

log "Installing Agent Manager ${CHART_VERSION} on k3d (~15-20 minutes)"
cd "$QUICK_START_DIR"
bash ./install.sh
log "Installed WSO2 Agent Manager ${CHART_VERSION} (images ${TAG}) from ${REGISTRY_HOST}/${PROJECT}/${IMAGES_SUBPATH}"
