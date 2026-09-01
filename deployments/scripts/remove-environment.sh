#!/bin/bash
set -euo pipefail

# Removes an environment: deletes it via the Agent Manager API, then
# deprovisions its Thunder ID instance, then uninstalls the API Platform
# Gateway helm release. Thunder teardown runs only after a confirmed
# 204/404 from AMS — tearing it down earlier would leave the environment
# live in AMS while its identity layer is permanently gone.
#
# All inputs are provided via environment variables so the script can be piped
# directly into bash:
#
#   curl -fsSL https://raw.githubusercontent.com/wso2/agent-manager/main/deployments/scripts/remove-environment.sh \
#     | ENV_NAME=staging \
#       AGENT_MANAGER_TOKEN=<token> \
#       bash
#
# Required:
#   - ENV_NAME: the environment name to remove (cannot be 'default')
#   - AGENT_MANAGER_TOKEN: bearer token authorized to delete environments
# Optional:
#   - ORG_NAME (default: default)
#   - AGENT_MANAGER_URL (default: http://api.amp.localhost:8080)
#   - GATEWAY_NAMESPACE (default: openchoreo-data-plane)
#   - DEPROVISION_THUNDER (default: true) — set to false to skip Thunder removal
#   - THUNDER_SCRIPT_URL — override the URL of remove-environment-thunder.sh

# --- Required inputs ---
: "${ENV_NAME:?ENV_NAME is required (e.g. ENV_NAME=staging)}"
: "${AGENT_MANAGER_TOKEN:?AGENT_MANAGER_TOKEN is required (bearer token)}"

if [ "$ENV_NAME" = "default" ]; then
    echo "❌ Cannot remove the default environment"
    exit 1
fi

# --- Configuration ---
ORG_NAME="${ORG_NAME:-default}"
AGENT_MANAGER_URL="${AGENT_MANAGER_URL:-http://api.amp.localhost:8080}"
AGENT_MANAGER_API_URL="${AGENT_MANAGER_API_URL:-${AGENT_MANAGER_URL}/api/v1}"
# Must match the per-org-env namespace add-environment.sh installs into.
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-${ORG_NAME}-${ENV_NAME}}"

# Release name MUST match what add-environment.sh installs. Single org segment.
RELEASE_NAME="api-platform-${ORG_NAME}-${ENV_NAME}"
RELEASE_NAME=$(echo "$RELEASE_NAME" | head -c 53 | sed 's/-*$//')

# Egress names MUST match what add-environment.sh's split-topology block installs.
# Detect rather than remember: a single-topology environment has neither, so
# uninstalling/deleting them below is a no-op and the teardown is byte-identical.
EGRESS_NAMESPACE="${GATEWAY_NAMESPACE}-egress"
EGRESS_RELEASE_NAME=$(echo "api-platform-${ORG_NAME}-${ENV_NAME}-egress" | head -c 53 | sed 's/-*$//')

echo "=== Removing Environment: ${ENV_NAME} ==="
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

# --- Step 1: Delete the environment via Agent Manager API ---
# Thunder is deprovisioned ONLY after a successful 204/404 below.
# Tearing it down first would permanently destroy the identity layer
# (Thunder DB, OAuth clients, JWKS) while the environment is still
# live in AMS/OpenChoreo — with no restore path if the DELETE fails.
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

echo ""
echo "🌍 Deleting environment '${ENV_NAME}'..."
RESP_FILE="$(mktemp)"
trap 'rm -f "$RESP_FILE"' EXIT
DEL_HTTP_CODE=$(curl -s -o "$RESP_FILE" -w "%{http_code}" -X DELETE \
    "${AGENT_MANAGER_API_URL}/orgs/${ORG_NAME}/environments/${ENV_NAME}" \
    -H "Authorization: Bearer ${AGENT_MANAGER_TOKEN}")

case "$DEL_HTTP_CODE" in
    204)
        echo "✅ Environment '${ENV_NAME}' deleted"
        ;;
    404)
        echo "ℹ️  Environment '${ENV_NAME}' not found — already deleted"
        ;;
    *)
        echo "⚠️  Failed to delete environment (HTTP ${DEL_HTTP_CODE})"
        cat "$RESP_FILE" 2>/dev/null; echo
        exit 1
        ;;
esac

