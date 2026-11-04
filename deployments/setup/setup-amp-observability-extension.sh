#!/bin/bash
set -euo pipefail

# Installs the wso2-amp-observability-extension chart (Agent Manager Observer)
# into the openchoreo-observability-plane namespace. In full mode the OpenChoreo
# plane must be up: the chart ships a ServiceMonitor, whose CRD comes from the
# plane's Prometheus metrics module. Lite mode has no Prometheus, and the chart
# omits the ServiceMonitor there. The amp-observer image must already be loaded
# into the cluster (build_observer_image in utils.sh).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/env.sh"
source "$SCRIPT_DIR/utils.sh"
PROJECT_ROOT="$1"

mode=$(observability_mode)
mode_values=()
if [ "$mode" = "lite" ]; then
    mode_values=(-f "${PROJECT_ROOT}/deployments/values/observer-lite.yaml")
fi

echo "📦 Observability Extension (Agent Manager Observer)"
echo "   Installing/upgrading Agent Manager Observer (local dev: JWKS disabled, unverified JWT parse)..."
helm upgrade --install wso2-amp-observability-extension "${PROJECT_ROOT}/deployments/helm-charts/wso2-amp-observability-extension" \
    --create-namespace \
    --namespace openchoreo-observability-plane \
    --timeout=10m \
    --set amObserver.developmentMode=true \
    --set amObserver.auth.isLocalDevEnv=true \
    --set-string amObserver.auth.jwksUrl="" \
    ${mode_values[@]+"${mode_values[@]}"}
echo ""
