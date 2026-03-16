#!/bin/bash
set -e

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
    helm upgrade --install openchoreo-control-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
        --version ${OPENCHOREO_VERSION} \
        --namespace openchoreo-control-plane \
        --create-namespace \
        --values "${SCRIPT_DIR}/../single-cluster/values-cp.yaml"

    echo "⏳ Waiting for Control Plane pods to be ready (timeout: 5 minutes)..."
    kubectl wait -n openchoreo-control-plane --for=condition=available --timeout=300s deployment --all
    echo "✅ OpenChoreo Control Plane ready"

    echo "⏳ Waiting for CA extractor job to complete..."
    kubectl wait --for=condition=Complete job/cluster-gateway-ca-extractor \
        -n openchoreo-control-plane --timeout=180s
    echo "✅ Cluster Gateway CA certificate ready"
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

    echo "⏳ Waiting for Data Plane pods to be ready (required for registration)..."
    kubectl wait -n openchoreo-data-plane --for=condition=available --timeout=300s deployment --all
    echo "✅ OpenChoreo Data Plane ready"

    # Wait for cert-manager to create the cluster-agent-tls secret
    wait_for_secret "openchoreo-data-plane" "cluster-agent-tls" 120

    # Register the Data Plane with the control plane
    echo "🔗 Registering Data Plane..."
    local ca_cert
    ca_cert=$(kubectl get secret cluster-agent-tls -n openchoreo-data-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)
    register_data_plane "$ca_cert" "default" "amp-openbao-store"

    # Verify DataPlane
    echo ""
    echo "🔍 Verifying DataPlane..."
    kubectl get dataplane -n default
    kubectl logs -n openchoreo-data-plane -l app=cluster-agent --tail=10
    echo "✅ OpenChoreo Data Plane registered and verified"
}

# Function to install Build Plane
install_build_plane() {
    echo "📦 Setting up OpenChoreo Build Plane..."
    echo "Setting up OC Build plane namespace and certificates..."
    create_plane_cert_resources openchoreo-build-plane

    # Install Docker Registry for Build Plane
    echo "🔧 Installing Docker Registry for Build Plane..."
    helm upgrade --install registry docker-registry \
      --repo https://twuni.github.io/docker-registry.helm \
      --namespace openchoreo-build-plane \
      --create-namespace \
      --values https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.16/install/k3d/single-cluster/values-registry.yaml

    echo "📦 Installing/Upgrading OpenChoreo Build Plane..."
    helm upgrade --install openchoreo-build-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
    --version ${OPENCHOREO_VERSION} \
    --namespace openchoreo-build-plane \
    --create-namespace \
    --values "${SCRIPT_DIR}/../single-cluster/values-bp.yaml"

    echo "⏳ Waiting for Build Plane pods to be ready..."
    kubectl wait -n openchoreo-build-plane --for=condition=available --timeout=300s deployment --all
    echo "✅ OpenChoreo Build Plane ready"

    # Wait for cert-manager to create the cluster-agent-tls secret
    wait_for_secret "openchoreo-build-plane" "cluster-agent-tls" 120

    # Registering the Build Plane with the control plane
    echo "🔗 Registering Build Plane..."
    BP_CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-build-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)
    register_build_plane "$BP_CA_CERT" "default" "openbao"

    # Verify BuildPlane
    echo ""
    echo "🔍 Verifying BuildPlane ..."
    kubectl get buildplane -n default
    kubectl logs -n openchoreo-build-plane -l app=cluster-agent --tail=10
    echo "✅ OpenChoreo Build Plane ready"
}

