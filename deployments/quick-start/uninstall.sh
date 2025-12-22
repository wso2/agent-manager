#!/bin/bash
set -euo pipefail

# ============================================================================
# Agent Management Platform - Complete Uninstallation
# ============================================================================
# This script completely removes:
# 1. All Agent Management Platform helm releases
# 2. All OpenChoreo helm releases (unless --amp-only is used)
# 3. All custom resources (DataPlane, BuildPlane, etc.)
# 4. Optionally deletes the k3d cluster
#
# Usage:
#   ./uninstall.sh                    # Uninstall platform but keep cluster
#   ./uninstall.sh --delete-cluster    # Uninstall platform and delete cluster
#   ./uninstall.sh --amp-only          # Uninstall only AMP, keep OpenChoreo
#   ./uninstall.sh --amp-only --delete-cluster  # Uninstall AMP and delete cluster
# ============================================================================

# Configuration
CLUSTER_NAME="amp-local"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Namespace definitions (match install-helpers.sh)
AMP_NS="${AMP_NS:-wso2-amp}"
BUILD_CI_NS="${BUILD_CI_NS:-openchoreo-build-plane}"
OBSERVABILITY_NS="${OBSERVABILITY_NS:-openchoreo-observability-plane}"
DEFAULT_NS="${DEFAULT_NS:-default}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

log_success() {
    echo -e "${GREEN}✓${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

log_error() {
    echo -e "${RED}✗${NC} $1"
}

log_step() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Parse command line arguments
DELETE_CLUSTER=false
AMP_ONLY=false

for arg in "$@"; do
    case "$arg" in
        --delete-cluster|-d)
            DELETE_CLUSTER=true
            ;;
        --amp-only|--platform-only)
            AMP_ONLY=true
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --amp-only, --platform-only    Uninstall only AMP resources, keep OpenChoreo"
            echo "  --delete-cluster, -d           Also delete the k3d cluster"
            echo "  --help, -h                     Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                            # Uninstall everything but keep cluster"
            echo "  $0 --amp-only                 # Uninstall only AMP, keep OpenChoreo"
            echo "  $0 --delete-cluster           # Uninstall everything and delete cluster"
            exit 0
            ;;
        *)
            log_error "Unknown option: $arg"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# ============================================================================
# MAIN UNINSTALLATION FLOW
# ============================================================================

if [ "${AMP_ONLY}" = true ]; then
    log_step "Agent Management Platform Uninstallation (AMP Only)"
else
    log_step "Agent Management Platform Uninstallation"
fi

# Check prerequisites
if ! command_exists kubectl; then
    log_error "kubectl is not installed"
    exit 1
fi

if ! command_exists helm; then
    log_error "helm is not installed"
    exit 1
fi

# Check if cluster context exists
if ! kubectl config get-contexts "${CLUSTER_CONTEXT}" &>/dev/null 2>&1; then
    log_warning "Cluster context '${CLUSTER_CONTEXT}' not found"
    log_info "Attempting to use current context..."
    CURRENT_CONTEXT=$(kubectl config current-context 2>/dev/null || echo "")
    if [ -z "${CURRENT_CONTEXT}" ]; then
        log_error "No kubectl context available"
        exit 1
    fi
    log_info "Using context: ${CURRENT_CONTEXT}"
else
    kubectl config use-context "${CLUSTER_CONTEXT}" &>/dev/null || true
fi

# ============================================================================
# Step 1: Uninstall Agent Management Platform Helm Releases
# ============================================================================

log_step "Step 1/5: Uninstalling Agent Management Platform"

# Uninstall Platform Resources Extension
if helm status "amp-platform-resources" -n "${DEFAULT_NS}" &>/dev/null 2>&1; then
    log_info "Uninstalling Platform Resources Extension..."
    if helm uninstall "amp-platform-resources" -n "${DEFAULT_NS}" &>/dev/null; then
        log_success "Platform Resources Extension uninstalled"
    else
        log_warning "Failed to uninstall Platform Resources Extension (non-fatal)"
    fi
else
    log_info "Platform Resources Extension not found, skipping..."
fi

# Uninstall Build Extension
if helm status "build-workflow-extensions" -n "${BUILD_CI_NS}" &>/dev/null 2>&1; then
    log_info "Uninstalling Build Extension..."
    if helm uninstall "build-workflow-extensions" -n "${BUILD_CI_NS}" &>/dev/null; then
        log_success "Build Extension uninstalled"
    else
        log_warning "Failed to uninstall Build Extension (non-fatal)"
    fi
else
    log_info "Build Extension not found, skipping..."
fi

