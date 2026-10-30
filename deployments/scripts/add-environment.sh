#!/bin/bash
# shellcheck source-path=SCRIPTDIR
set -euo pipefail

# Creates a new environment and installs its API Platform Gateway.
#
# All inputs are provided via environment variables so the script can be piped
# directly into bash:
#
#   curl -fsSL https://raw.githubusercontent.com/wso2/agent-manager/main/deployments/scripts/add-environment.sh \
#     | ENV_NAME=staging \
#       DISPLAY_NAME="Staging" \
#       AGENT_MANAGER_TOKEN=<token> \
#       bash
#
# Add IS_PRODUCTION=true for a production environment.
#
# The console resolves a unique ENV_NAME via POST /orgs/{org}/utils/generate-name
# and renders the full command for the user. Re-running with the same ENV_NAME
# is idempotent.
#
# Prerequisites:
#   - kubectl and helm must be configured
#   - AGENT_MANAGER_TOKEN: bearer token authorized to create environments
#   - ENV_NAME: resource name (lowercase alphanumeric with hyphens)
#   - DISPLAY_NAME: human-readable name
#   - CHART_VERSION: gateway-extension chart version, pinned to the platform
#     release so an added env runs the same chart. Injected by the console.
# Optional:
#   - CHART_VERSION: gateway-extension chart version (e.g. 0.15.0). Injected by the console.
#   - THUNDER_CHART_VERSION: ThunderID chart version for the per-env instance (default: 1.0.2).
#   - GATEWAY_CHART: path to a local chart directory or tarball (e.g. ./deployments/helm-charts/wso2-amp-api-platform-gateway-extension).
#     When set, CHART_VERSION is ignored and the local chart is used directly.
#   - IS_PRODUCTION (default: false)
#   - GATEWAY_TOPOLOGY (default: single): single|split. split installs a second Helm
#     release for the egress role and lowers the ENV_NAME ceiling by 7 characters.
#   - ORG_NAME (default: default), DATAPLANE_REF (default: default)
#   - AGENT_MANAGER_URL (default: http://api.amp.localhost:8080)
#   - ENV_INGRESS_HOST (default: the install's AGENTS_BASE_DOMAIN, else
#     am-gateway.localhost): agent-facing gateway host.
#   - ENV_INGRESS_HTTPS_HOST (default: unset): on TLS deployments, advertises an
#     https listener variant. Set ENV_INGRESS_HTTPS_HOST=$ENV_INGRESS_HOST for
#     the TLS toggle alone; without it the deployed-agent invoke URL is empty.
#   - ENV_INGRESS_PORT (default: the install's AGENTS_HTTP_PORT, else 19080) and
#     ENV_INGRESS_HTTPS_PORT (default: the install's AGENTS_HTTPS_PORT, else 443):
#     the port each listener variant serves. They differ on a plane gateway that
#     serves http on 80 and https on 443; neither is inferred from the other.
#   - GATEWAY_BASE_DOMAIN (default: the install's GATEWAY_BASE_DOMAIN, else
#     gateway.localhost): base domain for this environment's api-platform gateway.
#   - GATEWAY_VHOST_SCHEME (default: the install's GATEWAY_VHOST_SCHEME, else http)
#     and GATEWAY_VHOST_PORT (default: the install's, else 19080): the scheme and
#     port the gateway vhost is published on. A VM install fronts the runtime with
#     TLS on :443; a local k3d install serves plain http on the node port.
#   - GATEWAY_VHOST (default: composed from the three above): full override of the
#     published gateway URL, for a topology none of the recorded values describe.
#   - AMS_CONFIGMAP_NAME / AMS_CONFIGMAP_NAMESPACE (default: amp-api / wso2-amp):
#     where the recorded values above are read from on a non-default release.
#   - IDP_SKIP_TLS_VERIFY (default: true): skipTlsVerify for the seeded env-Thunder identity provider.
#   - THUNDER_DB_TYPE, THUNDER_DB_*, THUNDER_CACHE_TYPE, THUNDER_REDIS_*, THUNDER_REPLICAS,
#     THUNDER_STATE_ACCESS_MODE, THUNDER_FORCE_STORAGE_CHANGE: database, cache and replica
#     settings for this environment's Thunder (default: SQLite, in-memory cache, 1 replica).
#     Set them on the same command as this script, on `bash` (the last stage of a
#     curl | bash pipeline), not on `curl`; they are inherited by the chained
#     add-environment-thunder.sh, which documents them. When any of them is set, a
#     failed Thunder step stops this script instead of falling back to the shared
#     platform Thunder.

# --- Required inputs ---
: "${ENV_NAME:?ENV_NAME is required (e.g. ENV_NAME=staging)}"
: "${DISPLAY_NAME:?DISPLAY_NAME is required (e.g. DISPLAY_NAME=\"Staging\")}"
: "${AGENT_MANAGER_TOKEN:?AGENT_MANAGER_TOKEN is required (bearer token)}"
# CHART_VERSION is required when pulling from OCI but ignored when GATEWAY_CHART is set.
CHART_VERSION="${CHART_VERSION:-}"

# CHART_VERSION carries the Agent Manager release version here, so pin script
# fetches (below, and the chained add-environment-thunder.sh) to that release
# tag instead of `main` — same convention as getScriptRef(). Falls back to main
# when unset/dev (no matching tag). Computed early so both this script's own
# thunder-naming.sh fetch and the later Thunder-provisioning fetch agree. An
# update's chart version (2.0.0-u1) has no tag of its own, so the console passes
# SCRIPT_BASE_URL whenever it runs the script from a release bundle.
script_ref="main"
if [ -n "$CHART_VERSION" ] && [[ "$CHART_VERSION" != *dev* ]]; then
    script_ref="amp/v${CHART_VERSION#v}"
fi
SCRIPT_BASE_URL="${SCRIPT_BASE_URL:-https://raw.githubusercontent.com/wso2/agent-manager/${script_ref}/deployments/scripts}"

IS_PRODUCTION="${IS_PRODUCTION:-false}"
case "$IS_PRODUCTION" in
    true|false) ;;
    *)
        echo "❌ IS_PRODUCTION must be 'true' or 'false' (got '${IS_PRODUCTION}')"
        exit 1
        ;;
esac

if ! printf '%s' "$ENV_NAME" | grep -Eq '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'; then
    echo "❌ Invalid ENV_NAME '${ENV_NAME}'"
    echo "   Must be lowercase alphanumeric with hyphens (no leading/trailing hyphen)."
    exit 1
fi

# storage_settings_requested: 0 if any of the env-Thunder database/cache/replica settings
# (see the THUNDER_DB_* block in the header) was given. Thunder provisioning failures are
# otherwise tolerated below, falling back to the shared platform Thunder; that fallback is
# wrong when the caller asked for specific storage, so a failure must stop the run.
storage_settings_requested() {
    local v
    for v in $(compgen -v | grep -E '^THUNDER_(DB_|CACHE_|REDIS_|REPLICAS$|STATE_ACCESS_MODE$|FORCE_STORAGE_CHANGE$)'); do
        [ -n "${!v:-}" ] && return 0
    done
    return 1
}

# Pure environment checks belong before anything is created: storage settings need a
# per-environment Thunder to apply to.
if [ "${PROVISION_THUNDER:-true}" != "true" ] && storage_settings_requested; then
    echo "❌ THUNDER_* storage settings were given but PROVISION_THUNDER=false, so they would be ignored." >&2
    echo "   Unset one or the other." >&2
    exit 1
