#!/bin/bash
set -e

# Installs the API Platform Gateway extension chart.
# Must run AFTER Agent Manager is up and migrations have completed,
# because the bootstrap job registers the gateway via the Agent Manager API.
#
#   setup-gateway.sh           # default: agent-manager runs via docker-compose


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Gateway identity for the org's internal default environment (otherwise the
# agent-manager `gateways.vhost` row is set from the chart's misleading static
# default).
ORG_NAME="${ORG_NAME:-default}"
ENV_NAME="${ENV_NAME:-default}"
GATEWAY_VHOST_PORT="${GATEWAY_VHOST_PORT:-19080}"
GATEWAY_VHOST="${GATEWAY_VHOST:-http://${ENV_NAME}-${ORG_NAME}.gateway.localhost:${GATEWAY_VHOST_PORT}}"


echo "=== Installing API Platform Gateway ==="

# Verify Agent Manager is reachable
echo "⏳ Checking Agent Manager is healthy..."
MAX_WAIT=60
ELAPSED=0
AGENT_MANAGER_HEALTH_URL="${AGENT_MANAGER_HEALTH_URL:-http://localhost:9000/healthz}"
until curl -sf "$AGENT_MANAGER_HEALTH_URL" > /dev/null 2>&1; do
    if [ "$ELAPSED" -ge "$MAX_WAIT" ]; then
        echo "❌ Agent Manager not reachable at ${AGENT_MANAGER_HEALTH_URL} after ${MAX_WAIT}s"
        echo "   Make sure docker-compose services are up and migrations have run."
        exit 1
    fi
    sleep 3
    ELAPSED=$((ELAPSED + 3))
done
echo "✅ Agent Manager is healthy"

# Per-org-env namespace isolation: the gateway stack lives in its own
# "<org>-<env>" namespace. apiGateway.namespace must
# match --namespace — it drives where the chart renders the APIGateway CR,
# config, RestApis, kgateway backendRef and token secret.
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-${ORG_NAME}-${ENV_NAME}}"

# The gateway trusts the install's single Thunder (values-dev.yaml keymanagers).
HELM_ARGS=(
    upgrade --install "api-platform-${ORG_NAME}-${ENV_NAME}"
    "${SCRIPT_DIR}/../helm-charts/wso2-amp-api-platform-gateway-extension"
    --namespace "${GATEWAY_NAMESPACE}"
    --create-namespace
    --set apiGateway.namespace="${GATEWAY_NAMESPACE}"
    --set agentManager.orgName="${ORG_NAME}"
    --set gateway.environment="${ENV_NAME}"
    --set gateway.vhost="${GATEWAY_VHOST}"
    --set agentManager.apiUrl="http://host.docker.internal:9000/api/v1"
    --set apiGateway.controlPlane.host="host.docker.internal:9243"
    -f "${SCRIPT_DIR}/../helm-charts/wso2-amp-api-platform-gateway-extension/values-dev.yaml"
)
echo ""
echo "🌐 Installing gateway chart..."

# Ensure the gateway namespace exists and carries the label NetworkPolicies
# (e.g. teardown-amp.sh's namespace sweep) select AMP gateway namespaces by.
kubectl create namespace "${GATEWAY_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f - > /dev/null
kubectl label namespace "${GATEWAY_NAMESPACE}" "amp.wso2.com/api-platform-gateway=true" --overwrite > /dev/null

GATEWAY_ENCRYPTION_SECRET_NAME="${GATEWAY_ENCRYPTION_SECRET_NAME:-gateway-encryption-keys}"
GATEWAY_ENCRYPTION_SECRET_KEY="${GATEWAY_ENCRYPTION_SECRET_KEY:-default-aesgcm256-v1.bin}"

if ! kubectl auth can-i get secrets -n "${GATEWAY_NAMESPACE}" &>/dev/null; then
    echo "❌ Missing 'get' permission on secrets in '${GATEWAY_NAMESPACE}' — required to detect an existing gateway encryption key secret. Grant get (in addition to create) on secrets in this namespace to the identity running this script." >&2
    exit 1
fi

key_tmp="$(mktemp)"
trap 'rm -f "${key_tmp}"' EXIT INT TERM
openssl rand 32 > "${key_tmp}"
enc_create_out="$(kubectl create secret generic "${GATEWAY_ENCRYPTION_SECRET_NAME}" -n "${GATEWAY_NAMESPACE}" \
    "--from-file=${GATEWAY_ENCRYPTION_SECRET_KEY}=${key_tmp}" 2>&1)" && enc_create_rc=0 || enc_create_rc=$?
rm -f "${key_tmp}" # normal cleanup: don't leave the plaintext key on disk
trap - EXIT INT TERM
if [ "${enc_create_rc}" -eq 0 ]; then
    echo "✅ Gateway encryption key secret created in '${GATEWAY_NAMESPACE}'"
elif kubectl get secret "${GATEWAY_ENCRYPTION_SECRET_NAME}" -n "${GATEWAY_NAMESPACE}" &>/dev/null; then
    echo "⏭️  Gateway encryption key secret '${GATEWAY_ENCRYPTION_SECRET_NAME}' already exists in '${GATEWAY_NAMESPACE}', leaving it untouched."
else
    echo "❌ Failed to create gateway encryption key secret '${GATEWAY_ENCRYPTION_SECRET_NAME}' in '${GATEWAY_NAMESPACE}': ${enc_create_out}" >&2
    exit 1
fi

helm "${HELM_ARGS[@]}"

echo "⏳ Waiting for Gateway to be ready..."
if kubectl wait --for=condition=Programmed "apigateway/api-platform-${ORG_NAME}-${ENV_NAME}" -n "${GATEWAY_NAMESPACE}" --timeout=180s; then
    echo "✅ Gateway is programmed"
else
    echo "⚠️  Gateway did not become ready in time"
fi

# The OTEL ingest RestApi is provisioned by the gateway extension chart
# (templates/gateway-otel-restapi.yaml) with the restapi-target label that
# binds it to this gateway. Wait for that chart-managed route to program; it
# carries the jwt-auth claim mappings agents need to export traces. (The
# standalone values/otel-collector-rest-api.yaml is not applied here: it lacks
# the restapi-target label, so it can never bind and stays GatewayNotReady.)
OTEL_RESTAPI="api-platform-${ORG_NAME}-${ENV_NAME}-otel-restapi"

echo "⏳ Waiting for OTEL ingest RestApi to be programmed..."
if kubectl wait --for=condition=Programmed "restapi/${OTEL_RESTAPI}" -n "${GATEWAY_NAMESPACE}" --timeout=300s; then
    echo "✅ OTEL ingest RestApi is programmed"
else
    echo "❌ RestApi ${OTEL_RESTAPI} did not become Programmed in time"
    kubectl describe "restapi/${OTEL_RESTAPI}" -n "${GATEWAY_NAMESPACE}" || true
    exit 1
fi

echo ""
echo "✅ API Platform Gateway installed"
