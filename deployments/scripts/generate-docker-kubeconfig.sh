#!/bin/bash

# Generate Docker-specific kubeconfig for k3d cluster using internal networking
# This creates a kubeconfig with internal cluster networking suitable for containers

set -e

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

DOCKER_KUBECONFIG="$HOME/.kube/config-docker"
CLUSTER_NAME="openchoreo-local-v0.9"

echo "🔧 Generating Docker kubeconfig for k3d cluster..."

# Check if the specific cluster exists
if ! k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
    echo "❌ k3d cluster '${CLUSTER_NAME}' not found"
    echo "   Available clusters:"
    k3d cluster list
    echo "   Please run 'make setup-k3d' first"
    exit 1
fi

# Remove existing config-docker if it's a directory or file
if [ -e "$DOCKER_KUBECONFIG" ]; then
    echo "🧹 Removing existing $DOCKER_KUBECONFIG"
    rm -rf "$DOCKER_KUBECONFIG"
fi

# Create ~/.kube directory if it doesn't exist
mkdir -p "$(dirname "$DOCKER_KUBECONFIG")"

# Generate kubeconfig from k3d
echo "🔧 Generating kubeconfig for cluster: $CLUSTER_NAME"
if ! k3d kubeconfig get "$CLUSTER_NAME" > "$DOCKER_KUBECONFIG"; then
    echo "❌ Failed to generate kubeconfig for cluster '$CLUSTER_NAME'"
    exit 1
fi

# Replace the server URL with the internal Docker network hostname
# k3d exposes the API via the serverlb container on port 6443
INTERNAL_SERVER="https://k3d-${CLUSTER_NAME}-serverlb:6443"
echo "🔧 Updating server URL to use internal Docker network: $INTERNAL_SERVER"

# Use sed to replace the server URL in the kubeconfig
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    sed -i '' "s|server: https://.*:.*|server: ${INTERNAL_SERVER}|g" "$DOCKER_KUBECONFIG"
else
    # Linux
    sed -i "s|server: https://.*:.*|server: ${INTERNAL_SERVER}|g" "$DOCKER_KUBECONFIG"
fi

# Set the context
EXPECTED_CONTEXT="k3d-$CLUSTER_NAME"
if ! kubectl --kubeconfig="$DOCKER_KUBECONFIG" config use-context "$EXPECTED_CONTEXT" &> /dev/null; then
    echo "❌ Failed to set context to '$EXPECTED_CONTEXT'"
    exit 1
fi

echo "✅ Docker kubeconfig generated at: $DOCKER_KUBECONFIG"

# Show the generated server URL and context
SERVER_URL=$(kubectl --kubeconfig="$DOCKER_KUBECONFIG" config view -o jsonpath='{.clusters[0].cluster.server}')
echo "   Server URL: $SERVER_URL"
echo "   Context: $EXPECTED_CONTEXT"
