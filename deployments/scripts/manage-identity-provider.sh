#!/bin/bash
set -euo pipefail

# Adds or removes an identity provider (JWT token issuer) on the API Platform
# Gateway of a given environment, and syncs the Agent Manager mirror.
#
# The gateway runtime ConfigMap (Helm-managed keymanagers) is the source of truth.
# This script does a read-modify-write of that array and then calls the Agent
# Manager API so the console list and agent OAuth issuer options stay in sync.
#
# All inputs are provided via environment variables so the script can be piped
# directly into bash (the console renders the full command for the user):
#
#   curl -fsSL https://raw.githubusercontent.com/wso2/ai-agent-management-platform/main/deployments/scripts/manage-identity-provider.sh \
#     | ENV_NAME=staging \
#       GATEWAY_ID=<gateway-uuid> \
#       AGENT_MANAGER_TOKEN=<token> \
#       CHART_VERSION=0.15.0 \
#       IDP_NAME=MyIdP \
#       IDP_ISSUER=https://idp.example.com \
#       IDP_JWKS_URI=https://idp.example.com/oauth2/jwks \
#       bash
#
# Use ACTION=delete to remove an identity provider by name. Re-running with the
# same IDP_NAME is idempotent.
#
# Prerequisites:
#   - kubectl, helm, and jq must be configured/installed
#   - AGENT_MANAGER_TOKEN: bearer token authorized to update gateways
#   - CHART_VERSION: published gateway-extension chart version (e.g. 0.15.0)
#   - ENV_NAME: the environment whose gateway is targeted
#   - GATEWAY_ID: the gateway UUID (used for the Agent Manager mirror call)
#   - IDP_NAME: unique identity provider name (referenced as an issuer by agents)
# For ACTION=upsert (default):
#   - IDP_ISSUER: expected iss claim value
#   - IDP_JWKS_URI: remote JWKS endpoint
# Optional:
#   - ACTION (upsert|delete, default: upsert)
#   - IDP_SKIP_TLS_VERIFY (default: false)
#   - ORG_NAME (default: default)
#   - AGENT_MANAGER_URL (default: http://api.amp.localhost:8080)
#   - GATEWAY_NAMESPACE (default: <ORG_NAME>-<ENV_NAME>)

# --- Required inputs ---
: "${ENV_NAME:?ENV_NAME is required (e.g. ENV_NAME=staging)}"
: "${GATEWAY_ID:?GATEWAY_ID is required (gateway UUID)}"
: "${AGENT_MANAGER_TOKEN:?AGENT_MANAGER_TOKEN is required (bearer token)}"
: "${IDP_NAME:?IDP_NAME is required (identity provider name)}"

# Chart reference for the gateway-extension release. Defaults to the published OCI
# chart (CHART_VERSION required). For local development, point CHART_REF at the
# chart directory, e.g.
#   CHART_REF=deployments/helm-charts/wso2-amp-api-platform-gateway-extension
# in which case CHART_VERSION is ignored.
CHART_REF="${CHART_REF:-oci://ghcr.io/wso2/wso2-amp-api-platform-gateway-extension}"
case "$CHART_REF" in
    oci://*) : "${CHART_VERSION:?CHART_VERSION is required for an OCI chart (e.g. CHART_VERSION=0.15.0)}" ;;
esac

ACTION="${ACTION:-upsert}"
case "$ACTION" in
    upsert|delete) ;;
    *)
        echo "❌ ACTION must be 'upsert' or 'delete' (got '${ACTION}')"
        exit 1
        ;;
esac

if [ "$ACTION" = "upsert" ]; then
    : "${IDP_ISSUER:?IDP_ISSUER is required for upsert}"
    : "${IDP_JWKS_URI:?IDP_JWKS_URI is required for upsert}"
fi

if [ "$IDP_NAME" = "agent-manager-service" ]; then
    echo "❌ 'agent-manager-service' is a reserved internal identity provider and cannot be managed."
    exit 1
