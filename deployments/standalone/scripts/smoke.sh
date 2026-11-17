#!/usr/bin/env bash
# smoke.sh — smoke-test a running standalone stack.
#
#   scripts/smoke.sh amp|amp-core [--no-console]
#
# amp is the full stack (`make up`), amp-core the core binary without the AI
# Gateway (`make up-core`). The checks cover sign-in, the core API, which
# routes each distribution serves, a trace sent end to end, the console the API
# serves and the UI manifest. --no-console skips the console files, for an
# image built with CONSOLE=none.
#
# Settings come from the environment, else from .env next to the compose
# files: AMP_API_PORT, AMP_INTERNAL_PORT, OBSERVER_PORT, GATEWAY_PORT,
# GATEWAY_HOST, OTEL_COLLECTOR_PORT and AMP_ADMIN_PASSWORD. Sign-in uses the
# admin account, so the stack must run with AUTH_MODE=file. The token is never
# printed. Exits non-zero if any check fails.
set -uo pipefail

DIST=${1:-}
CONSOLE=yes
[ "${2:-}" = "--no-console" ] && CONSOLE=no
case "$DIST" in
  amp | amp-core) ;;
  *) echo "usage: $0 amp|amp-core [--no-console]" >&2; exit 2 ;;
esac
for tool in curl jq; do
  command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 2; }
done

ENV_FILE="$(cd "$(dirname "$0")/.." && pwd)/.env"
setting() { # NAME DEFAULT: the environment, else .env, else DEFAULT
  local v="${!1:-}"
  if [ -z "$v" ] && [ -f "$ENV_FILE" ]; then
    v=$(grep -E "^$1=" "$ENV_FILE" | tail -1 | cut -d= -f2-)
  fi
  printf '%s' "${v:-$2}"
}
API_PORT=$(setting AMP_API_PORT 9000)
API=http://localhost:$API_PORT
INT=https://localhost:$(setting AMP_INTERNAL_PORT 9243)
OBS=http://localhost:$(setting OBSERVER_PORT 9098)
GW=http://localhost:$(setting GATEWAY_PORT 19080)
GW_HOST=$(setting GATEWAY_HOST default-default.gateway.localhost)
OTLP=http://localhost:$(setting OTEL_COLLECTOR_PORT 4318)
PASSWORD=$(setting AMP_ADMIN_PASSWORD "")
[ -n "$PASSWORD" ] || { echo "AMP_ADMIN_PASSWORD is not set and not in $ENV_FILE" >&2; exit 2; }

