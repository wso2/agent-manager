#!/bin/bash
set -e

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/env.sh"
source "$SCRIPT_DIR/utils.sh"

# ============================================================================
# Configuration
# ============================================================================
PROFILE="${1:-dev}"
COLIMA_CPU=4
COLIMA_MEMORY=8
COLIMA_VM_TYPE="vz"

# Check prerequisites
echo "=== Checking prerequisites ==="
check_command colima
check_command docker
check_command k3d
check_command kubectl
check_command helm

echo ""
echo "=== Setting up Colima for Agent Manager Platform ==="
echo "Profile: $PROFILE"

# ============================================================================
# Step 1: Check Colima status
# ============================================================================
echo ""
echo "1️⃣  Check Colima status"
if colima status --profile "$PROFILE" &> /dev/null; then
    echo "✅ Colima is already running on profile '$PROFILE'"
    colima status --profile "$PROFILE"
    echo ""
    echo "⚠️  If you need to adjust resources, stop Colima first:"
    echo "   colima stop --profile $PROFILE"
    echo "   Then re-run this script"
    exit 0
fi

# ============================================================================
# Step 2: Start Colima
# ============================================================================
echo ""
echo "2️⃣  Start Colima"
echo "🚀 Starting Colima with OpenChoreo-compatible settings..."
echo "   Profile:  $PROFILE"
echo "   VM Type:  $COLIMA_VM_TYPE (Virtualization.framework) - required for stability"
echo "   Rosetta:  enabled (for x86_64 compatibility) - required"
echo "   CPU:      $COLIMA_CPU cores"
echo "   Memory:   $COLIMA_MEMORY GB"
echo ""

colima start --profile "$PROFILE" \
    --vm-type="$COLIMA_VM_TYPE" \
    --vz-rosetta \
    --network-address \
    --cpu "$COLIMA_CPU" \
    --memory "$COLIMA_MEMORY"

echo ""
echo "✅ Colima started successfully!"

# ============================================================================
# Step 3: Verify setup
# ============================================================================
echo ""
echo "3️⃣  Verify setup"

# Verify Docker is accessible
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not accessible. Colima may not have started correctly."
    exit 1
fi
echo "✅ Docker is running"

echo ""
echo "📊 Colima Status:"
colima status --profile "$PROFILE"

echo ""
echo "🐳 Docker Context:"
docker context show

echo ""
echo "✅ Setup complete! You can now proceed with k3d cluster setup."
echo ""
echo "💡 Useful commands:"
echo "   colima status --profile $PROFILE"
echo "   colima stop --profile $PROFILE"
