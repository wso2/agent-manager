#!/bin/bash
# Registry authentication helpers — the SINGLE source of truth for how the
# installers name the image registry and turn registry credentials into the
# artifacts an image pull needs. Meant to be sourced, not executed directly.
#
# Why this is one file: the registry host has to appear in three unrelated
# places — a `docker login` on the host, a containerd `configs:` entry inside
# the k3d node, and a kubernetes.io/dockerconfigjson Secret in every namespace
# that runs a first-party image. Getting them out of step does not fail loudly:
# a secret scoped to a different host than the image tag is simply ignored, the
# pull falls back to anonymous and 401s. So the host is derived here once and
# never spelled out again.
#
# Credentials are read from the environment or a file, or prompted for. They are
# deliberately NOT accepted as command-line arguments: argv is readable by every
# user on the host through ps, and lands in shell history.

# Registry prefix (host and organization/project) the first-party AMP images are
# pulled from. Matches the charts' global.ampImageRegistry.
AMP_PUBLIC_IMAGE_REGISTRY="ghcr.io/wso2"
AMP_IMAGE_REGISTRY="${AMP_IMAGE_REGISTRY:-$AMP_PUBLIC_IMAGE_REGISTRY}"

# Name of the docker-registry Secret the installers create and the charts
# reference. One name everywhere, so a chart value and a kubectl invocation in a
# different script cannot disagree.
AMP_REGISTRY_SECRET_NAME="${AMP_REGISTRY_SECRET_NAME:-wso2-registry-credentials}"

# Registry prefix the WSO2 API Platform gateway (its chart and its controller and
# runtime images) is pulled from. The gateway is not built from this repository,
# only published beside it, so it keeps this location even when
# AMP_IMAGE_REGISTRY points somewhere else, such as a stage project.
GATEWAY_REGISTRY="${GATEWAY_REGISTRY:-registry.wso2.com/wso2-agent-manager/sts}"

# The gateway's own pull Secret. It is separate from AMP_REGISTRY_SECRET_NAME
# because both registries can sit on one host, and a docker config holds one
# credential per host: a stage test pulls AMP from the stage project and the
# gateway from the release project, each with its own robot.
GATEWAY_REGISTRY_SECRET_NAME="${GATEWAY_REGISTRY_SECRET_NAME:-wso2-gateway-registry-credentials}"

# amp_registry_host -> the registry hostname alone, without the organization.
# `docker login`, the containerd configs: key and --docker-server all key on the
# host; the organization belongs only in the image reference. Keeps any port.
amp_registry_host() {
    printf '%s' "${AMP_IMAGE_REGISTRY%%/*}"
}

# amp_registry_is_private -> true when images come from somewhere other than the
# public registry, which is what makes credentials and a pull secret necessary.
amp_registry_is_private() {
    [ "$AMP_IMAGE_REGISTRY" != "$AMP_PUBLIC_IMAGE_REGISTRY" ]
}

# amp_registry_verify_tls
# Confirm the registry answers over HTTPS with a certificate this host already
# trusts, before any credential is sent to it.
#
# This cannot be left to docker. A daemon configured with insecure-registries
# will send the token to a plain-HTTP endpoint without complaint, and
# AMP_IMAGE_REGISTRY is operator-supplied, so the check belongs here.
#
# Any HTTP status at all means the handshake succeeded and the chain verified —
# 401 is the expected answer from an authenticated registry. curl reports 000
# when TLS itself failed.
#
# AMP_REGISTRY_ALLOW_INSECURE=true opts out, for a mirror deliberately served
# over plain HTTP on a trusted network.
amp_registry_verify_tls() {    # [host], defaults to the AMP registry host
    if [ "${AMP_REGISTRY_ALLOW_INSECURE:-false}" = "true" ]; then
        return 0
    fi
    local host code
    host="${1:-$(amp_registry_host)}"
    code="$(curl -sS -o /dev/null --max-time 15 -w '%{http_code}' "https://${host}/v2/" 2>/dev/null || true)"
    case "$code" in
        [1-5][0-9][0-9]) return 0 ;;
    esac
    echo "❌ ${host} did not answer over HTTPS with a trusted certificate." >&2
    echo "   Refusing to send registry credentials in cleartext." >&2
    echo "   Set AMP_REGISTRY_ALLOW_INSECURE=true only if that is intended." >&2
    return 1
}

