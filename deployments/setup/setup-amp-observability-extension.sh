#!/bin/bash
set -euo pipefail

# Installs the wso2-amp-observability-extension chart (Agent Manager Observer)
# into the OpenChoreo observability plane. The plane must be up: the chart ships
# a ServiceMonitor, whose CRD comes from the plane's metrics module. The
# amp-observer image must already be loaded into the cluster
# (`make -C agent-manager-observer docker-load-k3d`).

PROJECT_ROOT="$1"

echo "📦 Observability Extension (Agent Manager Observer)"
echo "   Installing/upgrading Agent Manager Observer (local dev: JWKS disabled, unverified JWT parse)..."
helm upgrade --install wso2-amp-observability-extension "${PROJECT_ROOT}/deployments/helm-charts/wso2-amp-observability-extension" \
    --create-namespace \
    --namespace openchoreo-observability-plane \
    --timeout=10m \
    --set amObserver.developmentMode=true \
    --set amObserver.auth.isLocalDevEnv=true \
    --set-string amObserver.auth.jwksUrl=""
echo ""
