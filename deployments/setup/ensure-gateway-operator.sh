#!/bin/bash
set -euo pipefail

# Ensures the WSO2 API Platform Gateway Operator is installed AT THE VERSION
# pinned in env.sh, upgrading a stale install if needed. Called by
# setup-openchoreo.sh (full setup) and setup-amp-extensions.sh (setup-amp):
# the operator is base-layer, but a version-pin bump in env.sh must still land
# on long-lived clusters or the AMP gateway charts render CRs (e.g.
# APIGateway .../v1) the old operator's CRDs don't serve.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/env.sh"

OPERATOR_CHART="oci://ghcr.io/wso2/api-platform/helm-charts/gateway-operator"
TARGET_CHART="gateway-operator-${GATEWAY_OPERATOR_VERSION}"

current_chart="$(helm list -n openchoreo-data-plane -f '^gateway-operator$' -o json 2>/dev/null \
    | grep -o '"chart":"[^"]*"' | cut -d'"' -f4 || true)"

if [ "$current_chart" = "$TARGET_CHART" ]; then
    # Chart version (and thus CRDs) is already current, but its Helm values might be different
    echo "⏭️  Gateway Operator already at ${TARGET_CHART} — reapplying Helm values, CRDs unchanged..."
elif [ -n "$current_chart" ]; then
    echo "⚠️  Gateway Operator is ${current_chart}, target is ${TARGET_CHART} — upgrading..."
    # helm upgrade never touches the chart's crds/ directory, so apply them
    # explicitly or the new CR apiVersions are never served. Safe: the chart's
    # CRDs keep serving the old versions alongside the new ones.
    echo "🔧 Applying Gateway Operator CRDs for ${GATEWAY_OPERATOR_VERSION}..."
    helm show crds "$OPERATOR_CHART" --version "${GATEWAY_OPERATOR_VERSION}" \
        | kubectl apply --server-side --force-conflicts -f -
fi

helm upgrade --install gateway-operator "$OPERATOR_CHART" \
    --version "${GATEWAY_OPERATOR_VERSION}" \
    --namespace openchoreo-data-plane \
    --create-namespace \
    --set logging.level=debug \
    --set gatewayApi.installStandardCRDs=false \
    --set "gateway.helm.chartVersion=${GATEWAY_CHART_VERSION}" \
    --set "gateway.values.gateway.controller.image.tag=${GATEWAY_IMAGE_VERSION}" \
    --set gateway.values.gateway.controller.image.repository=ghcr.io/wso2/api-platform/gateway-controller \
    --set "gateway.values.gateway.gatewayRuntime.image.tag=${GATEWAY_IMAGE_VERSION}" \
    --set gateway.values.gateway.gatewayRuntime.image.repository=ghcr.io/wso2/api-platform/gateway-runtime \
    --set gateway.values.gateway.controller.encryptionKeys.enabled=true \
    --set "gateway.values.gateway.controller.encryptionKeys.secretName=${GATEWAY_ENCRYPTION_SECRET_NAME}" \
    --set gateway.values.gateway.controller.deployment.livenessProbe.httpGet.path=/api/admin/v1/health \
    --set gateway.values.gateway.controller.deployment.livenessProbe.httpGet.port=admin \
    --set gateway.values.gateway.controller.deployment.readinessProbe.httpGet.path=/api/admin/v1/health \
    --set gateway.values.gateway.controller.deployment.readinessProbe.httpGet.port=admin

echo "⏳ Waiting for Gateway Operator to be ready..."
kubectl wait -n openchoreo-data-plane --for=condition=available --timeout=180s \
    deployment -l app.kubernetes.io/instance=gateway-operator 2>/dev/null \
    || kubectl wait -n openchoreo-data-plane --for=condition=available --timeout=180s deployment/gateway-operator
echo "✅ Gateway Operator at ${TARGET_CHART}"