fi

IDP_SKIP_TLS_VERIFY="${IDP_SKIP_TLS_VERIFY:-false}"
case "$IDP_SKIP_TLS_VERIFY" in
    true|false) ;;
    *)
        echo "❌ IDP_SKIP_TLS_VERIFY must be 'true' or 'false' (got '${IDP_SKIP_TLS_VERIFY}')"
        exit 1
        ;;
esac

# --- Configuration (can be overridden via env vars) ---
ORG_NAME="${ORG_NAME:-default}"
AGENT_MANAGER_URL="${AGENT_MANAGER_URL:-http://api.amp.localhost:8080}"
AGENT_MANAGER_API_URL="${AGENT_MANAGER_API_URL:-${AGENT_MANAGER_URL}/api/v1}"
# Must match the per-org-env namespace add-environment.sh installs into.
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-${ORG_NAME}-${ENV_NAME}}"

# Release name must match the gateway runtime release (api-platform-<org>-<env>),
# truncated to Helm's 53-char limit. DO NOT duplicate the org segment.
RELEASE_NAME="api-platform-${ORG_NAME}-${ENV_NAME}"
RELEASE_NAME=$(echo "$RELEASE_NAME" | head -c 53 | sed 's/-*$//')
GATEWAY_NAME="api-platform-${ORG_NAME}-${ENV_NAME}"
KM_PATH=".apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers"

for tool in kubectl helm jq curl; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "❌ Required tool '${tool}' is not installed or not in PATH."
        exit 1
    fi
done

# `command -v kubectl` passes with no kubeconfig, so check reachability too — otherwise
# this surfaces much later as an opaque "UPGRADE FAILED: cluster unreachable".
if ! kubectl version > /dev/null 2>&1; then
    echo "❌ kubectl is installed but cannot reach the cluster."
    echo "   Check: kubectl config current-context"
    echo "   A single-VM install configures the cluster for root only, so either:"
    echo "     - re-run this command with sudo (place it before 'bash'), or"
    echo "     - give your user a context:"
    echo "         sudo k3d kubeconfig merge amp-local --kubeconfig-merge-default --kubeconfig-switch-context"
    exit 1
fi

echo "=== ${ACTION} identity provider '${IDP_NAME}' on '${ENV_NAME}' ==="
echo ""

# --- Step 0: Verify Agent Manager is reachable ---
echo "⏳ Checking Agent Manager is healthy..."
if ! curl -sf "${AGENT_MANAGER_URL}/healthz" > /dev/null 2>&1; then
    echo "❌ Agent Manager not reachable at ${AGENT_MANAGER_URL}/healthz"
    echo "   Set AGENT_MANAGER_URL to the URL you use to reach the console's API."
    exit 1
fi
echo "✅ Agent Manager is healthy"

AUTH_HEADER="Authorization: Bearer ${AGENT_MANAGER_TOKEN}"

# --- Step 1: Read current keymanagers from the gateway release ---
echo ""
echo "🔎 Reading current identity providers from release '${RELEASE_NAME}'..."
CURRENT_VALUES=$(helm get values "${RELEASE_NAME}" -n "${GATEWAY_NAMESPACE}" -o json 2>/dev/null || echo '{}')
CURRENT_KM=$(echo "${CURRENT_VALUES}" | jq -c "${KM_PATH} // []")

# --- Step 2: Merge or remove the entry by name ---
if [ "$ACTION" = "upsert" ]; then
    echo "➕ Upserting identity provider '${IDP_NAME}'..."
    NEW_KM=$(echo "${CURRENT_KM}" | jq -c \
        --arg name "${IDP_NAME}" \
        --arg issuer "${IDP_ISSUER}" \
        --arg uri "${IDP_JWKS_URI}" \
        --argjson skip "${IDP_SKIP_TLS_VERIFY}" \
        'map(select(.name != $name)) + [{name: $name, issuer: $issuer, jwks: {remote: {uri: $uri, skipTlsVerify: $skip}}}]')
