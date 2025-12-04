#!/bin/bash
set -e

echo "=== Installing OpenChoreo on Kind Cluster ==="

# Check prerequisites
if ! command -v helm &> /dev/null; then
    echo "❌ Helm is not installed. Please install it first:"
    echo "   brew install helm"
    exit 1
fi

if ! kubectl cluster-info --context kind-openchoreo-local &> /dev/null; then
    echo "❌ Kind cluster 'openchoreo-local' is not running."
    echo "   Run: ./setup-kind.sh"
    exit 1
fi

echo "🔧 Setting kubectl context to openchoreo-local..."
kubectl config use-context kind-openchoreo-local

echo ""
echo "📦 Installing OpenChoreo core components..."
echo "   Reference: https://openchoreo.dev/docs/getting-started/single-cluster/"
echo "   This may take several minutes..."
echo ""

# ============================================================================
# CORE COMPONENTS (Required)
# ============================================================================

# Step 1: Install Cilium CNI
echo "1️⃣  Installing Cilium CNI..."
if helm status cilium -n cilium &>/dev/null; then
    echo "⏭️  Cilium already installed, skipping..."
else
    helm install cilium oci://ghcr.io/openchoreo/helm-charts/cilium \
      --version 0.3.2 \
      --create-namespace \
      --namespace cilium \
      --wait
fi

echo "⏳ Waiting for Cilium pods to be ready (timeout: 5 minutes)..."
kubectl wait --for=condition=Ready pod -l k8s-app=cilium -n cilium --timeout=300s
echo "✅ Cilium CNI ready"
echo ""

# Step 2: Install OpenChoreo Control Plane
echo "2️⃣  Installing OpenChoreo Control Plane..."
if helm status control-plane -n openchoreo-control-plane &>/dev/null; then
    echo "⏭️  Control Plane already installed, skipping..."
else
    echo "   This may take up to 10 minutes..."
    helm install control-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
      --version 0.3.2 \
      --create-namespace \
      --namespace openchoreo-control-plane \
      --timeout=10m
fi

echo "⏳ Waiting for Control Plane pods to be ready (timeout: 10 minutes)..."
kubectl wait --for=condition=Ready pod --all -n openchoreo-control-plane --timeout=600s
echo "✅ OpenChoreo Control Plane ready"
echo ""

# Step 3: Install OpenChoreo Data Plane
echo "3️⃣  Installing OpenChoreo Data Plane..."
if helm status data-plane -n openchoreo-data-plane &>/dev/null; then
    echo "⏭️  Data Plane already installed, skipping..."
else
    echo "   This may take up to 10 minutes..."
    # Disable cert-manager since it's already installed by control-plane
    helm install data-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
      --version 0.3.2 \
      --create-namespace \
      --namespace openchoreo-data-plane \
      --set cert-manager.enabled=false \
      --set cert-manager.crds.enabled=false \
      --timeout=10m
fi

echo "⏳ Waiting for Data Plane pods to be ready (timeout: 10 minutes)..."
kubectl wait --for=condition=Ready pod --all -n openchoreo-data-plane --timeout=600s
echo "✅ OpenChoreo Data Plane ready"
echo ""

# ============================================================================
# OPTIONAL COMPONENTS
# ============================================================================

# Check if user wants to install optional components
INSTALL_BUILD_PLANE="${INSTALL_BUILD_PLANE:-true}"
INSTALL_OBSERVABILITY="${INSTALL_OBSERVABILITY:-true}"
INSTALL_BACKSTAGE="${INSTALL_BACKSTAGE:-true}"
INSTALL_IDENTITY_PROVIDER="${INSTALL_IDENTITY_PROVIDER:-true}"

