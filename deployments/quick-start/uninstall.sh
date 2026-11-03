#!/bin/bash
set -euo pipefail

# ============================================================================
# Agent Management Platform - Complete Uninstallation
# ============================================================================
# This script completely removes:
# 1. All Agent Management Platform helm releases, including each environment's
#    Thunder and gateway
# 2. All OpenChoreo and prerequisite helm releases (unless --amp-only is used)
# 3. All custom resources, the namespaces install.sh created, and (unless
#    --amp-only is used) the CRDs it installed and the CoreDNS rewrites it added
# 4. Optionally deletes the k3d cluster
# It exits non-zero and lists what is left if anything could not be removed.
#
# Usage:
#   ./uninstall.sh                    # Uninstall platform but keep cluster
#   ./uninstall.sh --delete-cluster   # Delete the k3d cluster
#   ./uninstall.sh --delete-colima    # Delete the 'agent-manager' Colima profile
#   ./uninstall.sh --amp-only         # Uninstall only AMP, keep OpenChoreo
# ============================================================================

# Configuration
CLUSTER_NAME="amp-local"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"
COLIMA_PROFILE="agent-manager"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Namespace definitions (match install-helpers.sh)
AMP_NS="${AMP_NS:-wso2-amp}"
BUILD_CI_NS="${BUILD_CI_NS:-openchoreo-workflow-plane}"
OBSERVABILITY_NS="${OBSERVABILITY_NS:-openchoreo-observability-plane}"
DEFAULT_NS="${DEFAULT_NS:-default}"
EVALUATION_NS="${EVALUATION_NS:-openchoreo-workflow-plane}"

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

# Parse command line arguments
DELETE_CLUSTER=false
DELETE_COLIMA=false
AMP_ONLY=false

for arg in "$@"; do
    case "$arg" in
        --delete-cluster|-d)
            DELETE_CLUSTER=true
            ;;
        --delete-colima)
            DELETE_COLIMA=true
            ;;
        --amp-only|--platform-only)
            AMP_ONLY=true
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --amp-only, --platform-only    Uninstall only AMP resources, keep OpenChoreo"
            echo "  --delete-cluster, -d           Delete the entire k3d cluster"
            echo "  --delete-colima                Delete the '${COLIMA_PROFILE}' Colima profile (requires --delete-cluster)"
            echo "  --help, -h                     Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                            # Uninstall everything but keep cluster"
            echo "  $0 --amp-only                 # Uninstall only AMP, keep OpenChoreo"
            echo "  $0 --delete-cluster           # Delete the entire k3d cluster"
            echo "  $0 --delete-cluster --delete-colima  # Delete cluster and Colima profile"
            exit 0
            ;;
        *)
            log_error "Unknown option: $arg"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Validate flag combinations: --delete-colima requires --delete-cluster because
# deleting the Colima VM while the k3d cluster is still running inside it would
# leave the cluster in a corrupt state.
if [ "${DELETE_COLIMA}" = true ] && [ "${DELETE_CLUSTER}" = false ]; then
    log_error "--delete-colima requires --delete-cluster (the k3d cluster runs inside the Colima VM)"
    log_info "Usage: $0 --delete-cluster --delete-colima"
    exit 1
fi

# ============================================================================
# MAIN UNINSTALLATION FLOW
# ============================================================================

# Fast path: when --delete-cluster is requested, skip all component uninstall
# steps and just delete the k3d cluster. Deleting the cluster removes all
# workloads, CRDs, and namespaces anyway, so running the helm/kubectl cleanup
# first is redundant and slow.
if [ "${DELETE_CLUSTER}" = true ]; then
    log_step "Deleting k3d Cluster"

    if ! command_exists k3d; then
        log_error "k3d is not installed, cannot delete cluster"
        exit 1
    fi

    if k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
        log_info "Deleting k3d cluster '${CLUSTER_NAME}'..."
        if k3d cluster delete "${CLUSTER_NAME}" &>/dev/null; then
            log_success "Cluster '${CLUSTER_NAME}' deleted"
        else
            log_error "Failed to delete cluster '${CLUSTER_NAME}'"
            exit 1
        fi
    else
        log_info "Cluster '${CLUSTER_NAME}' not found, nothing to do"
    fi

    # Delete Colima profile if requested
    if [ "${DELETE_COLIMA}" = true ]; then
        log_step "Deleting Colima Profile"

        if ! command_exists colima; then
            log_warning "colima is not installed, skipping profile deletion"
        else
            if colima list 2>/dev/null | grep -q "${COLIMA_PROFILE}"; then
                log_info "Stopping Colima profile '${COLIMA_PROFILE}'..."
                colima stop --profile "${COLIMA_PROFILE}" &>/dev/null || true
                log_info "Deleting Colima profile '${COLIMA_PROFILE}'..."
                if colima delete --profile "${COLIMA_PROFILE}" --force &>/dev/null; then
                    log_success "Colima profile '${COLIMA_PROFILE}' deleted"
                else
                    log_error "Failed to delete Colima profile '${COLIMA_PROFILE}'"
                fi
            else
                log_info "Colima profile '${COLIMA_PROFILE}' not found, nothing to do"
            fi
        fi
    fi

    echo ""
    log_info "To reinstall, run: ./install.sh"
    echo ""
    exit 0
