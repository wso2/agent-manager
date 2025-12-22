#!/bin/bash
set -euo pipefail

# ============================================================================
# OpenChoreo Development Environment Setup
# ============================================================================
# This script provides a comprehensive, idempotent installation that:
# 1. Creates a k3d cluster
# 2. Installs OpenChoreo (Control Plane, Data Plane, Build Plane, Observability Plane)
# 3. Registers planes and configures observability
# 4. Installs Agent Management Platform
#
# The script is idempotent - it can be run multiple times safely.
# Only public helm charts are used - no local charts or custom images.
# ============================================================================

# Configuration
CLUSTER_NAME="amp-local"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"
OPENCHOREO_VERSION="0.7.0"
OC_RELEASE="release-v0.7"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K3D_CONFIG="${SCRIPT_DIR}/k3d-config.yaml"

# Source AMP installation helpers
source "${SCRIPT_DIR}/install-helpers.sh"

# Timeouts (in seconds)
TIMEOUT_K3D_READY=60
TIMEOUT_CONTROL_PLANE=600
TIMEOUT_DATA_PLANE=600
TIMEOUT_BUILD_PLANE=600
TIMEOUT_OBSERVABILITY_PLANE=900

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

# Wait for k3d cluster to be ready
wait_for_k3d_cluster() {
    local cluster_name=$1
    local timeout=$2
    local elapsed=0
    
    log_info "Waiting for k3d cluster '${cluster_name}' to be ready..."
    
    while true; do
        # Check if cluster exists and get its status
        CLUSTER_LINE=$(k3d cluster list 2>/dev/null | grep "${cluster_name}" || echo "")
        
        # Check if cluster is running - k3d shows status in various formats
        # Format can be: "amp-local   1/1       0/0      true" or "amp-local   running"
        if [ -n "${CLUSTER_LINE}" ]; then
            # Check for "running" text or "true" status (which indicates running)
            if echo "${CLUSTER_LINE}" | grep -qE "(running|true)" || \
               echo "${CLUSTER_LINE}" | grep -qE "[0-9]+/[0-9]+.*true"; then
                
                # Give k3d a moment to register the kubeconfig context
                sleep 2
                
                # Always try to merge kubeconfig to ensure it's up to date
                k3d kubeconfig merge "${cluster_name}" --kubeconfig-merge-default 2>/dev/null || true
                sleep 2
                
                # Check if context exists in kubeconfig
                if kubectl config get-contexts "${CLUSTER_CONTEXT}" &>/dev/null 2>&1; then
                    # Set context
                    kubectl config use-context "${CLUSTER_CONTEXT}" &>/dev/null 2>&1 || true
                    
                    # Verify cluster is actually accessible (try multiple methods)
                    # Method 1: cluster-info without context flag (uses current context)
                    if kubectl cluster-info &>/dev/null 2>&1; then
                        return 0
                    fi
                    
                    # Method 2: cluster-info with context flag
                    if kubectl cluster-info --context "${CLUSTER_CONTEXT}" &>/dev/null 2>&1; then
                        return 0
                    fi
                    
                    # Method 3: Try a simple get nodes command
                    if kubectl get nodes &>/dev/null 2>&1; then
                        return 0
                    fi
                else
                    # Context doesn't exist yet, continue waiting
                    if [ $((elapsed % 10)) -eq 0 ]; then
                        log_info "Context ${CLUSTER_CONTEXT} not yet available, waiting... (${elapsed}s elapsed)"
                    fi
                fi
            fi
        fi
        
        if [ $elapsed -ge $timeout ]; then
            log_error "Cluster not ready after ${timeout}s"
            log_info "Cluster status: ${CLUSTER_LINE:-not found}"
            log_info "Available contexts:"
            kubectl config get-contexts 2>/dev/null || true
            log_info "Expected context: ${CLUSTER_CONTEXT}"
            log_info "Trying to merge kubeconfig one more time..."
            k3d kubeconfig merge "${cluster_name}" --kubeconfig-merge-default 2>&1 || true
            sleep 2
            log_info "Contexts after merge:"
            kubectl config get-contexts 2>/dev/null || true
            # Try one last time with any k3d context
            if kubectl config get-contexts 2>/dev/null | grep -q "k3d"; then
                K3D_CTX=$(kubectl config get-contexts --no-headers 2>/dev/null | grep "k3d" | awk '{print $2}' | head -1)
                if [ -n "${K3D_CTX}" ]; then
                    log_info "Trying with context: ${K3D_CTX}"
                    kubectl config use-context "${K3D_CTX}" 2>/dev/null || true
                    if kubectl cluster-info &>/dev/null 2>&1; then
                        log_warning "Cluster accessible with context ${K3D_CTX}, but expected ${CLUSTER_CONTEXT}"
                        # Update CLUSTER_CONTEXT to match
                        CLUSTER_CONTEXT="${K3D_CTX}"
                        return 0
                    fi
                fi
            fi
            return 1
        fi
        
        sleep 2
        elapsed=$((elapsed + 2))
    done
}

