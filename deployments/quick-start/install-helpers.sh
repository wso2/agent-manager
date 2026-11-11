#!/bin/bash
# Helper functions for Agent Management Platform installation
# This file provides functions to install AMP helm charts from public registry

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

# Version
VERSION="${VERSION:-0.0.0-dev}"

# Helm chart registry and versions
HELM_CHART_REGISTRY="${HELM_CHART_REGISTRY:-ghcr.io/wso2}"

# Registry authentication helpers: the single source of truth for the registry
# host, the pull-secret name and the helm flags a private-registry install
# needs. Resolved the same two ways as DEPLOYMENTS_DIR in install.sh — one level
# up in a repo checkout, flat alongside in the dev-container image.
_amp_lib_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for _candidate in \
    "${DEPLOYMENTS_DIR:-}/scripts/lib-registry-auth.sh" \
    "${_amp_lib_dir}/../scripts/lib-registry-auth.sh" \
    "${_amp_lib_dir}/scripts/lib-registry-auth.sh"; do
    if [[ -n "$_candidate" && -f "$_candidate" ]]; then
        # shellcheck source=../scripts/lib-registry-auth.sh
        source "$_candidate"
        break
    fi
done
unset _amp_lib_dir _candidate

# Chart names
AMP_CHART_NAME="wso2-agent-manager"
OBSERVABILITY_CHART_NAME="wso2-amp-observability-extension"
THUNDER_EXTENSION_CHART_NAME="wso2-amp-thunder-extension"
GATEWAY_EXTENSION_CHART_NAME="wso2-amp-api-platform-gateway-extension"

AMP_NS="${AMP_NS:-wso2-amp}"
OBSERVABILITY_NS="${OBSERVABILITY_NS:-amp-observability}"
DEFAULT_NS="${DEFAULT_NS:-default}"
THUNDER_NS="${THUNDER_NS:-amp-thunder}"

# Helm arguments arrays (initialize if not set)
if [[ -z "${AMP_HELM_ARGS+x}" ]]; then
    AMP_HELM_ARGS=()
fi
if [[ -z "${OBSERVABILITY_HELM_ARGS+x}" ]]; then
    OBSERVABILITY_HELM_ARGS=()
fi
if [[ -z "${THUNDER_HELM_ARGS+x}" ]]; then
    # thunder.setup.admin.password fixes the console admin login at "admin"
    # for local convenience — the chart's own default (unset) would generate
    # a random one instead, which production wants but is unnecessary friction
    # on a machine only the person running this installer can reach.
    THUNDER_HELM_ARGS=(--set thunder.setup.admin.password=admin)
fi
if [[ -z "${GATEWAY_HELM_ARGS+x}" ]]; then
    GATEWAY_HELM_ARGS=()
fi

# A private-registry install has to tell each chart that runs a first-party
# image where to pull it from and which pull secret to use. amp_pull_secret_helm_args
# prints nothing for a public install, so these arrays are left untouched there
# and every existing helm invocation stays byte-identical.
if declare -F amp_pull_secret_helm_args >/dev/null 2>&1; then
    while IFS= read -r _reg_arg; do
        [[ -n "$_reg_arg" ]] || continue
        AMP_HELM_ARGS+=("$_reg_arg")
        OBSERVABILITY_HELM_ARGS+=("$_reg_arg")
    done < <(amp_pull_secret_helm_args)
    unset _reg_arg
fi

# Timeouts (in seconds)
TIMEOUT_AMP_INSTALL=1800
TIMEOUT_DEPLOYMENT=600

# ============================================================================
# HELPER FUNCTIONS
# ============================================================================

# Fallback logging functions (can be overridden by sourcing script)
if ! declare -f log_error >/dev/null 2>&1; then
    log_error() {
        echo "ERROR: $1" >&2
    }
fi

if ! declare -f log_warning >/dev/null 2>&1; then
    log_warning() {
        echo "WARNING: $1" >&2
    }
fi

# Check if helm release exists
helm_release_exists() {
    local release="$1"
    local namespace="$2"
    helm status "${release}" -n "${namespace}" &>/dev/null
}