# Uninstall Observability Extension
if helm status "amp-observability-traces" -n "${OBSERVABILITY_NS}" &>/dev/null 2>&1; then
    log_info "Uninstalling Observability Extension..."
    if helm uninstall "amp-observability-traces" -n "${OBSERVABILITY_NS}" &>/dev/null; then
        log_success "Observability Extension uninstalled"
    else
        log_warning "Failed to uninstall Observability Extension (non-fatal)"
    fi
else
    log_info "Observability Extension not found, skipping..."
fi

# Uninstall main Agent Management Platform
if helm status "amp" -n "${AMP_NS}" &>/dev/null 2>&1; then
    log_info "Uninstalling Agent Management Platform..."
    if helm uninstall "amp" -n "${AMP_NS}" &>/dev/null; then
        log_success "Agent Management Platform uninstalled"
    else
        log_warning "Failed to uninstall Agent Management Platform (non-fatal)"
    fi
else
    log_info "Agent Management Platform not found, skipping..."
fi

# Wait for resources to be cleaned up
log_info "Waiting for resources to be cleaned up..."
sleep 5

# ============================================================================
# Step 2: Delete AMP Custom Resources
# ============================================================================

log_step "Step 2/5: Deleting AMP Custom Resources"

# Delete any remaining AMP-related custom resources
log_info "Cleaning up AMP custom resources..."
for resource_type in organization project environment deploymentpipeline component workflowtemplate; do
    if kubectl get "${resource_type}" -A &>/dev/null 2>&1; then
        kubectl delete "${resource_type}" --all --all-namespaces --timeout=30s &>/dev/null || true
    fi
done

# Only delete DataPlane/BuildPlane if we're doing full uninstall
if [ "${AMP_ONLY}" = false ]; then
    # Delete DataPlane resources
    log_info "Deleting DataPlane resources..."
    if kubectl get dataplane -A &>/dev/null 2>&1; then
        kubectl delete dataplane --all --all-namespaces --timeout=30s &>/dev/null || true
        log_success "DataPlane resources deleted"
    else
        log_info "No DataPlane resources found"
    fi

    # Delete BuildPlane resources
    log_info "Deleting BuildPlane resources..."
    if kubectl get buildplane -A &>/dev/null 2>&1; then
        kubectl delete buildplane --all --all-namespaces --timeout=30s &>/dev/null || true
        log_success "BuildPlane resources deleted"
    else
        log_info "No BuildPlane resources found"
    fi
fi

# ============================================================================
# Step 3: Uninstall OpenChoreo Helm Releases (Skip if --amp-only)
# ============================================================================

if [ "${AMP_ONLY}" = false ]; then
    log_step "Step 3/5: Uninstalling OpenChoreo"

    # Uninstall Observability Plane
    if helm status "openchoreo-observability-plane" -n "openchoreo-observability-plane" &>/dev/null 2>&1; then
        log_info "Uninstalling OpenChoreo Observability Plane..."
        if helm uninstall "openchoreo-observability-plane" -n "openchoreo-observability-plane" &>/dev/null; then
            log_success "OpenChoreo Observability Plane uninstalled"
        else
            log_warning "Failed to uninstall OpenChoreo Observability Plane (non-fatal)"
        fi
    else
        log_info "OpenChoreo Observability Plane not found, skipping..."
    fi

    # Uninstall Build Plane
    if helm status "openchoreo-build-plane" -n "openchoreo-build-plane" &>/dev/null 2>&1; then
        log_info "Uninstalling OpenChoreo Build Plane..."
        if helm uninstall "openchoreo-build-plane" -n "openchoreo-build-plane" &>/dev/null; then
            log_success "OpenChoreo Build Plane uninstalled"
        else
            log_warning "Failed to uninstall OpenChoreo Build Plane (non-fatal)"
        fi
    else
        log_info "OpenChoreo Build Plane not found, skipping..."
    fi

    # Uninstall Data Plane
    if helm status "openchoreo-data-plane" -n "openchoreo-data-plane" &>/dev/null 2>&1; then
        log_info "Uninstalling OpenChoreo Data Plane..."
        if helm uninstall "openchoreo-data-plane" -n "openchoreo-data-plane" &>/dev/null; then
            log_success "OpenChoreo Data Plane uninstalled"
        else
            log_warning "Failed to uninstall OpenChoreo Data Plane (non-fatal)"
        fi
    else
        log_info "OpenChoreo Data Plane not found, skipping..."
    fi

    # Uninstall Control Plane (should be last as other planes may depend on it)
    if helm status "openchoreo-control-plane" -n "openchoreo-control-plane" &>/dev/null 2>&1; then
        log_info "Uninstalling OpenChoreo Control Plane..."
        if helm uninstall "openchoreo-control-plane" -n "openchoreo-control-plane" &>/dev/null; then
            log_success "OpenChoreo Control Plane uninstalled"
        else
            log_warning "Failed to uninstall OpenChoreo Control Plane (non-fatal)"
        fi
    else
        log_info "OpenChoreo Control Plane not found, skipping..."
    fi
