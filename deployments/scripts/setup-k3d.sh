#!/bin/bash
set -e
# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"
source "$SCRIPT_DIR/env.sh"

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

if ! command -v helm &> /dev/null; then
    echo "❌ helm is not installed. Please install it first:"
    echo "   brew install helm"
    exit 1
fi

# Check if cluster already exists
if k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
    echo "✅ k3d cluster '${CLUSTER_NAME}' already exists"

    # Regenerate kubeconfig to ensure it's fresh
    echo "🔄 Refreshing kubeconfig..."
    k3d kubeconfig merge ${CLUSTER_NAME} --kubeconfig-merge-default --kubeconfig-switch-context

    # Verify cluster is running
    echo "🔍 Checking cluster accessibility..."
    if kubectl cluster-info --context ${CLUSTER_CONTEXT} --request-timeout=10s &>/dev/null; then
        echo "✅ Cluster is running and accessible"
    else
        echo "⚠️  Cluster exists but is not accessible. Restarting cluster..."
        k3d cluster stop ${CLUSTER_NAME}
        k3d cluster start ${CLUSTER_NAME}

        # Wait for cluster to be ready
        echo "⏳ Waiting for cluster to be ready..."
        for i in {1..30}; do
            if kubectl cluster-info --context ${CLUSTER_CONTEXT} --request-timeout=5s &>/dev/null; then
                echo "✅ Cluster is now ready"
                break
            fi
            echo "   Attempt $i/30..."
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
    K3D_FIX_DNS=0 k3d cluster create --config ../single-cluster-config.yaml

    echo ""
    echo "✅ k3d cluster created successfully!"
fi

# Apply CoreDNS custom configuration for *.openchoreo.localhost resolution
echo ""
echo "🔧 Applying CoreDNS custom configuration..."
kubectl apply -f https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.16/install/k3d/common/coredns-custom.yaml
echo "✅ CoreDNS configured to resolve *.openchoreo.localhost"

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
echo ""
