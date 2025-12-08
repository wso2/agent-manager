#!/usr/bin/env bash
set -eo pipefail

# ============================================================================
# Agent Management Platform - Complete Bootstrap Installation
# ============================================================================
# This script provides a single-command installation that:
# 1. Creates a Kind cluster
# 2. Installs OpenChoreo
# 3. Installs Agent Management Platform
#
# Usage:
#   ./install.sh              # Full installation
#   ./install.sh --minimal    # Skip optional OpenChoreo components
#   ./install.sh --verbose    # Show detailed output
#   ./install.sh --help       # Show help
# ============================================================================

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/install-helpers.sh"

# Configuration
VERBOSE="${VERBOSE:-false}"
SKIP_KIND="${SKIP_KIND:-false}"
SKIP_OPENCHOREO="${SKIP_OPENCHOREO:-false}"
MINIMAL_MODE="${MINIMAL_MODE:-false}"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --minimal|--core-only)
            MINIMAL_MODE=true
            shift
            ;;
        --skip-kind)
            SKIP_KIND=true
            shift
            ;;
        --skip-openchoreo)
            SKIP_OPENCHOREO=true
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
            cat << EOF

🚀 Agent Management Platform - Bootstrap Installation

This script provides a complete one-command installation of:
  • Kind cluster (Kubernetes in Docker)
  • OpenChoreo platform
  • Agent Management Platform with observability

Usage:
  $0 [OPTIONS]

Options:
  --verbose, -v           Show detailed installation output
  --minimal, --core-only  Install only core OpenChoreo components (faster)
  --skip-kind             Skip Kind cluster creation (use existing cluster)
  --skip-openchoreo       Skip OpenChoreo installation (install platform only)
  --config FILE           Use custom configuration file for platform
  --help, -h              Show this help message

Examples:
  $0                      # Full installation (recommended)
  $0 --verbose            # Full installation with detailed output
  $0 --minimal            # Faster installation with core components only
  $0 --skip-kind          # Use existing Kind cluster
  $0 --config custom.yaml # Installation with custom platform config

After installation:
  Run ./port-forward.sh to access services from localhost

Prerequisites:
  • Docker (Docker Desktop or Colima)
  • kubectl
  • helm
  • kind

Installation Time:
  • Full installation: ~15-20 minutes
  • Minimal installation: ~10-12 minutes

For more information:
  • Quick Start Guide: https://github.com/wso2/ai-agent-management-platform/blob/main/docs/quick-start.md
  • Troubleshooting: See README.md for troubleshooting section
  • Documentation: https://github.com/wso2/agent-management-platform

EOF
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# ============================================================================
# MAIN INSTALLATION FLOW
# ============================================================================

# Print header
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "🚀 Agent Management Platform - Bootstrap"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [[ "$MINIMAL_MODE" == "true" ]]; then
    echo "Mode: Minimal (core components only)"
else
    echo "Mode: Full installation"
fi

if [[ "$VERBOSE" == "true" ]]; then
    echo "Verbosity: Detailed output enabled"
fi

echo ""
echo "This will install:"
echo "  ✓ Kind cluster (local Kubernetes)"
echo "  ✓ OpenChoreo platform"
echo "  ✓ Agent Management Platform"
echo "  ✓ Observability stack"
echo ""

if [[ "$VERBOSE" == "false" ]]; then
    echo "💡 Tip: Use --verbose for detailed progress information"
    echo ""
fi

# Estimate installation time
if [[ "$MINIMAL_MODE" == "true" ]]; then
    echo "⏱️  Estimated time: 10-12 minutes"
else
    echo "⏱️  Estimated time: 15-20 minutes"
fi
echo ""

# ============================================================================
# STEP 1: VERIFY PREREQUISITES
# ============================================================================

if [[ "$VERBOSE" == "false" ]]; then
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Step 1/4: Verifying prerequisites..."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
else
    log_info "Step 1/4: Verifying prerequisites..."
    echo ""
fi

if ! verify_openchoreo_prerequisites; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_error "Prerequisites check failed"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Please install the missing prerequisites and try again."
    echo ""
    exit 1