fi

# Wrong storage settings are rejected here as well, before the environment is created:
# the Thunder step that would reject them runs after Step 1, and a rejected run would
# leave a new environment behind with no Thunder. The rules are the ones
# add-environment-thunder.sh applies (deployments/scripts/thunder-storage.sh, same
# prefer-sibling, fall-back-to-fetch loading as thunder-naming.sh below). Nothing is
# loaded or fetched when no storage setting was given, so the default flow is unchanged.
if storage_settings_requested; then
    if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "$(dirname "${BASH_SOURCE[0]}")/thunder-storage.sh" ]; then
        # shellcheck source=thunder-storage.sh
        source "$(dirname "${BASH_SOURCE[0]}")/thunder-storage.sh"
    else
        _storage_lib_url="${THUNDER_STORAGE_LIB_URL:-${SCRIPT_BASE_URL}/thunder-storage.sh}"
        _storage_lib_tmp="$(mktemp)"
        if ! curl -fsSL "${_storage_lib_url}" -o "${_storage_lib_tmp}"; then
            echo "❌ Failed to fetch the Thunder storage validation helpers from ${_storage_lib_url}" >&2
            echo "   THUNDER_* storage settings were given, so they cannot be checked; nothing was created." >&2
            rm -f "${_storage_lib_tmp}"
            exit 1
        fi
        # shellcheck source=/dev/null
        source "${_storage_lib_tmp}"
        rm -f "${_storage_lib_tmp}"
        unset _storage_lib_url _storage_lib_tmp
    fi
    if ! validate_storage_config; then
        echo "   Fix the THUNDER_* settings above and re-run; nothing was created." >&2
        exit 1
    fi
fi

# --- Configuration (can be overridden via env vars) ---
ORG_NAME="${ORG_NAME:-default}"
# Namespace the OpenChoreo Environment CRs are created in. The platform-resources
# chart provisions them in its defaultResources namespace ("default"), which is NOT
# necessarily the org name — keep them distinct so a non-default ORG_NAME still
# annotates the right namespace.
ENVIRONMENT_NAMESPACE="${ENVIRONMENT_NAMESPACE:-default}"

GATEWAY_TOPOLOGY="${GATEWAY_TOPOLOGY:-single}"
case "$GATEWAY_TOPOLOGY" in
    single|split) ;;
    *)
        echo "❌ GATEWAY_TOPOLOGY must be 'single' or 'split' (got '${GATEWAY_TOPOLOGY}')"
        exit 1
        ;;
esac

# The APIGateway controller materializes a Service named
# "api-platform-<org>-<env>-gw-gateway-gateway-runtime" — a 27-char suffix, not the
# 24 this comment used to claim — which must stay within k8s's 63-char limit.
# So: len(org) + len(env) <= 63 - 13 ("api-platform-") - 1 ("-") - 27 = 22.
# Matches utils.MaxEnvNameLength in agent-manager-service.
# Split mode adds a second release whose names carry a further "-egress" (7 chars).
MAX_ENV_NAME_LEN=$((22 - ${#ORG_NAME}))
if [ "$GATEWAY_TOPOLOGY" = "split" ]; then
    MAX_ENV_NAME_LEN=$((MAX_ENV_NAME_LEN - 7))
fi
if [ "${#ENV_NAME}" -gt "$MAX_ENV_NAME_LEN" ]; then
    echo "❌ ENV_NAME '${ENV_NAME}' is ${#ENV_NAME} characters; max ${MAX_ENV_NAME_LEN} for org '${ORG_NAME}' in ${GATEWAY_TOPOLOGY} topology"
    echo "   The gateway Service name would exceed Kubernetes' 63-char limit."
    exit 1
fi
DATAPLANE_REF="${DATAPLANE_REF:-default}"
AGENT_MANAGER_URL="${AGENT_MANAGER_URL:-http://api.amp.localhost:8080}"
AGENT_MANAGER_API_URL="${AGENT_MANAGER_API_URL:-${AGENT_MANAGER_URL}/api/v1}"
# Per-org-env namespace isolation: each environment's gateway stack (APIGateway
# CR, runtime, RestApis, token secret) lives in its own "<org>-<env>" namespace.
# The kgateway ingress (gateway-default) stays in openchoreo-data-plane; the
# chart wires the cross-namespace route + ReferenceGrant automatically.
# The <org>-<env> name stays well under the 63-char namespace limit because the
# MAX_ENV_NAME_LEN check above already bounds org+env for the Service name.
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-${ORG_NAME}-${ENV_NAME}}"
IDP_SKIP_TLS_VERIFY="${IDP_SKIP_TLS_VERIFY:-true}"
case "$IDP_SKIP_TLS_VERIFY" in
    true|false) ;;
    *)
        echo "❌ IDP_SKIP_TLS_VERIFY must be 'true' or 'false' (got '${IDP_SKIP_TLS_VERIFY}')"
        exit 1
        ;;
esac

CHART_REF="${CHART_REF:-oci://ghcr.io/wso2/wso2-amp-api-platform-gateway-extension}"

# GATEWAY_CHART: optional path or ref to an alternative chart (e.g. a private OCI registry
# or a tarball). When unset, the published OCI chart at CHART_REF is used.
GATEWAY_CHART="${GATEWAY_CHART:-}"

# --- Resolve chart reference ---
# When GATEWAY_CHART is set, use it directly (no --version flag). The version of
# an OCI chart is resolved further down, once helm is signed in to its registry.
if [ -n "$GATEWAY_CHART" ]; then
    echo "📦 Using gateway chart: ${GATEWAY_CHART}"
    CHART_REF="${GATEWAY_CHART}"
    CHART_VERSION=""
fi

# GATEWAY_VHOST_PORT is resolved further down, together with the other values the
# install records on the AMS ConfigMap — defaulting it here would pre-empt that lookup.

# Base URL the gateway uses to reach Agent Manager. Both /api/v1 and the
# unauthenticated /auth/external/jwks.json endpoint are served from this host:port
# by AMS
AGENT_MANAGER_INTERNAL_BASE_URL="${AGENT_MANAGER_INTERNAL_BASE_URL:-http://host.docker.internal:9000}"
AGENT_MANAGER_INTERNAL_CP="${AGENT_MANAGER_INTERNAL_CP:-host.docker.internal:9243}"
AGENT_MANAGER_INTERNAL_API="${AGENT_MANAGER_INTERNAL_BASE_URL}/api/v1"
AGENT_MANAGER_INTERNAL_JWKS="${AGENT_MANAGER_INTERNAL_BASE_URL}/auth/external/jwks.json"

# Platform Thunder (shared) fallback identity — matches the chart's built-in defaults.
PLATFORM_THUNDER_ISSUER="${PLATFORM_THUNDER_ISSUER:-http://thunder.amp.localhost:8080}"
PLATFORM_THUNDER_JWKS="${PLATFORM_THUNDER_JWKS:-http://amp-thunder-extension-service.amp-thunder:8090/oauth2/jwks}"

# Load the shared Thunder naming helpers (thunder_release_name/thunder_host/
# thunder_issuer/etc.) — the single source of truth for this derivation, see
# deployments/scripts/thunder-naming.sh. Computes per-env Thunder coordinates
# so the gateway ThunderKeyManager points at THIS environment's Thunder (and
# respects THUNDER_HOST_BASE_DOMAIN/TLS_ENABLED on non-local deployments), not
# a stale hardcoded amp.localhost address. Prefers a local sibling file
# (checked-out repo); falls back to fetching it from the same ref this script
# itself would be fetched from when piped via curl | bash.
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "$(dirname "${BASH_SOURCE[0]}")/thunder-naming.sh" ]; then
  # shellcheck source=thunder-naming.sh
  source "$(dirname "${BASH_SOURCE[0]}")/thunder-naming.sh"