# --- Step 2: Deprovision the environment's Thunder ID instance ---
# Runs only after the environment is confirmed gone from AMS (204/404 above).
# Failure is non-fatal: gateway removal proceeds regardless.
if [ "${DEPROVISION_THUNDER:-true}" = "true" ]; then
    echo ""
    echo "🔐 Removing Thunder ID instance for '${ENV_NAME}'..."
    # The console overrides THUNDER_SCRIPT_URL to a release-pinned URL but never
    # sets SCRIPT_BASE_URL directly — derive it from THUNDER_SCRIPT_URL's own
    # directory so the chained script's thunder-naming.sh/ams-auth.sh fetches
    # use that SAME release ref instead of silently defaulting to main.
    if [ -z "${SCRIPT_BASE_URL:-}" ] && [ -n "${THUNDER_SCRIPT_URL:-}" ]; then
        SCRIPT_BASE_URL="$(dirname "$THUNDER_SCRIPT_URL")"
    fi
    SCRIPT_BASE_URL="${SCRIPT_BASE_URL:-https://raw.githubusercontent.com/wso2/agent-manager/main/deployments/scripts}"
    THUNDER_SCRIPT_URL="${THUNDER_SCRIPT_URL:-${SCRIPT_BASE_URL}/remove-environment-thunder.sh}"
    script_tmp="$(mktemp)"
    if curl -fsSL "${THUNDER_SCRIPT_URL}" -o "$script_tmp"; then
      # Forward the API URL and token already validated by this script so the
      # chained cleanup updates the same Agent Manager deployment.
      if ENV_NAME="${ENV_NAME}" ORG_NAME="${ORG_NAME}" SCRIPT_BASE_URL="${SCRIPT_BASE_URL}" \
          AMP_API_URL="${AGENT_MANAGER_API_URL}" AGENT_MANAGER_TOKEN="${AGENT_MANAGER_TOKEN}" \
          bash "$script_tmp"; then
        echo "✅ Thunder ID instance removed"
      else
        echo "⚠️  Thunder ID removal failed — continuing with gateway removal."
        echo "    Re-run: curl -fsSL ${THUNDER_SCRIPT_URL} | ENV_NAME=${ENV_NAME} ORG_NAME=${ORG_NAME} bash"
      fi
    else
      echo "⚠️  Failed to fetch Thunder ID removal script from ${THUNDER_SCRIPT_URL}"
    fi
    rm -f "$script_tmp"
fi

# --- Step 3: Uninstall the gateway helm release(s) ---
# Detect rather than remember: a single-topology environment has neither, so this is a
# no-op and the teardown is byte-identical.
if helm status "${EGRESS_RELEASE_NAME}" --namespace "${EGRESS_NAMESPACE}" > /dev/null 2>&1; then
    echo ""
    echo "🌐 Uninstalling egress API Platform Gateway..."
    helm uninstall "${EGRESS_RELEASE_NAME}" --namespace "${EGRESS_NAMESPACE}"
    echo "✅ Egress gateway helm release uninstalled"
fi

echo ""
echo "🌐 Uninstalling API Platform Gateway..."
if helm status "${RELEASE_NAME}" --namespace "${GATEWAY_NAMESPACE}" > /dev/null 2>&1; then
    helm uninstall "${RELEASE_NAME}" --namespace "${GATEWAY_NAMESPACE}"
    echo "✅ Gateway helm release uninstalled"
else
    echo "ℹ️  Gateway helm release '${RELEASE_NAME}' not found, skipping..."
fi

# Wait for gateway operator to clean up the APIGateway CR
GATEWAY_NAME=$(printf "api-platform-%s-%s" "${ORG_NAME}" "${ENV_NAME}" | head -c 63 | sed 's/-*$//')
echo ""
echo "⏳ Waiting for gateway resources to be cleaned up..."
if kubectl wait --for=delete "apigateway/${GATEWAY_NAME}" -n "${GATEWAY_NAMESPACE}" --timeout=120s 2>/dev/null; then
    echo "✅ Gateway resources cleaned up"
else
    echo "⚠️  Timed out or failed waiting for apigateway/${GATEWAY_NAME} to delete; continuing..."
fi

# --- Step 4: Delete the per-env gateway namespace ---
# Only when it follows the "<org>-<env>" isolation convention — never delete a
# shared namespace (e.g. a legacy install with GATEWAY_NAMESPACE=openchoreo-data-plane).
if [ "${GATEWAY_NAMESPACE}" = "${ORG_NAME}-${ENV_NAME}" ]; then
    if kubectl get namespace "${EGRESS_NAMESPACE}" > /dev/null 2>&1; then
        echo ""
        echo "🧹 Deleting egress gateway namespace '${EGRESS_NAMESPACE}'..."
        kubectl delete namespace "${EGRESS_NAMESPACE}" --timeout=120s 2>/dev/null \
            && echo "✅ Egress namespace deleted" \
            || echo "ℹ️  Egress namespace not found or already deleting, skipping..."
    fi

    echo ""
    echo "🧹 Deleting gateway namespace '${GATEWAY_NAMESPACE}'..."
    if kubectl delete namespace "${GATEWAY_NAMESPACE}" --timeout=120s 2>/dev/null; then
        echo "✅ Namespace deleted"
    else
        echo "ℹ️  Namespace '${GATEWAY_NAMESPACE}' not found or already deleting, skipping..."
    fi
fi

echo ""
echo "=== Environment '${ENV_NAME}' removed ==="
echo ""