pass=0 fail=0 skipped=0
check() { # NAME WANT GOT
  if [ "$2" = "$3" ]; then echo "PASS $1 ($3)"; pass=$((pass + 1))
  else echo "FAIL $1: want $2, got $3"; fail=$((fail + 1)); fi
}
skip() { echo "SKIP $1"; skipped=$((skipped + 1)); }
BODY=$(mktemp)
trap 'rm -f "$BODY"' EXIT
code() { curl -s -o "$BODY" -w '%{http_code}' "$@"; }
# The value for amp, or for amp-core.
pick() { if [ "$DIST" = amp ]; then echo "$1"; else echo "$2"; fi; }
has() { if grep -q "$1" "$BODY"; then echo yes; else echo no; fi; }
rfc3339() { # seconds offset from now, UTC
  local t=$(($(date +%s) + $1))
  date -u -d "@$t" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -r "$t" +%Y-%m-%dT%H:%M:%SZ
}
hex() { od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'; }

# --- Sign-in ---------------------------------------------------------------------
check health 200 "$(code "$API/healthz")"
[ "$fail" -eq 0 ] || { echo "== $DIST: the API at $API is not up; stopping" >&2; exit 1; }
check jwks 200 "$(code "$API/auth/external/jwks.json")"
TOKEN=$(jq -n --arg p "$PASSWORD" '{username: "admin", password: $p}' \
  | curl -s -X POST "$API/auth/login" -H 'Content-Type: application/json' -d @- \
  | jq -r '.access_token // empty')
if [ -n "$TOKEN" ]; then check login ok ok; else check login ok "no token (is AUTH_MODE file?)"; fi
A=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')

# --- Core API ------------------------------------------------------------------------
check orgs 200 "$(code "${A[@]}" "$API/api/v1/orgs")"
# A fresh install lists its default organization before anything is written to it.
ORG=$(jq -r '.organizations[0].name // empty' "$BODY" 2>/dev/null)
if [ -n "$ORG" ]; then check orgs-listed ok ok; else check orgs-listed ok "none"; fi
ORG=${ORG:-default}
B=$API/api/v1/orgs/$ORG
P=smoke-$RANDOM
AG=ext-$RANDOM
EXT='"provisioning":{"type":"external"},"agentType":{"type":"external-agent-api","subType":"custom-api"}'
check project-create 202 "$(code "${A[@]}" -X POST "$B/projects" -d "{\"name\":\"$P\",\"displayName\":\"Smoke $P\"}")"
check project-get 200 "$(code "${A[@]}" "$B/projects/$P")"
check agent-create 202 "$(code "${A[@]}" -X POST "$B/projects/$P/agents" -d "{\"name\":\"$AG\",\"displayName\":\"Ext $AG\",$EXT}")"
check agent-get 200 "$(code "${A[@]}" "$B/projects/$P/agents/$AG")"
check agent-list 200 "$(code "${A[@]}" "$B/projects/$P/agents")"
check agent-update 200 "$(code "${A[@]}" -X PUT "$B/projects/$P/agents/$AG" -d '{"displayName":"Renamed","description":"smoke"}')"
check agent-config 200 "$(code "${A[@]}" "$B/projects/$P/agents/$AG/configurations?environment=default")"
check agent-token 200 "$(code "${A[@]}" -X POST "$B/projects/$P/agents/$AG/token" -d '{}')"
AGENT_TOKEN=$(jq -r '.token // empty' "$BODY")
check unknown-extension 400 "$(code "${A[@]}" -X POST "$B/projects/$P/agents" -d "{\"name\":\"x$AG\",\"displayName\":\"X\",$EXT,\"extensions\":{\"nosuch\":{}}}")"
# An LLM binding names a provider: amp has none by that name, amp-core no LLM support.
check agent-with-llm-config-refused "$(pick 404 400)" "$(code "${A[@]}" -X POST "$B/projects/$P/agents" -d "{\"name\":\"llm$AG\",\"displayName\":\"L\",$EXT,\"modelConfig\":[{\"providerName\":\"nosuch-provider\"}]}")"
check agent-llm-not-created 404 "$(code "${A[@]}" "$B/projects/$P/agents/llm$AG")"
check environments 200 "$(code "${A[@]}" "$B/environments")"
check identity-roles 200 "$(code "${A[@]}" "$B/identities/roles")"
check events-types 200 "$(code "${A[@]}" "$B/events/event-types")"

# --- Gateway routes: served by amp, absent from amp-core -----------------------------------
check gateways "$(pick 200 404)" "$(code "${A[@]}" "$B/gateways")"
check llm-providers "$(pick 200 404)" "$(code "${A[@]}" "$B/llm-providers")"
check env-gateways "$(pick 200 404)" "$(code "${A[@]}" "$B/environments/default/gateways")"
check agent-deployments "$(pick 200 404)" "$(code "${A[@]}" "$B/projects/$P/agents/$AG/deployments")"
check catalog "$(pick 200 404)" "$(code "${A[@]}" "$B/catalog")"
check mcp-endpoint 401 "$(code -X POST "$API/mcp" -d '{}')"
check ws-internal "$(pick 401 404)" "$(code -k "$INT/api/internal/v1/ws/gateways/connect")"
# Not-yet-implemented routes: answered 501 by amp's legacy bundle, absent from amp-core.
check agent-kinds "$(pick 501 404)" "$(code "${A[@]}" "$B/agent-kinds")"
check monitors "$(pick 501 404)" "$(code "${A[@]}" "$B/projects/$P/agents/$AG/monitors")"
check evaluators "$(pick 200 404)" "$(code "${A[@]}" "$B/evaluators")"
check module-route-404 404 "$(code "${A[@]}" "$API/api/v1/m/testmod/whoami")"
check workload-exchange-bad 401 "$(code -k -X POST "$INT/api/internal/v1/workload-tokens" -d '{"credentialId":"x","secret":"y"}')"

# --- A trace, end to end ----------------------------------------------------------------
# amp: through the gateway's /otel with the agent's token; the gateway labels
# the spans. amp-core: straight to the collector, labelled by the agent.
TRACE_ID=$(hex 16)
NOW_NS="$(date +%s)000000000"
SPAN=$(jq -nc --arg t "$TRACE_ID" --arg s "$(hex 8)" --arg n "$NOW_NS" '{resourceSpans: [{
  resource: {attributes: [{key: "service.name", value: {stringValue: "smoke"}}]},
  scopeSpans: [{scope: {name: "smoke"}, spans: [{traceId: $t, spanId: $s, name: "smoke",
    kind: 1, startTimeUnixNano: $n, endTimeUnixNano: $n}]}]}]}')