else
  _naming_lib_url="${THUNDER_NAMING_LIB_URL:-${SCRIPT_BASE_URL}/thunder-naming.sh}"
  _naming_lib_tmp="$(mktemp)"
  if ! curl -fsSL "${_naming_lib_url}" -o "${_naming_lib_tmp}"; then
    echo "❌ Failed to fetch Thunder naming helpers from ${_naming_lib_url}" >&2
    rm -f "${_naming_lib_tmp}"
    exit 1
  fi
  # shellcheck source=/dev/null
  source "${_naming_lib_tmp}"
  rm -f "${_naming_lib_tmp}"
  unset _naming_lib_url _naming_lib_tmp
fi

# Load the shared AMS auth helpers (get_ams_token/get_thunder_url_handle) — see
# deployments/scripts/ams-auth.sh. Same prefer-local-sibling, fallback-to-curl-fetch
# pattern as the thunder-naming.sh load above. get_thunder_url_handle is the single
# implementation every script uses to learn an ALREADY-provisioned env-Thunder's
# registered handle — see Step 2b below.
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "$(dirname "${BASH_SOURCE[0]}")/ams-auth.sh" ]; then
  # shellcheck source=ams-auth.sh
  source "$(dirname "${BASH_SOURCE[0]}")/ams-auth.sh"
else
  _ams_auth_lib_url="${AMS_AUTH_LIB_URL:-${SCRIPT_BASE_URL}/ams-auth.sh}"
  _ams_auth_lib_tmp="$(mktemp)"
  if ! curl -fsSL "${_ams_auth_lib_url}" -o "${_ams_auth_lib_tmp}"; then
    echo "❌ Failed to fetch AMS auth helpers from ${_ams_auth_lib_url}" >&2
    rm -f "${_ams_auth_lib_tmp}"
    exit 1
  fi
  # shellcheck source=/dev/null
  source "${_ams_auth_lib_tmp}"
  rm -f "${_ams_auth_lib_tmp}"
  unset _ams_auth_lib_url _ams_auth_lib_tmp
fi

# Load the shared registry auth helpers (deployments/scripts/lib-registry-auth.sh)
# so a private-registry environment gets its image pull secret. Optional, unlike
# the two libs above: a public install needs nothing from it, so a missing or
# unreachable copy must not block adding an environment. Only fetched when a
# registry is actually configured, to avoid a pointless network call.
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "$(dirname "${BASH_SOURCE[0]}")/lib-registry-auth.sh" ]; then
  # shellcheck source=lib-registry-auth.sh
  source "$(dirname "${BASH_SOURCE[0]}")/lib-registry-auth.sh"
elif [ -n "${AMP_IMAGE_REGISTRY:-}" ]; then
  # Deliberately not fetched over the network. This helper handles registry
  # credentials, so sourcing a copy pulled from a configurable URL would execute
  # unverified code in the very context that holds them. A private-registry
  # install runs from the extracted release bundle, where this file sits next to
  # this script, so require it rather than downloading a substitute.
  #
  # Fatal, not a warning: without it amp_create_pull_secret never runs, the
  # environment comes up with no pull secret in its workflow namespace, and the
  # first evaluation run fails to pull with nothing pointing back to here.
  echo "❌ AMP_IMAGE_REGISTRY is set, but lib-registry-auth.sh was not found next" >&2
  echo "   to this script. Run add-environment.sh from the extracted release" >&2
  echo "   bundle so the image pull secret can be created in workflows-${ENV_NAME}." >&2
  exit 1
fi

echo "=== Adding Environment: ${DISPLAY_NAME} (${ENV_NAME}) ==="
echo ""

# Checked before any work: an unreachable cluster otherwise surfaces much later as an
# opaque helm/kubectl error, and `command -v kubectl` passes with no kubeconfig at all.
if ! kubectl version > /dev/null 2>&1; then
    echo "❌ kubectl cannot reach the cluster."
    echo "   Check: kubectl config current-context"
    echo "   A single-VM install configures the cluster for root only, so either:"
    echo "     - re-run this command with sudo (place it before 'bash'), or"
    echo "     - give your user a context:"
    echo "         sudo k3d kubeconfig merge amp-local --kubeconfig-merge-default --kubeconfig-switch-context"
    exit 1
fi

# A private-registry environment needs the image pull secret in the namespace
# OpenChoreo runs THIS environment's evaluation workflows in ("workflows-<env>"),
# because the evaluation job's pod runs there rather than in the extension
# chart's release namespace. The chart references the secret but does not create
# it, so each new environment needs its own copy. Idempotent, and a no-op for a
# public install.
if declare -F amp_registry_is_private >/dev/null 2>&1 && amp_registry_is_private; then
    echo "⏳ Creating image pull secret in workflows-${ENV_NAME}..."
    if amp_create_pull_secret "workflows-${ENV_NAME}"; then
        echo "✅ Image pull secret created in workflows-${ENV_NAME}"
    else
        echo "❌ Failed to create the image pull secret in workflows-${ENV_NAME}" >&2
        exit 1
    fi
fi

# A gateway extension chart in the private registry is pulled by helm on this
# machine, which neither the pull secret nor a node's registry config covers, so
# sign helm in with the same credentials. amp_registry_helm_login only acts when
# the chart is on the image registry's host.
case "$CHART_REF" in
    oci://*)
        if declare -F amp_registry_helm_login >/dev/null 2>&1 && amp_registry_is_private; then
            HELM_CHART_REGISTRY="${HELM_CHART_REGISTRY:-${CHART_REF#oci://}}"
            _chart_host="${CHART_REF#oci://}"
            _chart_host="${_chart_host%%/*}"
            if [ "$_chart_host" != "$(amp_registry_host)" ]; then
                # Different credentials from the image registry's; say so rather
                # than fail later with a bare 401 from helm.
                echo "ℹ️  The gateway extension chart is on ${_chart_host}, not the image registry."
                echo "   If that registry is private, helm must already be signed in to it"
                echo "   (helm registry login ${_chart_host})."
            elif ! amp_registry_helm_login; then
                echo "❌ Failed to sign helm in to the chart registry for ${CHART_REF}" >&2
                exit 1
            fi
            unset _chart_host
        fi
        ;;
esac

# --- Resolve chart version ---
# Resolve the latest OCI version or use the pinned CHART_VERSION. After the
# sign-in above, which a private chart needs before helm can read it.
if [ -z "$GATEWAY_CHART" ]; then
    if [ -z "$CHART_VERSION" ]; then
        echo "🔎 Resolving latest gateway chart version from ${CHART_REF}..."
        CHART_VERSION=$(helm show chart "${CHART_REF}" 2>/dev/null | awk '/^version:/ {print $2; exit}')
        if [ -z "$CHART_VERSION" ]; then
            echo "❌ Could not resolve the latest chart version from ${CHART_REF}"
            echo "   Pin a version explicitly and retry (e.g. CHART_VERSION=0.15.0)."
            exit 1
        fi
        echo "✅ Using latest chart version: ${CHART_VERSION}"
    else
        echo "📌 Using pinned chart version: ${CHART_VERSION}"
    fi
