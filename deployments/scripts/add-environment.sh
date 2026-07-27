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
#   - THUNDER_CHART_VERSION: ThunderID chart version for the per-env instance (default: 0.45.0).
#   - GATEWAY_CHART: path to a local chart directory or tarball (e.g. ./deployments/helm-charts/wso2-amp-api-platform-gateway-extension).
#     When set, CHART_VERSION is ignored and the local chart is used directly.
#   - IS_PRODUCTION (default: false)
#   - ORG_NAME (default: default), DATAPLANE_REF (default: default)
#   - AGENT_MANAGER_URL (default: http://api.amp.localhost:8080)
#   - ENV_INGRESS_HOST (default: am-gateway.localhost): agent-facing gateway host.
#   - ENV_INGRESS_HTTPS_HOST (default: unset): on TLS deployments, advertises an
#     https listener variant. Set ENV_INGRESS_HTTPS_HOST=$ENV_INGRESS_HOST for
#     the TLS toggle alone; without it the deployed-agent invoke URL is empty.
#   - ENV_INGRESS_HTTPS_PORT (default: 443): port for the https listener variant.
#   - IDP_SKIP_TLS_VERIFY (default: true): skipTlsVerify for the seeded env-Thunder identity provider.

# --- Required inputs ---
: "${ENV_NAME:?ENV_NAME is required (e.g. ENV_NAME=staging)}"
: "${DISPLAY_NAME:?DISPLAY_NAME is required (e.g. DISPLAY_NAME=\"Staging\")}"
: "${AGENT_MANAGER_TOKEN:?AGENT_MANAGER_TOKEN is required (bearer token)}"
# CHART_VERSION is required when pulling from OCI but ignored when GATEWAY_CHART is set.
CHART_VERSION="${CHART_VERSION:-}"

# CHART_VERSION carries the Agent Manager release version here (console sets
# it from ampVersion, see getAmpVersionHelm()), so pin script fetches (below,
# and the chained add-environment-thunder.sh) to that release tag instead of
# `main` — same convention as getScriptRef(). Falls back to main when
# unset/dev (no matching tag). Computed early so both this script's own
# thunder-naming.sh fetch and the later Thunder-provisioning fetch agree.
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

# --- Configuration (can be overridden via env vars) ---
ORG_NAME="${ORG_NAME:-default}"
# Namespace the OpenChoreo Environment CRs are created in. The platform-resources
# chart provisions them in its defaultResources namespace ("default"), which is NOT
# necessarily the org name — keep them distinct so a non-default ORG_NAME still
# annotates the right namespace.
ENVIRONMENT_NAMESPACE="${ENVIRONMENT_NAMESPACE:-default}"

