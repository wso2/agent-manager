#!/usr/bin/env bash
# Repoint the first-party image repositories in the shipped charts.
# Usage: rewrite-image-registry.sh <from-prefix> <to-prefix>
#   e.g. rewrite-image-registry.sh ghcr.io/wso2 registry.example.com/my-group
#
# Run from the repository root, after update-helm-charts.sh has stamped the
# version. A build that publishes images somewhere other than the public
# registry has to ship charts whose defaults point there, or the operator has to
# override every image by hand.
#
# The rewrite is driven by an explicit allowlist of image names read from
# .github/release-config.json, never by matching the registry prefix. That
# distinction is the whole safety property here: the public org also hosts a
# sibling product (ghcr.io/wso2/api-platform/*), and the charts additionally
# reference ghcr.io/thunder-id and other third-party images. Those are not ours
# to move, they are not mirrored to the private registry, and a prefix-based
# sed would silently retag all of them into a registry that has never held them.
#
# amp-python-instrumentation-provider is deliberately NOT in scope even though it
# is first-party. It runs as an init container in agent namespaces created at
# deploy time, where no pull secret can be placed ahead of the pod, so it
# stays on the public registry. See the instrumentation catalog guide for the
# supported way to mirror it.
set -euo pipefail

FROM="${1:?usage: rewrite-image-registry.sh <from-prefix> <to-prefix>}"
TO="${2:?usage: rewrite-image-registry.sh <from-prefix> <to-prefix>}"
CONFIG_FILE="${CONFIG_FILE:-.github/release-config.json}"
CHARTS_DIR="${CHARTS_DIR:-deployments/helm-charts}"

[ -f "$CONFIG_FILE" ] || { echo "❌ Not found: $CONFIG_FILE (run from the repository root)" >&2; exit 1; }
[ -d "$CHARTS_DIR" ] || { echo "❌ Not found: $CHARTS_DIR" >&2; exit 1; }

# Trailing slashes would produce a doubled separator in the rewritten value.
FROM="${FROM%/}"
TO="${TO%/}"

IMAGES="$(jq -r '.images[].name' "$CONFIG_FILE")"
[ -n "$IMAGES" ] || { echo "❌ No images in $CONFIG_FILE" >&2; exit 1; }

echo "Rewriting first-party image repositories: ${FROM} -> ${TO}"

TOTAL=0
while IFS= read -r image; do
    [ -n "$image" ] || continue
    count=0
    # values.schema.json carries the same repository as a documented default, and
    # the generated reference pages are built from it, so both move together or
    # the chart and its documentation disagree.
    while IFS= read -r file; do
        # The image name must end the path segment: "amp-api" must not match
        # "amp-api-client", and the anchor keeps the sibling product's nested
        # paths (api-platform/gateway-controller) out of range entirely.
        if grep -qE "${FROM}/${image}([\"':[:space:]]|$)" "$file" 2>/dev/null; then
            perl -pi -e "s{\Q${FROM}/${image}\E(?=[\"':\\s]|\$)}{${TO}/${image}}g" "$file"
            count=$((count + 1))
        fi
    done < <(find "$CHARTS_DIR" -type f \( -name values.yaml -o -name values.schema.json \))
    if [ "$count" -gt 0 ]; then
        echo "  ✅ ${image}: ${count} file(s)"
        TOTAL=$((TOTAL + count))
    else
        echo "  –  ${image}: not referenced by any chart"
    fi
done <<< "$IMAGES"

if [ "$TOTAL" -eq 0 ]; then
    # Silence here means the prefix was wrong, or the charts moved. Either way a
    # release that continues would publish charts still pointing at the public
    # registry, which fails only later as an ImagePullBackOff on the customer's
    # cluster. Fail now instead.
    echo "❌ Nothing was rewritten. Is '${FROM}' the current prefix?" >&2
    exit 1
fi

echo "Rewrote ${TOTAL} file reference(s)."

# The images left on the public registry are load-bearing, so say so rather than
# leaving the reader to diff the charts.
echo "Deliberately unchanged:"
grep -rhoE '(ghcr\.io|docker\.io|quay\.io|registry-1\.docker\.io)/[a-z0-9._/-]+' \
    "$CHARTS_DIR"/*/values.yaml 2>/dev/null | sort -u | sed 's/^/  /'
