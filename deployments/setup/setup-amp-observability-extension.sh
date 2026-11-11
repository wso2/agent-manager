#!/bin/bash
set -euo pipefail

# Installs AMP's trace pipeline into the amp-observability namespace:
# OpenSearch + the OpenTelemetry collector from upstream charts
# (../scripts/lib-observability.sh), then the wso2-amp-observability-extension
# chart (Agent Manager Observer). The amp-observer image must already be loaded
# into the cluster (`make -C agent-manager-observer docker-load-k3d`).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$1"
source "$SCRIPT_DIR/env.sh"
# shellcheck source=../scripts/lib-observability.sh
source "$SCRIPT_DIR/../scripts/lib-observability.sh"

kubectl config use-context "$CLUSTER_CONTEXT" >/dev/null

echo "📦 Trace storage (OpenSearch + OpenTelemetry Collector)"
install_observability_stack "${PROJECT_ROOT}/deployments"

echo "📦 Observability Extension (Agent Manager Observer)"
echo "   Installing/upgrading Agent Manager Observer (local dev: JWKS disabled, unverified JWT parse)..."
helm upgrade --install wso2-amp-observability-extension "${PROJECT_ROOT}/deployments/helm-charts/wso2-amp-observability-extension" \
    --create-namespace \
    --namespace "${OBSERVABILITY_NAMESPACE}" \
    --timeout=10m \
    --set amObserver.developmentMode=true \
    --set amObserver.auth.isLocalDevEnv=true \
    --set-string amObserver.auth.jwksUrl=""
echo ""
