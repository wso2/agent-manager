#!/bin/bash
set -euo pipefail

# ============================================================================
# Agent Manager Local Installation (k3d)
# ============================================================================
# This script provides a comprehensive, idempotent installation that:
# 1. Creates a k3d cluster
# 2. Installs cluster prerequisites (cert-manager, kgateway + the AMP Gateway,
#    OpenBao) and AMP's trace storage (OpenSearch + OpenTelemetry Collector)
# 3. Installs Agent Management Platform
#
# The script is idempotent - it can be run multiple times safely.
# Only public helm charts are used - no local charts or custom images.
# ============================================================================

# Configuration
CLUSTER_NAME="amp-local"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K3D_CONFIG="${K3D_CONFIG:-${SCRIPT_DIR}/k3d-config.yaml}"

# Directory holding the deployment resources this installer applies
# (values/, k8s/, scripts/). These used to be fetched from
# raw.githubusercontent.com at install time, but GitHub rate-limits
# unauthenticated requests per client IP and returns HTTP 429 once the
# per-IP budget is spent (common behind shared/corporate NAT). helm --values
# <url> and kubectl apply -f <url> do not retry on 429, so a single throttled
# response aborts the install. Resolve the bundled copies on disk instead:
# in a repo checkout they live one level up from quick-start/; the dev-container
# image bundles the same tree flat alongside the scripts.
if [[ -d "${SCRIPT_DIR}/../values" ]]; then
    DEPLOYMENTS_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
else
    DEPLOYMENTS_DIR="${SCRIPT_DIR}"
fi

# WSO2 API Platform / Gateway Operator versions
GATEWAY_OPERATOR_VERSION="0.11.3"
# gateway-controller/gateway-runtime 1.1.x reject every RestApi/LlmProvider
# deployment with a bare 404 despite correctly-configured basic auth (confirmed
# via a live tcpdump of the operator's request — same 404 across 1.1.0 and
# 1.1.5). 1.2.0-alpha2 does not have this bug (verified end-to-end: RestApi
# reaches Programmed=True). Chart *version* and container *image tag* are
# pinned separately below — setting chartVersion alone still runs an older
# default image, so GATEWAY_IMAGE_VERSION must also be threaded through.
GATEWAY_CHART_VERSION="2026.09.24"
GATEWAY_IMAGE_VERSION="2026.09.24"

# The gateway chart (1.2.0-beta+) requires an encryption key to be mounted from a Kubernetes Secret
GATEWAY_ENCRYPTION_SECRET_NAME="gateway-encryption-keys"
GATEWAY_ENCRYPTION_SECRET_KEY="default-aesgcm256-v1.bin"

# Namespace the gateway operator runs in (it watches every namespace).
GATEWAY_OPERATOR_NAMESPACE="amp-gateway-operator"

# Source AMP installation helpers
source "${SCRIPT_DIR}/install-helpers.sh"

# Timeouts (in seconds)
TIMEOUT_K3D_READY=60

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
    # Required ports for k3d cluster (host:container mapping), each a listener
    # on the shared AMP Gateway (k8s/amp-gateway.yaml) except 8443.
    # 8080  - Console, API, Thunder auth, external AI gateway control plane
    # 8443  - Thunder HTTPS
    # 11080 - Agent Manager Observer
    # 19080 - API Platform Gateway vhosts (LLM proxy, OTLP /otel)
    local required_ports=(8080 8443 11080 19080)
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

# ============================================================================
# MAIN INSTALLATION FLOW
# ============================================================================

log_step "Agent Manager Installation"