fi

# --- Step 0: Verify Agent Manager is reachable ---
echo "⏳ Checking Agent Manager is healthy..."
MAX_WAIT=30
ELAPSED=0
until curl -sf "${AGENT_MANAGER_URL}/healthz" > /dev/null 2>&1; do
    if [ "$ELAPSED" -ge "$MAX_WAIT" ]; then
        echo "❌ Agent Manager not reachable at ${AGENT_MANAGER_URL}/healthz after ${MAX_WAIT}s"
        echo "   Set AGENT_MANAGER_URL to the URL you use to reach the console's API."
        exit 1
    fi
    sleep 3
    ELAPSED=$((ELAPSED + 3))
done
echo "✅ Agent Manager is healthy"

AUTH_HEADER="Authorization: Bearer ${AGENT_MANAGER_TOKEN}"
# Escape backslashes and double quotes so the display name survives JSON embedding.
DISPLAY_NAME_JSON=$(printf '%s' "${DISPLAY_NAME}" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')

# --- Step 1: Create environment ---
echo ""
echo "🌍 Creating environment '${ENV_NAME}'..."

AMS_CONFIGMAP_NAME="${AMS_CONFIGMAP_NAME:-amp-api}"
AMS_CONFIGMAP_NAMESPACE="${AMS_CONFIGMAP_NAMESPACE:-wso2-amp}"

# Value the install recorded for itself. An absent ConfigMap (legacy or local
# install) yields empty so callers fall back; an unreachable or forbidden cluster must
# not silently do the same, or the fallback picks localhost hosts that resolve nowhere
# and the failure only surfaces as a TLS error at invoke time.
#
# stdout and stderr are captured separately: folding them together would splice any
# kubectl warning into the value on the success path, silently yielding a hostname or
# port built from a diagnostic string.
AMS_CONFIG_VALUE=""
ams_config_value() {
    local out err rc
    AMS_CONFIG_VALUE=""
    err="$(mktemp)"
    out="$(kubectl get configmap "${AMS_CONFIGMAP_NAME}" -n "${AMS_CONFIGMAP_NAMESPACE}" \
        -o "jsonpath={.data.$1}" 2>"${err}")" && rc=0 || rc=$?
    if [ "$rc" -eq 0 ]; then
        AMS_CONFIG_VALUE="$out"
    elif ! grep -q 'NotFound' "${err}"; then
        echo "❌ Could not read ${AMS_CONFIGMAP_NAMESPACE}/${AMS_CONFIGMAP_NAME}: $(cat "${err}")" >&2
        echo "   Fix cluster access, or set ENV_INGRESS_HOST and GATEWAY_VHOST explicitly." >&2
        rm -f "${err}"
        exit 1
    fi
    rm -f "${err}"
}

# --- Agent-facing ingress (the Environment CR's external listeners) ---
if [ -z "${ENV_INGRESS_HOST:-}" ]; then
    ams_config_value AGENTS_BASE_DOMAIN
    ENV_INGRESS_HOST="${AMS_CONFIG_VALUE}"
fi
ENV_INGRESS_HOST="${ENV_INGRESS_HOST:-am-gateway.localhost}"
ENV_INGRESS_HTTPS_HOST="${ENV_INGRESS_HTTPS_HOST:-}"
# Each listener variant carries the port ITS listener serves, and the two are not
# always the same: a Caddy-fronted VM terminates both on 443, while a plane gateway
# commonly serves http on 80 and https on 443. Guessing one from the other publishes
# "http://<host>:443" — an http scheme on the TLS port, which a browser blocks as
# mixed content from the https console. So take both from the install.
if [ -z "${ENV_INGRESS_PORT:-}" ]; then
    ams_config_value AGENTS_HTTP_PORT
    ENV_INGRESS_PORT="${AMS_CONFIG_VALUE}"
fi
ENV_INGRESS_PORT="${ENV_INGRESS_PORT:-19080}"
if [ -z "${ENV_INGRESS_HTTPS_PORT:-}" ]; then
    ams_config_value AGENTS_HTTPS_PORT
    ENV_INGRESS_HTTPS_PORT="${AMS_CONFIG_VALUE}"
fi
ENV_INGRESS_HTTPS_PORT="${ENV_INGRESS_HTTPS_PORT:-443}"

# --- Published gateway vhost (scheme + host + port) ---
# The vhost is the public URL the controller mints into LLM-proxy and OTel endpoints,
# and the console appends "/otel" to it verbatim. Getting the hostname right is not
# enough: on a VM the runtime is reached over TLS on :443 through the front proxy, and
# the node port (19080) is bound to loopback only, so the localhost-era "http://<host>:19080"
# still resolves to nothing callable. Take scheme and port from the install too.
if [ -z "${GATEWAY_BASE_DOMAIN:-}" ]; then
    ams_config_value GATEWAY_BASE_DOMAIN
    GATEWAY_BASE_DOMAIN="${AMS_CONFIG_VALUE}"
fi
GATEWAY_BASE_DOMAIN="${GATEWAY_BASE_DOMAIN:-gateway.localhost}"
if [ -z "${GATEWAY_VHOST_SCHEME:-}" ]; then
    ams_config_value GATEWAY_VHOST_SCHEME
    GATEWAY_VHOST_SCHEME="${AMS_CONFIG_VALUE}"
fi
GATEWAY_VHOST_SCHEME="${GATEWAY_VHOST_SCHEME:-http}"
if [ -z "${GATEWAY_VHOST_PORT:-}" ]; then
    ams_config_value GATEWAY_VHOST_PORT
    GATEWAY_VHOST_PORT="${AMS_CONFIG_VALUE}"
fi
GATEWAY_VHOST_PORT="${GATEWAY_VHOST_PORT:-19080}"

# Omit the port when it is the scheme's default. The installer writes the default
# environment's vhost without one ("https://gateway.<base>"), and the two must agree:
# they are compared as strings wherever a caller matches an endpoint back to a gateway.
gateway_vhost_url() {   # <scheme> <host> <port>
    if { [ "$1" = "https" ] && [ "$3" = "443" ]; } || { [ "$1" = "http" ] && [ "$3" = "80" ]; }; then
        printf '%s://%s' "$1" "$2"
    else
        printf '%s://%s:%s' "$1" "$2" "$3"
    fi
}

# Without an https variant the console reports an empty invoke URL on TLS deployments.
# A recorded (non-localhost) agents base means the install fronts agents publicly, so
# advertise the TLS variant alongside the plain one.
if [ -z "${ENV_INGRESS_HTTPS_HOST}" ] && [ "${ENV_INGRESS_HOST}" != "am-gateway.localhost" ]; then
    ENV_INGRESS_HTTPS_HOST="${ENV_INGRESS_HOST}"
fi

# Build the external listener set. Always advertise http; add an https variant when
# ENV_INGRESS_HTTPS_HOST is set (TLS deployments). The console reads the https
# endpoint variant when tlsEnabled=true, and an Environment's external gateway
# wholly replaces the dataplane's, so an http-only override on a TLS platform leaves
# the deployed-agent invoke URL empty (try-out then 405s against the console host).
EXTERNAL_LISTENERS="\"http\": {\"host\": \"${ENV_INGRESS_HOST}\", \"port\": ${ENV_INGRESS_PORT}}"
if [ -n "${ENV_INGRESS_HTTPS_HOST}" ]; then
    EXTERNAL_LISTENERS="${EXTERNAL_LISTENERS}, \"https\": {\"host\": \"${ENV_INGRESS_HTTPS_HOST}\", \"port\": ${ENV_INGRESS_HTTPS_PORT}}"
fi

# Caddy's *.<base> site issues the cert for the resulting hostname on VM installs.
GATEWAY_HOSTNAME="${ENV_NAME}-${ORG_NAME}.${GATEWAY_BASE_DOMAIN}"
GATEWAY_VHOST="${GATEWAY_VHOST:-$(gateway_vhost_url "${GATEWAY_VHOST_SCHEME}" "${GATEWAY_HOSTNAME}" "${GATEWAY_VHOST_PORT}")}"

# Optional pod runtime isolation tier for this environment. "gvisor" makes agents run
# under the runsc RuntimeClass (requires a gVisor node — see `make setup-gvisor`); "kata"
# makes them run under the kata-qemu RuntimeClass in a lightweight VM (requires a Kata node
# with nested virtualization — see `make setup-kata`). Empty (default) uses the standard
# runc runtime. Only include the field in the payload when set, so runc environments send
# the exact same request body as before.
ISOLATION_TIER="${ISOLATION_TIER:-}"
ISOLATION_TIER_FIELD=""
if [ -n "${ISOLATION_TIER}" ]; then
    ISOLATION_TIER_FIELD="\"isolationTier\": \"${ISOLATION_TIER}\","
    echo "   Isolation tier: ${ISOLATION_TIER}"
fi

ENV_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${AGENT_MANAGER_API_URL}/orgs/${ORG_NAME}/environments" \
    -H "${AUTH_HEADER}" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"${ENV_NAME}\",
        \"displayName\": \"${DISPLAY_NAME_JSON}\",
        ${ISOLATION_TIER_FIELD}
        \"dataplaneRef\": \"${DATAPLANE_REF}\",
        \"dnsPrefix\": \"${ENV_NAME}\",
        \"isProduction\": ${IS_PRODUCTION},
        \"gateway\": {
            \"ingress\": {
                \"external\": {
                    ${EXTERNAL_LISTENERS}
                }
            }
        }
    }")

