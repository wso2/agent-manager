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
OPENCHOREO_VERSION="1.1.1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K3D_CONFIG="${K3D_CONFIG:-${SCRIPT_DIR}/k3d-config.yaml}"

# Directory holding the deployment resources this installer applies
# (single-cluster/, values/, k8s/, scripts/). These used to be fetched from
# raw.githubusercontent.com at install time, but GitHub rate-limits
# unauthenticated requests per client IP and returns HTTP 429 once the
# per-IP budget is spent (common behind shared/corporate NAT). helm --values
# <url> and kubectl apply -f <url> do not retry on 429, so a single throttled
# response aborts the install. Resolve the bundled copies on disk instead:
# in a repo checkout they live one level up from quick-start/; the dev-container
# image bundles the same tree flat alongside the scripts.
if [[ -d "${SCRIPT_DIR}/../single-cluster" ]]; then
    DEPLOYMENTS_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
else
    DEPLOYMENTS_DIR="${SCRIPT_DIR}"
fi

# WSO2 API Platform / Gateway Operator versions
GATEWAY_OPERATOR_VERSION="0.10.1"
# gateway-controller/gateway-runtime 1.1.x reject every RestApi/LlmProvider
# deployment with a bare 404 despite correctly-configured basic auth (confirmed
# via a live tcpdump of the operator's request — same 404 across 1.1.0 and
# 1.1.5). 1.2.0-alpha2 does not have this bug (verified end-to-end: RestApi
# reaches Programmed=True). Chart *version* and container *image tag* are
# pinned separately below — setting chartVersion alone still runs an older
# default image, so GATEWAY_IMAGE_VERSION must also be threaded through.
GATEWAY_CHART_VERSION="1.2.0-beta"
GATEWAY_IMAGE_VERSION="1.2.0-beta"

# The 1.2.0-beta gateway chart requires an encryption key to be mounted from a Kubernetes Secret
GATEWAY_ENCRYPTION_SECRET_NAME="gateway-encryption-keys"
GATEWAY_ENCRYPTION_SECRET_KEY="default-aesgcm256-v1.bin"

# OpenChoreo community module versions compatible with OpenChoreo ${OPENCHOREO_VERSION}
OBSERVABILITY_LOGS_OPENSEARCH_VERSION="0.4.1"
OBSERVABILITY_TRACING_OPENSEARCH_VERSION="0.4.1"
OBSERVABILITY_METRICS_PROMETHEUS_VERSION="0.6.1"

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
    # Required ports for k3d cluster (host:container mapping).
    # AMP Console/API ride the control-plane gateway (8080) and the Traces
    # Observer rides the observability-plane gateway (11080); OTLP ingest
    # rides the data-plane gateway (19080) — none need their own host port.
    # 8080  - Control Plane Gateway HTTP (Console, API, Thunder auth, OpenChoreo API,
    #         external AI gateway control plane)
    # 8443  - Control Plane Gateway HTTPS
    # 10082 - Container Registry (Workflow Plane)
    # 11080 - Observability Plane Gateway HTTP (Agent Manager Observer, Observer API)
    # 11082 - OpenSearch API
    # 11085 - OpenSearch HTTPS
    # 19080 - Data Plane Gateway HTTP (agent workloads, OTLP /otel)
    # 19443 - Data Plane Gateway HTTPS
    local required_ports=(8080 8443 10082 11080 11082 11085 19080 19443)
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

# Verify Docker is pointing at the dedicated 'agent-manager' Colima profile.
# Detection works from inside the dev container because `docker info` queries
# the daemon: Colima names the VM node after its profile ('colima' for the
# default profile, 'colima-<profile>' otherwise). Non-Colima Docker setups
# (Docker Desktop, Rancher, Linux) fall through this check.
EXPECTED_COLIMA_PROFILE="agent-manager"
check_colima_profile() {
    local node_name
    node_name=$(docker info --format '{{.Name}}' 2>/dev/null || echo "")

    # Non-Colima daemon -> nothing to enforce
    case "${node_name}" in
        colima|colima-*) ;;
        *) return 0 ;;
    esac

    local expected="colima-${EXPECTED_COLIMA_PROFILE}"
    if [ "${node_name}" = "${expected}" ]; then
        log_success "Colima profile '${EXPECTED_COLIMA_PROFILE}' is active"
        return 0
    fi

    log_error "Expected Colima profile '${EXPECTED_COLIMA_PROFILE}', got '${node_name}'."
    log_info  "Start it with: colima start --profile ${EXPECTED_COLIMA_PROFILE} --vm-type=vz --vz-rosetta --network-address --cpu 4 --memory 8"
    return 1
}