# Check and fix Docker permissions
check_docker_permissions() {
    # Try docker ps first — Docker contexts (e.g., Colima profiles) route to
    # the correct socket automatically, so we don't need to check a hardcoded path.
    if docker ps &>/dev/null; then
        log_success "Docker access verified"
        return 0
    fi

    # docker ps failed — check common socket locations for a helpful error message.
    local docker_sock="/var/run/docker.sock"
    if [ ! -S "${docker_sock}" ]; then
        log_error "Docker is not accessible and socket not found at ${docker_sock}"
        log_info "Make sure Docker is running."
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
log_step "Step 1/9: Verifying prerequisites"

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

# Charts on the same private registry as the images need a helm login as well.
# Doing it here also proves the credentials before any cluster is created.
if declare -F amp_registry_helm_login >/dev/null 2>&1; then
    if ! amp_registry_helm_login; then
        log_error "Could not log helm in to the chart registry ${HELM_CHART_REGISTRY}"
        exit 1
    fi
fi

log_success "All prerequisites verified"

# ============================================================================
# Step 2: Setup k3d Cluster
# ============================================================================

log_step "Step 2/9: Setting up k3d cluster"

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
    # k3d writes the node's registries.yaml at creation time only, so a re-run
    # against an existing cluster cannot add it. That file is the only mechanism
    # covering images pulled into namespaces created later, so say so
    # rather than letting agent deploys fail with no explanation. A cluster an
    # earlier private install created already has it, so check the node first.
    if declare -F amp_registry_is_private >/dev/null 2>&1 && amp_registry_is_private; then
        node_creds=0
        amp_k3d_node_has_registry_credentials "k3d-${CLUSTER_NAME}-server-0" || node_creds=$?
        if [ "${node_creds}" -eq 1 ]; then
            log_warning "Reusing an existing cluster: its node has no registry credentials."
            log_warning "Agent image pulls will fail. Delete the cluster and re-run to add them."
        elif [ "${node_creds}" -ne 0 ]; then
            log_warning "Reusing an existing cluster: could not check its node for registry credentials."
            log_warning "If agent image pulls fail, delete the cluster and re-run to add them."
        fi
    fi
else
    log_info "Creating k3d cluster..."

    # A private registry needs credentials inside the node: its containerd sees
    # neither the host's docker config nor a namespace's imagePullSecrets for
    # namespaces created later. k3d writes registries.config through to
    # the node's registries.yaml verbatim, so append the auth block there.
    # Written to a 0600 temp file because it contains the registry token; the
    # credential still ends up in the node's /etc/rancher/k3s/registries.yaml.
    if declare -F amp_registry_is_private >/dev/null 2>&1 && amp_registry_is_private; then
        log_info "Adding registry credentials to the k3d node configuration..."
        _k3d_config_with_auth="$(mktemp)"
        chmod 600 "${_k3d_config_with_auth}"
        if ! { cat "${K3D_CONFIG}"; amp_registries_yaml_fragment; } > "${_k3d_config_with_auth}"; then
            rm -f "${_k3d_config_with_auth}"
            log_error "Failed to render the k3d registry credentials"
            exit 1
        fi
        K3D_CONFIG="${_k3d_config_with_auth}"
        # Remove it as soon as the cluster is up; k3d has copied the content in.
        trap 'rm -f "${_k3d_config_with_auth:-}"' EXIT
    fi

    # Create k3d cluster
    if k3d cluster create --config "${K3D_CONFIG}"; then
        log_success "k3d cluster created successfully"
        # `subnet:` in k3d-config.yaml pins only the server node; pin the
        # loadbalancer too so it can never take the server's address.
        pin_serverlb_ip || exit 1
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

log_step "Step 3/9: Applying CoreDNS Custom Configuration"

log_info "Applying CoreDNS custom configuration for AMP..."
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
# Step 4: Install Cluster Prerequisites
# ============================================================================

log_step "Step 4/9: Installing Cluster Prerequisites (Cert Manager, Gateway API CRDs, kgateway, AMP Gateway)"

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

# Install kgateway CRDs
log_info "Installing kgateway CRDs..."
if helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
    --kube-context ${CLUSTER_CONTEXT} \
    --namespace kgateway-system \
    --create-namespace \
    --version v2.3.11 \
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
    --namespace kgateway-system \
    --create-namespace \
    --version v2.3.11 \
    --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true \
    --timeout 180s &>/dev/null; then
    log_success "kgateway installed successfully"
else
    log_error "Failed to install kgateway"
    exit 1
fi

# Shared ingress the AMP charts attach their HTTPRoutes to.
log_info "Applying the AMP Gateway..."
if kubectl apply -f "${DEPLOYMENTS_DIR}/k8s/amp-gateway.yaml" &>/dev/null; then
    log_success "AMP Gateway applied"
else
    log_error "Failed to apply the AMP Gateway"
    exit 1
fi

# ============================================================================
# Step 5: Setup Secrets (OpenBao)
# ============================================================================

log_step "Step 5/9: Setup Secrets (OpenBao)"

# OpenBao is agent-manager-service's KV store for the secrets it manages.
log_info "Installing OpenBao..."
if helm upgrade --install openbao oci://ghcr.io/openbao/charts/openbao \
    --kube-context ${CLUSTER_CONTEXT} \
    --namespace openbao \
    --create-namespace \
    --version 0.25.6 \
    --values "${DEPLOYMENTS_DIR}/values/openbao.yaml" \
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

# ============================================================================
# Step 6: Install Trace Storage (OpenSearch + OpenTelemetry Collector)
# ============================================================================

log_step "Step 6/9: Installing Trace Storage (OpenSearch + OpenTelemetry Collector)"

# shellcheck source=../scripts/lib-observability.sh
source "${DEPLOYMENTS_DIR}/scripts/lib-observability.sh"
if ! install_observability_stack "${DEPLOYMENTS_DIR}"; then
    log_error "Failed to install trace storage"
    exit 1
fi
log_success "Trace storage installed"

# ============================================================================
# Step 7: Install Gateway Operator
# ============================================================================


log_step "Step 7/9: Installing Gateway Operator"
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
    "${GATEWAY_OPERATOR_NAMESPACE}" \
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

log_success "Gateway Operator setup complete"

# ============================================================================
# Step 8: Install AMP Thunder Extension
# ============================================================================

log_step "Step 8/9: Installing WSO2 AMP Thunder Extension"

log_info "Installing WSO2 AMP Thunder Extension..."
log_info "Gateway API CRDs and Gateway Operator are now available"
if ! install_registry_credentials; then
    exit 1
fi

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
# Step 9: Install Agent Management Platform
# ============================================================================

log_step "Step 9/9: Installing Agent Management Platform"

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

log_success "Agent Manager is ready."
# The console sign-in comes first, as the one next step. The API and trace
# endpoints are left out: the console shows an agent's trace endpoint where it
# is needed, and the API is reached through the console or amctl.
# Localhost URLs apply to the local quick-start. Wrappers that front the cluster
# with a reverse proxy (e.g. the VM installer) set SHOW_LOCALHOST_URLS=false and
# print their own reachable URLs instead.
if [[ "${SHOW_LOCALHOST_URLS:-true}" == "true" ]]; then
  # Admin login for the local console. Fetched fresh from the amp-admin-credentials
  # Secret (see wso2-amp-thunder-extension/templates/admin-credentials.yaml) rather
  # than tracked through the install. Wrappers that set SHOW_LOCALHOST_URLS=false
  # print their own reachable console URL and their own copy of this instead —
  # this localhost URL would be wrong there.
  bash "${DEPLOYMENTS_DIR}/scripts/print-admin-credentials.sh" \
    "http://console.amp.localhost:8080" "${THUNDER_NS}" || true
fi

echo ""
echo ""
log_info "To check status: kubectl get pods -A"

# These hints point at ./uninstall.sh, a sibling of this script in the repo
# checkout. Wrappers that source install.sh from a bundle (e.g. the VM installer)
# have no such sibling left on disk — bootstrap.sh deletes the unpacked bundle on
# exit — and tear down extra host state of their own, so they suppress these lines
# and document their own teardown. Defaults to SHOW_LOCALHOST_URLS: both answer
# "is this the plain local quick-start?".
if [[ "${SHOW_UNINSTALL_HINTS:-${SHOW_LOCALHOST_URLS:-true}}" == "true" ]]; then
  echo ""
  # Full path: the one-command installers run this script from an unpacked
  # bundle, so the caller's shell is rarely in this directory. Shell-quoted so
  # the commands still paste correctly when the path contains spaces.
  UNINSTALL_SCRIPT="$(printf '%q' "${SCRIPT_DIR}/uninstall.sh")"
  log_step "Uninstall Options"
  log_info "Uninstall platform (keep cluster):       bash ${UNINSTALL_SCRIPT}"
  log_info "Uninstall and delete k3d cluster:        bash ${UNINSTALL_SCRIPT} --delete-cluster"
  log_info "Full cleanup (including Colima profile):  bash ${UNINSTALL_SCRIPT} --delete-cluster --delete-colima"
fi
echo ""
