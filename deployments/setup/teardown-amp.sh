#!/bin/bash
# Tears down everything ABOVE the OpenChoreo base layer so `make setup-amp`
# can rebuild it in minutes. The base (colima, k3d, prerequisites, OpenChoreo
# planes + observability modules, gateway-operator, agent-sandbox) is never
# touched — use `make teardown` for a full wipe.
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

echo "=== Tearing Down AMP Layer (OpenChoreo base preserved) ==="

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
# Enumerated by name pattern, not a hardcoded list, so env-Thunder instances
# (amp-thunder-<org>-<env>), per-env gateways (api-platform-<org>-<env> and the
# operator-spawned *-gateway runtimes), and the optional AI gateway are all
# covered. No base release matches these prefixes.
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
# Step 4: Delete AMS-created OpenChoreo CRs
# ============================================================================
# Dynamic state created through the API during testing (deployed agents, extra
# environments, ...). Plane registrations (clusterdataplane, observabilityplane,
# etc.), authz bindings, and chart-owned definition kinds (componenttypes,
# traits, workflows, resourcetypes) are deliberately NOT touched — they belong
# to the base. Child kinds first so finalizers unwind cleanly; the control
# plane is alive and GCs the dp-* workload namespaces itself.
echo ""
echo "4️⃣  Delete AMS-created OpenChoreo resources"
if $CLUSTER_UP; then
    STATE_KINDS=(
        workflowruns componentreleases releasebindings renderedreleases
        resourcereleasebindings resourcereleases resources workloads
        components projects secretreferences
        observabilityalertrules observabilityalertsnotificationchannels
        environments deploymentpipelines
    )
    for kind in "${STATE_KINDS[@]}"; do
        found="$(kubectl get "${kind}.openchoreo.dev" -A --no-headers 2>/dev/null | wc -l | tr -d ' ')"
        [ "$found" != "0" ] || continue
        echo "🗑️  Deleting ${found} ${kind}..."
        kubectl delete "${kind}.openchoreo.dev" --all -A --wait=true --timeout=120s \
            || warn "deleting ${kind} timed out (stuck finalizers?)"
    done
    echo "✅ OpenChoreo state resources deleted"
fi

# ============================================================================
# Step 5: Delete leftover AMP namespaces
# ============================================================================
echo ""
echo "5️⃣  Delete AMP namespaces"
if $CLUSTER_UP; then
    amp_namespaces="$(kubectl get ns -o name 2>/dev/null | sed 's|^namespace/||' \
        | grep -E '^(amp-thunder|dp-)' || true)"
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
# Step 6: Purge AMS-written OpenBao secrets
# ============================================================================
# AMS writes dynamic secrets under org subtrees (secret/<org>/...). The flat
# keys at the mount root (secret/amp-system-client-secret, ...) are baseline
# bootstrap values seeded by the OpenBao pod's postStart hook — those stay,
# since they only regenerate on pod restart and the base is not restarted here.
echo ""
echo "6️⃣  Purge AMS-written OpenBao secrets"
if $CLUSTER_UP; then
    BAO_POD="$(kubectl get pods -n openbao -l app.kubernetes.io/name=openbao \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
    if [ -z "$BAO_POD" ]; then
        echo "⏭️  OpenBao pod not found, skipping"
    else
        bao_exec() {
            kubectl exec -n openbao "$BAO_POD" -- \
                env BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN=root bao "$@"
        }
        purge_tree() { # recursively delete every leaf under secret/<subpath>
            local subpath="$1" entry
            for entry in $(bao_exec kv list -format=yaml "secret/${subpath}" 2>/dev/null \
                    | sed -n 's/^- //p'); do
                case "$entry" in
                    */) purge_tree "${subpath}${entry}" ;;
                    *)  bao_exec kv metadata delete "secret/${subpath}${entry}" >/dev/null 2>&1 \
                            || warn "failed to delete OpenBao secret/${subpath}${entry}" ;;
                esac
            done
        }
        deleted_any=false
        for entry in $(bao_exec kv list -format=yaml secret/ 2>/dev/null | sed -n 's/^- //p'); do
            case "$entry" in
                */)
                    echo "🗑️  Purging OpenBao subtree secret/${entry}..."
                    purge_tree "$entry"
                    deleted_any=true
                    ;;
            esac
        done
        $deleted_any && echo "✅ Dynamic OpenBao secrets purged" \
                     || echo "⏭️  No dynamic OpenBao subtrees found (baseline keys preserved)"
    fi
fi

# ============================================================================
# Step 7: Verification summary
# ============================================================================
echo ""
echo "7️⃣  Verification"
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