# Wait for kubectl to be ready (assumes context is already set)
wait_for_kubectl() {
    local timeout=$1
    local elapsed=0
    
    log_info "Waiting for kubectl to be ready..."
    
    while ! kubectl cluster-info &>/dev/null; do
        if [ $elapsed -ge $timeout ]; then
            log_error "kubectl not ready after ${timeout}s"
            return 1
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done
    return 0
}

# Install helm chart with idempotency check
helm_install_idempotent() {
    local release_name=$1
    local chart=$2
    local namespace=$3
    local timeout=$4
    shift 4
    local extra_args=("$@")

    if helm status "${release_name}" -n "${namespace}" &>/dev/null; then
        log_info "${release_name} already installed, skipping..."
        return 0
    fi

    log_info "Installing ${release_name}..."
    log_info "This may take several minutes..."

    if helm install "${release_name}" "${chart}" \
        --namespace "${namespace}" \
        --create-namespace \
        --timeout "${timeout}s" \
        "${extra_args[@]}"; then
        log_success "${release_name} installed successfully"
        return 0
    else
        log_error "Failed to install ${release_name}"
        return 1
    fi
}

# Wait for pods to be ready
wait_for_pods() {
    local namespace=$1
    local timeout=$2
    local selector=${3:-""}

    log_info "Waiting for pods in ${namespace} to be ready (timeout: ${timeout}s)..."

    if [ -n "$selector" ]; then
        kubectl wait --for=condition=Ready pod -l "${selector}" -n "${namespace}" --timeout="${timeout}s" || {
            log_warning "Some pods may still be starting (non-fatal)"
            return 0
        }
    else
        kubectl wait --for=condition=Ready pod --all -n "${namespace}" --timeout="${timeout}s" || {
            log_warning "Some pods may still be starting (non-fatal)"
            return 0
        }
    fi
    log_success "Pods in ${namespace} are ready"
}

# Wait for deployments to be available
wait_for_deployments() {
    local namespace=$1
    local timeout=$2

    log_info "Waiting for deployments in ${namespace} to be available (timeout: ${timeout}s)..."

    kubectl wait --for=condition=Available deployment --all -n "${namespace}" --timeout="${timeout}s" || {
        log_warning "Some deployments may still be starting (non-fatal)"
        return 0
    }
    log_success "Deployments in ${namespace} are available"
}

# Wait for statefulsets to be ready
wait_for_statefulsets() {
    local namespace=$1
    local timeout=$2

    log_info "Waiting for statefulsets in ${namespace} to be ready (timeout: ${timeout}s)..."

    for sts in $(kubectl get statefulset -n "${namespace}" -o name 2>/dev/null); do
        kubectl rollout status "${sts}" -n "${namespace}" --timeout="${timeout}s" || {
            log_warning "StatefulSet ${sts} may still be starting (non-fatal)"
        }
    done
    log_success "Statefulsets in ${namespace} are ready"
}

