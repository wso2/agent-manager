#!/bin/bash
set -e

# Installs the API Platform Gateway extension chart.
# Must run AFTER Agent Manager is up and migrations have completed,
# because the bootstrap job registers the gateway via the Agent Manager API.
#
#   setup-gateway.sh           # default: agent-manager runs via docker-compose


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Gateway identity for the bootstrap default environment. Keep in sync with
# add-environment.sh so every env's gateway record uses the same vhost convention
# (otherwise the agent-manager `gateways.vhost` row is set from the chart's
# misleading static default).
ORG_NAME="${ORG_NAME:-default}"
ENV_NAME="${ENV_NAME:-default}"
GATEWAY_VHOST_PORT="${GATEWAY_VHOST_PORT:-19080}"
GATEWAY_VHOST="${GATEWAY_VHOST:-http://${ENV_NAME}-${ORG_NAME}.gateway.localhost:${GATEWAY_VHOST_PORT}}"
IDP_SKIP_TLS_VERIFY="${IDP_SKIP_TLS_VERIFY:-true}"
case "$IDP_SKIP_TLS_VERIFY" in
    true|false) ;;
    *)
        echo "❌ IDP_SKIP_TLS_VERIFY must be 'true' or 'false' (got '${IDP_SKIP_TLS_VERIFY}')"
        exit 1
        ;;
esac

# shellcheck source=../scripts/thunder-naming.sh
source "${SCRIPT_DIR}/../scripts/thunder-naming.sh"

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
# "<org>-<env>" namespace (see add-environment.sh). apiGateway.namespace must
# match --namespace — it drives where the chart renders the APIGateway CR,
# config, RestApis, kgateway backendRef and token secret.
GATEWAY_NAMESPACE="${GATEWAY_NAMESPACE:-${ORG_NAME}-${ENV_NAME}}"

# Wire the gateway's ThunderKeyManager to this environment's own Thunder instance
# when it exists, mirroring the THUNDER_PROVISIONED logic in add-environment.sh.
THUNDER_RELEASE="$(thunder_release_name "${ORG_NAME}" "${ENV_NAME}")"
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
if helm status "${THUNDER_RELEASE}" --namespace "${THUNDER_RELEASE}" > /dev/null 2>&1; then
    echo "✅ Env-Thunder instance found (Helm release: ${THUNDER_RELEASE}) — wiring gateway to it"
    THUNDER_ISSUER="$(thunder_issuer "${ORG_NAME}" "${ENV_NAME}")"
    THUNDER_INTERNAL_JWKS="http://${THUNDER_RELEASE}-service.${THUNDER_RELEASE}.svc.cluster.local:8090/oauth2/jwks"
    HELM_ARGS+=(
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].name=ThunderKeyManager"
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].issuer=${THUNDER_ISSUER}"
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].jwks.remote.uri=${THUNDER_INTERNAL_JWKS}"
        --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].jwks.remote.skipTlsVerify=${IDP_SKIP_TLS_VERIFY}"
        # Name must match keymanagers[].name, which is always "ThunderKeyManager" (set above).
        --set "bootstrap.identityProviders[0].name=ThunderKeyManager"
        --set "bootstrap.identityProviders[0].issuer=${THUNDER_ISSUER}"
        --set "bootstrap.identityProviders[0].jwksUri=${THUNDER_INTERNAL_JWKS}"
        --set "bootstrap.identityProviders[0].skipTlsVerify=${IDP_SKIP_TLS_VERIFY}"
    )
else
    echo "ℹ️  No env-Thunder instance found for '${ENV_NAME}' — gateway will use values-dev.yaml's platform Thunder default"
fi

echo ""
echo "🌐 Installing gateway chart..."

# Ensure the gateway namespace exists and carries the label the sandbox
# NetworkPolicy (agent-api ComponentType) matches for agent telemetry egress.
# Without it, agents in this (default) environment cannot reach the gateway's
# OTEL or managed LLM/MCP endpoints when it runs outside openchoreo-data-plane.
kubectl create namespace "${GATEWAY_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f - > /dev/null
kubectl label namespace "${GATEWAY_NAMESPACE}" "amp.wso2.com/api-platform-gateway=true" --overwrite > /dev/null

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
