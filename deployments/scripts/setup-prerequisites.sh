#!/bin/bash
set -e
# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"
source "$SCRIPT_DIR/env.sh"

echo "=== Installing Pre-requisites for OpenChoreo ==="
if ! kubectl cluster-info --context $CLUSTER_CONTEXT &> /dev/null; then
    echo "❌ K3d cluster '$CLUSTER_CONTEXT' is not running."
    echo "   Run: ./setup-k3d.sh"
    exit 1
fi

# Install Gateway API CRDs
echo ""
echo "🔧 Installing Gateway API CRDs..."
GATEWAY_API_CRD="https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml"
if kubectl apply --server-side --force-conflicts -f "${GATEWAY_API_CRD}" &>/dev/null; then
    echo "✅ Gateway API CRDs applied successfully"
else
    echo "❌ Failed to apply Gateway API CRDs"
    exit 1
fi

# Install cert-manager
echo ""
echo "🔧 Installing cert-manager..."
CERT_MANAGER_VERSION="v1.18.4"
 helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
    --kube-context ${CLUSTER_CONTEXT} \
    --version ${CERT_MANAGER_VERSION} \
    --namespace cert-manager \
    --create-namespace \
    --set crds.enabled=true

echo ""
echo "⏳ Waiting for cert-manager to be ready..."
kubectl wait --for=condition=available deployment/cert-manager -n cert-manager --context ${CLUSTER_CONTEXT} --timeout=120s

echo ""
echo "✅ cert-manager is ready!"

# Install External Secret Operator
echo ""
echo "🔧 Installing External Secret Operator..."
EXTERNAL_SECRETS_VERSION="1.3.2"
helm upgrade --install external-secrets oci://ghcr.io/external-secrets/charts/external-secrets \
    --kube-context ${CLUSTER_CONTEXT} \
    --namespace external-secrets \
    --create-namespace \
    --version ${EXTERNAL_SECRETS_VERSION} \
    --set installCRDs=true
echo ""
echo "⏳ Waiting for External Secret Operator to be ready..."
kubectl wait --for=condition=Available deployment/external-secrets -n external-secrets --context ${CLUSTER_CONTEXT} --timeout=180s
echo "✅ External Secret Operator is ready!"


# Install Kgateway CRDs
echo ""
echo "🔧 Installing Kgateway CRDs..."
helm upgrade --install kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
  --create-namespace --namespace openchoreo-control-plane \
  --version v2.2.1

helm upgrade --install kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
  --namespace openchoreo-control-plane --create-namespace \
  --version v2.2.1 \
  --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true

echo ""
echo "✅ Kgateway installed successfully!"
echo ""

echo "🔧 Applying OpenChoreo secrets..."
if kubectl apply -f - <<EOF
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
echo "✅ OpenChoreo secrets applied successfully!"
else
    echo "❌ Failed to apply OpenChoreo secrets"
    exit 1
fi