# Function to install Observability Plane
install_observability_plane() {
    echo "📦 Installing OpenChoreo Observability Plane..."
    echo "Setting up OC Observability plane namespace and certificates..."
    create_plane_cert_resources openchoreo-observability-plane

    echo "Pull Secrets for OpenChoreo Observability Plane..."
    create_external_secrets_obs_plane

    echo "⏳ Waiting for ExternalSecrets to sync..."
    kubectl wait --for=condition=Ready externalsecret/observer-opensearch-credentials -n openchoreo-observability-plane --timeout=120s
    echo "✅ ExternalSecrets synced"

    echo "   This may take up to 15 minutes..."
    kubectl apply -f ${PROJECT_ROOT}/deployments/values/oc-collector-configmap.yaml -n openchoreo-observability-plane
    helm upgrade --install openchoreo-observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
    --version ${OPENCHOREO_VERSION} \
    --namespace openchoreo-observability-plane \
    --create-namespace \
    --values "${SCRIPT_DIR}/../single-cluster/values-op.yaml" \
    --set observer.extraEnv.AUTH_SERVER_BASE_URL=http://thunder-service.openchoreo-control-plane.svc.cluster.local:8090 \
    --timeout 15m
    echo "✅ OpenChoreo Observability Plane installed/upgraded successfully"

    # Install OpenSearch based logs module
    echo "Installing OpenSearch based logs module..."
    helm upgrade --install observability-logs-opensearch \
      oci://ghcr.io/openchoreo/charts/observability-logs-opensearch \
      --create-namespace \
      --namespace openchoreo-observability-plane \
      --version 0.3.1 \
      --set openSearchSetup.openSearchSecretName="opensearch-admin-credentials"
    echo "✅ OpenSearch based logs module installed"

    # Enable log collection
    echo "Enabling log collection in Observability Plane..."
    helm upgrade observability-logs-opensearch \
      oci://ghcr.io/openchoreo/charts/observability-logs-opensearch \
      --create-namespace \
      --namespace openchoreo-observability-plane \
      --version 0.3.1 \
      --reuse-values \
      --set fluent-bit.enabled=true
    echo "✅ OpenSearch Log collection enabled"

    # Prometheus based metrics module
    echo "Installing Prometheus based metrics module..."
    helm upgrade --install observability-metrics-prometheus \
      oci://ghcr.io/openchoreo/charts/observability-metrics-prometheus \
      --create-namespace \
      --namespace openchoreo-observability-plane \
      --version 0.2.0
    echo "✅ Prometheus based metrics module installed"

    echo "⏳ Waiting for Observability Plane pods to be ready..."
    kubectl wait -n openchoreo-observability-plane --for=condition=available --timeout=300s deployment --all
    echo "✅ OpenChoreo Observability Plane deployments ready"

    # Wait for cert-manager to create the cluster-agent-tls secret
    wait_for_secret "openchoreo-observability-plane" "cluster-agent-tls" 120

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
# Step 3: Install Build Plane and Observability Plane IN PARALLEL
# ============================================================================
echo ""
echo "3️⃣  Build Plane + Observability Plane (parallel)"
echo ""

run_parallel_tasks \
    "Build Plane:install_build_plane" \
    "Observability Plane:install_observability_plane" \
    || exit 1

echo "✅ Both Build Plane and Observability Plane installed successfully"
echo ""

# ============================================================================
# Step 4: Configure observability integration (requires both planes to be ready)
# ============================================================================
echo "4️⃣  Configuring observability integration..."
# Configure DataPlane observer
if kubectl get dataplane default -n default &>/dev/null; then
    kubectl patch dataplane default -n default --type merge -p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}' \
        && echo "   ✅ DataPlane observer configured" \
        || echo "   ⚠️  DataPlane observer configuration failed (non-fatal)"
else
    echo "   ⚠️  DataPlane resource not found yet "
fi

# Configure BuildPlane observer
if kubectl get buildplane default -n default &>/dev/null; then
    kubectl patch buildplane default -n default --type merge -p '{"spec":{"observabilityPlaneRef":{"kind":"ObservabilityPlane","name":"default"}}}' \
        && echo "   ✅ BuildPlane observer configured" \
        || echo "   ⚠️  BuildPlane observer configuration failed (non-fatal)"
else
    echo "   ⚠️  BuildPlane resource not found yet"
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
helm dependency update "${SCRIPT_DIR}/../helm-charts/wso2-amp-thunder-extension" &
helm dependency update "${SCRIPT_DIR}/../helm-charts/wso2-amp-secrets-extension" &
wait
echo "✅ Helm dependencies updated"
echo ""

# Define installation functions for parallel execution
install_thunder_extension() {
    echo "📦 Installing/Upgrading WSO2 AMP Thunder Extension..."
    helm upgrade --install amp-thunder-extension "${SCRIPT_DIR}/../helm-charts/wso2-amp-thunder-extension" \
        --namespace amp-thunder --create-namespace
    echo "✅ AMP Thunder Extension installed/upgraded successfully"
}

install_build_ci_workflows() {
    echo "📦 Installing/Upgrading Custom Build CI Workflows..."
    helm upgrade --install amp-custom-build-ci-workflows "${SCRIPT_DIR}/../helm-charts/wso2-amp-build-extension" \
        --namespace openchoreo-build-plane
    echo "✅ Custom Build CI Workflows installed/upgraded successfully"
}

install_evaluation_workflows() {
    echo "📦 Installing/Upgrading Evaluation Workflows Extension..."
    helm upgrade --install amp-evaluation-workflows-extension "${SCRIPT_DIR}/../helm-charts/wso2-amp-evaluation-extension" \
        --namespace openchoreo-build-plane \
        --set ampEvaluation.image.repository="amp-evaluation-monitor" \
        --set ampEvaluation.publisher.endpoint="http://agent-manager-service:8080" \
        --set ampEvaluation.publisher.apiKey="dev-publisher-api-key"
    echo "✅ Evaluation Workflows Extension installed/upgraded successfully"
}