ENV_HTTP_CODE=$(echo "$ENV_RESPONSE" | tail -1)
ENV_BODY=$(echo "$ENV_RESPONSE" | sed '$d')

if [ "$ENV_HTTP_CODE" = "201" ]; then
    echo "✅ Environment '${ENV_NAME}' created"
elif [ "$ENV_HTTP_CODE" = "409" ]; then
    echo "ℹ️  Environment '${ENV_NAME}' already exists, continuing..."
    # The create request is the only place the API accepts isolationTier, so a
    # re-run against an existing environment (e.g. after a partial first run)
    # would otherwise silently drop it. The Environment CR annotation is the
    # source of truth, so apply it directly instead.
    if [ -n "${ISOLATION_TIER}" ]; then
        if kubectl annotate environment "${ENV_NAME}" -n "${ENVIRONMENT_NAMESPACE}" \
            "openchoreo.dev/isolation-tier=${ISOLATION_TIER}" --overwrite > /dev/null 2>&1; then
            echo "✅ Isolation tier '${ISOLATION_TIER}' applied to existing environment"
        else
            echo "⚠️  Could not set isolation tier on the existing environment. Apply it manually:"
            echo "    kubectl annotate environment ${ENV_NAME} -n ${ENVIRONMENT_NAMESPACE} openchoreo.dev/isolation-tier=${ISOLATION_TIER} --overwrite"
        fi
    fi
else
    echo "❌ Failed to create environment (HTTP ${ENV_HTTP_CODE})"
    echo "   Response: ${ENV_BODY}"
    exit 1
fi

# --- Step 2: Provision the environment's Thunder ID instance ---
# Each environment gets its own Thunder ID (the identity provider for that environment's
# agent OAuth clients). Provisioned BEFORE the gateway (previously this ran after) so the
# gateway's ThunderKeyManager is only ever wired to an address that is confirmed to exist.
# The old order wired the gateway to a computed-but-not-yet-created Thunder address and
# then provisioned Thunder as a non-fatal afterthought — if that step failed (or was
# skipped via PROVISION_THUNDER=false), the gateway was left permanently pointed at a
# per-env Thunder instance that was never created, silently breaking agent JWT validation.
# Check the cluster for an existing Thunder Helm release UNCONDITIONALLY — this is
# the only reliable source of truth for THUNDER_PROVISIONED, and must run whether or
# not PROVISION_THUNDER is true. Otherwise re-running with PROVISION_THUNDER=false
# against an environment that already has a live env-Thunder skips this check
# entirely, THUNDER_PROVISIONED stays false, and the gateway below gets rewired back
# to platform Thunder's issuer/JWKS — invalidating every JWT the still-running
# env-Thunder already issued.
THUNDER_RELEASE_NAME="$(thunder_release_name "${ORG_NAME}" "${ENV_NAME}")"
THUNDER_NS="${THUNDER_RELEASE_NAME}"
THUNDER_PROVISIONED=false
if helm status "${THUNDER_RELEASE_NAME}" --namespace "${THUNDER_NS}" > /dev/null 2>&1; then
    echo "✅ Thunder ID instance already exists (Helm release: ${THUNDER_RELEASE_NAME})"
    THUNDER_PROVISIONED=true
fi