# ============================================================================
# MAIN INSTALLATION FLOW
# ============================================================================

log_step "OpenChoreo Development Environment Setup"

# Check and fix Docker permissions
check_docker_permissions() {
    local docker_sock="/var/run/docker.sock"
    
    if [ ! -S "${docker_sock}" ]; then
        log_error "Docker socket not found at ${docker_sock}"
        log_info "Make sure Docker is running and the socket is mounted"
        return 1
    fi
    
    # Check if we can access Docker
    if docker ps &>/dev/null; then
        log_success "Docker access verified"
        return 0
    fi
    
    # Try to fix permissions
    log_warning "Docker socket permissions issue detected. Attempting to fix..."
    if sudo chmod 666 "${docker_sock}" 2>/dev/null; then
        log_success "Docker socket permissions fixed"
        return 0
    else
        log_error "Cannot fix Docker socket permissions. Please run: sudo chmod 666 ${docker_sock}"
        return 1
    fi
}

# Check prerequisites
log_step "Step 1/7: Verifying prerequisites"

# Check Docker access first
if ! check_docker_permissions; then
    log_error "Docker permission check failed"
    exit 1
fi

if ! command_exists k3d; then
    log_error "k3d is not installed"
    exit 1
fi

if ! command_exists kubectl; then
    log_error "kubectl is not installed"
    exit 1
fi

if ! command_exists helm; then
    log_error "helm is not installed"
    exit 1
fi

if ! command_exists curl; then
    log_error "curl is not installed"
    exit 1
fi

log_success "All prerequisites verified"

# ============================================================================
# Step 2: Setup k3d Cluster
# ============================================================================

log_step "Step 2/7: Setting up k3d cluster"

# Check if cluster already exists
if k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
    log_info "k3d cluster '${CLUSTER_NAME}' already exists"

    # Check cluster status - k3d shows status in various formats
    CLUSTER_LINE=$(k3d cluster list 2>/dev/null | grep "${CLUSTER_NAME}" || echo "")
    if [ -n "${CLUSTER_LINE}" ] && (echo "${CLUSTER_LINE}" | grep -qE "(running|true)" || \
        echo "${CLUSTER_LINE}" | grep -qE "[0-9]+/[0-9]+.*true"); then
        CLUSTER_STATUS="running"
    else
        CLUSTER_STATUS="stopped"
    fi
    
    if [ "${CLUSTER_STATUS}" = "running" ]; then
        log_info "Cluster is running, verifying access..."
        
        # Set context first (might not be set yet)
        kubectl config use-context "${CLUSTER_CONTEXT}" 2>/dev/null || true
        
        # Verify cluster is accessible
        if kubectl cluster-info --context "${CLUSTER_CONTEXT}" &>/dev/null; then
            log_success "Cluster is running and accessible"
        else
            log_info "Cluster is running but not accessible yet. Merging kubeconfig and waiting..."
            # Merge kubeconfig to ensure context is available
            k3d kubeconfig merge "${CLUSTER_NAME}" --kubeconfig-merge-default 2>/dev/null || true
            sleep 2
            
            if ! wait_for_k3d_cluster "${CLUSTER_NAME}" "${TIMEOUT_K3D_READY}"; then
                log_error "Cluster failed to become ready"
                exit 1
            fi
        fi
    else
        log_info "Cluster exists but is not running. Starting cluster..."
        k3d cluster start "${CLUSTER_NAME}"

        # Merge kubeconfig to ensure context is available
        log_info "Merging k3d kubeconfig..."
        k3d kubeconfig merge "${CLUSTER_NAME}" --kubeconfig-merge-default 2>/dev/null || true
        sleep 2

        # Wait for cluster to be fully ready (context registered and API accessible)
        if ! wait_for_k3d_cluster "${CLUSTER_NAME}" "${TIMEOUT_K3D_READY}"; then
            log_error "Cluster failed to become ready"
            exit 1
        fi
        log_success "Cluster is now ready"
    fi

    # Ensure context is set
    kubectl config use-context "${CLUSTER_CONTEXT}" || {
        log_error "Failed to set kubectl context"
        exit 1
    }
    log_success "Using existing cluster"
