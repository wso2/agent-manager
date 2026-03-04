#!/bin/bash
set -e

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"

source "$SCRIPT_DIR/env.sh"

# Select k3d config based on DEV_MODE
if [ "${DEV_MODE}" = "helm" ]; then
    K3D_CONFIG="../dev-cluster-config.yaml"
    echo "=== Setting up k3d Cluster for OpenChoreo + AMP (Helm mode) ==="
else
    K3D_CONFIG="../single-cluster-config.yaml"
    echo "=== Setting up k3d Cluster for OpenChoreo ==="
fi

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

if ! command -v helm &> /dev/null; then
    echo "❌ helm is not installed. Please install it first:"
    echo "   brew install helm"
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
    
    # When using Helm mode, verify AMP ports are mapped
    if [ "${DEV_MODE}" = "helm" ]; then
        echo ""
        echo "🔍 Checking AMP port mappings..."
        MISSING_PORTS=""
        for PORT in 3000 9000; do
            if ! docker port "k3d-${CLUSTER_NAME}-serverlb" "${PORT}/tcp" &>/dev/null; then
                MISSING_PORTS="${MISSING_PORTS} ${PORT}"
            fi
        done
        if [ -n "$MISSING_PORTS" ]; then
            echo "⚠️  AMP ports not mapped:${MISSING_PORTS}"
            echo "   The cluster was created without AMP port mappings."
            echo "   To fix, delete and recreate the cluster:"
            echo "     k3d cluster delete ${CLUSTER_NAME}"
            echo "     DEV_MODE=helm make setup-k3d"
        else
            echo "✅ AMP ports are mapped correctly"
        fi
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

    # Create k3d cluster with appropriate configuration
    echo "🚀 Creating k3d cluster with config: ${K3D_CONFIG}..."
    k3d cluster create --config "${K3D_CONFIG}"

    echo ""
    echo "✅ k3d cluster created successfully!"
fi

# Generate Machine IDs for observability
echo ""
echo "🆔 Generating Machine IDs for Fluent Bit observability..."
NODES=$(k3d node list -o json | grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/"name"[[:space:]]*:[[:space:]]*"//;s/"$//' | grep "^k3d-$CLUSTER_NAME-")
if [[ -z "$NODES" ]]; then
    echo "⚠️  Could not retrieve node list"
else
    for NODE in $NODES; do
        echo "   🔧 Generating machine ID for ${NODE}..."
        if docker exec ${NODE} sh -c "cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id" 2>/dev/null; then
            echo "   ✅ Machine ID generated for ${NODE}"
        else
            echo "   ⚠️  Could not generate Machine ID for ${NODE} (it may not be running)"
        fi
    done
fi
echo "✅ Machine ID generation complete"

# Install cert-manager
echo ""
echo "🔧 Installing cert-manager..."
if helm status cert-manager -n cert-manager --kube-context ${CLUSTER_CONTEXT} &>/dev/null; then
    echo "✅ cert-manager is already installed"
else
    echo "📦 Installing cert-manager..."
    
    helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
    --kube-context ${CLUSTER_CONTEXT} \
    --version v1.18.4 \
    --namespace cert-manager \
    --create-namespace \
    --set crds.enabled=true
    
    echo ""
    echo "⏳ Waiting for cert-manager to be ready..."
    kubectl wait --for=condition=available deployment/cert-manager -n cert-manager --context ${CLUSTER_CONTEXT} --timeout=120s
    
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
echo "🔧 Installing Gateway API CRDs..."
GATEWAY_API_CRD="https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml"
if kubectl apply --server-side --force-conflicts -f "${GATEWAY_API_CRD}" &>/dev/null; then
    echo "✅ Gateway API CRDs applied successfully"
else
    echo "❌ Failed to apply Gateway API CRDs"
    exit 1
fi

echo ""
echo "🔧 Installing External Secret Operator..."
helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
    --kube-context ${CLUSTER_CONTEXT} \
    --namespace external-secrets \
    --create-namespace \
    --version 1.3.2 \
    --set installCRDs=true

echo ""
echo "⏳ Waiting for External Secret Operator to be ready..."
kubectl wait --for=condition=Available deployment/external-secrets -n external-secrets --context ${CLUSTER_CONTEXT} --timeout=180s

echo "✅ External Secret Operator is ready!"

echo ""
echo "✅ Setup complete! You can now proceed with OpenChoreo installation."
