#!/bin/sh
# Prepares the AI Gateway for a standalone (no Kubernetes) install:
#   1. generates the at-rest encryption key and HTTPS listener certificate once,
#   2. registers the gateway with Agent Manager and mints its registration token,
#   3. renders config.toml (shared by gateway-controller and gateway-runtime).
# With --apis it instead loads the OTLP trace-ingest API into a running
# controller. Everything it writes goes to $GW_DIR and is reused on reruns.
set -eu

GW_DIR="${GW_DIR:-/amp/gw}"
TEMPLATE="${TEMPLATE:-/amp/gateway/config.toml.tmpl}"

log() { echo "[gateway-init] $*"; }
die() { echo "[gateway-init] ERROR: $*" >&2; exit 1; }

# json_field NAME - first string value of "NAME" in the JSON on stdin.
json_field() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }

load_apis() {
    : "${CONTROLLER_URL:?}" "${GATEWAY_ADMIN_PASSWORD:?}" "${OTEL_UPSTREAM_URL:?}"
    sed "s#@@OTEL_UPSTREAM_URL@@#${OTEL_UPSTREAM_URL}#g" /amp/gateway/otel-ingest-api.yaml > /tmp/otel-api.yaml
    auth="admin:${GATEWAY_ADMIN_PASSWORD}"
    if curl -sf -u "$auth" "${CONTROLLER_URL}/api/management/v1/rest-apis/otel-ingest-v1.0" >/dev/null; then
        log "OTLP ingest API already deployed; updating it"
        method=PUT; url="${CONTROLLER_URL}/api/management/v1/rest-apis/otel-ingest-v1.0"
    else
        method=POST; url="${CONTROLLER_URL}/api/management/v1/rest-apis"
    fi
    status=$(curl -s -o /tmp/resp -w '%{http_code}' -X "$method" -u "$auth" \
        -H 'Content-Type: application/yaml' --data-binary @/tmp/otel-api.yaml "$url")
    case "$status" in
        2*) log "OTLP ingest API deployed" ;;
        *) die "$method $url returned $status: $(cat /tmp/resp)" ;;
    esac
}

if [ "${1:-}" = "--apis" ]; then
    load_apis
    exit 0
fi

: "${AMP_API_URL:?}" "${IDP_TOKEN_URL:?}" "${IDP_CLIENT_ID:?}" "${IDP_CLIENT_SECRET:?}"
: "${ORG_NAME:?}" "${ENVIRONMENT_NAME:?}" "${GATEWAY_NAME:?}" "${GATEWAY_VHOST:?}" "${GATEWAY_TYPE:?}"
: "${CONTROLPLANE_HOST:?}" "${GATEWAY_ADMIN_PASSWORD:?}"
: "${AMP_JWKS_URL:?}" "${THUNDER_ISSUER:?}" "${THUNDER_JWKS_URL:?}"

mkdir -p "$GW_DIR/aesgcm-keys" "$GW_DIR/listener-certs"

key="$GW_DIR/aesgcm-keys/default-aesgcm256-v1.bin"
if [ ! -s "$key" ]; then
    openssl rand 32 > "$key"
    log "generated the encryption key"
fi

crt="$GW_DIR/listener-certs/default-listener.crt"
if [ ! -s "$crt" ]; then
    gw_host=$(echo "$GATEWAY_VHOST" | sed -e 's#^[a-z]*://##' -e 's#[:/].*##')
    openssl req -x509 -newkey rsa:2048 -nodes -days 825 -subj "/CN=${gw_host}" \
        -addext "subjectAltName=DNS:${gw_host},DNS:localhost" \
        -keyout "$GW_DIR/listener-certs/default-listener.key" -out "$crt" 2>/dev/null
    log "generated a self-signed listener certificate for ${gw_host}"
fi

log "waiting for Agent Manager at ${AMP_API_URL}"
i=0
until curl -sf "${AMP_API_URL%/api/v1}/healthz" >/dev/null; do
    i=$((i + 1)); [ "$i" -ge 60 ] && die "Agent Manager not healthy after 120s"
    sleep 2
done

