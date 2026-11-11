#!/bin/bash
# Shared setup for the local and quick-start platform credentials. Production
# Helm installs can supply their own AMP_PLATFORM_VAULT_SEED Kubernetes Secret.

init_platform_vault() (
    set -euo pipefail
    umask 077
    local dir="$1" seed_file="$1/seed.json" tmp
    mkdir -p "$dir"
    tmp=$(mktemp "$dir/seed.XXXXXX")
    trap 'rm -f "$tmp"' EXIT

    if [[ ! -s "$seed_file" ]]; then
        # The OpenSearch Helm hooks base64-encode admin:<password> into a
        # header. Keep the input within 57 bytes to avoid base64 line wrapping.
        jq -n             --arg search_password "Aa1!$(openssl rand -hex 23)"             --arg observer "$(openssl rand -hex 32)"             --arg workload_publisher "$(openssl rand -hex 32)"             --arg monitor_publisher "$(openssl rand -hex 32)"             '{
                "opensearch-username": "admin",
                "opensearch-password": $search_password,
                "observer-oauth-client-secret": $observer,
                "workflow-plane-oauth-client-secret": $workload_publisher,
                "amp-publisher-client-secret": $monitor_publisher
            }' > "$tmp"
        # Publish only a complete file and preserve a seed created concurrently.
        ln "$tmp" "$seed_file" 2>/dev/null || test -s "$seed_file"
        rm -f "$tmp"
    fi
    if [[ -n "${AMP_REGISTRY_PUSH_CONFIG_FILE:-}" ]]; then
        if ! jq -e '.auths | type == "object" and length > 0' "$AMP_REGISTRY_PUSH_CONFIG_FILE" >/dev/null; then
            echo "AMP_REGISTRY_PUSH_CONFIG_FILE must contain Docker registry credentials" >&2
            exit 1
        fi
        jq --rawfile auth "$AMP_REGISTRY_PUSH_CONFIG_FILE" '.["registry-push-secret"] = $auth' "$seed_file" > "$tmp"
        mv "$tmp" "$seed_file"
    fi
    jq -c '{
        thunder: {
            bootstrap: {
                observerResourceReaderClient: {clientSecret: .["observer-oauth-client-secret"]},
                workloadPublisherClient: {clientSecret: .["workflow-plane-oauth-client-secret"]},
                ampPublisherClient: {clientSecret: .["amp-publisher-client-secret"]}
            }
        }
    }' "$seed_file" > "$dir/thunder-values.json"
    jq -cr '"AMP_PLATFORM_VAULT_SEED=" + tojson' "$seed_file" > "$dir/compose.env"
)

platform_vault_has_registry_credentials() {
    jq -e 'has("registry-push-secret")' "$1/seed.json" >/dev/null
}
