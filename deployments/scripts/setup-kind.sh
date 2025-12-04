#!/bin/bash
set -e

echo "=== Setting up Kind Cluster for OpenChoreo ==="

# Check prerequisites
if ! command -v kind &> /dev/null; then
    echo "❌ Kind is not installed. Please install it first:"
    echo "   brew install kind"
    exit 1
fi

if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed. Please install it first:"
    echo "   brew install kubectl"
    exit 1
fi

# Check if cluster already exists
if kind get clusters 2>/dev/null | grep -q "^openchoreo-local$"; then
    echo "✅ Kind cluster 'openchoreo-local' already exists"
    echo ""
    echo "Cluster info:"
    kubectl cluster-info --context kind-openchoreo-local
    echo ""
    echo "⚠️  To recreate the cluster, delete it first:"
    echo "   kind delete cluster --name openchoreo-local"
    exit 0
fi

# Create /tmp/kind-shared directory for OpenChoreo
echo "📁 Creating shared directory for OpenChoreo..."
mkdir -p /tmp/kind-shared

# Create Kind cluster with OpenChoreo configuration
echo "🚀 Creating Kind cluster with OpenChoreo configuration..."
kind create cluster --config ../kind-config.yaml

echo ""
echo "✅ Kind cluster created successfully!"
echo ""
echo "📊 Cluster Info:"
kubectl cluster-info --context kind-openchoreo-local

echo ""
echo "🔍 Cluster Nodes:"
kubectl get nodes

echo ""
echo "✅ Setup complete! You can now proceed with OpenChoreo installation."