# amp_registry_load_credentials
# Populates AMP_REGISTRY_USERNAME / AMP_REGISTRY_PASSWORD from, in order:
#   1. the environment, when already set
#   2. AMP_REGISTRY_CREDENTIALS_FILE, a KEY=value file (expected mode 600)
#   3. an interactive prompt, when stdin is a terminal
# Returns non-zero when it cannot obtain both without a terminal, so a
# non-interactive install fails with a clear message instead of hanging.
amp_registry_load_credentials() {
    local file="${AMP_REGISTRY_CREDENTIALS_FILE:-}"

    if [ -n "$file" ] && { [ -z "${AMP_REGISTRY_USERNAME:-}" ] || [ -z "${AMP_REGISTRY_PASSWORD:-}" ]; }; then
        if [ ! -f "$file" ]; then
            echo "❌ Registry credentials file not found: $file" >&2
            return 1
        fi
        # Warn rather than refuse: the file may be on a filesystem that cannot
        # represent modes, but a world-readable credential is worth saying out loud.
        local mode
        mode="$(stat -f '%OLp' "$file" 2>/dev/null || stat -c '%a' "$file" 2>/dev/null || echo '')"
        case "$mode" in
            ''|600|400) ;;
            *) echo "⚠️  $file is mode $mode; 600 is expected for a credentials file" >&2 ;;
        esac
        # Parsed, never sourced: a credentials file must not be able to run code.
        # IFS='=' with two targets keeps any '=' inside the password intact.
        local key value
        while IFS='=' read -r key value || [ -n "$key" ]; do
            case "$key" in
                AMP_REGISTRY_USERNAME) [ -n "${AMP_REGISTRY_USERNAME:-}" ] || AMP_REGISTRY_USERNAME="${value%$'\r'}" ;;
                AMP_REGISTRY_PASSWORD) [ -n "${AMP_REGISTRY_PASSWORD:-}" ] || AMP_REGISTRY_PASSWORD="${value%$'\r'}" ;;
            esac
        done < "$file"
    fi

    if [ -z "${AMP_REGISTRY_USERNAME:-}" ]; then
        if [ -t 0 ]; then
            printf 'Registry username for %s: ' "$(amp_registry_host)" >&2
            IFS= read -r AMP_REGISTRY_USERNAME
        else
            echo "❌ No registry username. Set AMP_REGISTRY_USERNAME or AMP_REGISTRY_CREDENTIALS_FILE." >&2
            return 1
        fi
    fi

    if [ -z "${AMP_REGISTRY_PASSWORD:-}" ]; then
        if [ -t 0 ]; then
            # read -s so the token never reaches the terminal or a transcript.
            printf 'Registry token for %s: ' "$(amp_registry_host)" >&2
            IFS= read -rs AMP_REGISTRY_PASSWORD
            printf '\n' >&2
        else
            echo "❌ No registry token. Set AMP_REGISTRY_PASSWORD or AMP_REGISTRY_CREDENTIALS_FILE." >&2
            return 1
        fi
    fi

    [ -n "${AMP_REGISTRY_USERNAME:-}" ] && [ -n "${AMP_REGISTRY_PASSWORD:-}" ] || return 1

    # Checked here rather than in one caller, because this is the single point
    # every credential-emitting path goes through: the docker login, the
    # dockerconfigjson Secret, and the containerd fragment. Guarding only the
    # login would have left the other two sending the token unchecked.
    amp_registry_verify_tls
}

# _amp_yaml_escape <value> -> the value safe to place inside a double-quoted
# YAML scalar.
#
# Backslash first, or it would double-escape every escape added after it. Tab,
# carriage return and line feed are escaped too: a double-quoted scalar cannot
# carry them literally, so one in a credential would either change the value the
# node receives or invalidate the generated k3d configuration outright.
_amp_yaml_escape() {
    local v="$1"
    v="${v//\\/\\\\}"
    v="${v//\"/\\\"}"
    v="${v//$'\t'/\\t}"
    v="${v//$'\r'/\\r}"
    v="${v//$'\n'/\\n}"
    printf '%s' "$v"
}

# amp_registry_docker_login
# Authenticates the LOCAL docker daemon. Needed for images the host pulls
# itself (the quick-start container) — this does NOT authenticate pulls made by
# a cluster node, which has its own credential store.
amp_registry_docker_login() {
    amp_registry_load_credentials || return 1
    printf '%s' "$AMP_REGISTRY_PASSWORD" \
        | docker login "$(amp_registry_host)" -u "$AMP_REGISTRY_USERNAME" --password-stdin
}

