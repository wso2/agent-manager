#!/bin/bash
# agent-manager-service (AMS) auth helpers — the SINGLE source of truth for
# minting a Bearer token to call AMS's admin API from a provisioning script.
# Meant to be sourced, not executed directly. Every script that talks to AMS's
# thunder-system-client endpoint sources THIS file instead of re-implementing
# these functions — see each caller's naming-lib loader for how it's fetched
# when run standalone via curl | bash. Do not add a copy of these functions
# anywhere else.

# json_escape -> prints $1 with backslash/quote escaped for a JSON string value.
json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g'
}

# get_ams_token [max_retries] -> prints a Bearer token for calling AMS.
# Uses AGENT_MANAGER_TOKEN if set, else mints one via client_credentials against platform Thunder.
get_ams_token() {
  local max_retries="${1:-5}"

  if [ -n "${AGENT_MANAGER_TOKEN:-}" ]; then
    printf '%s' "${AGENT_MANAGER_TOKEN}"
    return 0
  fi

  local idp_token_url="${IDP_TOKEN_URL:-http://thunder.amp.localhost:8080/oauth2/token}"
  local idp_client_id="${IDP_CLIENT_ID:-amp-api-client}"
  local idp_client_secret="${IDP_CLIENT_SECRET:-}"
  if [ -z "$idp_client_secret" ]; then
    local encoded
    encoded="$(kubectl get secret amp-platform-oauth -n "${THUNDER_NS:-amp-thunder}" \
      -o 'jsonpath={.data.AMP_API_CLIENT_SECRET}')" || return 1
    idp_client_secret="$(printf '%s' "$encoded" | openssl base64 -d -A)" || return 1
    if [ -z "$idp_client_secret" ]; then
      echo "Set IDP_CLIENT_SECRET or AGENT_MANAGER_TOKEN for the platform Thunder client." >&2
      return 1
    fi
  fi

  # Explicit scope required: an unscoped client_credentials token gets HTTP 403
  # from AMS's RBAC check on the thunder-system-client route (confirmed live).
  local token_response
  token_response="$(curl -sf --max-time 30 --retry "$max_retries" --retry-delay 5 \
    -X POST "${idp_token_url}" \
    -u "${idp_client_id}:${idp_client_secret}" \
    -d "grant_type=client_credentials" \
    --data-urlencode "scope=amp:org:manage-service-account" 2>/dev/null)" || return 1

  local access_token
  access_token="$(printf '%s' "${token_response}" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)"
  [ -z "$access_token" ] && return 1
  printf '%s' "$access_token"
}

# get_thunder_url_handle ORG ENV [max_retries] -> prints ORG/ENV's registered
# env-Thunder URL handle (learned via GET — the caller has no other way to know
# whether register_thunder_url stored a caller-supplied or server-generated
# value), or returns 1 if none is registered / the lookup fails.
#
# Every script that needs to ADDRESS an already-provisioned env-Thunder (as
# opposed to add-environment-thunder.sh's register_thunder_url, which REGISTERS
# one) calls this single implementation instead of re-deriving the handle
# locally — thunder_host/thunder_issuer (see thunder-naming.sh) require the
# real registered handle as input and have no fallback of their own, so a
# caller that skips this lookup and guesses a value instead wires the gateway
# to a hostname Thunder never actually issues tokens for.
get_thunder_url_handle() {
  local org="$1" env_name="$2" max_retries="${3:-5}"
  local amp_api_url="${AMP_API_URL:-http://localhost:9000/api/v1}"

  local access_token
  if ! access_token="$(get_ams_token "$max_retries")"; then
    return 1
  fi

  local response http_code resp_body
  response="$(curl -s -w '\n%{http_code}' \
    --max-time 30 --retry "$max_retries" --retry-delay 5 \
    -X GET "${amp_api_url}/orgs/${org}/environments/${env_name}/thunder-url" \
    -H "Authorization: Bearer ${access_token}" 2>/dev/null)"
  http_code="$(printf '%s' "$response" | tail -n1)"
  resp_body="$(printf '%s' "$response" | sed '$d')"
  [ "$http_code" = "200" ] || return 1

  local handle
  handle="$(printf '%s' "$resp_body" | grep -o '"handle":"[^"]*"' | cut -d'"' -f4)"
  [ -z "$handle" ] && return 1
  printf '%s' "$handle"
}
