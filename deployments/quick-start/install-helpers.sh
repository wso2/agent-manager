#!/bin/bash
# Helper functions for Agent Management Platform installation
# This file provides functions to install AMP helm charts from public registry

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

# Version
VERSION="${VERSION:-0.2.5}"

# Helm chart registry and versions
HELM_CHART_REGISTRY="${HELM_CHART_REGISTRY:-ghcr.io/wso2}"

# Chart names
AMP_CHART_NAME="wso2-ai-agent-management-platform"
BUILD_CI_CHART_NAME="wso2-amp-build-extension"
OBSERVABILITY_CHART_NAME="wso2-amp-observability-extension"
PLATFORM_RESOURCES_CHART_NAME="wso2-amp-platform-resources-extension"

# Namespace definitions
AMP_NS="${AMP_NS:-wso2-amp}"
BUILD_CI_NS="${BUILD_CI_NS:-openchoreo-build-plane}"
OBSERVABILITY_NS="${OBSERVABILITY_NS:-openchoreo-observability-plane}"
DEFAULT_NS="${DEFAULT_NS:-default}"

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

# Timeouts (in seconds)
TIMEOUT_AMP_INSTALL=1800
TIMEOUT_DEPLOYMENT=600

# ============================================================================
# HELPER FUNCTIONS
# ============================================================================

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

    if kubectl wait --for=jsonpath='{.status.readyReplicas}'=1 statefulset/"${statefulset}" -n "${namespace}" --timeout="${timeout}s" &>/dev/null; then
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

    # Wait for traces-observer if enabled
    if kubectl get deployment amp-traces-observer -n "${OBSERVABILITY_NS}" &>/dev/null; then
        if ! wait_for_deployment "amp-traces-observer" "${OBSERVABILITY_NS}" "${TIMEOUT_DEPLOYMENT}"; then
            echo "Traces Observer Service deployment failed to become ready"
            echo ""
            echo "Traces Observer pod status:"
            kubectl get pods -n "${OBSERVABILITY_NS}" -l app.kubernetes.io/component=traces-observer 2>&1 || true
            return 1
        fi
    fi

    return 0
}

# Install Build Extension
install_build_extension() {
    local chart_ref="oci://${HELM_CHART_REGISTRY}/${BUILD_CI_CHART_NAME}"
    local chart_version="${VERSION}"
    local release_name="build-workflow-extensions"

    # Install Helm chart
    if ! install_amp_helm_chart "${release_name}" "${chart_ref}" "${BUILD_CI_NS}" "${TIMEOUT_AMP_INSTALL}" \
        --version "${chart_version}" \
        "${BUILD_CI_HELM_ARGS[@]}"; then
        return 1
    fi

    return 0
}


# Install Platform Resources Extension
install_platform_resources_extension() {
    local chart_ref="oci://${HELM_CHART_REGISTRY}/${PLATFORM_RESOURCES_CHART_NAME}"
    local chart_version="${VERSION}"
    local release_name="amp-platform-resources"

    # Install Helm chart
    if ! install_amp_helm_chart "${release_name}" "${chart_ref}" "${DEFAULT_NS}" "${TIMEOUT_AMP_INSTALL}" \
        --version "${chart_version}" \
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

    # Check if OpenChoreo Build Plane is available
    if ! kubectl get namespace "${BUILD_CI_NS}" &>/dev/null; then
        log_error "OpenChoreo Build Plane not found"
        echo ""
        echo "The Agent Management Platform requires OpenChoreo Build Plane."
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