install_secrets_extension() {
    echo "📦 Installing/Upgrading Secrets Extension (OpenBao)..."
    echo "   Setting up OpenBao for data plane secret management..."
    helm upgrade --install amp-secrets "${SCRIPT_DIR}/../helm-charts/wso2-amp-secrets-extension" \
        --namespace amp-secrets --create-namespace \
        --set openbao.server.dev.enabled=true
    echo "✅ Secrets Extension installed/upgraded successfully"
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
    "Build CI Workflows:install_build_ci_workflows" \
    "Evaluation Workflows:install_evaluation_workflows" \
    "Secrets Extension:install_secrets_extension" \
    "Platform Resources:install_platform_resources" \
    || exit 1

echo "✅ All AMP extensions installed successfully"
echo ""

# ============================================================================
# Step 6: Install Observability Extension (Traces Observer Service)
# ============================================================================
echo "6️⃣  Observability Extension (Traces Observer Service)"
if helm status wso2-amp-observability-extension -n openchoreo-observability-plane &>/dev/null; then
    echo "⏭️  WSO2 AMP Observability Extension already installed, skipping..."
else
    echo "Building and loading Traces Observer Service Docker image into k3d cluster..."
    make -C ${PROJECT_ROOT}/traces-observer-service docker-load-k3d
    sleep 10
    echo "   Traces Observer Service to the Observability Plane for tracing ingestion..."
    helm install wso2-amp-observability-extension ${PROJECT_ROOT}/deployments/helm-charts/wso2-amp-observability-extension \
        --create-namespace \
        --namespace openchoreo-observability-plane \
        --timeout=10m \
        --set tracesObserver.developmentMode=true
fi
echo ""

# ============================================================================
# Step 7: Install Gateway Operator
# ============================================================================
echo "7️⃣  Gateway Operator"
if helm status gateway-operator -n openchoreo-data-plane &>/dev/null; then
    echo "⏭️  Gateway Operator already installed, skipping..."
else
    helm install gateway-operator oci://ghcr.io/wso2/api-platform/helm-charts/gateway-operator \
        --version 0.4.0 \
        --namespace openchoreo-data-plane \
        --create-namespace \
        --set logging.level=debug \
        --set gateway.helm.chartVersion=0.9.0
    echo "✅ Gateway Operator installed successfully"
fi
echo ""

# ============================================================================
# Step 8: Apply Gateway Operator Configuration
# ============================================================================
echo "8️⃣  Gateway Operator Configuration"
# Create local config from template for development
echo "   Creating local development config..."
cp "${SCRIPT_DIR}/../values/api-platform-operator-full-config.yaml" "${SCRIPT_DIR}/../values/api-platform-operator-local-config.yaml"
# Update JWKS URI for local development
config_file="${SCRIPT_DIR}/../values/api-platform-operator-local-config.yaml"
source_uri='http://amp-api.wso2-amp.svc.cluster.local:9000/auth/external/jwks.json'
target_uri='http://host.k3d.internal:9000/auth/external/jwks.json'

if sed --version >/dev/null 2>&1; then
  sed -i "s|${source_uri}|${target_uri}|g" "$config_file"
else
  sed -i '' "s|${source_uri}|${target_uri}|g" "$config_file"
fi

grep -q "$target_uri" "$config_file" || {
  echo "Failed to rewrite JWKS URI in $config_file"
  exit 1
}
kubectl apply -f "${SCRIPT_DIR}/../values/api-platform-operator-local-config.yaml"
echo "✅ Gateway configuration applied"
echo ""

# ============================================================================
# Step 9: Apply Gateway and API Resources
# ============================================================================
echo "9️⃣  Gateway and API Resources"
kubectl apply -f "${SCRIPT_DIR}/../values/obs-gateway.yaml"

echo "⏳ Waiting for Gateway to be ready..."
if kubectl wait --for=condition=Programmed gateway/obs-gateway -n openchoreo-data-plane --timeout=180s; then
    echo "✅ Gateway is programmed"
else
    echo "⚠️  Gateway did not become ready in time"
fi

echo ""
echo "Gateway status:"
kubectl get gateway obs-gateway -n openchoreo-data-plane -o yaml
echo ""

kubectl apply -f "${SCRIPT_DIR}/../values/otel-collector-rest-api.yaml"

echo "⏳ Waiting for RestApi to be programmed..."
if kubectl wait --for=condition=Programmed restapi/traces-api-secure -n openchoreo-data-plane --timeout=120s; then
    echo "✅ RestApi is programmed"
else
    echo "⚠️  RestApi did not become ready in time"
fi
echo "✅ Gateway and API resources applied"
echo ""

# ============================================================================
# VERIFICATION - Wait for remaining components to be ready
# ============================================================================

echo ""
echo "🔍 Final Verification - Waiting for remaining components..."
echo ""

run_parallel_tasks \
    "Thunder Extension:wait_for_namespace_ready amp-thunder 'Thunder Extension'" \
    "OpenBao:wait_for_pods_ready amp-secrets app.kubernetes.io/name=amp-secrets-openbao OpenBao 120"

echo ""
echo "📊 Final Pod Status:"
echo ""
echo "--- Control Plane ---"
kubectl get pods -n openchoreo-control-plane
echo ""
echo "--- Data Plane ---"
kubectl get pods -n openchoreo-data-plane
echo ""
echo "--- Build Plane ---"
kubectl get pods -n openchoreo-build-plane
echo ""
echo "--- Observability Plane ---"
kubectl get pods -n openchoreo-observability-plane
echo ""
echo "--- Thunder Extension ---"
kubectl get pods -n amp-thunder
echo ""
echo "--- OpenBao ---"
kubectl get pods -n amp-secrets
echo ""

echo "✅ OpenChoreo installation complete!"
echo ""
