#!/bin/bash
set -e

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"

source "$SCRIPT_DIR/env.sh"

PROJECT_ROOT="$1"

echo "=== Installing OpenChoreo on k3d ==="

# Check prerequisites
if ! command -v helm &> /dev/null; then
    echo "❌ Helm is not installed. Please install it first:"
    echo "   brew install helm"
    exit 1
fi

if ! kubectl cluster-info --context $CLUSTER_CONTEXT &> /dev/null; then
    echo "❌ K3d cluster '$CLUSTER_CONTEXT' is not running."
    echo "   Run: ./setup-k3d.sh"
    exit 1
fi

echo "🔧 Setting kubectl context to $CLUSTER_CONTEXT..."
kubectl config use-context $CLUSTER_CONTEXT

echo ""
echo "📦 Installing OpenChoreo core components..."
echo "   Reference: https://openchoreo.dev/docs/getting-started/try-it-out/on-self-hosted-kubernetes/"
echo "   This may take several minutes..."
echo ""

# ============================================================================
# CORE COMPONENTS (Required)
# ============================================================================

# Step 1: Install OpenChoreo Control Plane
echo "1️⃣  Installing/Upgrading OpenChoreo Control Plane..."
echo "   This may take up to 10 minutes..."
helm upgrade --install openchoreo-control-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
--version ${OPENCHOREO_PATCH_VERSION} \
--namespace openchoreo-control-plane \
--create-namespace \
--values "${SCRIPT_DIR}/../single-cluster/values-cp.yaml"

echo "⏳ Waiting for Control Plane pods to be ready (timeout: 5 minutes)..."
kubectl wait -n openchoreo-control-plane --for=condition=available --timeout=300s deployment --all
# Wait for jobs only if any exist
if kubectl get jobs -n openchoreo-control-plane --no-headers 2>/dev/null | grep -q .; then
    kubectl wait -n openchoreo-control-plane --for=condition=complete --timeout=300s job --all
fi
echo "✅ OpenChoreo Control Plane ready"
echo ""

# Create Certificate for Control Plane TLS
echo "📜 Creating Certificate for Control Plane TLS..."
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: control-plane-tls
  namespace: openchoreo-control-plane
spec:
  secretName: control-plane-tls
  issuerRef:
    name: openchoreo-selfsigned-issuer
    kind: ClusterIssuer
  dnsNames:
    - "*.openchoreo.localhost"
EOF
echo "✅ Control Plane TLS Certificate created"
echo ""

# ============================================================================
# Step 2: Install OpenChoreo Data Plane
echo "2️⃣  Installing/Upgrading OpenChoreo Data Plane..."
echo "   This may take up to 10 minutes..."
helm upgrade --install openchoreo-data-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
--version ${OPENCHOREO_VERSION} \
--namespace openchoreo-data-plane \
--create-namespace \
--values "${SCRIPT_DIR}/../single-cluster/values-dp.yaml"

# Create Certificate for Gateway TLS
echo "📜 Creating Certificate for Gateway TLS..."
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: openchoreo-gateway-tls
  namespace: openchoreo-data-plane
spec:
  secretName: openchoreo-gateway-tls
  issuerRef:
    name: openchoreo-selfsigned-issuer
    kind: ClusterIssuer
  dnsNames:
    - "localhost"
EOF
echo "✅ Gateway TLS Certificate created"
echo ""

# Registering the Data Plane with the control plane
echo "3️⃣  Registering Data Plane..."
CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-data-plane -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d || echo "")
if [ -n "$CA_CERT" ]; then
    kubectl apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: DataPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "default-dataplane"
  clusterAgent:
    clientCA:
      value: |
$(echo "$CA_CERT" | sed 's/^/        /')
  gateway:
    organizationVirtualHost: "openchoreoapis.internal"
    publicVirtualHost: "localhost"
  secretStoreRef:
    name: default
EOF
    echo "✅ Data Plane registered successfully"
else
    echo "⚠️  CA certificate not found; skipping DataPlane registration"
fi
echo ""

# Verify DataPlane
echo ""
echo "🔍 Verifying DataPlane..."
kubectl get dataplane -n default
kubectl logs -n openchoreo-data-plane -l app=cluster-agent --tail=10
echo "Verify API Platform Gateway pods:"
kubectl get pods -n openchoreo-data-plane --selector="app.kubernetes.io/instance=api-platform-default-gateway"
echo "✅ OpenChoreo Data Plane ready"
echo ""


# ============================================================================
# Step 3: Install OpenChoreo Build Plane

echo "3️⃣  Setting up OpenChoreo Build Plane..."
# Install Docker Registry for Build Plane
echo "🔧 Installing Docker Registry for Build Plane..."
helm upgrade --install registry docker-registry \
  --repo https://twuni.github.io/docker-registry.helm \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --set persistence.enabled=true \
  --set persistence.size=10Gi \
  --set service.type=LoadBalancer