# amp_registry_helm_login
# Authenticates helm for the charts. Charts are pulled by helm on the host, not
# by a node, so neither the pull secret nor the node's registries.yaml covers
# them. Only acts when HELM_CHART_REGISTRY is on the same private host as the
# images, which is the one case these credentials are for.
amp_registry_helm_login() {
    amp_registry_is_private || return 0
    local chart_host="${HELM_CHART_REGISTRY:-}"
    chart_host="${chart_host#oci://}"
    chart_host="${chart_host%%/*}"
    [ "$chart_host" = "$(amp_registry_host)" ] || return 0
    amp_registry_load_credentials || return 1
    printf '%s' "$AMP_REGISTRY_PASSWORD" \
        | helm registry login "$chart_host" --username "$AMP_REGISTRY_USERNAME" --password-stdin
}

# amp_create_pull_secret <namespace>...
# Creates or updates the docker-registry Secret in each namespace. Idempotent,
# so it is safe to re-run on an upgrade.
amp_create_pull_secret() {
    amp_registry_load_credentials || return 1
    _amp_apply_pull_secret "$AMP_REGISTRY_SECRET_NAME" "$(amp_registry_host)" \
        "$AMP_REGISTRY_USERNAME" "$AMP_REGISTRY_PASSWORD" "" "$@"
}

# _amp_apply_pull_secret <secret> <host> <username> <password> <with-plain-keys> <namespace>...
# Creates or updates a docker-registry Secret for <host> in each namespace. When
# <with-plain-keys> is non-empty the Secret also carries the credential as plain
# username/password keys, the form the gateway operator reads to pull a chart.
_amp_apply_pull_secret() {
    local secret="$1" host="$2" user="$3" pass="$4" plain="$5"
    shift 5
    # Build the docker config here rather than letting kubectl take the token as
    # --docker-password: argv is readable by any user on the host through ps,
    # which is the same reason these credentials are not accepted as a flag.
    # base64 output is alphanumeric, so neither value can break the JSON.
    local cfg ns rc=0 user_file pass_file
    local extra=()
    cfg="$(mktemp)"
    chmod 600 "$cfg"
    if ! printf '{"auths":{"%s":{"auth":"%s"}}}' \
        "$host" \
        "$(printf '%s:%s' "$user" "$pass" | base64 | tr -d '\n')" \
        > "$cfg"; then
        rm -f "$cfg"
        return 1
    fi
    if [ -n "$plain" ]; then
        # Files, not --from-literal, to keep the token out of argv.
        user_file="$(mktemp)"; pass_file="$(mktemp)"
        chmod 600 "$user_file" "$pass_file"
        printf '%s' "$user" > "$user_file"
        printf '%s' "$pass" > "$pass_file"
        extra=(--from-file=username="$user_file" --from-file=password="$pass_file")
    fi
    for ns in "$@"; do
        # Create the namespace only when absent. Several of these namespaces are
        # owned by a Helm release or by OpenChoreo, and applying a bare Namespace
        # over one of those would fight its ownership metadata.
        # --ignore-not-found so an absent namespace exits zero with no output,
        # which separates it from a real failure. Without that, a permissions or
        # connectivity error fell through to create and surfaced as "cannot
        # create" rather than naming what actually went wrong.
        local found
        if ! found="$(kubectl get namespace "$ns" --ignore-not-found -o name 2>&1)"; then
            echo "❌ Cannot query namespace ${ns}: ${found}" >&2
            rc=1
            continue
        fi
        if [ -z "$found" ]; then
            kubectl create namespace "$ns" >/dev/null || { rc=1; continue; }
        fi
        kubectl create secret generic "$secret" \
            --namespace "$ns" \
            --type=kubernetes.io/dockerconfigjson \
            --from-file=.dockerconfigjson="$cfg" \
            ${extra[@]+"${extra[@]}"} \
            --dry-run=client -o yaml | kubectl apply -f - >/dev/null || rc=1
    done
    rm -f "$cfg" ${user_file:+"$user_file"} ${pass_file:+"$pass_file"}
    # Return the failure explicitly. Both callers test this in an `if`, which
    # disables errexit inside the function, so the cleanup above would otherwise
    # be the last command and report success over a Secret that was never
    # created — leaving the pull to fail much later with nothing pointing here.
    return "$rc"
}

# amp_registries_yaml_fragment
# Prints the containerd `configs:` block for the k3d node's registries.yaml,
# indented to sit inside the `registries.config: |` literal of k3d-config.yaml.
#
# This is the only mechanism that covers pulls into dynamically created
# namespaces: a cluster node's containerd sees neither the host's docker config
# nor a namespace's imagePullSecrets when the namespace did not exist at install
# time. k3d passes registries.config through verbatim (verified against v5.8.3),
# and the VM installer's rewrite of the mirror endpoint does not match these
# lines, so the block survives both.
amp_registries_yaml_fragment() {
    amp_registry_load_credentials || return 1
    cat <<EOF
    configs:
      "$(amp_registry_host)":
        auth:
          username: "$(_amp_yaml_escape "$AMP_REGISTRY_USERNAME")"
          password: "$(_amp_yaml_escape "$AMP_REGISTRY_PASSWORD")"
EOF
}

