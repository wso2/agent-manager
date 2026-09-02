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

# Chart names
AMP_CHART_NAME="wso2-agent-manager"
OBSERVABILITY_CHART_NAME="wso2-amp-observability-extension"
PLATFORM_RESOURCES_CHART_NAME="wso2-amp-platform-resources-extension"
THUNDER_EXTENSION_CHART_NAME="wso2-amp-thunder-extension"
EVALUATION_CHART_NAME="wso2-amp-evaluation-extension"
GATEWAY_EXTENSION_CHART_NAME="wso2-amp-api-platform-gateway-extension"

# Agent Sandbox community module (openchoreo registry, versioned independently of AMP)
AGENT_SANDBOX_CHART_REF="oci://ghcr.io/openchoreo/helm-charts/agent-sandbox"
AGENT_SANDBOX_MODULE_VERSION="${AGENT_SANDBOX_MODULE_VERSION:-0.1.1}"
AGENT_SANDBOX_UPSTREAM_VERSION="${AGENT_SANDBOX_UPSTREAM_VERSION:-v0.4.6}"

# Namespace definitions
AMP_NS="${AMP_NS:-wso2-amp}"
OBSERVABILITY_NS="${OBSERVABILITY_NS:-openchoreo-observability-plane}"
DEFAULT_NS="${DEFAULT_NS:-default}"
DATA_PLANE_NS="${DATA_PLANE_NS:-openchoreo-data-plane}"
THUNDER_NS="${THUNDER_NS:-amp-thunder}"
EVALUATION_NS="${EVALUATION_NS:-openchoreo-workflow-plane}"
BUILD_CI_NS="${BUILD_CI_NS:-openchoreo-workflow-plane}"

# Helm arguments arrays (initialize if not set)
if [[ -z "${AMP_HELM_ARGS+x}" ]]; then
    AMP_HELM_ARGS=()
fi
if [[ -z "${OBSERVABILITY_HELM_ARGS+x}" ]]; then
    OBSERVABILITY_HELM_ARGS=()
fi
if [[ -z "${PLATFORM_RESOURCES_HELM_ARGS+x}" ]]; then
    PLATFORM_RESOURCES_HELM_ARGS=()
fi
if [[ -z "${THUNDER_HELM_ARGS+x}" ]]; then
    # thunder.setup.admin.password fixes the console admin login at "admin"
    # for local convenience — the chart's own default (unset) would generate
    # a random one instead, which production wants but is unnecessary friction
    # on a machine only the person running this installer can reach.
    THUNDER_HELM_ARGS=(--set thunder.setup.admin.password=admin)
fi
if [[ -z "${EVALUATION_HELM_ARGS+x}" ]]; then
    EVALUATION_HELM_ARGS=()
fi
if [[ -z "${GATEWAY_HELM_ARGS+x}" ]]; then
    GATEWAY_HELM_ARGS=()
fi
if [[ -z "${CP_HELM_ARGS+x}" ]]; then
    CP_HELM_ARGS=()
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