# Check and fix Docker permissions
check_docker_permissions() {
    # Try docker ps first — Docker contexts (e.g., Colima profiles) route to
    # the correct socket automatically, so we don't need to check a hardcoded path.
    if docker ps &>/dev/null; then
        log_success "Docker access verified"
        check_colima_profile || return 1
        return 0
    fi

    # docker ps failed — check common socket locations for a helpful error message.
    local docker_sock="/var/run/docker.sock"
    if [ ! -S "${docker_sock}" ]; then
        log_error "Docker is not accessible and socket not found at ${docker_sock}"
        log_info "Make sure Docker is running. If using Colima, ensure the correct profile is started."
        return 1
    fi

    # Socket exists but docker ps failed — likely a permissions issue.
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
# COREDNS_FILE is overridable (env) so the VM installer can substitute a config
# that rewrites the in-cluster names to the k3d server node instead of
# host.k3d.internal — required once host ports are loopback-bound. See
# deployments/vm/lib-vm.sh:render_coredns_vm_config.
COREDNS_FILE="${COREDNS_FILE:-${DEPLOYMENTS_DIR}/k8s/coredns-amp-custom.yaml}"
if kubectl apply -f "${COREDNS_FILE}"; then
    log_success "CoreDNS custom configuration applied successfully"
else
    log_error "Failed to apply CoreDNS custom configuration"
    exit 1
fi

# CoreDNS's reload plugin can miss the override files if the configmap is mounted
# after the pod's initial parse. Restart CoreDNS so the rewrite rules take effect
# before any client caches a wrong resolution.
log_info "Restarting CoreDNS to load rewrite rules..."
if kubectl rollout restart deployment/coredns -n kube-system && \
   kubectl rollout status deployment/coredns -n kube-system --timeout=60s; then
    log_success "CoreDNS restarted and ready"
else
    log_error "Failed to restart CoreDNS"
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
    --values "${DEPLOYMENTS_DIR}/single-cluster/values-openbao.yaml" \
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

if [[ "$(helm status "openchoreo-control-plane" -n "openchoreo-control-plane" -o json 2>/dev/null | grep -o '"status":"[^"]*"' | grep -o '[^"]*$')" == "deployed" ]]; then
    log_info "openchoreo-control-plane already installed, skipping..."
else
    log_info "Installing openchoreo-control-plane..."
    log_info "This may take several minutes..."
    CP_INSTALL_OUTPUT=""
    if ! CP_INSTALL_OUTPUT=$(helm install "openchoreo-control-plane" \
        "oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane" \
        --namespace "openchoreo-control-plane" \
        --create-namespace \
        --timeout "${TIMEOUT_CONTROL_PLANE}s" \
        --version "${OPENCHOREO_VERSION}" \
        --values "${DEPLOYMENTS_DIR}/single-cluster/values-cp.yaml" \
        "${CP_HELM_ARGS[@]}" 2>&1); then
        echo "$CP_INSTALL_OUTPUT"
        if echo "$CP_INSTALL_OUTPUT" | grep -q "no endpoints available for service \"controller-manager-webhook-service\""; then
            log_warning "Control Plane webhook was not ready. Waiting for deployments and retrying once..."
            kubectl wait -n openchoreo-control-plane --for=condition=available --timeout=300s deployment --all || true
            helm upgrade --install "openchoreo-control-plane" \
                "oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane" \
                --namespace "openchoreo-control-plane" \
                --create-namespace \
                --timeout "${TIMEOUT_CONTROL_PLANE}s" \
                --version "${OPENCHOREO_VERSION}" \
                --values "${DEPLOYMENTS_DIR}/single-cluster/values-cp.yaml" \
                "${CP_HELM_ARGS[@]}" || { log_error "Failed to install openchoreo-control-plane after retry"; exit 1; }
        else
            log_error "Failed to install openchoreo-control-plane"
            exit 1
        fi
    fi
    log_success "openchoreo-control-plane installed successfully"
fi

wait_for_pods "openchoreo-control-plane" "${TIMEOUT_CONTROL_PLANE}"

# ----------------------------------------------------------------------------
# ThunderID v0.45 identity migration (sub -> client_id) for the control plane.
# v0.45 client_credentials tokens carry the client identity in 'client_id'
# (sub is now a random UUID). OpenChoreo still resolves service accounts by
# 'claim: sub', so without these patches every service-account call returns 403
# (project/agent create) and environment creation fails. The chart schema does
# not expose the claim, so patch the configmap via server-side apply (Apply op,
# helm field manager) and migrate OpenChoreo's built-in ClusterAuthzRoleBindings.
# ----------------------------------------------------------------------------
log_info "Patching openchoreo-api-config: service_account entitlement claim -> client_id..."
if kubectl get configmap openchoreo-api-config -n openchoreo-control-plane &>/dev/null; then
    patched_yaml=$(kubectl get configmap openchoreo-api-config -n openchoreo-control-plane -o yaml \
        | sed -E "s/claim:[[:space:]]*['\"]?sub['\"]?/claim: client_id/g")
    if echo "$patched_yaml" | grep -q "claim: client_id"; then
        echo "$patched_yaml" | kubectl apply --server-side --field-manager=helm --force-conflicts -f -
        kubectl rollout restart deployment/openchoreo-api -n openchoreo-control-plane
        kubectl rollout status deployment/openchoreo-api -n openchoreo-control-plane --timeout=120s
        log_success "openchoreo-api-config patched (client_id claim)"
    else
        log_error "Failed to patch openchoreo-api-config entitlement claim to client_id"
        exit 1
    fi
else
    log_warning "openchoreo-api-config not found - skipping claim patch"
fi

log_info "Migrating service-account ClusterAuthzRoleBindings: claim sub -> client_id..."
for binding in $(kubectl get clusterauthzrolebindings.openchoreo.dev \
    -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true); do
    claim=$(kubectl get clusterauthzrolebinding.openchoreo.dev "$binding" \
        -o jsonpath='{.spec.entitlement.claim}' 2>/dev/null || true)
    if [ "$claim" = "sub" ]; then
        if kubectl patch clusterauthzrolebinding.openchoreo.dev "$binding" \
            --type=merge -p '{"spec":{"entitlement":{"claim":"client_id"}}}' >/dev/null 2>&1; then
            log_info "  migrated ${binding}"
        else
            log_warning "  failed to migrate ClusterAuthzRoleBinding '${binding}'"
        fi
    fi
done
log_success "Service-account bindings migrated to client_id"

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
    --values "${DEPLOYMENTS_DIR}/single-cluster/values-dp.yaml"

# Register Data Plane with Control Plane
log_info "Registering Data Plane with Control Plane..."
# Wait for the cert-manager Certificate to be Ready (not just the Secret) to avoid a race
# where cert-manager re-issues the cert after we read it but before the agent connects.
log_info "Waiting for data plane agent certificate to be ready..."
if ! kubectl wait -n openchoreo-data-plane \
    --for=condition=Ready certificate/cluster-agent-dataplane-tls --timeout=180s; then
    log_error "Data plane agent certificate not ready. Cannot register data plane."
    exit 1
fi
CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-data-plane -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d || echo "")

# Data plane gateway external ingress (advertised host/port for deployed-agent
# endpoints). Overridable so the VM installer can advertise a public sslip.io host
# fronted by Caddy instead of the local amp.localhost:19080. The default
# keeps the local-install behaviour. See deployments/quick-start/vm/lib-vm.sh:
# render_dataplane_external_ingress.
DP_EXTERNAL_INGRESS="${DP_EXTERNAL_INGRESS:-$(cat <<'EOB'
        http:
          host: "agentmanager.localhost"
          listenerName: http
          port: 19080
        https:
          host: "agentmanager.localhost"
          listenerName: https
          port: 19443
EOB
)}"

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
        name: ${DP_EXTERNAL_GATEWAY_NAME:-gateway-default}
        namespace: ${DP_EXTERNAL_GATEWAY_NAMESPACE:-openchoreo-data-plane}
