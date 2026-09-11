#!/bin/bash
set -euo pipefail

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"
source "$SCRIPT_DIR/env.sh"
source "$SCRIPT_DIR/utils.sh"
PROJECT_ROOT="$1"

echo "=== Installing OpenChoreo on k3d ==="
# Check prerequisites
if ! kubectl cluster-info --context $CLUSTER_CONTEXT &> /dev/null; then
    echo "❌ K3d cluster '$CLUSTER_CONTEXT' is not running."
    echo "   Run: ./setup-k3d.sh && ./setup-pre-requisites.sh"
    exit 1
fi

echo "🔧 Setting kubectl context to $CLUSTER_CONTEXT..."
kubectl config use-context $CLUSTER_CONTEXT

echo ""
echo "📦 Installing OpenChoreo core components..."
echo "   This may take several minutes..."
echo ""

# ============================================================================
# CORE COMPONENTS (Required)
# ============================================================================

# Helm does NOT update a chart's crds/ directory on `helm upgrade` — only on the
# first install.
sync_chart_crds() {
    local chart="$1"
    local tmp rc=0
    if ! tmp="$(mktemp -d)"; then
        echo "❌ Failed to create temp dir for ${chart} CRD synchronization" >&2
        return 1
    fi
    _sync_chart_crds_body "$chart" "$tmp" || rc=$?
    rm -rf "$tmp"
    return "$rc"
}

_sync_chart_crds_body() {
    local chart="$1"
    local tmp="$2"
    if ! helm pull "oci://ghcr.io/openchoreo/helm-charts/${chart}" \
        --version "${OPENCHOREO_VERSION}" --untar --untardir "$tmp" >/dev/null 2>&1; then
        echo "❌ Failed to download ${chart} chart for CRD synchronization" >&2
        return 1
    fi
    if compgen -G "$tmp/${chart}/crds/*.yaml" >/dev/null; then
        echo "   Syncing ${chart} CRDs to ${OPENCHOREO_VERSION} (helm upgrade does not update crds/)..."
        if ! kubectl apply --server-side --force-conflicts -f "$tmp/${chart}/crds/" >/dev/null; then
            echo "❌ Failed to apply ${chart} CRDs" >&2
            return 1
        fi
    fi
    return 0
}

# verify_agent_logs prints a plane's cluster-agent logs as a post-setup sanity
# check.
verify_agent_logs() {
    local ns="$1"
    local out
    if out=$(kubectl logs -n "$ns" -l app=cluster-agent --tail=10 2>&1); then
        echo "$out"
        return 0
    fi
    if printf '%s' "$out" | grep -qiE "ContainerCreating|waiting to start|PodInitializing"; then
        echo "⚠️  cluster-agent logs not available yet in ${ns} (pod still starting):"
        printf '   %s\n' "$out"
        return 0
    fi
    echo "❌ Failed to read cluster-agent logs in ${ns}:" >&2
    printf '%s\n' "$out" >&2
    return 1
}

