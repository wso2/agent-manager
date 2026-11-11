#!/usr/bin/env bash
# install-advanced.sh — config-driven Agent Manager install on a VM with Docker.
# Run ON the target VM with sudo. Custom domain, with kgateway terminating TLS on :443
# (no Caddy, no lego). Two TLS modes, both ending at the same Secret:
#   TLS_MODE=dns01 (default) — cert-manager issues and auto-renews a publicly-trusted
#     wildcard cert via the ACME DNS-01 challenge. Works on a public OR private VM:
#     issuance is egress-only (the ACME CA reads a DNS TXT record, never calls the VM).
#   TLS_MODE=byoc — you supply the certificate and key. No ACME, no DNS-provider
#     credential, no egress to a CA — so this also covers air-gapped VMs — but nothing
#     auto-renews, and the cert must carry every SAN this install serves.
# See --init for the annotated config template.
#
# Usage:
#   sudo ./install-advanced.sh --config amp-config.env
#   ./install-advanced.sh --init > amp-config.env      # emit annotated template
#   sudo ./install-advanced.sh --config amp-config.env --dry-run   # validate + render only
#
# To install from a private image registry, set AMP_IMAGE_REGISTRY,
# AMP_REGISTRY_USERNAME and AMP_REGISTRY_PASSWORD in the config file (see the
# template --init writes). --dry-run reports the registry without the token.
set -euo pipefail

VM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# This installer wraps the quick-start installer (install.sh + k3d-config.yaml).
QS_DIR="$(cd "${VM_DIR}/../quick-start" && pwd)"

