#!/bin/bash
set -e

CLUSTER_NAME="openchoreo-local-v0.7"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"

echo "=== Setting up k3d Cluster for OpenChoreo ==="

# Check prerequisites
if ! command -v k3d &> /dev/null; then
    echo "❌ k3d is not installed. Please install it first:"
    echo "   brew install k3d"
    exit 1
fi

if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed. Please install it first:"
    echo "   brew install kubectl"
    exit 1
fi

# Check if cluster already exists
if k3d cluster list 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "✅ k3d cluster '${CLUSTER_NAME}' already exists"
    echo ""
    echo "Cluster info:"
    kubectl cluster-info --context ${CLUSTER_CONTEXT}
    echo ""
    echo "⚠️  To recreate the cluster, delete it first:"
    echo "   k3d cluster delete ${CLUSTER_NAME}"
    exit 0
fi

# Create /tmp/k3d-shared directory for OpenChoreo
echo "📁 Creating shared directory for OpenChoreo..."
mkdir -p /tmp/k3d-shared

# Create k3d cluster with OpenChoreo configuration
echo "🚀 Creating k3d cluster with OpenChoreo configuration..."
k3d cluster create --config ../single-cluster-config.yaml

echo ""
echo "✅ k3d cluster created successfully!"
echo ""
echo "📊 Cluster Info:"
kubectl cluster-info --context ${CLUSTER_CONTEXT}

echo ""
echo "🔍 Cluster Nodes:"
kubectl get nodes

echo ""
echo "✅ Setup complete! You can now proceed with OpenChoreo installation."