# Function to install Control Plane
install_control_plane() {
    echo "📦 Installing/Upgrading OpenChoreo Control Plane..."
    echo "   This may take up to 10 minutes..."

    # Delete the configmap before Helm runs so it is always recreated from chart defaults
    # with a clean field manager state. Prevents Apply/Update operation splits that
    # cause 'conflict with helm' errors on repeated setup runs.
    kubectl delete configmap openchoreo-api-config -n openchoreo-control-plane &>/dev/null || true

    # Apply CRDs before the upgrade so new/changed kinds are registered before the
    # controllers start (helm upgrade skips crds/). No-op on a clean first install.
    sync_chart_crds openchoreo-control-plane

    local install_output
    if ! install_output=$(helm upgrade --install openchoreo-control-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
        --version "${OPENCHOREO_VERSION}" \
        --namespace openchoreo-control-plane \
        --create-namespace \
        --values "${SCRIPT_DIR}/../single-cluster/values-cp.yaml" 2>&1); then

        echo "$install_output"

        if echo "$install_output" | grep -q "no endpoints available for service \"controller-manager-webhook-service\""; then
            echo "⚠️  Control Plane webhook was not ready. Waiting for deployments and retrying once..."
            kubectl wait -n openchoreo-control-plane --for=condition=available --timeout=300s deployment --all || true

            helm upgrade --install openchoreo-control-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
                --version "${OPENCHOREO_VERSION}" \
                --namespace openchoreo-control-plane \
                --create-namespace \
                --values "${SCRIPT_DIR}/../single-cluster/values-cp.yaml"
        else
            echo "❌ OpenChoreo Control Plane install failed"
            return 1
        fi
    fi

    echo "⏳ Waiting for Control Plane deployments to be ready (timeout: 5 minutes)..."
    kubectl wait -n openchoreo-control-plane --for=condition=available --timeout=300s deployment --all

    # Thunder's client_credentials tokens carry the subject as 'client_id', not the
    # 'sub' OpenChoreo's own service-account entitlement claim mapping defaults to.
    # The Helm chart schema doesn't expose this setting, so patch the configmap using
    # server-side apply (Apply operation) to stay compatible with Helm's field manager.
    echo "🔧 Patching openchoreo-api-config: service_account entitlement claim → client_id..."
    if kubectl get configmap openchoreo-api-config -n openchoreo-control-plane &>/dev/null; then
        patched_yaml=$(kubectl get configmap openchoreo-api-config -n openchoreo-control-plane -o yaml \
            | sed -E "s/claim:[[:space:]]*['\"]?sub['\"]?/claim: client_id/g")
        if ! echo "$patched_yaml" | grep -q "claim: client_id"; then
            echo "❌ Failed to patch openchoreo-api-config entitlement claim to client_id"
            return 1
        fi
        echo "$patched_yaml" | kubectl apply --server-side --field-manager=helm --force-conflicts -f -
        kubectl rollout restart deployment/openchoreo-api -n openchoreo-control-plane
        kubectl rollout status deployment/openchoreo-api -n openchoreo-control-plane --timeout=120s
        echo "✅ openchoreo-api-config patched (client_id claim)"
    else
        echo "⚠️  openchoreo-api-config not found — skipping claim patch"
    fi

    # v0.45 puts the client name in 'client_id' (was 'sub'), so the built-in service-account
    # bindings keyed on 'sub' stop matching and return 403. Switch them to 'client_id'.
    # Use server-side apply with field-manager=helm (like the configmap patch above) instead of
    # `kubectl patch`, which claims ownership under a `kubectl-patch` field manager and causes
    # 'conflict with "kubectl-patch"' errors on the next `helm upgrade --install`.
    echo "🔧 Migrating service-account ClusterAuthzRoleBindings: claim sub → client_id..."
    for binding in $(kubectl get clusterauthzrolebindings.openchoreo.dev \
        -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
        claim=$(kubectl get clusterauthzrolebinding.openchoreo.dev "$binding" \
            -o jsonpath='{.spec.entitlement.claim}' 2>/dev/null)
        if [ "$claim" = "sub" ]; then
            patched_binding_yaml=$(kubectl get clusterauthzrolebinding.openchoreo.dev "$binding" -o yaml \
                | sed -E "s/claim:[[:space:]]*['\"]?sub['\"]?/claim: client_id/g")
            if ! echo "$patched_binding_yaml" | grep -q "claim: client_id"; then
                echo "❌ Failed to patch ClusterAuthzRoleBinding '${binding}' entitlement claim to client_id"
                return 1
            fi
            if echo "$patched_binding_yaml" | kubectl apply --server-side --field-manager=helm --force-conflicts -f - >/dev/null 2>&1; then
                echo "   ✓ migrated ${binding}"
            else
                echo "❌ Failed to migrate ClusterAuthzRoleBinding '${binding}' to client_id"
                return 1
            fi
        fi
    done
    echo "✅ Service-account bindings migrated to client_id"

    echo "✅ OpenChoreo Control Plane ready"
}

# Function to install Data Plane
install_data_plane() {
    echo "📦 Installing/Upgrading OpenChoreo Data Plane..."
    echo "Setting up OC Data plane namespace and certificates..."
    create_plane_cert_resources openchoreo-data-plane

    helm upgrade --install openchoreo-data-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
        --version ${OPENCHOREO_VERSION} \
        --namespace openchoreo-data-plane \
        --create-namespace \
        --values "${SCRIPT_DIR}/../single-cluster/values-dp.yaml"

    echo "⏳ Waiting for Data Plane pods to be ready..."
    kubectl wait -n openchoreo-data-plane --for=condition=available --timeout=300s deployment --all
    echo "✅ OpenChoreo Data Plane ready"

    # Register the Data Plane with the control plane
    # Wait for the cert-manager Certificate to be Ready (not just the Secret) to avoid a race
    # where cert-manager re-issues the cert after we read it but before the agent connects.
    echo "⏳ Waiting for data plane agent certificate to be ready..."
    if ! kubectl wait -n openchoreo-data-plane \
        --for=condition=Ready certificate/cluster-agent-dataplane-tls --timeout=180s; then
        echo "❌ Data plane agent certificate not ready. Cannot register data plane."
        return 1
    fi
    echo "🔗 Registering Data Plane..."
    local ca_cert
    ca_cert=$(kubectl get secret cluster-agent-tls -n openchoreo-data-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)
    register_data_plane "$ca_cert" "default" "default"

    # Verify DataPlane
    echo ""
    echo "🔍 Verifying DataPlane..."
    kubectl get clusterdataplane -n default
    # Non-fatal: a freshly-rolled agent pod may still be ContainerCreating, which
    # makes `kubectl logs` return BadRequest and abort the script under `set -e`.
    verify_agent_logs openchoreo-data-plane
    echo "✅ OpenChoreo Data Plane registered and verified"
}

# Function to install Workflow Plane
install_workflow_plane() {
    echo "📦 Setting up OpenChoreo Workflow Plane..."
    echo "Setting up OC Workflow plane namespace and certificates..."
    create_plane_cert_resources openchoreo-workflow-plane

    # Install Docker Registry for Workflow Plane
    echo "🔧 Installing Docker Registry for Workflow Plane..."
    helm upgrade --install registry docker-registry \
      --repo https://twuni.github.io/docker-registry.helm \
      --namespace openchoreo-workflow-plane \
      --create-namespace \
      --values https://raw.githubusercontent.com/openchoreo/openchoreo/v${OPENCHOREO_VERSION}/install/k3d/single-cluster/values-registry.yaml
    
    echo "📦 Installing/Upgrading OpenChoreo Workflow Plane..."
    helm upgrade --install openchoreo-workflow-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-workflow-plane \
    --version ${OPENCHOREO_VERSION} \
    --namespace openchoreo-workflow-plane \
    --create-namespace \
    --values "${SCRIPT_DIR}/../single-cluster/values-wp.yaml"

    echo "⏳ Waiting for Workflow Plane pods to be ready..."
    kubectl wait -n openchoreo-workflow-plane --for=condition=available --timeout=300s deployment --all
    echo "✅ OpenChoreo Workflow Plane ready"

    # Registering the Workflow Plane with the control plane
    echo "🔗 Registering Workflow Plane..."
    WP_CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-workflow-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)
    register_workflow_plane "$WP_CA_CERT" "default" "default"

    # Verify WorkflowPlane
    echo ""
    echo "🔍 Verifying WorkflowPlane ..."
    kubectl get clusterworkflowplane -n default
    verify_agent_logs openchoreo-workflow-plane
    echo "✅ OpenChoreo Workflow Plane ready"
}

