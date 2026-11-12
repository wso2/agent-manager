#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/env.sh"

# Keep the developer stack aligned with the Thunder release installed by setup.
# Export either value explicitly to use a different set of client credentials.
if [[ -z "${IDP_CLIENT_SECRET:-}" || -z "${THUNDER_CLIENT_SECRET:-}" ]]; then
    command -v jq >/dev/null || { echo "Install jq or export both Thunder client credentials." >&2; exit 1; }
    bootstrap=$(helm get values amp-thunder-extension --namespace amp-thunder \
        --kube-context "$CLUSTER_CONTEXT" --all --output json |
        jq -ce '.thunder.bootstrap')
    if [[ -z "${IDP_CLIENT_SECRET:-}" ]]; then
        IDP_CLIENT_SECRET=$(jq -er '.ampApiClient.clientSecret | strings | select(length > 0)' <<< "$bootstrap")
    fi
    if [[ -z "${THUNDER_CLIENT_SECRET:-}" ]]; then
        THUNDER_CLIENT_SECRET=$(jq -er '.ampSystemClient.clientSecret | strings | select(length > 0)' <<< "$bootstrap")
    fi
fi

export IDP_CLIENT_SECRET THUNDER_CLIENT_SECRET
exec docker compose -f "$SCRIPT_DIR/../docker-compose.yml" up -d --build "$@"
