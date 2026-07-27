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

# Function to install Control Plane
install_control_plane() {
    echo "📦 Installing/Upgrading OpenChoreo Control Plane..."
    echo "   This may take up to 10 minutes..."

    # Delete the configmap before Helm runs so it is always recreated from chart defaults
    # with a clean field manager state. Prevents Apply/Update operation splits that
    # cause 'conflict with helm' errors on repeated setup runs.
    kubectl delete configmap openchoreo-api-config -n openchoreo-control-plane &>/dev/null || true

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

    # ThunderID v0.45.0 uses 'client_id' (not 'sub') for client_credentials tokens.
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
    kubectl logs -n openchoreo-data-plane -l app=cluster-agent --tail=10
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
    --create-namespace

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
    kubectl logs -n openchoreo-workflow-plane -l app=cluster-agent --tail=10
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
    echo "🔧 Patching observer-auth-config: service_account entitlement claim → client_id..."
    if kubectl get configmap observer-auth-config -n openchoreo-observability-plane &>/dev/null; then
        patched_obs_yaml=$(kubectl get configmap observer-auth-config -n openchoreo-observability-plane -o yaml \
            | sed -E "s/claim:[[:space:]]*['\"]?sub['\"]?/claim: client_id/g")
        if ! echo "$patched_obs_yaml" | grep -q "claim: client_id"; then
            echo "❌ Failed to patch observer-auth-config entitlement claim to client_id"
            return 1
        fi
        echo "$patched_obs_yaml" | kubectl apply --server-side --field-manager=helm --force-conflicts -f -
        kubectl rollout restart deployment/observer -n openchoreo-observability-plane
        kubectl rollout status deployment/observer -n openchoreo-observability-plane --timeout=120s
        echo "✅ observer-auth-config patched (client_id claim)"
    else
        echo "⚠️  observer-auth-config not found — skipping observer claim patch"
    fi

    # Registering the Observability Plane with the control plane
    echo "🔗 Registering Observability Plane..."
    OP_CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-observability-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)
    register_observability_plane "$OP_CA_CERT" "default" "http://observer.openchoreo.localhost:11080"

    # Verify ObservabilityPlane
    echo ""
    echo "🔍 Verifying ObservabilityPlane ..."
    kubectl get observabilityplane -n default
    kubectl logs -n openchoreo-observability-plane -l app=cluster-agent --tail=10
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
# Step 5: Install AMP Extensions IN PARALLEL
# ============================================================================
# Pre-update helm dependencies (must run before parallel installs)
echo ""
echo "5️⃣  AMP Extensions (parallel)"
echo "   Updating Helm dependencies..."
helm dependency update "${SCRIPT_DIR}/../helm-charts/wso2-amp-thunder-extension"

echo "✅ Helm dependencies updated"
echo ""

# Define installation functions for parallel execution
install_thunder_extension() {
    echo "📦 Installing/Upgrading WSO2 AMP Thunder Extension..."

    # Detect an image mismatch and do a clean uninstall+install so the
    # pre-install setup job re-runs and re-bootstraps the database.
    local target_image="ghcr.io/thunder-id/thunderid:0.45.0"
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

    helm upgrade --install amp-thunder-extension "${SCRIPT_DIR}/../helm-charts/wso2-amp-thunder-extension" \
        --namespace amp-thunder --create-namespace
    echo "✅ AMP Thunder Extension installed/upgraded successfully"
}

install_evaluation_workflows() {
    echo "📦 Installing/Upgrading Evaluation Workflows Extension..."
    helm upgrade --install amp-evaluation-workflows-extension "${SCRIPT_DIR}/../helm-charts/wso2-amp-evaluation-extension" \
        --namespace openchoreo-workflow-plane \
        --set ampEvaluation.image.repository="amp-evaluation-monitor" \
        --set ampEvaluation.publisher.endpoint="http://agent-manager-service:8080" \
        --set ampEvaluation.publisher.idpTokenUrl="http://amp-thunder-extension-service.amp-thunder.svc.cluster.local:8090/oauth2/token" \
        --set ampEvaluation.publisher.clientId="amp-publisher-client"
    echo "✅ Evaluation Workflows Extension installed/upgraded successfully"
}

install_platform_resources() {
    echo "📦 Installing/Upgrading Default Platform Resources..."
    echo "   Creating default Organization, Project, Environment, and DeploymentPipeline..."
    helm upgrade --install amp-default-platform-resources "${SCRIPT_DIR}/../helm-charts/wso2-amp-platform-resources-extension" \
        --namespace default
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

# Default-env Thunder provisioning moved to the Makefile's setup-default-env-thunder
# target: it now needs AMS up (store_via_ams), which this script runs before.

# ============================================================================
# Step 6: Install Observability Extension (Agent Manager Observer)
# ============================================================================
echo "6️⃣  Observability Extension (Agent Manager Observer)"
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

# ============================================================================
# Step 7: Install Gateway Operator
# ============================================================================
echo "7️⃣  Gateway Operator"
if helm status gateway-operator -n openchoreo-data-plane &>/dev/null; then
    echo "⏭️  Gateway Operator already installed, skipping..."
else
    helm install gateway-operator oci://ghcr.io/wso2/api-platform/helm-charts/gateway-operator \
        --version "${GATEWAY_OPERATOR_VERSION}" \
        --namespace openchoreo-data-plane \
        --create-namespace \
        --set logging.level=debug \
        --set gatewayApi.installStandardCRDs=false \
        --set "gateway.helm.chartVersion=${GATEWAY_CHART_VERSION}" \
        --set "gateway.values.gateway.controller.image.tag=${GATEWAY_IMAGE_VERSION}" \
        --set gateway.values.gateway.controller.image.repository=ghcr.io/wso2/api-platform/gateway-controller \
        --set "gateway.values.gateway.gatewayRuntime.image.tag=${GATEWAY_IMAGE_VERSION}" \
        --set gateway.values.gateway.gatewayRuntime.image.repository=ghcr.io/wso2/api-platform/gateway-runtime \
        --set gateway.values.gateway.controller.encryptionKeys.enabled=true \
        --set "gateway.values.gateway.controller.encryptionKeys.secretName=${GATEWAY_ENCRYPTION_SECRET_NAME}" \
        --set gateway.values.gateway.controller.deployment.livenessProbe.httpGet.path=/api/admin/v1/health \
        --set gateway.values.gateway.controller.deployment.livenessProbe.httpGet.port=admin \
        --set gateway.values.gateway.controller.deployment.readinessProbe.httpGet.path=/api/admin/v1/health \
        --set gateway.values.gateway.controller.deployment.readinessProbe.httpGet.port=admin
    echo "✅ Gateway Operator installed successfully"
fi
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
echo "🔍 Final Verification - Waiting for remaining components..."
echo ""

wait_for_namespace_ready amp-thunder 'Thunder Extension'

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
