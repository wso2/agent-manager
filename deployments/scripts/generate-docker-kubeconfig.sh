#!/bin/bash

# Generate Docker-specific kubeconfig for kind cluster using --internal flag
# This creates a kubeconfig with internal cluster networking suitable for containers

set -e

# Check prerequisites
if ! command -v kind &> /dev/null; then
    echo "❌ kind is not installed. Please install it first:"
    echo "   brew install kind"
    exit 1
fi

if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed. Please install it first:"
    echo "   brew install kubectl"
    exit 1
fi

DOCKER_KUBECONFIG="$HOME/.kube/config-docker"
CLUSTER_NAME="openchoreo-local"

echo "🔧 Generating Docker kubeconfig using kind --internal..."

# Check if the specific cluster exists
if ! kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    echo "❌ Kind cluster '${CLUSTER_NAME}' not found"
    echo "   Available clusters:"
    kind get clusters
    echo "   Please run 'make setup-kind' first"
    exit 1
fi

# Remove existing config-docker if it's a directory or file
if [ -e "$DOCKER_KUBECONFIG" ]; then
    echo "🧹 Removing existing $DOCKER_KUBECONFIG"
    rm -f "$DOCKER_KUBECONFIG"
fi

# Create ~/.kube directory if it doesn't exist
mkdir -p "$(dirname "$DOCKER_KUBECONFIG")"

# Generate kubeconfig with internal cluster IP
echo "🔧 Generating kubeconfig for cluster: $CLUSTER_NAME"
if ! kind get kubeconfig --name "$CLUSTER_NAME" --internal > "$DOCKER_KUBECONFIG"; then
    echo "❌ Failed to generate kubeconfig for cluster '$CLUSTER_NAME'"
    exit 1
fi

# Set the context
EXPECTED_CONTEXT="kind-$CLUSTER_NAME"
if ! kubectl --kubeconfig="$DOCKER_KUBECONFIG" config use-context "$EXPECTED_CONTEXT" &> /dev/null; then
    echo "❌ Failed to set context to '$EXPECTED_CONTEXT'"
    exit 1
fi

echo "✅ Docker kubeconfig generated at: $DOCKER_KUBECONFIG"

# Show the generated server URL and context
SERVER_URL=$(kubectl --kubeconfig="$DOCKER_KUBECONFIG" config view -o jsonpath='{.clusters[0].cluster.server}')
echo "   Server URL: $SERVER_URL"
echo "   Context: $EXPECTED_CONTEXT"