# Provision a per-environment Thunder instance for the default environment.
# Downloads add-environment-thunder.sh from the published release and runs it using
# ITS OWN default chart: the upstream ThunderID release chart
# (oci://ghcr.io/thunder-id/helm-charts/thunderid), NOT the agent-manager
# wso2-amp-thunder-extension chart. CHART_VERSION is intentionally left unset here
# so the script pins its own validated ThunderID version — the agent-manager
# release VERSION has no bearing on which ThunderID release env-Thunder runs.
install_default_env_thunder() {
    # Prefer the copy bundled next to the installer (DEPLOYMENTS_DIR is set by
    # install.sh). Fetching it from raw.githubusercontent.com is a fallback for
    # standalone use; GitHub rate-limits unauthenticated per-IP requests (429),
    # so avoid the network whenever the bundled script is present.
    local bundled_script="${DEPLOYMENTS_DIR:-}/scripts/add-environment-thunder.sh"
    local script_path tmp_script="" script_base_url=""

    if [[ -n "${DEPLOYMENTS_DIR:-}" && -f "${bundled_script}" ]]; then
        script_path="${bundled_script}"
    else
        local script_url="https://raw.githubusercontent.com/wso2/agent-manager/amp/v${VERSION}/deployments/scripts/add-environment-thunder.sh"
        tmp_script="$(mktemp)"
        if ! curl -fsSL --connect-timeout 30 "${script_url}" -o "${tmp_script}" 2>/dev/null; then
            echo "Failed to download add-environment-thunder.sh from ${script_url}"
            rm -f "${tmp_script}"
            return 1
        fi
        script_path="${tmp_script}"
        # Not bundled, so it runs from a temp file with no local siblings — pass
        # SCRIPT_BASE_URL (derived from script_url) so its own thunder-naming.sh/
        # ams-auth.sh fetches use this SAME release ref instead of defaulting to main.
        script_base_url="$(dirname "$script_url")"
    fi

    # AMP_API_URL and IDP_TOKEN_URL address the host-facing ingress (this runs
    # off-cluster, not on the gateway Job's in-cluster DNS); AMS is confirmed
    # healthy before this runs. The localhost defaults hold only where the routes
    # are still bound to *.amp.localhost — a deployment that rehosts them (the VM
    # installers publish *.amp.<host> instead) must export both, or the route
    # match fails with a 404 and no env-Thunder is ever created.
    ENV_NAME=default \
        DISPLAY_NAME="Default" \
        ORG_NAME=default \
        THUNDER_HANDLE=default-idp \
        AMP_API_URL="${AMP_API_URL:-http://api.amp.localhost:8080/api/v1}" \
        IDP_TOKEN_URL="${IDP_TOKEN_URL:-http://thunder.amp.localhost:8080/oauth2/token}" \
        SCRIPT_BASE_URL="${script_base_url}" \
        bash "${script_path}"
    local status=$?
    [[ -n "${tmp_script}" ]] && rm -f "${tmp_script}"
    if [[ $status -ne 0 ]]; then
        return $status
    fi

    # Learn the handle add-environment-thunder.sh's register_thunder_url actually
    # stored via GET — this process has no other way to know it (a re-run
    # reuses whatever was registered on the first run, not necessarily
    # THUNDER_HANDLE above). Every later consumer of the default environment's
    # Thunder address (the gateway wiring below, and install.sh's completion
    # banner) reads this SAME global instead of re-deriving anything, so they
    # can't drift out of sync with each other.
    if ! DEFAULT_ENV_THUNDER_HANDLE="$(AMP_API_URL="${AMP_API_URL:-http://api.amp.localhost:8080/api/v1}" \
        IDP_TOKEN_URL="${IDP_TOKEN_URL:-http://thunder.amp.localhost:8080/oauth2/token}" \
        get_thunder_url_handle default default)"; then
        echo "Provisioned the default environment's Thunder instance, but could not learn its"
        echo "registered URL handle — cannot safely wire the gateway/print its console URL."
        return 1
    fi
    return 0
}

# Install AMP Thunder Extension
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

# Install Evaluation Extension
install_evaluation_extension() {
    local chart_ref="oci://${HELM_CHART_REGISTRY}/${EVALUATION_CHART_NAME}"
    local chart_version="${VERSION}"
    local release_name="amp-evaluation-extension"

    ensure_workflows_namespace

    # The eval pod runs untrusted evaluator code, so scope its API-server egress to the k3d node
    # network rather than taking the chart's RFC1918 default, which also spans pod and service CIDRs.
    local api_server_args=() node_cidr
    node_cidr=$(docker network inspect "k3d-${CLUSTER_NAME}" \
        --format '{{ (index .IPAM.Config 0).Subnet }}' 2>/dev/null || echo "")
    if [[ -n "$node_cidr" ]]; then
        api_server_args=(--set "networkPolicy.evaluationJob.apiServer.cidrs[0]=${node_cidr}")
    fi

    # Install Helm chart
    if ! install_amp_helm_chart "${release_name}" "${chart_ref}" "${EVALUATION_NS}" "${TIMEOUT_AMP_INSTALL}" \
        --version "${chart_version}" \
        ${api_server_args[@]+"${api_server_args[@]}"} \
        "${EVALUATION_HELM_ARGS[@]}"; then
        return 1
    fi

    return 0
}