${DP_EXTERNAL_INGRESS}
  secretStoreRef:
    name: default
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
    # The registry values live in the upstream OpenChoreo repo, not ours, so we
    # can't bundle them alongside the installer. helm --values <url> does not
    # retry on GitHub's per-IP 429, so pre-download the file with backoff and
    # hand helm the local path instead of letting helm fetch it.
    registry_values="$(mktemp)"
    registry_values_url="https://raw.githubusercontent.com/openchoreo/openchoreo/v${OPENCHOREO_VERSION}/install/k3d/single-cluster/values-registry.yaml"
    if ! curl -fsSL --retry 5 --retry-all-errors --retry-delay 2 --connect-timeout 30 \
        "${registry_values_url}" -o "${registry_values}"; then
        rm -f "${registry_values}"
        log_error "Failed to download registry values from ${registry_values_url}"
        exit 1
    fi
    if helm upgrade --install registry docker-registry \
        --repo https://twuni.github.io/docker-registry.helm \
        --namespace openchoreo-workflow-plane \
        --create-namespace \
        --values "${registry_values}" \
        --timeout 120s; then
        rm -f "${registry_values}"
        log_success "Docker Registry installed successfully"
    else
        rm -f "${registry_values}"
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
CONFIGMAP_FILE="${DEPLOYMENTS_DIR}/values/oc-collector-configmap.yaml"

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
    --values "${DEPLOYMENTS_DIR}/single-cluster/values-op.yaml"

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
    --version "${OBSERVABILITY_LOGS_OPENSEARCH_VERSION}" \
    --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
    --set adapter.openSearchSecretName="opensearch-admin-credentials" \
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
    --version "${OBSERVABILITY_LOGS_OPENSEARCH_VERSION}" \
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
    --version "${OBSERVABILITY_METRICS_PROMETHEUS_VERSION}" \
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
    --version "${OBSERVABILITY_TRACING_OPENSEARCH_VERSION}" \
    --set openSearch.enabled=false \
    --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
    --set opentelemetry-collector.configMap.existingName="amp-opentelemetry-collector-config" \
    --timeout 10m; then
    log_success "OpenSearch based tracing module installed"
