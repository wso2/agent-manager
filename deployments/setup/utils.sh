# shellcheck shell=bash
# Util: Check if a port is in use
#
# On Linux a published container port is held by a root-owned docker-proxy,
# which lsof hides from an unprivileged caller — the port reads as free and the
# cluster only fails much later, at bind time. ss reads /proc/net/tcp and sees
# every listener regardless of owner. macOS ships no ss, but there the listener
# belongs to the invoking user, so lsof is accurate.
is_port_in_use() {
    local port="$1"
    if command -v ss &> /dev/null; then
        [ -n "$(ss -ltnH "sport = :$port" 2>/dev/null)" ]
        return
    fi
    lsof -i :"$port" -sTCP:LISTEN &> /dev/null
}

# Util: Name the Docker containers publishing a port, so a conflict report says
# what to stop instead of only which port is taken.
containers_publishing_port() {
    docker ps --filter "publish=$1" --format '{{.Names}}' 2>/dev/null \
        | tr '\n' ' ' \
        | sed 's/ *$//'
}

# Util: The command that shows which process owns a listening port, per platform.
port_owner_command() {
    if command -v ss &> /dev/null; then
        echo "ss -ltnp 'sport = :<port>'"
    else
        echo "lsof -i :<port>"
    fi
}

# Util: Check all required ports for k3d cluster are available
check_required_ports() {
    local ports=(
        "6550:Kubernetes API"
        "8080:Control Plane HTTP"
        "8443:Control Plane HTTPS"
        "19080:Data Plane HTTP"
        "19443:Data Plane HTTPS"
        "10081:Argo Workflows UI"
        "10082:Container Registry"
        "11080:Observability HTTP"
        "11085:OpenSearch HTTPS"
        "11081:OpenSearch Dashboard"
        "11082:OpenSearch API"
    )

    local blocked_ports=()
    echo "🔍 Checking port availability..."

    for port_info in "${ports[@]}"; do
        local port="${port_info%%:*}"
        local desc="${port_info#*:}"
        if is_port_in_use "$port"; then
            local holders
            holders=$(containers_publishing_port "$port")
            if [ -n "$holders" ]; then
                blocked_ports+=("$port ($desc) — held by container(s): $holders")
            else
                blocked_ports+=("$port ($desc)")
            fi
        fi
    done

    if [ ${#blocked_ports[@]} -gt 0 ]; then
        echo "❌ The following ports are already in use:"
        for blocked in "${blocked_ports[@]}"; do
            echo "   - $blocked"
        done
        echo ""
        echo "Please free these ports before creating the cluster."
        echo "You can find processes using a port with: $(port_owner_command)"
        return 1
    fi

    echo "✅ All required ports are available"
    return 0
}

# Util: Get minimum required version for a command (bash 3.x compatible)
get_min_version() {
    case "$1" in
        docker)  echo "26" ;;
        k3d)     echo "5.8" ;;
        kubectl) echo "1.32" ;;
        helm)    echo "3.12" ;;
        go)      echo "1.25" ;;
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

# Util: Suggest how to install a missing command on the current platform.
install_hint() {
    if [ "$(uname -s)" = "Darwin" ]; then
        echo "brew install $1"
    else
        echo "install '$1' with your distribution's package manager"
    fi
}

