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

# A release stuck in pending-* cannot be upgraded until someone rolls it back,
# so it is listed in order to be reported as a failure rather than skipped
# silently. The status flags are explicit because Helm 3 and Helm 4 disagree
# on what `helm list` shows by default.
if ! RELEASES="$(helm list -A --deployed --failed --pending -o json | jq -r --arg prefix "${CHART_NAME}-" \
        '.[] | select(.chart | startswith($prefix)) | "\(.name) \(.namespace) \(.status)"')"; then
    echo "❌ Could not list Helm releases" >&2
    exit 1
fi

if [ -z "${RELEASES}" ]; then
    echo "ℹ️  No ${CHART_NAME} releases to upgrade"
    exit 0
fi

SUMMARY=()
FAILED=0
while read -r release namespace status <&3; do
    echo ""
    echo "=== Upgrading gateway release ${namespace}/${release} ==="
    if [[ "${status}" == pending-* ]]; then
        echo "❌ Release is ${status}; resolve it (helm rollback or uninstall) and re-run" >&2
        SUMMARY+=("❌ ${namespace}/${release}: not upgraded, release is ${status}")
        FAILED=1
    elif ! helm upgrade "${release}" "${CHART_REF}" -n "${namespace}" --reset-then-reuse-values; then
        SUMMARY+=("❌ ${namespace}/${release}: helm upgrade failed")
        FAILED=1
    elif ! wait_gateway_rollout "${release}" "${namespace}" "${GATEWAY_ROLLOUT_TIMEOUT:-600}"; then
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