else
    log_error "Failed to install opensearch based tracing module (non-fatal)"
fi

# ThunderID v0.45: the observer resolves service accounts from its own config
# (observer-auth-config), still keyed on 'claim: sub'. Patch to client_id, else
# agent build-log / observability queries return 403.
log_info "Patching observer-auth-config: service_account entitlement claim -> client_id..."
if kubectl get configmap observer-auth-config -n openchoreo-observability-plane &>/dev/null; then
    patched_obs_yaml=$(kubectl get configmap observer-auth-config -n openchoreo-observability-plane -o yaml \
        | sed -E "s/claim:[[:space:]]*['\"]?sub['\"]?/claim: client_id/g")
    if echo "$patched_obs_yaml" | grep -q "claim: client_id"; then
        echo "$patched_obs_yaml" | kubectl apply --server-side --field-manager=helm --force-conflicts -f -
        kubectl rollout restart deployment/observer -n openchoreo-observability-plane
        kubectl rollout status deployment/observer -n openchoreo-observability-plane --timeout=120s
        log_success "observer-auth-config patched (client_id claim)"
    else
        log_warning "observer-auth-config has no 'sub' claim to patch - skipping"
    fi
else
    log_warning "observer-auth-config not found - skipping observer claim patch"
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
# Pinning gateway.helm.chartVersion alone is NOT enough: the chart's own
# default values.yaml can still reference an older gateway-controller/
# gateway-runtime image tag independent of the chart version, so the image
# tag/repo must be set explicitly here too (confirmed by inspecting the
# actual running pod images — chartVersion=1.2.0-alpha alone still ran an
# older image until these were added).
# Admin API on this image is served under /api/admin/v1 on the "admin" named
# port — GET /health on the "rest" port goes through Gin + basic auth and
# returns 401 for kube-probe (see gateway-controller admin OpenAPI spec).
helm_install_idempotent \
    "gateway-operator" \
    "oci://ghcr.io/wso2/api-platform/helm-charts/gateway-operator" \
    "openchoreo-data-plane" \
    "600" \
    --version "${GATEWAY_OPERATOR_VERSION}" \
    --set "logging.level=debug" \
    --set gatewayApi.installStandardCRDs=false \
    --set "gateway.helm.chartVersion=${GATEWAY_CHART_VERSION}" \
    --set "gateway.values.gateway.controller.image.tag=${GATEWAY_IMAGE_VERSION}" \
    --set gateway.values.gateway.controller.image.repository=ghcr.io/wso2/api-platform/gateway-controller \
    --set "gateway.values.gateway.gatewayRuntime.image.tag=${GATEWAY_IMAGE_VERSION}" \
    --set gateway.values.gateway.gatewayRuntime.image.repository=ghcr.io/wso2/api-platform/gateway-runtime \
    --set gateway.values.gateway.controller.encryptionKeys.enabled=true \
    --set "gateway.values.gateway.controller.encryptionKeys.secretName=${GATEWAY_ENCRYPTION_SECRET_NAME}" \
    --set gateway.values.gateway.controller.deployment.livenessProbe.httpGet.path=/api/admin/v1/health \
    --set gateway.values.gateway.controller.deployment.livenessProbe.httpGet.port=admin \
    --set gateway.values.gateway.controller.deployment.readinessProbe.httpGet.path=/api/admin/v1/health \
    --set gateway.values.gateway.controller.deployment.readinessProbe.httpGet.port=admin