# Util: Check if a command is installed and version is compatible
check_command() {
    local cmd="$1"
    if ! command -v "$cmd" &> /dev/null; then
        echo "❌ $cmd is not installed. Please install it first:"
        echo "   $(install_hint "$cmd")"
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

# Util: Make host.k3d.internal / host.docker.internal resolve inside pods
#
# k3d injects these aliases only into the nodes' /etc/hosts, not into pod DNS,
# so in-cluster clients (gateway controller, observability, helm bootstrap
# Jobs) cannot reach the host through them. We add both to the CoreDNS
# NodeHosts file, which the k3s node controller preserves across restarts.
#
# This also closes a setup race: NodeHosts is NOT shipped in the coredns Addon
# manifest — k3s populates it asynchronously after a server (re)start. The
# coredns Deployment mounts NodeHosts as a *non-optional* configmap key, so
# until k3s writes it a freshly restarted CoreDNS pod cannot mount and the
# rollout times out. Writing the key here guarantees it exists before the
# subsequent `kubectl rollout restart deployment/coredns`.
ensure_coredns_host_aliases() {
    echo "🔧 Ensuring host.k3d.internal / host.docker.internal resolve in-cluster..."

    local host_ip
    host_ip=$(docker network inspect "k3d-${CLUSTER_NAME}" \
        --format '{{ (index .IPAM.Config 0).Gateway }}' 2>/dev/null)
    if [[ -z "$host_ip" ]]; then
        echo "❌ Could not determine k3d host gateway IP for network k3d-${CLUSTER_NAME}"
        return 1
    fi

    # Node entries already written by k3s's node controller. Empty during the
    # post-restart race window — that is fine: k3s re-adds them later and keeps
    # the alias lines we append.
    local existing
    existing=$(kubectl get configmap coredns -n kube-system \
        --context "${CLUSTER_CONTEXT}" -o jsonpath='{.data.NodeHosts}' 2>/dev/null)

    # Drop any alias lines from a previous run so re-runs stay idempotent.
    local node_lines
    node_lines=$(printf '%s\n' "$existing" \
        | grep -vE '[[:space:]](host\.k3d\.internal|host\.docker\.internal)$' \
        | sed '/^[[:space:]]*$/d' || true)

    local desired
    desired=$(printf '%s\n%s host.k3d.internal\n%s host.docker.internal\n' \
        "$node_lines" "$host_ip" "$host_ip" \
        | sed '/^[[:space:]]*$/d')

    # Escape each line into a literal \n for the JSON merge patch (awk keeps
    # this portable across BSD/macOS and GNU sed). out ends with a trailing \n
    # to match the file format coredns writes.
    local patch_value
    patch_value=$(printf '%s\n' "$desired" | awk '{ out = out $0 "\\n" } END { printf "%s", out }')

    if ! kubectl patch configmap coredns -n kube-system --context "${CLUSTER_CONTEXT}" \
        --type merge -p "{\"data\":{\"NodeHosts\":\"${patch_value}\"}}"; then
        echo "❌ Failed to patch CoreDNS NodeHosts"
        return 1
    fi
    echo "✅ CoreDNS NodeHosts updated (host.k3d.internal, host.docker.internal -> ${host_ip})"
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
          host: "agentmanager.localhost"
          listenerName: http
          port: 19080
        https:
          host: "agentmanager.localhost"
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

# Fork-join over named shell commands. start_task forks one with its output
# captured; collect_tasks waits for every task forked so far, prints each one's
# output and fails if any failed. run_parallel_tasks is the common case of
# forking several and collecting at once. Forking one, doing other work in the
# foreground, then collecting is the same two calls spread apart; that
# foreground work may itself call run_parallel_tasks, which collects only the
# tasks it forked and leaves the earlier ones outstanding.
#
# While tasks are outstanding the script's EXIT trap belongs to abandon_tasks,
# so a script that fails before collecting stops the tasks (and everything they
# forked) instead of leaving them running, and removes their log files.
TASK_NAMES=()
TASK_PIDS=()
TASK_LOGS=()

start_task() {
    local name="$1"
    local command="$2"
    local log_file
    log_file=$(mktemp)

    # The child starts with an empty task list so its own fork-joins never try
    # to wait on the parent's tasks.
    (TASK_NAMES=(); TASK_PIDS=(); TASK_LOGS=(); eval "$command") > "$log_file" 2>&1 &
    TASK_NAMES+=("$name")
    TASK_PIDS+=("$!")
    TASK_LOGS+=("$log_file")
    trap abandon_tasks EXIT
    echo "   Started: $name (PID: $!)"
}

kill_process_tree() {
    local pid="$1"
    local child
    for child in $(pgrep -P "$pid" 2>/dev/null); do
        kill_process_tree "$child"
    done
    kill "$pid" 2>/dev/null || true
}

abandon_tasks() {
    local i
    for i in "${!TASK_PIDS[@]}"; do
        echo "⚠️  Stopping unfinished background task: ${TASK_NAMES[$i]} (PID: ${TASK_PIDS[$i]})" >&2
        kill_process_tree "${TASK_PIDS[$i]}"
        wait "${TASK_PIDS[$i]}" 2>/dev/null || true
        rm -f "${TASK_LOGS[$i]}"
    done
    TASK_NAMES=()
    TASK_PIDS=()
    TASK_LOGS=()
}

# Usage: collect_tasks [first]
# Collects the tasks forked since index `first` (every task by default).
collect_tasks() {
    local first="${1:-0}"
    local failed=0
    local i status
    for ((i = first; i < ${#TASK_PIDS[@]}; i++)); do
        wait "${TASK_PIDS[$i]}" && status=0 || status=$?
        echo ""
        echo "========== ${TASK_NAMES[$i]} =========="
        cat "${TASK_LOGS[$i]}"
        echo ""
        rm -f "${TASK_LOGS[$i]}"
        if [ "$status" -ne 0 ]; then
            echo "❌ ${TASK_NAMES[$i]} failed with exit code: $status"
            failed=1
        fi
    done
    TASK_NAMES=("${TASK_NAMES[@]:0:$first}")
    TASK_PIDS=("${TASK_PIDS[@]:0:$first}")
    TASK_LOGS=("${TASK_LOGS[@]:0:$first}")
    if [ "${#TASK_PIDS[@]}" -eq 0 ]; then
        trap - EXIT
    fi
    return $failed
}

# Usage: run_parallel_tasks "task1_name:task1_cmd" "task2_name:task2_cmd" ...
run_parallel_tasks() {
    local first="${#TASK_PIDS[@]}"
    local task
    for task in "$@"; do
        start_task "${task%%:*}" "${task#*:}"
    done
    collect_tasks "$first"
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

# Pin the serverlb's container IP so it cannot drift onto the server node's address.
pin_serverlb_ip() {
    local network="k3d-${CLUSTER_NAME}"
    local lb="k3d-${CLUSTER_NAME}-serverlb"
    local current pinned

    if ! docker container inspect "$lb" &>/dev/null; then
        return 0    # cluster created with --no-lb
    fi

    pinned="$(docker container inspect "$lb" \
        --format "{{with (index .NetworkSettings.Networks \"${network}\")}}{{with .IPAMConfig}}{{.IPv4Address}}{{end}}{{end}}" 2>/dev/null)"
    if [ -n "$pinned" ]; then
        return 0    # already static
    fi

    current="$(docker container inspect "$lb" \
        --format "{{with (index .NetworkSettings.Networks \"${network}\")}}{{.IPAddress}}{{end}}" 2>/dev/null)"
    if [ -z "$current" ]; then
        echo "⚠️  Could not read the loadbalancer address; leaving it dynamic"
        return 0
    fi

    docker stop "$lb" >/dev/null 2>&1 || true
    docker network disconnect "$network" "$lb" >/dev/null 2>&1 || true
    if ! docker network connect --ip "$current" "$network" "$lb" >/dev/null 2>&1; then
        # Pinning is hardening, not correctness: an unpinned but running cluster is
        # fine, so restore the dynamic attachment rather than failing the install.
        docker network connect "$network" "$lb" >/dev/null 2>&1 || true
        if ! docker start "$lb" >/dev/null 2>&1; then
            echo "❌ Could not restart ${lb}; the cluster API will be unreachable"
            return 1
        fi
        echo "⚠️  Could not pin the loadbalancer to ${current}; left it dynamic"
        return 0
    fi
    # This function stopped a running container. Failing to bring it back leaves the
    # cluster API unreachable, so report that instead of claiming success.
    if ! docker start "$lb" >/dev/null 2>&1; then
        echo "❌ Could not restart ${lb} after pinning to ${current}; the cluster API will be unreachable"
        return 1
    fi
    echo "✅ Loadbalancer pinned to ${current}"
}

# Pods that exit while their backend is still starting (the observability
# adapters do this until OpenSearch or Prometheus is up) land in
# CrashLoopBackOff, where Kubernetes waits up to 5 minutes before retrying.
# Deleting such a pod restarts it immediately. The delete is forced because a
# crash-looped container has nothing to shut down, yet a graceful delete still
# holds the pod for ~20s before the replacement is created.
restart_crashlooping_pods() {
    local ns="$1"
    local selector="$2"
    kubectl get pods -n "$ns" -l "$selector" \
        -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.containerStatuses[*].state.waiting.reason}{"\n"}{end}' 2>/dev/null \
        | awk -F'\t' '$2 ~ /CrashLoopBackOff/ {print $1}' \
        | while read -r pod; do
            echo "   ↻ Restarting $pod (CrashLoopBackOff)"
            kubectl delete pod "$pod" -n "$ns" --force --grace-period=0 --wait=false 2>/dev/null \
                || echo "❌ Failed to delete $pod in $ns; it stays in CrashLoopBackOff until the kubelet retries" >&2
        done || true
}

# Waits for every deployment in the namespace, restarting only the pods matching
# the selector out of CrashLoopBackOff on the way. Anything else that crash-loops
# keeps its restart history and surfaces in the timeout report.
wait_for_deployments_restarting_crashloops() {
    local ns="$1"
    local crashloop_selector="$2"
    local timeout_seconds="$3"
    local deadline=$(( $(date +%s) + timeout_seconds ))

    while true; do
        restart_crashlooping_pods "$ns" "$crashloop_selector"
        kubectl wait -n "$ns" --for=condition=available --timeout=10s deployment --all &>/dev/null && return 0
        if [ "$(date +%s)" -ge "$deadline" ]; then
            echo "❌ Deployments in $ns not available after ${timeout_seconds}s:"
            kubectl get pods -n "$ns"
            return 1
        fi
    done
}