log() { printf '\033[0;34m[install-advanced]\033[0m %s\n' "$*"; }
die() { printf '\033[0;31m[install-advanced] ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# shellcheck source=lib-vm.sh
source "${VM_DIR}/lib-vm.sh"
# shellcheck source=lib-advanced.sh
source "${VM_DIR}/lib-advanced.sh"
# shellcheck source=lib-bootstrap.sh
source "${VM_DIR}/lib-bootstrap.sh"
# shellcheck source=lib-certmanager.sh
source "${VM_DIR}/lib-certmanager.sh"

print_template() {
  cat <<'TEMPLATE'
# amp-config.env — Agent Manager advanced VM install configuration.
# Sourced by install-advanced.sh. Lines are shell assignments.
#
# kgateway terminates TLS on :443 (there is no Caddy). Pick how the certificate
# it serves is obtained with TLS_MODE — everything downstream is identical.

# --- Required ---
AMP_VERSION=                       # REQUIRED: amp/v* release tag, e.g. 1.0.0
                                   # (see github.com/wso2/agent-manager/releases)
DOMAIN_BASE=amp.mycompany.com      # service hosts derived as <svc>.<DOMAIN_BASE>

# --- Private image registry (optional) ---
# Leave both unset to pull the AMP images from the public registry. When set, the
# installer redirects every first-party image, gives the cluster node registry
# credentials, and creates the image pull secret in each namespace that needs one.
# Third-party images (cert-manager, kgateway, OpenSearch, PostgreSQL) always come
# from their own public registries.
#
# AMP_REGISTRY_PASSWORD is a live credential: keep this file mode 600, as the
# DNS-01 provider tokens below already require. --dry-run never prints it.
#
# QUOTE the username if it contains a '$' — some registries name machine
# accounts that way, e.g. robot$project+name. This file is sourced, so an
# unquoted '$' is expanded by the
# shell and the install fails with an unrelated "unbound variable" error (or, if
# that name happens to be set, silently authenticates as the wrong user).
#AMP_IMAGE_REGISTRY=registry.example.com/my-org
#AMP_REGISTRY_USERNAME='robot$my-org+installer'
#AMP_REGISTRY_PASSWORD=

# --- TLS mode: dns01 (default) or byoc ---
TLS_MODE=dns01

# === TLS_MODE=dns01 — cert-manager issues + auto-renews a publicly-trusted cert ===
# The ACME CA validates by reading a DNS TXT record, so the VM needs NO inbound
# access for issuance (egress only) and this works on a private VM too. You must
# control the DNS zone for DOMAIN_BASE.
#
# cert-manager writes a TXT record to prove control of the zone, then issues a
# wildcard certificate covering every service host and the three dynamic-tier
# wildcards. Set DNS_PROVIDER and that provider's credentials below; the installer
# turns them into the Kubernetes Secret the ClusterIssuer references.
ACME_EMAIL=ops@mycompany.com       # ACME account contact (required for dns01)
DNS_PROVIDER=cloudflare            # cloudflare | route53 | clouddns | azuredns
#   Cloudflare:        CLOUDFLARE_API_TOKEN=...            (scoped Zone.DNS:Edit token)
#   AWS Route53:       AWS_ACCESS_KEY_ID=... AWS_SECRET_ACCESS_KEY=... AWS_REGION=us-east-1
#   Google Cloud DNS:  GCP_PROJECT=... GCP_SERVICE_ACCOUNT_FILE=/opt/amp/gcp-sa.json
#   Azure DNS:         AZURE_TENANT_ID=... AZURE_CLIENT_ID=... AZURE_CLIENT_SECRET=... \
#                      AZURE_SUBSCRIPTION_ID=... AZURE_RESOURCE_GROUP=...
# ACME_SERVER=https://acme-staging-v02.api.letsencrypt.org/directory  # optional: LE staging for testing

# === TLS_MODE=byoc — you supply the certificate ===
# No ACME and no DNS-provider credential, so this works even with no route to a
# public CA. Nothing auto-renews: you must replace the cert before it expires.
# The certificate must carry ALL of these SANs (--dry-run prints the exact list):
#   console/api/thunder/observer/gateway[/cp].<DOMAIN_BASE>
#   *.agents.<DOMAIN_BASE>    *.<DOMAIN_BASE>    *.gateway.<DOMAIN_BASE>
# *.<DOMAIN_BASE> covers per-environment Thunder (handles sit directly under the base
# domain), but NOT *.agents or *.gateway — they sit one label deeper.
# TLS_CERT_FILE=/opt/amp/certs/fullchain.pem   # cert + intermediates, PEM
# TLS_KEY_FILE=/opt/amp/certs/privkey.pem      # matching private key, PEM
# TLS_CA_FILE=/opt/amp/certs/ca.pem            # only if the cert chains to a private CA:
#                                              # makes in-cluster components trust it

# --- Optional ---
EXTERNAL_GATEWAYS=true             # expose the cp endpoint for external data-plane gateways

# --- Optional per-service host overrides (default: <svc>.<DOMAIN_BASE>) ---
# HOST_CONSOLE=console.amp.mycompany.com
# HOST_API=api.amp.mycompany.com
# HOST_THUNDER=thunder.amp.mycompany.com
# HOST_OBSERVER=observer.amp.mycompany.com
# HOST_GATEWAY=gateway.amp.mycompany.com
# HOST_CP=cp.amp.mycompany.com
# AGENTS_BASE=agents.amp.mycompany.com   # deployed-agent wildcard base
TEMPLATE
}

CONFIG_FILE="" DRY_RUN="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --init) print_template; exit 0 ;;
    --config) CONFIG_FILE="${2:?--config requires a path}"; shift 2 ;;
    --dry-run) DRY_RUN="true"; shift ;;
    -h|--help) grep '^#' "$0" | grep -v '^#!' | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown flag: $1" ;;
  esac
done

[[ -n "$CONFIG_FILE" ]] || die "--config <file> is required (or --init to emit a template)"

# Load + validate config, derive hostnames.
load_config "$CONFIG_FILE" || die "could not load config: $CONFIG_FILE"
if ! validate_config; then
  printf '%s\n' "${CONFIG_ERRORS[@]}" >&2
  die "config validation failed (${#CONFIG_ERRORS[@]} error(s)) — fix amp-config.env and re-run"
