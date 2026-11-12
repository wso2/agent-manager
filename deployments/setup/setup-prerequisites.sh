#!/bin/bash
set -e

# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"
source "$SCRIPT_DIR/env.sh"
source "$SCRIPT_DIR/utils.sh"

# ============================================================================
# Version Constants
# ============================================================================
GATEWAY_API_VERSION="v1.4.1"
CERT_MANAGER_VERSION="v1.19.2"
KGATEWAY_VERSION="v2.3.11"

echo "=== Installing cluster prerequisites ==="

# Check prerequisites
if ! kubectl cluster-info --context $CLUSTER_CONTEXT &> /dev/null; then
    echo "❌ K3d cluster '$CLUSTER_CONTEXT' is not running."
    echo "   Run: ./setup-k3d.sh"
    exit 1
fi

# ============================================================================
# Step 1: Install Gateway API CRDs
# ============================================================================
echo ""
echo "1️⃣  Gateway API CRDs"
GATEWAY_API_CRD="https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/experimental-install.yaml"
if kubectl --context "${CLUSTER_CONTEXT}" apply --server-side --force-conflicts -f "${GATEWAY_API_CRD}" &>/dev/null; then
    echo "✅ Gateway API CRDs applied successfully"
else
    echo "❌ Failed to apply Gateway API CRDs"
    exit 1
fi

# ============================================================================
# Step 2: Install cert-manager
# ============================================================================
echo ""
echo "2️⃣  cert-manager"
helm_install_if_not_exists "cert-manager" "cert-manager" \
    "oci://quay.io/jetstack/charts/cert-manager" \
    --version ${CERT_MANAGER_VERSION} \
    --set crds.enabled=true

echo "⏳ Waiting for cert-manager to be ready..."
kubectl wait --for=condition=available deployment/cert-manager -n cert-manager --context ${CLUSTER_CONTEXT} --timeout=120s
echo "✅ cert-manager is ready!"

# ============================================================================
# Step 3: Install Kgateway
# ============================================================================
echo ""
echo "3️⃣  Kgateway"
helm_install_if_not_exists "kgateway-crds" "kgateway-system" \
    "oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds" \
    --version ${KGATEWAY_VERSION}

helm_install_if_not_exists "kgateway" "kgateway-system" \
    "oci://cr.kgateway.dev/kgateway-dev/charts/kgateway" \
    --version ${KGATEWAY_VERSION} \
    --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true
echo "✅ Kgateway installed successfully"

# ============================================================================
# Step 4: AMP Gateway (shared ingress the AMP charts attach HTTPRoutes to)
# ============================================================================
echo ""
echo "4️⃣  AMP Gateway"
kubectl --context "${CLUSTER_CONTEXT}" apply -f "${SCRIPT_DIR}/../k8s/amp-gateway.yaml"
echo "✅ AMP Gateway applied"

echo ""
echo "✅ All prerequisites installed successfully!"