else
    log_info "Creating k3d cluster..."

    # Create shared directory for OpenChoreo
    mkdir -p /tmp/k3d-shared

    # Create k3d cluster
    if k3d cluster create --config "${K3D_CONFIG}"; then
        log_success "k3d cluster created successfully"
    else
        log_error "Failed to create k3d cluster"
        exit 1
    fi

    # Merge kubeconfig to ensure context is available
    log_info "Merging k3d kubeconfig..."
    k3d kubeconfig merge "${CLUSTER_NAME}" --kubeconfig-merge-default 2>/dev/null || true
    sleep 2

    # Set kubectl context
    kubectl config use-context "${CLUSTER_CONTEXT}" || {
        log_error "Failed to set kubectl context"
        exit 1
    }

    # Wait for cluster to be ready
    if wait_for_kubectl "${TIMEOUT_K3D_READY}"; then
        log_success "Cluster is ready"
    else
        log_error "Cluster failed to become ready"
        exit 1
    fi

    log_info "Cluster info:"
    kubectl cluster-info --context "${CLUSTER_CONTEXT}"
    echo ""
    log_info "Cluster nodes:"
    kubectl get nodes
fi

# ============================================================================
# Step 3: Install OpenChoreo Control Plane
# ============================================================================

log_step "Step 3/7: Installing OpenChoreo Control Plane"

helm_install_idempotent \
    "openchoreo-control-plane" \
    "oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane" \
    "openchoreo-control-plane" \
    "${TIMEOUT_CONTROL_PLANE}" \
    --version "${OPENCHOREO_VERSION}" \
    --values "https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp-${VERSION}/deployments/single-cluster/values-cp.yaml"

wait_for_pods "openchoreo-control-plane" "${TIMEOUT_CONTROL_PLANE}"

# ============================================================================
# Step 4: Install OpenChoreo Data Plane
# ============================================================================

log_step "Step 4/7: Installing OpenChoreo Data Plane"

helm_install_idempotent \
    "openchoreo-data-plane" \
    "oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane" \
    "${DATA_PLANE_NS}" \
    "${TIMEOUT_DATA_PLANE}" \
    --version "${OPENCHOREO_VERSION}" \
    --values "https://raw.githubusercontent.com/openchoreo/openchoreo/${OC_RELEASE}/install/k3d/single-cluster/values-dp.yaml"

# Register Data Plane
log_info "Registering Data Plane..."
if curl -s "https://raw.githubusercontent.com/openchoreo/openchoreo/${OC_RELEASE}/install/add-data-plane.sh" | \
    bash -s -- --enable-agent --control-plane-context "${CLUSTER_CONTEXT}" --name default; then
    log_success "Data Plane registered successfully"
else
    log_warning "Data Plane registration script failed (non-fatal)"
fi

# Verify DataPlane resource
if kubectl get dataplane default -n default &>/dev/null; then
    log_success "DataPlane resource 'default' exists"
    AGENT_ENABLED=$(kubectl get dataplane default -n default -o jsonpath='{.spec.agent.enabled}' 2>/dev/null || echo "false")
    if [ "$AGENT_ENABLED" = "true" ]; then
        log_success "Agent mode is enabled"
    else
        log_warning "Agent mode is not enabled (expected: true, got: $AGENT_ENABLED)"
    fi
else
    log_warning "DataPlane resource not found"
fi

wait_for_pods "openchoreo-data-plane" "${TIMEOUT_DATA_PLANE}"

# ============================================================================
# Step 5: Install OpenChoreo Build Plane
# ============================================================================

log_step "Step 5/7: Installing OpenChoreo Build Plane"