fi
# Declare the host vars in this scope so the lib-vm.sh cores (dynamic scope) see them.
AMP_HOST_CONSOLE="" AMP_HOST_API="" AMP_HOST_THUNDER="" AMP_HOST_OBSERVER=""
AMP_HOST_GATEWAY="" AMP_HOST_CP="" AMP_AGENTS_BASE=""
derive_hosts

TLS_MODE_RESOLVED="$(tls_mode)"

# In byoc mode, check the supplied certificate now — validate_cert needs the derived
# hostnames, so it cannot run inside validate_config. This must still happen before
# phase 1, which opens the firewall and creates the cluster: a cert whose SANs miss a
# tier would otherwise only surface as a browser error at the end of a 20-minute install.
if [[ "$TLS_MODE_RESOLVED" == "byoc" ]]; then
  if ! validate_cert "$TLS_CERT_FILE" "$TLS_KEY_FILE"; then
    printf '%s\n' "${CERT_ERRORS[@]}" >&2
    die "certificate validation failed (${#CERT_ERRORS[@]} error(s)) — reissue the cert with the required SANs and re-run"
  fi
  log "Certificate check passed: cert/key match, not expired, SANs cover every service host and dynamic tier"
fi

# Names of the cert-manager + gateway resources the installer creates post-install.
DNS01_SECRET="amp-dns01-credentials"
ACME_ISSUER="amp-acme-dns01"
WILDCARD_CERT="amp-wildcard-tls"
WILDCARD_SECRET="amp-wildcard-tls"
CONSOLIDATED_GATEWAY="amp-consolidated-gateway"
GATEWAY_NS="amp-gateway"

# The shared AMP Gateway's kgateway Service (deployments/k8s/amp-gateway.yaml), which
# the front-proxy routes forward to by host — one listener port per service group. It
# lives in the same namespace as the consolidated :443 Gateway, so no ReferenceGrant.
AMP_GW_NS="amp-gateway"; AMP_GW_SVC="amp-gateway"
CP_GW_PORT=8080      # console, API, Thunder, gateway management
OBS_GW_PORT=11080    # Agent Manager Observer
DP_GW_PORT=19080     # API Platform Gateway vhosts

