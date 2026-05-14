#!/bin/bash
set -e

# Installs the API Platform Gateway extension chart.
# Must run AFTER Agent Manager is up and migrations have completed,
# because the bootstrap job registers the gateway via the Agent Manager API.
#
#   setup-gateway.sh           # default: agent-manager runs via docker-compose


SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

echo ""
echo "🌐 Installing gateway chart..."
helm upgrade --install api-platform-default-default \
    "${SCRIPT_DIR}/../helm-charts/wso2-amp-api-platform-gateway-extension" \
    --namespace openchoreo-data-plane \
    --set agentManager.orgName=default \
    --set gateway.environment=default \
    --set agentManager.apiUrl="http://host.docker.internal:9000/api/v1" \
    --set apiGateway.controlPlane.host="host.docker.internal:9243" \
    -f "${SCRIPT_DIR}/../helm-charts/wso2-amp-api-platform-gateway-extension/values-dev.yaml"

echo "⏳ Waiting for Gateway to be ready..."
if kubectl wait --for=condition=Programmed apigateway/api-platform-default-default -n openchoreo-data-plane --timeout=180s; then
    echo "✅ Gateway is programmed"
else
    echo "⚠️  Gateway did not become ready in time"
fi

kubectl apply -f "${SCRIPT_DIR}/../values/otel-collector-rest-api.yaml"

echo "⏳ Waiting for RestApi to be programmed..."
if kubectl wait --for=condition=Programmed restapi/amp-otel-collector-tracing-rest-api  -n openchoreo-data-plane --timeout=120s; then
    echo "✅ RestApi is programmed"
else
    echo "⚠️  RestApi did not become ready in time"
fi

echo ""
echo "✅ API Platform Gateway installed"
