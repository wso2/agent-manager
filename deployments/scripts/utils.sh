# Util: Check if a command is installed
check_command() {
    local cmd="$1"
    if ! command -v "$cmd" &> /dev/null; then
        echo "❌ $cmd is not installed. Please install it first:"
        echo "   brew install $cmd"
        exit 1
    fi
}

# Util: Install helm chart only if not already installed
helm_install_if_not_exists() {
    local release_name="$1"
    local namespace="$2"
    local chart="$3"
    shift 3
    local extra_args=("$@")

    if helm status "$release_name" -n "$namespace" &>/dev/null; then
        echo "⏭️  $release_name already installed in $namespace, skipping..."
        return 0
    fi

    echo "📦 Installing $release_name..."
    helm install "$release_name" "$chart" \
        --namespace "$namespace" \
        --create-namespace \
        "${extra_args[@]}"
    echo "✅ $release_name installed successfully"
}

# Util: Generate machine IDs for k3d nodes (required for Fluent Bit observability)
generate_machine_ids() {
    local cluster_name="$1"
    echo "🆔 Generating Machine IDs for Fluent Bit observability..."

    # Extract node names from k3d node list JSON output
    local json_output
    json_output=$(k3d node list -o json)

    local nodes
    nodes=$(echo "$json_output" \
        | grep -o '"name"[[:space:]]*:[[:space:]]*"[^"]*"' \
        | sed 's/"name"[[:space:]]*:[[:space:]]*"//;s/"$//' \
        | grep "^k3d-${cluster_name}-")

    if [[ -z "$nodes" ]]; then
        echo "⚠️  Could not retrieve node list"
        return 1
    fi

    for node in $nodes; do
        echo "   🔧 Generating machine ID for ${node}..."
        if docker exec "${node}" sh -c "cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id" 2>/dev/null; then
            echo "   ✅ Machine ID generated for ${node}"
        else
            echo "   ⚠️  Could not generate Machine ID for ${node} (it may not be running)"
        fi
    done

    echo "✅ Machine ID generation complete"
}

# Util: Refresh kubeconfig for k3d cluster
refresh_kubeconfig() {
    echo "🔄 Refreshing kubeconfig..."
    k3d kubeconfig merge ${CLUSTER_NAME} --kubeconfig-merge-default --kubeconfig-switch-context
}

# Util: Wait for cluster to be ready (max 30 attempts, 2s interval)
wait_for_cluster() {
    echo "⏳ Waiting for cluster to be ready..."
    for i in {1..30}; do
        if kubectl cluster-info --context ${CLUSTER_CONTEXT} --request-timeout=5s &>/dev/null; then
            echo "✅ Cluster is now ready"
            return 0
        fi
        echo "   Attempt $i/30..."
        sleep 2
    done
    return 1
}

# Util: Ensure cluster is accessible (refresh kubeconfig + wait)
ensure_cluster_accessible() {
    refresh_kubeconfig

    echo "🔍 Checking cluster accessibility..."
    if kubectl cluster-info --context ${CLUSTER_CONTEXT} --request-timeout=10s &>/dev/null; then
        echo "✅ Cluster is running and accessible"
        return 0
    fi

    echo "⚠️  Cluster not accessible. Restarting..."
    k3d cluster stop ${CLUSTER_NAME} 2>/dev/null || true
    k3d cluster start ${CLUSTER_NAME}

    refresh_kubeconfig
    wait_for_cluster
}

# Util: Register DataPlane
register_data_plane() {
    # $1: CA (already base64 decoded)
    # $2: planeID (e.g. "default")
    # $3: secretStoreRef name (empty if not needed)
    local ca_cert="$1"
    local plane_id="$2"
    local secret_store="$3"

    if [ -z "$ca_cert" ]; then
        echo "❌ CA certificate not found. Cannot register DataPlane."
        echo "   Ensure cluster-agent-tls secret exists in openchoreo-data-plane namespace."
        exit 1
    fi

    echo "Registering DataPlane ..."
    cat <<EOF | kubectl apply -f -
apiVersion: openchoreo.dev/v1alpha1
kind: DataPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "$plane_id"
  secretStoreRef:
    name: "$secret_store"
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
}

