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
EXTERNAL_SECRETS_VERSION="1.3.2"
KGATEWAY_VERSION="v2.2.1"

echo "=== Installing Pre-requisites for OpenChoreo ==="

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
# Step 3: Install External Secrets Operator
# ============================================================================
echo ""
echo "3️⃣  External Secrets Operator"
helm_install_if_not_exists "external-secrets" "external-secrets" \
    "oci://ghcr.io/external-secrets/charts/external-secrets" \
    --version ${EXTERNAL_SECRETS_VERSION} \
    --set installCRDs=true

echo "⏳ Waiting for External Secrets Operator to be ready..."
kubectl wait --for=condition=Available deployment --all -n external-secrets --context ${CLUSTER_CONTEXT} --timeout=180s
echo "✅ External Secrets Operator is ready!"

# ============================================================================
# Step 4: Install Kgateway
# ============================================================================
echo ""
echo "4️⃣  Kgateway"
helm_install_if_not_exists "kgateway-crds" "openchoreo-control-plane" \
    "oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds" \
    --version ${KGATEWAY_VERSION}

helm_install_if_not_exists "kgateway" "openchoreo-control-plane" \
    "oci://cr.kgateway.dev/kgateway-dev/charts/kgateway" \
    --version ${KGATEWAY_VERSION} \
    --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true

# ============================================================================
# Step 5: Apply OpenChoreo Secrets
# ============================================================================
echo ""
echo "5️⃣  OpenChoreo Secrets"
if kubectl --context "${CLUSTER_CONTEXT}" apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ClusterSecretStore
metadata:
  name: default
spec:
  provider:
    fake:
      data:
       # OpenSearch (observability)
      - key: opensearch-username
        value: "admin"
      - key: opensearch-password
        value: "ThisIsTheOpenSearchPassword1"
EOF
then
    echo "✅ OpenChoreo secrets applied successfully!"
else
    echo "❌ Failed to apply OpenChoreo secrets"
    exit 1
fi

echo ""
echo "✅ All prerequisites installed successfully!"