echo "⏳ Waiting for Docker Registry to be ready..."
kubectl wait --for=condition=available deployment/registry-docker-registry -n openchoreo-build-plane --timeout=120s

echo "4️⃣  Installing/Upgrading OpenChoreo Build Plane..."
helm upgrade --install openchoreo-build-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
--version ${OPENCHOREO_VERSION} \
--namespace openchoreo-build-plane \
--create-namespace \
--values "${SCRIPT_DIR}/../single-cluster/values-bp.yaml"

# Registering the Build Plane with the control plane
echo "5️⃣  Registering Build Plane..."
BP_CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-build-plane -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d || echo "")
if [ -n "$BP_CA_CERT" ]; then
    kubectl apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: BuildPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "default-buildplane"
  secretStoreRef:
    name: openbao
  clusterAgent:
    clientCA:
      value: |
$(echo "$BP_CA_CERT" | sed 's/^/        /')
EOF
    echo "✅ Build Plane registered successfully"
else
    echo "⚠️  CA certificate not found; skipping BuildPlane registration"
fi
echo ""

# Verify BuildPlane
echo ""
echo "🔍 Verifying BuildPlane ..."
kubectl get buildplane -n default
kubectl logs -n openchoreo-build-plane -l app=cluster-agent --tail=10
echo "✅ OpenChoreo Build Plane ready"
echo ""

# ============================================================================
# Install Custom Build CI Workflows
echo "6️⃣ Installing/Upgrading Custom Build CI Workflows..."
helm upgrade --install amp-custom-build-ci-workflows "${SCRIPT_DIR}/../helm-charts/wso2-amp-build-extension" --namespace openchoreo-build-plane
echo "✅ Custom Build CI Workflows installed/upgraded successfully"
echo ""

# ============================================================================
# Install Evaluation Workflows Extension
echo "7️⃣ Installing/Upgrading Evaluation Workflows Extension..."
helm upgrade --install amp-evaluation-workflows-extension "${SCRIPT_DIR}/../helm-charts/wso2-amp-evaluation-extension" --namespace openchoreo-build-plane
echo "✅ Evaluation Workflows Extension installed/upgraded successfully"
echo ""

# Install Secrets Extension (OpenBao)
echo "8️⃣ Installing/Upgrading Secrets Extension (OpenBao)..."
echo "   Setting up OpenBao for data plane secret management..."
helm dependency update "${SCRIPT_DIR}/../helm-charts/wso2-amp-secrets-extension"
helm upgrade --install amp-secrets "${SCRIPT_DIR}/../helm-charts/wso2-amp-secrets-extension" --namespace amp-secrets --create-namespace
echo "⏳ Waiting for OpenBao to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=openbao -n amp-secrets --timeout=120s || echo "⚠️  OpenBao pods may still be starting"
echo "✅ Secrets Extension installed/upgraded successfully"
echo ""

# Install Default Platform Resources
echo "9️⃣ Installing/Upgrading Default Platform Resources..."
echo "   Creating default Organization, Project, Environment, and DeploymentPipeline..."
helm upgrade --install amp-default-platform-resources "${SCRIPT_DIR}/../helm-charts/wso2-amp-platform-resources-extension" --namespace default
echo "✅ Default Platform Resources installed/upgraded successfully"
echo ""

# ============================================================================
# Step 4: Install OpenChoreo  Observability Plane
echo "🔟  Installing OpenChoreo Observability Plane..."
if helm status openchoreo-observability-plane -n openchoreo-observability-plane &>/dev/null; then
    echo "⏭️  Observability Plane already installed, skipping..."
else
    echo "   This may take up to 15 minutes..."
    kubectl create namespace openchoreo-observability-plane --dry-run=client -o yaml | kubectl apply -f -

    kubectl apply -f $1/deployments/values/oc-collector-configmap.yaml -n openchoreo-observability-plane

    helm install openchoreo-observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
        --version ${OPENCHOREO_VERSION} \
        --namespace openchoreo-observability-plane \
        --create-namespace \
    --values "${SCRIPT_DIR}/../single-cluster/values-op.yaml" \
    --timeout 15m
fi

echo "✅ OpenSearch ready"

if helm status wso2-amp-observability-extension -n openchoreo-observability-plane &>/dev/null; then
    echo "⏭️  WSO2 AMP Observability Extension already installed, skipping..."