log_success "Gateway Operator installed"

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

# Install agent sandbox module (required: agents run as sandboxed pods)
log_info "Installing Agent Sandbox Module (sandboxed agent runtime)..."
if ! install_agent_sandbox_module; then
    log_error "Failed to install Agent Sandbox Module"
    echo ""
    echo "Agent deployments cannot work without the Agent Sandbox module."
    echo ""
    echo "Troubleshooting steps:"
    echo "  1. Check Helm release: helm list -n ${DATA_PLANE_NS}"
    echo "  2. Check controller: kubectl get pods -n agent-sandbox-system"
    echo "  3. Check CRDs: kubectl get crd | grep agents.x-k8s.io"
    exit 1
fi
log_success "Agent Sandbox Module installed successfully"
echo ""

# Install platform resources extension
log_info "Installing Platform Resources Extension (Default Organization, Project, Environment, DeploymentPipeline)..."
if ! install_platform_resources_extension; then
    log_warning "Platform Resources Extension installation failed (non-fatal)"
    echo "The platform is installed but platform resources features may not work."
fi

log_success "Platform Resources Extension installed successfully"
echo ""

# Provision per-environment Thunder instance for the default environment.
# Mirrors setup-openchoreo.sh:install_default_env_thunder().
# Must run after the default environment exists (created by platform-resources above).
# The wait for the platform Thunder TLS cert is handled internally by add-environment-thunder.sh.

log_info "Provisioning Thunder identity provider for the default environment..."
ENV_THUNDER_PROVISIONED=false
# Fatal: without env-Thunder the gateway extension silently skips its
# ThunderKeyManager identity provider (install_gateway_extension only sets
# bootstrap.identityProviders when this release exists), so the gateway keeps a
# keymanager that Agent Manager has no mirror row for. That drift surfaces much
# later as an empty Identity Providers page, long after the installer claimed
# success — fail here instead, while the cause is still on screen.
if ! install_default_env_thunder; then
    log_error "Default environment Thunder provisioning failed"
    echo "Re-run manually once the platform is ready:"
    echo "  ENV_NAME=default DISPLAY_NAME=Default ORG_NAME=default \\"
    echo "  AMP_API_URL=${AMP_API_URL:-http://api.amp.localhost:8080/api/v1} \\"
    echo "  IDP_TOKEN_URL=${IDP_TOKEN_URL:-http://thunder.amp.localhost:8080/oauth2/token} \\"
    # install_default_env_thunder() prefers the bundled script and falls back to
    # downloading it; a standalone (curl-piped) install has no scripts/ directory,
    # so pointing at the bundled path there would send the operator to a file that
    # does not exist. Name the release copy instead, and pass SCRIPT_BASE_URL so
    # the script fetches its own siblings from that same release rather than main.
    env_thunder_script="${DEPLOYMENTS_DIR}/scripts/add-environment-thunder.sh"
    if [[ -f "${env_thunder_script}" ]]; then
        echo "  bash ${env_thunder_script}"
    else
        env_thunder_base="https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/scripts"
        echo "  SCRIPT_BASE_URL=${env_thunder_base} \\"
        echo "  bash <(curl -fsSL ${env_thunder_base}/add-environment-thunder.sh)"
    fi
    exit 1
