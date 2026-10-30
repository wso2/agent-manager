#!/bin/bash
set -euo pipefail

# Ensures the WSO2 API Platform Gateway Operator, and the gateway it deploys,
# are installed AT THE VERSIONS pinned in the gateway extension chart's
# gatewayRelease values, upgrading a stale install if needed. Called by
# setup-openchoreo.sh (full setup) and setup-amp-extensions.sh (setup-amp):
# the operator is base-layer, but a version-pin bump must still land
# on long-lived clusters or the AMP gateway charts render CRs (e.g.
# APIGateway .../v1) the old operator's CRDs don't serve.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/env.sh"
# shellcheck source=../scripts/lib-registry-auth.sh
source "$SCRIPT_DIR/../scripts/lib-registry-auth.sh"

OPERATOR_CHART="oci://ghcr.io/wso2/api-platform/helm-charts/gateway-operator"

# The gateway release the gateway extension chart in this tree is tested with.
GATEWAY_PINS="$(amp_gateway_release_pins \
    < "$SCRIPT_DIR/../helm-charts/wso2-amp-api-platform-gateway-extension/values.yaml")"
if [ -z "$GATEWAY_PINS" ]; then
    echo "❌ No gatewayRelease pins in the gateway extension chart's values.yaml" >&2
    exit 1
fi
{ read -r GATEWAY_CHART_VERSION; read -r GATEWAY_IMAGE_VERSION; read -r GATEWAY_OPERATOR_VERSION; } <<< "$GATEWAY_PINS"
TARGET_CHART="gateway-operator-${GATEWAY_OPERATOR_VERSION}"

current_chart="$(helm list -n openchoreo-data-plane -f '^gateway-operator$' -o json 2>/dev/null \
    | grep -o '"chart":"[^"]*"' | cut -d'"' -f4 || true)"

if [ "$current_chart" = "$TARGET_CHART" ]; then
    # Chart version (and thus CRDs) is already current, details like image tag might be different
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

# The gateway chart and images come from GATEWAY_REGISTRY with their own pull
# Secret, which the operator reads from its namespace.
amp_create_gateway_pull_secret openchoreo-data-plane
GATEWAY_REGISTRY_HELM_ARGS=()
while IFS= read -r arg; do GATEWAY_REGISTRY_HELM_ARGS+=("$arg"); done < <(amp_gateway_operator_helm_args)

helm upgrade --install gateway-operator "$OPERATOR_CHART" \
    --version "${GATEWAY_OPERATOR_VERSION}" \
    --namespace openchoreo-data-plane \
    --create-namespace \
    --set logging.level=debug \
    --set gatewayApi.installStandardCRDs=false \
    --set "gateway.helm.chartVersion=${GATEWAY_CHART_VERSION}" \
    "${GATEWAY_REGISTRY_HELM_ARGS[@]}" \
    --set "gateway.values.gateway.controller.image.tag=${GATEWAY_IMAGE_VERSION}" \
    --set "gateway.values.gateway.gatewayRuntime.image.tag=${GATEWAY_IMAGE_VERSION}" \
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
