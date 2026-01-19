#!/bin/bash
set -e

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"

PROJECT_ROOT="$1"
CLUSTER_NAME="openchoreo-local-v0.9"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"

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
echo "1️⃣  Installing/Upgrading WSO2 AMP Thunder Extension..."
helm upgrade --install amp-thunder-extension "${SCRIPT_DIR}/../helm-charts/wso2-amp-thunder-extension" --namespace amp-thunder --create-namespace
echo "✅ AMP Thunder Extension installed/upgraded successfully"

echo "⏳ Waiting for AMP Thunder Extension pods to be ready (timeout: 5 minutes)..."
kubectl wait -n amp-thunder --for=condition=available --timeout=300s deployment --all
# Wait for jobs only if any exist
if kubectl get jobs -n amp-thunder --no-headers 2>/dev/null | grep -q .; then
    kubectl wait -n amp-thunder --for=condition=complete --timeout=300s job --all
fi
echo "✅ AMP Thunder Extension ready"
echo ""

echo ""
echo "📦 Installing OpenChoreo core components..."
echo "   Reference: https://openchoreo.dev/docs/getting-started/try-it-out/on-self-hosted-kubernetes/"
echo "   This may take several minutes..."
echo ""

# ============================================================================
# CORE COMPONENTS (Required)
# ============================================================================

# Step 1: Install OpenChoreo Control Plane
echo "2️⃣  Installing/Upgrading OpenChoreo Control Plane..."
echo "   This may take up to 10 minutes..."
helm upgrade --install openchoreo-control-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
--version 0.9.0 \
--namespace openchoreo-control-plane \
--create-namespace \
--values "${SCRIPT_DIR}/../values/control-plane-values.yaml"

echo "⏳ Waiting for Control Plane pods to be ready (timeout: 5 minutes)..."
kubectl wait -n openchoreo-control-plane --for=condition=available --timeout=300s deployment --all
# Wait for jobs only if any exist
if kubectl get jobs -n openchoreo-control-plane --no-headers 2>/dev/null | grep -q .; then
    kubectl wait -n openchoreo-control-plane --for=condition=complete --timeout=300s job --all
fi
echo "✅ OpenChoreo Control Plane ready"
echo ""

# ============================================================================
# Step 2: Install OpenChoreo Data Plane
echo "3️⃣  Installing/Upgrading OpenChoreo Data Plane..."
echo "   This may take up to 10 minutes..."
helm upgrade --install openchoreo-data-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
--version 0.9.0 \
--namespace openchoreo-data-plane \
--create-namespace \
--values "${SCRIPT_DIR}/../values/data-plane-values.yaml"

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
echo "4️⃣  Registering Data Plane..."
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


echo "Applying HTTPRoute CRD..."
HTTP_ROUTE_CRD="https://raw.githubusercontent.com/kubernetes-sigs/gateway-api/refs/tags/v1.4.1/config/crd/experimental/gateway.networking.k8s.io_httproutes.yaml"
if kubectl apply --server-side --force-conflicts -f "${HTTP_ROUTE_CRD}" &>/dev/null; then
    echo "✅ HTTPRoute CRD applied successfully"
else
    echo "❌ Failed to apply HTTPRoute CRD"
fi

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
echo "4️⃣  Installing/Upgrading OpenChoreo Build Plane..."
helm upgrade --install openchoreo-build-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
--version 0.9.0 \
--namespace openchoreo-build-plane \
--create-namespace \
--values "${SCRIPT_DIR}/../values/build-plane-values.yaml"

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
echo "5️⃣ Installing/Upgrading Custom Build CI Workflows..."
helm upgrade --install amp-custom-build-ci-workflows "${SCRIPT_DIR}/../helm-charts/wso2-amp-build-extension" --namespace openchoreo-build-plane
echo "✅ Custom Build CI Workflows installed/upgraded successfully"
echo ""

# Install Default Platform Resources
echo "6️⃣ Installing/Upgrading Default Platform Resources..."
echo "   Creating default Organization, Project, Environment, and DeploymentPipeline..."
helm upgrade --install amp-default-platform-resources "${SCRIPT_DIR}/../helm-charts/wso2-amp-platform-resources-extension" --namespace default
echo "✅ Default Platform Resources installed/upgraded successfully"
echo ""

# ============================================================================
# Step 4: Install OpenChoreo  Observability Plane
echo "7️⃣  Installing OpenChoreo Observability Plane..."
if helm status openchoreo-observability-plane -n openchoreo-observability-plane &>/dev/null; then
    echo "⏭️  Observability Plane already installed, skipping..."
else
    echo "   This may take up to 15 minutes..."
    kubectl create namespace openchoreo-observability-plane --dry-run=client -o yaml | kubectl apply -f -

    kubectl apply -f $1/deployments/values/oc-collector-configmap.yaml -n openchoreo-observability-plane

    helm install openchoreo-observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
        --version 0.9.0 \
        --namespace openchoreo-observability-plane \
        --create-namespace \
    --values "${SCRIPT_DIR}/../values/observability-plane-values.yaml" \
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