else
    # Guard: do not allow removing the reserved internal provider from the array.
    echo "➖ Removing identity provider '${IDP_NAME}'..."
    NEW_KM=$(echo "${CURRENT_KM}" | jq -c \
        --arg name "${IDP_NAME}" \
        'map(select(.name != $name))')
fi

# Write the full replacement keymanagers array to a values overlay. Helm replaces
# arrays from -f (it does not merge them), so the overlay carries the complete
# desired list while --reuse-values preserves every other release value.
TMP_VALUES=$(mktemp)
trap 'rm -f "${TMP_VALUES}"' EXIT
echo "${NEW_KM}" | jq '{apiGateway: {config: {policyConfigurations: {jwtauth_v1: {keymanagers: .}}}}}' > "${TMP_VALUES}"

# --- Step 3: Re-apply the gateway release (ConfigMap source of truth) ---
echo ""
echo "🌐 Applying gateway configuration for '${ENV_NAME}'..."
# --version only applies to OCI/remote charts; a local chart path carries its own version.
VERSION_ARG=""
case "$CHART_REF" in
    oci://*) VERSION_ARG="--version ${CHART_VERSION}" ;;
esac
# shellcheck disable=SC2086
helm upgrade "${RELEASE_NAME}" \
    "${CHART_REF}" \
    ${VERSION_ARG} \
    --namespace "${GATEWAY_NAMESPACE}" \
    --reuse-values \
    -f "${TMP_VALUES}"

# --- Step 4: Sync the Agent Manager mirror ---
echo ""
IDP_ENCODED=$(printf '%s' "${IDP_NAME}" | jq -sRr @uri)
MIRROR_URL="${AGENT_MANAGER_API_URL}/orgs/${ORG_NAME}/gateways/${GATEWAY_ID}/identity-providers/${IDP_ENCODED}"
if [ "$ACTION" = "upsert" ]; then
    echo "🔁 Syncing Agent Manager mirror (upsert)..."
    SYNC_RESPONSE=$(curl -s -w "\n%{http_code}" -X PUT "${MIRROR_URL}" \
        -H "${AUTH_HEADER}" \
        -H "Content-Type: application/json" \
        -d "{\"issuer\":\"${IDP_ISSUER}\",\"jwksUri\":\"${IDP_JWKS_URI}\",\"skipTlsVerify\":${IDP_SKIP_TLS_VERIFY}}")
else
    echo "🔁 Syncing Agent Manager mirror (delete)..."
    SYNC_RESPONSE=$(curl -s -w "\n%{http_code}" -X DELETE "${MIRROR_URL}" \
        -H "${AUTH_HEADER}")
fi

SYNC_HTTP_CODE=$(echo "$SYNC_RESPONSE" | tail -1)
SYNC_BODY=$(echo "$SYNC_RESPONSE" | sed '$d')
case "$SYNC_HTTP_CODE" in
    200|204)
        echo "✅ Agent Manager mirror synced"
        ;;
    *)
        echo "❌ Failed to sync Agent Manager mirror (HTTP ${SYNC_HTTP_CODE})"
        echo "   Response: ${SYNC_BODY}"
        exit 1
        ;;
esac

# --- Step 5: Wait for the gateway to re-program ---
echo ""
echo "⏳ Waiting for gateway '${GATEWAY_NAME}' to re-program..."
if kubectl wait --for=condition=Programmed "apigateway/${GATEWAY_NAME}" -n "${GATEWAY_NAMESPACE}" --timeout=180s 2>/dev/null; then
    echo "✅ Gateway re-programmed"
else
    echo "⚠️  Gateway did not become ready in time — check: kubectl get apigateway ${GATEWAY_NAME} -n ${GATEWAY_NAMESPACE}"
fi

echo ""
echo "=== Done: ${ACTION} identity provider '${IDP_NAME}' on '${ENV_NAME}' ==="