# Wait for a deployment to be available
wait_for_deployment() {
    local deployment="$1"
    local namespace="$2"
    local timeout="${3:-600}"

    if kubectl wait --for=condition=Available deployment/"${deployment}" -n "${namespace}" --timeout="${timeout}s" &>/dev/null; then
        return 0
    else
        return 1
    fi
}

# Wait for statefulset to be ready
wait_for_statefulset() {
    local statefulset="$1"
    local namespace="$2"
    local timeout="${3:-600}"

    # Get the desired replica count
    local replicas
    replicas=$(kubectl get statefulset/"${statefulset}" -n "${namespace}" -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "1")
 
    if kubectl wait --for=jsonpath="{.status.readyReplicas}"="${replicas}" statefulset/"${statefulset}" -n "${namespace}" --timeout="${timeout}s" &>/dev/null; then
        return 0
    else
        return 1
    fi
}

# Install helm chart with idempotency check
install_amp_helm_chart() {
    local release_name="$1"
    local chart_ref="$2"
    local namespace="$3"
    local timeout="${4:-1800}"
    shift 4
    local extra_args=("$@")

    # Check if release already exists
    if helm_release_exists "${release_name}" "${namespace}"; then
        return 0
    fi

    # Install the chart
    if helm install "${release_name}" "${chart_ref}" \
        --namespace "${namespace}" \
        --create-namespace \
        --timeout "${timeout}s" \
        "${extra_args[@]}"; then
        return 0
    else
        return 1
    fi
}

# ============================================================================
# INSTALLATION FUNCTIONS
# ============================================================================

# Install Agent Management Platform
# Create the image pull secret in every namespace that runs a first-party image.
# A no-op for a public install. Called before the first chart install, because a
# secret that arrives after the pod does still leaves it in ImagePullBackOff.

install_registry_credentials() {
    if ! declare -F amp_registry_is_private >/dev/null 2>&1; then
        return 0
    fi
    if ! amp_registry_is_private; then
        return 0
    fi
    log_info "Creating image pull secret for $(amp_registry_host)..."
    if amp_create_pull_secret "${AMP_NS}" "${OBSERVABILITY_NS}"; then
        log_success "Image pull secret created"
        return 0
    fi
    log_error "Failed to create the image pull secret"
    return 1
}

