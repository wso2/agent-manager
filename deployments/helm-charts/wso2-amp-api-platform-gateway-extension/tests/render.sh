#!/usr/bin/env bash
set -uo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

CONTROLLER_REPOSITORY="ghcr.io/wso2/api-platform/gateway-controller"
CONTROLLER_TAG="main-40eddf7@sha256:65692a5935c8bb2072944a6c0aab304d8aaa2288dd474926cbdd7ab7597e93d0"
RUNTIME_REPOSITORY="ghcr.io/wso2/api-platform/gateway-runtime"
RUNTIME_TAG="main-40eddf7@sha256:56da03028624194632e7c2ca480fc57bbd9ff3b6dee8f16a06ef586d13fcb947"

failures=0

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

# Prints "<controller repo> <controller tag> <runtime repo> <runtime tag>" as
# the gateway chart will read them from the configRef ConfigMap's values.yaml.
gateway_images() {
    python3 - "$1" <<'PY'
import sys
import yaml

with open(sys.argv[1], encoding="utf-8") as stream:
    documents = list(yaml.safe_load_all(stream))

config = next(
    doc for doc in documents
    if isinstance(doc, dict)
    and doc.get("kind") == "ConfigMap"
    and doc.get("metadata", {}).get("labels", {}).get("app.kubernetes.io/component") == "gateway-config"
)
gateway = yaml.safe_load(config["data"]["values.yaml"])["gateway"]
controller = gateway["controller"]["image"]
runtime = gateway["gatewayRuntime"]["image"]
print(controller["repository"], controller["tag"], runtime["repository"], runtime["tag"])
PY
}

render() {
    local out="$1"
    shift
    helm template test "$CHART_DIR" "$@" >"$out" 2>"$out.err"
}

if render "$workdir/defaults.yaml" && images="$(gateway_images "$workdir/defaults.yaml")" \
    && [ "$images" = "$CONTROLLER_REPOSITORY $CONTROLLER_TAG $RUNTIME_REPOSITORY $RUNTIME_TAG" ]; then
    pass gateway_image_defaults
else
    fail gateway_image_defaults
fi

if render "$workdir/mirror.yaml" --set apiGateway.image.runtime.repository=mirror.local/gw-runtime \
    && images="$(gateway_images "$workdir/mirror.yaml")" \
    && [ "$images" = "$CONTROLLER_REPOSITORY $CONTROLLER_TAG mirror.local/gw-runtime $RUNTIME_TAG" ]; then
    pass gateway_image_mirror_override
else
    fail gateway_image_mirror_override
fi

# An empty tag would let the gateway chart fall back to its own default image,
# silently undoing the pin, so the chart must refuse to render. The same key set
# non-empty must render, or the refusal proves nothing about the tag.
if render "$workdir/set-tag.yaml" --set apiGateway.image.runtime.tag=v1 \
    && ! render "$workdir/empty-tag.yaml" --set apiGateway.image.runtime.tag=; then
    pass gateway_image_empty_tag_refused
else
    fail gateway_image_empty_tag_refused
fi

if [ "$failures" -gt 0 ]; then
    echo "$failures gateway extension render check(s) failed"
    exit 1
fi
echo "Gateway extension render checks passed"