if [ "$INSTALL_BUILD_PLANE" = "true" ]; then
    echo "4️⃣  Installing OpenChoreo Build Plane (optional)..."
    if helm status build-plane -n openchoreo-build-plane &>/dev/null; then
        echo "⏭️  Build Plane already installed, skipping..."
    else
        helm install build-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
          --version 0.3.2 \
          --create-namespace \
          --namespace openchoreo-build-plane \
          --timeout=10m
    fi

    echo "⏳ Waiting for Build Plane pods to be ready..."
    kubectl wait --for=condition=Ready pod --all -n openchoreo-build-plane --timeout=600s
    echo "✅ OpenChoreo Build Plane ready"
    echo ""

    # Configure Build Plane
    echo "5️⃣  Configuring Build Plane..."
    if curl -s https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/install/add-build-plane.sh | bash; then
        echo "✅ Build Plane configured successfully"
    else
        echo "⚠️  Build Plane configuration script failed (non-fatal)"
    fi
    echo ""

    # Register the Data Plane
    echo "5️⃣.1 Registering Data Plane..."
    if curl -s https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.3/install/add-default-dataplane.sh | bash; then
        echo "✅ Data Plane registered successfully"
    else
        echo "⚠️  Data Plane registration script failed (non-fatal)"
    fi
    echo ""

    # Install Custom Build CI Workflows
    echo "5️⃣.2 Installing Custom Build CI Workflows..."
    if helm status custom-build-ci-workflows -n openchoreo-build-plane &>/dev/null; then
        echo "⏭️  Custom Build CI Workflows already installed, skipping..."
    else
        helm install custom-build-ci-workflows ../helm-charts/build-ci --namespace openchoreo-build-plane
        echo "✅ Custom Build CI Workflows installed successfully"
    fi
    echo ""
fi

if [ "$INSTALL_OBSERVABILITY" = "true" ]; then
    echo "6️⃣  Installing OpenChoreo Observability Plane (optional)..."
    if helm status observability-plane -n openchoreo-observability-plane &>/dev/null; then
        echo "⏭️  Observability Plane already installed, skipping..."
    else
        echo "   This includes OpenSearch and OpenSearch Dashboards..."
        helm install observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
          --wait \
          --version 0.3.2 \
          --create-namespace \
          --namespace openchoreo-observability-plane \
          --timeout=15m
    fi

    echo "⏳ Waiting for OpenSearch and OpenSearch Dashboards pods to be ready..."
    kubectl wait --for=condition=Ready pod --all -n openchoreo-observability-plane --timeout=900s || {
        echo "⚠️  Some OpenSearch and OpenSearch Dashboards pods may still be starting (non-fatal)"
    }
    echo "✅ OpenSearch and OpenSearch Dashboards ready"

    if helm status observability-dataprepper -n openchoreo-observability-plane &>/dev/null; then
        echo "⏭️  Observability Dataprepper already installed, skipping..."
    else
        echo "Building and loading Traces Observer Service Docker image into Kind cluster..."
        make -C $1/traces-observer-service docker-load-kind
        sleep 10        
        echo "   Installing Dataprepper & Traces Observer Service to the Observability Plane for tracing ingestion..."
        helm install observability-dataprepper $1/deployments/helm-charts/observability-dataprepper \
          --create-namespace \
          --namespace openchoreo-observability-plane \
          --timeout=10m
    fi

    echo "⏳ Waiting for Observability Plane pods to be ready..."
    kubectl wait --for=condition=Ready pod --all -n openchoreo-observability-plane --timeout=600s || {
        echo "⚠️  Some Observability pods may still be starting (non-fatal)"
    }
    echo "✅ OpenChoreo Observability Plane ready"
    echo ""

    # Configure observer only if both Build and Observability planes are installed
    if [ "$INSTALL_BUILD_PLANE" = "true" ]; then
        echo "7️⃣  Configuring observability integration..."

        # Wait for default resources to be created
        echo "   Waiting for default DataPlane and BuildPlane resources..."
        sleep 10

        # Configure DataPlane observer (non-fatal)
        if kubectl get dataplane default -n default &>/dev/null; then
            kubectl patch dataplane default -n default --type merge \
              -p '{"spec":{"observer":{"url":"http://observer.openchoreo-observability-plane:8080","authentication":{"basicAuth":{"username":"dummy","password":"dummy"}}}}}' \
              && echo "   ✅ DataPlane observer configured" \
              || echo "   ⚠️  DataPlane observer configuration failed (non-fatal)"
        else
            echo "   ⚠️  DataPlane resource not found yet (will use default observer)"
        fi

        # Configure BuildPlane observer (non-fatal)
        if kubectl get buildplane default -n default &>/dev/null; then
            kubectl patch buildplane default -n default --type merge \
              -p '{"spec":{"observer":{"url":"http://observer.openchoreo-observability-plane:8080","authentication":{"basicAuth":{"username":"dummy","password":"dummy"}}}}}' \
              && echo "   ✅ BuildPlane observer configured" \
              || echo "   ⚠️  BuildPlane observer configuration failed (non-fatal)"
        else
            echo "   ⚠️  BuildPlane resource not found yet (will use default observer)"
        fi
        echo ""
    fi
