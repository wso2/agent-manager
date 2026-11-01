#!/bin/bash
set -uo pipefail

# Re-renders every API Platform gateway extension release against one chart and
# waits until each gateway runs the images that chart names.
#
#   upgrade-gateways.sh [chart-ref]    # default: the local extension chart
#
# --reset-then-reuse-values keeps each release's own overrides (env Thunder
# wiring, vhost, namespace) while taking the new chart's defaults, which is
# where the gateway image is pinned. Plain --reuse-values would keep the
# image the release was first installed with.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_NAME="wso2-amp-api-platform-gateway-extension"
CHART_REF="${1:-${SCRIPT_DIR}/../helm-charts/${CHART_NAME}}"

# shellcheck source=./gateway-rollout.sh
source "${SCRIPT_DIR}/gateway-rollout.sh"

if ! RELEASES="$(helm list -A -o json | jq -r --arg prefix "${CHART_NAME}-" \
        '.[] | select(.chart | startswith($prefix)) | "\(.name) \(.namespace)"')"; then
    echo "❌ Could not list Helm releases" >&2
    exit 1
fi

if [ -z "${RELEASES}" ]; then
    echo "ℹ️  No ${CHART_NAME} releases to upgrade"
    exit 0
fi

SUMMARY=()
FAILED=0
while read -r release namespace <&3; do
    echo ""
    echo "=== Upgrading gateway release ${namespace}/${release} ==="
    if ! helm upgrade "${release}" "${CHART_REF}" -n "${namespace}" --reset-then-reuse-values; then
        SUMMARY+=("❌ ${namespace}/${release}: helm upgrade failed")
        FAILED=1
    elif ! wait_gateway_rollout "${release}" "${namespace}"; then
        SUMMARY+=("❌ ${namespace}/${release}: gateway did not roll out")
        FAILED=1
    else
        SUMMARY+=("✅ ${namespace}/${release}: upgraded and rolled out")
    fi
done 3<<< "${RELEASES}"

echo ""
echo "=== Gateway upgrade summary ==="
printf '%s\n' "${SUMMARY[@]}"
exit "${FAILED}"