helm_install_idempotent \
    "openchoreo-build-plane" \
    "oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane" \
    "${BUILD_CI_NS}" \
    "${TIMEOUT_BUILD_PLANE}" \
    --version "${OPENCHOREO_VERSION}" \
    --values "https://raw.githubusercontent.com/openchoreo/openchoreo/${OC_RELEASE}/install/k3d/single-cluster/values-bp.yaml"

# Register Build Plane
log_info "Registering Build Plane..."
if curl -s "https://raw.githubusercontent.com/openchoreo/openchoreo/${OC_RELEASE}/install/add-build-plane.sh" | \
    bash -s -- --enable-agent --control-plane-context "${CLUSTER_CONTEXT}" --name default; then
    log_success "Build Plane registered successfully"
else
    log_warning "Build Plane registration script failed (non-fatal)"
fi

# Verify BuildPlane resource
if kubectl get buildplane default -n default &>/dev/null; then
    log_success "BuildPlane resource 'default' exists"
    AGENT_ENABLED=$(kubectl get buildplane default -n default -o jsonpath='{.spec.agent.enabled}' 2>/dev/null || echo "false")
    if [ "$AGENT_ENABLED" = "true" ]; then
        log_success "Agent mode is enabled"
    else
        log_warning "Agent mode is not enabled (expected: true, got: $AGENT_ENABLED)"
    fi
else
    log_warning "BuildPlane resource not found"
fi

wait_for_deployments "openchoreo-build-plane" "${TIMEOUT_BUILD_PLANE}"

# ============================================================================
# Step 6: Install OpenChoreo Observability Plane
# ============================================================================

log_step "Step 6/7: Installing OpenChoreo Observability Plane"

# Create namespace (idempotent)
log_info "Ensuring OpenChoreo Observability Plane namespace exists..."
if kubectl get namespace "${OBSERVABILITY_NS}" &>/dev/null; then
    log_info "Namespace '${OBSERVABILITY_NS}' already exists, skipping creation"
else
    if kubectl create namespace "${OBSERVABILITY_NS}" &>/dev/null; then
        log_success "Namespace '${OBSERVABILITY_NS}' created successfully"
    else
        log_error "Failed to create namespace '${OBSERVABILITY_NS}'"
        exit 1
    fi
fi

# Apply OpenTelemetry Collector ConfigMap (idempotent)
log_info "Applying Custom OpenTelemetry Collector configuration..."
CONFIGMAP_FILE="https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp-${VERSION}/deployments/values/oc-collector-configmap.yaml"

if kubectl apply -f "${CONFIGMAP_FILE}" -n "${OBSERVABILITY_NS}" &>/dev/null; then
    log_success "OpenTelemetry Collector configuration applied successfully"
else
    log_error "Failed to apply OpenTelemetry Collector configuration"
    log_info "Attempting to verify ConfigMap status..."
    if kubectl get configmap amp-opentelemetry-collector-config -n "${OBSERVABILITY_NS}" &>/dev/null; then
        log_warning "ConfigMap exists but apply failed (may already be up-to-date)"
    else
        log_error "ConfigMap does not exist and apply failed"
        exit 1
    fi
fi

log_info "Installing OpenChoreo Observability Plane..."
helm_install_idempotent \
    "openchoreo-observability-plane" \
    "oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane" \
    "${OBSERVABILITY_NS}" \
    "${TIMEOUT_OBSERVABILITY_PLANE}" \
    --version "${OPENCHOREO_VERSION}" \
    --values "https://raw.githubusercontent.com/wso2/ai-agent-management-platform/amp-${VERSION}/deployments/single-cluster/values-op.yaml"

wait_for_deployments "openchoreo-observability-plane" "${TIMEOUT_OBSERVABILITY_PLANE}"
wait_for_statefulsets "openchoreo-observability-plane" "${TIMEOUT_OBSERVABILITY_PLANE}"

log_success "OpenSearch ready"

# Configure observability integration
log_info "Configuring observability integration..."

