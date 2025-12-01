#!/usr/bin/env bash
set -eo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/install-helpers.sh"

# Configuration
VERBOSE="${VERBOSE:-false}"
AUTO_PORT_FORWARD="${AUTO_PORT_FORWARD:-true}"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --no-port-forward)
            AUTO_PORT_FORWARD=false
            shift
            ;;
        --config)
            if [[ -f "$2" ]]; then
                AMP_HELM_ARGS+=("-f" "$2")
            else
                log_error "Config file not found: $2"
                exit 1
            fi
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Install Agent Management Platform with observability"
            echo ""
            echo "Options:"
            echo "  --verbose, -v           Show detailed installation output"
            echo "  --no-port-forward       Skip automatic port forwarding"
            echo "  --config FILE           Use custom configuration file"
            echo "  --help, -h              Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                      # Simple installation"
            echo "  $0 --verbose            # Installation with detailed output"
            echo "  $0 --config custom.yaml # Installation with custom config"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Print simple header
if [[ "$VERBOSE" == "false" ]]; then
    echo ""
    echo "🚀 Installing Agent Management Platform..."
    echo ""
else
    log_info "Starting Agent Management Platform installation..."
    log_info "Configuration:"
    log_info "  Kubernetes context: $(kubectl config current-context)"
    log_info "  Platform namespace: $AMP_NS"
    log_info "  Observability namespace: $OBSERVABILITY_NS"
    echo ""
fi

# Step 1: Verify prerequisites
if [[ "$VERBOSE" == "false" ]]; then
    echo "✓ Validating prerequisites..."
else
    log_info "Step 1: Validating prerequisites..."
fi

if ! verify_prerequisites_silent; then
    log_error "Prerequisites check failed. Run with --verbose for details."
    exit 1
fi

# Step 2: Install Core Platform
if [[ "$VERBOSE" == "false" ]]; then
    echo "✓ Installing platform components..."
else
    log_info "Step 2: Installing Agent Management Platform..."
fi

if ! install_agent_management_platform_silent; then
    log_error "Platform installation failed. Run with --verbose for details."
    exit 1
fi

# Step 3: Install Observability (always enabled)
if [[ "$VERBOSE" == "false" ]]; then
    echo "✓ Installing observability stack..."
else
    log_info "Step 3: Installing Observability Stack..."
fi

if ! install_observability_dataprepper_silent; then
    log_error "Observability installation failed. Run with --verbose for details."
    exit 1
fi

# Step 4: Start services
if [[ "$VERBOSE" == "false" ]]; then
    echo "✓ Starting services..."
else
    log_info "Step 4: Starting port forwarding..."
fi

# Start port forwarding
if [[ "${AUTO_PORT_FORWARD}" == "true" ]]; then
    PORT_FORWARD_SCRIPT="${SCRIPT_DIR}/port-forward.sh"
    if [[ -f "$PORT_FORWARD_SCRIPT" ]]; then
        if [[ "$VERBOSE" == "true" ]]; then
            log_info "Starting port forwarding in background..."
        fi
        
        # Run port-forward script in background
        bash "$PORT_FORWARD_SCRIPT" > /dev/null 2>&1 &
        PORT_FORWARD_PID=$!
        
        # Save PID to file for easy cleanup
        echo "$PORT_FORWARD_PID" > "${SCRIPT_DIR}/.port-forward.pid"
        
        # Give port forwarding a moment to start
        sleep 2
    else
        if [[ "$VERBOSE" == "true" ]]; then
            log_warning "Port forward script not found at: $PORT_FORWARD_SCRIPT"
        fi
    fi
fi

# Print success message
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Installation Complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "🌐 Access your platform:"
echo ""
echo "   Console:         http://localhost:3000"
echo "   API:             http://localhost:8080"
echo "   Traces Observer: http://localhost:9098"
echo "   Data Prepper:    http://localhost:21893"
echo ""
echo "🚀 Next steps:"
echo ""
echo "   1. Open console: open http://localhost:3000"
echo "   2. Deploy sample agent: cd ../runtime/sample-agents/python-agent"
echo "   3. View traces in the console"
echo ""
echo "📚 Documentation: https://github.com/wso2/agent-management-platform"
echo ""
if [[ "${AUTO_PORT_FORWARD}" == "true" ]]; then
    echo "💡 To stop port forwarding: ./stop-port-forward.sh"
    echo ""
fi

if [[ "$VERBOSE" == "true" ]]; then
    echo ""
    log_info "Installation Details:"
    log_info "  Cluster: $(kubectl config current-context)"
    log_info "  Platform Namespace: $AMP_NS"
    log_info "  Observability Namespace: $OBSERVABILITY_NS"
    echo ""
    log_info "Deployed Components:"
    kubectl get pods -n "$AMP_NS" 2>/dev/null || true
    echo ""
    kubectl get pods -n "$OBSERVABILITY_NS" 2>/dev/null || true
fi

