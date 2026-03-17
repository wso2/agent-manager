
# Util: Get minimum required version for a command (bash 3.x compatible)
get_min_version() {
    case "$1" in
        docker)  echo "26" ;;
        k3d)     echo "5.8" ;;
        kubectl) echo "1.32" ;;
        helm)    echo "3.12" ;;
        *)       echo "" ;;
    esac
}

# Util: check version is greater than or equal by comparing two version strings (returns 0 if $1 >= $2)
version_gte() {
    local ver1="$1"
    local ver2="$2"
    # Use sort -V for version comparison
    [ "$(printf '%s\n%s' "$ver2" "$ver1" | sort -V | head -n1)" = "$ver2" ]
}

# Util: Extract version number from command output
get_version() {
    "$1" version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+' | head -1
}

# Util: Check if a command is installed and version is compatible
check_command() {
    local cmd="$1"
    if ! command -v "$cmd" &> /dev/null; then
        echo "❌ $cmd is not installed. Please install it first:"
        echo "   brew install $cmd"
        exit 1
    fi

    # Check version compatibility
    local min_version
    min_version=$(get_min_version "$cmd")
    if [ -n "$min_version" ]; then
        local current_version
        current_version=$(get_version "$cmd")

        if [ -n "$current_version" ]; then
            if ! version_gte "$current_version" "$min_version"; then
                echo "⚠️  Warning: $cmd version $current_version is below minimum required v$min_version+"
            fi
        fi
    fi
}

# Util: Install helm chart only if not already installed
helm_install_if_not_exists() {
    local release_name="$1"
    local namespace="$2"
    local chart="$3"
    shift 3
    local extra_args=("$@")

    if helm status "$release_name" -n "$namespace" --kube-context "${CLUSTER_CONTEXT}" &>/dev/null; then
        echo "⏭️  $release_name already installed in $namespace, skipping..."
        return 0
    fi

    echo "📦 Installing $release_name..."
    helm install "$release_name" "$chart" \
        --namespace "$namespace" \
        --create-namespace \
        --kube-context "${CLUSTER_CONTEXT}" \
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
    # $3: secretStoreRef name (required)
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
kind: ClusterDataPlane
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
    ingress:
        external:
            name: gateway-default
            namespace: openchoreo-data-plane
            http:
                host: "openchoreoapis.localhost"
                listenerName: http
                port: 19080
            https:
                host: "openchoreoapis.localhost"
                listenerName: https
                port: 19443      
EOF
    echo "✅ DataPlane registered successfully"
}

# Util: Register WorkflowPlane
register_workflow_plane() {
    # $1: CA (already base64 decoded)
    # $2: planeID (e.g. "default")
    # $3: secretStoreRef name (required)
    local ca_cert="$1"
    local plane_id="$2"
    local secret_store="$3"

    if [ -z "$ca_cert" ]; then
        echo "❌ CA certificate not found. Cannot register WorkflowPlane."
        echo "   Ensure cluster-agent-tls secret exists in openchoreo-workflow-plane namespace."
        exit 1
    fi

    echo "Registering WorkflowPlane ..."
    cat <<EOF | kubectl apply -f -
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterWorkflowPlane
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
    echo "✅ WorkflowPlane registered successfully"
}

# Util: Register ObservabilityPlane
register_observability_plane() {
    # $1: CA (already base64 decoded)
    # $2: planeID (e.g. "default")
    # $3: observerURL (required)
    local ca_cert="$1"
    local plane_id="$2"
    local observer_url="$3"

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
      property: value
  - secretKey: password
    remoteRef:
      key: opensearch-password
      property: value
EOF
    
    kubectl apply -f - <<EOF
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: observer-secret
  namespace: $ns
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  target:
    name: observer-secret
  data:
  - secretKey: OPENSEARCH_USERNAME
    remoteRef:
      key: opensearch-username
      property: value
  - secretKey: OPENSEARCH_PASSWORD
    remoteRef:
      key: opensearch-password
      property: value
  - secretKey: UID_RESOLVER_OAUTH_CLIENT_SECRET
    remoteRef:
      key: observer-oauth-client-secret
      property: value
EOF
    echo "✅ External secrets for OpenChoreo Observability Plane created"

}

create_plane_cert_resources() {
  local PLANE_NAMESPACE="$1"
  echo "Setting up certificate resources in namespace '$PLANE_NAMESPACE'..."
  # 1. Create namespace if not exists
  kubectl create namespace "$PLANE_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

  # 2. Wait for cert-manager to issue the cluster-gateway CA
    kubectl wait -n openchoreo-control-plane --for=condition=Ready certificate/cluster-gateway-ca --timeout=120s

  # 3. Copy cluster-gateway-ca ConfigMap from control-plane to desired namespace
  CA_CRT=$(kubectl get secret cluster-gateway-ca \
    -n openchoreo-control-plane -o jsonpath='{.data.ca\.crt}'| base64 -d)

  kubectl create configmap cluster-gateway-ca \
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
        local pid=$!
        pids+=("$pid")
        echo "   Started: $name (PID: $pid)"
    done

    echo ""

    # Wait for all tasks and collect exit statuses
    local statuses=()
    local status
    for pid in "${pids[@]}"; do
        wait "$pid" && status=0 || status=$?
        statuses+=("$status")
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
        return 0
    else
        echo "⚠️  $label: some deployments may not be ready"
        return 1
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
        return 0
    else
        echo "⚠️  $display_name: some pods may not be ready"
        return 1
    fi
}

# Util: Wait for a secret to exist (created by cert-manager)
# Usage: wait_for_secret "namespace" "secret_name" [timeout_seconds]
wait_for_secret() {
    local namespace="$1"
    local secret_name="$2"
    local timeout="${3:-120}"
    local interval=5
    local elapsed=0

    echo "⏳ Waiting for secret '$secret_name' in namespace '$namespace'..."
    while [ $elapsed -lt $timeout ]; do
        if kubectl get secret "$secret_name" -n "$namespace" &>/dev/null; then
            echo "✅ Secret '$secret_name' is ready"
            return 0
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
        echo "   Waiting... ($elapsed/${timeout}s)"
    done

    echo "❌ Timeout waiting for secret '$secret_name' in namespace '$namespace'"
    return 1
}