# Install Agent Sandbox module (required — agents run as sandboxed pods
# rendered from SandboxTemplate/SandboxWarmPool CRDs this module provides)
install_agent_sandbox_module() {
    local release_name="agent-sandbox"

    # Install Helm chart
    if ! install_amp_helm_chart "${release_name}" "${AGENT_SANDBOX_CHART_REF}" "${DATA_PLANE_NS}" "${TIMEOUT_AMP_INSTALL}" \
        --version "${AGENT_SANDBOX_MODULE_VERSION}" \
        --wait \
        --set namespace=openchoreo-control-plane \
        --set dataPlaneNamespace="${DATA_PLANE_NS}" \
        --set dataPlaneServiceAccount=cluster-agent-dataplane \
        --set upstream.version="${AGENT_SANDBOX_UPSTREAM_VERSION}"; then
        return 1
    fi

    # Wait for the sandbox controller to come up
    if ! kubectl wait -n agent-sandbox-system \
        --for=condition=available \
        --timeout=180s \
        deployment/agent-sandbox-controller &>/dev/null; then
        return 1
    fi

    return 0
}

# OpenChoreo creates this only once a workflow has run there, so both charts that place a
# NetworkPolicy in it would otherwise fail; unowned, neither chart's lookup guard renders it.
ensure_workflows_namespace() {
    kubectl create namespace workflows-default --dry-run=client -o yaml | kubectl apply -f - >/dev/null
}

# Install Platform Resources Extension
install_platform_resources_extension() {
    local chart_ref="oci://${HELM_CHART_REGISTRY}/${PLATFORM_RESOURCES_CHART_NAME}"
    local chart_version="${VERSION}"
    local release_name="amp-platform-resources"

    ensure_workflows_namespace

    # Narrow the build netpol's API-server and registry egress to the k3d node network,
    # which carries both; the chart's RFC1918 default also spans pod and service CIDRs.
    local netpol_args=() node_cidr
    node_cidr=$(docker network inspect "k3d-${CLUSTER_NAME}" \
        --format '{{ (index .IPAM.Config 0).Subnet }}' 2>/dev/null || echo "")
    if [[ -n "$node_cidr" ]]; then
        netpol_args=(
            --set "networkPolicy.buildWorkflows.apiServer.cidrs[0]=${node_cidr}"
            --set "networkPolicy.buildWorkflows.registry.cidrs[0]=${node_cidr}"
        )
    fi

    # Install Helm chart
    if ! install_amp_helm_chart "${release_name}" "${chart_ref}" "${DEFAULT_NS}" "${TIMEOUT_AMP_INSTALL}" \
        --version "${chart_version}" \
        ${netpol_args[@]+"${netpol_args[@]}"} \
        "${PLATFORM_RESOURCES_HELM_ARGS[@]}"; then
        return 1
    fi

    return 0
}

