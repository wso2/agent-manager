#!/usr/bin/env bash
set -eo pipefail

# Get the absolute path of the script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper functions
source "${SCRIPT_DIR}/install-helpers.sh"

# Configuration
FORCE="${FORCE:-false}"
DELETE_NAMESPACES="${DELETE_NAMESPACES:-false}"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --force|-f)
            FORCE=true
            shift
            ;;
        --delete-namespaces)
            DELETE_NAMESPACES=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Uninstall Agent Management Platform"
            echo ""
            echo "Options:"
            echo "  --force, -f            Skip confirmation prompt"
            echo "  --delete-namespaces    Delete namespaces after uninstalling"
            echo "  --help, -h             Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                     # Interactive uninstall"
            echo "  $0 --force             # Uninstall without confirmation"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Confirmation prompt
if [[ "$FORCE" != "true" ]]; then
    echo ""
    echo "⚠️  This will uninstall Agent Management Platform"
    echo ""
    echo "The following will be removed:"
    echo "  - Agent Management Platform (namespace: $AMP_NS)"
    echo "  - Observability components (namespace: $OBSERVABILITY_NS)"
    if [[ "$DELETE_NAMESPACES" == "true" ]]; then
        echo "  - Namespaces will be deleted"
    fi
    echo ""
    read -p "Are you sure you want to continue? (yes/no): " -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]; then
        log_info "Uninstall cancelled"
        exit 0
    fi
fi

log_info "Starting uninstallation..."
echo ""

# Stop port forwarding if running
log_info "Stopping port forwarding..."
if [[ -f "${SCRIPT_DIR}/.port-forward.pid" ]]; then
    PORT_FORWARD_PID=$(cat "${SCRIPT_DIR}/.port-forward.pid")
    if kill "$PORT_FORWARD_PID" 2>/dev/null; then
        log_success "Port forwarding stopped (PID: $PORT_FORWARD_PID)"
    fi
    rm -f "${SCRIPT_DIR}/.port-forward.pid"
fi

# Kill all kubectl port-forward processes
pkill -f "kubectl port-forward" 2>/dev/null || true
log_success "All port forwarding processes stopped"
echo ""

# Uninstall Observability
log_info "Uninstalling Observability components..."
if helm_release_exists "amp-observability-traces" "$OBSERVABILITY_NS"; then
    helm uninstall amp-observability-traces -n "$OBSERVABILITY_NS" 2>/dev/null || true
    log_success "Observability components uninstalled"
else
    log_info "Observability components not found, skipping"
fi
echo ""

# Uninstall Agent Management Platform
log_info "Uninstalling Agent Management Platform..."
# Check for both release names (silent version uses "amp", non-silent uses "agent-management-platform")
if helm_release_exists "amp" "$AMP_NS"; then
    helm uninstall amp -n "$AMP_NS" 2>/dev/null || true
    log_success "Agent Management Platform uninstalled (release: amp)"
elif helm_release_exists "agent-management-platform" "$AMP_NS"; then
    helm uninstall agent-management-platform -n "$AMP_NS" 2>/dev/null || true
    log_success "Agent Management Platform uninstalled (release: agent-management-platform)"
else
    log_info "Agent Management Platform not found, skipping"
fi
echo ""

# Delete namespaces if requested
if [[ "$DELETE_NAMESPACES" == "true" ]]; then
    log_info "Deleting namespaces..."
    
    if namespace_exists "$AMP_NS"; then
        kubectl delete namespace "$AMP_NS" --timeout=60s 2>/dev/null || true
        log_success "Namespace $AMP_NS deleted"
    fi
    
    # Note: We don't delete observability namespace as it may be shared with OpenChoreo
    log_warning "Observability namespace ($OBSERVABILITY_NS) not deleted (shared with OpenChoreo)"
    echo ""
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Uninstallation Complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
log_success "Agent Management Platform has been uninstalled"
echo ""

if [[ "$DELETE_NAMESPACES" != "true" ]]; then
    log_info "To completely remove all resources including namespaces, run:"
    log_info "  $0 --force --delete-namespaces"
    echo ""
fi

