#!/bin/bash
set -e

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"

CLUSTER_NAME="openchoreo-local-v0.9"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"

echo "=== Installing WSO2 AMP Thunder Extension ==="

# Check prerequisites
if ! command -v helm &> /dev/null; then
    echo "❌ Helm is not installed. Please install it first:"
    echo "   brew install helm"
    exit 1
fi

if ! kubectl cluster-info --context $CLUSTER_CONTEXT &> /dev/null; then
    echo "❌ K3d cluster '$CLUSTER_CONTEXT' is not running."
    echo "   Run: ./setup-k3d.sh"
    exit 1
fi

echo "🔧 Setting kubectl context to $CLUSTER_CONTEXT..."
kubectl config use-context $CLUSTER_CONTEXT

echo ""
echo "1️⃣  Installing/Upgrading WSO2 AMP Thunder Extension..."
echo "📦 Updating Helm dependencies..."
helm dependency update "${SCRIPT_DIR}/../helm-charts/wso2-amp-thunder-extension"
helm upgrade --install amp-thunder-extension "${SCRIPT_DIR}/../helm-charts/wso2-amp-thunder-extension" --namespace amp-thunder --create-namespace
echo "✅ AMP Thunder Extension installed/upgraded successfully"

echo "⏳ Waiting for AMP Thunder Extension pods to be ready (timeout: 5 minutes)..."
kubectl wait -n amp-thunder --for=condition=available --timeout=300s deployment --all
# Wait for jobs only if any exist
if kubectl get jobs -n amp-thunder --no-headers 2>/dev/null | grep -q .; then
    kubectl wait -n amp-thunder --for=condition=complete --timeout=300s job --all
fi
echo "✅ AMP Thunder Extension ready"
echo ""

echo "🔍 Verifying installation..."
kubectl get pods -n amp-thunder
echo ""

echo "✅ WSO2 AMP Thunder Extension installation complete!"
