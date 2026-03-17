#!/bin/bash
set -e
# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"
source "$SCRIPT_DIR/env.sh"
source "$SCRIPT_DIR/utils.sh"

echo "=== Setting up k3d Cluster for OpenChoreo ==="

# Check if cluster already exists
if k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
    echo "✅ k3d cluster '${CLUSTER_NAME}' already exists"

    ensure_cluster_accessible

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
    k3d cluster create --config ../k3d-local-config.yaml

    echo ""
    echo "✅ k3d cluster created successfully!"

    refresh_kubeconfig

    if ! wait_for_cluster; then
        echo "❌ Cluster failed to become ready after 30 attempts"
        echo "   Try running: k3d kubeconfig merge ${CLUSTER_NAME} --kubeconfig-merge-default --kubeconfig-switch-context"
        exit 1
    fi
fi

# Apply CoreDNS custom configuration for *.openchoreo.localhost resolution
echo ""
echo "🔧 Applying CoreDNS custom configuration..."
kubectl apply --context "${CLUSTER_CONTEXT}" -f https://raw.githubusercontent.com/openchoreo/openchoreo/v1.0.0-rc.1/install/k3d/common/coredns-custom.yaml
echo "✅ CoreDNS configured to resolve *.openchoreo.localhost"

# Generate Machine IDs for observability
echo ""
generate_machine_ids "$CLUSTER_NAME"
echo ""