fi

if [[ "$VERBOSE" == "false" ]]; then
    echo "✓ All prerequisites verified"
else
    log_success "Prerequisites check passed"
fi
echo ""

# ============================================================================
# STEP 2: SETUP KIND CLUSTER
# ============================================================================

if [[ "$SKIP_KIND" == "true" ]]; then
    if [[ "$VERBOSE" == "false" ]]; then
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "Step 2/4: Skipping Kind cluster setup (--skip-kind)"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
    else
        log_info "Step 2/4: Skipping Kind cluster setup (--skip-kind)"
        echo ""
    fi
else
    if [[ "$VERBOSE" == "false" ]]; then
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "Step 2/4: Setting up Kind cluster..."
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "⏱️  This may take 2-3 minutes..."
        echo ""
    else
        log_info "Step 2/4: Setting up Kind cluster..."
        echo ""
    fi
    
    if ! setup_kind_cluster "openchoreo-local" "${SCRIPT_DIR}/kind-config.yaml"; then
        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        log_error "Failed to setup Kind cluster"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "The Kind cluster could not be created."
        echo ""
        echo "Common solutions:"
        echo "  1. Delete existing cluster: kind delete cluster --name openchoreo-local"
        echo "  2. Restart Docker"
        echo "  3. Check Docker has sufficient resources (4GB+ RAM recommended)"
        echo ""
        echo "For more help, see: ./README.md"
        echo ""
        exit 1
    fi
    
    if [[ "$VERBOSE" == "false" ]]; then
        echo "✓ Kind cluster ready"
    fi
    echo ""
fi

# ============================================================================
# STEP 3: INSTALL OPENCHOREO
# ============================================================================

if [[ "$SKIP_OPENCHOREO" == "true" ]]; then
    if [[ "$VERBOSE" == "false" ]]; then
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "Step 3/4: Skipping OpenChoreo installation (--skip-openchoreo)"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
    else
        log_info "Step 3/4: Skipping OpenChoreo installation (--skip-openchoreo)"
        echo ""
    fi
else
    if [[ "$VERBOSE" == "false" ]]; then
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo "Step 3/4: Installing OpenChoreo..."
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        if [[ "$MINIMAL_MODE" == "true" ]]; then
            echo "⏱️  This may take 10-12 minutes (core components only)..."
        else
            echo "⏱️  This may take 12-15 minutes (full installation)..."
        fi
        echo ""
        echo "Installing components:"
        echo "  • Cilium CNI"
        echo "  • OpenChoreo Control Plane"
        echo "  • OpenChoreo Data Plane"
        echo "  • OpenChoreo Observability Plane"
        echo ""
    else
        log_info "Step 3/4: Installing OpenChoreo..."
        echo ""
    fi
    
    # Install core components
    if ! install_openchoreo_core; then
        echo ""
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        log_error "Failed to install OpenChoreo"
        echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        echo "OpenChoreo installation failed."
        echo ""
        echo "Troubleshooting steps:"
        echo "  1. Check cluster status: kubectl get nodes"
        echo "  2. Check pod status: kubectl get pods --all-namespaces"
        echo "  3. View logs: kubectl logs -n <namespace> <pod-name>"
        echo "  4. Ensure Docker has sufficient resources (4GB+ RAM)"
        echo ""
        echo "To clean up and retry:"
        echo "  ./uninstall.sh"
        echo "  ./install.sh"
        echo ""
        echo "For more help, see: ./README.md"
        echo ""
        exit 1
    fi
    
    if [[ "$VERBOSE" == "false" ]]; then
        echo "✓ OpenChoreo installed successfully"
    fi
    echo ""
fi

# ============================================================================
# STEP 4: INSTALL AGENT MANAGEMENT PLATFORM
# ============================================================================

if [[ "$VERBOSE" == "false" ]]; then
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Step 4/4: Installing Agent Management Platform..."
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "⏱️  This may take 5-8 minutes..."
    echo ""
else
    log_info "Step 4/4: Installing Agent Management Platform..."
    echo ""
fi