# The APIGateway controller materializes a Service named
# "api-platform-<org>-<env>-gateway-gateway-runtime" (24-char suffix), which
# must stay within k8s's 63-char metadata.name limit.
# So: len(env) <= 63 - 13 ("api-platform-") - 1 ("-") - 24 - len(org) = 25 - len(org)
MAX_ENV_NAME_LEN=$((25 - ${#ORG_NAME}))
if [ "${#ENV_NAME}" -gt "$MAX_ENV_NAME_LEN" ]; then
    echo "❌ ENV_NAME '${ENV_NAME}' is ${#ENV_NAME} characters; max ${MAX_ENV_NAME_LEN} for org '${ORG_NAME}'"
    echo "   The gateway Service name would exceed Kubernetes' 63-char limit."
    echo "   Use a shorter env name (e.g. 'staging' instead of 'staging-environment')."
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

CHART_REF="oci://ghcr.io/wso2/wso2-amp-api-platform-gateway-extension"

# GATEWAY_CHART: optional path or ref to an alternative chart (e.g. a private OCI registry
# or a tarball). When unset, the published OCI chart at CHART_REF is used.
GATEWAY_CHART="${GATEWAY_CHART:-}"

# --- Resolve chart reference and version ---
# When GATEWAY_CHART is set, use it directly (no --version flag).
# Otherwise resolve the latest OCI version or use the pinned CHART_VERSION.
if [ -n "$GATEWAY_CHART" ]; then
    echo "📦 Using gateway chart: ${GATEWAY_CHART}"
    CHART_REF="${GATEWAY_CHART}"
    CHART_VERSION=""
elif [ -z "$CHART_VERSION" ]; then
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

# Port the gateway runtime is exposed on (matches values.yaml gateway.vhost default).
GATEWAY_VHOST_PORT="${GATEWAY_VHOST_PORT:-19080}"

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

echo "=== Adding Environment: ${DISPLAY_NAME} (${ENV_NAME}) ==="
echo ""

# --- Step 0: Verify Agent Manager is reachable ---
echo "⏳ Checking Agent Manager is healthy..."
MAX_WAIT=30
ELAPSED=0
until curl -sf "${AGENT_MANAGER_URL}/healthz" > /dev/null 2>&1; do
    if [ "$ELAPSED" -ge "$MAX_WAIT" ]; then
        echo "❌ Agent Manager not reachable at ${AGENT_MANAGER_URL}/healthz after ${MAX_WAIT}s"
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

ENV_INGRESS_HOST="${ENV_INGRESS_HOST:-am-gateway.localhost}"
ENV_INGRESS_HTTPS_HOST="${ENV_INGRESS_HTTPS_HOST:-}"
ENV_INGRESS_HTTPS_PORT="${ENV_INGRESS_HTTPS_PORT:-443}"

# Build the external listener set. Always advertise http; add an https variant when
# ENV_INGRESS_HTTPS_HOST is set (TLS deployments). The console reads the https
# endpoint variant when tlsEnabled=true, and an Environment's external gateway
# wholly replaces the dataplane's, so an http-only override on a TLS platform leaves
# the deployed-agent invoke URL empty (try-out then 405s against the console host).
EXTERNAL_LISTENERS="\"http\": {\"host\": \"${ENV_INGRESS_HOST}\", \"port\": ${GATEWAY_VHOST_PORT}}"
if [ -n "${ENV_INGRESS_HTTPS_HOST}" ]; then
    EXTERNAL_LISTENERS="${EXTERNAL_LISTENERS}, \"https\": {\"host\": \"${ENV_INGRESS_HTTPS_HOST}\", \"port\": ${ENV_INGRESS_HTTPS_PORT}}"
fi

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
      if ENV_NAME="${ENV_NAME}" DISPLAY_NAME="${DISPLAY_NAME}" ORG_NAME="${ORG_NAME}" \
          DATAPLANE_REF="${DATAPLANE_REF}" THUNDER_CHART="${THUNDER_CHART:-}" \
          CHART_VERSION="${THUNDER_CHART_VERSION:-}" SCRIPT_BASE_URL="${SCRIPT_BASE_URL}" \
          AMP_API_URL="${AGENT_MANAGER_API_URL}" AGENT_MANAGER_TOKEN="${AGENT_MANAGER_TOKEN}" \
          bash "$script_tmp"; then
        echo "✅ Thunder ID instance provisioned"
        THUNDER_PROVISIONED=true
      else
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

# --- Step 3: Helm install the gateway ---
echo ""
echo "🌐 Installing API Platform Gateway for '${ENV_NAME}'..."

# Ensure the gateway namespace exists and carries the label the sandbox
# NetworkPolicy (agent-api ComponentType) matches for agent gateway egress.
# Without it, agents in this environment cannot reach the gateway's OTEL or
# managed LLM/MCP endpoints when it runs outside openchoreo-data-plane.
kubectl create namespace "${GATEWAY_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f - > /dev/null
kubectl label namespace "${GATEWAY_NAMESPACE}" "amp.wso2.com/api-platform-gateway=true" --overwrite > /dev/null

# gateway-controller 1.2.0-beta requires an AES-256 at-rest encryption key,
# mounted from a Secret in the SAME namespace as the gateway release 
GATEWAY_ENCRYPTION_SECRET_NAME="${GATEWAY_ENCRYPTION_SECRET_NAME:-gateway-encryption-keys}"
GATEWAY_ENCRYPTION_SECRET_KEY="${GATEWAY_ENCRYPTION_SECRET_KEY:-default-aesgcm256-v1.bin}"
if kubectl get secret "${GATEWAY_ENCRYPTION_SECRET_NAME}" -n "${GATEWAY_NAMESPACE}" &>/dev/null; then
    echo "⏭️  Gateway encryption key secret '${GATEWAY_ENCRYPTION_SECRET_NAME}' already exists in '${GATEWAY_NAMESPACE}', leaving it untouched."
else
    echo "🔑 Provisioning gateway at-rest encryption key in '${GATEWAY_NAMESPACE}'..."
    key_tmp="$(mktemp)"
    openssl rand 32 > "${key_tmp}"
    kubectl create secret generic "${GATEWAY_ENCRYPTION_SECRET_NAME}" -n "${GATEWAY_NAMESPACE}" \
        "--from-file=${GATEWAY_ENCRYPTION_SECRET_KEY}=${key_tmp}"
    rm -f "${key_tmp}" # don't leave the plaintext key on disk
    echo "✅ Gateway encryption key secret created"
fi

# Release name must match the gateway runtime service lookup expected by
# the kgateway routes (api-platform-<org>-<env> derives from _helpers.tpl
# apiGatewayName). DO NOT duplicate the org segment.
RELEASE_NAME="api-platform-${ORG_NAME}-${ENV_NAME}"
# Truncate to 53 chars to stay within Helm's release-name limit, stripping
# any trailing hyphens left by truncation.
RELEASE_NAME=$(echo "$RELEASE_NAME" | head -c 53 | sed 's/-*$//')

HELM_ARGS=(
    upgrade --install "${RELEASE_NAME}"
    "${CHART_REF}"
    --namespace "${GATEWAY_NAMESPACE}"
    --create-namespace
    # apiGateway.namespace drives where the chart renders the APIGateway CR,
    # config, RestApis, kgateway backendRef and token secret — --namespace alone
    # only places the Helm release. Both must point at the same namespace.
    --set apiGateway.namespace="${GATEWAY_NAMESPACE}"
    --set agentManager.orgName="${ORG_NAME}"
    --set gateway.environment="${ENV_NAME}"
    --set gateway.displayName="${DISPLAY_NAME} API Platform Gateway"
    --set gateway.vhost="http://${ENV_NAME}-${ORG_NAME}.gateway.localhost:${GATEWAY_VHOST_PORT}"
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

if [ "$THUNDER_PROVISIONED" = "true" ]; then
    # Per-env Thunder JWKS wiring: the gateway's ThunderKeyManager must trust the
    # JWT tokens minted by THIS environment's Thunder (not the shared platform Thunder).
    # Only reached when Thunder provisioning above succeeded, so this address is
    # guaranteed to already be live — never wired speculatively.
    #   issuer        = Thunder's publicUrl / jwt.issuer (what it stamps into the JWT iss claim)
    #   internal_jwks = Thunder's K8s service DNS — avoids routing through the ingress
    #                   Service name follows the chart template: {{ .Release.Name }}-service
    THUNDER_RELEASE="$(thunder_release_name "${ORG_NAME}" "${ENV_NAME}")"
    THUNDER_ISSUER="$(thunder_issuer "${ORG_NAME}" "${ENV_NAME}")"
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

helm "${HELM_ARGS[@]}"

# --- Step 4: Wait for gateway to be ready ---
GATEWAY_NAME="api-platform-${ORG_NAME}-${ENV_NAME}"
echo ""
echo "⏳ Waiting for gateway '${GATEWAY_NAME}' to be ready..."
if kubectl wait --for=condition=Programmed "apigateway/${GATEWAY_NAME}" -n "${GATEWAY_NAMESPACE}" --timeout=180s 2>/dev/null; then
    echo "✅ Gateway is programmed"
else
    echo "⚠️  Gateway did not become ready in time — check: kubectl get apigateway ${GATEWAY_NAME} -n ${GATEWAY_NAMESPACE}"
fi

echo ""
echo "=== Environment '${ENV_NAME}' setup complete ==="
echo ""
echo "  Environment:     ${ENV_NAME}"
echo "  Display Name:    ${DISPLAY_NAME}"
echo "  Gateway Runtime: ${ENV_NAME}-${ORG_NAME}.gateway.${ENV_INGRESS_HOST}:${GATEWAY_VHOST_PORT}"
echo ""
