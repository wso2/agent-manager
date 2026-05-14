#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/env.sh"

# Parse flags
PLATFORM=false
GATEWAY=false
BACKGROUND=false

for arg in "$@"; do
    case "$arg" in
        --platform)   PLATFORM=true ;;
        --gateway)    GATEWAY=true ;;
        --background) BACKGROUND=true ;;
    esac
done

# Default: all services if no group specified
if ! $PLATFORM && ! $GATEWAY; then
    PLATFORM=true
    GATEWAY=true
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

start_forward() {
    local desc="$1"; shift
    echo "   $desc"
    kubectl port-forward "$@" > /dev/null 2>&1 &
}

if $PLATFORM; then
    echo "🔧 Platform services:"
    start_forward "Thunder IDP (8090)"               -n amp-thunder svc/amp-thunder-extension-service 8090:8090
    start_forward "OpenSearch (9200)"                -n openchoreo-observability-plane svc/opensearch 9200:9200
    start_forward "OpenTelemetry Collector (21893)"  -n openchoreo-observability-plane svc/opentelemetry-collector 21893:4318
    start_forward "Traces Observer (9098)"           -n openchoreo-observability-plane svc/amp-traces-observer 9098:9098
    start_forward "Observer API (8085)"              -n openchoreo-observability-plane svc/observer 8085:8080
    start_forward "OpenBao (8200)"                   -n openbao svc/openbao 8200:8200
    start_forward "OpenChoreo API (8195)"            -n openchoreo-control-plane svc/openchoreo-api 8195:8080
fi

if $GATEWAY; then
    echo "🌐 Gateway:"
    GW_SVC="api-platform-default-default-gateway-gateway-runtime"
    if kubectl get svc "$GW_SVC" -n openchoreo-data-plane &>/dev/null; then
        start_forward "API Platform Gateway HTTP (22893)"  -n openchoreo-data-plane "svc/$GW_SVC" 22893:22893
        start_forward "API Platform Gateway HTTPS (22894)" -n openchoreo-data-plane "svc/$GW_SVC" 22894:22894
    else
        echo "   ⚠️  Gateway not deployed yet — skipping (22893/22894)"
    fi
fi

echo ""
echo "✅ Active endpoints:"
if $PLATFORM; then
    echo "   Thunder IDP:              http://localhost:8090"
    echo "   Observer API:             http://localhost:8085"
    echo "   OpenSearch:               http://localhost:9200"
    echo "   Traces Observer:          http://localhost:9098"
    echo "   OpenBao:     http://localhost:8200"
fi
if $GATEWAY; then
    echo "   API Platform Gateway:     http://localhost:22893"
    echo "   API Platform Gateway TLS: https://localhost:22894"
fi

echo ""
if $BACKGROUND; then
    echo "💡 Running in background — use 'make stop-port-forward' to stop"
else
    echo "💡 Keep this terminal open. Press Ctrl+C to stop."
    wait
fi