# Quick start runs the event bus inside the service (embedded NATS, in memory)
# instead of a NATS pod: one less workload on a laptop cluster, at the cost of
# losing queued event deliveries on a restart. The chart defaults to events
# off; quick start explicitly enables embedded mode. Production users can
# explicitly enable a NATS server. Autoscaling is off, as embedded supports
# one replica only. AMP_HELM_ARGS comes last, so passing
# --set agentManagerService.config.events.mode=nats there switches a
# quick-start install back to a NATS server.
install_agent_management_platform() {
    local chart_ref="oci://${HELM_CHART_REGISTRY}/${AMP_CHART_NAME}"
    local chart_version="${VERSION}"
    local release_name="amp"
    local helm_log="/tmp/helm-amp-install.log"

    # Install Helm chart
    if ! install_amp_helm_chart "${release_name}" "${chart_ref}" "${AMP_NS}" "${TIMEOUT_AMP_INSTALL}" \
        --version "${chart_version}" \
        --set console.config.instrumentationUrl="http://default-default.gateway.localhost:19080/otel" \
        --set agentManagerService.config.amObserverPublicURL="http://traces.amp.localhost:11080" \
        --set agentManagerService.config.events.mode=embedded \
        --set agentManagerService.autoscaling.enabled=false \
        "${AMP_HELM_ARGS[@]}" >"${helm_log}" 2>&1; then
        echo "Helm installation log (last 50 lines):"
        tail -50 "${helm_log}" 2>/dev/null || cat "${helm_log}" 2>/dev/null || echo "Log file not available"
        echo ""
        echo "Helm release status:"
        helm status "${release_name}" -n "${AMP_NS}" 2>&1 || echo "Release not found"
        echo ""
        echo "Pods in namespace ${AMP_NS}:"
        kubectl get pods -n "${AMP_NS}" 2>&1 || echo "No pods found"
        echo ""
        echo "Events in namespace ${AMP_NS}:"
        kubectl get events -n "${AMP_NS}" --sort-by='.lastTimestamp' | tail -20 2>&1 || true
        return 1
    fi

    # Wait for PostgreSQL StatefulSet (Bitnami subchart uses release-name-postgresql)
    if ! wait_for_statefulset "${release_name}-postgresql" "${AMP_NS}" "${TIMEOUT_DEPLOYMENT}"; then
        echo "PostgreSQL StatefulSet failed to become ready"
        echo ""
        echo "PostgreSQL pod status:"
        kubectl get pods -n "${AMP_NS}" -l app.kubernetes.io/name=postgresql 2>&1 || true
        echo ""
        echo "PostgreSQL StatefulSet status:"
        kubectl get statefulset "${release_name}-postgresql" -n "${AMP_NS}" 2>&1 || true
        echo ""
        echo "PostgreSQL pod logs (if available):"
        kubectl logs -n "${AMP_NS}" -l app.kubernetes.io/name=postgresql --tail=30 2>&1 || true
        return 1
    fi

    # Wait for agent manager service (amp-api)
    if ! wait_for_deployment "amp-api" "${AMP_NS}" "${TIMEOUT_DEPLOYMENT}"; then
        echo "Agent Manager Service deployment failed to become ready"
        echo ""
        echo "Agent Manager Service pod status:"
        kubectl get pods -n "${AMP_NS}" -l app.kubernetes.io/component=agent-manager-service 2>&1 || true
        echo ""
        echo "Agent Manager Service pod logs:"
        kubectl logs -n "${AMP_NS}" -l app.kubernetes.io/component=agent-manager-service --tail=50 2>&1 || true
        return 1
    fi

    # Wait for console (amp-console)
    if ! wait_for_deployment "amp-console" "${AMP_NS}" "${TIMEOUT_DEPLOYMENT}"; then
        echo "Console deployment failed to become ready"
        echo ""
        echo "Console pod status:"
        kubectl get pods -n "${AMP_NS}" -l app.kubernetes.io/component=console 2>&1 || true
        echo ""
        echo "Console pod logs:"
        kubectl logs -n "${AMP_NS}" -l app.kubernetes.io/component=console --tail=50 2>&1 || true
        return 1
    fi

    return 0
}

# Install Observability Extension
install_observability_extension() {
    local chart_ref="oci://${HELM_CHART_REGISTRY}/${OBSERVABILITY_CHART_NAME}"
    local chart_version="${VERSION}"
    local release_name="amp-observability-traces"

    # Install Helm chart
    if ! install_amp_helm_chart "${release_name}" "${chart_ref}" "${OBSERVABILITY_NS}" "${TIMEOUT_AMP_INSTALL}" \
        --version "${chart_version}" \
        "${OBSERVABILITY_HELM_ARGS[@]}"; then
        return 1
    fi

    # Wait for amp-observer if enabled
    if kubectl get deployment amp-observer -n "${OBSERVABILITY_NS}" &>/dev/null; then
        if ! wait_for_deployment "amp-observer" "${OBSERVABILITY_NS}" "${TIMEOUT_DEPLOYMENT}"; then
            echo "Agent Manager Observer deployment failed to become ready"
            echo ""
            echo "Agent Manager Observer pod status:"
            kubectl get pods -n "${OBSERVABILITY_NS}" -l app.kubernetes.io/component=observer 2>&1 || true
            return 1
        fi
    fi

    return 0
}

install_amp_thunder_extension() {
    local chart_ref="oci://${HELM_CHART_REGISTRY}/${THUNDER_EXTENSION_CHART_NAME}"
    local chart_version="${VERSION}"
    local release_name="amp-thunder-extension"

    # Install Helm chart. The chart's agentManagerMcpBaseUrl/observerMcpBaseUrl
    # defaults are already this install's gateway origins, so no MCP resource
    # identifier override is needed here.
    if ! install_amp_helm_chart "${release_name}" "${chart_ref}" "${THUNDER_NS}" "${TIMEOUT_AMP_INSTALL}" \
        --version "${chart_version}" \
        "${THUNDER_HELM_ARGS[@]}"; then
        return 1
    fi

    return 0
}