fi
log_success "Default environment Thunder identity provider provisioned"
ENV_THUNDER_PROVISIONED=true
echo ""

# Install observability extension
log_info "Installing Observability Extension (Agent Manager Observer)..."
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

# Install API Platform Gateway Extension
# Must run after:
#   - Agent Management Platform (amp-api service must be healthy)
#   - Thunder Extension (IDP must be ready for client_credentials token exchange)
#   - Gateway Operator (must be running to consume the APIGateway CR)
log_info "Installing API Platform Gateway Extension (gateway registration + APIGateway CR)..."
if ! install_gateway_extension; then
    log_warning "Gateway Extension installation failed (non-fatal)"
    echo "The platform is installed but the API Platform Gateway may not be registered."
    echo ""
    echo "Troubleshooting steps (the gateway stack lives in its per-org-env namespace):"
    echo "  1. Check bootstrap job: kubectl get jobs -n default-default"
    echo "  2. Check bootstrap logs: kubectl logs -n default-default -l app.kubernetes.io/component=gateway-bootstrap"
    echo "  3. Check APIGateway CR: kubectl get apigateway api-platform-default-default -n default-default"
    echo "  4. Check Helm release: helm list -n default-default"
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
# Localhost URLs apply to the local quick-start. Wrappers that front the cluster
# with a reverse proxy (e.g. the VM installer) set SHOW_LOCALHOST_URLS=false and
# print their own reachable URLs instead.
if [[ "${SHOW_LOCALHOST_URLS:-true}" == "true" ]]; then
  log_info "Agent Management Platform Console: http://console.amp.localhost:8080"
  log_info "Observability Gateway (for traces): http://default-default.gateway.localhost:19080/otel"
fi

# Print the default environment's Thunder ID console + admin credentials — the
# release/namespace/host follow the fixed ENV_NAME=default ORG_NAME=default naming
# convention (see thunder_release_name/thunder_namespace/thunder_host in
# deployments/scripts/thunder-naming.sh). The password lives only in a K8s Secret (never written
# to disk by add-environment-thunder.sh), so it's fetched fresh here rather than
# re-printed from that script's own (long since scrolled-past) console output.
if [[ "${ENV_THUNDER_PROVISIONED}" == "true" ]]; then
  # Dynamically derive the coordinates using the shared naming helper.
  # Avoids hardcoded literals which would break if custom org/env names are used
  # or if the derived names are hash-truncated (natural names > 53 chars).
  default_org="default"
  default_env="default"
  default_release="$(thunder_release_name "${default_org}" "${default_env}")"
  default_ns="${default_release}"

  ENV_THUNDER_ADMIN_PASSWORD="$(kubectl get secret "${default_release}-admin-credentials" \
    -n "${default_ns}" -o jsonpath='{.data.password}' 2>/dev/null | base64 -d 2>/dev/null || true)"
  if [[ -n "${ENV_THUNDER_ADMIN_PASSWORD}" ]]; then
    log_step "Default Environment Thunder ID Console"
    log_info "URL:      http://${default_org}-${default_env}.thunder.amp.localhost:8080/console"
    log_info "Username: admin"
    log_info "Password: ${ENV_THUNDER_ADMIN_PASSWORD}"
  fi
fi

echo ""
echo ""
log_info "To check status: kubectl get pods -A"
echo ""
log_step "Uninstall Options"
log_info "Uninstall platform (keep cluster):       ./uninstall.sh"
log_info "Uninstall and delete k3d cluster:        ./uninstall.sh --delete-cluster"
log_info "Full cleanup (including Colima profile):  ./uninstall.sh --delete-cluster --delete-colima"
echo ""
