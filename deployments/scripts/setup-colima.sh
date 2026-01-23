#!/bin/bash
set -e

# Accept profile name as argument (default: dev)
PROFILE="${1:-dev}"

echo "=== Setting up Colima for Agent Manager Platform ==="
echo "Profile: $PROFILE"
echo ""

# Check if Colima is installed
if ! command -v colima &> /dev/null; then
    echo "❌ Colima is not installed. Please install it first:"
    echo "   brew install colima"
    exit 1
fi

# Check if Colima is already running on this profile
if colima status --profile "$PROFILE" &> /dev/null; then
    echo "✅ Colima is already running on profile '$PROFILE'"
    colima status --profile "$PROFILE"
    echo ""
    echo "⚠️  If you need to adjust resources, stop Colima first:"
    echo "   colima stop --profile $PROFILE"
    echo "   Then re-run this script"
    exit 0
fi

# Start Colima with OpenChoreo-compatible configuration
echo "🚀 Starting Colima with OpenChoreo-compatible settings..."
echo "   Profile: $PROFILE"
echo "   VM Type: vz (Virtualization.framework) - required for stability"
echo "   Rosetta: enabled (for x86_64 compatibility) - required"
echo "   CPU: 4 cores"
echo "   Memory: 8 GB"

colima start --profile "$PROFILE" --vm-type=vz --vz-rosetta --cpu 4 --memory 8

echo ""
echo "✅ Colima started successfully!"
echo ""
echo "📊 Colima Status:"
colima status --profile "$PROFILE"

echo ""
echo "🐳 Docker Context:"
docker context show

echo ""
echo "✅ Setup complete! You can now proceed with Kind cluster setup."
echo ""
echo "💡 To use this profile:"
echo "   colima status --profile $PROFILE"
echo "   colima stop --profile $PROFILE"
