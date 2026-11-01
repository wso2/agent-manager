#!/usr/bin/env bash
# Unit tests for deployments/setup/gateway-rollout.sh.
# Run: bash deployments/setup/tests/gateway-rollout-test.sh
#
# helm and kubectl are stubbed on PATH and serve canned JSON, so this needs only
# bash, awk and jq. The release is deliberately named differently from its
# APIGateway (as `make setup-ai-gateway` does with gateway.name=default): the
# library must find the gateway through the release's manifest.
set -uo pipefail

LIB="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/gateway-rollout.sh"
FAILURES=0
STUB="$(mktemp -d)"
trap 'rm -rf "$STUB"' EXIT

RELEASE="amp-ai-gateway"
RELEASE_NS="openchoreo-data-plane"
GATEWAY="default"
GATEWAY_NS="default-default"
CONTROLLER_REPO="ghcr.io/wso2/api-platform/gateway-controller"
RUNTIME_REPO="ghcr.io/wso2/api-platform/gateway-runtime"
NEW_CONTROLLER="${CONTROLLER_REPO}:main-40eddf7@sha256:65692a5935c8bb2072944a6c0aab304d8aaa2288dd474926cbdd7ab7597e93d0"
NEW_RUNTIME="${RUNTIME_REPO}:main-40eddf7@sha256:56da03028624194632e7c2ca480fc57bbd9ff3b6dee8f16a06ef586d13fcb947"
OLD_RUNTIME="${RUNTIME_REPO}:2026.09.24"
OTHER_RUNTIME="${RUNTIME_REPO}:main-0000000"

pass() { printf 'ok   - %s\n' "$1"; }
fail() { printf 'FAIL - %s\n' "$1"; FAILURES=$((FAILURES + 1)); }

assert_rc() {
    local label="$1" expected="$2" actual="$3"
    if [[ "$expected" == "$actual" ]]; then pass "$label"; else
        fail "$label"; printf '      expected rc %s, got %s\n' "$expected" "$actual"; fi
}

assert_contains() {
    local label="$1" needle="$2" haystack="$3"
    if [[ "$haystack" == *"$needle"* ]]; then pass "$label"; else
        fail "$label"; printf '      %q not found in:\n%s\n' "$needle" "$haystack"; fi
}

assert_not_contains() {
    local label="$1" needle="$2" haystack="$3"
    if [[ "$haystack" != *"$needle"* ]]; then pass "$label"; else
        fail "$label"; printf '      %q unexpectedly found in:\n%s\n' "$needle" "$haystack"; fi
}

# --- canned cluster state -----------------------------------------------------

cat > "$STUB/manifest.yaml" <<EOF
---
# Source: wso2-amp-api-platform-gateway-extension/templates/gateway-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${RELEASE}-config
  namespace: ${GATEWAY_NS}
---
# Source: wso2-amp-api-platform-gateway-extension/templates/gateway-cr.yaml
apiVersion: gateway.api-platform.wso2.com/v1
kind: APIGateway
metadata:
  name: ${GATEWAY}
  namespace: ${GATEWAY_NS}
  labels:
    app.kubernetes.io/name: not-the-gateway-name
spec:
  configRef:
    name: ${RELEASE}-config
---
apiVersion: gateway.api-platform.wso2.com/v1
kind: RestApi
metadata:
  name: ${GATEWAY}-otel-restapi
  namespace: ${GATEWAY_NS}
EOF

# Mirrors the shape of templates/gateway-config.yaml's values.yaml, including
# the decoys a naive grep would trip on: a block scalar and another image key.
cat > "$STUB/values.yaml" <<EOF
gateway:
  developmentMode: true
  config_toml: |
    [controller]
    image: "not-this-one"
  config:
    policy_configurations:
      jwtauth_v1:
        keymanagers:
        - name: ThunderKeyManager
          image:
            repository: "nope"
  controller:
    image:
      repository: "${CONTROLLER_REPO}"
      tag: "${NEW_CONTROLLER#"${CONTROLLER_REPO}":}"
    controlPlane:
      host: "host.docker.internal:9243"

  gatewayRuntime:
    image:
      repository: "${RUNTIME_REPO}"
      tag: "${NEW_RUNTIME#"${RUNTIME_REPO}":}"
    deployment:
      podLabels:
        openchoreo.dev/system-component: gateway-runtime
EOF

jq -n --rawfile v "$STUB/values.yaml" \
    '{apiVersion: "v1", kind: "ConfigMap", metadata: {name: "'"${RELEASE}"'-config"}, data: {"values.yaml": $v}}' \
    > "$STUB/configmap.json"

# The APIGateway reports Programmed=True throughout: the library must not trust it.
cat > "$STUB/apigateway.json" <<EOF
{"apiVersion": "gateway.api-platform.wso2.com/v1", "kind": "APIGateway",
 "metadata": {"name": "${GATEWAY}", "namespace": "${GATEWAY_NS}"},
 "spec": {"configRef": {"name": "${RELEASE}-config"}},
 "status": {"conditions": [{"type": "Programmed", "status": "True", "reason": "Programmed"}]}}