# Verify prerequisites for AMP installation
# Note: This function uses logging functions from the main script (log_error, log_warning)
verify_amp_prerequisites() {
    # Trace storage (OpenSearch) lives in the observability namespace; the
    # observer and the gateway's /otel route depend on it.
    if ! kubectl get namespace "${OBSERVABILITY_NS}" &>/dev/null; then
        log_error "Observability namespace ${OBSERVABILITY_NS} not found"
        echo ""
        echo "Install the trace storage (deployments/scripts/lib-observability.sh) first."
        return 1
    fi

    if ! kubectl get statefulset opensearch-master -n "${OBSERVABILITY_NS}" &>/dev/null; then
        log_warning "OpenSearch not found in ${OBSERVABILITY_NS}"
        log_warning "Traces will not be stored without OpenSearch"
    fi

    return 0
}

# Install API Platform Gateway Extension
# Installs wso2-amp-api-platform-gateway-extension which:
#   1. Runs a bootstrap Job to register the gateway in Agent Manager and generate a token
#   2. Deploys an APIGateway CR consumed by the gateway-operator to spin up the full stack
install_gateway_extension() {
    local chart_ref="oci://${HELM_CHART_REGISTRY}/${GATEWAY_EXTENSION_CHART_NAME}"
    local chart_version="${VERSION}"
    local release_name="api-platform-default-default"
    local gateway_vhost="http://default-default.gateway.localhost:19080"
    # Per-org-env namespace isolation: the default env's gateway stack lives in
    # its own "<org>-<env>" namespace.
    local gateway_namespace="default-default"
    local otel_restapi="${release_name}-otel-restapi"
    # gateway-operator's own child Helm release inserts a "-gw-" infix between
    # the APIGateway release name and each of its component names 
    local gateway_runtime_deployment="${release_name}-gw-gateway-gateway-runtime"

    # Create and label the namespace before Helm; the label marks AMP gateway
    # namespaces (NetworkPolicies and teardown select on it).
    if ! kubectl create namespace "${gateway_namespace}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null; then
        log_error "Failed to create gateway namespace ${gateway_namespace}"
        return 1
    fi
    if ! kubectl label namespace "${gateway_namespace}" \
        "amp.wso2.com/api-platform-gateway=true" --overwrite >/dev/null; then
        log_error "Failed to label gateway namespace ${gateway_namespace}"
        return 1
    fi

    # gateway-controller (1.2.0-beta+) mounts an AES-256 at-rest encryption key from
    # a Secret in the SAME namespace as the gateway release.
    local enc_secret_name="${GATEWAY_ENCRYPTION_SECRET_NAME:-gateway-encryption-keys}"
    local enc_secret_key="${GATEWAY_ENCRYPTION_SECRET_KEY:-default-aesgcm256-v1.bin}"

    if ! kubectl auth can-i get secrets -n "${gateway_namespace}" &>/dev/null; then
        log_error "Missing 'get' permission on secrets in '${gateway_namespace}' — required to detect an existing gateway encryption key secret. Grant get (in addition to create) on secrets in this namespace to the identity running this installer."
        return 1
    fi

    local key_tmp
    key_tmp="$(mktemp)"
    if ! openssl rand 32 > "${key_tmp}"; then
        log_error "Failed to generate gateway encryption key"
        rm -f "${key_tmp}"
        return 1
    fi
    local enc_create_out enc_create_rc
    enc_create_out="$(kubectl create secret generic "${enc_secret_name}" -n "${gateway_namespace}" \
        "--from-file=${enc_secret_key}=${key_tmp}" 2>&1)" && enc_create_rc=0 || enc_create_rc=$?
    rm -f "${key_tmp}" # don't leave the plaintext key on disk
    if [ "${enc_create_rc}" -eq 0 ]; then
        log_success "Gateway encryption key secret created in '${gateway_namespace}'"
    elif kubectl get secret "${enc_secret_name}" -n "${gateway_namespace}" &>/dev/null; then
        log_info "Gateway encryption key secret '${enc_secret_name}' already exists in '${gateway_namespace}', leaving it untouched."
    else
        log_error "Failed to create gateway encryption key secret '${enc_secret_name}' in '${gateway_namespace}': ${enc_create_out}"
        return 1
    fi

    # Install Helm chart. apiGateway.namespace drives where the chart renders
    # the APIGateway CR, config, RestApis, kgateway backendRef and token secret
    # — it must match the release namespace.
    if ! install_amp_helm_chart "${release_name}" "${chart_ref}" "${gateway_namespace}" "${TIMEOUT_AMP_INSTALL}" \
        --version "${chart_version}" \
        --set apiGateway.namespace="${gateway_namespace}" \
        --set agentManager.orgName=default \
        --set gateway.environment=default \
        --set gateway.vhost="${gateway_vhost}" \
        "${GATEWAY_HELM_ARGS[@]}"; then
        return 1
    fi

    # Wait for the bootstrap job to complete (the Helm hook runs asynchronously)
    log_info "Waiting for gateway bootstrap job to complete..."
    if ! kubectl wait --for=condition=complete "job/${release_name}-bootstrap" \
        -n "${gateway_namespace}" --timeout=300s 2>/dev/null; then
        log_error "Gateway bootstrap job did not complete within 300s"
        return 1
    fi

    # Registration completing does not mean the generated runtime is accepting
    # traffic yet. Wait for the operator, runtime deployment, and chart-managed
    # OTEL route before reporting the quick-start installation as ready.
    log_info "Waiting for API Platform Gateway to be programmed..."
    if ! kubectl wait --for=condition=Programmed "apigateway/${release_name}" \
        -n "${gateway_namespace}" --timeout=300s; then
        log_error "API Platform Gateway did not become Programmed within 300s"
        return 1
    fi

    log_info "Waiting for gateway runtime to become available..."
    if ! kubectl wait --for=condition=Available "deployment/${gateway_runtime_deployment}" \
        -n "${gateway_namespace}" --timeout=300s; then
        log_error "Gateway runtime did not become available within 300s"
        return 1
    fi

    log_info "Waiting for OTEL ingest RestApi to be programmed..."
    if ! kubectl wait --for=condition=Programmed "restapi/${otel_restapi}" \
        -n "${gateway_namespace}" --timeout=300s; then
        log_error "OTEL ingest RestApi did not become Programmed within 300s"
        return 1
    fi

    return 0
}

