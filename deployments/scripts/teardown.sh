#!/bin/bash
set -e

echo "=== Tearing Down Agent Manager Development Environment ==="

# Stop Docker Compose services
if [ -f "../docker-compose.yml" ]; then
    echo "🛑 Stopping Agent Manager platform services..."
    cd ..
    docker compose down -v
    cd scripts
    echo "✅ Platform services stopped"
else
    echo "⚠️  docker-compose.yml not found, skipping platform teardown"
fi

echo ""

# Delete Kind cluster
if command -v kind &> /dev/null; then
    if kind get clusters 2>/dev/null | grep -q "^openchoreo-local$"; then
        echo "🛑 Deleting Kind cluster 'openchoreo-local'..."
        kind delete cluster --name openchoreo-local
        echo "✅ Kind cluster deleted"
    else
        echo "⚠️  Kind cluster 'openchoreo-local' not found"
    fi
else
    echo "⚠️  Kind not installed, skipping cluster deletion"
fi

echo ""

# Clean up Docker kubeconfig
if [ -f ~/.kube/config-docker ]; then
    echo "🧹 Removing Docker kubeconfig..."
    rm -f ~/.kube/config-docker
    echo "✅ Docker kubeconfig removed"
fi

# Clean up kubeconfig marker file
if [ -f ../../.make/kubeconfig-docker-generated ]; then
    echo "🧹 Removing kubeconfig marker file..."
    rm -f ../../.make/kubeconfig-docker-generated
    echo "✅ Kubeconfig marker removed"
fi

echo ""

# Clean up shared directory
if [ -d "/tmp/kind-shared" ]; then
    echo "🧹 Cleaning up /tmp/kind-shared..."
    rm -rf /tmp/kind-shared
    echo "✅ Shared directory cleaned"
fi

echo ""

# Note about Colima
echo "ℹ️  Note: Colima is still running. To stop it:"
echo "   colima stop"
echo ""
echo "   To completely remove Colima:"
echo "   colima delete"

echo ""
echo "✅ Teardown complete!"
