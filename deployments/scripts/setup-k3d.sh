#!/bin/bash
set -e

CLUSTER_NAME="openchoreo-local-v0.9"
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
if k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
    echo "✅ k3d cluster '${CLUSTER_NAME}' already exists"
    
    # Verify cluster is running
    if kubectl cluster-info --context ${CLUSTER_CONTEXT} &>/dev/null; then
        echo "✅ Cluster is running and accessible"
    else
        echo "⚠️  Cluster exists but is not accessible. Starting cluster..."
        k3d cluster start ${CLUSTER_NAME}
        
        # Wait for cluster to be ready
        echo "⏳ Waiting for cluster to be ready..."
        for i in {1..30}; do
            if kubectl cluster-info --context ${CLUSTER_CONTEXT} &>/dev/null; then
                echo "✅ Cluster is now ready"
                break
            fi
            sleep 2
        done
    fi
    
    echo ""
    echo "Cluster info:"
    kubectl cluster-info --context ${CLUSTER_CONTEXT}
    echo ""
    echo "✅ Using existing cluster"
else
    # Create /tmp/k3d-shared directory for OpenChoreo
    echo "📁 Creating shared directory for OpenChoreo..."
    mkdir -p /tmp/k3d-shared

    # Create k3d cluster with OpenChoreo configuration
    echo "🚀 Creating k3d cluster with OpenChoreo configuration..."
    echo "   Setting K3D_FIX_DNS=0 to avoid DNS issues (see k3d-io/k3d#1449)"
    k3d cluster create --config ../single-cluster-config.yaml

    echo ""
    echo "✅ k3d cluster created successfully!"
fi

# Install cert-manager
echo ""
echo "🔧 Installing cert-manager..."
if helm status cert-manager -n cert-manager &>/dev/null; then
    echo "✅ cert-manager is already installed"
else
    echo "📦 Installing cert-manager..."
    
    helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
    --version v1.18.4 \
    --namespace cert-manager \
    --create-namespace \
    --set crds.enabled=true
    
    echo ""
    echo "⏳ Waiting for cert-manager to be ready..."
    kubectl wait --for=condition=available deployment/cert-manager -n cert-manager --timeout=120s
    
    echo ""
    echo "✅ cert-manager is ready!"
fi
echo ""
echo "📊 Cluster Info:"
kubectl cluster-info --context ${CLUSTER_CONTEXT}

echo ""
echo "🔍 Cluster Nodes:"
kubectl get nodes

echo ""
echo "✅ Setup complete! You can now proceed with OpenChoreo installation."