fi

if [ "$INSTALL_BACKSTAGE" = "true" ]; then
    echo "8️⃣  Installing OpenChoreo Backstage Portal (optional)..."
    if helm status openchoreo-backstage-demo -n openchoreo-control-plane &>/dev/null; then
        echo "⏭️  Backstage Portal already installed, skipping..."
    else
        helm install openchoreo-backstage-demo oci://ghcr.io/openchoreo/helm-charts/backstage-demo \
          --version 0.3.2 \
          --namespace openchoreo-control-plane
    fi

    echo "⏳ Waiting for Backstage pod to be ready (timeout: 5 minutes)..."
    kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=backstage -n openchoreo-control-plane --timeout=300s
    echo "✅ Backstage Portal ready"
    echo ""
fi

if [ "$INSTALL_IDENTITY_PROVIDER" = "true" ]; then
    echo "9️⃣  Installing OpenChoreo Default Identity Provider (optional)..."
    echo "   ⚠️  Note: This is for demo purposes only, not production-ready"
    if helm status identity-provider -n openchoreo-identity-system &>/dev/null; then
        echo "⏭️  Identity Provider already installed, skipping..."
    else
        helm install identity-provider oci://ghcr.io/openchoreo/helm-charts/openchoreo-identity-provider \
          --version 0.3.2 \
          --create-namespace \
          --namespace openchoreo-identity-system \
          --timeout=10m
    fi

    echo "⏳ Waiting for Identity Provider pods to be ready (timeout: 5 minutes)..."
    kubectl wait --for=condition=Ready pod --all -n openchoreo-identity-system --timeout=300s
    echo "✅ Identity Provider ready"
    echo ""
fi

# ============================================================================
# VERIFICATION
# ============================================================================

echo "🔍 Verifying installation..."
echo ""

echo "Installed components:"
kubectl get pods -n cilium
echo ""
kubectl get pods -n openchoreo-control-plane
echo ""
kubectl get pods -n openchoreo-data-plane
echo ""

if [ "$INSTALL_BUILD_PLANE" = "true" ]; then
    kubectl get pods -n openchoreo-build-plane
    echo ""
fi

if [ "$INSTALL_OBSERVABILITY" = "true" ]; then
    kubectl get pods -n openchoreo-observability-plane
    echo ""
fi

if [ "$INSTALL_BACKSTAGE" = "true" ]; then
    echo "Backstage Portal:"
    kubectl get pods -n openchoreo-control-plane -l app.kubernetes.io/name=backstage
    echo ""
fi

if [ "$INSTALL_IDENTITY_PROVIDER" = "true" ]; then
    echo "Identity Provider:"
    kubectl get pods -n openchoreo-identity-system
    echo ""
fi

echo "✅ OpenChoreo installation complete!"
echo ""
echo "📊 Access services using port-forwarding:"
echo "   # OpenChoreo Control Plane API"
echo "   kubectl port-forward -n openchoreo-control-plane svc/openchoreo-control-plane 8000:8080"
echo ""

if [ "$INSTALL_OBSERVABILITY" = "true" ]; then
    echo "   # OpenSearch"
    echo "   kubectl port-forward -n openchoreo-observability-plane svc/opensearch 9200:9200"
    echo ""
    echo "   # OpenSearch Dashboards"
    echo "   kubectl port-forward -n openchoreo-observability-plane svc/opensearch-dashboards 5601:5601"
    echo ""
fi

if [ "$INSTALL_BACKSTAGE" = "true" ]; then
    echo "   # Backstage Portal"
    echo "   kubectl port-forward -n openchoreo-control-plane svc/backstage-demo 7007:7007"
    echo "   Then access: http://localhost:7007"
    echo ""
fi

if [ "$INSTALL_IDENTITY_PROVIDER" = "true" ]; then
    echo "   # Identity Provider"
    echo "   kubectl port-forward -n openchoreo-identity-system svc/identity-provider 9090:8090"
    echo ""
fi

echo "   Or use: make port-forward"
echo ""
echo "💡 To skip optional components:"
echo "   INSTALL_BUILD_PLANE=false INSTALL_OBSERVABILITY=false INSTALL_BACKSTAGE=false INSTALL_IDENTITY_PROVIDER=false ./setup-openchoreo.sh"
