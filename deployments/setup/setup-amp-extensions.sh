#!/bin/bash
set -euo pipefail

# Installs the AMP extension charts that run in the k3d cluster (Thunder) and
# reconciles the API Platform Gateway operator. Called by `make setup-amp`,
# both during full setup and after a partial teardown (teardown-amp.sh).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
source "$SCRIPT_DIR/env.sh"
source "$SCRIPT_DIR/utils.sh"

if ! kubectl cluster-info --context $CLUSTER_CONTEXT &> /dev/null; then
    echo "❌ K3d cluster '$CLUSTER_CONTEXT' is not running."
    exit 1
fi
kubectl config use-context $CLUSTER_CONTEXT >/dev/null

echo "=== Installing AMP Extensions ==="

# Pre-update helm dependencies (must run before parallel installs)
echo "   Updating Helm dependencies..."
helm dependency update "${SCRIPT_DIR}/../helm-charts/wso2-amp-thunder-extension"
echo "✅ Helm dependencies updated"
echo ""

install_thunder_extension() {
    echo "📦 Installing/Upgrading WSO2 AMP Thunder Extension..."

    # Detect an image mismatch and do a clean uninstall+install so the
    # pre-install setup job re-runs and re-bootstraps the database.
    local target_image="ghcr.io/thunder-id/thunderid:1.0.2"
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
    # matches the authorize request's `resource` value exactly.
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

install_thunder_extension || exit 1
echo ""

wait_for_namespace_ready amp-thunder 'Thunder Extension'

# Reconcile the gateway-operator here so an env.sh pin bump lands on
# long-lived clusters too (the AMP gateway charts need its newer CRDs).
"${SCRIPT_DIR}/ensure-gateway-operator.sh"

echo "✅ AMP extensions ready"