# amp_pull_secret_helm_args
# Prints the helm flags that point a chart at the private registry, one token per
# line so a caller can read them into an array. Prints nothing for a public
# install, which keeps every existing invocation byte-identical.
amp_pull_secret_helm_args() {
    amp_registry_is_private || return 0
    printf '%s\n' \
        "--set" "global.ampImageRegistry=${AMP_IMAGE_REGISTRY}" \
        "--set" "global.imagePullSecrets[0]=${AMP_REGISTRY_SECRET_NAME}"
}

# amp_gateway_registry_host -> the gateway registry's hostname alone.
amp_gateway_registry_host() {
    printf '%s' "${GATEWAY_REGISTRY%%/*}"
}

# amp_gateway_load_credentials
# Populates GATEWAY_REGISTRY_USERNAME / GATEWAY_REGISTRY_PASSWORD. Without both
# set, they are the AMP registry credentials: a customer has one robot for the
# release project, which holds AMP and the gateway alike. Only a stage test sets
# them apart.
amp_gateway_load_credentials() {
    if [ -z "${GATEWAY_REGISTRY_USERNAME:-}" ] || [ -z "${GATEWAY_REGISTRY_PASSWORD:-}" ]; then
        amp_registry_load_credentials || return 1
        GATEWAY_REGISTRY_USERNAME="$AMP_REGISTRY_USERNAME"
        GATEWAY_REGISTRY_PASSWORD="$AMP_REGISTRY_PASSWORD"
    fi
    amp_registry_verify_tls "$(amp_gateway_registry_host)"
}

# amp_create_gateway_pull_secret <namespace>...
# Creates the gateway's pull Secret in each namespace: the gateway operator's,
# where it pulls the gateway chart, and every namespace a gateway runs in. Each
# gateway pod lists it, so its images are pulled with these credentials and not
# the k3d node's, which are the AMP registry's.
amp_create_gateway_pull_secret() {
    amp_gateway_load_credentials || return 1
    _amp_apply_pull_secret "$GATEWAY_REGISTRY_SECRET_NAME" "$(amp_gateway_registry_host)" \
        "$GATEWAY_REGISTRY_USERNAME" "$GATEWAY_REGISTRY_PASSWORD" plain "$@"
}

# amp_gateway_operator_helm_args
# Prints the gateway-operator flags that pull the gateway chart and images from
# GATEWAY_REGISTRY with the gateway's pull Secret, one token per line. The
# operator reads the Secret from its own namespace for the chart, and each
# gateway lists it as an image pull secret.
amp_gateway_operator_helm_args() {
    printf '%s\n' \
        "--set" "gateway.helm.chartName=oci://${GATEWAY_REGISTRY}/helm-charts/gateway" \
        "--set" "gateway.helm.registryCredentialsSecret.name=${GATEWAY_REGISTRY_SECRET_NAME}" \
        "--set" "gateway.values.imagePullSecrets[0]=${GATEWAY_REGISTRY_SECRET_NAME}" \
        "--set" "gateway.values.gateway.controller.image.repository=${GATEWAY_REGISTRY}/gateway-controller" \
        "--set" "gateway.values.gateway.gatewayRuntime.image.repository=${GATEWAY_REGISTRY}/gateway-runtime"
}

# amp_gateway_release_pins
# Reads a gateway extension chart's values.yaml on stdin and prints, one per
# line, the gateway chart version, the gateway image tag and the gateway-operator
# chart version from its gatewayRelease block. Prints nothing when the block or
# any pin is missing, as in a chart published before the block existed, so the
# caller keeps its own defaults.
# Parsed with awk rather than yq, which the installer does not require.
amp_gateway_release_pins() {
    awk '
        /^gatewayRelease:/ { block = 1; next }
        block && /^[^[:space:]#]/ { block = 0 }
        block && $1 == "chartVersion:" { chart = $2 }
        block && $1 == "imageVersion:" { image = $2 }
        block && $1 == "operatorVersion:" { operator = $2 }
        END {
            gsub(/["\047]/, "", chart); gsub(/["\047]/, "", image); gsub(/["\047]/, "", operator)
            if (chart != "" && image != "" && operator != "") { print chart; print image; print operator }
        }
    '
}
