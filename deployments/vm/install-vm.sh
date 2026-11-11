#!/usr/bin/env bash
# install-vm.sh — run ON the target VM (with sudo) to install Agent Manager.
# Usage:
#   sudo ./install-vm.sh --host <PUBLIC_IP> --version <amp-release> \
#                        [--email <addr>] [--no-external-gateways] \
#                        [--registry <host/org>] [--registry-credentials <file>]
#
# --host: the VM's PUBLIC IPv4 address. Public URLs are derived as
#   *.amp.<IP>.sslip.io, and a cloud VM usually can't read its own public IP
#   (it is NAT'd), so pass it explicitly — you already know it (you used it to
#   SSH in).
# --version: the amp/v* release to install (e.g. 0.15.0). Required — the charts
#   and manifests are pulled per-release; there is no sensible default.
#
# --registry / --registry-credentials: install from a private image registry
#   instead of the public default. The credentials file holds
#   AMP_REGISTRY_USERNAME=... and AMP_REGISTRY_PASSWORD=... and should be mode
#   600; they are read from the file rather than the command line because argv
#   is readable by every user on the VM through ps.
#
# TLS is always Let's Encrypt, 443-only: certificates issue via the TLS-ALPN-01
# challenge inside the :443 handshake, so only inbound 443 is required (no port
# 80). The public :443 must reach the VM as raw TCP (no TLS-terminating proxy in
# front). Docker, k3d, kubectl, helm, curl and lsof must already be installed on the
# VM — the installer verifies they are present and exits with install hints if any
# are missing.
set -euo pipefail

VM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# This installer wraps the quick-start installer (install.sh + k3d-config.yaml).
QS_DIR="$(cd "${VM_DIR}/../quick-start" && pwd)"
# shellcheck source=lib-vm.sh
source "${VM_DIR}/lib-vm.sh"
# shellcheck source=lib-bootstrap.sh
source "${VM_DIR}/lib-bootstrap.sh"

VM_IP="" ACME_EMAIL="" EXTERNAL_GATEWAYS="true"
# Capture the amp release from --version or the VERSION env, but keep it out of
# the exported environment until the install step: get.docker.com (and other
# piped installers run during bootstrap) read $VERSION as the version to install
# and fail on the AMP release string.
AMP_VERSION="${VERSION:-}"
unset VERSION 2>/dev/null || true

log() { printf '\033[0;34m[install-vm]\033[0m %s\n' "$*"; }
die() { printf '\033[0;31m[install-vm] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }
require_value() { [[ -n "${2:-}" && "${2:-}" != --* ]] || die "$1 requires a value"; }

REGISTRY_PREFIX=""
REGISTRY_CREDENTIALS_FILE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --host) require_value "$1" "${2:-}"; VM_IP="$2"; shift 2 ;;
    --version) require_value "$1" "${2:-}"; AMP_VERSION="$2"; shift 2 ;;
    --email) require_value "$1" "${2:-}"; ACME_EMAIL="$2"; shift 2 ;;
    --no-external-gateways) EXTERNAL_GATEWAYS="false"; shift ;;
    --registry) require_value "$1" "${2:-}"; REGISTRY_PREFIX="$2"; shift 2 ;;
    --registry-credentials) require_value "$1" "${2:-}"; REGISTRY_CREDENTIALS_FILE="$2"; shift 2 ;;
    -h|--help) grep '^#' "$0" | grep -v '^#!' | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown flag: $1" ;;
  esac
done

[[ "$(id -u)" -eq 0 ]] || \
  die "run with sudo — this installs Docker, opens the firewall and creates the cluster: sudo $0 --host <IP> --version <release>"
if [[ -n "${REGISTRY_CREDENTIALS_FILE:-}" && ! -r "$REGISTRY_CREDENTIALS_FILE" ]]; then
  die "--registry-credentials file not readable: ${REGISTRY_CREDENTIALS_FILE}"
