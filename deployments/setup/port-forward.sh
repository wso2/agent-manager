#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/env.sh"

# Parse flags
PLATFORM=false
BACKGROUND=false

for arg in "$@"; do
    case "$arg" in
        --platform)   PLATFORM=true ;;
        --background) BACKGROUND=true ;;
    esac
done

# Default: all services if no group specified
if ! $PLATFORM; then
    PLATFORM=true
fi

# Check prerequisites
if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl is not installed"
    exit 1
fi

if ! kubectl cluster-info --context "$CLUSTER_CONTEXT" &> /dev/null; then
    echo "❌ k3d cluster '$CLUSTER_NAME' is not running"
    exit 1
fi

kubectl config use-context "$CLUSTER_CONTEXT"

# Stop any existing port-forwards before starting fresh
bash "$SCRIPT_DIR/stop-port-forward.sh" 2>/dev/null || true

echo ""
echo "=== Starting Port Forwarding ==="

# In interactive mode, kill background jobs on exit
cleanup() {
    echo ""
    echo "🛑 Stopping all port forwarding..."
    jobs -p | xargs kill 2>/dev/null || true
    exit 0
}
if ! $BACKGROUND; then
    trap cleanup EXIT INT TERM
fi

# PORT_FORWARD_ADDRESS optionally binds the forwards to a specific address.
# Unset = kubectl default (127.0.0.1). CI sets 0.0.0.0 so the agent-manager
# container (docker-compose) can reach them via the host/bridge gateway —
# a 127.0.0.1-only forward is unreachable from inside the container on Linux.
#
# So on Linux, default to loopback *plus* the Docker bridge gateway: containers
# reach the host through the address their `host-gateway` alias resolves to, and
# binding it makes the forwards reachable from docker-compose without publishing
# anything to the LAN, while loopback keeps http://localhost:<port> working for
# the developer. macOS needs none of this — Docker Desktop already routes
# host.docker.internal traffic to the host's loopback.
PF_ADDRESS="${PORT_FORWARD_ADDRESS:-}"
if [ -z "$PF_ADDRESS" ] && [ "$(uname -s)" = "Linux" ]; then
    bridge_gateway=$(docker network inspect bridge \
        --format '{{ (index .IPAM.Config 0).Gateway }}' 2>/dev/null)
    if [ -n "$bridge_gateway" ]; then
        PF_ADDRESS="127.0.0.1,$bridge_gateway"
    else
        echo "⚠️  Could not determine the Docker bridge gateway address."
        echo "   Forwards will bind to localhost only, so containers cannot reach them."
        echo "   Set PORT_FORWARD_ADDRESS explicitly to override."
    fi
fi

start_forward() {
    local desc="$1"; shift
    echo "   $desc"
    if [ -n "$PF_ADDRESS" ]; then
        kubectl port-forward --address "$PF_ADDRESS" "$@" > /dev/null 2>&1 &
    else
        kubectl port-forward "$@" > /dev/null 2>&1 &
    fi
}

if $PLATFORM; then
    echo "🔧 Platform services:"
    start_forward "Thunder IDP (8090)"               -n amp-thunder svc/amp-thunder-extension-service 8090:8090
    start_forward "OpenSearch (9200)"                -n "$OBSERVABILITY_NAMESPACE" svc/opensearch 9200:9200
    start_forward "OpenTelemetry Collector (21893)"  -n "$OBSERVABILITY_NAMESPACE" svc/opentelemetry-collector 21893:4318
    start_forward "Agent Manager Observer (9098)"    -n "$OBSERVABILITY_NAMESPACE" svc/amp-observer 9098:9098
fi

echo ""
echo "✅ Active endpoints:"
if $PLATFORM; then
    echo "   Thunder IDP:              http://localhost:8090"
    echo "   OpenSearch:               http://localhost:9200"
    echo "   Agent Manager Observer:   http://localhost:9098"
fi

echo ""
if $BACKGROUND; then
    echo "💡 Running in background — use 'make stop-port-forward' to stop"
else
    echo "💡 Keep this terminal open. Press Ctrl+C to stop."
    wait
fi