if [ "${PROVISION_THUNDER:-true}" = "true" ]; then
    echo ""
    echo "🔐 Provisioning Thunder ID instance for '${ENV_NAME}'..."

    THUNDER_SCRIPT_URL="${THUNDER_SCRIPT_URL:-${SCRIPT_BASE_URL}/add-environment-thunder.sh}"
    script_tmp="$(mktemp)"
    if curl -fsSL "${THUNDER_SCRIPT_URL}" -o "$script_tmp"; then
      # Reset CHART_VERSION so the AMP release version doesn't bleed into the ThunderID chart
      # install. SCRIPT_BASE_URL IS forwarded (unlike CHART_VERSION) so the chained script
      # fetches thunder-naming.sh from the same git ref as this one — see thunder-naming.sh.
      # AMP_API_URL/AGENT_MANAGER_TOKEN forward this call's already-verified AMS
      # reachability + bearer token to the chained script's store_via_ams.
      # THUNDER_HANDLE, when set, forwards the user-chosen unguessable URL handle
      # (see thunder-naming.sh's thunder_host) to register_thunder_url. Empty is
      # fine too — add-environment-thunder.sh always registers a handle now, generating
      # one itself when THUNDER_HANDLE is unset (see register_thunder_url). Step 2b
      # below learns whichever handle actually got stored, since this process has no
      # other way to find out.
      # THUNDER_HOST_BASE_DOMAIN/TLS_ENABLED/PLATFORM_THUNDER_* must ALSO be forwarded
      # on any non-local-dev deployment (see add-environment-thunder.sh's own doc
      # comment) — a VM install sets all five together, deployment-wide, when it
      # provisions the default environment (deployments/vm/lib-vm.sh), but this
      # add-environment.sh call is a SEPARATE process from that one, so without
      # forwarding them explicitly here every environment created afterward would
      # silently fall back to add-environment-thunder.sh's own local-dev defaults
      # (amp.localhost, TLS disabled) regardless of what this deployment actually
      # uses. Empty is fine when unset — same fallback either way, just explicit.
      if ENV_NAME="${ENV_NAME}" DISPLAY_NAME="${DISPLAY_NAME}" ORG_NAME="${ORG_NAME}" \
          DATAPLANE_REF="${DATAPLANE_REF}" THUNDER_CHART="${THUNDER_CHART:-}" \
          THUNDER_HANDLE="${THUNDER_HANDLE:-}" \
          CHART_VERSION="${THUNDER_CHART_VERSION:-}" SCRIPT_BASE_URL="${SCRIPT_BASE_URL}" \
          AMP_API_URL="${AGENT_MANAGER_API_URL}" AGENT_MANAGER_TOKEN="${AGENT_MANAGER_TOKEN}" \
          THUNDER_HOST_BASE_DOMAIN="${THUNDER_HOST_BASE_DOMAIN:-}" \
          TLS_ENABLED="${TLS_ENABLED:-}" \
          PLATFORM_THUNDER_ISSUER="${PLATFORM_THUNDER_ISSUER:-}" \
          PLATFORM_THUNDER_JWKS_URL="${PLATFORM_THUNDER_JWKS_URL:-}" \
          PLATFORM_THUNDER_TOKEN_AUDIENCE="${PLATFORM_THUNDER_TOKEN_AUDIENCE:-}" \
          bash "$script_tmp"; then
        echo "✅ Thunder ID instance provisioned"
        THUNDER_PROVISIONED=true
      else
        if storage_settings_requested; then
          echo "❌ Thunder ID provisioning failed, and THUNDER_* storage settings were given, so this run" >&2
          echo "   will not fall back to the shared platform Thunder or an existing instance." >&2
          echo "   Fix the error above, then re-run this script with the same ENV_NAME and THUNDER_* settings." >&2
          echo "   If the error says the Thunder install failed on its first run, remove it first, because a" >&2
          echo "   plain re-run is refused until you do:" >&2
          echo "     curl -fsSL ${SCRIPT_BASE_URL}/remove-environment-thunder.sh | ENV_NAME=${ENV_NAME} ORG_NAME=${ORG_NAME} bash" >&2
          rm -f "$script_tmp"
          exit 1
        fi
        echo "⚠️  Thunder ID provisioning failed."
        if [ "$THUNDER_PROVISIONED" = "true" ]; then
          echo "    Existing Thunder instance retained — gateway wiring will use it."
        else
          echo "    The gateway will use its default ThunderKeyManager (shared platform Thunder)"
          echo "    instead of an address that doesn't exist. To fix:"
          echo "    1) Re-run: curl -fsSL ${THUNDER_SCRIPT_URL} | ENV_NAME=${ENV_NAME} DISPLAY_NAME=\"${DISPLAY_NAME}\" ORG_NAME=${ORG_NAME} AMP_API_URL=${AGENT_MANAGER_API_URL} AGENT_MANAGER_TOKEN=<token> bash"
          echo "    2) Re-run this add-environment.sh with the same ENV_NAME (idempotent) to re-wire the gateway"
        fi
      fi
    else
      if storage_settings_requested; then
        echo "❌ Failed to fetch the Thunder ID provisioning script from ${THUNDER_SCRIPT_URL}, and THUNDER_*" >&2
        echo "   storage settings were given, so this run will not continue without it." >&2
        rm -f "$script_tmp"
        exit 1
      fi
      echo "⚠️  Failed to fetch Thunder ID provisioning script from ${THUNDER_SCRIPT_URL}"
      if [ "$THUNDER_PROVISIONED" = "true" ]; then
        echo "    Existing Thunder instance retained — gateway wiring will use it."
      else
        echo "    The gateway will use its default ThunderKeyManager (shared platform Thunder)."
      fi
    fi
    rm -f "$script_tmp"
else
    echo ""
    echo "ℹ️  PROVISION_THUNDER=false — skipping per-env Thunder; gateway will use its default"
    echo "    ThunderKeyManager (shared platform Thunder) instead of a per-env address."
fi

# --- Step 2b: Learn this environment's registered Thunder URL handle ---
# add-environment-thunder.sh (above) registers a handle with agent-manager-service
# BEFORE installing Thunder — either THUNDER_HANDLE as forwarded, or (when unset) a
# 10-character one agent-manager-service generated on its own. Either way, THIS
# script (a separate process) has no way to know which one actually got stored, and
# there's no pattern to compute from org/env instead — so it must ask
# agent-manager-service directly via GET before wiring the gateway's ThunderKeyManager
# in Step 3, or an auto-generated handle would leave the gateway trusting an issuer no
# real token ever presents.
THUNDER_URL_HANDLE=""
if [ "$THUNDER_PROVISIONED" = "true" ]; then
    if ! THUNDER_URL_HANDLE="$(AGENT_MANAGER_TOKEN="${AGENT_MANAGER_TOKEN}" AMP_API_URL="${AGENT_MANAGER_API_URL}" \
        get_thunder_url_handle "${ORG_NAME}" "${ENV_NAME}")"; then
        echo "⚠️  Could not learn the registered Thunder URL handle."
        echo "    The env-Thunder instance is running, but the gateway cannot be wired to it"
        echo "    without its handle — falling back to the shared platform Thunder instead."
        echo "    Re-run this script (idempotent) once agent-manager-service is reachable to fix wiring."
        THUNDER_PROVISIONED=false
    fi
fi

# --- Step 3: Helm install the gateway ---
echo ""
echo "🌐 Installing API Platform Gateway for '${ENV_NAME}'..."

# Ensure the gateway namespace exists and carries the label the sandbox
# NetworkPolicy (agent-api ComponentType) matches for agent gateway egress.
# Without it, agents in this environment cannot reach the gateway's OTEL or
# managed LLM/MCP endpoints when it runs outside openchoreo-data-plane.
kubectl create namespace "${GATEWAY_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f - > /dev/null
kubectl label namespace "${GATEWAY_NAMESPACE}" "amp.wso2.com/api-platform-gateway=true" --overwrite > /dev/null

# The gateway images come from a private registry, pulled with the gateway's own
# Secret from the gateway's namespace (see lib-registry-auth.sh).
ensure_gateway_pull_secret() {   # <namespace>
    if ! declare -F amp_create_gateway_pull_secret > /dev/null 2>&1; then
        echo "❌ lib-registry-auth.sh was not found next to this script, so the gateway" >&2
        echo "   pull secret cannot be created in '$1'. Run add-environment.sh from the" >&2
        echo "   extracted release bundle." >&2
        exit 1
    fi
    if ! amp_create_gateway_pull_secret "$1"; then
        echo "❌ Failed to create the gateway pull secret in '$1'" >&2
        exit 1
    fi
}
ensure_gateway_pull_secret "${GATEWAY_NAMESPACE}"

# gateway-controller (1.2.0-beta+) requires an AES-256 at-rest encryption key,
# mounted from a Secret in the SAME namespace as the gateway release
GATEWAY_ENCRYPTION_SECRET_NAME="${GATEWAY_ENCRYPTION_SECRET_NAME:-gateway-encryption-keys}"
GATEWAY_ENCRYPTION_SECRET_KEY="${GATEWAY_ENCRYPTION_SECRET_KEY:-default-aesgcm256-v1.bin}"