# Function to install Observability Plane
install_observability_plane() {
    echo "📦 Installing OpenChoreo Observability Plane..."
    echo "Setting up OC Observability plane namespace and certificates..."
    create_plane_cert_resources openchoreo-observability-plane

    echo "Pull Secrets for OpenChoreo Observability Plane..."
    create_external_secrets_obs_plane

    echo "⏳ Waiting for ExternalSecrets to sync..."
    kubectl wait -n openchoreo-observability-plane \
        --for=condition=Ready externalsecret/opensearch-admin-credentials \
        externalsecret/observer-secret --timeout=60s
    echo "✅ ExternalSecrets synced"

    echo "   This may take up to 15 minutes..."
    kubectl apply -f ${PROJECT_ROOT}/deployments/values/oc-collector-configmap.yaml -n openchoreo-observability-plane
    # Apply CRDs before the upgrade (helm upgrade skips crds/). No-op on first install.
    sync_chart_crds openchoreo-observability-plane
    helm upgrade --install openchoreo-observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
    --version ${OPENCHOREO_VERSION} \
    --namespace openchoreo-observability-plane \
    --create-namespace \
    --values "${SCRIPT_DIR}/../single-cluster/values-op.yaml" \
    --timeout 25m
    echo "✅ OpenChoreo Observability Plane installed/upgraded successfully"

    # Install OpenSearch based logs module
    echo "Installing OpenSearch based logs module..."
    helm upgrade --install observability-logs-opensearch \
      oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
      --create-namespace \
      --namespace openchoreo-observability-plane \
      --version "${OBSERVABILITY_LOGS_OPENSEARCH_VERSION}" \
      --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
      --set adapter.openSearchSecretName="opensearch-admin-credentials"
    echo "✅ OpenSearch based logs module installed"

    # Enable logs collection in the configured logs module.
    # fluent-bit.tolerations[operator=Exists] lets the Fluent Bit log collector (a
    # DaemonSet, one pod per node) run on EVERY node, including tainted isolation-tier
    # nodes such as the gVisor node (setup-gvisor-node.sh) or the Kata node
    # (setup-kata-node.sh). Without it, agents on those nodes produce no runtime logs
    # because no collector tails that node's container logs. operator=Exists tolerates
    # any taint, so both the gvisor and kata taints are covered. Harmless for all-runc
    # clusters (the toleration just matches nothing).
    echo "Enabling log collection in Observability Plane..."
    helm upgrade observability-logs-opensearch \
      oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \
      --create-namespace \
      --namespace openchoreo-observability-plane \
      --version "${OBSERVABILITY_LOGS_OPENSEARCH_VERSION}" \
      --reuse-values \
      --set fluent-bit.enabled=true \
      --set "fluent-bit.tolerations[0].operator=Exists"
    echo "✅ OpenSearch Log collection enabled"

    echo "Enabling opensearch based tracing module..."
    helm upgrade --install observability-traces-opensearch \
    oci://ghcr.io/openchoreo/helm-charts/observability-tracing-opensearch \
        --create-namespace \
        --namespace openchoreo-observability-plane \
        --version "${OBSERVABILITY_TRACING_OPENSEARCH_VERSION}" \
        --set openSearch.enabled=false \
        --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials" \
        --set opentelemetry-collector.configMap.existingName="amp-opentelemetry-collector-config"

    # Prometheus based metrics module.
    # The values file relabels sandbox-cgroup cAdvisor series (gVisor/Kata
    # isolation tiers) to container="sandbox" so the metrics adapter's
    # container-scoped usage queries return data for sandboxed agents too —
    # without it, CPU/memory usage charts are empty on gVisor/Kata
    # environments (usage lives in the sandbox cgroup, not per-container
    # cgroups, on those runtimes).
    echo "Installing Prometheus based metrics module..."
    install_metrics_prometheus() {
        helm upgrade --install observability-metrics-prometheus \
          oci://ghcr.io/openchoreo/helm-charts/observability-metrics-prometheus \
          --create-namespace \
          --namespace openchoreo-observability-plane \
          --version "${OBSERVABILITY_METRICS_PROMETHEUS_VERSION}" \
          --values "${PROJECT_ROOT}/deployments/values/observability-metrics-prometheus.yaml" \
          --set adapter.image.tag="" \
          --timeout 10m
    }
    if ! metrics_output=$(install_metrics_prometheus 2>&1); then
        echo "$metrics_output"
        if echo "$metrics_output" | grep -q "ensure CRDs are installed first"; then
            echo "⚠️  Prometheus CRDs not yet established. Waiting for them and retrying once..."
            kubectl wait --for=condition=established --timeout=120s \
                crd/servicemonitors.monitoring.coreos.com \
                crd/podmonitors.monitoring.coreos.com \
                crd/prometheuses.monitoring.coreos.com \
                crd/alertmanagers.monitoring.coreos.com 2>/dev/null || true
            install_metrics_prometheus
        else
            echo "❌ Prometheus based metrics module install failed"
            return 1
        fi
    fi
    echo "✅ Prometheus based metrics module installed"

    echo "⏳ Waiting for Observability Plane pods to be ready..."
    kubectl wait -n openchoreo-observability-plane --for=condition=available --timeout=300s deployment --all
    echo "✅ OpenChoreo Observability Plane deployments ready"

    # The observer resolves service accounts from its own config (observer-auth-config),
    # still keyed on 'claim: sub'. Patch it to 'client_id' (like openchoreo-api-config),
    # else agent build-log queries get 403.
    #
    # observer-auth-config is populated asynchronously: the configmap can exist
    # before its service-account 'claim: sub' is written. Patching too early sees
    # no 'sub' and would fail below; wait for the claim to surface (either 'sub'
    # to patch, or 'client_id' if a prior run already did).
    echo "🔧 Patching observer-auth-config: service_account entitlement claim → client_id..."
    obs_auth_claim=""
    for _ in $(seq 1 30); do
        obs_auth_claim=$(kubectl get configmap observer-auth-config -n openchoreo-observability-plane -o yaml 2>/dev/null \
            | grep -oE "claim:[[:space:]]*['\"]?(sub|client_id)['\"]?" | head -1)
        [ -n "$obs_auth_claim" ] && break
        sleep 4
    done
    if [ -z "$obs_auth_claim" ]; then
        # Distinguish a genuinely-absent configmap (skip) from one that is present
        # but whose service-account claim never surfaced (fail) so the outcome is
        # not misleading.
        if kubectl get configmap observer-auth-config -n openchoreo-observability-plane &>/dev/null; then
            echo "❌ observer-auth-config present but its service-account claim never surfaced after waiting"
            return 1
        fi
        echo "⚠️  observer-auth-config not found — skipping observer claim patch"
    else
        patched_obs_yaml=$(kubectl get configmap observer-auth-config -n openchoreo-observability-plane -o yaml \
            | sed -E "s/claim:[[:space:]]*['\"]?sub['\"]?/claim: client_id/g")
        if ! echo "$patched_obs_yaml" | grep -qE "claim:[[:space:]]*['\"]?client_id['\"]?"; then
            echo "❌ Failed to patch observer-auth-config entitlement claim to client_id"
            return 1
        fi
        echo "$patched_obs_yaml" | kubectl apply --server-side --field-manager=helm --force-conflicts -f -
        kubectl rollout restart deployment/observer -n openchoreo-observability-plane
        kubectl rollout status deployment/observer -n openchoreo-observability-plane --timeout=120s
        echo "✅ observer-auth-config patched (client_id claim)"
    fi

    # Registering the Observability Plane with the control plane
    echo "🔗 Registering Observability Plane..."
    OP_CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-observability-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)
    register_observability_plane "$OP_CA_CERT" "default" "http://observer.openchoreo.localhost:11080"

    # Verify ObservabilityPlane
    echo ""
    echo "🔍 Verifying ObservabilityPlane ..."
    kubectl get observabilityplane -n default
    verify_agent_logs openchoreo-observability-plane
    echo "✅ OpenChoreo Observability Plane ready"
}