# Pin the serverlb's container IP so it cannot drift onto the server node's address. 
pin_serverlb_ip() {
    local network="k3d-${CLUSTER_NAME}"
    local lb="k3d-${CLUSTER_NAME}-serverlb"
    local current pinned

    if ! docker container inspect "$lb" &>/dev/null; then
        return 0    # cluster created with --no-lb
    fi

    pinned="$(docker container inspect "$lb" \
        --format "{{with (index .NetworkSettings.Networks \"${network}\")}}{{with .IPAMConfig}}{{.IPv4Address}}{{end}}{{end}}" 2>/dev/null)"
    if [ -n "$pinned" ]; then
        return 0    # already static
    fi

    current="$(docker container inspect "$lb" \
        --format "{{with (index .NetworkSettings.Networks \"${network}\")}}{{.IPAddress}}{{end}}" 2>/dev/null)"
    if [ -z "$current" ]; then
        log_warning "Could not read the loadbalancer address; leaving it dynamic"
        return 0
    fi

    docker stop "$lb" >/dev/null 2>&1 || true
    docker network disconnect "$network" "$lb" >/dev/null 2>&1 || true
    if ! docker network connect --ip "$current" "$network" "$lb" >/dev/null 2>&1; then
        # Pinning is hardening, not correctness: an unpinned but running cluster is
        # fine, so restore the dynamic attachment rather than failing the install.
        docker network connect "$network" "$lb" >/dev/null 2>&1 || true
        if ! docker start "$lb" >/dev/null 2>&1; then
            log_error "Could not restart ${lb}; the cluster API will be unreachable"
            return 1
        fi
        log_warning "Could not pin the loadbalancer to ${current}; left it dynamic"
        return 0
    fi
    # This function stopped a running container. Failing to bring it back leaves the
    # cluster API unreachable, so report that instead of claiming success.
    if ! docker start "$lb" >/dev/null 2>&1; then
        log_error "Could not restart ${lb} after pinning to ${current}; the cluster API will be unreachable"
        return 1
    fi
    log_success "Loadbalancer pinned to ${current}"
}