fi
[[ -n "$VM_IP" ]] || die "--host <PUBLIC_IP> is required (the VM's public IPv4 — sslip.io hostnames embed it)"
[[ "$VM_IP" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || \
  die "--host must be an IPv4 address (got '${VM_IP}')"
[[ -n "$AMP_VERSION" ]] || \
  die "--version <release> is required (an existing amp/v* tag, e.g. --version 0.15.0); see https://github.com/wso2/agent-manager/tags"

start_caddy() {
  mkdir -p /opt/amp

  # amp-api auto-generates thunder-ask-secret (see the wso2-agent-manager
  # chart's secret.yaml) once the "amp" release exists — which it always does
  # by this point, since start_caddy only runs after run_install's base
  # installer returns. Read it back so Caddy can present it on its own
  # on-demand-TLS ask calls (see caddyfile()'s ask_secret param), isolating
  # that traffic's rate-limit budget from the public internet's, which also
  # reaches /internal/thunder-ask through the api host's own catch-all route.
  # Left empty (never fatal) if the secret or key is missing — e.g. an
  # operator-supplied existingSecret override with no such key — the ask call
  # still works, it just falls back to sharing the public budget.
  local thunder_ask_secret=""
  thunder_ask_secret="$(kubectl get secret amp-api -n wso2-amp -o jsonpath='{.data.thunder-ask-secret}' 2>/dev/null | base64 -d 2>/dev/null)" || thunder_ask_secret=""

  render_caddyfile "$VM_IP" "$ACME_EMAIL" "$EXTERNAL_GATEWAYS" "$thunder_ask_secret" >/opt/amp/Caddyfile
  log "Wrote /opt/amp/Caddyfile"

  docker rm -f amp-caddy >/dev/null 2>&1 || true
  docker run -d --name amp-caddy --restart unless-stopped \
    --network host \
    -v amp-caddy-data:/data \
    -v amp-caddy-config:/config \
    -v /opt/amp/Caddyfile:/etc/caddy/Caddyfile:ro \
    caddy:2
  verify_caddy_up
  prewarm_fixed_host_certs "$VM_IP" "$EXTERNAL_GATEWAYS"
}

run_install() {
  # Build the override arrays install.sh honors.
  # shellcheck disable=SC2034  # arrays are inherited by the subshell that sources install.sh
  mapfile -t AMP_HELM_ARGS < <(build_amp_helm_args "$VM_IP" "$EXTERNAL_GATEWAYS")
  # shellcheck disable=SC2034
  mapfile -t THUNDER_HELM_ARGS < <(build_thunder_helm_args "$VM_IP")
  # shellcheck disable=SC2034
  mapfile -t GATEWAY_HELM_ARGS < <(build_gateway_helm_args "$VM_IP")
    # shellcheck disable=SC2034
  mapfile -t OBSERVABILITY_HELM_ARGS < <(build_observability_helm_args "$VM_IP")
  # install.sh builds chart refs + raw manifest URLs from amp/v${VERSION}; export it
  # only now, after bootstrap, so the piped installers above never saw it.
  export VERSION="$AMP_VERSION"

  # Private-registry handoff: install.sh sources lib-registry-auth.sh, which reads
  # these. Exported only when given, so a public install sees an unchanged
  # environment and keeps its existing behaviour exactly.
  if [[ -n "${REGISTRY_PREFIX:-}" ]]; then
    export AMP_IMAGE_REGISTRY="$REGISTRY_PREFIX"
  fi
  if [[ -n "${REGISTRY_CREDENTIALS_FILE:-}" ]]; then
    export AMP_REGISTRY_CREDENTIALS_FILE="$REGISTRY_CREDENTIALS_FILE"
  fi

  # Suppress install.sh's localhost completion URLs — they are unreachable on a VM
  # (k3d ports are loopback-bound). This script prints the public sslip.io URLs below.
  export SHOW_LOCALHOST_URLS=false

  # Loopback-bound k3d config.
  render_k3d_vm_config <"${QS_DIR}/k3d-config.yaml" >/tmp/k3d-config-vm.yaml
  export K3D_CONFIG=/tmp/k3d-config-vm.yaml

  # CoreDNS rewrites pointed at the k3d server node (not host.k3d.internal), so
  # in-cluster name resolution still reaches the service ports after they are
  # loopback-bound. CLUSTER_NAME is fixed to "amp-local" in install.sh, so the
  # single server node is always "k3d-amp-local-server-0".
  render_coredns_vm_config "k3d-amp-local-server-0" >/tmp/coredns-amp-vm.yaml
  export COREDNS_FILE=/tmp/coredns-amp-vm.yaml

  log "Running base installer with sslip.io overrides (https)"
  # Subshell: install.sh's exit calls stay contained; arrays are inherited.
  # The `|| rc=$?` keeps the subshell out of set -e so we capture its status.
  local rc=0
  ( set +e; source "${QS_DIR}/install.sh" ) || rc=$?
  [[ "$rc" -eq 0 ]] || die "Base installer exited $rc"

  start_caddy
}

log "Phase 1/2: preflight (verify tools + firewall)"
verify_prerequisites
ensure_inotify_limits
ensure_firewall 443
ensure_disk

log "Phase 2/2: install Agent Manager + start Caddy (this takes 8-15 min)"
run_install

log "Done. Access URLs:"
cat <<EOF

  Console:   https://$(vm_host console "$VM_IP")
  API:       https://$(vm_host api "$VM_IP")
  Thunder:   https://$(vm_host thunder "$VM_IP")
  Observer:  https://$(vm_host observer "$VM_IP")
  OTel ingest: https://$(vm_host gateway "$VM_IP")/otel
  Deployed agents: https://<org>-<project>.agents.${VM_IP}.sslip.io/...
EOF
[[ "$EXTERNAL_GATEWAYS" == "true" ]] && echo "  Gateway control plane: https://$(vm_host cp "$VM_IP")  (connect external gateways here; registration token is secret-bearing)"

# install.sh's own admin-credentials print is suppressed by SHOW_LOCALHOST_URLS=false
# above (it only knows the unreachable localhost console URL), so print it here with
# the real one instead.
bash "${QS_DIR}/../scripts/print-admin-credentials.sh" \
  "https://$(vm_host console "$VM_IP")" "${THUNDER_NS:-amp-thunder}" || true