else
    echo "Building and loading Traces Observer Service Docker image into k3d cluster..."
    make -C $1/traces-observer-service docker-load-k3d
    sleep 10        
    echo "   Traces Observer Service to the Observability Plane for tracing ingestion..."
    helm install wso2-amp-observability-extension $1/deployments/helm-charts/wso2-amp-observability-extension \
        --create-namespace \
        --namespace openchoreo-observability-plane \
        --timeout=10m \
        --set tracesObserver.developmentMode=true
fi

# Registering the Observability Plane with the control plane
echo "5️⃣  Registering Observability Plane..."
OP_CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-observability-plane -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d || echo "")
if [ -n "$OP_CA_CERT" ]; then
    kubectl apply -f - <<EOF
apiVersion: openchoreo.dev/v1alpha1
kind: ObservabilityPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "default-observabilityplane"
  clusterAgent:
    clientCA:
      value: |
$(echo "$OP_CA_CERT" | sed 's/^/        /')
  observerURL: http://observer.openchoreo-observability-plane.svc.cluster.local:8080
EOF
    echo "✅ Observability Plane registered successfully"
else
    echo "⚠️  CA certificate not found; skipping ObservabilityPlane registration"
fi

echo "7️⃣  Configuring observability integration..."
 # Configure DataPlane observer
if kubectl get dataplane default -n default &>/dev/null; then
    kubectl patch dataplane default -n default --type merge -p '{"spec":{"observabilityPlaneRef":"default"}}' \
        && echo "   ✅ DataPlane observer configured" \
        || echo "   ⚠️  DataPlane observer configuration failed (non-fatal)"
else
    echo "   ⚠️  DataPlane resource not found yet "
fi

# Configure BuildPlane observer
if kubectl get buildplane default -n default &>/dev/null; then
    kubectl patch buildplane default -n default --type merge -p '{"spec":{"observabilityPlaneRef":"default"}}' \
        && echo "   ✅ BuildPlane observer configured" \
        || echo "   ⚠️  BuildPlane observer configuration failed (non-fatal)"
else
    echo "   ⚠️  BuildPlane resource not found yet"
fi
echo ""

# Verify ObservabilityPlane
echo ""
echo "🔍 Verifying ObservabilityPlane ..."
kubectl get observabilityplane -n default
kubectl logs -n openchoreo-observability-plane -l app=cluster-agent --tail=10
echo "✅ OpenChoreo Observability Plane ready"
echo ""

# Enable Logs Collection
helm upgrade --install openchoreo-observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
  --version  ${OPENCHOREO_VERSION} \
  --namespace openchoreo-observability-plane \
  --reuse-values \
  --set fluent-bit.enabled=true \
  --timeout 10m

echo "✅ Logs collection enabled in Observability Plane"
echo ""

# ============================================================================
# Step 5: Install Gateway Operator
echo "9️⃣  Installing Gateway Operator..."
if helm status gateway-operator -n openchoreo-data-plane &>/dev/null; then
    echo "⏭️  Gateway Operator already installed, skipping..."
else
    helm install gateway-operator oci://ghcr.io/wso2/api-platform/helm-charts/gateway-operator \
        --version 0.2.0 \
        --namespace openchoreo-data-plane \
        --create-namespace \
        --set logging.level=debug \
        --set gateway.helm.chartVersion=0.3.0
    echo "✅ Gateway Operator installed successfully"
fi
echo ""

# Apply Gateway Operator Configuration
echo "🔟 Applying Gateway Operator Configuration..."
# Create local config from template for development
echo "   Creating local development config..."
cp "${SCRIPT_DIR}/../values/api-platform-operator-full-config.yaml" "${SCRIPT_DIR}/../values/api-platform-operator-local-config.yaml"
# Update JWKS URI for local development
sed -i '' 's|http://amp-api.wso2-amp.svc.cluster.local:9000/auth/external/jwks.json|http://host.docker.internal:9000/auth/external/jwks.json|g' "${SCRIPT_DIR}/../values/api-platform-operator-local-config.yaml"
kubectl apply -f "${SCRIPT_DIR}/../values/api-platform-operator-local-config.yaml"
echo "✅ Gateway configuration applied"
echo ""

# Apply Gateway and API Resources
echo "1️⃣1️⃣ Applying Gateway and API Resources..."
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

echo ""
echo "RestApi status:"
kubectl get restapi traces-api-secure -n openchoreo-data-plane -o yaml
echo ""

echo "✅ Gateway and API resources applied"
echo ""

# ============================================================================
# VERIFICATION
# ============================================================================

echo "🔍 Verifying installation..."
echo ""

echo "Verify All Resources:"
kubectl get pods -n openchoreo-control-plane
echo ""

kubectl get pods -n openchoreo-data-plane
echo ""

kubectl get pods -n openchoreo-build-plane
echo ""

kubectl get pods -n openchoreo-observability-plane
echo ""

echo "✅ OpenChoreo installation complete!"
echo ""
