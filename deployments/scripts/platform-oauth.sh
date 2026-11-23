#!/usr/bin/env bash
# Shared credentials for the bundled single-cluster installers. The source
# Secret is created before OpenBao and Thunder start and survives Helm retries.

# Every call targets PLATFORM_OAUTH_CONTEXT, falling back to the installer's
# CLUSTER_CONTEXT, so Secrets land on the same cluster as the rest of the
# install even when the active kubeconfig context points elsewhere. With
# neither set, the active context is used.
platform_oauth_context() {
    printf '%s' "${PLATFORM_OAUTH_CONTEXT:-${CLUSTER_CONTEXT:-}}"
}

platform_oauth_kubectl() {
    local context
    context="$(platform_oauth_context)"
    if [[ -n "$context" ]]; then
        kubectl --context "$context" "$@"
    else
        kubectl "$@"
    fi
}

platform_oauth_helm() {
    local context
    context="$(platform_oauth_context)"
    if [[ -n "$context" ]]; then
        helm --kube-context "$context" "$@"
    else
        helm "$@"
    fi
}

# A missing Secret or empty key returns 2. API and decoding failures return 1.
platform_oauth_secret_data() {
    local namespace="$1" name="$2" key="$3" encoded
    encoded="$(platform_oauth_kubectl get secret "$name" -n "$namespace" --ignore-not-found -o "jsonpath={.data.$key}")" || return 1
    if [[ -z "$encoded" ]]; then
        printf 'Missing %s in %s/%s\n' "$key" "$namespace" "$name" >&2
        return 2
    fi
    printf '%s' "$encoded"
}

platform_oauth_read() {
    local key="$1" namespace="${THUNDER_NS:-amp-thunder}" encoded
    encoded="$(PLATFORM_OAUTH_CONTEXT="${2:-$(platform_oauth_context)}" platform_oauth_secret_data "$namespace" amp-platform-oauth "$key")" || return $?
    printf '%s' "$encoded" | openssl base64 -d -A || return 1
}

platform_oauth_init() (
    set -o pipefail
    local namespace="${THUNDER_NS:-amp-thunder}" existing releases directory key
    platform_oauth_kubectl create namespace "$namespace" --dry-run=client -o yaml | platform_oauth_kubectl apply -f - >/dev/null || exit 1
    existing="$(platform_oauth_kubectl get secret amp-platform-oauth -n "$namespace" --ignore-not-found -o name)" || exit 1
    [[ -z "$existing" ]] || exit 0

    releases="$(platform_oauth_helm list -n "$namespace" --filter '^amp-thunder-extension$' -q)" || exit 1
    if [[ -n "$releases" ]]; then
        printf '%s\n' 'Thunder is already installed. Rotate its OAuth clients and provision amp-platform-oauth before continuing.' >&2
        exit 1
    fi

    umask 077
    directory="$(mktemp -d)" || exit 1
    trap 'rm -rf "$directory"' EXIT
    local -a sources=()
    for key in AMP_API_CLIENT_SECRET AMP_SYSTEM_CLIENT_SECRET AMP_PUBLISHER_CLIENT_SECRET AM_OBSERVER_CLIENT_SECRET OBSERVER_RESOURCE_READER_SECRET WORKLOAD_PUBLISHER_SECRET; do
        openssl rand -hex 32 | tr -d '\n' > "$directory/$key" || exit 1
        [[ -s "$directory/$key" ]] || exit 1
        sources+=("--from-file=$key=$directory/$key")
    done
    if ! platform_oauth_kubectl create secret generic amp-platform-oauth -n "$namespace" "${sources[@]}" >/dev/null; then
        # A concurrent installer may have created the Secret first.
        platform_oauth_kubectl get secret amp-platform-oauth -n "$namespace" >/dev/null || exit 1
    fi
)

# Copy only the credentials used by the target component. Pass extra data as
# key=value pairs, which are kept out of kubectl's process arguments.
platform_oauth_copy() (
    set -o pipefail
    local namespace="$1" name="$2" key value pair directory existing
    shift 2
    platform_oauth_kubectl create namespace "$namespace" --dry-run=client -o yaml | platform_oauth_kubectl apply -f - >/dev/null || exit 1
    existing="$(platform_oauth_kubectl get secret "$name" -n "$namespace" --ignore-not-found -o name)" || exit 1
    if [[ -n "$existing" ]]; then
        for pair in "$@"; do
            platform_oauth_secret_data "$namespace" "$name" "${pair%%=*}" >/dev/null || exit 1
        done
        exit 0
    fi
    umask 077
    directory="$(mktemp -d)" || exit 1
    trap 'rm -rf "$directory"' EXIT
    local -a sources=()
    for pair in "$@"; do
        key="${pair%%=*}"
        value="${pair#*=}"
        printf '%s' "$value" > "$directory/$key" || exit 1
        sources+=("--from-file=$key=$directory/$key")
    done
    if ! platform_oauth_kubectl create secret generic "$name" -n "$namespace" "${sources[@]}" >/dev/null; then
        for pair in "$@"; do
            platform_oauth_secret_data "$namespace" "$name" "${pair%%=*}" >/dev/null || exit 1
        done
    fi
)

