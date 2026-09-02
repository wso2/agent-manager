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
    echo "   Please install Node.js version >=20.19.0 or >=22.12.0."
    exit 1
fi

# Check Node.js version: must be >=20.19.0 or >=22.12.0
NODE_MAJOR=$(node -v | sed 's/^v//' | cut -d'.' -f1)
NODE_MINOR=$(node -v | sed 's/^v//' | cut -d'.' -f2)

if ! { [ "$NODE_MAJOR" -eq 20 ] && [ "$NODE_MINOR" -ge 19 ]; } && \
   ! { [ "$NODE_MAJOR" -eq 22 ] && [ "$NODE_MINOR" -ge 12 ]; } && \
   ! [ "$NODE_MAJOR" -gt 22 ]; then
    echo "❌ Node.js version must be >=20.19.0 or >=22.12.0."
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
# Export console host path so docker-compose can align WORKDIR with the host,
# preventing pnpm virtual-store / node_modules path mismatches.
export CONSOLE_HOST_PATH="$(cd "$SCRIPT_DIR/../../console" && pwd)"

# The console service mounts named volumes at these two paths inside the
# bind-mounted console directory. Any that do not exist yet, Docker creates as
# the container's user — root — and on Linux that ownership lands on the host
# verbatim, so the later host-side `rush install` gets EACCES. (macOS remaps
# bind-mount ownership to the invoking user, which is why it only bites here.)
# Creating them first leaves them owned by this user; Docker never re-chowns an
# existing mountpoint.
mkdir -p "$CONSOLE_HOST_PATH/node_modules" "$CONSOLE_HOST_PATH/common/temp"

# Must migrate before agent-manager-service starts: it crashes on a fresh volume
# (missing tables), and Air never auto-restarts a crash — only rebuilds on file changes.
echo "🐘 Starting Postgres and waiting for it to be healthy..."
docker compose -f "$COMPOSE_FILE" up -d --wait postgres

# The migration run loads agent-manager-service/.env via ENV_FILE_PATH and
# panics if it is missing. That file is gitignored, so a fresh checkout (e.g.
# CI) won't have it; seed it from the tracked example, whose DB creds match the
# compose-exposed Postgres on localhost:5432.
if [ ! -f "$PROJECT_ROOT/agent-manager-service/.env" ]; then
    echo "🌱 Seeding agent-manager-service/.env from .env.example..."
    cp "$PROJECT_ROOT/agent-manager-service/.env.example" "$PROJECT_ROOT/agent-manager-service/.env"
fi

echo "🗄️  Running database migrations..."
if ! (cd "$PROJECT_ROOT/agent-manager-service" && ENV_FILE_PATH=.env go run -mod=readonly . -migrate -server=false); then
    echo "❌ Database migrations failed. Aborting platform setup."
    exit 1
fi
echo "✅ Migrations completed"

echo "🚀 Starting Agent Manager platform services..."
# COMPOSE_SERVICES optionally restricts the bring-up to a subset of services
# (e.g. CI heavy tier skips the console). Unset = all services (local default).
# Split on whitespace into an array so multiple services work without exposing
# the value to glob expansion.
if [ -n "${COMPOSE_SERVICES:-}" ]; then
    read -r -a compose_services <<< "${COMPOSE_SERVICES}"
    docker compose -f "$COMPOSE_FILE" up -d "${compose_services[@]}"
else
    docker compose -f "$COMPOSE_FILE" up -d
fi

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
