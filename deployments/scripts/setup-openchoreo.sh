#!/bin/bash
set -e

PROJECT_ROOT="$1"
CLUSTER_NAME="openchoreo-local-v0.7"
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
echo "   Reference: https://openchoreo.dev/docs/getting-started/single-cluster/"
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
    helm install openchoreo-control-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
    --version 0.7.0 \
    --namespace openchoreo-control-plane \
    --create-namespace \
    --values https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.7/install/k3d/single-cluster/values-cp.yaml \
    --set global.defaultResources.enabled=false
fi

echo "⏳ Waiting for Control Plane pods to be ready (timeout: 10 minutes)..."
kubectl wait --for=condition=Ready pod --all -n openchoreo-control-plane --timeout=600s
echo "✅ OpenChoreo Control Plane ready"
echo ""

# Step 2: Install OpenChoreo Data Plane
echo "3️⃣  Installing OpenChoreo Data Plane..."
if helm status openchoreo-data-plane -n openchoreo-data-plane &>/dev/null; then
    echo "⏭️  Data Plane already installed, skipping..."
else
    echo "   This may take up to 10 minutes..."
    helm install openchoreo-data-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
    --version 0.7.0 \
    --namespace openchoreo-data-plane \
    --create-namespace \
    --values https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.7/install/k3d/single-cluster/values-dp.yaml
fi

# Registering the Data Plane
echo "5️⃣.1 Registering Data Plane..."
if curl -s https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.7/install/add-data-plane.sh | bash -s -- --enable-agent --control-plane-context ${CLUSTER_CONTEXT} --name default; then
    echo "✅ Data Plane registered successfully"
else
    echo "⚠️  Data Plane registration script failed (non-fatal)"
fi

 # Verify DataPlane resource and agent mode
echo ""
echo "🔍 Verifying DataPlane resource..."
if kubectl get dataplane default -n default &>/dev/null; then
    echo "✅ DataPlane resource 'default' exists"
    AGENT_ENABLED=$(kubectl get dataplane default -n default -o jsonpath='{.spec.agent.enabled}' 2>/dev/null || echo "false")
    if [ "$AGENT_ENABLED" = "true" ]; then
        echo "✅ Agent mode is enabled"
    else
        echo "⚠️  Agent mode is not enabled (expected: true, got: $AGENT_ENABLED)"
    fi
else
    echo "⚠️  DataPlane resource not found"
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
    if helm status openchoreo-build-plane -n openchoreo-build-plane &>/dev/null; then
        echo "⏭️  Build Plane already installed, skipping..."
    else
        helm install openchoreo-build-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
        --version 0.7.0 \
        --namespace openchoreo-build-plane \
        --create-namespace \
        --values https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.7/install/k3d/single-cluster/values-bp.yaml
    fi

    # Register Build Plane
    echo "5️⃣  Registering Build Plane..."
    if curl -s https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.7/install/add-build-plane.sh | bash -s -- --enable-agent --control-plane-context ${CLUSTER_CONTEXT} --name default; then
        echo "✅ Build Plane registered successfully"
    else
        echo "⚠️  Build Plane registration script failed (non-fatal)"
    fi
    echo ""

    # Verify BuildPlane resource and agent mode
    echo ""
    echo "🔍 Verifying BuildPlane resource..."
    if kubectl get buildplane default -n default &>/dev/null; then
        echo "✅ BuildPlane resource 'default' exists"
        AGENT_ENABLED=$(kubectl get buildplane default -n default -o jsonpath='{.spec.agent.enabled}' 2>/dev/null || echo "false")
        if [ "$AGENT_ENABLED" = "true" ]; then
            echo "✅ Agent mode is enabled"
        else
            echo "⚠️  Agent mode is not enabled (expected: true, got: $AGENT_ENABLED)"
        fi
    else
        echo "⚠️  BuildPlane resource not found"
    fi

    echo "⏳ Waiting for Build Plane pods to be ready..."
    kubectl wait --for=condition=Available deployment --all -n openchoreo-build-plane --timeout=600s
    echo "✅ OpenChoreo Build Plane ready"
    echo ""

    echo $PROJECT_ROOT
    # Install Custom Build CI Workflows
    echo "5️⃣.2 Installing Custom Build CI Workflows..."
    if helm status custom-build-ci-workflows -n openchoreo-build-plane &>/dev/null; then
        echo "⏭️  Custom Build CI Workflows already installed, skipping..."
    else
        helm install custom-build-ci-workflows $PROJECT_ROOT/deployments/helm-charts/wso2-amp-build-extension --namespace openchoreo-build-plane
        echo "✅ Custom Build CI Workflows installed successfully"
    fi
    echo ""
fi

if [ "$INSTALL_OBSERVABILITY" = "true" ]; then
    echo "6️⃣  Installing OpenChoreo Observability Plane (optional)..."
    if helm status openchoreo-observability-plane -n openchoreo-observability-plane &>/dev/null; then
        echo "⏭️  Observability Plane already installed, skipping..."
    else
        echo "   This includes OpenSearch and OpenSearch Dashboards..."
        helm install openchoreo-observability-plane oci://ghcr.io/openchoreo/helm-charts/openchoreo-observability-plane \
        --version 0.7.0 \
        --namespace openchoreo-observability-plane \
        --create-namespace \
        --values https://raw.githubusercontent.com/openchoreo/openchoreo/release-v0.7/install/k3d/single-cluster/values-op.yaml
    fi

    echo "⏳ Waiting for OpenSearch and OpenSearch Dashboards pods to be ready..."
    kubectl wait --for=condition=Ready pod --all -n openchoreo-observability-plane --timeout=900s || {
        echo "⚠️  Some OpenSearch and OpenSearch Dashboards pods may still be starting (non-fatal)"
    }
    echo "✅ OpenSearch and OpenSearch Dashboards ready"

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

# ============================================================================
# VERIFICATION
# ============================================================================

echo "🔍 Verifying installation..."
echo ""

echo "Verify Plane Resources:"
kubectl get dataplane,buildplane -A
echo ""

echo "=== DataPlane Agent Status ==="
kubectl get pods -n openchoreo-data-plane -l app=cluster-agent
echo ""

echo "=== DataPlane Agent Connection Logs ==="
kubectl logs -n openchoreo-data-plane -l app=cluster-agent --tail=5 2>/dev/null | grep "connected to control plane" || echo "   (No connection logs found or agent not ready)"
echo ""

if [ "$INSTALL_BUILD_PLANE" = "true" ]; then
    echo "=== BuildPlane Agent Status ==="
    kubectl get pods -n openchoreo-build-plane -l app=cluster-agent
    echo ""

    echo "=== BuildPlane Agent Connection Logs ==="
    kubectl logs -n openchoreo-build-plane -l app=cluster-agent --tail=5 2>/dev/null | grep "connected to control plane" || echo "   (No connection logs found or agent not ready)"
    echo ""
fi

echo "=== Gateway Registration ==="
kubectl logs -n openchoreo-control-plane -l app=cluster-gateway --tail=20 2>/dev/null | grep "agent registered" | tail -5 || echo "   (No registration logs found or gateway not ready)"
echo ""

echo "Verify All Resources:"
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

echo "✅ OpenChoreo installation complete!"
echo ""
echo "📊 Backstage Console:	http://openchoreo.localhost:8080"
echo "   (username: admin@openchoreo.dev, password: Admin@123)"
echo "🔍 API: http://api.openchoreo.localhost:8080"
echo ""
echo "💡 To skip optional components:"
echo "   INSTALL_BUILD_PLANE=false INSTALL_OBSERVABILITY=false"
