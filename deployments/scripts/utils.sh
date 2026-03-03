

# Util: Register DataPlane
register_data_plane() {
    # $1: CA (already base64 decoded)
    # $2: planeID (e.g. "default")
    # $3: secretStoreRef name (empty if not needed)
    local ca_cert="$1"
    local plane_id="$2"
    local secret_store="$3"

    if [ -n "$ca_cert" ]; then
        echo "Registering DataPlane ..."
        cat <<EOF | kubectl apply -f -
apiVersion: openchoreo.dev/v1alpha1
kind: DataPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "$plane_id"
$( [ -n "$secret_store" ] && echo "  secretStoreRef:\n    name: $secret_store" )
  clusterAgent:
    clientCA:
      value: |
$(echo "$ca_cert" | sed 's/^/        /')
  gateway:
    publicVirtualHost: "openchoreoapis.localhost"
    publicHTTPPort: 19080
    publicHTTPSPort: 19443
EOF
        echo "✅ DataPlane registered successfully"
    else
        echo "⚠️  CA certificate not found; skipping DataPlane registration"
    fi
}

# Util: Register BuildPlane
register_build_plane() {
    # $1: CA (already base64 decoded)
    # $2: planeID (e.g. "default")
    # $3: secretStoreRef name (empty if not needed)
    local ca_cert="$1"
    local plane_id="$2"
    local secret_store="$3"

    if [ -n "$ca_cert" ]; then
        echo "Registering BuildPlane ..."
        cat <<EOF | kubectl apply -f -
apiVersion: openchoreo.dev/v1alpha1
kind: BuildPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "$plane_id"
$( [ -n "$secret_store" ] && echo "  secretStoreRef:\n    name: $secret_store" )
  clusterAgent:
    clientCA:
      value: |
$(echo "$ca_cert" | sed 's/^/        /')
EOF
        echo "✅ BuildPlane registered successfully"
    else
        echo "⚠️  CA certificate not found; skipping BuildPlane registration"
    fi
}

# Util: Register ObservabilityPlane
register_observability_plane() {
    # $1: CA (already base64 decoded)
    # $2: planeID (e.g. "default")
    # $3: observerURL (required)
    # $4: secretStoreRef name (empty if not needed)
    local ca_cert="$1"
    local plane_id="$2"
    local observer_url="$3"
    local secret_store="$4"

    if [ -n "$ca_cert" ]; then
        echo "Registering ObservabilityPlane ..."
        cat <<EOF | kubectl apply -f -
apiVersion: openchoreo.dev/v1alpha1
kind: ObservabilityPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "$plane_id"
$( [ -n "$secret_store" ] && echo "  secretStoreRef:\n    name: $secret_store" )
  clusterAgent:
    clientCA:
      value: |
$(echo "$ca_cert" | sed 's/^/        /')
  observerURL: $observer_url
EOF
        echo "✅ ObservabilityPlane registered successfully"
    else
        echo "⚠️  CA certificate not found; skipping ObservabilityPlane registration"
    fi
}

# Util to create/external secrets for OpenChoreo Observability Plane
create_external_secrets_obs_plane() {
    local ns="openchoreo-observability-plane"
    kubectl apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: observer-opensearch-credentials
  namespace: $ns
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: observer-opensearch-credentials
  data:
  - secretKey: username
    remoteRef:
      key: opensearch-username
  - secretKey: password
    remoteRef:
      key: opensearch-password
EOF
    
    kubectl apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: opensearch-admin-credentials
  namespace: $ns
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: opensearch-admin-credentials
  data:
  - secretKey: username
    remoteRef:
      key: opensearch-username
  - secretKey: password
    remoteRef:
      key: opensearch-password
EOF
    echo "✅ External secrets for OpenChoreo Observability Plane created"
}
create_plane_cert_resources() {
  local PLANE_NAMESPACE="$1"

  # 1. Create namespace if not exists
  kubectl create namespace "$PLANE_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

  # 2. Copy cluster-gateway-ca ConfigMap from control-plane to desired namespace
  CA_CRT=$(kubectl get configmap cluster-gateway-ca \
    -n openchoreo-control-plane -o jsonpath='{.data.ca\.crt}')

  kubectl create configmap cluster-gateway-ca \
    --from-literal=ca.crt="$CA_CRT" \
    -n "$PLANE_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

  # 3. Copy cluster-gateway-ca Secret from control-plane to desired namespace
  TLS_CRT=$(kubectl get secret cluster-gateway-ca \
    -n openchoreo-control-plane -o jsonpath='{.data.tls\.crt}' | base64 -d)
  TLS_KEY=$(kubectl get secret cluster-gateway-ca \
    -n openchoreo-control-plane -o jsonpath='{.data.tls\.key}' | base64 -d)

  kubectl create secret generic cluster-gateway-ca \
    --from-literal=tls.crt="$TLS_CRT" \
    --from-literal=tls.key="$TLS_KEY" \
    --from-literal=ca.crt="$CA_CRT" \
    -n "$PLANE_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
}



