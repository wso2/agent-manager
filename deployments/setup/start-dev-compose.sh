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
compose=(docker compose -f "$SCRIPT_DIR/../docker-compose.yml")
"${compose[@]}" build agent-manager-service
# Keys volumes created by the earlier root-run image are owned by root; hand them
# to the runtime user. Safe to repeat on a volume that is already migrated.
# A plain docker run mounts only the volume, so it does not clash with a running
# service over its fixed network address.
project=$("${compose[@]}" config | sed -n 's/^name: //p')
keys_volume="${project}_agent_manager_keys"
if docker volume inspect "$keys_volume" >/dev/null 2>&1; then
    image="${project}-agent-manager-service"
    docker run --rm --user root --entrypoint chown \
        -v "$keys_volume:/var/lib/agent-manager/keys" \
        "$image" -R 1000:1000 /var/lib/agent-manager/keys
fi
# --wait fails the command when a service exits or never turns healthy.
exec "${compose[@]}" up -d --build --wait "$@"