fi

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

# Only ever act on the cluster install.sh created. Falling back to the current
# context could uninstall cert-manager and friends from an unrelated cluster.
if ! kubectl config get-contexts "${CLUSTER_CONTEXT}" &>/dev/null; then
    log_error "Cluster context '${CLUSTER_CONTEXT}' not found; nothing was changed"
    log_info "If the cluster is gone, there is nothing to uninstall"
    exit 1
fi
if ! kubectl config use-context "${CLUSTER_CONTEXT}" &>/dev/null; then
    log_error "Could not switch to context '${CLUSTER_CONTEXT}'; nothing was changed"
    exit 1
fi
# Every check below reads a failed call as "already removed", so an unreachable
# cluster would otherwise end in a success message with nothing removed.
if ! kubectl get --raw /readyz --request-timeout=15s &>/dev/null; then
    log_error "Cluster '${CLUSTER_NAME}' is not reachable; nothing was changed"
    log_info "Start it with: k3d cluster start ${CLUSTER_NAME} (and its Docker or Colima VM first)"
    exit 1
fi

# Releases install.sh creates, in reverse install order, as "<release> <namespace>".
# Each environment also gets a Thunder release and a gateway; those are found
# through their namespaces (environment_namespaces) rather than listed here.
AMP_RELEASES="
amp-evaluation-extension ${EVALUATION_NS}
amp-observability-traces ${OBSERVABILITY_NS}
amp-platform-resources ${DEFAULT_NS}
build-workflow-extensions ${BUILD_CI_NS}
agent-sandbox openchoreo-data-plane
amp ${AMP_NS}
amp-thunder-extension amp-thunder
"
OPENCHOREO_RELEASES="
gateway-operator openchoreo-data-plane
observability-traces-opensearch ${OBSERVABILITY_NS}
observability-metrics-prometheus ${OBSERVABILITY_NS}
observability-logs-opensearch ${OBSERVABILITY_NS}
openchoreo-observability-plane ${OBSERVABILITY_NS}
openchoreo-workflow-plane openchoreo-workflow-plane
registry openchoreo-workflow-plane
openchoreo-data-plane openchoreo-data-plane
openchoreo-control-plane openchoreo-control-plane
kgateway openchoreo-control-plane
kgateway-crds openchoreo-control-plane
openbao openbao
external-secrets external-secrets
cert-manager cert-manager
"
AMP_NAMESPACES="${AMP_NS} amp-thunder agent-sandbox-system"
OPENCHOREO_NAMESPACES="openchoreo-control-plane openchoreo-data-plane openchoreo-workflow-plane ${OBSERVABILITY_NS} cert-manager external-secrets openbao"
# API groups of the CRDs install.sh brings in (charts and the Gateway API bundle).
KUBELET_SERVICE="openchoreo-observability-kubelet"

# Entries install.sh adds to the coredns-custom ConfigMap, as "<key> <hostname
# it rewrites>": deployments/k8s/coredns-amp-custom.yaml on k3d, and
# render_coredns_vm_config in deployments/vm/lib-vm.sh on a VM.
# The cluster owner can keep entries of their own in the same ConfigMap.
COREDNS_ENTRIES="
openchoreo.override openchoreo\\.localhost
amp.override amp\\.localhost
agentmanager.override agentmanager\\.localhost
amgateway.override am-gateway\\.localhost
gateway.override gateway\\.localhost
hostdocker.override host\\.docker\\.internal
hostalias.override host\\.k3d\\.internal|host\\.docker\\.internal
"
COREDNS_RESTART_FAILED=false

