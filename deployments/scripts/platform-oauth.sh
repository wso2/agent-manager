#!/usr/bin/env bash
# Shared credentials for the bundled single-cluster installers. The source
# Secret is created before OpenBao and Thunder start and survives Helm retries.

# A missing Secret or empty key returns 2. API and decoding failures return 1.
platform_oauth_secret_data() {
    local namespace="$1" name="$2" key="$3" encoded
    shift 3
    encoded="$(kubectl "$@" get secret "$name" -n "$namespace" --ignore-not-found -o "jsonpath={.data.$key}")" || return 1
    if [[ -z "$encoded" ]]; then
        printf 'Missing %s in %s/%s\n' "$key" "$namespace" "$name" >&2
        return 2
    fi
    printf '%s' "$encoded"
}

platform_oauth_read() {
    local key="$1" namespace="${THUNDER_NS:-amp-thunder}" encoded
    local -a context=()
    if [[ -n "${2:-}" ]]; then
        context=(--context "$2")
    fi
    encoded="$(platform_oauth_secret_data "$namespace" amp-platform-oauth "$key" ${context[@]+"${context[@]}"})" || return $?
    printf '%s' "$encoded" | openssl base64 -d -A || return 1
}

platform_oauth_init() (
    set -o pipefail
    local namespace="${THUNDER_NS:-amp-thunder}" existing releases directory key
    kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f - >/dev/null || exit 1
    existing="$(kubectl get secret amp-platform-oauth -n "$namespace" --ignore-not-found -o name)" || exit 1
    [[ -z "$existing" ]] || exit 0

    releases="$(helm list -n "$namespace" --filter '^amp-thunder-extension$' -q)" || exit 1
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
    if ! kubectl create secret generic amp-platform-oauth -n "$namespace" "${sources[@]}" >/dev/null; then
        # A concurrent installer may have created the Secret first.
        kubectl get secret amp-platform-oauth -n "$namespace" >/dev/null || exit 1
    fi
)

# Copy only the credentials used by the target component. Pass extra data as
# key=value pairs, which are kept out of kubectl's process arguments.
platform_oauth_copy() (
    set -o pipefail
    local namespace="$1" name="$2" key value pair directory existing
    shift 2
    kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f - >/dev/null || exit 1
    existing="$(kubectl get secret "$name" -n "$namespace" --ignore-not-found -o name)" || exit 1
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
    if ! kubectl create secret generic "$name" -n "$namespace" "${sources[@]}" >/dev/null; then
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
    existing="$(kubectl get secret amp-platform-oauth -n "$namespace" --ignore-not-found -o name)" || return 1
    [ -n "$existing" ] || return 0
    IDP_CLIENT_SECRET="${IDP_CLIENT_SECRET:-$(platform_oauth_read AMP_API_CLIENT_SECRET)}" || return 1
    THUNDER_SYSTEM_CLIENT_SECRET="${THUNDER_SYSTEM_CLIENT_SECRET:-$(platform_oauth_read AMP_SYSTEM_CLIENT_SECRET)}" || return 1
    export IDP_CLIENT_SECRET THUNDER_SYSTEM_CLIENT_SECRET
}