# Verify OpenChoreo Observability Plane is available
if ! kubectl get namespace openchoreo-observability-plane >/dev/null 2>&1; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_error "OpenChoreo Observability Plane not found"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "The Agent Management Platform requires OpenChoreo Observability Plane."
    echo ""
    echo "This should have been installed in Step 3."
    echo "Please run the full bootstrap without --skip-openchoreo"
    echo ""
    exit 1
fi


if ! kubectl get namespace openchoreo-build-plane >/dev/null 2>&1; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_error "OpenChoreo Build Plane not found"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "The Agent Management Platform requires OpenChoreo Build Plane."
    echo ""
    echo "This should have been installed in Step 3."
    echo "Please run the full bootstrap"
    echo ""
    exit 1
fi

# Install platform components
if [[ "$VERBOSE" == "false" ]]; then
    echo "Installing components:"
    echo "  • PostgreSQL database"
    echo "  • Agent Manager Service"
    echo "  • Console (Web UI)"
    echo "  • Observability stack"
    echo ""
fi

if ! install_agent_management_platform_silent; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_error "Failed to install Agent Management Platform"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Platform installation failed."
    echo ""
    echo "Troubleshooting steps:"
    echo "  1. Check pod status: kubectl get pods -n agent-management-platform"
    echo "  2. View logs: kubectl logs -n agent-management-platform <pod-name>"
    echo "  3. Check Helm release: helm list -n agent-management-platform"
    echo ""
    echo "To retry platform installation only:"
    echo "  ./install.sh"
    echo ""
    echo "For more help, see: ./TROUBLESHOOTING.md"
    echo ""
    exit 1
fi

if [[ "$VERBOSE" == "false" ]]; then
    echo "✓ Platform components installed"
    echo ""
fi

# Install observability
if [[ "$VERBOSE" == "false" ]]; then
    echo "Installing observability stack..."
    echo ""
fi

if ! install_observability_dataprepper_silent; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_error "Failed to install observability stack"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Observability installation failed."
    echo ""
    echo "The platform is installed but observability features may not work."
    echo ""
    echo "Troubleshooting steps:"
    echo "  1. Check pod status: kubectl get pods -n openchoreo-observability-plane"
    echo "  2. View logs: kubectl logs -n openchoreo-observability-plane <pod-name>"
    echo ""
    exit 1
fi

if [[ "$VERBOSE" == "false" ]]; then
    echo "✓ Observability stack ready"
    echo ""
fi

if ! install_build_ci; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_error "Failed to install Build CI"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Build CI installation failed."
    echo ""
    exit 1
fi

if [[ "$VERBOSE" == "false" ]]; then
    echo "✓ Build CI ready"
    echo ""
fi

# ============================================================================
# SUCCESS MESSAGE
# ============================================================================

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Installation Complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "🚀 Next steps:"
echo ""
echo "   1. Start port forwarding:"
echo "      ./port-forward.sh"
echo ""
echo "   2. Access your platform:"
echo "      Console:         http://localhost:3000"
echo "      API:             http://localhost:8080"
echo "      Traces Observer: http://localhost:9098"
echo "      Data Prepper:    http://localhost:21893"
echo ""
echo "   3. Deploy an agent:"
echo "      cd ../runtime/sample-agents/python-agent"
echo ""
echo "   4. View traces in the console"
echo ""
echo "💡 Port forwarding must be running to access services from localhost"
echo "   To stop: Press Ctrl+C in the port-forward.sh terminal"
echo ""
echo "🛑 To uninstall everything:"
echo "   ./uninstall.sh"
echo ""

if [[ "$VERBOSE" == "true" ]]; then
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_info "Installation Summary"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    log_info "Cluster: $(kubectl config current-context)"
    log_info "Platform Namespace: $AMP_NS"
    log_info "Observability Namespace: $OBSERVABILITY_NS"
    echo ""
    log_info "Deployed Components:"
    echo ""
    kubectl get pods -n "$AMP_NS" 2>/dev/null || true
    echo ""
    kubectl get pods -n "$OBSERVABILITY_NS" 2>/dev/null || true
    echo ""
fi