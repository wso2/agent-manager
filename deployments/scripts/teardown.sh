#!/bin/bash
set -e

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/env.sh"
source "$SCRIPT_DIR/utils.sh"

echo "=== Tearing Down Agent Manager Development Environment ==="

# ============================================================================
# Step 1: Stop Docker Compose services
# ============================================================================
echo ""
echo "1️⃣  Stop Docker Compose services"
if [ -f "$SCRIPT_DIR/../docker-compose.yml" ]; then
    echo "🛑 Stopping Agent Manager platform services..."
    docker compose -f "$SCRIPT_DIR/../docker-compose.yml" down -v
    echo "✅ Platform services stopped"
else
    echo "⚠️  docker-compose.yml not found, skipping platform teardown"
fi

# ============================================================================
# Step 2: Delete K3d cluster
# ============================================================================
echo ""
echo "2️⃣  Delete K3d cluster"
if command -v k3d &> /dev/null; then
    if k3d cluster list 2>/dev/null | grep -q "$CLUSTER_NAME"; then
        echo "🛑 Deleting K3d cluster '$CLUSTER_NAME'..."
        k3d cluster delete "$CLUSTER_NAME"
        echo "✅ K3d cluster deleted"
    else
        echo "⚠️  K3d cluster '$CLUSTER_NAME' not found"
    fi
else
    echo "⚠️  K3d not installed, skipping cluster deletion"
fi

# ============================================================================
# Step 3: Delete Colima dev profile
# ============================================================================
echo ""
echo "3️⃣  Delete Colima dev profile"
if command -v colima &> /dev/null; then
    if colima list 2>/dev/null | grep -q "dev"; then
        echo "🛑 Deleting Colima dev profile..."
        colima stop dev 2>/dev/null || true
        colima delete dev --force 2>/dev/null || true
        echo "✅ Colima dev profile deleted"
    else
        echo "⏭️  Colima dev profile not found, skipping"
    fi
else
    echo "⏭️  Colima not installed, skipping"
fi

echo ""
echo "ℹ️  Note: Colima default profile may still be running."
echo "   To completely remove Colima: colima delete"
echo ""
echo "✅ Teardown complete!"
