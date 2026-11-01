# shellcheck shell=bash
# Waits for an API Platform gateway to run the images its Helm release names.
# Source this file; it only defines functions. Needs helm, kubectl, jq and awk.
#
# The gateway-operator redeploys a gateway asynchronously when its configRef
# ConfigMap changes: it first flips the APIGateway's Programmed condition to
# False, then redeploys. Programmed=True read right after `helm upgrade` can
# therefore still describe the previous rollout. The Deployments running the
# images the ConfigMap names is the only signal that cannot be stale.

# Prints "<name> <namespace>" of the APIGateway that a Helm release renders.
# The chart names it gateway.name when set, so it need not match the release,
# and places it in apiGateway.namespace, which need not be the release's.
_gateway_rollout_apigateway() {
    local release="$1" namespace="$2"
    helm get manifest "$release" -n "$namespace" | awk -v release_ns="$namespace" '
        function unquote(s) { gsub(/^"|"$/, "", s); return s }
        function emit() {
            if (kind != "APIGateway" || name == "") return
            print name, (ns != "" ? ns : release_ns)
            found = 1
            exit
        }
        /^---/ { emit(); kind = ""; name = ""; ns = ""; in_metadata = 0; next }
        /^metadata:/ { in_metadata = 1; next }
        /^[^ #]/ { in_metadata = 0 }
        /^kind:/ { kind = $2 }
        in_metadata && /^  name:/ { name = unquote($2) }
        in_metadata && /^  namespace:/ { ns = unquote($2) }
        END { if (!found) emit() }
    '
}

# Prints the scalar at a dotted path (e.g. gateway.controller.image.tag) of the
# block-style YAML on stdin. Sufficient for the chart-rendered values.yaml: block
# scalars and list items are skipped, never descended into.
_gateway_rollout_yaml_value() {
    awk -v path="$1" '
        BEGIN { want_depth = split(path, want, "."); block_indent = -1 }
        /^[[:space:]]*(#.*)?$/ { next }
        {
            match($0, /^ */); indent = RLENGTH
            if (block_indent >= 0 && indent > block_indent) next
            block_indent = -1

            line = substr($0, indent + 1)
            if (line !~ /^[A-Za-z0-9_.-]+:([[:space:]]|$)/) next
            key = line; sub(/:.*/, "", key)
            value = line; sub(/^[^:]*:[[:space:]]*/, "", value)

            while (depth > 0 && indents[depth] >= indent) depth--
            depth++; keys[depth] = key; indents[depth] = indent

            if (value ~ /^[|>]/) { block_indent = indent; next }
            if (depth != want_depth || value == "") next
            for (i = 1; i <= depth; i++) if (keys[i] != want[i]) next

            if (value ~ /^"/) { value = substr(value, 2); sub(/".*/, "", value) }
            else sub(/[[:space:]]+#.*/, "", value)
            print value
            exit
        }
    '
}

# Prints one "<deployment> <role> <image>" line per container in <namespace>
# whose image comes from the controller or runtime repository. Matching is by
# repository, not by Deployment name: the operator owns the names.
_gateway_rollout_containers() {
    local namespace="$1" controller_repo="$2" runtime_repo="$3"
    kubectl get deployments -n "$namespace" -o json | jq -r \
        --arg controller "$controller_repo" --arg runtime "$runtime_repo" '
        def from($repo): . == $repo or startswith($repo + ":") or startswith($repo + "@");
        .items[] | .metadata.name as $deployment
        | .spec.template.spec.containers[]
        | if (.image | from($controller)) then "\($deployment) controller \(.image)"
          elif (.image | from($runtime)) then "\($deployment) runtime \(.image)"
          else empty end'
}

# Succeeds when the containers listing has both roles and every image is the
# expected one for its role.
_gateway_rollout_images_current() {
    local containers="$1" expected_controller="$2" expected_runtime="$3"
    awk -v controller="$expected_controller" -v runtime="$expected_runtime" '
        { seen[$2] = 1 }
        $2 == "controller" && $3 != controller { stale = 1 }
        $2 == "runtime" && $3 != runtime { stale = 1 }
        END { exit !(seen["controller"] && seen["runtime"] && !stale) }
    ' <<< "$containers"
}

# wait_gateway_rollout <release> <namespace> [timeout_seconds]
#
# Returns 0 once every Deployment of the release's gateway runs the controller
# and runtime images its configRef ConfigMap names and `kubectl rollout status`
# succeeds for each. Otherwise returns 1, printing the expected and actual
# images. <namespace> is the Helm release's namespace.
wait_gateway_rollout() {
    local release="$1" namespace="$2" timeout="${3:-300}"
    local poll="${GATEWAY_ROLLOUT_POLL_SECONDS:-5}"
    local deadline=$((SECONDS + timeout))
    local gateway="" gateway_ns="" configmap values
    local controller_repo controller_tag runtime_repo runtime_tag

    read -r gateway gateway_ns < <(_gateway_rollout_apigateway "$release" "$namespace")
    if [ -z "$gateway" ]; then
        echo "❌ Helm release '${release}' in '${namespace}' renders no APIGateway" >&2
        return 1
    fi

    configmap="$(kubectl get apigateway "$gateway" -n "$gateway_ns" -o json \
        | jq -r '.spec.configRef.name // empty')" || configmap=""
    if [ -z "$configmap" ]; then
        echo "❌ APIGateway ${gateway_ns}/${gateway} has no readable spec.configRef.name" >&2
        return 1
    fi

    values="$(kubectl get configmap "$configmap" -n "$gateway_ns" -o json \
        | jq -r '.data["values.yaml"] // empty')" || values=""
    controller_repo="$(_gateway_rollout_yaml_value gateway.controller.image.repository <<< "$values")"
    controller_tag="$(_gateway_rollout_yaml_value gateway.controller.image.tag <<< "$values")"
    runtime_repo="$(_gateway_rollout_yaml_value gateway.gatewayRuntime.image.repository <<< "$values")"
    runtime_tag="$(_gateway_rollout_yaml_value gateway.gatewayRuntime.image.tag <<< "$values")"
    if [ -z "$controller_repo" ] || [ -z "$controller_tag" ] || [ -z "$runtime_repo" ] || [ -z "$runtime_tag" ]; then
        echo "❌ ConfigMap ${gateway_ns}/${configmap} does not name gateway.controller.image and gateway.gatewayRuntime.image in values.yaml" >&2
        return 1
    fi
    local expected_controller="${controller_repo}:${controller_tag}"
    local expected_runtime="${runtime_repo}:${runtime_tag}"

    echo "⏳ Waiting for gateway ${gateway_ns}/${gateway} to run ${expected_controller} and ${expected_runtime}..."
    local containers
    while :; do
        containers="$(_gateway_rollout_containers "$gateway_ns" "$controller_repo" "$runtime_repo")" || containers=""
        if _gateway_rollout_images_current "$containers" "$expected_controller" "$expected_runtime"; then
            break
        fi
        if [ "$SECONDS" -ge "$deadline" ]; then
            {
                echo "❌ Gateway ${gateway_ns}/${gateway} does not run the images its ConfigMap ${configmap} names after ${timeout}s"
                echo "   expected controller: ${expected_controller}"
                echo "   expected runtime:    ${expected_runtime}"
                if [ -n "$containers" ]; then
                    echo "   actual:"
                    awk '{ printf "     deployment/%s (%s): %s\n", $1, $2, $3 }' <<< "$containers"
                else
                    echo "   actual: no Deployment in ${gateway_ns} runs either image"
                fi
            } >&2
            return 1
        fi
        sleep "$poll"
    done

    local deployment remaining
    while read -r deployment <&3; do
        remaining=$((deadline - SECONDS))
        [ "$remaining" -ge 1 ] || remaining=1
        if ! kubectl rollout status "deployment/${deployment}" -n "$gateway_ns" --timeout="${remaining}s"; then
            echo "❌ deployment/${deployment} in ${gateway_ns} did not finish rolling out" >&2
            return 1
        fi
    done 3< <(awk '{ print $1 }' <<< "$containers" | sort -u)
    echo "✅ Gateway ${gateway_ns}/${gateway} runs ${expected_controller} and ${expected_runtime}"
}
