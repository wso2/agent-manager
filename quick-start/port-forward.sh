#!/usr/bin/env bash

# Port forwarding script for Agent Management Platform services
# Uses socat to forward ports from container to Kind worker node NodePorts
# Modeled after OpenChoreo's approach for reliability

set -eo pipefail

# Default namespaces (can be overridden via environment variables)
AMP_NS="${AMP_NS:-wso2-amp}"
OBSERVABILITY_NS="${OBSERVABILITY_NS:-openchoreo-observability-plane}"
DATA_PLANE_NS="${DATA_PLANE_NS:-openchoreo-data-plane}"
CLUSTER_NAME="${CLUSTER_NAME:-openchoreo-local}"
WORKER_NODE="${WORKER_NODE:-${CLUSTER_NAME}-worker}"

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${RESET} $1" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${RESET} $1" >&2
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${RESET} $1" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${RESET} $1" >&2
}

# Check if socat is installed
check_socat() {
    if ! command -v socat >/dev/null 2>&1; then
        log_error "socat is not installed"
        echo "" >&2
        echo "Please install socat:" >&2
        echo "  • macOS: brew install socat" >&2
        echo "  • Ubuntu/Debian: apt-get install socat" >&2
        echo "  • Alpine: apk add socat" >&2
        echo "" >&2
        return 1
    fi
    return 0
}

# Check if service backend is healthy and responding
verify_backend_health() {
    local worker_node="$1"
    local node_port="$2"
    local description="$3"
    local timeout="${4:-60}"

    log_info "Verifying $description backend is ready..."

    local elapsed=0
    while [ $elapsed -lt $timeout ]; do
        # Try to connect to the backend service
        if timeout 5 bash -c "echo > /dev/tcp/$worker_node/$node_port" 2>/dev/null; then
            log_success "$description backend is responding"
            return 0
        fi

        if [ $((elapsed % 10)) -eq 0 ] && [ $elapsed -gt 0 ]; then
            log_info "Waiting for $description to be ready... ($elapsed/$timeout seconds)"
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done

    log_warning "$description backend not responding after ${timeout}s (may still be starting)"
    return 1
}

# Get NodePort for a service
get_nodeport() {
    local svc_name="$1"
    local namespace="$2"
    local port_name="${3:-}"
    local timeout=30
    local elapsed=0

    log_info "Finding NodePort for $svc_name in $namespace..."

    while [ $elapsed -lt $timeout ]; do
        local nodeport
        if [ -n "$port_name" ]; then
            # Get NodePort by port name
            nodeport=$(kubectl get svc "$svc_name" -n "$namespace" \
                -o jsonpath="{.spec.ports[?(@.name=='$port_name')].nodePort}" 2>/dev/null) || true
        else
            # Get first NodePort
            nodeport=$(kubectl get svc "$svc_name" -n "$namespace" \
                -o jsonpath='{.spec.ports[0].nodePort}' 2>/dev/null) || true
        fi

        if [[ -n "$nodeport" && "$nodeport" != "null" ]]; then
            echo "$nodeport"
            return 0
        fi

        log_info "Waiting for service $svc_name... (attempt $((elapsed / 2 + 1))/15)"
        sleep 2
        elapsed=$((elapsed + 2))
    done

    log_error "Could not retrieve NodePort for $svc_name"
    return 1
}

# Setup port forwarding using socat
setup_port_forward() {
    local local_port="$1"
    local nodeport="$2"
    local description="$3"
    local verify_health="${4:-true}"

    # Verify backend health before setting up port forward
    if [[ "$verify_health" == "true" ]]; then
        if ! verify_backend_health "$WORKER_NODE" "$nodeport" "$description" 60; then
            log_warning "Proceeding with port forwarding despite health check warning"
        fi
    fi

    log_info "Setting up port-forward proxy from $local_port to $WORKER_NODE:$nodeport ($description)..."

    if socat TCP-LISTEN:$local_port,fork,reuseaddr,bind=0.0.0.0 TCP:$WORKER_NODE:$nodeport &
    then
        local pid=$!
        sleep 1
        if kill -0 $pid 2>/dev/null; then
            log_success "$description forwarding active (PID: $pid)"
            return 0
        else
            log_error "$description forwarding failed to start"
            return 1
        fi
    else
        log_error "Failed to start socat for $description"
        return 1
    fi
}