# preflight_dns — advisory only. DNS-01 needs NO inbound and does NOT require the service
# hostnames to point at this VM (the ACME CA proves control by reading a TXT record the
# provider API writes). The A records only matter for clients reaching the services, so
# report whether they resolve here without ever aborting the install.
preflight_dns() {
  local -a cand=(); local ip pub
  while IFS= read -r ip; do [[ -n "$ip" ]] && cand+=("$ip"); done < <(_local_ips)
  pub="$(_public_ip)"; [[ -n "$pub" ]] && cand+=("$pub")
  (( ${#cand[@]} )) || { log "Could not determine the VM's IP for the DNS check; skipping."; return 0; }
  validate_dns "${cand[@]}" >/dev/null 2>&1 || true   # advisory: validate_dns hard-fails only in the (removed) letsencrypt mode
  if (( ${#DNS_ERRORS[@]} == 0 )); then
    log "DNS check: all service hostnames resolve to this VM."
  else
    log "DNS check (advisory): some hostnames don't resolve to this VM yet — point your DNS (or client /etc/hosts) at it before connecting. Certificate issuance itself needs no inbound and no A records."
  fi
}

# apply_advanced_tls — after the base install, create the TLS Secret, the single :443
# HTTPS Gateway that terminates TLS with it, and the front-proxy routes that forward by
# host to the AMP Gateway's listeners. This replaces the old lego + Caddy path entirely.
#
# The two modes differ only in how the Secret comes to exist: dns01 creates the
# cert-manager objects (provider Secret + ACME ClusterIssuer + Certificate) and waits
# for issuance; byoc writes the operator's cert/key straight into the Secret. The
# Gateway and the routes are byte-identical either way.
apply_advanced_tls() {
  if [[ "$TLS_MODE_RESOLVED" == "byoc" ]]; then
    log "Applying supplied TLS certificate + consolidated :443 gateway + front-proxy routes"
    { render_byoc_tls_secret "$WILDCARD_SECRET" "$TLS_CERT_FILE" "$TLS_KEY_FILE"
      echo "---"
      # Persist the operator CA in-cluster so workloads can trust the listener too.
      if [[ -n "${TLS_CA_FILE:-}" ]]; then
        render_platform_ca_configmap "$TLS_CA_FILE"
        echo "---"
      fi
      render_consolidated_gateway "$CONSOLIDATED_GATEWAY" "$WILDCARD_SECRET" 443
      echo "---"
      render_frontproxy_resources
    } | kubectl apply -f - || die "failed to apply TLS/gateway resources"
  else
    log "Applying cert-manager DNS-01 resources (provider=${DNS_PROVIDER}) + consolidated :443 gateway + front-proxy routes"
    { render_dns01_credentials_secret "$DNS01_SECRET"
      echo "---"
      render_acme_clusterissuer "$ACME_ISSUER" "$DNS01_SECRET"
      echo "---"
      render_wildcard_certificate "$WILDCARD_CERT" "$WILDCARD_SECRET" "$ACME_ISSUER"
      echo "---"
      render_consolidated_gateway "$CONSOLIDATED_GATEWAY" "$WILDCARD_SECRET" 443
      echo "---"
      render_frontproxy_resources
    } | kubectl apply -f - || die "failed to apply cert-manager/gateway resources"

    log "Waiting for cert-manager to issue the wildcard cert via DNS-01 (can take a few minutes)…"
    kubectl wait --for=condition=Ready "certificate/${WILDCARD_CERT}" -n "$GATEWAY_NS" --timeout=600s \
      || die "cert-manager did not issue the cert — inspect: kubectl describe certificate ${WILDCARD_CERT} -n ${GATEWAY_NS}; kubectl get challenge -A"
  fi

  # The Secret exists either way by now; the remaining question is whether kgateway has
  # accepted the listener and programmed the dataplane. This matters most in byoc mode,
  # which has no certificate wait above and would otherwise print its access URLs before
  # :443 could answer. Advisory, not fatal: the platform is fully installed by this point,
  # so aborting would gain nothing, and a slow VM taking longer than the timeout is not a
  # failure. Same rationale as preflight_dns.
  log "Waiting for the consolidated :443 gateway to be programmed…"
  if ! kubectl wait --for=condition=Programmed "gateway/${CONSOLIDATED_GATEWAY}" -n "$GATEWAY_NS" --timeout=300s; then
    log "NOTE: the consolidated gateway is not Programmed yet. It may still settle; if :443 stays unreachable, inspect: kubectl describe gateway ${CONSOLIDATED_GATEWAY} -n ${GATEWAY_NS}"
  fi
}

# render_frontproxy_resources — emit the host-based routes that make the consolidated
# :443 Gateway forward to the AMP Gateway's listeners. The *.agents and
# *.<gateway host> wildcards cover hosts created after install. Gateway API always
# matches an exact hostname before a wildcard.
render_frontproxy_resources() {
  # Console/api/thunder/cp -> :8080 listener.
  local -a cp_hosts=("$AMP_HOST_CONSOLE" "$AMP_HOST_API" "$AMP_HOST_THUNDER")
  [[ -n "${AMP_HOST_CP:-}" ]] && cp_hosts+=("$AMP_HOST_CP")
  render_frontproxy_route amp-frontproxy-controlplane "$CONSOLIDATED_GATEWAY" \
    "$AMP_GW_NS" "$AMP_GW_SVC" "$CP_GW_PORT" "${cp_hosts[@]}"
  echo "---"
  # Observer -> :11080 listener.
  render_frontproxy_route amp-frontproxy-observability "$CONSOLIDATED_GATEWAY" \
    "$AMP_GW_NS" "$AMP_GW_SVC" "$OBS_GW_PORT" "$AMP_HOST_OBSERVER"
  echo "---"
  # OTel/LLM-proxy gateway host + agent wildcard -> :19080 listener, which
  # discriminates by Host.
  render_frontproxy_route amp-frontproxy-dataplane "$CONSOLIDATED_GATEWAY" \
    "$AMP_GW_NS" "$AMP_GW_SVC" "$DP_GW_PORT" "$AMP_HOST_GATEWAY" "*.${AMP_AGENTS_BASE}" \
    "*.${AMP_HOST_GATEWAY}"
}

run_advanced_install() {
  [[ "$(id -u)" -eq 0 ]] || die "run with sudo — this opens the firewall and creates the cluster"

  log "Phase 1/3: preflight (verify tools + firewall)"
  verify_prerequisites
  ensure_inotify_limits
  ensure_firewall 443     # inbound :443 for client traffic; certificate issuance needs no inbound
  ensure_disk
  preflight_dns

  log "Phase 2/3: install Agent Manager (TLS_MODE=${TLS_MODE_RESOLVED}, no Caddy) — 8-15 min"
  # Hostname-driven helm overrides (identical cores to the simple path). Every chart keeps
  # its native routes on the AMP Gateway — the consolidated :443 gateway forwards to them
  # by host (see render_frontproxy_resources), so no ocIngress/external-gateway repointing
  # is needed here.
  # shellcheck disable=SC2034  # arrays are inherited by the subshell that sources install.sh
  mapfile -t AMP_HELM_ARGS < <(amp_helm_args)
  # shellcheck disable=SC2034
  mapfile -t THUNDER_HELM_ARGS < <(thunder_helm_args)
  # shellcheck disable=SC2034
  mapfile -t GATEWAY_HELM_ARGS < <(gateway_helm_args)
  # shellcheck disable=SC2034
  mapfile -t OBSERVABILITY_HELM_ARGS < <(observability_helm_args)

  export VERSION="$AMP_VERSION"
  export SHOW_LOCALHOST_URLS=false

  # k3d: publish :443 (the consolidated gateway) to the host; keep the plane ports
  # loopback-bound (only :443 faces the network). Render to mktemp files, not fixed
  # /tmp paths: the installer runs as root, so a fixed path a local user pre-created
  # as a symlink could redirect a root-owned write (symlink/TOCTOU).
  K3D_CONFIG="$(mktemp)"; export K3D_CONFIG
  render_k3d_advanced_config <"${QS_DIR}/k3d-config.yaml" >"$K3D_CONFIG"
  COREDNS_FILE="$(mktemp)"; export COREDNS_FILE
  render_coredns_vm_config "k3d-amp-local-server-0" >"$COREDNS_FILE"

  log "Running base installer with custom-domain overrides (TLS_MODE=${TLS_MODE_RESOLVED})"
  local rc=0
  ( set +e; source "${QS_DIR}/install.sh" ) || rc=$?
  [[ "$rc" -eq 0 ]] || die "Base installer exited $rc"

  if [[ "$TLS_MODE_RESOLVED" == "byoc" ]]; then
    log "Phase 3/3: install the supplied TLS certificate + expose :443"
  else
    log "Phase 3/3: issue TLS certificate (cert-manager DNS-01) + expose :443"
  fi
  apply_advanced_tls

  log "Done. Access URLs:"
  cat <<EOF

  Console:   https://${AMP_HOST_CONSOLE}
  API:       https://${AMP_HOST_API}
  Thunder:   https://${AMP_HOST_THUNDER}
  Observer:  https://${AMP_HOST_OBSERVER}
  OTel ingest: https://${AMP_HOST_GATEWAY}/otel
  Deployed agents: https://<org>-<project>.${AMP_AGENTS_BASE}/...
EOF
  [[ -n "$AMP_HOST_CP" ]] && echo "  Gateway control plane: https://${AMP_HOST_CP}  (connect external gateways here; registration token is secret-bearing)"

  # install.sh's own admin-credentials print is suppressed by SHOW_LOCALHOST_URLS=false
  # (it only knows the unreachable localhost console URL), so print it here with the
  # real one instead.
  bash "${QS_DIR}/../scripts/print-admin-credentials.sh" \
    "https://${AMP_HOST_CONSOLE}" "${THUNDER_NS:-amp-thunder}" || true
}

if [[ "$DRY_RUN" == "true" ]]; then
  log "DRY RUN — derived hosts:"
  printf '  console=%s api=%s thunder=%s observer=%s gateway=%s cp=%s agents=%s\n' \
    "$AMP_HOST_CONSOLE" "$AMP_HOST_API" "$AMP_HOST_THUNDER" "$AMP_HOST_OBSERVER" \
    "$AMP_HOST_GATEWAY" "${AMP_HOST_CP:-<none>}" "$AMP_AGENTS_BASE"
  log "DRY RUN — TLS mode: ${TLS_MODE_RESOLVED}"
  # Confirm the registry wiring without echoing the token: dry-run goes to stdout
  # (terminal scrollback, CI logs), same reasoning as the Secret placeholders below.
  if [[ -n "${AMP_IMAGE_REGISTRY:-}" ]]; then
    log "DRY RUN — image registry: ${AMP_IMAGE_REGISTRY}"
    if [[ -n "${AMP_REGISTRY_USERNAME:-}" && -n "${AMP_REGISTRY_PASSWORD:-}" ]]; then
      printf '  credentials: user=%s token=<omitted in dry-run>\n' "$AMP_REGISTRY_USERNAME"
    else
      printf '  credentials: <none configured — set AMP_REGISTRY_USERNAME and AMP_REGISTRY_PASSWORD>\n'
    fi
  else
    log "DRY RUN — image registry: public default"
  fi
  log "DRY RUN — amp helm args:"; amp_helm_args
  log "DRY RUN — required cert SANs:"; cert_dns_names
  if [[ "$TLS_MODE_RESOLVED" == "byoc" ]]; then
    # Show what the supplied cert actually carries next to the required list above, so a
    # missing SAN is obvious here rather than after a 20-minute install.
    log "DRY RUN — SANs in ${TLS_CERT_FILE}:"
    cert_sans "$TLS_CERT_FILE"
    log "DRY RUN — TLS + gateway + front-proxy resources:"
    # Do NOT render the real Secret: it embeds the private key, and dry-run goes to
    # stdout (terminal scrollback, CI logs). Same reasoning as the DNS-01 branch below.
    printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: %s\ntype: kubernetes.io/tls\ndata:\n  <tls.crt and tls.key omitted in dry-run>\n---\n' \
      "$WILDCARD_SECRET" "$GATEWAY_NS"
    # The CA is public (it travels in the TLS handshake), so unlike the key it is safe
    # to render in full here.
    if [[ -n "${TLS_CA_FILE:-}" ]]; then
      render_platform_ca_configmap "$TLS_CA_FILE"; echo "---"
    fi
  else
    log "DRY RUN — cert-manager + gateway + front-proxy resources:"
    # Do NOT render the provider-credential Secret here: it holds the live token/key
    # (Cloudflare token, AWS secret, GCP SA JSON, Azure secret) and dry-run goes to stdout
    # (terminal scrollback, CI logs). Show a redacted placeholder instead.
    printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: %s\ntype: Opaque\nstringData:\n  <%s credentials omitted in dry-run>\n---\n' \
      "$DNS01_SECRET" "$CERT_MANAGER_NS" "$DNS_PROVIDER"
    render_acme_clusterissuer "$ACME_ISSUER" "$DNS01_SECRET"; echo "---"
    render_wildcard_certificate "$WILDCARD_CERT" "$WILDCARD_SECRET" "$ACME_ISSUER"; echo "---"
  fi
  render_consolidated_gateway "$CONSOLIDATED_GATEWAY" "$WILDCARD_SECRET" 443; echo "---"
  render_frontproxy_resources
  log "DRY RUN — DNS pre-flight (advisory):"; preflight_dns
  exit 0
fi

run_advanced_install