# Util: Register BuildPlane
register_build_plane() {
    # $1: CA (already base64 decoded)
    # $2: planeID (e.g. "default")
    # $3: secretStoreRef name (empty if not needed)
    local ca_cert="$1"
    local plane_id="$2"
    local secret_store="$3"

    if [ -z "$ca_cert" ]; then
        echo "❌ CA certificate not found. Cannot register BuildPlane."
        echo "   Ensure cluster-agent-tls secret exists in openchoreo-build-plane namespace."
        exit 1
    fi

    echo "Registering BuildPlane ..."
    cat <<EOF | kubectl apply -f -
apiVersion: openchoreo.dev/v1alpha1
kind: BuildPlane
metadata:
  name: default
  namespace: default
spec:
  planeID: "$plane_id"
  secretStoreRef:
    name: "$secret_store"
  clusterAgent:
    clientCA:
      value: |
$(echo "$ca_cert" | sed 's/^/        /')
EOF
    echo "✅ BuildPlane registered successfully"
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

    if [ -z "$ca_cert" ]; then
        echo "❌ CA certificate not found. Cannot register ObservabilityPlane."
        echo "   Ensure cluster-agent-tls secret exists in openchoreo-observability-plane namespace."
        exit 1
    fi

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
  echo "Setting up certificate resources in namespace '$PLANE_NAMESPACE'..."
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

# Util: Run multiple tasks in parallel and collect results
# Usage: run_parallel_tasks "task1_name:task1_func" "task2_name:task2_func" ...
# Each task is a "name:function" pair. Function args can be passed after the function name.
run_parallel_tasks() {
    local tasks=("$@")
    local pids=()
    local logs=()
    local names=()

    # Start all tasks in background
    for task in "${tasks[@]}"; do
        local name="${task%%:*}"
        local func="${task#*:}"
        local log_file
        log_file=$(mktemp)

        names+=("$name")
        logs+=("$log_file")

        # Run function in background, capturing output
        eval "$func" > "$log_file" 2>&1 &
        pids+=($!)
        echo "   Started: $name (PID: ${pids[-1]})"
    done

    echo ""

    # Wait for all tasks and collect exit statuses
    local statuses=()
    for pid in "${pids[@]}"; do
        wait "$pid" || true
        statuses+=($?)
    done

    # Output all logs
    echo ""
    for i in "${!names[@]}"; do
        echo "========== ${names[$i]} =========="
        cat "${logs[$i]}"
        echo ""
    done

    # Cleanup temp files
    for log_file in "${logs[@]}"; do
        rm -f "$log_file"
    done

    # Check for failures
    local failed=0
    for i in "${!statuses[@]}"; do
        if [ "${statuses[$i]}" -ne 0 ]; then
            echo "❌ ${names[$i]} failed with exit code: ${statuses[$i]}"
            failed=1
        fi
    done

    return $failed
}

# Util: Wait for all deployments in a namespace to be ready
# Usage: wait_for_namespace_ready "namespace" "label" [timeout_seconds]
wait_for_namespace_ready() {
    local namespace="$1"
    local label="$2"
    local timeout="${3:-300}"

    echo "⏳ Waiting for $label deployments..."
    if kubectl wait -n "$namespace" --for=condition=available --timeout="${timeout}s" deployment --all 2>&1; then
        echo "✅ $label ready"
    else
        echo "⚠️  $label: some deployments may not be ready"
    fi
}

# Util: Wait for pods with a specific label to be ready (for StatefulSets)
# Usage: wait_for_pods_ready "namespace" "label_selector" "display_name" [timeout_seconds]
wait_for_pods_ready() {
    local namespace="$1"
    local selector="$2"
    local display_name="$3"
    local timeout="${4:-120}"

    echo "⏳ Waiting for $display_name pods..."
    if kubectl wait -n "$namespace" --for=condition=ready pod -l "$selector" --timeout="${timeout}s" 2>&1; then
        echo "✅ $display_name ready"
    else
        echo "⚠️  $display_name: some pods may not be ready"
    fi
}