if [ "$DIST" = amp ]; then
  sent=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$GW/otel/v1/traces" -H "Host: $GW_HOST" \
    -H "x-amp-api-key: $AGENT_TOKEN" -H 'Content-Type: application/json' -d "$SPAN")
else
  sent=$(curl -s -o /dev/null -w '%{http_code}' -X POST "$OTLP/v1/traces" \
    -H "x-user-organization-name: $ORG" -H "x-user-project-name: $P" -H "x-user-agent-name: $AG" \
    -H 'Content-Type: application/json' -d "$SPAN")
fi
check trace-ingest 200 "$sent"
found=no
for _ in $(seq 1 30); do
  code "${A[@]}" "$OBS/api/v1/traces?organization=$ORG&project=$P&agent=$AG&startTime=$(rfc3339 -3600)&endTime=$(rfc3339 60)" >/dev/null
  if [ "$(has "$TRACE_ID")" = yes ]; then found=yes; break; fi
  sleep 2
done
check trace-queryable yes "$found"

check agent-delete 204 "$(code "${A[@]}" -X DELETE "$B/projects/$P/agents/$AG")"
check agent-gone 404 "$(code "${A[@]}" "$B/projects/$P/agents/$AG")"
check project-delete 204 "$(code "${A[@]}" -X DELETE "$B/projects/$P")"

# --- Console, served by the API ---------------------------------------------------------
if [ "$CONSOLE" = yes ]; then
  check console-index 200 "$(code "$API/")"
  check console-index-html yes "$(has '<div id="root">')"
  ASSET=$(grep -o '/assets/[^"]*\.js' "$BODY" | head -1)
  check console-deep-link 200 "$(code "$API/org/default/projects/x/agents")"
  check console-deep-link-is-index yes "$(has '<div id="root">')"
  check console-hashed-asset "200 public, max-age=31536000, immutable" \
    "$(curl -s -o /dev/null -w '%{http_code} %header{cache-control}' "$API$ASSET")"
  check console-missing-asset 404 "$(code "$API/assets/nosuch.js")"
  check console-api-typo-404 404 "$(code "$API/api/nosuch")"
  check console-config 200 "$(code "$API/config.js")"
  check config-authmode yes "$(has "authMode: 'file'")"
  check config-apibase yes "$(has "apiBaseUrl: '$API'")"
  check config-console-env yes "$(has "gatewayVersion: 'v")"
  check config-scopes-gateway "$(pick yes no)" "$(has 'amp:gateway:read')"
  check config-scopes-core yes "$(has 'amp:agent:create')"
else
  skip "console files (--no-console)"
fi

# --- UI manifest -------------------------------------------------------------------------
check ui-manifest-needs-token 401 "$(code "$API/api/v1/ui-manifest")"
check ui-manifest 200 "$(code "${A[@]}" "$API/api/v1/ui-manifest")"
CORE_PAGES=ai-gateway/external-agent,ai-observability/traces
GW_PAGES=ai-gateway/gateways,ai-gateway/llm-providers,ai-gateway/mcp-proxies,ai-gateway/agent-security,ai-gateway/configure-agent,agent-identity/agent-identity
check ui-manifest-pages "$(pick "$CORE_PAGES,$GW_PAGES" "$CORE_PAGES")" "$(jq -r '.pages | join(",")' "$BODY")"
check ui-manifest-modules "$(pick core,gateway core)" "$(jq -r '[.modules[].name] | join(",")' "$BODY")"

echo "== $DIST: $pass passed, $fail failed, $skipped skipped"
[ "$fail" -eq 0 ]
