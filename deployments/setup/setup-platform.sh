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
    if [ "$(uname -s)" = "Linux" ]; then
        echo "❌ Docker is not running. Start the daemon first:"
        echo "   sudo systemctl start docker"
    else
        echo "❌ Docker is not running. Please start Colima first:"
        echo "   ./setup-colima.sh"
    fi
    exit 1
fi

if ! docker compose version &> /dev/null; then
    echo "❌ Docker Compose is not installed or not available."
    echo "   Please install Docker Compose plugin."
    exit 1
fi

if ! docker buildx version &> /dev/null; then
    echo "❌ Docker Buildx is not installed or not available."
    echo "   Please install Docker Buildx plugin."
    exit 1
fi

# Checked up front rather than at first use: the migration step below shells out
# to `go run`, and it only runs after images are built and Postgres is up — so a
# missing toolchain would otherwise surface many minutes into the setup.
check_command go

if ! command -v node &> /dev/null; then
    echo "❌ Node.js is not installed."
    echo "   Please install Node.js 22.x (>=22.23.4), 24.x (>=24.22.1) or >=26.12.1."
    exit 1
fi

# Check Node.js version: 22.x >=22.23.4, 24.x >=24.22.1, or >=26.12.1
NODE_MAJOR=$(node -v | sed 's/^v//' | cut -d'.' -f1)
NODE_MINOR=$(node -v | sed 's/^v//' | cut -d'.' -f2)
NODE_PATCH=$(node -v | sed 's/^v//' | cut -d'.' -f3)
NODE_VER=$((NODE_MINOR * 1000 + NODE_PATCH))

if ! { [ "$NODE_MAJOR" -eq 22 ] && [ "$NODE_VER" -ge 23004 ]; } && \
   ! { [ "$NODE_MAJOR" -eq 24 ] && [ "$NODE_VER" -ge 22001 ]; } && \
   ! { [ "$NODE_MAJOR" -eq 26 ] && [ "$NODE_VER" -ge 12001 ]; } && \
   ! [ "$NODE_MAJOR" -gt 26 ]; then
    echo "❌ Node.js must be 22.x (>=22.23.4), 24.x (>=24.22.1) or >=26.12.1."
    echo "   Node 20, 23 and 25, and older patch releases of 22.x, 24.x and 26.x, are not supported."
    echo "   Current version: $(node -v)"
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
if make -C "$PROJECT_ROOT/evaluation-job" docker-load-k3d K3D_CLUSTER_NAME="$CLUSTER_NAME"; then
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
# The service applies its database migrations itself on start (-migrate in .air.toml): the SQLite file lives in
# the agent_manager_data volume, which only the container can reach.
echo "🚀 Starting Agent Manager platform services..."
# --build is required, not an optimisation: plain `up -d` builds an image only
# when none exists, so an image left over from an earlier checkout is reused
# even after its Dockerfile changed. That silently runs a stale toolchain — a
# pre-1.26 image against a go.mod requiring go >= 1.26.0 fails inside Air with
# "GOTOOLCHAIN=local", and the only symptom is a health check timing out much
# later. Layer caching makes the no-change case cost a couple of seconds.
docker compose -f "$COMPOSE_FILE" up -d --build

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
echo "   API:       http://localhost:9000"
echo "   Database:  SQLite at /var/lib/agent-manager/agent-manager.db in the agent-manager-service container (make db-connect)"
echo ""
echo "📋 Useful commands:"
echo "   View logs:      docker compose -f deployments/docker-compose.yml logs -f"
echo "   Stop services:  docker compose -f deployments/docker-compose.yml down"
