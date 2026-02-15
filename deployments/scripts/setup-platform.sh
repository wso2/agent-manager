#!/bin/bash
set -e

echo "=== Setting up Agent Manager Core Platform ==="

# Check if Docker is available
if ! docker info &> /dev/null; then
    echo "❌ Docker is not running. Please start Colima first:"
    echo "   ./setup-colima.sh"
    exit 1
fi

# Get project root (two directories up from this script)
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Build and load evaluation-job image to k3d
echo "📊 Building evaluation-job image and loading to k3d..."
cd "${PROJECT_ROOT}/evaluation-job"
make docker-load-k3d || {
    echo "⚠️  Failed to build/load evaluation-job to k3d"
    echo "   Make sure k3d cluster is running"
    echo "   You can load it later with: cd evaluation-job && make docker-load-k3d"
}

# Check if docker-compose file exists
if [ ! -f "${PROJECT_ROOT}/deployments/docker-compose.yml" ]; then
    echo "❌ docker-compose.yml not found"
    exit 1
fi

echo "🚀 Starting Agent Manager platform services..."
cd "${PROJECT_ROOT}/deployments"
docker compose up -d

echo ""
echo "⏳ Waiting for services to be healthy..."
sleep 5

echo ""
echo "📊 Service Status:"
docker compose ps

echo ""
echo "✅ Agent Manager platform is running!"
echo ""
echo "🌐 Access points:"
echo "   Console:   http://localhost:3000"
echo "   API:       http://localhost:9000"
echo "   Database:  postgresql://agentmanager:agentmanager@localhost:5432/agentmanager"
echo ""
echo "📋 View logs:"
echo "   docker compose logs -f"
echo ""
echo "🛑 Stop services:"
echo "   docker compose down"