EOF

write_deployments() {
    local controller_image="$1" runtime_image="$2"
    jq -n --arg c "$controller_image" --arg r "$runtime_image" '{items: [
        {metadata: {name: "default-gw-gateway-controller"},
         spec: {template: {spec: {containers: [{name: "controller", image: $c}]}}}},
        {metadata: {name: "default-gw-gateway-gateway-runtime"},
         spec: {template: {spec: {containers: [{name: "runtime", image: $r},
                                               {name: "sidecar", image: "busybox:1.36"}]}}}},
        {metadata: {name: "unrelated"},
         spec: {template: {spec: {containers: [{name: "web", image: "nginx:1.27"}]}}}}
    ]}' > "$STUB/deployments.json"
}

mkdir -p "$STUB/bin"
cat > "$STUB/bin/helm" <<EOF
#!/usr/bin/env bash
echo "helm \$*" >> "$STUB/calls.log"
if [[ "\$*" == "get manifest ${RELEASE} -n ${RELEASE_NS}" ]]; then
    cat "$STUB/manifest.yaml"; exit 0
fi
echo "helm stub: unexpected call: \$*" >&2; exit 1
EOF

cat > "$STUB/bin/kubectl" <<EOF
#!/usr/bin/env bash
echo "kubectl \$*" >> "$STUB/calls.log"
case "\$*" in
    "get apigateway ${GATEWAY} -n ${GATEWAY_NS} -o json") cat "$STUB/apigateway.json" ;;
    "get configmap ${RELEASE}-config -n ${GATEWAY_NS} -o json") cat "$STUB/configmap.json" ;;
    "get deployments -n ${GATEWAY_NS} -o json") cat "$STUB/deployments.json" ;;
    "rollout status deployment/"*" -n ${GATEWAY_NS} --timeout="*) exit "\${ROLLOUT_RC:-0}" ;;
    *) echo "kubectl stub: unexpected call: \$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$STUB/bin/helm" "$STUB/bin/kubectl"

# Each case runs in a subshell so the stubbed PATH and env cannot leak.
run_wait() {
    : > "$STUB/calls.log"
    ( export PATH="$STUB/bin:$PATH" GATEWAY_ROLLOUT_POLL_SECONDS=1
      # shellcheck source-path=SCRIPTDIR source=../gateway-rollout.sh
      . "$LIB"
      wait_gateway_rollout "$RELEASE" "$RELEASE_NS" "$@" ) 2>&1
}

# --- cases --------------------------------------------------------------------

# Programmed=True, ConfigMap names the new digest, a Deployment still runs the old tag.
write_deployments "$NEW_CONTROLLER" "$OLD_RUNTIME"
out="$(run_wait 2)"; rc=$?
assert_rc "stale runtime under Programmed=True is not ready" 1 "$rc"
assert_not_contains "no rollout status is awaited while images are stale" "rollout status" "$(cat "$STUB/calls.log")"

# Both images match and the rollout completes.
write_deployments "$NEW_CONTROLLER" "$NEW_RUNTIME"
out="$(ROLLOUT_RC=0 run_wait 5)"; rc=$?
assert_rc "matching images and a finished rollout succeed" 0 "$rc"
calls="$(cat "$STUB/calls.log")"
assert_contains "the APIGateway is found through the release manifest" "helm get manifest ${RELEASE} -n ${RELEASE_NS}" "$calls"
assert_contains "the controller Deployment's rollout is awaited" "rollout status deployment/default-gw-gateway-controller" "$calls"
assert_contains "the runtime Deployment's rollout is awaited" "rollout status deployment/default-gw-gateway-gateway-runtime" "$calls"
assert_not_contains "a Deployment running neither image is ignored" "deployment/unrelated" "$calls"

# Images match but the rollout itself fails.
out="$(ROLLOUT_RC=1 run_wait 5)"; rc=$?
assert_rc "a failed rollout status fails the wait" 1 "$rc"

# Controller matches, runtime does not: both images are reported.
write_deployments "$NEW_CONTROLLER" "$OTHER_RUNTIME"
out="$(run_wait 2)"; rc=$?
assert_rc "a mismatched runtime image fails the wait" 1 "$rc"
assert_contains "the expected runtime image is printed" "$NEW_RUNTIME" "$out"
assert_contains "the actual runtime image is printed" "$OTHER_RUNTIME" "$out"

# No gateway Deployment exists yet (the operator has not created them).
jq -n '{items: []}' > "$STUB/deployments.json"
out="$(run_wait 2)"; rc=$?
assert_rc "no gateway Deployments yet is not ready" 1 "$rc"

echo
if [[ "$FAILURES" -gt 0 ]]; then
    echo "$FAILURES failure(s)"
    exit 1
fi
echo "all passed"
