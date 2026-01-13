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
echo "📦 Installing OpenChoreo core components..."
echo "   Reference: https://openchoreo.dev/docs/getting-started/try-it-out/on-self-hosted-kubernetes/"
echo "   This may take several minutes..."
echo ""

# ============================================================================
# CORE COMPONENTS (Required)
# ============================================================================

# Step 1: Install OpenChoreo Control Plane
echo "2️⃣  Installing OpenChoreo Control Plane..."
if helm status openchoreo-control-plane -n openchoreo-control-plane &>/dev/null; then
    echo "⏭️  Control Plane already installed, skipping..."
else
    echo "   This may take up to 10 minutes..."
    helm upgrade --install openchoreo-control-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
    --version 0.9.0 \
    --namespace openchoreo-control-plane \
    --create-namespace \
    --values "${SCRIPT_DIR}/../values/control-plane-values.yaml"
fi

echo "⏳ Waiting for Control Plane pods to be ready (timeout: 5 minutes)..."
kubectl wait -n openchoreo-control-plane --for=condition=available --timeout=300s deployment --all
kubectl wait -n openchoreo-control-plane --for=condition=complete  job --all
echo "✅ OpenChoreo Control Plane ready"
echo ""

# ============================================================================
# Step 2: Install OpenChoreo Data Plane
echo "3️⃣  Installing OpenChoreo Data Plane..."
if helm status openchoreo-data-plane -n openchoreo-data-plane &>/dev/null; then
    echo "⏭️  Data Plane already installed, skipping..."
else
    echo "   This may take up to 10 minutes..."
    helm upgrade --install openchoreo-data-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
    --version 0.9.0 \
    --namespace openchoreo-data-plane \
    --create-namespace \
    --values "${SCRIPT_DIR}/../values/data-plane-values.yaml"
fi

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
    - "*.openchoreoapis.localhost"
EOF
echo "✅ Gateway TLS Certificate created"
echo ""

# Registering the Data Plane with the control plane
echo "4️⃣  Registering Data Plane..."
CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-data-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)
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
    publicVirtualHost: "openchoreoapis.localhost"
  secretStoreRef:
    name: default
EOF
echo "✅ Data Plane registered successfully"
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
echo "4️⃣  Installing OpenChoreo Build Plane..."
if helm status openchoreo-build-plane -n openchoreo-build-plane &>/dev/null; then
    echo "⏭️  Build Plane already installed, skipping..."
else
    helm upgrade --install openchoreo-build-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
    --version 0.9.0 \
    --namespace openchoreo-build-plane \
    --create-namespace \
    --values "${SCRIPT_DIR}/../values/build-plane-values.yaml"
fi

# Registering the Build Plane with the control plane
echo "5️⃣  Registering Build Plane..."
BP_CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-build-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)
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
echo "5️⃣ Installing Custom Build CI Workflows..."
if helm status amp-custom-build-ci-workflows -n openchoreo-build-plane &>/dev/null; then
    echo "⏭️  Custom Build CI Workflows already installed, skipping..."
else
    helm install amp-custom-build-ci-workflows "${SCRIPT_DIR}/../helm-charts/wso2-amp-build-extension" --namespace openchoreo-build-plane --set global.agentManagerService.url="http://host.docker.internal:9000"
    echo "✅ Custom Build CI Workflows installed successfully"
fi
echo ""

# Install Default Platform Resources
echo "6️⃣ Installing Default Platform Resources..."
if helm status amp-default-platform-resources &>/dev/null; then
    echo "⏭️  Platform Resources already installed, skipping..."
else
    echo "   Creating default Organization, Project, Environment, and DeploymentPipeline..."
    helm install amp-default-platform-resources "${SCRIPT_DIR}/../helm-charts/wso2-amp-platform-resources-extension" --namespace default
    echo "✅ Default Platform Resources installed successfully"
fi
echo ""

# ============================================================================
# Step 4: Install OpenChoreo  Observability Plane
echo "7️⃣  Installing OpenChoreo Observability Plane..."
if helm status openchoreo-observability-plane -n openchoreo-observability-plane &>/dev/null; then
    echo "⏭️  Observability Plane already installed, skipping..."
else
    helm upgrade --install openchoreo-observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
    --version 0.9.0 \
    --namespace openchoreo-observability-plane \
    --create-namespace \
    --values "${SCRIPT_DIR}/../values/observability-plane-values.yaml" \
    --timeout 10m
fi

# Registering the Observability Plane with the control plane
echo "5️⃣  Registering Observability Plane..."
OP_CA_CERT=$(kubectl get secret cluster-agent-tls -n openchoreo-observability-plane -o jsonpath='{.data.ca\.crt}' | base64 -d)
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