# ============================================================================
# Step 1: Install Control Plane (must complete before Data Plane)
# ============================================================================
echo "1️⃣  Control Plane"
install_control_plane
echo ""

# ============================================================================
# Step 2: Install and Register Data Plane
# ============================================================================
echo "2️⃣  Data Plane"
install_data_plane
echo ""


# ============================================================================
# Step 3: Install Workflow Plane and Observability Plane IN PARALLEL
# ============================================================================
echo ""
echo "3️⃣  Workflow Plane + Observability Plane (parallel)"
echo ""

run_parallel_tasks \
    "Workflow Plane:install_workflow_plane" \
    "Observability Plane:install_observability_plane" \
    || exit 1

echo "✅ Both Workflow Plane and Observability Plane installed successfully"
echo ""

# ============================================================================
# Step 4: Configure observability integration (requires both planes to be ready)
# ============================================================================
echo "4️⃣  Configuring observability integration..."
# Configure DataPlane observer
if kubectl get clusterdataplane default -n default &>/dev/null; then
    kubectl patch clusterdataplane default -n default --type merge -p '{"spec":{"observabilityPlaneRef":{"kind":"ClusterObservabilityPlane","name":"default"}}}' \
        && echo "   ✅ DataPlane observer configured" \
        || echo "   ⚠️  DataPlane observer configuration failed (non-fatal)"