basic=$(printf '%s:%s' "$IDP_CLIENT_ID" "$IDP_CLIENT_SECRET" | base64 | tr -d '\n')
access=$(curl -sf -X POST "$IDP_TOKEN_URL" -H "Authorization: Basic ${basic}" \
    --data-urlencode grant_type=client_credentials \
    --data-urlencode "scope=amp:gateway:create amp:gateway:read amp:gateway:update amp:gateway:token-manage amp:environment:read" \
    | json_field access_token) || true
[ -n "$access" ] || die "could not get a token from ${IDP_TOKEN_URL}"
bearer="Authorization: Bearer ${access}"

list=$(curl -sf -H "$bearer" "${AMP_API_URL}/orgs/${ORG_NAME}/gateways?limit=100") || die "listing gateways failed"
gw_id=$(echo "$list" | grep -o '"uuid":"[^"]*"[^}]*"name":"'"$GATEWAY_NAME"'"\|"name":"'"$GATEWAY_NAME"'"[^}]*"uuid":"[^"]*"' \
    | grep -o '"uuid":"[^"]*"' | head -1 | cut -d'"' -f4) || true

if [ -n "$gw_id" ]; then
    log "gateway '${GATEWAY_NAME}' already registered (${gw_id})"
else
    envs=$(curl -sf -H "$bearer" "${AMP_API_URL}/orgs/${ORG_NAME}/environments") || die "listing environments failed"
    env_id=$(echo "$envs" | grep -o '"id":"[^"]*"[^}]*"name":"'"$ENVIRONMENT_NAME"'"\|"name":"'"$ENVIRONMENT_NAME"'"[^}]*"id":"[^"]*"' \
        | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4) || true
    [ -n "$env_id" ] || die "environment '${ENVIRONMENT_NAME}' not found: ${envs}"
    body='{"name":"'"$GATEWAY_NAME"'","displayName":"'"${GATEWAY_DISPLAY_NAME:-$GATEWAY_NAME}"'","gatewayType":"'"$GATEWAY_TYPE"'","vhost":"'"$GATEWAY_VHOST"'","environmentIds":["'"$env_id"'"]}'
    resp=$(curl -s -X POST -H "$bearer" -H 'Content-Type: application/json' -d "$body" "${AMP_API_URL}/orgs/${ORG_NAME}/gateways")
    gw_id=$(echo "$resp" | json_field uuid)
    [ -n "$gw_id" ] || die "registering the gateway failed: ${resp}"
    log "registered gateway '${GATEWAY_NAME}' (${gw_id})"
fi

token_file="$GW_DIR/registration-token"
if [ -s "$token_file" ] && curl -sf -H "$bearer" "${AMP_API_URL}/orgs/${ORG_NAME}/gateways/${gw_id}/tokens" | grep -q '"id"'; then
    log "reusing the existing registration token"
else
    resp=$(curl -s -X POST -H "$bearer" "${AMP_API_URL}/orgs/${ORG_NAME}/gateways/${gw_id}/tokens")
    token=$(echo "$resp" | json_field token)
    [ -n "$token" ] || die "minting a registration token failed: ${resp}"
    (umask 077; printf '%s' "$token" > "$token_file")
    log "minted a registration token"
fi

gw_host=$(echo "$GATEWAY_VHOST" | sed -e 's#^[a-z]*://##' -e 's#[:/].*##')
(umask 027; sed -e "s#@@CONTROLPLANE_HOST@@#${CONTROLPLANE_HOST}#g" \
    -e "s#@@CONTROLPLANE_TOKEN@@#$(cat "$token_file")#g" \
    -e "s#@@GATEWAY_ADMIN_PASSWORD@@#${GATEWAY_ADMIN_PASSWORD}#g" \
    -e "s#@@AMP_JWKS_URL@@#${AMP_JWKS_URL}#g" \
    -e "s#@@THUNDER_ISSUER@@#${THUNDER_ISSUER}#g" \
    -e "s#@@THUNDER_JWKS_URL@@#${THUNDER_JWKS_URL}#g" \
    -e "s#@@GATEWAY_HOST@@#${gw_host}#g" \
    "$TEMPLATE" > "$GW_DIR/config.toml")

# gateway-controller and gateway-runtime run as uid 10001.
chown -R 10001:10001 "$GW_DIR"
log "rendered ${GW_DIR}/config.toml"