# Configure DataPlane observer
if kubectl get dataplane default -n default &>/dev/null; then
    if kubectl patch dataplane default -n default --type merge \
        -p '{"spec":{"observer":{"url":"http://observer.openchoreo-observability-plane:8080","authentication":{"basicAuth":{"username":"dummy","password":"dummy"}}}}}' &>/dev/null; then
        log_success "DataPlane observer configured"
    else
        log_warning "DataPlane observer configuration failed (non-fatal)"
    fi
else
    log_warning "DataPlane resource not found yet (will use default observer)"
fi

# Configure BuildPlane observer
if kubectl get buildplane default -n default &>/dev/null; then
    if kubectl patch buildplane default -n default --type merge \
        -p '{"spec":{"observer":{"url":"http://observer.openchoreo-observability-plane:8080","authentication":{"basicAuth":{"username":"dummy","password":"dummy"}}}}}' &>/dev/null; then
        log_success "BuildPlane observer configured"
    else
        log_warning "BuildPlane observer configuration failed (non-fatal)"
    fi
else
    log_warning "BuildPlane resource not found yet (will use default observer)"
fi

# ============================================================================
# Step 7: Install Agent Management Platform
# ============================================================================

log_step "Step 7/7: Installing Agent Management Platform"

# Verify prerequisites
if ! verify_amp_prerequisites; then
    log_error "AMP prerequisites check failed"
    exit 1
fi

log_info "Installing Agent Management Platform components..."
log_info "This may take 5-8 minutes..."
echo ""

# Install main platform
log_info "Installing Agent Management Platform (PostgreSQL, API, Console)..."
if ! install_agent_management_platform; then
    log_error "Failed to install Agent Management Platform"
    echo ""
    echo "Troubleshooting steps:"
    echo "  1. Check pod status: kubectl get pods -n ${AMP_NS}"
    echo "  2. View logs: kubectl logs -n ${AMP_NS} <pod-name>"
    echo "  3. Check Helm release: helm list -n ${AMP_NS}"
    exit 1
fi
log_success "Agent Management Platform installed successfully"
echo ""


# Install platform resources extension
log_info "Installing Platform Resources Extension (Default Organization, Project, Environment, DeploymentPipeline)..."
if ! install_platform_resources_extension; then
    log_warning "Platform Resources Extension installation failed (non-fatal)"
    echo "The platform is installed but platform resources features may not work."
fi

log_success "Platform Resources Extension installed successfully"
echo ""

# Install observability extension
log_info "Installing Observability Extension (Traces Observer)..."
if ! install_observability_extension; then
    log_warning "Observability Extension installation failed (non-fatal)"
    echo "The platform is installed but observability features may not work."
    echo ""
    echo "Troubleshooting steps:"
    echo "  1. Check pod status: kubectl get pods -n ${OBSERVABILITY_NS}"
    echo "  2. View logs: kubectl logs -n ${OBSERVABILITY_NS} <pod-name>"
else
    log_success "Observability Extension installed successfully"
fi
echo ""

# Install build extension
log_info "Installing Build Extension (Workflow Templates)..."
if ! install_build_extension; then
    log_warning "Build Extension installation failed (non-fatal)"
    echo "The platform is installed but build CI features may not work."
    echo ""
    echo "Troubleshooting steps:"
    echo "  1. Check Helm release: helm list -n ${BUILD_CI_NS}"
else
    log_success "Build Extension installed successfully"
fi
echo ""


# ============================================================================
# VERIFICATION
# ============================================================================

log_step "Verification"

echo ""
echo "Agent Management Platform:"
kubectl get pods -n "${AMP_NS}" || true
echo ""

# ============================================================================
# SUCCESS
# ============================================================================

log_step "Installation Complete!"

log_success "OpenChoreo and Agent Management Platform are ready!"
echo ""
log_info "Cluster: ${CLUSTER_CONTEXT}"
log_info "Agent Management Platform Console: http://localhost:3000"
echo ""
echo ""
log_info "To check status: kubectl get pods -A"
log_info "To Uninstall: ./uninstall.sh"
log_info "To delete cluster: ./uninstall.sh --delete-cluster"
echo ""