platform_oauth_for_openbao() {
    local reader publisher amp_publisher
    reader="$(platform_oauth_read OBSERVER_RESOURCE_READER_SECRET)" || return 1
    publisher="$(platform_oauth_read WORKLOAD_PUBLISHER_SECRET)" || return 1
    amp_publisher="$(platform_oauth_read AMP_PUBLISHER_CLIENT_SECRET)" || return 1
    platform_oauth_copy openbao amp-platform-oauth \
        "OBSERVER_RESOURCE_READER_SECRET=$reader" "WORKLOAD_PUBLISHER_SECRET=$publisher" \
        "AMP_PUBLISHER_CLIENT_SECRET=$amp_publisher"
}

platform_oauth_for_gateway() {
    local secret
    secret="$(platform_oauth_read AMP_API_CLIENT_SECRET)" || return 1
    platform_oauth_copy "$1" gateway-idp-credentials "client-id=amp-api-client" "client-secret=$secret"
}

platform_oauth_for_observer() {
    local secret
    secret="$(platform_oauth_read AM_OBSERVER_CLIENT_SECRET)" || return 1
    platform_oauth_copy "$1" amp-observer-oauth "idpClientId=am-observer-client" "idpClientSecret=$secret"
}

platform_oauth_for_manager() {
    local api system
    api="$(platform_oauth_read AMP_API_CLIENT_SECRET)" || return 1
    system="$(platform_oauth_read AMP_SYSTEM_CLIENT_SECRET)" || return 1
    platform_oauth_copy "$1" amp-platform-oauth "AMP_API_CLIENT_SECRET=$api" "AMP_SYSTEM_CLIENT_SECRET=$system"
}

platform_oauth_test_env() {
    if [ -n "${IDP_CLIENT_SECRET:-}" ] && [ -n "${THUNDER_SYSTEM_CLIENT_SECRET:-}" ]; then
        return 0
    fi
    local namespace="${THUNDER_NS:-amp-thunder}" existing
    existing="$(platform_oauth_kubectl get secret amp-platform-oauth -n "$namespace" --ignore-not-found -o name)" || return 1
    [ -n "$existing" ] || return 0
    IDP_CLIENT_SECRET="${IDP_CLIENT_SECRET:-$(platform_oauth_read AMP_API_CLIENT_SECRET)}" || return 1
    THUNDER_SYSTEM_CLIENT_SECRET="${THUNDER_SYSTEM_CLIENT_SECRET:-$(platform_oauth_read AMP_SYSTEM_CLIENT_SECRET)}" || return 1
    export IDP_CLIENT_SECRET THUNDER_SYSTEM_CLIENT_SECRET
}

# Bring a running OpenBao in line with amp-platform-oauth. OpenBao seeds its
# flat bootstrap keys only when its pod starts, so a reset that keeps OpenBao
# (or a cluster installed before per-install credentials) would otherwise keep
# serving old values to the workflow and observer planes. Values are passed on
# stdin, never as process arguments, and each write is read back to verify it.
platform_oauth_sync_openbao() (
    set -o pipefail
    local pod key path value stored directory
    pod="$(platform_oauth_kubectl get pods -n openbao -l app.kubernetes.io/name=openbao \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)" || pod=""
    if [[ -z "$pod" ]]; then
        printf '%s\n' 'OpenBao is not running; its credentials are seeded when it starts.' >&2
        exit 0
    fi

    umask 077
    directory="$(mktemp -d)" || exit 1
    trap 'rm -rf "$directory"' EXIT
    local -a sources=()
    for key in OBSERVER_RESOURCE_READER_SECRET WORKLOAD_PUBLISHER_SECRET AMP_PUBLISHER_CLIENT_SECRET; do
        platform_oauth_read "$key" > "$directory/$key" || exit 1
        sources+=("--from-file=$key=$directory/$key")
    done
    # Replace the OpenBao copy so a later pod restart seeds the same values.
    platform_oauth_kubectl create secret generic amp-platform-oauth -n openbao "${sources[@]}" \
        --dry-run=client -o yaml | platform_oauth_kubectl apply -f - >/dev/null || exit 1

    for pair in workflow-plane-oauth-client-secret=WORKLOAD_PUBLISHER_SECRET \
        amp-publisher-client-secret=AMP_PUBLISHER_CLIENT_SECRET \
        observer-oauth-client-secret=OBSERVER_RESOURCE_READER_SECRET; do
        path="${pair%%=*}"
        key="${pair#*=}"
        value="$(cat "$directory/$key")" || exit 1
        platform_oauth_kubectl exec -i -n openbao "$pod" -- env BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN=root \
            bao kv put "secret/$path" value=- < "$directory/$key" >/dev/null || exit 1
        stored="$(platform_oauth_kubectl exec -n openbao "$pod" -- env BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN=root \
            bao kv get -field=value "secret/$path")" || exit 1
        if [[ "$stored" != "$value" ]]; then
            printf 'OpenBao secret/%s does not match amp-platform-oauth after update\n' "$path" >&2
            exit 1
        fi
    done
)