# coredns_entries_ours: prints the installer's entries still in coredns-custom,
# one key per line: those whose key and rewrite both match. Returns 2 when the
# ConfigMap cannot be read, so a failed lookup is never taken for a clean one.
coredns_entries_ours() {
    local found key pattern value
    found="$(kubectl get configmap coredns-custom -n kube-system --ignore-not-found -o name 2>/dev/null)" || return 2
    [ -n "${found}" ] || return 0
    while read -r key pattern; do
        [ -n "${key}" ] || continue
        value="$(kubectl get configmap coredns-custom -n kube-system -o jsonpath="{.data.${key//./\\.}}" 2>/dev/null)" || return 2
        if printf '%s' "${value}" | grep -qF 'name regex' && printf '%s' "${value}" | grep -qF "${pattern}"; then
            printf '%s\n' "${key}"
        fi
    done <<< "${COREDNS_ENTRIES}"
}

# remove_coredns_entries: removes the installer's entries from coredns-custom,
# and the ConfigMap itself only when nothing else is left in it, then restarts
# CoreDNS so the rewrites stop taking effect.
remove_coredns_entries() {
    local ours total binary ops key
    ours="$(coredns_entries_ours)" || { log_warning "Could not read the coredns-custom ConfigMap"; return 0; }
    [ -n "${ours}" ] || return 0
    log_info "Removing the CoreDNS rewrites..."
    # A failed read must not look like an empty ConfigMap, or the whole
    # ConfigMap, the owner's entries included, would be deleted.
    if ! total="$(kubectl get configmap coredns-custom -n kube-system -o go-template='{{len .data}}' 2>/dev/null)" \
            || ! [[ "${total}" =~ ^[0-9]+$ ]]; then
        log_warning "Could not read the coredns-custom ConfigMap"
        return 0
    fi
    if ! binary="$(kubectl get configmap coredns-custom -n kube-system \
            -o go-template='{{with .binaryData}}{{len .}}{{else}}0{{end}}' 2>/dev/null)" \
            || ! [[ "${binary}" =~ ^[0-9]+$ ]]; then
        log_warning "Could not read the coredns-custom ConfigMap"
        return 0
    fi
    # Deleted whole only when everything in it is ours; binaryData never is.
    if [ "${total}" -le "$(printf '%s\n' "${ours}" | wc -l)" ] && [ "${binary}" -eq 0 ]; then
        kubectl delete configmap coredns-custom -n kube-system --ignore-not-found &>/dev/null || true
    else
        ops=""
        for key in ${ours}; do
            ops="${ops}${ops:+,}{\"op\":\"remove\",\"path\":\"/data/${key}\"}"
        done
        kubectl patch configmap coredns-custom -n kube-system --type=json -p "[${ops}]" &>/dev/null || true
    fi
    if ! { kubectl rollout restart deployment/coredns -n kube-system &>/dev/null \
            && kubectl rollout status deployment/coredns -n kube-system --timeout=90s &>/dev/null; }; then
        COREDNS_RESTART_FAILED=true
        log_warning "CoreDNS did not restart; it keeps the old rewrites until it does"
    fi
}

# clear_stuck_finalizers: a CRD stays in Terminating while an object of its
# kind still carries a finalizer, and once Step 3 has removed the controllers
# nothing will clear it. For the platform CRDs that are terminating, clears the
# finalizers of their objects that are being deleted, says which, and waits for
# the CRDs to go. Live CRDs and live objects are never touched.
clear_stuck_finalizers() {
    local stuck crd kind obj ns name finalizers
    stuck="$(kubectl get crd \
        -o jsonpath='{range .items[?(@.metadata.deletionTimestamp)]}{.metadata.name}{"\n"}{end}' 2>/dev/null \
        | grep -E "\.(${CRD_GROUPS})\$" || true)"
    [ -n "${stuck}" ] || return 0
    for kind in ${stuck}; do
        while IFS=' ' read -r obj finalizers; do
            [ -n "${obj}" ] && [ -n "${finalizers}" ] || continue
            ns="${obj%%/*}"
            name="${obj#*/}"
            log_warning "Clearing the finalizers of ${kind} ${obj#/}, whose controller is already uninstalled"
            kubectl patch "${kind}" "${name}" ${ns:+-n "${ns}"} --type=merge \
                -p '{"metadata":{"finalizers":null}}' &>/dev/null || true
        done <<< "$(kubectl get "${kind}" -A \
            -o jsonpath='{range .items[?(@.metadata.deletionTimestamp)]}{.metadata.namespace}/{.metadata.name} {.metadata.finalizers}{"\n"}{end}' 2>/dev/null)"
    done
    for crd in ${stuck}; do
        kubectl wait --for=delete "crd/${crd}" --timeout=60s &>/dev/null || true
    done
}

# object_left <description> <kubectl get args...>: adds the object to
# LEFT_OBJECTS when it still exists, or when it cannot be checked.
object_left() {
    local what="$1" found
    shift
    if found="$(kubectl get "$@" --ignore-not-found -o name 2>/dev/null)"; then
        [ -z "${found}" ] || LEFT_OBJECTS="${LEFT_OBJECTS} ${what}"
    else
        LEFT_OBJECTS="${LEFT_OBJECTS} ${what} (could not be checked)"
    fi
}

CRD_GROUPS='openchoreo\.dev|external-secrets\.io|cert-manager\.io|argoproj\.io|monitoring\.coreos\.com|agents\.x-k8s\.io|gateway\.api-platform\.wso2\.com|gateway\.kgateway\.dev|gateway\.networking\.k8s\.io|gateway\.networking\.x-k8s\.io'

# uninstall_release <release> <namespace>: quiet when the release is absent, so
# an install without an optional component does not print a line for it.
uninstall_release() {
    helm status "$1" -n "$2" &>/dev/null || return 0
    log_info "Uninstalling $1 (namespace $2)..."
    if helm uninstall "$1" -n "$2" --wait --timeout 5m &>/dev/null; then
        log_success "$1 uninstalled"
    else
        log_warning "Failed to uninstall $1 (non-fatal)"
    fi
}

uninstall_releases() {
    local release ns
    while read -r release ns; do
        [ -n "${release}" ] || continue
        uninstall_release "${release}" "${ns}"
    done <<< "$1"
}

# environment_namespaces: what install.sh and add-environment create per
# organization or environment, whose names carry the org and environment.
environment_namespaces() {
    {
        kubectl get namespaces -o name 2>/dev/null | sed 's#^namespace/##' \
            | grep -E '^(dp-|workflows-|openchoreo-kv-|amp-thunder-)' || true
        kubectl get namespaces -l amp.wso2.com/api-platform-gateway=true -o name 2>/dev/null \
            | sed 's#^namespace/##'
    } | sort -u
}

# existing_namespaces <name>...: the given namespaces that still exist,
# including any stuck in Terminating.
existing_namespaces() {
    local ns
    for ns in "$@"; do
        kubectl get namespace "${ns}" &>/dev/null && printf '%s\n' "${ns}"
    done
    return 0
}

delete_namespaces() {
    [ $# -gt 0 ] || return 0
    log_info "Deleting namespaces: $*"
    kubectl delete namespace "$@" --ignore-not-found --timeout=180s &>/dev/null \
        || log_warning "Some namespaces did not finish deleting within 3 minutes"
}

# ============================================================================
# Step 1: Delete custom resources and per-environment namespaces
# ============================================================================
# Done while the controllers are still running, so the finalizers on these
# resources are processed instead of leaving them stuck in deletion.

log_step "Step 1/4: Deleting Custom Resources and Environments"

log_info "Cleaning up AMP custom resources..."
for resource_type in organization project environment deploymentpipeline component workflowtemplate; do
    if kubectl get "${resource_type}" -A &>/dev/null; then
        kubectl delete "${resource_type}" --all --all-namespaces --timeout=30s &>/dev/null || true
    fi
done

ENV_NAMESPACES="$(environment_namespaces)"
for ns in ${ENV_NAMESPACES}; do
    # Base releases before their "-gw" gateway, which the base release's
    # Gateway resource owns.
    for release in $(helm list -n "${ns}" -q 2>/dev/null | sort); do
        uninstall_release "${release}" "${ns}"
    done
done
# shellcheck disable=SC2086 # one namespace per word
delete_namespaces ${ENV_NAMESPACES}

if [ "${AMP_ONLY}" = false ]; then
    # install.sh applies some of these with kubectl, so no release owns them,
    # and their cleanup finalizer needs the operator that Step 3 removes.
    if kubectl get externalsecrets.external-secrets.io -A &>/dev/null; then
        log_info "Deleting ExternalSecrets..."
        kubectl delete externalsecrets.external-secrets.io,pushsecrets.external-secrets.io \
            --all --all-namespaces --timeout=60s &>/dev/null || true
    fi
    for resource_type in clusterdataplane clusterworkflowplane clusterobservabilityplane; do
        if kubectl get "${resource_type}" &>/dev/null; then
            log_info "Deleting ${resource_type} resources..."
            kubectl delete "${resource_type}" --all --timeout=30s &>/dev/null || true
        fi
    done
    # Every other namespaced OpenChoreo kind, such as the WorkflowRuns of agent
    # builds and evaluation monitors. They live in the organization's namespace,
    # which is kept, and their cleanup finalizers need the controllers Step 3
    # removes. A kind that cannot be listed stops the uninstall here, before
    # those controllers go, so its cleanup can still run on a second attempt.
    UNCHECKED=""
    if ! OPENCHOREO_KINDS="$(kubectl api-resources --api-group=openchoreo.dev --namespaced=true -o name 2>/dev/null)"; then
        UNCHECKED=" the OpenChoreo resource kinds (discovery failed)"
    fi
    for resource_type in ${OPENCHOREO_KINDS}; do
        if ! found="$(kubectl get "${resource_type}" -A -o name 2>/dev/null)"; then
            UNCHECKED="${UNCHECKED} ${resource_type}"
        elif [ -n "${found}" ]; then
            log_info "Deleting ${resource_type}..."
            kubectl delete "${resource_type}" --all --all-namespaces --timeout=60s &>/dev/null || true
        fi
    done
    if [ -n "${UNCHECKED}" ]; then
        log_error "Could not check${UNCHECKED}"
        log_info "Stopped before removing the controllers that clean these up. Fix the cluster access and run uninstall.sh again."
        exit 1
    fi
fi

# ============================================================================
# Step 2: Uninstall Agent Management Platform Helm Releases
# ============================================================================

log_step "Step 2/4: Uninstalling Agent Management Platform"
uninstall_releases "${AMP_RELEASES}"
# Created outside the releases' tracked resources (the sandbox controller's
# kubectl-applied manifest and a Helm hook), so uninstalling leaves them behind.
kubectl delete clusterrolebinding,clusterrole \
    agent-sandbox-controller agent-sandbox-controller-extensions \
    --ignore-not-found &>/dev/null || true
kubectl delete clusterrolebinding label-default-namespace-rolebinding --ignore-not-found &>/dev/null || true
kubectl delete clusterrole label-default-namespace-role --ignore-not-found &>/dev/null || true
kubectl delete serviceaccount label-default-namespace-sa -n "${DEFAULT_NS}" --ignore-not-found &>/dev/null || true

# ============================================================================
# Step 3: Uninstall OpenChoreo and Prerequisites (Skip if --amp-only)
# ============================================================================

if [ "${AMP_ONLY}" = false ]; then
    log_step "Step 3/4: Uninstalling OpenChoreo and Prerequisites"
    uninstall_releases "${OPENCHOREO_RELEASES}"
    # Applied with kubectl by install.sh, not owned by any release.
    kubectl delete clusterrolebinding,clusterrole wso2-api-platform-gateway-module \
        --ignore-not-found &>/dev/null || true
    # Created by the Prometheus operator for kubelet scraping, so no release owns it.
    kubectl delete service,endpoints "${KUBELET_SERVICE}" -n kube-system --ignore-not-found &>/dev/null || true
    kubectl delete endpointslices -n kube-system -l "kubernetes.io/service-name=${KUBELET_SERVICE}" \
        --ignore-not-found &>/dev/null || true
    remove_coredns_entries
else
    log_step "Step 3/4: Skipping OpenChoreo Uninstallation (--amp-only mode)"
    log_info "OpenChoreo resources are preserved"
fi

# ============================================================================
# Step 4: Clean Up Namespaces and CRDs
# ============================================================================

log_step "Step 4/4: Cleaning Up Namespaces and CRDs"

if [ "${AMP_ONLY}" = true ]; then
    PLATFORM_NAMESPACES="${AMP_NAMESPACES}"
else
    PLATFORM_NAMESPACES="${AMP_NAMESPACES} ${OPENCHOREO_NAMESPACES}"
fi
# shellcheck disable=SC2046,SC2086 # one namespace per word
delete_namespaces $(existing_namespaces ${PLATFORM_NAMESPACES})

if [ "${AMP_ONLY}" = false ]; then
    PLATFORM_CRDS="$(kubectl get crd -o name 2>/dev/null | grep -E "\.(${CRD_GROUPS})\$" || true)"
    if [ -n "${PLATFORM_CRDS}" ]; then
        log_info "Deleting $(printf '%s\n' "${PLATFORM_CRDS}" | wc -l | tr -d ' ') CRDs..."
        # shellcheck disable=SC2086 # one CRD per word
        kubectl delete ${PLATFORM_CRDS} --ignore-not-found --timeout=120s &>/dev/null \
            || log_warning "Some CRDs did not finish deleting within 2 minutes"
    fi
    clear_stuck_finalizers
fi

# ============================================================================
# VERIFICATION
# ============================================================================
# Report what is actually left rather than assuming the steps above worked.

log_step "Verification"

LEFT_RELEASES=""
while read -r release ns; do
    [ -n "${release}" ] || continue
    helm status "${release}" -n "${ns}" &>/dev/null && LEFT_RELEASES="${LEFT_RELEASES} ${ns}/${release}"
done <<< "${AMP_RELEASES}$([ "${AMP_ONLY}" = false ] && printf '%s' "${OPENCHOREO_RELEASES}")"
# shellcheck disable=SC2086 # one namespace per word
LEFT_NAMESPACES="$( { existing_namespaces ${PLATFORM_NAMESPACES}; environment_namespaces; } | sort -u | tr '\n' ' ')"
LEFT_CRDS=""
if [ "${AMP_ONLY}" = false ]; then
    LEFT_CRDS="$(kubectl get crd -o name 2>/dev/null | grep -cE "\.(${CRD_GROUPS})\$" || true)"
    [ "${LEFT_CRDS}" != "0" ] || LEFT_CRDS=""
fi
# Objects outside the platform namespaces that no release owns.
LEFT_OBJECTS=""
object_left "serviceaccount/${DEFAULT_NS}/label-default-namespace-sa" \
    serviceaccount label-default-namespace-sa -n "${DEFAULT_NS}"
if [ "${AMP_ONLY}" = false ]; then
    object_left "service/kube-system/${KUBELET_SERVICE}" service "${KUBELET_SERVICE}" -n kube-system
    object_left "endpoints/kube-system/${KUBELET_SERVICE}" endpoints "${KUBELET_SERVICE}" -n kube-system
    object_left "endpointslices of ${KUBELET_SERVICE}" endpointslices -n kube-system \
        -l "kubernetes.io/service-name=${KUBELET_SERVICE}"
    if LEFT_COREDNS="$(coredns_entries_ours)"; then
        [ -z "${LEFT_COREDNS}" ] \
            || LEFT_OBJECTS="${LEFT_OBJECTS} coredns-custom entries ($(printf '%s' "${LEFT_COREDNS}" | tr '\n' ' ' | sed 's/ $//'))"
    else
        LEFT_OBJECTS="${LEFT_OBJECTS} configmap/kube-system/coredns-custom (could not be checked)"
    fi
    [ "${COREDNS_RESTART_FAILED}" = false ] \
        || LEFT_OBJECTS="${LEFT_OBJECTS} CoreDNS restart (retry: kubectl rollout restart deployment/coredns -n kube-system)"
fi

if [ -z "${LEFT_RELEASES}" ] && [ -z "${LEFT_NAMESPACES// /}" ] && [ -z "${LEFT_CRDS}" ] && [ -z "${LEFT_OBJECTS}" ]; then
    log_step "Uninstallation Complete!"
    if [ "${AMP_ONLY}" = true ]; then
        log_success "AMP resources have been removed!"
        log_info "OpenChoreo resources have been preserved."
    else
        log_success "All platform resources have been removed!"
    fi
    log_info "Cluster '${CLUSTER_NAME}' has been preserved."
    echo ""
    log_info "To reinstall, run: ./install.sh"
    if [ "${AMP_ONLY}" = false ]; then
        log_info "To also delete the cluster: ./uninstall.sh --delete-cluster"
        log_info "To delete everything including Colima profile: ./uninstall.sh --delete-cluster --delete-colima"
    fi
    echo ""
    exit 0
fi

log_step "Uninstallation Incomplete"
[ -z "${LEFT_RELEASES}" ] || log_warning "Helm releases still installed:${LEFT_RELEASES}"
[ -z "${LEFT_NAMESPACES// /}" ] || log_warning "Namespaces still present: ${LEFT_NAMESPACES}"
[ -z "${LEFT_CRDS}" ] || log_warning "CRDs still present: ${LEFT_CRDS}"
[ -z "${LEFT_OBJECTS}" ] || log_warning "Other objects still present:${LEFT_OBJECTS}"
echo ""
log_info "Installing again on top of these leftovers is not supported."
log_info "For a clean slate, run: ./uninstall.sh --delete-cluster"
echo ""
exit 1