# Creates the encryption key Secret in <namespace>, or keeps the one already there.
# Called once per gateway namespace: the controller mounts it from its own namespace.
ensure_gateway_encryption_key() {   # <namespace>
    local ns="$1"
    if ! kubectl auth can-i get secrets -n "${ns}" &>/dev/null; then
        echo "❌ Missing 'get' permission on secrets in '${ns}' — required to detect an existing gateway encryption key secret. Grant get (in addition to create) on secrets in this namespace to the identity running this script." >&2
        exit 1
    fi

    # Global on purpose: the trap below runs after this function returns.
    key_tmp="$(mktemp)"
    # Remove the plaintext key on every exit path — openssl/kubectl failure (set -e)
    # or an interrupt — not only via the normal cleanup below.
    trap 'rm -f "${key_tmp}"' EXIT INT TERM
    openssl rand 32 > "${key_tmp}"
    enc_create_out="$(kubectl create secret generic "${GATEWAY_ENCRYPTION_SECRET_NAME}" -n "${ns}" \
        "--from-file=${GATEWAY_ENCRYPTION_SECRET_KEY}=${key_tmp}" 2>&1)" && enc_create_rc=0 || enc_create_rc=$?
    rm -f "${key_tmp}" # normal cleanup: don't leave the plaintext key on disk
    trap - EXIT INT TERM
    if [ "${enc_create_rc}" -eq 0 ]; then
        echo "✅ Gateway encryption key secret created in '${ns}'"
    elif kubectl get secret "${GATEWAY_ENCRYPTION_SECRET_NAME}" -n "${ns}" &>/dev/null; then
        echo "⏭️  Gateway encryption key secret '${GATEWAY_ENCRYPTION_SECRET_NAME}' already exists in '${ns}', leaving it untouched."
    else
        echo "❌ Failed to create gateway encryption key secret '${GATEWAY_ENCRYPTION_SECRET_NAME}' in '${ns}': ${enc_create_out}" >&2
        exit 1
    fi
}

ensure_gateway_encryption_key "${GATEWAY_NAMESPACE}"

# The production guide keeps the real gateway IDP credentials in this Secret. A pod cannot read a
# Secret from another namespace, so put a copy beside each gateway. When it does not exist
# (quick-start, k3d, VM) nothing changes: the chart's default credentials are used, as before.
# Returns 0 when copied, 2 when the Secret does not exist, 1 on any other failure.
copy_idp_secret() {
    local id sec err
    err="$(mktemp)"
    if ! id="$(kubectl get secret gateway-idp-credentials -n openchoreo-data-plane -o jsonpath='{.data.client-id}' 2>"${err}")"; then
        if grep -q 'NotFound' "${err}"; then rm -f "${err}"; return 2; fi
        cat "${err}" >&2; rm -f "${err}"; return 1
    fi
    rm -f "${err}"
    sec="$(kubectl get secret gateway-idp-credentials -n openchoreo-data-plane -o jsonpath='{.data.client-secret}' 2>/dev/null)" || return 1
    [ -n "${id}" ] && [ -n "${sec}" ] || return 1
    printf 'apiVersion: v1\nkind: Secret\nmetadata:\n  name: gateway-idp-credentials\ndata:\n  client-id: %s\n  client-secret: %s\n' "${id}" "${sec}" \
        | kubectl apply -n "$1" -f - > /dev/null
}

# Release name must match the gateway runtime service lookup expected by
# the kgateway routes (api-platform-<org>-<env> derives from _helpers.tpl
# apiGatewayName). DO NOT duplicate the org segment.
RELEASE_NAME="api-platform-${ORG_NAME}-${ENV_NAME}"
# Truncate to 53 chars to stay within Helm's release-name limit, stripping
# any trailing hyphens left by truncation.
RELEASE_NAME=$(echo "$RELEASE_NAME" | head -c 53 | sed 's/-*$//')

# Shared across both releases in split topology (chart ref, agentManager.*,
# gateway.environment, apiGateway.controlPlane.*, keymanager/identityProvider
# wiring). Per-release settings (--namespace, apiGateway.namespace, gateway.type,
# gateway.vhost, gateway.displayName, and for egress gateway.name/gateway.hostname)
# are passed explicitly on each `helm upgrade --install` invocation below.
HELM_ARGS=(
    --create-namespace
    --set agentManager.orgName="${ORG_NAME}"
    --set gateway.environment="${ENV_NAME}"
    --set agentManager.apiUrl="${AGENT_MANAGER_INTERNAL_API}"
    --set apiGateway.controlPlane.host="${AGENT_MANAGER_INTERNAL_CP}"
    --set apiGateway.controlPlane.tls.insecureSkipVerify=true
    --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[0].name=agent-manager-service"
    --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[0].issuer=agent-manager-service"
    --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[0].jwks.remote.uri=${AGENT_MANAGER_INTERNAL_JWKS}"
    --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[0].jwks.remote.skipTlsVerify=true"
)
if [ -n "$CHART_VERSION" ]; then
    HELM_ARGS+=(--version "${CHART_VERSION}")
fi

IDP_SECRET_COPIED=false
IDP_COPY_RC=0
copy_idp_secret "${GATEWAY_NAMESPACE}" || IDP_COPY_RC=$?
if [ "${IDP_COPY_RC}" -eq 0 ]; then
    IDP_SECRET_COPIED=true
    echo "🔑 Using the gateway IDP credentials from openchoreo-data-plane/gateway-idp-credentials"
    HELM_ARGS+=(--set agentManager.idp.existingSecret=gateway-idp-credentials)
elif [ "${IDP_COPY_RC}" -ne 2 ]; then
    # Only a missing Secret may fall back to the chart default; any other failure would
    # install a gateway with the wrong password.
    echo "❌ Could not copy openchoreo-data-plane/gateway-idp-credentials into '${GATEWAY_NAMESPACE}'." >&2
    exit 1
fi

if [ "$THUNDER_PROVISIONED" = "true" ]; then
    # Per-env Thunder JWKS wiring: the gateway's ThunderKeyManager must trust the
    # JWT tokens minted by THIS environment's Thunder (not the shared platform Thunder).
    # Only reached when Thunder provisioning above succeeded, so this address is
    # guaranteed to already be live — never wired speculatively.
    #   issuer        = Thunder's publicUrl / jwt.issuer (what it stamps into the JWT iss claim)
    #   internal_jwks = Thunder's K8s service DNS — avoids routing through the ingress
    #                   Service name follows the chart template: {{ .Release.Name }}-service
    # THUNDER_URL_HANDLE (learned via GET in Step 2b above, since this script can't
    # otherwise know whether add-environment-thunder.sh used THUNDER_HANDLE as given
    # or generated its own) drives the issuer — the ACTUAL issuer the deployed
    # Thunder stamps into every JWT is handle-based, with no pattern computable
    # from org/env instead. Getting this wrong would make the gateway trust an
    # issuer no real token ever presents, rejecting every agent request. Release
    # name is unaffected — it never honored the handle.
    THUNDER_RELEASE="$(thunder_release_name "${ORG_NAME}" "${ENV_NAME}")"
    THUNDER_ISSUER="$(thunder_issuer "${THUNDER_URL_HANDLE}")"
    THUNDER_INTERNAL_JWKS="http://${THUNDER_RELEASE}-service.${THUNDER_RELEASE}.svc.cluster.local:8090/oauth2/jwks"
    HELM_ARGS+=(
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].name=ThunderKeyManager"
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].issuer=${THUNDER_ISSUER}"
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].jwks.remote.uri=${THUNDER_INTERNAL_JWKS}"
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].jwks.remote.skipTlsVerify=${IDP_SKIP_TLS_VERIFY}"
    )
    # Mirror the env-Thunder into Agent Manager as this gateway's identity provider.
    # Name must match keymanagers[].name, which is always "ThunderKeyManager" (set above).
    HELM_ARGS+=(
        --set "bootstrap.identityProviders[0].name=ThunderKeyManager"
        --set "bootstrap.identityProviders[0].issuer=${THUNDER_ISSUER}"
        --set "bootstrap.identityProviders[0].jwksUri=${THUNDER_INTERNAL_JWKS}"
        --set "bootstrap.identityProviders[0].skipTlsVerify=${IDP_SKIP_TLS_VERIFY}"
    )
