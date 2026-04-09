#!/bin/bash
set -euo pipefail

# ============================================================================
# OpenChoreo Development Environment Setup
# ============================================================================
# This script provides a comprehensive, idempotent installation that:
# 1. Creates a k3d cluster
# 2. Installs OpenChoreo (Control Plane, Data Plane, Workflow Plane, Observability Plane)
# 3. Registers planes and configures observability
# 4. Installs Agent Management Platform
#
# The script is idempotent - it can be run multiple times safely.
# Only public helm charts are used - no local charts or custom images.
# ============================================================================

# Configuration
CLUSTER_NAME="amp-local"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"
OPENCHOREO_VERSION="1.0.0-rc.1"
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

# Colors for output (8-bit mode for maximum compatibility)
RED='\033[1;31m'
GREEN='\033[1;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Helper functions
log_info() {
    echo -e "${NC}ℹ${NC} $1"
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
    echo -e "${NC}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${NC}$1${NC}"
    echo -e "${NC}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check if a port is available
check_port_available() {
    local port=$1
    local hex_port
    hex_port=$(printf "%04X" "${port}")

    # Use /proc/net/tcp if available (reliable in Linux containers)
    local proc_files=()
    [ -r /proc/net/tcp ]  && proc_files+=(/proc/net/tcp)
    [ -r /proc/net/tcp6 ] && proc_files+=(/proc/net/tcp6)
    if [ ${#proc_files[@]} -gt 0 ]; then
        if grep -qE ":${hex_port} .* 0A " "${proc_files[@]}" 2>/dev/null; then
            return 1  # Port is in use
        fi
        return 0  # Port is available
    fi

    # Fall back to lsof for macOS
    if lsof -iTCP:"${port}" -sTCP:LISTEN -Pn >/dev/null 2>&1; then
        return 1  # Port is in use
    fi
    return 0  # Port is available
}

# Check all required ports are available
check_required_ports() {
    # Required ports for k3d cluster (host:container mapping)
    # 3000  - AMP Console UI
    # 8080  - kgateway HTTP (Thunder auth + OpenChoreo API routing)
    # 8443  - kgateway HTTPS
    # 8084  - AI Gateway HTTP
    # 8243  - AI Gateway HTTPS
    # 9000  - AMP API service
    # 9098  - AMP Traces Observer
    # 9243  - AMP Internal API endpoint
    # 10082 - Container Registry (Workflow Plane)
    # 11080 - Observer API (Observability Plane)
    # 11082 - OpenSearch API
    # 11085 - OpenSearch HTTPS
    # 19080 - Data Plane Gateway HTTP (agent workloads)
    # 19443 - Data Plane Gateway HTTPS
    # 21893 - OTel Collector
    # 22893 - Observability Gateway HTTP
    # 22894 - Observability Gateway HTTPS
    local required_ports=(3000 8080 8443 8084 8243 9000 9098 9243 10082 11080 11082 11085 19080 19443 21893 22893 22894)
    local ports_in_use=()

    for port in "${required_ports[@]}"; do
        if ! check_port_available "$port"; then
            ports_in_use+=("$port")
        fi
    done

    if [ ${#ports_in_use[@]} -gt 0 ]; then
        log_error "The following required ports are already in use: ${ports_in_use[*]}"
        log_info "Please free these ports before running the installer."
        log_info "To find processes using these ports, run: lsof -i :<port>"
        return 1
    fi

    return 0
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

# Wait for workloads to be ready in a namespace.
# Uses deployment and statefulset waits instead of raw pod waits to avoid a
# race condition where kubectl wait captures a Job pod in Running phase, then
# hangs indefinitely when it transitions to Succeeded (since Succeeded pods
# never become Ready).
wait_for_pods() {
    local namespace=$1
    local timeout=$2
    local selector=${3:-""}

    local all_ready=true
    if [ -n "$selector" ]; then
        log_info "Waiting for pods (${selector}) in ${namespace} to be ready (timeout: ${timeout}s)..."
        kubectl wait --for=condition=Ready pod -l "${selector}" --field-selector=status.phase!=Succeeded -n "${namespace}" --timeout="${timeout}s" || {
            log_warning "Some pods may still be starting (non-fatal)"
            all_ready=false
        }
    else
        log_info "Waiting for workloads in ${namespace} to be ready (timeout: ${timeout}s)..."
        kubectl wait --for=condition=Available deployment --all -n "${namespace}" --timeout="${timeout}s" 2>/dev/null || {
            log_warning "Some deployments may still be starting (non-fatal)"
            all_ready=false
        }
        for sts in $(kubectl get statefulset -n "${namespace}" -o name 2>/dev/null); do
            kubectl rollout status "${sts}" -n "${namespace}" --timeout="${timeout}s" 2>/dev/null || {
                log_warning "StatefulSet ${sts} may still be starting (non-fatal)"
                all_ready=false
            }
        done
        for job in $(kubectl get jobs -n "${namespace}" -o name 2>/dev/null); do
            if kubectl get "${job}" -n "${namespace}" -o jsonpath='{.status.conditions[?(@.type=="Failed")].status}' 2>/dev/null | grep -q '^True$'; then
                log_warning "Job ${job} has failed; skipping Complete wait (non-fatal)"
                all_ready=false
                continue
            fi
            kubectl wait --for=condition=Complete "${job}" -n "${namespace}" --timeout="${timeout}s" 2>/dev/null || {
                log_warning "Job ${job} may still be running (non-fatal)"
                all_ready=false
            }
        done
    fi
    if [ "${all_ready}" = true ]; then
        log_success "Workloads in ${namespace} are ready"
    else
        log_warning "Workloads in ${namespace} are running but some were not fully ready"
    fi
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

# Wait for a job to complete
wait_for_job() {
    local job_name=$1
    local namespace=$2
    local timeout=$3
    local interval=5
    local elapsed=0

    log_info "Waiting for job '${job_name}' in ${namespace} to exist (timeout: ${timeout}s)..."

    # First, poll for job existence
    while [ $elapsed -lt $timeout ]; do
        if kubectl get job/"${job_name}" -n "${namespace}" --context ${CLUSTER_CONTEXT} &>/dev/null; then
            log_info "Job '${job_name}' found, waiting for completion..."
            break
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
    done

    if [ $elapsed -ge $timeout ]; then
        log_warning "Job '${job_name}' not found after ${timeout}s"
        return 1
    fi

    # Calculate remaining timeout for completion wait
    local remaining_timeout=$((timeout - elapsed))
    if [ $remaining_timeout -lt 10 ]; then
        remaining_timeout=10
    fi

    # Now wait for job completion
    if kubectl wait --for=condition=Complete job/"${job_name}" -n "${namespace}" --context ${CLUSTER_CONTEXT} --timeout="${remaining_timeout}s" 2>/dev/null; then
        log_success "Job '${job_name}' completed successfully"
        return 0
    else
        log_warning "Job '${job_name}' did not complete in time"
        return 1
    fi
}

# Wait for a secret to exist
wait_for_secret() {
    local namespace=$1
    local secret_name=$2
    local timeout=${3:-120}
    local interval=5
    local elapsed=0

    log_info "Waiting for secret '${secret_name}' in ${namespace} (timeout: ${timeout}s)..."

    while [ $elapsed -lt $timeout ]; do
        if kubectl get secret "${secret_name}" -n "${namespace}" &>/dev/null; then
            log_success "Secret '${secret_name}' is ready"
            return 0
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
    done

    log_warning "Timeout waiting for secret '${secret_name}' in ${namespace}"
    return 1
}

# Copy cluster-gateway-ca certificate resources from control plane to a target plane namespace
create_plane_cert_resources() {
    local target_namespace=$1

    log_info "Creating certificate resources in ${target_namespace}..."

    # Create namespace if it doesn't exist
    kubectl create namespace "${target_namespace}" --dry-run=client -o yaml | kubectl apply -f - 2>/dev/null

    # Wait for cert-manager to issue the cluster-gateway CA
    log_info "Waiting for cluster-gateway-ca certificate to be ready..."
    if ! kubectl wait -n openchoreo-control-plane \
        --for=condition=Ready certificate/cluster-gateway-ca --timeout=120s 2>/dev/null; then
        log_warning "Timeout waiting for cluster-gateway-ca certificate"
        return 1
    fi

    # Get CA certificate from control plane secret
    CA_CRT=$(kubectl get secret cluster-gateway-ca \
        -n openchoreo-control-plane -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d)

    if [ -z "$CA_CRT" ]; then
        log_warning "Could not retrieve CA certificate from control plane"
        return 1
    fi

    # Create configmap in target namespace
    if kubectl create configmap cluster-gateway-ca \
        --from-literal=ca.crt="$CA_CRT" \
        -n "${target_namespace}" --dry-run=client -o yaml | kubectl apply -f - 2>/dev/null; then
        log_success "cluster-gateway-ca configmap created in ${target_namespace}"
    else
        log_error "Failed to create cluster-gateway-ca configmap in ${target_namespace}"
        return 1
    fi

    return 0
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
log_step "Step 1/13: Verifying prerequisites"

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

if ! command_exists lsof; then
    log_error "lsof is not installed"
    exit 1
fi

# Check if required ports are available (only when creating a new cluster)
if ! k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
    log_info "Checking required ports are available..."
    if ! check_required_ports; then
        exit 1
    fi
    log_success "All required ports are available"
fi

log_success "All prerequisites verified"

# ============================================================================
# Step 2: Setup k3d Cluster
# ============================================================================

log_step "Step 2/13: Setting up k3d cluster"

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
# Step 3: Apply CoreDNS Custom Configuration
# ============================================================================

log_step "Step 3/13: Applying CoreDNS Custom Configuration"

log_info "Applying CoreDNS custom configuration for OpenChoreo and AMP..."
COREDNS_FILE="https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/k8s/coredns-amp-custom.yaml"
if kubectl apply -f "${COREDNS_FILE}"; then
    log_success "CoreDNS custom configuration applied successfully"
else
    log_error "Failed to apply CoreDNS custom configuration"
    exit 1
fi

# ============================================================================
# Step 4: Generate Machine IDs for observability
# ============================================================================

log_step "Step 4/13: Generating Machine IDs for observability"

log_info "Generating Machine IDs for Fluent Bit observability..."
NODES=$(k3d node list -o json | grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/"name"[[:space:]]*:[[:space:]]*"//;s/"$//' | grep "^k3d-$CLUSTER_NAME-")
if [[ -z "$NODES" ]]; then
    log_error "Could not retrieve node list"
else
    for NODE in $NODES; do
        log_info "Generating machine ID for ${NODE}..."
        if docker exec ${NODE} sh -c "cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id" 2>/dev/null; then
            log_success "Machine ID generated for ${NODE}"
        else
            log_error "Could not generate Machine ID for ${NODE}"
        fi
    done
fi
log_success "Machine ID generation complete"

# ============================================================================
# Step 5: Install Cluster Prerequisites
# ============================================================================

log_step "Step 5/13: Installing Cluster Prerequisites (Cert Manager, Gateway API CRDs, External Secrets, kgateway)"

# Install Cert Manager
log_info "Installing Cert Manager..."
helm_install_idempotent \
    "cert-manager" \
    "oci://quay.io/jetstack/charts/cert-manager" \
    "cert-manager" \
    300 \
    --version v1.19.2 \
    --set crds.enabled=true

wait_for_pods "cert-manager" 300

# Install Gateway API CRDs
log_info "Installing Gateway API CRDs..."
GATEWAY_API_CRD="https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml"
if kubectl apply --server-side --force-conflicts -f "${GATEWAY_API_CRD}" &>/dev/null; then
    log_success "Gateway API CRDs applied successfully"
else
    log_error "Failed to apply Gateway API CRDs"
    exit 1
fi

# Install External Secrets Operator
log_info "Installing External Secret Operator..."
if helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
    --kube-context ${CLUSTER_CONTEXT} \
    --namespace external-secrets \
    --create-namespace \
    --version 1.3.2 \
    --set installCRDs=true \
    --timeout 180s &>/dev/null; then
    log_success "External Secret Operator installed successfully"
else
    log_error "Failed to install External Secret Operator"
    exit 1
fi

log_info "Waiting for External Secret Operator to be ready..."
if kubectl wait --for=condition=Available deployment/external-secrets -n external-secrets --context ${CLUSTER_CONTEXT} --timeout=180s 2>/dev/null; then
    log_success "External Secret Operator is ready"
else
    log_warning "External Secret Operator may still be starting (non-fatal)"
fi

wait_for_pods "external-secrets" 180

# Install kgateway CRDs
log_info "Installing kgateway CRDs..."
if helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
    --kube-context ${CLUSTER_CONTEXT} \
    --namespace openchoreo-control-plane \
    --create-namespace \
    --version v2.2.1 \
    --timeout 180s &>/dev/null; then
    log_success "kgateway CRDs installed successfully"
else
    log_error "Failed to install kgateway CRDs"
    exit 1
fi

# Install kgateway
log_info "Installing kgateway..."
if helm upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
    --kube-context ${CLUSTER_CONTEXT} \
    --namespace openchoreo-control-plane \
    --create-namespace \
    --version v2.2.1 \
    --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true \
    --timeout 180s &>/dev/null; then
    log_success "kgateway installed successfully"
else
    log_error "Failed to install kgateway"
    exit 1
fi

# ============================================================================
# Step 6: Setup Secrets (OpenBao for Workflow Plane)
# ============================================================================

log_step "Step 6/13: Setup Secrets (OpenBao for Workflow Plane)"

# Install OpenBao for Workflow Plane secret management
log_info "Installing OpenBao for Workflow Plane..."
if helm upgrade --install openbao oci://ghcr.io/openbao/charts/openbao \
    --kube-context ${CLUSTER_CONTEXT} \
    --namespace openbao \
    --create-namespace \
    --version 0.25.6 \
    --values "https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/single-cluster/values-openbao.yaml" \
    --timeout 180s &>/dev/null; then
    log_success "OpenBao installed successfully"
else
    log_error "Failed to install OpenBao"
    exit 1
fi

log_info "Waiting for OpenBao to be ready..."
if kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=openbao -n openbao --context ${CLUSTER_CONTEXT} --timeout=120s 2>/dev/null; then
    log_success "OpenBao is ready"
else
    log_warning "OpenBao may still be starting (non-fatal)"
fi

# Configure External Secrets ClusterSecretStore for OpenBao
log_info "Configuring External Secrets ClusterSecretStore for OpenBao..."
if kubectl --context ${CLUSTER_CONTEXT} apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: external-secrets-openbao
  namespace: openbao
---
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: default
spec:
  provider:
    vault:
      server: "http://openbao.openbao.svc:8200"
      path: "secret"
      version: "v2"
      auth:
        kubernetes:
          mountPath: "kubernetes"
          role: "openchoreo-secret-writer-role"
          serviceAccountRef:
            name: "external-secrets-openbao"
            namespace: "openbao"
EOF
then
    log_success "External Secrets ClusterSecretStore configured for OpenBao"
else
    log_error "Failed to configure ClusterSecretStore for OpenBao"
    exit 1
fi

# ============================================================================
# Step 7: Install OpenChoreo Control Plane
# ============================================================================

log_step "Step 7/13: Installing OpenChoreo Control Plane"

helm_install_idempotent \
    "openchoreo-control-plane" \
    "oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane" \
    "openchoreo-control-plane" \
    "${TIMEOUT_CONTROL_PLANE}" \
    --version "${OPENCHOREO_VERSION}" \
    --values "https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/single-cluster/values-cp.yaml"

wait_for_pods "openchoreo-control-plane" "${TIMEOUT_CONTROL_PLANE}"

# ============================================================================
# Step 8: Install OpenChoreo Data Plane
# ============================================================================

log_step "Step 8/13: Installing OpenChoreo Data Plane"

create_plane_cert_resources "openchoreo-data-plane"

helm_install_idempotent \
    "openchoreo-data-plane" \
    "oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane" \
    "${DATA_PLANE_NS}" \
    "${TIMEOUT_DATA_PLANE}" \
    --version "${OPENCHOREO_VERSION}" \
    --values "https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/single-cluster/values-dp.yaml"

# Register Data Plane with Control Plane
log_info "Registering Data Plane with Control Plane..."
wait_for_secret "openchoreo-data-plane" "cluster-agent-tls" 180
CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-data-plane -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d || echo "")

if [ -n "$CA_CERT" ]; then
    if kubectl apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterDataPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "default"
  clusterAgent:
    clientCA:
      value: |
$(echo "$CA_CERT" | sed 's/^/        /')
  gateway:
    ingress:
      external:
        name: gateway-default
        namespace: openchoreo-data-plane
        http:
          host: "openchoreoapis.localhost"
          listenerName: http
          port: 19080
        https:
          host: "openchoreoapis.localhost"
          listenerName: https
          port: 19443
  secretStoreRef:
    name: amp-openbao-store
EOF
    then
        log_success "Data Plane registered with Control Plane successfully"
    else
        log_warning "Failed to register Data Plane (non-fatal)"
    fi
else
    log_warning "CA certificate not found, skipping Data Plane registration"
fi

# Verify ClusterDataPlane resource
if kubectl get clusterdataplane default -n default &>/dev/null; then
    log_success "ClusterDataPlane resource 'default' exists"
else
    log_warning "ClusterDataPlane resource not found"
fi
wait_for_pods "openchoreo-data-plane" "${TIMEOUT_DATA_PLANE}"

# ============================================================================
# Step 9: Install OpenChoreo Workflow Plane
# ============================================================================

log_step "Step 9/13: Installing OpenChoreo Workflow Plane"

create_plane_cert_resources "openchoreo-workflow-plane"

# Install Docker Registry for Workflow Plane
log_info "Installing Docker Registry for Workflow Plane..."
if helm status registry -n openchoreo-workflow-plane &>/dev/null; then
    log_info "Docker Registry already installed, skipping..."
else
    if helm upgrade --install registry docker-registry \
        --repo https://twuni.github.io/docker-registry.helm \
        --namespace openchoreo-workflow-plane \
        --create-namespace \
        --values https://raw.githubusercontent.com/openchoreo/openchoreo/v1.0.0-rc.1/install/k3d/single-cluster/values-registry.yaml\
        --timeout 120s; then
        log_success "Docker Registry installed successfully"
    else
        log_error "Failed to install Docker Registry"
        exit 1
    fi
fi

log_info "Waiting for Docker Registry to be ready..."
if kubectl wait --for=condition=available deployment/registry -n openchoreo-workflow-plane --timeout=120s 2>/dev/null; then
    log_success "Docker Registry is ready"
else
    log_warning "Docker Registry may still be starting (non-fatal)"
fi

helm_install_idempotent \
    "openchoreo-workflow-plane" \
    "oci://ghcr.io/openchoreo/helm-charts/openchoreo-workflow-plane" \
    "${BUILD_CI_NS}" \
    "${TIMEOUT_BUILD_PLANE}" \
    --version "${OPENCHOREO_VERSION}"


# Register Workflow Plane with Control Plane
log_info "Registering Workflow Plane with Control Plane..."
wait_for_secret "openchoreo-workflow-plane" "cluster-agent-tls" 180
BP_CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-workflow-plane -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d || echo "")

if [ -n "$BP_CA_CERT" ]; then
    if kubectl apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterWorkflowPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "default"
  secretStoreRef:
    name: default
  clusterAgent:
    clientCA:
      value: |
$(echo "$BP_CA_CERT" | sed 's/^/        /')
EOF
    then
        log_success "Workflow Plane registered with Control Plane successfully"
    else
        log_warning "Failed to register Workflow Plane (non-fatal)"
    fi
else
    log_warning "Workflow Plane CA certificate not found, skipping Workflow Plane registration"
fi

# Verify ClusterWorkflowPlane resource
if kubectl get clusterworkflowplane default -n default &>/dev/null; then
    log_success "ClusterWorkflowPlane resource 'default' exists"
else
    log_warning "ClusterWorkflowPlane resource not found"
fi

wait_for_deployments "openchoreo-workflow-plane" "${TIMEOUT_BUILD_PLANE}"

# ============================================================================
# Step 10: Install OpenChoreo Observability Plane
# ============================================================================

log_step "Step 10/13: Installing OpenChoreo Observability Plane"

create_plane_cert_resources "openchoreo-observability-plane"

# Create ExternalSecret for OpenSearch credentials
log_info "Creating ExternalSecret for OpenSearch credentials..."
if kubectl apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: observer-opensearch-credentials
  namespace: openchoreo-observability-plane
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: observer-opensearch-credentials
  data:
  - secretKey: username
    remoteRef:
      key: opensearch-username
  - secretKey: password
    remoteRef:
      key: opensearch-password
EOF
then
    log_success "ExternalSecret for OpenSearch credentials created successfully"
else
    log_warning "Failed to create ExternalSecret for OpenSearch credentials (non-fatal)"
fi

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
CONFIGMAP_FILE="https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/values/oc-collector-configmap.yaml"

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
    --set observer.extraEnv.AUTH_SERVER_BASE_URL=http://thunder-service.openchoreo-control-plane.svc.cluster.local:8090 \
    --values "https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/single-cluster/values-op.yaml"

# Install Observability Modules
log_info "Installing Observability Modules..."

# Create ExternalSecret for OpenSearch admin credentials
log_info "Creating ExternalSecret for OpenSearch admin credentials..."
if kubectl apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: opensearch-admin-credentials
  namespace: openchoreo-observability-plane
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: opensearch-admin-credentials
  data:
  - secretKey: username
    remoteRef:
      key: opensearch-username
      property: value
  - secretKey: password
    remoteRef:
      key: opensearch-password
      property: value
EOF
then
    log_success "ExternalSecret for OpenSearch admin credentials created successfully"
else
    log_warning "Failed to create ExternalSecret for OpenSearch admin credentials (non-fatal)"
fi

# Create ExternalSecret for observer service
log_info "Creating ExternalSecret for observer service..."
if kubectl apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: observer-secret
  namespace: openchoreo-observability-plane
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: observer-secret
  data:
  - secretKey: OPENSEARCH_USERNAME
    remoteRef:
      key: opensearch-username
      property: value
  - secretKey: OPENSEARCH_PASSWORD
    remoteRef:
      key: opensearch-password
      property: value
  - secretKey: UID_RESOLVER_OAUTH_CLIENT_SECRET
    remoteRef:
      key: observer-oauth-client-secret
      property: value
EOF
then
    log_success "ExternalSecret for observer service created successfully"
else
    log_warning "Failed to create ExternalSecret for observer service (non-fatal)"
fi

wait_for_secret "openchoreo-observability-plane" "opensearch-admin-credentials" 180
wait_for_secret "openchoreo-observability-plane" "observer-secret" 180
# Install observability-logs-opensearch
log_info "Installing observability-logs-opensearch..."
if helm upgrade --install observability-logs-opensearch \
    oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
    --create-namespace \
    --namespace openchoreo-observability-plane \
    --version 0.3.8 \
    --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
    --timeout 10m; then
    log_success "observability-logs-opensearch installed successfully"
else
    log_error "Failed to install observability-logs-opensearch (non-fatal)"
fi

# Enable log collection with fluent-bit
log_info "Enabling log collection with fluent-bit..."
if helm upgrade observability-logs-opensearch \
    oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
    --namespace openchoreo-observability-plane \
    --version 0.3.8 \
    --reuse-values \
    --set fluent-bit.enabled=true \
    --timeout 10m; then
    log_success "Log collection enabled with fluent-bit"
else
    log_error "Failed to enable log collection (non-fatal)"
fi

# Install observability-metrics-prometheus
log_info "Installing observability-metrics-prometheus..."
if helm upgrade --install observability-metrics-prometheus \
    oci://ghcr.io/openchoreo/helm-charts/observability-metrics-prometheus \
    --create-namespace \
    --namespace openchoreo-observability-plane \
    --version 0.2.4 \
    --timeout 10m; then
    log_success "observability-metrics-prometheus installed successfully"
else
    log_error "Failed to install observability-metrics-prometheus (non-fatal)"
fi

# Install observability-traces-opensearch
log_info "Enabling opensearch based tracing module..."
if helm upgrade --install observability-traces-opensearch \
    oci://ghcr.io/openchoreo/helm-charts/observability-tracing-opensearch \
    --create-namespace \
    --namespace openchoreo-observability-plane \
    --version 0.3.7 \
    --set openSearch.enabled=false \
    --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
    --set opentelemetry-collector.configMap.existingName="amp-opentelemetry-collector-config" \
    --timeout 10m; then
    log_success "OpenSearch based tracing module installed"
else
    log_error "Failed to install opensearch based tracing module (non-fatal)"
fi

# Register Observability Plane with Control Plane
log_info "Registering Observability Plane with Control Plane..."
wait_for_secret "openchoreo-observability-plane" "cluster-agent-tls" 180
OP_CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-observability-plane -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d || echo "")

if [ -n "$OP_CA_CERT" ]; then
    if kubectl apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterObservabilityPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "default"
  clusterAgent:
    clientCA:
      value: |
$(echo "$OP_CA_CERT" | sed 's/^/        /')
  observerURL: http://observer.openchoreo.localhost:11080
EOF
    then
        log_success "Observability Plane registered with Control Plane successfully"
    else
        log_warning "Failed to register Observability Plane (non-fatal)"
    fi
else
    log_warning "Observability Plane CA certificate not found, skipping Observability Plane registration"
fi

wait_for_deployments "openchoreo-observability-plane" "${TIMEOUT_OBSERVABILITY_PLANE}"
wait_for_statefulsets "openchoreo-observability-plane" "${TIMEOUT_OBSERVABILITY_PLANE}"

# Configure observability integration
log_info "Configuring observability integration..."

# Configure ClusterDataPlane observer
if kubectl get clusterdataplane default -n default &>/dev/null; then
    if kubectl patch clusterdataplane default -n default --type merge \
        -p '{"spec":{"observabilityPlaneRef":{"kind":"ClusterObservabilityPlane","name":"default"}}}' &>/dev/null; then
        log_success "ClusterDataPlane observability plane reference configured"
    else
        log_warning "ClusterDataPlane observability plane configuration failed (non-fatal)"
    fi
else
    log_warning "ClusterDataPlane resource not found yet (will use default observer)"
fi

# Configure ClusterWorkflowPlane observer
if kubectl get clusterworkflowplane default -n default &>/dev/null; then
    if kubectl patch clusterworkflowplane default -n default --type merge \
        -p '{"spec":{"observabilityPlaneRef":{"kind":"ClusterObservabilityPlane","name":"default"}}}' &>/dev/null; then
        log_success "ClusterWorkflowPlane observability plane reference configured"
    else
        log_warning "ClusterWorkflowPlane observability plane configuration failed (non-fatal)"
    fi
else
    log_warning "ClusterWorkflowPlane resource not found yet (will use default observer)"
fi

# ============================================================================
# Step 11: Install Gateway Operator
# ============================================================================


log_step "Step 11/13: Installing Gateway Operator"
log_info "Installing Gateway Operator..."
helm_install_idempotent \
    "gateway-operator" \
    "oci://ghcr.io/wso2/api-platform/helm-charts/gateway-operator" \
    "openchoreo-data-plane" \
    "600" \
    --version "0.4.0" \
    --set "logging.level=debug" \
    --set "gateway.helm.chartVersion=1.0.0"

log_success "Gateway Operator installed"

# Apply Gateway Operator Configuration
log_info "Applying Gateway Operator Configuration..."
GATEWAY_CONFIG_FILE="https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/values/api-platform-operator-full-config.yaml"

if kubectl apply -f "${GATEWAY_CONFIG_FILE}" &>/dev/null; then
    log_success "Gateway Operator configuration applied successfully"
else
    log_error "Failed to apply Gateway Operator configuration"
    log_info "Attempting to download and apply locally..."
    if curl -sSL "${GATEWAY_CONFIG_FILE}" | kubectl apply -f - &>/dev/null; then
        log_success "Gateway Operator configuration applied successfully"
    else
        log_warning "Failed to apply Gateway Operator configuration (non-fatal)"
    fi
fi

# Grant RBAC for WSO2 API Platform CRDs
log_info "Granting RBAC for WSO2 API Platform CRDs..."
if kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: wso2-api-platform-gateway-module
rules:
  - apiGroups: ["gateway.api-platform.wso2.com"]
    resources: ["restapis", "apigateways"]
    verbs: ["*"]
  - apiGroups: ["gateway.kgateway.dev"]
    resources: ["backends"]
    verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: wso2-api-platform-gateway-module
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: wso2-api-platform-gateway-module
subjects:
  - kind: ServiceAccount
    name: cluster-agent-dataplane
    namespace: openchoreo-data-plane
EOF
then
    log_success "RBAC for WSO2 API Platform CRDs applied"
else
    log_warning "Failed to apply RBAC for WSO2 API Platform CRDs (non-fatal)"
fi

# Apply Gateway and API Resources
log_info "Applying Gateway and API Resources..."

# Apply Gateway
GATEWAY_FILE="https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/values/obs-gateway.yaml"
if kubectl apply -f "${GATEWAY_FILE}" &>/dev/null; then
    log_success "Gateway resource applied"
else
    log_warning "Failed to apply Gateway resource (non-fatal)"
fi

# Wait for Gateway to be ready
log_info "Waiting for Gateway to be programmed..."
if kubectl wait --for=condition=Programmed apigateway/obs-gateway -n openchoreo-data-plane --timeout=180s 2>/dev/null; then
    log_success "Gateway is programmed"
else
    log_warning "Gateway did not become ready in time (non-fatal)"
fi

# Apply RestApi
RESTAPI_FILE="https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/values/otel-collector-rest-api.yaml"
if kubectl apply -f "${RESTAPI_FILE}" &>/dev/null; then
    log_success "RestApi resource applied"
else
    log_warning "Failed to apply RestApi resource (non-fatal)"
fi

# Wait for RestApi to be ready
log_info "Waiting for RestApi to be programmed..."
if kubectl wait --for=condition=Programmed restapi/traces-api-secure -n openchoreo-data-plane --timeout=120s 2>/dev/null; then
    log_success "RestApi is programmed"
else
    log_warning "RestApi did not become ready in time (non-fatal)"
fi

log_success "Gateway Operator setup complete"

# ============================================================================
# Step 12: Install AMP Thunder Extension
# ============================================================================

log_step "Step 12/13: Installing WSO2 AMP Thunder Extension"

log_info "Installing WSO2 AMP Thunder Extension..."
log_info "Gateway API CRDs and Gateway Operator are now available"
if ! install_amp_thunder_extension; then
    log_warning "AMP Thunder Extension installation failed (non-fatal)"
    echo "The installation will continue but thunder extension features may not work."
    echo ""
    echo "Troubleshooting steps:"
    echo "  1. Check Helm release: helm list -n amp-thunder"
    echo "  2. Check pod status: kubectl get pods -n amp-thunder"
else
    log_success "AMP Thunder Extension installed successfully"
fi
echo ""

# ============================================================================
# Step 13: Install Agent Management Platform
# ============================================================================

log_step "Step 13/13: Installing Agent Management Platform"

# Verify prerequisites
if ! verify_amp_prerequisites; then
    log_error "AMP prerequisites check failed"
    exit 1
fi

log_info "Installing Agent Management Platform components..."
log_info "This may take 5-8 minutes..."
echo ""

# Install secrets extension (OpenBao) FIRST so AMP can connect to it
# Enable dev mode for quickstart
SECRETS_HELM_ARGS+=(
    "--set" "openbao.server.dev.enabled=true"
)
log_info "Installing Secrets Extension (OpenBao for secret management)..."
if ! install_secrets_extension; then
    log_warning "Secrets Extension installation failed (non-fatal)"
    echo "The platform is installed but secret management features may not work."
else
    log_success "Secrets Extension installed successfully"
fi
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

# Install evaluation extension
log_info "Installing Evaluation Extension (Monitor Evaluation Workflows)..."
if ! install_evaluation_extension; then
    log_warning "Evaluation Extension installation failed (non-fatal)"
    echo "The platform is installed but evaluation features may not work."
    echo ""
    echo "Troubleshooting steps:"
    echo "  1. Check Helm release: helm list -n ${EVALUATION_NS}"
else
    log_success "Evaluation Extension installed successfully"
fi
echo ""

# Install gateway extension
# Must run after:
#   - Agent Management Platform (amp-api service must be healthy)
#   - Thunder Extension (IDP must be ready for client_credentials token exchange)
#   - Gateway Operator (must be running to consume the APIGateway CR)
log_info "Installing Gateway Extension (AI Gateway registration + APIGateway CR)..."
if ! install_gateway_extension; then
    log_warning "Gateway Extension installation failed (non-fatal)"
    echo "The platform is installed but the AI gateway may not be registered."
    echo ""
    echo "Troubleshooting steps:"
    echo "  1. Check bootstrap job: kubectl get jobs -n ${GATEWAY_NS}"
    echo "  2. Check bootstrap logs: kubectl logs -n ${GATEWAY_NS} -l app.kubernetes.io/component=gateway-bootstrap"
    echo "  3. Check APIGateway CR: kubectl get apigateway ai-gateway -n openchoreo-data-plane"
    echo "  4. Check Helm release: helm list -n ${GATEWAY_NS}"
else
    log_success "Gateway Extension installed successfully"
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
log_info "Observability Gateway (for traces): http://localhost:22893/otel"
echo ""
echo ""
log_info "To check status: kubectl get pods -A"
log_info "To Uninstall: ./uninstall.sh"
log_info "To delete cluster: ./uninstall.sh --delete-cluster"
echo ""

