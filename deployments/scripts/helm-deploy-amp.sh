#!/bin/bash
set -e

# Deploy AMP to the k3d cluster using Helm.
# Usage:
#   ./helm-deploy-amp.sh              # Install or upgrade
#   ./helm-deploy-amp.sh --uninstall  # Uninstall (preserves cluster)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
CHART_DIR="$ROOT_DIR/deployments/helm-charts/wso2-agent-manager"
VALUES_FILE="$ROOT_DIR/deployments/values/values-local.yaml"

source "$SCRIPT_DIR/env.sh"

if [ "$1" = "--uninstall" ]; then
    echo "=== Uninstalling AMP from k3d ==="
    if helm status "$AMP_RELEASE_NAME" -n "$AMP_NAMESPACE" --kube-context "${CLUSTER_CONTEXT}" &>/dev/null; then
        helm uninstall "$AMP_RELEASE_NAME" -n "$AMP_NAMESPACE" --kube-context "${CLUSTER_CONTEXT}"
        echo "AMP uninstalled. Cluster and namespace preserved."
    else
        echo "AMP release '${AMP_RELEASE_NAME}' not found in namespace '${AMP_NAMESPACE}'."
    fi
    exit 0
fi

echo "=== Deploying AMP to k3d cluster ==="
echo ""

# Verify cluster is accessible
if ! kubectl cluster-info --context "${CLUSTER_CONTEXT}" &>/dev/null; then
    echo "k3d cluster '${CLUSTER_NAME}' is not accessible."
    echo "Run 'make setup-k3d' or 'k3d cluster start ${CLUSTER_NAME}' first."
    exit 1
fi

# Create namespace if it doesn't exist
kubectl create namespace "$AMP_NAMESPACE" --context "${CLUSTER_CONTEXT}" --dry-run=client -o yaml | \
    kubectl apply --context "${CLUSTER_CONTEXT}" -f -

# Update Helm dependencies
echo "Updating Helm chart dependencies..."
helm dependency update "$CHART_DIR"
echo ""

# Install or upgrade
echo "Running helm upgrade --install..."
helm upgrade --install "$AMP_RELEASE_NAME" "$CHART_DIR" \
    --namespace "$AMP_NAMESPACE" \
    --kube-context "${CLUSTER_CONTEXT}" \
    --values "$VALUES_FILE" \
    --wait \
    --timeout 5m

echo ""
echo "Waiting for deployments to be ready..."
kubectl wait --for=condition=Available deployment --all \
    -n "$AMP_NAMESPACE" \
    --context "${CLUSTER_CONTEXT}" \
    --timeout=300s 2>/dev/null || true

echo ""
echo "AMP deployed successfully!"
echo ""
echo "Services:"
echo "  Console:   http://localhost:3000"
echo "  API:       http://localhost:9000"
echo ""
echo "Status:"
kubectl get pods -n "$AMP_NAMESPACE" --context "${CLUSTER_CONTEXT}"
