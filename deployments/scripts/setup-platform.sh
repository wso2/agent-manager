#!/bin/bash
set -e

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/env.sh"
source "$SCRIPT_DIR/utils.sh"

# Project root is two directories up from scripts
PROJECT_ROOT="$SCRIPT_DIR/../.."
COMPOSE_FILE="$SCRIPT_DIR/../docker-compose.yml"

echo "=== Setting up Agent Manager Core Platform ==="

# Check prerequisites
if ! docker info &> /dev/null; then
    echo "❌ Docker is not running. Please start Colima first:"
    echo "   ./setup-colima.sh"
    exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
    echo "❌ docker-compose.yml not found at $COMPOSE_FILE"
    exit 1
fi

# ============================================================================
# Step 1: Build and load evaluation-job image
# ============================================================================
echo ""
echo "1️⃣  Build and load evaluation-job image"
echo "📊 Building evaluation-job image and loading to k3d..."
if make -C "$PROJECT_ROOT/evaluation-job" docker-load-k3d; then
    echo "✅ evaluation-job image loaded to k3d"
else
    echo "⚠️  Failed to build/load evaluation-job to k3d"
    echo "   Make sure k3d cluster is running"
    echo "   You can load it later with: cd evaluation-job && make docker-load-k3d"
fi

# ============================================================================
# Step 2: Start platform services
# ============================================================================
echo ""
echo "2️⃣  Start platform services"
echo "🚀 Starting Agent Manager platform services..."
docker compose -f "$COMPOSE_FILE" up -d

echo ""
echo "⏳ Waiting for services to be healthy..."
sleep 5

# ============================================================================
# Step 3: Verify services
# ============================================================================
echo ""
echo "3️⃣  Verify services"
echo "📊 Service Status:"
docker compose -f "$COMPOSE_FILE" ps

echo ""
echo "✅ Agent Manager platform is running!"
echo ""
echo "🌐 Access points:"
echo "   Console:   http://localhost:3000"
echo "   API:       http://localhost:9000"
echo "   Database:  postgresql://agentmanager:agentmanager@localhost:5432/agentmanager"
echo ""
echo "📋 Useful commands:"
echo "   View logs:      docker compose -f deployments/docker-compose.yml logs -f"
echo "   Stop services:  docker compose -f deployments/docker-compose.yml down"