# Verify prerequisites for AMP installation
# Note: This function uses logging functions from the main script (log_error, log_warning)
verify_amp_prerequisites() {
    # Check if OpenChoreo Observability Plane is available
    if ! kubectl get namespace "${OBSERVABILITY_NS}" &>/dev/null; then
        log_error "OpenChoreo Observability Plane not found"
        echo ""
        echo "The Agent Management Platform requires OpenChoreo Observability Plane."
        echo "Please install it first."
        return 1
    fi

    # Check if OpenChoreo Workflow Plane is available
    if ! kubectl get namespace "${BUILD_CI_NS}" &>/dev/null; then
        log_error "OpenChoreo Workflow Plane not found"
        echo ""
        echo "The Agent Management Platform requires OpenChoreo Workflow Plane."
        echo "Please install it first."
        return 1
    fi

    # Verify OpenSearch is accessible
    if ! kubectl get pods -n "${OBSERVABILITY_NS}" -l app=opensearch &>/dev/null; then
        log_warning "OpenSearch pods not found in observability plane"
        log_warning "Installation may fail without OpenSearch"
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
    local idp_skip_tls_verify="${IDP_SKIP_TLS_VERIFY:-true}"
    # Per-org-env namespace isolation: the default env's gateway stack lives in
    # its own "<org>-<env>" namespace, mirroring add-environment.sh.
    local gateway_namespace="default-default"
    local otel_restapi="${release_name}-otel-restapi"
    # gateway-operator's own child Helm release inserts a "-gw-" infix between
    # the APIGateway release name and each of its component names 
    local gateway_runtime_deployment="${release_name}-gw-gateway-gateway-runtime"

    # Sandboxed agents can egress only to namespaces carrying this label. Create
    # and label the namespace before Helm so the policy is effective as soon as
    # the gateway runtime starts.
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

    # Wire the gateway's ThunderKeyManager to the default environment's own Thunder
    # instance when it exists, mirroring the THUNDER_PROVISIONED logic in
    # add-environment.sh. keymanagers[0] is re-asserted alongside keymanagers[1]
    # because this install uses no -f values file, so --set on keymanagers[1]
    # alone would otherwise drop keymanagers[0] (verified via `helm template`).
    local thunder_args=()
    local thunder_release
    thunder_release="$(thunder_release_name default default)"
    if helm status "${thunder_release}" --namespace "${thunder_release}" &>/dev/null && [[ -n "${DEFAULT_ENV_THUNDER_HANDLE:-}" ]]; then
        local thunder_issuer_url thunder_jwks
        thunder_issuer_url="$(thunder_issuer "${DEFAULT_ENV_THUNDER_HANDLE}")"
        thunder_jwks="http://${thunder_release}-service.${thunder_release}.svc.cluster.local:8090/oauth2/jwks"
        thunder_args=(
            --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[0].name=agent-manager-service"
            --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[0].issuer=agent-manager-service"
            --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[0].jwks.remote.uri=http://amp-api.wso2-amp.svc.cluster.local:9000/auth/external/jwks.json"
            --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[0].jwks.remote.skipTlsVerify=true"
            --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].name=ThunderKeyManager"
            --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].issuer=${thunder_issuer_url}"
            --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].jwks.remote.uri=${thunder_jwks}"
            --set "apiGateway.config.policyConfigurations.jwtauth_v1.keymanagers[1].jwks.remote.skipTlsVerify=${idp_skip_tls_verify}"
            # Name must match keymanagers[].name, which is always "ThunderKeyManager" (set above).
            --set "bootstrap.identityProviders[0].name=ThunderKeyManager"
            --set "bootstrap.identityProviders[0].issuer=${thunder_issuer_url}"
            --set "bootstrap.identityProviders[0].jwksUri=${thunder_jwks}"
            --set "bootstrap.identityProviders[0].skipTlsVerify=${idp_skip_tls_verify}"
        )
    elif helm status "${thunder_release}" --namespace "${thunder_release}" &>/dev/null; then
        log_warning "Default environment Thunder release exists, but its registered URL handle is unknown"
        log_warning "— leaving the gateway on its default ThunderKeyManager instead of guessing an address."
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
        "${thunder_args[@]}" \
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

# ---------------------------------------------------------------------------
# Load the shared Thunder naming helpers (thunder_release_name/etc.) — the
# single source of truth for this derivation, see deployments/scripts/
# thunder-naming.sh. install.sh sources this file locally (never via
# curl | bash), so no network-fetch fallback is needed here — but the layout
# on disk differs between a repo checkout (scripts/ one level up from
# quick-start/) and the packaged quick-start image (scripts/ copied flat
# alongside install.sh — see the Dockerfile). DEPLOYMENTS_DIR is computed by
# install.sh to account for exactly this, and is already used the same way
# elsewhere in this file (install_default_env_thunder's bundled_script) — reuse
# it here too instead of hardcoding a checkout-only relative path.
# ---------------------------------------------------------------------------
source "${DEPLOYMENTS_DIR}/scripts/thunder-naming.sh"

# Load the shared AMS auth helpers (get_ams_token/get_thunder_url_handle) — see
# deployments/scripts/ams-auth.sh. Same local-only sourcing rationale as above:
# install.sh always has this file on disk (repo checkout or packaged image), so
# no network-fetch fallback is needed. get_thunder_url_handle is the single
# implementation every script uses to learn an already-provisioned env-Thunder's
# registered handle — see install_default_env_thunder above and DEFAULT_ENV_THUNDER_HANDLE.
source "${DEPLOYMENTS_DIR}/scripts/ams-auth.sh"

# Populated by install_default_env_thunder once it succeeds — the resolved handle
# (caller-supplied or server-generated) that register_thunder_url actually stored
# for the default environment. Every later consumer (the gateway wiring above, and
# install.sh's completion banner) reads this SAME global so they can't drift.
DEFAULT_ENV_THUNDER_HANDLE=""
