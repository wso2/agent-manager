#!/bin/bash
# Tears down the AMP layer so `make setup-amp` can rebuild it in minutes. The
# base (colima, k3d, and the prerequisites: cert-manager, kgateway, the AMP
# Gateway, the gateway-operator) is never touched — use
# `make teardown` for a full wipe.
#
# Every step is presence-guarded and best-effort: this must work from broken
# or partially-installed states. Failures are collected and reported at the end.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/env.sh"

WARNINGS=()
warn() {
    echo "⚠️  $1"
    WARNINGS+=("$1")
}

echo "=== Tearing Down AMP Layer (cluster base preserved) ==="

CLUSTER_UP=true
if ! kubectl cluster-info --context "$CLUSTER_CONTEXT" &>/dev/null; then
    CLUSTER_UP=false
    warn "Cluster '$CLUSTER_CONTEXT' unreachable — skipping all in-cluster teardown steps"
else
    kubectl config use-context "$CLUSTER_CONTEXT" >/dev/null
fi

# ============================================================================
# Step 1: Stop port-forwards and the console
# ============================================================================
echo ""
echo "1️⃣  Stop port-forwards and the console"
"$SCRIPT_DIR/stop-port-forward.sh" 2>/dev/null || true
"$SCRIPT_DIR/stop-console.sh" 2>/dev/null || true
echo "✅ Port-forwards and console stopped"

# ============================================================================
# Step 2: Stop Docker Compose services (wipes DB volume)
# ============================================================================
echo ""
echo "2️⃣  Stop Docker Compose services"
if [ -f "$SCRIPT_DIR/../docker-compose.yml" ] && docker info &>/dev/null; then
    docker compose -f "$SCRIPT_DIR/../docker-compose.yml" down -v || warn "docker compose down failed"
    echo "✅ Platform services stopped, volumes removed"
else
    echo "⏭️  docker-compose.yml or docker unavailable, skipping"
fi

# ============================================================================
# Step 3: Uninstall AMP helm releases
# ============================================================================
# Enumerated by name pattern, not a hardcoded list, so per-org gateways
# (api-platform-<org>-<env> and the operator-spawned *-gateway runtimes) and the
# optional AI gateway are all covered. No base release matches these prefixes.
# The trace store (opensearch, opentelemetry-collector) goes with its namespace
# in the namespace sweep below.
echo ""
echo "3️⃣  Uninstall AMP helm releases"
if $CLUSTER_UP; then
    releases="$(helm list -A --no-headers 2>/dev/null | awk '{print $1 "\t" $2}' \
        | grep -E '^(amp-|api-platform-|wso2-amp-)' || true)"
    if [ -z "$releases" ]; then
        echo "⏭️  No AMP releases found"
    else
        # Gateways first: the operator is still running and GCs the APIGateway CR's
        # children; extensions afterwards.
        for pass in '^api-platform-' '.'; do
            while IFS=$'\t' read -r name ns; do
                [ -n "$name" ] || continue
                echo "$name" | grep -qE "$pass" || continue
                echo "🗑️  helm uninstall $name (namespace: $ns)"
                # Tolerate a concurrent GC by the gateway-operator: only a release
                # that still exists after a failed uninstall is a real problem.
                if ! helm uninstall "$name" -n "$ns" --wait --timeout=3m; then
                    helm status "$name" -n "$ns" &>/dev/null \
                        && warn "helm uninstall $name failed"
                fi
                releases="$(printf '%s\n' "$releases" | grep -v "^${name}	" || true)"
            done <<< "$releases"
        done
        echo "✅ AMP releases uninstalled"
    fi
fi

# ============================================================================
# Step 4: Delete leftover AMP namespaces
# ============================================================================
echo ""
echo "4️⃣  Delete AMP namespaces"
if $CLUSTER_UP; then
    amp_namespaces="$(kubectl get ns -o name 2>/dev/null | sed 's|^namespace/||' \
        | grep -E '^(amp-thunder|amp-observability)$' || true)"
    gateway_namespaces="$(kubectl get ns -l 'amp.wso2.com/api-platform-gateway=true' -o name 2>/dev/null \
        | sed 's|^namespace/||' || true)"
    all_ns="$(printf '%s\n%s\n' "$amp_namespaces" "$gateway_namespaces" | grep -v '^$' | sort -u)"
    if [ -z "$all_ns" ]; then
        echo "⏭️  No AMP namespaces found"
    else
        for ns in $all_ns; do
            echo "🗑️  Deleting namespace $ns..."
            kubectl delete ns "$ns" --wait=true --timeout=180s \
                || warn "namespace $ns deletion timed out"
        done
        echo "✅ AMP namespaces deleted"
    fi
fi

# ============================================================================
# Step 5: Verification summary
# ============================================================================
echo ""
echo "5️⃣  Verification"
if $CLUSTER_UP; then
    leftovers="$(helm list -A --no-headers 2>/dev/null | awk '{print $1}' \
        | grep -E '^(amp-|api-platform-|wso2-amp-)' || true)"
    if [ -n "$leftovers" ]; then
        warn "AMP helm releases still present: $(echo "$leftovers" | tr '\n' ' ')"
    fi
    echo ""
    echo "📊 Remaining helm releases (should all be base-layer):"
    helm list -A 2>/dev/null || true
fi

echo ""
if [ "${#WARNINGS[@]}" -gt 0 ]; then
    echo "⚠️  AMP teardown finished with ${#WARNINGS[@]} warning(s):"
    printf '   - %s\n' "${WARNINGS[@]}"
    echo "   Review before running 'make setup-amp' (it is idempotent, leftovers are usually absorbed)."
    exit 1
fi
echo "✅ AMP teardown complete! Rebuild with: make setup-amp"
