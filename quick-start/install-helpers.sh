#!/usr/bin/env bash

# Helper functions for Agent Management Platform installation
# Assumes cluster is already set up and configured

set -eo pipefail

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

# Configuration variables
# Remote Helm chart repository and versions
HELM_CHART_REGISTRY="${HELM_CHART_REGISTRY:-ghcr.io/wso2}"
AMP_CHART_VERSION="${AMP_CHART_VERSION:-0.0.0-dev}"
BUILD_CI_CHART_VERSION="${BUILD_CI_CHART_VERSION:-0.0.0-dev}"
OBSERVABILITY_CHART_VERSION="${OBSERVABILITY_CHART_VERSION:-0.0.0-dev}"

# Chart names
AMP_CHART_NAME="wso2-ai-agent-management-platform"
BUILD_CI_CHART_NAME="wso2-amp-build-extension"
OBSERVABILITY_CHART_NAME="wso2-amp-observability-extension"

# Default namespace definitions (can be overridden via environment variables)
AMP_NS="${AMP_NS:-wso2-amp}"
BUILD_CI_NS="${BUILD_CI_NS:-openchoreo-build-plane}"
OBSERVABILITY_NS="${OBSERVABILITY_NS:-openchoreo-observability-plane}"

# Helm arguments arrays (initialize if not set)
if [[ -z "${AMP_HELM_ARGS+x}" ]]; then
    AMP_HELM_ARGS=()
fi
if [[ -z "${BUILD_CI_HELM_ARGS+x}" ]]; then
    BUILD_CI_HELM_ARGS=()
fi
if [[ -z "${OBSERVABILITY_HELM_ARGS+x}" ]]; then
    OBSERVABILITY_HELM_ARGS=()
fi

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${RESET} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${RESET} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${RESET} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${RESET} $1"
}

# Check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check if namespace exists
namespace_exists() {
    local namespace="$1"
    kubectl get namespace "$namespace" >/dev/null 2>&1
}

# Check if helm release exists
helm_release_exists() {
    local release="$1"
    local namespace="$2"
    helm list -n "$namespace" --short 2>/dev/null | grep -q "^${release}$"
}