# Kill existing socat processes
cleanup_existing() {
    log_info "Cleaning up existing socat processes..."
    pkill socat 2>/dev/null || true
    sleep 1
}

# Main execution
main() {
    echo "" >&2
    log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_info "Agent Management Platform - Port Forwarding"
    log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "" >&2
    log_info "Using socat-based port forwarding (OpenChoreo approach)"
    echo "" >&2
    log_info "Configuration:"
    log_info "  Cluster: $CLUSTER_NAME"
    log_info "  Worker Node: $WORKER_NODE"
    log_info "  AMP Namespace: $AMP_NS"
    log_info "  Observability Namespace: $OBSERVABILITY_NS"
    log_info "  Data Plane Namespace: $DATA_PLANE_NS"
    echo "" >&2

    # Verify prerequisites
    if ! check_socat; then
        exit 1
    fi

    # Cleanup existing socat processes
    cleanup_existing

    # Port forward Console (3000)
    log_info "Setting up Console port forwarding (3000)..."
    if nodeport_console=$(get_nodeport "amp-console" "$AMP_NS"); then
        setup_port_forward 3000 "$nodeport_console" "Console"
    else
        log_warning "Skipping Console (service not ready or not NodePort type)"
    fi
    echo "" >&2

    # Port forward Agent Manager Service (8080)
    log_info "Setting up Agent Manager Service port forwarding (8080)..."
    if nodeport_agent_mgr=$(get_nodeport "amp-api" "$AMP_NS"); then
        setup_port_forward 8080 "$nodeport_agent_mgr" "Agent Manager Service"
    else
        log_warning "Skipping Agent Manager Service (service not ready or not NodePort type)"
    fi
    echo "" >&2

    # Port forward Traces Observer Service (9098)
    log_info "Setting up Traces Observer Service port forwarding (9098)..."
    if nodeport_traces=$(get_nodeport "amp-traces-observer" "$OBSERVABILITY_NS"); then
        setup_port_forward 9098 "$nodeport_traces" "Traces Observer Service"
    else
        log_warning "Skipping Traces Observer Service (service not found or not NodePort type)"
    fi
    echo "" >&2

    # Port forward OTel collector(21893)
    log_info "Setting up OTel collector port forwarding (21893)..."
    if nodeport_otel_collector=$(get_nodeport "opentelemetry-collector" "$OBSERVABILITY_NS"); then
        setup_port_forward 21893 "$nodeport_otel_collector" "OTel Collector"
    else
        log_warning "Skipping OTel Collector (service not found or not NodePort type)"
    fi
    echo "" >&2

    # Port forward External Gateway (8443)
    log_info "Setting up External Gateway port forwarding (8443)..."
    # Gateway uses LoadBalancer which creates NodePort in Kind
    if nodeport_gateway=$(get_nodeport "gateway-external" "$DATA_PLANE_NS"); then
        setup_port_forward 8443 "$nodeport_gateway" "External Gateway"
    else
        log_warning "Skipping External Gateway (service not found)"
    fi
    echo "" >&2

    log_success "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_success "Port forwarding setup complete!"
    log_success "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "" >&2
    log_info "Services accessible at:"
    echo "  • Console:           http://localhost:3000" >&2
    echo "  • Agent Manager:     http://localhost:8080" >&2
    echo "  • Traces Observer:   http://localhost:9098" >&2
    echo "  • OTel Collector:      http://localhost:21893" >&2
    echo "  • External Gateway:  https://localhost:8443" >&2
    echo "" >&2
    log_info "Port forwarding is active. Press Ctrl+C to stop."
    echo "" >&2

    # Keep the script running and wait for interrupt
    wait
}

# Trap to cleanup on interrupt
trap cleanup_existing INT TERM

# Run main
main