else
    echo "   ⚠️  DataPlane resource not found yet "
fi

# Configure WorkflowPlane observer
if kubectl get clusterworkflowplane default -n default &>/dev/null; then
    kubectl patch clusterworkflowplane default -n default --type merge -p '{"spec":{"observabilityPlaneRef":{"kind":"ClusterObservabilityPlane","name":"default"}}}' \
        && echo "   ✅ WorkflowPlane observer configured" \
        || echo "   ⚠️  WorkflowPlane observer configuration failed (non-fatal)"
else
    echo "   ⚠️  WorkflowPlane resource not found yet"
fi
echo ""

echo "All core OpenChoreo planes are installed and registered!"


# ============================================================================
# Step 5+6: Install AMP Extensions (extracted to setup-amp-extensions.sh so
# `make setup-amp` can reinstall them without redoing the OpenChoreo base).
# Default-env Thunder provisioning stays in the Makefile's
# setup-default-env-thunder target: it needs AMS up (store_via_ams).
# ============================================================================
echo ""
echo "5️⃣ +6️⃣  AMP Extensions"
"${SCRIPT_DIR}/setup-amp-extensions.sh" "${PROJECT_ROOT}" || exit 1
echo ""

# ============================================================================
# Step 7: Install Gateway Operator
# ============================================================================
echo "7️⃣  Gateway Operator"
"${SCRIPT_DIR}/ensure-gateway-operator.sh" || exit 1
echo ""