# Wait for pods to be ready in a namespace
wait_for_pods() {
    local namespace="$1"
    local timeout="${2:-300}" # 5 minutes default
    local label_selector="${3:-}"

    log_info "Waiting for pods in namespace '$namespace' to be ready..."

    local selector_flag=""
    if [[ -n "$label_selector" ]]; then
        selector_flag="-l $label_selector"
    fi

    if ! timeout "$timeout" bash -c "
        while true; do
            pods=\$(kubectl get pods -n '$namespace' $selector_flag --no-headers 2>/dev/null || true)
            if [[ -z \"\$pods\" ]]; then
                echo 'No pods found yet, waiting...'
                sleep 5
                continue
            fi
            if echo \"\$pods\" | grep -v 'Running\|Completed' | grep -q .; then
                echo 'Waiting for pods to be ready...'
                sleep 5
            else
                echo 'All pods are ready!'
                break
            fi
        done
    "; then
        log_error "Timeout waiting for pods in namespace '$namespace'"
        kubectl get pods -n "$namespace" $selector_flag 2>/dev/null || true
        return 1
    fi

    log_success "All pods in namespace '$namespace' are ready"
}

# Wait for a deployment to be ready
wait_for_deployment() {
    local deployment="$1"
    local namespace="$2"
    local timeout="${3:-300}"

    log_info "Waiting for deployment '$deployment' in namespace '$namespace' to be ready..."

    if kubectl wait --for=condition=available --timeout="${timeout}s" \
        deployment/"$deployment" -n "$namespace" 2>/dev/null; then
        log_success "Deployment '$deployment' is ready"
        return 0
    else
        log_warning "Deployment '$deployment' may still be starting"
        kubectl get deployment "$deployment" -n "$namespace" 2>/dev/null || true
        return 0
    fi
}

# Patch APIClass for CORS configuration
patch_apiclass_cors() {
    local apiclass_name="${1:-default-with-cors}"
    local namespace="${2:-default}"
    local origin="${3:-http://localhost:3000}"

    log_info "Patching APIClass '$apiclass_name' in namespace '$namespace' to allow CORS origin '$origin'..."

    # Check if APIClass exists
    if ! kubectl get apiclass "$apiclass_name" -n "$namespace" >/dev/null 2>&1; then
        log_warning "APIClass '$apiclass_name' not found in namespace '$namespace', skipping CORS patch"
        return 0
    fi

    # Apply the CORS patch
    if kubectl patch apiclass "$apiclass_name" -n "$namespace" --type json \
        -p "[{\"op\":\"add\",\"path\":\"/spec/restPolicy/defaults/cors/allowOrigins/-\",\"value\":\"$origin\"}]" 2>/dev/null; then
        log_success "APIClass '$apiclass_name' patched successfully with CORS origin '$origin'"
    else
        # If the patch fails (e.g., origin already exists), try to verify it exists
        if kubectl get apiclass "$apiclass_name" -n "$namespace" -o jsonpath='{.spec.restPolicy.defaults.cors.allowOrigins}' 2>/dev/null | grep -q "$origin"; then
            log_info "CORS origin '$origin' already exists in APIClass '$apiclass_name'"
        else
            log_warning "Failed to patch APIClass '$apiclass_name'. This may be expected if CORS is already configured."
        fi
    fi
}

# Install a remote OCI helm chart with idempotency
install_remote_helm_chart() {
    local release_name="$1"
    local chart_ref="$2"  # Full OCI reference like oci://ghcr.io/org/chart:version
    local namespace="$3"
    local create_namespace="${4:-true}"
    local wait_flag="${5:-false}"
    local timeout="${6:-1800}"
    shift 6
    local additional_args=("$@")

    log_info "Installing Helm chart '$chart_ref' as release '$release_name' in namespace '$namespace'..."

    # Check if release already exists
    if helm_release_exists "$release_name" "$namespace"; then
        log_warning "Helm release '$release_name' already exists in namespace '$namespace'"

        # Try to upgrade the release
        local upgrade_args=(
            "upgrade" "$release_name" "$chart_ref"
            "--namespace" "$namespace"
            "--timeout" "${timeout}s"
        )

        if [[ "$wait_flag" == "true" ]]; then
            upgrade_args+=("--wait")
        fi

        upgrade_args+=("${additional_args[@]}")

        log_info "Upgrading release '$release_name' from '$chart_ref'..."
        if helm "${upgrade_args[@]}"; then
            log_success "Helm release '$release_name' upgraded successfully"
        else
            log_error "Failed to upgrade Helm release '$release_name'"
            return 1
        fi
    else
        # Create namespace if needed and doesn't exist
        if [[ "$create_namespace" == "true" ]] && ! namespace_exists "$namespace"; then
            log_info "Creating namespace '$namespace'..."
            kubectl create namespace "$namespace"
        fi

        # Install new release
        local install_args=(
            "install" "$release_name" "$chart_ref"
            "--namespace" "$namespace"
            "--timeout" "${timeout}s"
        )

        if [[ "$wait_flag" == "true" ]]; then
            install_args+=("--wait")
        fi

        install_args+=("${additional_args[@]}")

        log_info "Installing release '$release_name' from '$chart_ref' (timeout: ${timeout}s)"
        log_info "This may take several minutes..."

        if helm "${install_args[@]}"; then
            log_success "Helm release '$release_name' installed successfully"
        else
            log_error "Failed to install Helm release '$release_name'"
            return 1
        fi
    fi
}

# Install Agent Management Platform
install_agent_management_platform() {
    log_info "Installing Agent Management Platform..."

    local chart_ref="oci://${HELM_CHART_REGISTRY}/${AMP_CHART_NAME}"
    local chart_version="${AMP_CHART_VERSION}"

    log_info "Using chart: $chart_ref:$chart_version"

    # Start a background process to monitor pod status
    (
        sleep 10  # Give it time to start creating resources
        while true; do
            log_info "Current pod status in namespace $AMP_NS:"
            kubectl get pods -n "$AMP_NS" 2>/dev/null || echo "No pods yet..."
            sleep 15
        done
    ) &
    local monitor_pid=$!

    # Add version to helm args
    local version_args=("--version" "$chart_version")
    local release_name="agent-management-platform"
    
    install_remote_helm_chart "$release_name" "$chart_ref" "$AMP_NS" "true" "false" "1800" \
        "${version_args[@]}" "${AMP_HELM_ARGS[@]}"

    # Stop the monitoring process
    kill $monitor_pid 2>/dev/null || true

    # Wait for PostgreSQL to be ready
    log_info "Waiting for PostgreSQL to be ready..."
    wait_for_deployment "${release_name}-postgresql" "$AMP_NS" 600

    # Wait for agent manager service to be ready
    log_info "Waiting for Agent Manager Service to be ready..."
    wait_for_deployment "amp-api" "$AMP_NS" 600

    # Wait for console to be ready
    log_info "Waiting for Console to be ready..."
    wait_for_deployment "amp-console" "$AMP_NS" 600

    # Patch APIClass for CORS configuration
    local apiclass_name="${APICLASS_NAME:-default-with-cors}"
    local apiclass_ns="${APICLASS_NAMESPACE:-default}"
    local cors_origin="${CORS_ORIGIN:-http://localhost:3000}"
    patch_apiclass_cors "$apiclass_name" "$apiclass_ns" "$cors_origin"
}

# Install Build CI
install_build_ci() {
    log_info "Installing Build Workflow Extensions..."

    local chart_ref="oci://${HELM_CHART_REGISTRY}/${BUILD_CI_CHART_NAME}"
    local chart_version="${BUILD_CI_CHART_VERSION}"

    log_info "Using chart: $chart_ref:$chart_version"

    # Add version to helm args
    local version_args=("--version" "$chart_version")
    
    install_remote_helm_chart "build-workflow-extensions" "$chart_ref" "$BUILD_CI_NS" "true" "false" "1800" \
        "${version_args[@]}" "${BUILD_CI_HELM_ARGS[@]}"

    log_success "Build Workflow Extensions installed successfully"
}

# Install Observability DataPrepper
install_observability_dataprepper() {
    log_info "Installing Observability Extensions..."

    local chart_ref="oci://${HELM_CHART_REGISTRY}/${OBSERVABILITY_CHART_NAME}"
    local chart_version="${OBSERVABILITY_CHART_VERSION}"

    log_info "Using chart: $chart_ref:$chart_version"

    # Add version to helm args
    local version_args=("--version" "$chart_version")
    
    install_remote_helm_chart "amp-observability-traces" "$chart_ref" "$OBSERVABILITY_NS" "true" "false" "1800" \
        "${version_args[@]}" "${OBSERVABILITY_HELM_ARGS[@]}"

    # Wait for data-prepper to be ready
    log_info "Waiting for DataPrepper to be ready..."
    wait_for_deployment "data-prepper" "$OBSERVABILITY_NS" 600

    # Wait for traces-observer if enabled
    if kubectl get deployment amp-traces-observer -n "$OBSERVABILITY_NS" >/dev/null 2>&1; then
        log_info "Waiting for Traces Observer Service to be ready..."
        wait_for_deployment "amp-traces-observer" "$OBSERVABILITY_NS" 600
    fi
}

# Silent version for simple installer
install_observability_dataprepper_silent() {
    local chart_ref="oci://${HELM_CHART_REGISTRY}/${OBSERVABILITY_CHART_NAME}"
    local chart_version="${OBSERVABILITY_CHART_VERSION}"
    local version_args=("--version" "$chart_version")
    
    install_remote_helm_chart "amp-observability-traces" "$chart_ref" "$OBSERVABILITY_NS" "true" "false" "1800" \
        "${version_args[@]}" "${OBSERVABILITY_HELM_ARGS[@]}" >/dev/null 2>&1 || return 1
    
    wait_for_deployment "data-prepper" "$OBSERVABILITY_NS" 600 >/dev/null 2>&1 || return 1
    
    if kubectl get deployment amp-traces-observer -n "$OBSERVABILITY_NS" >/dev/null 2>&1; then
        wait_for_deployment "amp-traces-observer" "$OBSERVABILITY_NS" 600 >/dev/null 2>&1 || return 1
    fi
    
    return 0
}

# Silent version of AMP installation
install_agent_management_platform_silent() {
    local chart_ref="oci://${HELM_CHART_REGISTRY}/${AMP_CHART_NAME}"
    local chart_version="${AMP_CHART_VERSION}"
    local version_args=("--version" "$chart_version")
    local release_name="amp"
    local helm_log="/tmp/helm-amp-install.log"

    # Install Helm chart - capture output for debugging but don't show unless there's an error
    if ! install_remote_helm_chart "$release_name" "$chart_ref" "$AMP_NS" "true" "true" "1800" \
        "${version_args[@]}" "${AMP_HELM_ARGS[@]}" >"$helm_log" 2>&1; then
        log_error "Helm chart installation failed"
        echo ""
        echo "Helm installation log (last 50 lines):"
        tail -50 "$helm_log" 2>/dev/null || cat "$helm_log" 2>/dev/null || echo "Log file not available"
        echo ""
        echo "Helm release status:"
        helm status "$release_name" -n "$AMP_NS" 2>&1 || echo "Release not found"
        echo ""
        echo "Pods in namespace $AMP_NS:"
        kubectl get pods -n "$AMP_NS" 2>&1 || echo "No pods found"
        echo ""
        echo "Events in namespace $AMP_NS:"
        kubectl get events -n "$AMP_NS" --sort-by='.lastTimestamp' | tail -20 2>&1 || true
        return 1
    fi
    
    # Wait for PostgreSQL deployment (Bitnami subchart uses release-name-postgresql)
    if ! wait_for_deployment "${release_name}-postgresql" "$AMP_NS" 600 >/dev/null 2>&1; then
        log_error "PostgreSQL deployment failed to become ready"
        echo ""
        echo "PostgreSQL pod status:"
        kubectl get pods -n "$AMP_NS" -l app.kubernetes.io/name=postgresql 2>&1 || true
        echo ""
        echo "PostgreSQL pod events:"
        kubectl get events -n "$AMP_NS" --field-selector involvedObject.name=$(kubectl get pods -n "$AMP_NS" -l app.kubernetes.io/name=postgresql -o jsonpath='{.items[0].metadata.name}' 2>/dev/null) 2>&1 | tail -10 || true
        echo ""
        echo "PostgreSQL pod logs (if available):"
        kubectl logs -n "$AMP_NS" -l app.kubernetes.io/name=postgresql --tail=30 2>&1 || true
        return 1
    fi
    
    # Wait for agent manager service (amp-api)
    if ! wait_for_deployment "amp-api" "$AMP_NS" 600 >/dev/null 2>&1; then
        log_error "Agent Manager Service deployment failed to become ready"
        echo ""
        echo "Agent Manager Service pod status:"
        kubectl get pods -n "$AMP_NS" -l app.kubernetes.io/component=agent-manager-service 2>&1 || true
        echo ""
        echo "Agent Manager Service pod logs:"
        kubectl logs -n "$AMP_NS" -l app.kubernetes.io/component=agent-manager-service --tail=50 2>&1 || true
        return 1
    fi
    
    # Wait for console (amp-console)
    if ! wait_for_deployment "amp-console" "$AMP_NS" 600 >/dev/null 2>&1; then
        log_error "Console deployment failed to become ready"
        echo ""
        echo "Console pod status:"
        kubectl get pods -n "$AMP_NS" -l app.kubernetes.io/component=console 2>&1 || true
        echo ""
        echo "Console pod logs:"
        kubectl logs -n "$AMP_NS" -l app.kubernetes.io/component=console --tail=50 2>&1 || true
        return 1
    fi
    
    # Patch APIClass for CORS configuration (non-fatal)
    local apiclass_name="${APICLASS_NAME:-default-with-cors}"
    local apiclass_ns="${APICLASS_NAMESPACE:-default}"
    local cors_origin="${CORS_ORIGIN:-http://localhost:3000}"
    patch_apiclass_cors "$apiclass_name" "$apiclass_ns" "$cors_origin" >/dev/null 2>&1 || true
    
    return 0
}

# Silent prerequisite verification
verify_prerequisites_silent() {
    command_exists kubectl || return 1
    command_exists helm || return 1
    kubectl cluster-info >/dev/null 2>&1 || return 1
    
    # Check for OpenChoreo Observability Plane (required)
    if ! kubectl get namespace openchoreo-observability-plane >/dev/null 2>&1; then
        echo ""
        echo "❌ OpenChoreo Observability Plane not found"
        echo ""
        echo "The Agent Management Platform requires OpenChoreo Observability Plane."
        echo ""
        echo "Please install it first:"
        echo "  helm install observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \\"
        echo "    --version 0.3.2 \\"
        echo "    --namespace openchoreo-observability-plane \\"
        echo "    --create-namespace"
        echo ""
        echo "Documentation: https://openchoreo.dev/docs/v0.3.x/observability/"
        echo ""
        return 1
    fi
    
    # Verify OpenSearch is accessible
    if ! kubectl get pods -n openchoreo-observability-plane -l app=opensearch >/dev/null 2>&1; then
        echo ""
        echo "⚠️  Warning: OpenSearch pods not found in observability plane"
        echo "   Installation may fail without OpenSearch"
        echo ""
    fi
    
    return 0
}

# Verify prerequisites
verify_prerequisites() {
    log_info "Verifying prerequisites..."

    local missing_tools=()

    if ! command_exists kubectl; then
        missing_tools+=("kubectl")
    fi

    if ! command_exists helm; then
        missing_tools+=("helm")
    fi

    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        return 1
    fi

    # Check if kubectl can connect to a cluster
    if ! kubectl cluster-info >/dev/null 2>&1; then
        log_error "kubectl cannot connect to a cluster. Please ensure KUBECONFIG is set correctly."
        return 1
    fi

    log_success "All prerequisites verified"
}

# Print installation summary
print_installation_summary() {
    log_success "Agent Management Platform installation completed successfully!"
    echo ""
    log_info "Installation Summary:"
    log_info "  Cluster: $(kubectl config current-context)"
    log_info "  Agent Management Platform Namespace: $AMP_NS"
    log_info "  Build CI Namespace: $BUILD_CI_NS"
    log_info "  Observability Namespace: $OBSERVABILITY_NS"
    echo ""
    log_info "Deployed Components:"
    kubectl get pods -n "$AMP_NS" 2>/dev/null || true
    echo ""
    log_info "To access the console, run:"
    log_info "  kubectl port-forward -n $AMP_NS svc/amp-console 3000:3000"
    log_info "  Then open: http://localhost:3000"
    echo ""
    log_info "To access the agent manager API, run:"
    log_info "  kubectl port-forward -n $AMP_NS svc/amp-api 8080:8080"
    log_info "  API endpoint: http://localhost:8080"
}

# Clean up function
cleanup() {
    log_info "Cleanup complete"
}

# Register cleanup function
trap cleanup EXIT

# ============================================================================
# KIND CLUSTER SETUP FUNCTIONS
# ============================================================================

# Check if Docker is running
verify_docker_running() {
    if ! docker info >/dev/null 2>&1; then
        log_error "Docker is not running"
        echo ""
        echo "   The installation requires Docker to be running."
        echo ""
        echo "   → Start Docker Desktop, or"
        echo "   → Start Colima: colima start"
        echo ""
        echo "   Then run this script again."
        echo ""
        return 1
    fi
    return 0
}

# Check if Kind is installed
verify_kind_installed() {
    if ! command_exists kind; then
        log_error "Kind is not installed"
        echo ""
        echo "   Kind (Kubernetes in Docker) is required for local installation."
        echo ""
        echo "   Install Kind:"
        echo "   → macOS: brew install kind"
        echo "   → Linux: curl -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64"
        echo "            chmod +x ./kind && sudo mv ./kind /usr/local/bin/kind"
        echo ""
        echo "   Documentation: https://kind.sigs.k8s.io/docs/user/quick-start/"
        echo ""
        return 1
    fi
    return 0
}

# Setup Kind cluster
setup_kind_cluster() {
    local cluster_name="${1:-openchoreo-local}"
    local config_file="${2:-./kind-config.yaml}"
    
    log_info "Setting up Kind cluster '$cluster_name'..."
    
    # Ensure container is connected to kind network (critical when running in Docker container)
    if docker network inspect kind &>/dev/null 2>&1; then
        local container_id="$(cat /etc/hostname 2>/dev/null || echo "")"
        if [[ -n "$container_id" ]] && [[ "$container_id" != "localhost" ]]; then
            if [ "$(docker inspect -f '{{json .NetworkSettings.Networks.kind}}' "${container_id}" 2>/dev/null)" = "null" ]; then
                log_info "Connecting container to kind network..."
                docker network connect "kind" "${container_id}" >/dev/null 2>&1 || true
                sleep 2
            fi
        fi
    fi
    
    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "^${cluster_name}$"; then
        log_warning "Kind cluster '$cluster_name' already exists"
        
        # Check if cluster container is actually running
        local control_plane_container="${cluster_name}-control-plane"
        if ! docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${control_plane_container}$"; then
            log_warning "Cluster container '$control_plane_container' is not running. Attempting to recover..."
            docker start "${control_plane_container}" >/dev/null 2>&1 || true
            sleep 5
        fi
        
        # Always refresh kubeconfig with correct internal IP (critical for containerized environments)
        log_info "Refreshing kubeconfig for existing cluster..."
        
        # Get control plane IP from Docker network (prefer kind network)
        local control_plane_ip=$(docker inspect "${control_plane_container}" --format '{{range $key, $value := .NetworkSettings.Networks}}{{if eq $key "kind"}}{{$value.IPAddress}}{{end}}{{end}}' 2>/dev/null | head -1)
        
        # Fallback to any network IP if kind network IP not found
        if [[ -z "$control_plane_ip" ]]; then
            control_plane_ip=$(docker inspect "${control_plane_container}" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' 2>/dev/null | head -1)
        fi
        
        # Configure kubeconfig with internal IP
        if [[ -n "$control_plane_ip" ]]; then
            log_info "Configuring kubeconfig with control plane IP: ${control_plane_ip}"
            mkdir -p /state/kube
            if kind get kubeconfig --name "${cluster_name}" 2>/dev/null | sed "s|server: https://127.0.0.1:[0-9]*|server: https://${control_plane_ip}:6443|" > /state/kube/config-internal.yaml; then
                export KUBECONFIG=/state/kube/config-internal.yaml
                kubectl config use-context "kind-${cluster_name}" >/dev/null 2>&1 || true
            fi
        else
            log_warning "Could not determine control plane IP, trying default kubeconfig"
            mkdir -p /state/kube
            if kind get kubeconfig --name "${cluster_name}" >/dev/null 2>&1 > /state/kube/config-internal.yaml; then
                export KUBECONFIG=/state/kube/config-internal.yaml
                kubectl config use-context "kind-${cluster_name}" >/dev/null 2>&1 || true
            fi
        fi
        
        # Wait a moment for the API server to be ready
        sleep 2
        
        # Verify cluster is accessible with retries
        local max_retries=5
        local retry_count=0
        local cluster_accessible=false
        
        while [ $retry_count -lt $max_retries ]; do
            if kubectl cluster-info >/dev/null 2>&1; then
                cluster_accessible=true
                break
            fi
            
            # Try with explicit context
            if kubectl cluster-info --context "kind-${cluster_name}" >/dev/null 2>&1; then
                cluster_accessible=true
                break
            fi
            
            # Refresh kubeconfig again if first retry
            if [[ $retry_count -eq 1 ]] && [[ -n "$control_plane_ip" ]]; then
                log_info "Retrying kubeconfig refresh..."
                if kind get kubeconfig --name "${cluster_name}" 2>/dev/null | sed "s|server: https://127.0.0.1:[0-9]*|server: https://${control_plane_ip}:6443|" > /state/kube/config-internal.yaml; then
                    export KUBECONFIG=/state/kube/config-internal.yaml
                    kubectl config use-context "kind-${cluster_name}" >/dev/null 2>&1 || true
                fi
            fi
            
            retry_count=$((retry_count + 1))
            if [ $retry_count -lt $max_retries ]; then
                log_info "Cluster not yet accessible, retrying ($retry_count/$max_retries)..."
                sleep 3
            fi
        done
        
        if [ "$cluster_accessible" = true ]; then
            log_success "Using existing Kind cluster '$cluster_name'"
            if [[ -n "$control_plane_ip" ]]; then
                echo "✓ kubectl configured to connect at ${control_plane_ip}"
            fi
            return 0
        else
            log_error "Cluster exists but is not accessible after recovery attempts."
            echo ""
            echo "   Troubleshooting steps:"
            echo "   1. Check cluster container: docker ps | grep ${control_plane_container}"
            echo "   2. Check container logs: docker logs ${control_plane_container}"
            echo "   3. Verify network: docker network inspect kind"
            echo "   4. Delete and recreate: kind delete cluster --name $cluster_name"
            echo ""
            return 1
        fi
    fi
    
    # Create shared directory for OpenChoreo
    log_info "Creating shared directory for OpenChoreo..."
    mkdir -p /tmp/kind-shared
    
    # Check if config file exists
    if [[ ! -f "$config_file" ]]; then
        log_error "Kind configuration file not found: $config_file"
        return 1
    fi
    
    # Create Kind cluster
    log_info "Creating Kind cluster (this may take 2-3 minutes)..."
    if kind create cluster --config "$config_file" 2>&1 | tee /tmp/kind-create.log; then
        log_success "Kind cluster created successfully"
    else
        log_error "Failed to create Kind cluster"
        echo ""
        echo "   Common causes:"
        echo "   • Port 6443 already in use"
        echo "   • Insufficient Docker resources"
        echo "   • Previous cluster not fully deleted"
        echo ""
        echo "   Try:"
        echo "   1. Delete any existing cluster: kind delete cluster --name $cluster_name"
        echo "   2. Restart Docker"
        echo "   3. Run this script again"
        echo ""
        return 1
    fi
    
    # Wait for cluster to be ready
    log_info "Waiting for cluster nodes to be ready..."
    if ! wait_for_kind_cluster_ready "$cluster_name"; then
        log_error "Cluster nodes did not become ready in time"
        return 1
    fi
    
    log_success "Kind cluster is ready"
    return 0
}

# Wait for Kind cluster to be ready
wait_for_kind_cluster_ready() {
    local cluster_name="${1:-openchoreo-local}"
    local timeout=600
    local elapsed=0
    
    while [ $elapsed -lt $timeout ]; do
        if kubectl get nodes --context "kind-${cluster_name}" >/dev/null 2>&1; then
            if kubectl wait --for=condition=Ready nodes --all --timeout=10s --context "kind-${cluster_name}" >/dev/null 2>&1; then
                return 0
            fi
        fi
        sleep 5
        elapsed=$((elapsed + 5))
    done
    
    return 1
}

# ============================================================================
# OPENCHOREO INSTALLATION FUNCTIONS
# ============================================================================

# OpenChoreo configuration
OPENCHOREO_VERSION="${OPENCHOREO_VERSION:-0.3.2}"
OPENCHOREO_REGISTRY="oci://ghcr.io/openchoreo/helm-charts"

# Install OpenChoreo Cilium CNI
install_openchoreo_cilium() {
    log_info "Installing Cilium CNI..."
    
    if helm status cilium -n cilium >/dev/null 2>&1; then
        log_warning "Cilium already installed, skipping..."
        return 0
    fi
    
    install_remote_helm_chart "cilium" \
        "${OPENCHOREO_REGISTRY}/cilium" \
        "cilium" \
        "true" \
        "true" \
        "300" \
        "--version" "$OPENCHOREO_VERSION"
    
    log_info "Waiting for Cilium pods to be ready..."
    kubectl wait --for=condition=Ready pod -l k8s-app=cilium -n cilium --timeout=300s 2>&1 | grep -v "no matching resources" || true
    
    log_success "Cilium CNI ready"
    return 0
}

# Install OpenChoreo Control Plane
install_openchoreo_control_plane() {
    log_info "Installing OpenChoreo Control Plane (this may take up to 10 minutes)..."
    
    if helm status control-plane -n openchoreo-control-plane >/dev/null 2>&1; then
        log_warning "Control Plane already installed, skipping..."
        return 0
    fi
    
    install_remote_helm_chart "control-plane" \
        "${OPENCHOREO_REGISTRY}/openchoreo-control-plane" \
        "openchoreo-control-plane" \
        "true" \
        "false" \
        "600" \
        "--version" "$OPENCHOREO_VERSION"
    
    log_info "Waiting for Control Plane pods to be ready..."
    if ! kubectl wait --for=condition=Ready pod --all -n openchoreo-control-plane --timeout=600s 2>/dev/null; then
        log_warning "Some Control Plane pods may still be starting (non-fatal)"
    fi
    
    log_success "OpenChoreo Control Plane ready"
    return 0
}

# Install OpenChoreo Data Plane
install_openchoreo_data_plane() {
    log_info "Installing OpenChoreo Data Plane (this may take up to 10 minutes)..."
    
    if helm status data-plane -n openchoreo-data-plane >/dev/null 2>&1; then
        log_warning "Data Plane already installed, skipping..."
        return 0
    fi
    
    # Disable cert-manager since it's already installed by control-plane
    install_remote_helm_chart "data-plane" \
        "${OPENCHOREO_REGISTRY}/openchoreo-data-plane" \
        "openchoreo-data-plane" \
        "true" \
        "false" \
        "600" \
        "--version" "$OPENCHOREO_VERSION" \
        "--set" "cert-manager.enabled=false" \
        "--set" "cert-manager.crds.enabled=false"
    
    log_info "Waiting for Data Plane pods to be ready..."
    if ! kubectl wait --for=condition=Ready pod --all -n openchoreo-data-plane --timeout=600s 2>/dev/null; then
        log_warning "Some Data Plane pods may still be starting (non-fatal)"
    fi

    # Register the Data Plane
    log_info "Registering Data Plane..."
    if curl -s https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/install/add-default-dataplane.sh | bash; then
        log_success "Data Plane registered successfully"
    else
        log_warning "Data Plane registration script failed (non-fatal)"
    fi

    log_info "Configuring observability integration..."

        # Wait for default resources to be created
    log_info "Waiting for default DataPlane and BuildPlane resources..."
    sleep 10

        # Configure DataPlane observer (non-fatal)
    if kubectl get dataplane default -n default &>/dev/null; then
        kubectl patch dataplane default -n default --type merge \
            -p '{"spec":{"observer":{"url":"http://observer.openchoreo-observability-plane:8080","authentication":{"basicAuth":{"username":"dummy","password":"dummy"}}}}}' \
            && log_success "DataPlane observer configured" \
            || log_warning "DataPlane observer configuration failed (non-fatal)"
    else
        log_warning "DataPlane resource not found yet (will use default observer)"
    fi
    
    log_success "OpenChoreo Data Plane ready"
    return 0
}

# Install OpenChoreo Observability Plane
install_openchoreo_observability_plane() {
    log_info "Installing OpenChoreo Observability Plane (this may take up to 15 minutes)..."
    log_info "This includes OpenSearch and OpenSearch Dashboards..."
    
    if helm status observability-plane -n openchoreo-observability-plane >/dev/null 2>&1; then
        log_warning "Observability Plane already installed, skipping..."
        return 0
    fi
    
    install_remote_helm_chart "observability-plane" \
        "${OPENCHOREO_REGISTRY}/openchoreo-observability-plane" \
        "openchoreo-observability-plane" \
        "true" \
        "true" \
        "900" \
        "--version" "$OPENCHOREO_VERSION"
    
    log_info "Waiting for Observability Plane pods to be ready..."
    if ! kubectl wait --for=condition=Ready pod --all -n openchoreo-observability-plane --timeout=900s 2>/dev/null; then
        log_warning "Some Observability pods may still be starting (non-fatal)"
    fi
    
    log_success "OpenChoreo Observability Plane ready"
    return 0
}

# Install OpenChoreo core components (required)
install_openchoreo_core() {
    log_info "Installing OpenChoreo core components..."
    echo ""
    
    # Set kubectl context
    kubectl config use-context kind-openchoreo-local >/dev/null 2>&1
    
    # Install Cilium CNI
    if ! install_openchoreo_cilium; then
        log_error "Failed to install Cilium CNI"
        return 1
    fi
    echo ""
    
    # Install Control Plane
    if ! install_openchoreo_control_plane; then
        log_error "Failed to install OpenChoreo Control Plane"
        echo ""
        echo "   Troubleshooting:"
        echo "   1. Check pod status: kubectl get pods -n openchoreo-control-plane"
        echo "   2. View logs: kubectl logs -n openchoreo-control-plane <pod-name>"
        echo "   3. Check resources: docker stats"
        echo ""
        return 1
    fi
    echo ""
    
    # Install Data Plane
    if ! install_openchoreo_data_plane; then
        log_error "Failed to install OpenChoreo Data Plane"
        echo ""
        echo "   Troubleshooting:"
        echo "   1. Check pod status: kubectl get pods -n openchoreo-data-plane"
        echo "   2. View logs: kubectl logs -n openchoreo-data-plane <pod-name>"
        echo ""
        return 1
    fi
    echo ""

    # Install Build Plane
    if ! install_openchoreo_build_plane; then
        log_error "Failed to install OpenChoreo Build Plane"
        echo ""
        echo "   Troubleshooting:"
        echo "   1. Check pod status: kubectl get pods -n openchoreo-build-plane"
        echo "   2. View logs: kubectl logs -n openchoreo-build-plane <pod-name>"
        echo ""
        return 1
    fi
    echo ""
    
    # Install Observability Plane (required for Agent Management Platform)
    if ! install_openchoreo_observability_plane; then
        log_error "Failed to install OpenChoreo Observability Plane"
        echo ""
        echo "   This component is required for the Agent Management Platform."
        echo ""
        echo "   Troubleshooting:"
        echo "   1. Check pod status: kubectl get pods -n openchoreo-observability-plane"
        echo "   2. Ensure sufficient resources (4GB+ RAM recommended)"
        echo "   3. View logs: kubectl logs -n openchoreo-observability-plane <pod-name>"
        echo ""
        return 1
    fi
    echo ""
    
    log_success "OpenChoreo core components installed successfully"
    return 0
}

install_openchoreo_build_plane() {
    log_info "Installing OpenChoreo Build Plane (this may take up to 10 minutes)..."
    
    if helm status build-plane -n openchoreo-build-plane >/dev/null 2>&1; then
        log_warning "Build Plane already installed, skipping..."
        return 0
    fi

    install_remote_helm_chart "build-plane" \
        "${OPENCHOREO_REGISTRY}/openchoreo-build-plane" \
        "openchoreo-build-plane" \
        "true" \
        "true" \
        "600" \
        "--version" "$OPENCHOREO_VERSION"

    log_info "Waiting for Build Plane pods to be ready..."
    if ! kubectl wait --for=condition=Ready pod --all -n openchoreo-build-plane --timeout=600s 2>/dev/null; then
        log_warning "Some Build Plane pods may still be starting (non-fatal)"
    fi
        # Configure Build Plane
    log_info "Configuring Build Plane..."
    if curl -s https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/install/add-build-plane.sh | bash; then
        log_success "Build Plane configured successfully"
    else
        log_warning "Build Plane configuration script failed (non-fatal)"
    fi

    if kubectl get buildplane default -n default &>/dev/null; then
        kubectl patch buildplane default -n default --type merge \
            -p '{"spec":{"observer":{"url":"http://observer.openchoreo-observability-plane:8080","authentication":{"basicAuth":{"username":"dummy","password":"dummy"}}}}}' \
            && log_success "BuildPlane observer configured" \
            || log_warning "BuildPlane observer configuration failed (non-fatal)"
    else
        log_warning "BuildPlane resource not found yet (will use default observer)"
    fi
    
    log_success "OpenChoreo Build Plane ready"
    return 0
}

# Verify OpenChoreo prerequisites for bootstrap
verify_openchoreo_prerequisites() {
    log_info "Verifying OpenChoreo prerequisites..."
    
    # Check kubectl
    if ! command_exists kubectl; then
        log_error "kubectl is not installed"
        echo ""
        echo "   Install kubectl:"
        echo "   → macOS: brew install kubectl"
        echo "   → Linux: https://kubernetes.io/docs/tasks/tools/"
        echo ""
        return 1
    fi
    
    # Check helm
    if ! command_exists helm; then
        log_error "Helm is not installed"
        echo ""
        echo "   Install Helm:"
        echo "   → macOS: brew install helm"
        echo "   → Linux: https://helm.sh/docs/intro/install/"
        echo ""
        return 1
    fi
    
    # Check Docker
    if ! verify_docker_running; then
        return 1
    fi
    
    # Check Kind
    if ! verify_kind_installed; then
        return 1
    fi
    
    log_success "All prerequisites verified"
    return 0
}