else
    # Re-assert the platform Thunder keymanager explicitly. Helm --set on an indexed array
    # replaces the whole list, so keymanagers[0] above already dropped the chart default.
    HELM_ARGS+=(
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].name=ThunderKeyManager"
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].issuer=${PLATFORM_THUNDER_ISSUER}"
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].jwks.remote.uri=${PLATFORM_THUNDER_JWKS}"
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].jwks.remote.skipTlsVerify=true"
    )
    echo "ℹ️  Per-env Thunder not provisioned — gateway will use shared platform Thunder."
fi

if [ "$GATEWAY_TOPOLOGY" = "split" ]; then
    INGRESS_TYPE="INGRESS"
else
    INGRESS_TYPE="BOTH"
fi

helm upgrade --install "${RELEASE_NAME}" "${CHART_REF}" \
    --namespace "${GATEWAY_NAMESPACE}" \
    --set apiGateway.namespace="${GATEWAY_NAMESPACE}" \
    --set gateway.type="${INGRESS_TYPE}" \
    --set gateway.displayName="${DISPLAY_NAME} API Platform Gateway" \
    --set gateway.hostname="${GATEWAY_HOSTNAME}" \
    --set gateway.vhost="${GATEWAY_VHOST}" \
    "${HELM_ARGS[@]}"

if [ "$GATEWAY_TOPOLOGY" = "split" ]; then
    echo ""
    echo "🌐 Installing egress API Platform Gateway for '${ENV_NAME}'..."

    EGRESS_NAMESPACE="${GATEWAY_NAMESPACE}-egress"
    EGRESS_RELEASE_NAME=$(echo "api-platform-${ORG_NAME}-${ENV_NAME}-egress" | head -c 53 | sed 's/-*$//')
    EGRESS_GATEWAY_NAME="api-platform-${ORG_NAME}-${ENV_NAME}-egress"
    EGRESS_HOSTNAME="${ENV_NAME}-${ORG_NAME}-egress.${GATEWAY_BASE_DOMAIN}"
    EGRESS_VHOST="$(gateway_vhost_url "${GATEWAY_VHOST_SCHEME}" "${EGRESS_HOSTNAME}" "${GATEWAY_VHOST_PORT}")"

    # Both namespaces must carry this label. The sandbox NetworkPolicy at
    # wso2-amp-platform-resources-extension/templates/component-types/agent-api.yaml:206-213
    # selects gateway namespaces by it on port 22893 — a cluster-wide namespaceSelector, so
    # a labelled egress namespace is permitted automatically with no policy change. The
    # label is stamped only by shell scripts, NEVER by the chart. Missing it produces a
    # connection timeout at agent runtime with nothing in the control plane explaining why.
    kubectl create namespace "${EGRESS_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f - > /dev/null
    kubectl label namespace "${EGRESS_NAMESPACE}" "amp.wso2.com/api-platform-gateway=true" --overwrite > /dev/null
    # The egress controller mounts the encryption key from ITS namespace; without this copy
    # its pod stays in ContainerCreating and the egress gateway never becomes Programmed.
    ensure_gateway_encryption_key "${EGRESS_NAMESPACE}"
    ensure_gateway_pull_secret "${EGRESS_NAMESPACE}"
    # The egress release shares HELM_ARGS, so it also references the Secret and needs its own
    # copy. Stop on failure, as for the main gateway: the default credentials would be wrong.
    if [ "${IDP_SECRET_COPIED}" = "true" ]; then
        copy_idp_secret "${EGRESS_NAMESPACE}" || {
            echo "❌ Could not copy openchoreo-data-plane/gateway-idp-credentials into '${EGRESS_NAMESPACE}'." >&2
            exit 1
        }
    fi

    # gateway.name MUST be set: apiGatewayName (_helpers.tpl:56-62) defaults to
    # api-platform-<org>-<env> with no discriminator, so both releases would produce the
    # same gateway name. The name also feeds the chart-computed runtimeUrl
    # (<apiGatewayName>-gw-gateway-gateway-runtime.<apiGateway.namespace>:22893), which the
    # chart POSTs/PUTs to AMS at registration — AMS just stores it, no name-based derivation.
    #
    # gateway.hostname MUST be set and MUST equal the host inside gateway.vhost:
    # gatewayHostname (_helpers.tpl:68-74) defaults to <env>-<org>.gateway.localhost with
    # no discriminator, so two releases would claim the same hostname on the same shared
    # gateway-default kgateway. A hostname/vhost mismatch means the catch-all HTTPRoute
    # never matches and every egress call 404s.
    helm upgrade --install "${EGRESS_RELEASE_NAME}" "${CHART_REF}" \
        --namespace "${EGRESS_NAMESPACE}" \
        --set apiGateway.namespace="${EGRESS_NAMESPACE}" \
        --set gateway.type="EGRESS" \
        --set gateway.name="${EGRESS_GATEWAY_NAME}" \
        --set gateway.displayName="${DISPLAY_NAME} API Platform Gateway (Egress)" \
        --set gateway.hostname="${EGRESS_HOSTNAME}" \
        --set gateway.vhost="${EGRESS_VHOST}" \
        "${HELM_ARGS[@]}"
fi

# --- Step 4: Wait for gateway to be ready ---
GATEWAY_NAME="api-platform-${ORG_NAME}-${ENV_NAME}"
echo ""
echo "⏳ Waiting for gateway '${GATEWAY_NAME}' to be ready..."
if kubectl wait --for=condition=Programmed "apigateway/${GATEWAY_NAME}" -n "${GATEWAY_NAMESPACE}" --timeout=180s 2>/dev/null; then
    echo "✅ Gateway is programmed"
else
    echo "⚠️  Gateway did not become ready in time — check: kubectl get apigateway ${GATEWAY_NAME} -n ${GATEWAY_NAMESPACE}"
fi

if [ "$GATEWAY_TOPOLOGY" = "split" ]; then
    echo "⏳ Waiting for egress gateway '${EGRESS_GATEWAY_NAME}' to be ready..."
    if kubectl wait --for=condition=Programmed "apigateway/${EGRESS_GATEWAY_NAME}" -n "${EGRESS_NAMESPACE}" --timeout=180s 2>/dev/null; then
        echo "✅ Egress gateway is programmed"
    else
        echo "⚠️  Egress gateway did not become ready in time — check: kubectl get apigateway ${EGRESS_GATEWAY_NAME} -n ${EGRESS_NAMESPACE}"
    fi
fi

echo ""
echo "=== Environment '${ENV_NAME}' setup complete ==="
echo ""
echo "  Environment:     ${ENV_NAME}"
echo "  Display Name:    ${DISPLAY_NAME}"
echo "  Gateway Vhost:   ${GATEWAY_VHOST}"
echo "  Agent Endpoints: ${ENV_NAME}-${ORG_NAME}.${ENV_INGRESS_HTTPS_HOST:-${ENV_INGRESS_HOST}}"
echo ""