# ============================================================================
# Step 8: Grant RBAC for WSO2 API Platform CRDs
# ============================================================================
echo "8️⃣  RBAC for WSO2 API Platform CRDs"

kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: wso2-api-platform-gateway-module
rules:
  - apiGroups: ["gateway.api-platform.wso2.com"]
    resources: ["restapis", "apigateways"]
    verbs: ["*"]
  - apiGroups: ["gateway.kgateway.dev"]
    resources: ["backends"]
    verbs: ["*"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: wso2-api-platform-gateway-module
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: wso2-api-platform-gateway-module
subjects:
  - kind: ServiceAccount
    name: cluster-agent-dataplane
    namespace: openchoreo-data-plane
EOF
echo "✅ RBAC for WSO2 API Platform CRDs applied"
echo ""

# ============================================================================
# Step 9: API Resources (Gateway is installed later by setup-platform.sh
# after Agent Manager is running on docker-compose)
# ============================================================================
echo "9️⃣  API Resources (Gateway installed after Agent Manager is up)"
echo "   Gateway chart will be installed by setup-platform.sh"
echo ""

# ============================================================================
# VERIFICATION - Wait for remaining components to be ready
# ============================================================================

echo ""
echo "🔍 Final Verification"
echo ""
echo "📊 Final Pod Status:"
echo ""
echo "--- Control Plane ---"
kubectl get pods -n openchoreo-control-plane
echo ""
echo "--- Data Plane ---"
kubectl get pods -n openchoreo-data-plane
echo ""
echo "--- Workflow Plane ---"
kubectl get pods -n openchoreo-workflow-plane
echo ""
echo "--- Observability Plane ---"
kubectl get pods -n openchoreo-observability-plane
echo ""
echo "--- Thunder Extension ---"
kubectl get pods -n amp-thunder
echo ""

echo "✅ OpenChoreo installation complete!"
echo ""

# Print a credential summary — env-Thunder instances (generated passwords).
_active_count=0
while IFS= read -r _ns; do
  [ -n "$_ns" ] || continue
  _secret="${_ns}-admin-credentials"
  kubectl get secret "$_secret" -n "$_ns" &>/dev/null 2>&1 || continue
  _host="$(kubectl get httproute "$_ns" -n openchoreo-control-plane -o jsonpath='{.spec.hostnames[0]}' 2>/dev/null || echo "")"
  if [ -z "$_host" ]; then
    # Skip advertising URL details if the routing HTTPRoute is not yet created.
    # Checked BEFORE counting/printing the header so a skipped instance never
    # inflates _active_count or opens the banner with nothing under it.
    continue
  fi
  _active_count=$((_active_count + 1))
  if [ "$_active_count" -eq 1 ]; then
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  Thunder ID — Provisioned Instances (save these credentials!)"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  fi
  # Guarded: this is the final summary step after a successful install, so a
  # missing/undecodable password key or a transient kubectl blip here must not
  # abort the whole script under set -e.
  _pass="$(kubectl get secret "$_secret" -n "$_ns" -o jsonpath='{.data.password}' 2>/dev/null | base64 -d)" || _pass="<unavailable>"
  echo "  ${_ns#amp-thunder-}:"
  echo "    URL:      http://${_host}:8080/console"
  echo "    Username: admin"
  echo "    Password: ${_pass}"
  echo ""
done < <(kubectl get namespaces -o name 2>/dev/null | sed 's|^namespace/||' | grep '^amp-thunder-')
if [ "$_active_count" -gt 0 ]; then
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo ""
fi