else
    log_step "Step 3/5: Skipping OpenChoreo Uninstallation (--amp-only mode)"
    log_info "OpenChoreo resources are preserved"
fi

# ============================================================================
# Step 4: Clean Up Namespaces
# ============================================================================

log_step "Step 4/5: Cleaning Up Namespaces"

# Delete AMP namespace
if kubectl get namespace "${AMP_NS}" &>/dev/null 2>&1; then
    log_info "Deleting namespace ${AMP_NS}..."
    if kubectl delete namespace "${AMP_NS}" --timeout=60s &>/dev/null; then
        log_success "Namespace ${AMP_NS} deleted"
    else
        log_warning "Failed to delete namespace ${AMP_NS} (may contain finalizers)"
        log_info "Attempting force delete..."
        kubectl delete namespace "${AMP_NS}" --force --grace-period=0 &>/dev/null || true
    fi
else
    log_info "Namespace ${AMP_NS} not found, skipping..."
fi

# Delete OpenChoreo namespaces only if not in --amp-only mode
if [ "${AMP_ONLY}" = false ]; then
    for ns in "openchoreo-control-plane" "openchoreo-data-plane" "openchoreo-build-plane" "openchoreo-observability-plane"; do
        if kubectl get namespace "${ns}" &>/dev/null 2>&1; then
            # Check if namespace has no pods (simplified check)
            POD_COUNT=$(kubectl get pods -n "${ns}" --no-headers 2>/dev/null | wc -l || echo "0")
            if [ "${POD_COUNT}" -eq 0 ]; then
                log_info "Deleting namespace ${ns}..."
                kubectl delete namespace "${ns}" --timeout=60s &>/dev/null || {
                    log_warning "Failed to delete namespace ${ns}, attempting force delete..."
                    kubectl delete namespace "${ns}" --force --grace-period=0 &>/dev/null || true
                }
            else
                log_info "Namespace ${ns} still contains resources, skipping deletion"
            fi
        fi
    done
else
    log_info "Preserving OpenChoreo namespaces (--amp-only mode)"
fi

# ============================================================================
# Step 5: Delete Cluster (Optional)
# ============================================================================

if [ "${DELETE_CLUSTER}" = true ]; then
    log_step "Step 5/5: Deleting k3d Cluster"
    
    if ! command_exists k3d; then
        log_warning "k3d is not installed, skipping cluster deletion"
    else
        if k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
            log_info "Deleting k3d cluster '${CLUSTER_NAME}'..."
            if k3d cluster delete "${CLUSTER_NAME}" &>/dev/null; then
                log_success "Cluster '${CLUSTER_NAME}' deleted"
            else
                log_error "Failed to delete cluster '${CLUSTER_NAME}'"
            fi
        else
            log_info "Cluster '${CLUSTER_NAME}' not found, skipping..."
        fi
    fi
else
    log_step "Step 5/5: Skipping Cluster Deletion"
    log_info "Cluster preserved. To delete the cluster, run:"
    log_info "  ./uninstall.sh --delete-cluster"
fi

# ============================================================================
# VERIFICATION
# ============================================================================

log_step "Verification"

log_info "Remaining Helm Releases:"
helm list -A 2>/dev/null || log_info "No helm releases found"
echo ""

log_info "Remaining Namespaces:"
kubectl get namespaces 2>/dev/null | grep -E "(openchoreo|wso2-amp)" || log_info "No AMP/OpenChoreo namespaces found"
echo ""

# ============================================================================
# SUCCESS
# ============================================================================

log_step "Uninstallation Complete!"

if [ "${AMP_ONLY}" = true ]; then
    if [ "${DELETE_CLUSTER}" = true ]; then
        log_success "AMP resources have been removed and cluster has been deleted!"
    else
        log_success "AMP resources have been removed!"
        log_info "OpenChoreo resources have been preserved."
        log_info "Cluster '${CLUSTER_NAME}' has been preserved."
    fi
else
    if [ "${DELETE_CLUSTER}" = true ]; then
        log_success "All platform resources and cluster have been removed!"
    else
        log_success "All platform resources have been removed!"
        log_info "Cluster '${CLUSTER_NAME}' has been preserved."
    fi
fi

echo ""
if [ "${AMP_ONLY}" = true ]; then
    log_info "To reinstall AMP, run: ./install.sh"
else
    log_info "To reinstall, run: ./install.sh"
fi
echo ""

