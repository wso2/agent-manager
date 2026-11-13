#!/bin/bash
set -e

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/utils.sh"

# ============================================================================
# Linux: skip the VM entirely
# ============================================================================
# Colima exists to give macOS a Linux Docker daemon. On Linux that daemon is
# already native, and interposing a VM would strand docker compose in a daemon
# separate from the host's — bind mounts and published ports stop lining up.
# So on Linux we only validate the native daemon.
if [ "$(uname -s)" = "Linux" ]; then
    echo "=== Checking prerequisites ==="
    check_command docker

    echo ""
    echo "=== Linux detected — using the native Docker daemon (no Colima VM) ==="

    # Checked before reachability: a stale Colima context makes `docker info`
    # fail too, and the honest diagnosis is the wrong context rather than a
    # stopped daemon. Such a context silently points every later docker call
    # at a VM daemon while the compose state lives on the native one.
    DOCKER_CONTEXT_NAME="$(docker context show)"
    case "$DOCKER_CONTEXT_NAME" in
        colima*)
            echo "❌ Active Docker context is '$DOCKER_CONTEXT_NAME' (a Colima VM)."
            echo "   On Linux this hides the native daemon from docker compose."
            echo "   Switch back with:  docker context use default"
            exit 1
            ;;
    esac

    if ! docker info > /dev/null 2>&1; then
        echo "❌ Cannot reach the Docker daemon."
        echo "   Start it with:  sudo systemctl start docker"
        echo "   If this user has never used Docker:"
        echo "     sudo usermod -aG docker \$USER   (then log out and back in)"
        exit 1
    fi

    echo "✅ Docker is running (context: $DOCKER_CONTEXT_NAME)"
    echo ""
    echo "✅ Setup complete! Start AMP with: make dev-up"
    exit 0
fi

# ============================================================================
# Configuration (macOS)
# ============================================================================
# Sized for the Compose stack: a cold `make up` peaks below 3 GB in the VM,
# and the running stack uses about 1.6 GB.
PROFILE="${1:-amp-lite}"
COLIMA_CPU="${COLIMA_CPU:-2}"
COLIMA_MEMORY="${COLIMA_MEMORY:-4}"
COLIMA_VM_TYPE="${COLIMA_VM_TYPE:-vz}"

# Check prerequisites
echo "=== Checking prerequisites ==="
check_command colima
check_command docker

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
echo "🚀 Starting Colima with Agent Manager-compatible settings..."
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
echo "✅ Setup complete! Start AMP with: make dev-up"
echo ""
echo "💡 Useful commands:"
echo "   colima status --profile $PROFILE"
echo "   colima stop --profile $PROFILE"
