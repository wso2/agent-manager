#!/bin/bash
set -e
# Get the absolute directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Change to script directory to ensure consistent working directory
cd "$SCRIPT_DIR"
source "$SCRIPT_DIR/env.sh"

# Utility function to install helm chart only if not already installed
helm_install_if_not_exists() {
    local release_name="$1"
    local namespace="$2"
    local chart="$3"
    shift 3
    local extra_args=("$@")

    if helm status "$release_name" -n "$namespace" --kube-context ${CLUSTER_CONTEXT} &>/dev/null; then
        echo "⏭️  $release_name already installed in $namespace, skipping..."
        return 0
    fi

    echo "📦 Installing $release_name..."
    helm install "$release_name" "$chart" \
        --kube-context ${CLUSTER_CONTEXT} \
        --namespace "$namespace" \
        --create-namespace \
        "${extra_args[@]}"
}

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
helm_install_if_not_exists "cert-manager" "cert-manager" \
    "oci://quay.io/jetstack/charts/cert-manager" \
    --version ${CERT_MANAGER_VERSION} \
    --set crds.enabled=true

echo "⏳ Waiting for cert-manager to be ready..."
kubectl wait --for=condition=available deployment/cert-manager -n cert-manager --context ${CLUSTER_CONTEXT} --timeout=120s
echo "✅ cert-manager is ready!"

# Install External Secret Operator
echo ""
echo "🔧 Installing External Secret Operator..."
EXTERNAL_SECRETS_VERSION="1.3.2"
helm_install_if_not_exists "external-secrets" "external-secrets" \
    "oci://ghcr.io/external-secrets/charts/external-secrets" \
    --version ${EXTERNAL_SECRETS_VERSION} \
    --set installCRDs=true

echo "⏳ Waiting for External Secret Operator to be ready..."
kubectl wait --for=condition=Available deployment --all -n external-secrets --context ${CLUSTER_CONTEXT} --timeout=180s
echo "✅ External Secret Operator is ready!"

# Install Kgateway CRDs
echo ""
echo "🔧 Installing Kgateway..."
KGATEWAY_VERSION="v2.2.1"
helm_install_if_not_exists "kgateway-crds" "openchoreo-control-plane" \
    "oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds" \
    --version ${KGATEWAY_VERSION}

helm_install_if_not_exists "kgateway" "openchoreo-control-plane" \
    "oci://cr.kgateway.dev/kgateway-dev/charts/kgateway" \
    --version ${KGATEWAY_VERSION} \
    --set controller.extraEnv.KGW_ENABLE_GATEWAY_API_EXPERIMENTAL_FEATURES=true

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
then
    echo "✅ OpenChoreo secrets applied successfully!"
else
    echo "❌ Failed to apply OpenChoreo secrets"
    exit 1
fi
