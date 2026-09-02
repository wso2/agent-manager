#!/bin/bash
set -euo pipefail

# Installs the AMP extension charts (everything from deployments/helm-charts that
# sits atop the OpenChoreo base). Called by setup-openchoreo.sh during full setup
# and by `make setup-amp` after a partial teardown (teardown-amp.sh).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
source "$SCRIPT_DIR/env.sh"
source "$SCRIPT_DIR/utils.sh"
PROJECT_ROOT="$1"

if ! kubectl cluster-info --context $CLUSTER_CONTEXT &> /dev/null; then
    echo "❌ K3d cluster '$CLUSTER_CONTEXT' is not running."
    exit 1
fi
kubectl config use-context $CLUSTER_CONTEXT >/dev/null

echo "=== Installing AMP Extensions ==="

# Two charts put a NetworkPolicy here and install in parallel below; created unowned, so
# neither chart's lookup guard renders it and neither hits "invalid ownership metadata".
echo "   Ensuring workflows-default namespace exists..."
kubectl create namespace workflows-default --dry-run=client -o yaml | kubectl apply -f -

# Pre-update helm dependencies (must run before parallel installs)
echo "   Updating Helm dependencies..."
helm dependency update "${SCRIPT_DIR}/../helm-charts/wso2-amp-thunder-extension"
echo "✅ Helm dependencies updated"
echo ""

install_thunder_extension() {
    echo "📦 Installing/Upgrading WSO2 AMP Thunder Extension..."

    # Detect an image mismatch and do a clean uninstall+install so the
    # pre-install setup job re-runs and re-bootstraps the database.
    local target_image="ghcr.io/thunder-id/thunderid:1.0.0"
    local selector="app.kubernetes.io/instance=amp-thunder-extension"
    if helm status amp-thunder-extension -n amp-thunder &>/dev/null; then
        local current_image
        current_image=$(kubectl get pods -n amp-thunder -l "$selector" \
            -o jsonpath='{range .items[*]}{.spec.containers[0].image}{"\n"}{end}' 2>/dev/null \
            | grep -v "^$" | head -1 || echo "")
        if [[ -z "$current_image" ]]; then
            echo "❌ Could not determine current Thunder image; refusing destructive reset"
            return 1
        fi
        if [[ "$current_image" != "$target_image" ]]; then
            echo "⚠️  Thunder version mismatch (installed: '${current_image}', target: '${target_image}')"
            echo "   Uninstalling for clean reinstall (setup job must re-run with new scope format)..."
            if ! helm uninstall amp-thunder-extension -n amp-thunder --wait --timeout=2m; then
                echo "❌ Failed to uninstall existing Thunder release; aborting clean reinstall"
                helm status amp-thunder-extension -n amp-thunder 2>/dev/null || true
                return 1
            fi

            # Explicitly delete the PVC so the setup job initialises a fresh database.
            if kubectl get pvc -n amp-thunder -l "$selector" -o name 2>/dev/null | grep -q .; then
                if ! kubectl delete pvc -n amp-thunder -l "$selector" --wait --timeout=60s; then
                    echo "❌ Failed to delete Thunder PVC(s); aborting to avoid reusing the stale database"
                    kubectl get pvc -n amp-thunder -l "$selector" 2>/dev/null || true
                    return 1
                fi
                # Confirm none linger (async delete / stuck finalizer)
                if kubectl get pvc -n amp-thunder -l "$selector" -o name 2>/dev/null | grep -q .; then
                    echo "❌ Thunder PVC(s) still present after delete; aborting clean reinstall"
                    return 1
                fi
            fi
            echo "✅ Existing Thunder release removed (database reset)"
        else
            echo "   Thunder is already at target version, skipping reinstall."
        fi
    fi

    # The dev stack runs amp-api on a published host port rather than behind the
    # gateway, so override the MCP resource identifier to that origin — Thunder
    # matches the authorize request's `resource` value exactly, and this dev
    # stack never serves the chart's own k3d-gateway default at all.
    # thunder.setup.admin.password fixes the console admin login at "admin" for
    # local dev convenience — the chart's own default (unset) would generate a
    # random one instead, which is what production wants but is annoying to
    # look up every time on a machine only you can reach anyway.
    helm upgrade --install amp-thunder-extension "${SCRIPT_DIR}/../helm-charts/wso2-amp-thunder-extension" \
        --namespace amp-thunder --create-namespace \
        --set thunder.bootstrap.agentManagerMcpBaseUrl=http://localhost:9000 \
        --set thunder.setup.admin.password=admin
    echo "✅ AMP Thunder Extension installed/upgraded successfully"
}

install_evaluation_workflows() {
    echo "📦 Installing/Upgrading Evaluation Workflows Extension..."

    # The k3d node network carries both the API server and the host-published agent-manager-service,
    # so it is the ipBlock for both NetworkPolicy exceptions; disable the policy if we can't compute it.
    local node_cidr network_policy_enabled
    node_cidr=$(docker network inspect "k3d-${CLUSTER_NAME}" \
        --format '{{ (index .IPAM.Config 0).Subnet }}' 2>/dev/null || echo "")
    if [[ -z "$node_cidr" ]]; then
        echo "⚠️  Could not determine k3d docker network subnet for k3d-${CLUSTER_NAME} — disabling"
        echo "   the evaluation-job egress NetworkPolicy for this install so local eval runs keep working."
        network_policy_enabled="false"
    else
        network_policy_enabled="true"
    fi

    helm upgrade --install amp-evaluation-workflows-extension "${SCRIPT_DIR}/../helm-charts/wso2-amp-evaluation-extension" \
        --namespace openchoreo-workflow-plane \
        --set ampEvaluation.image.repository="amp-evaluation-monitor" \
        --set ampEvaluation.publisher.endpoint="http://agent-manager-service:8080" \
        --set ampEvaluation.publisher.idpTokenUrl="http://amp-thunder-extension-service.amp-thunder.svc.cluster.local:8090/oauth2/token" \
        --set ampEvaluation.publisher.clientId="amp-publisher-client" \
        --set networkPolicy.evaluationJob.enabled="${network_policy_enabled}" \
        --set networkPolicy.evaluationJob.devEgress.cidr="${node_cidr}" \
        --set "networkPolicy.evaluationJob.devEgress.ports={8080,19080}" \
        --set "networkPolicy.evaluationJob.apiServer.cidrs[0]=${node_cidr}"
    echo "✅ Evaluation Workflows Extension installed/upgraded successfully"
}

install_platform_resources() {
    echo "📦 Installing/Upgrading Default Platform Resources..."
    echo "   Creating default Organization, Project, Environment, and DeploymentPipeline..."

    # Narrow the build netpol's API-server and registry egress to the k3d node network,
    # which carries both; disable the policy rather than let a failed derivation break builds.
    # The CIDRs are only passed when derived, so a later --reuse-values upgrade that turns the
    # policy back on cannot resurrect it with both rules emptied out.
    local node_cidr
    local -a netpol_args
    node_cidr=$(docker network inspect "k3d-${CLUSTER_NAME}" \
        --format '{{ (index .IPAM.Config 0).Subnet }}' 2>/dev/null || echo "")
    if [[ -z "$node_cidr" ]]; then
        echo "⚠️  Could not determine k3d docker network subnet for k3d-${CLUSTER_NAME} — disabling"
        echo "   the build-workflow egress NetworkPolicy for this install so local builds keep working."
        netpol_args=(--set networkPolicy.buildWorkflows.enabled=false)
    else
        netpol_args=(
            --set networkPolicy.buildWorkflows.enabled=true
            --set "networkPolicy.buildWorkflows.apiServer.cidrs[0]=${node_cidr}"
            --set "networkPolicy.buildWorkflows.registry.cidrs[0]=${node_cidr}"
        )
    fi

    helm upgrade --install amp-default-platform-resources "${SCRIPT_DIR}/../helm-charts/wso2-amp-platform-resources-extension" \
        --namespace default \
        "${netpol_args[@]}"
    echo "✅ Default Platform Resources installed/upgraded successfully"
}

echo "🚀 Starting PARALLEL installation of AMP extensions..."
echo ""

run_parallel_tasks \
    "Thunder Extension:install_thunder_extension" \
    "Evaluation Workflows:install_evaluation_workflows" \
    "Platform Resources:install_platform_resources" \
    || exit 1

echo "✅ All AMP extensions installed successfully"
echo ""

# ============================================================================
# Observability Extension (Agent Manager Observer)
# ============================================================================
echo "📦 Observability Extension (Agent Manager Observer)"
if ! helm status wso2-amp-observability-extension -n openchoreo-observability-plane &>/dev/null; then
    echo "Building and loading Agent Manager Observer Docker image into k3d cluster..."
    make -C ${PROJECT_ROOT}/agent-manager-observer docker-load-k3d
    sleep 10
fi
echo "   Installing/upgrading Agent Manager Observer (local dev: JWKS disabled, unverified JWT parse)..."
helm upgrade --install wso2-amp-observability-extension ${PROJECT_ROOT}/deployments/helm-charts/wso2-amp-observability-extension \
    --create-namespace \
    --namespace openchoreo-observability-plane \
    --timeout=10m \
    --set amObserver.developmentMode=true \
    --set amObserver.auth.isLocalDevEnv=true \
    --set-string amObserver.auth.jwksUrl=""
echo ""

wait_for_namespace_ready amp-thunder 'Thunder Extension'

# The gateway-operator is base-layer, but a stale install breaks the AMP
# gateway charts when the env.sh pin moves (their CRs need the newer CRDs) —
# reconcile it here so setup-amp works on long-lived clusters.
"${SCRIPT_DIR}/ensure-gateway-operator.sh"

echo "✅ AMP extensions ready"
