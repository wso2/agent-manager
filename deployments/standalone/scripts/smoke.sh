#!/usr/bin/env bash
# smoke.sh — smoke-test a running standalone stack.
#
#   scripts/smoke.sh amp|amp-core|amp-off [--no-console]
#
# amp is the full stack (`make up`), amp-core the core binary without the AI
# Gateway (`make up-core`), and amp-off the full stack with the ai-gateway
# module disabled (`amp module disable ai-gateway`), which must look like
# amp-core. The checks cover sign-in, the core API, which
# routes each distribution serves, the console the API serves and the UI
# manifest. --no-console skips the console files, for an
# image built with CONSOLE=none.
#
# Settings come from the environment, else from .env next to the compose
# files: AMP_API_PORT and AMP_INTERNAL_PORT. The admin password is
# AMP_ADMIN_PASSWORD if set, else the one amp-init generated, read from the
# amp-api container. Sign-in uses the admin account, so the stack must run
# with file sign-in (auth.mode: file). The token is never
# printed. Exits non-zero if any check fails.
set -uo pipefail

DIST=${1:-}
CONSOLE=yes
[ "${2:-}" = "--no-console" ] && CONSOLE=no
case "$DIST" in
  amp | amp-core | amp-off) ;;
  *) echo "usage: $0 amp|amp-core|amp-off [--no-console]" >&2; exit 2 ;;
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
PASSWORD=${AMP_ADMIN_PASSWORD:-}
if [ -z "$PASSWORD" ]; then
  # make runs amp-core as its own project; CI runs either under the file's.
  for project in $(setting COMPOSE_PROJECT_NAME "") amp-lite-core amp-lite; do
    [ "$DIST" != amp-core ] && [ "$project" = amp-lite-core ] && continue
    api=$(docker ps -q --filter "label=com.docker.compose.project=$project" \
      --filter "label=com.docker.compose.service=amp-api" | head -1)
    [ -n "$api" ] && PASSWORD=$(docker exec "$api" cat /var/lib/agent-manager/secrets/admin-password 2>/dev/null)
    [ -n "$PASSWORD" ] && break
  done
fi
[ -n "$PASSWORD" ] || { echo "AMP_ADMIN_PASSWORD is not set and the generated one could not be read" >&2; exit 2; }

pass=0 fail=0 skipped=0
check() { # NAME WANT GOT
  if [ "$2" = "$3" ]; then echo "PASS $1 ($3)"; pass=$((pass + 1))
  else echo "FAIL $1: want $2, got $3"; fail=$((fail + 1)); fi
}
skip() { echo "SKIP $1"; skipped=$((skipped + 1)); }
BODY=$(mktemp)
trap 'rm -f "$BODY"' EXIT
code() { curl -s -o "$BODY" -w '%{http_code}' "$@"; }
# The value for amp, or for amp-core (and amp-off).
pick() { if [ "$DIST" = amp ]; then echo "$1"; else echo "$2"; fi; }
has() { if grep -q "$1" "$BODY"; then echo yes; else echo no; fi; }

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
# The AI Gateway module's API.
GW=$API/m/ai-gateway/orgs/$ORG
# Migrated gateway resources take the org from the token.
GWR=$API/m/ai-gateway
P=smoke-$RANDOM
AG=ext-$RANDOM
# Agents are documents at /api/v1/agents; the project is metadata.project, or ?project= on an agent's own path.
AGS=$API/api/v1/agents
EXT='"provisioning":{"type":"external"},"agentType":{"type":"external-agent-api","subType":"custom-api"}'
agent_doc() { echo "{\"apiVersion\":\"ai-control-plane.choreolabs.com/v1\",\"kind\":\"Agent\",\"metadata\":{\"name\":\"$1\",\"project\":\"$P\"},\"spec\":{\"displayName\":\"$2\",$EXT$3}}"; }
check project-create 202 "$(code "${A[@]}" -X POST "$B/projects" -d "{\"name\":\"$P\",\"displayName\":\"Smoke $P\"}")"
check project-get 200 "$(code "${A[@]}" "$B/projects/$P")"
check agent-create 202 "$(code "${A[@]}" -X POST "$AGS" -d "$(agent_doc "$AG" "Ext $AG")")"
check agent-get 200 "$(code "${A[@]}" "$AGS/$AG?project=$P")"
check agent-list 200 "$(code "${A[@]}" "$AGS?project=$P")"
check agent-update 200 "$(code "${A[@]}" -X PUT "$AGS/$AG?project=$P" -d "$(agent_doc "$AG" "Renamed" ',"description":"smoke"')")"
check agent-token 200 "$(code "${A[@]}" -X POST "$AGS/$AG/token?project=$P" -d '{}')"
if jq -e '.token | type == "string" and length > 0' "$BODY" >/dev/null 2>&1; then check agent-token-issued yes yes
else check agent-token-issued yes no; fi
check unknown-extension 400 "$(code "${A[@]}" -X POST "$AGS" -d "$(agent_doc "x$AG" "X" ',"extensions":{"nosuch":{}}')")"
# An LLM binding goes in the AI Gateway's extension block: amp has no provider by
# that name, and amp-core no ai-gateway module.
check agent-with-llm-config-refused 400 "$(code "${A[@]}" -X POST "$AGS" -d "$(agent_doc "llm$AG" "L" ',"extensions":{"ai-gateway":{"modelConfig":[{"providerName":"nosuch-provider"}]}}')")"
check agent-llm-not-created 404 "$(code "${A[@]}" "$AGS/llm$AG?project=$P")"
check environments 200 "$(code "${A[@]}" "$B/environments")"
check identity-roles 200 "$(code "${A[@]}" "$B/identities/roles")"
check events-types 200 "$(code "${A[@]}" "$B/events/event-types")"

# --- Gateway routes: served by amp, absent from amp-core -----------------------------------
check gateways "$(pick 200 404)" "$(code "${A[@]}" "$GW/gateways")"
check llm-providers "$(pick 200 404)" "$(code "${A[@]}" "$GWR/llm-providers")"
check llm-providers-old-path 404 "$(code "${A[@]}" "$GW/llm-providers")"
check env-gateways "$(pick 200 404)" "$(code "${A[@]}" "$GW/environments/default/gateways")"
check agent-deployments "$(pick 200 404)" "$(code "${A[@]}" "$GWR/agents/$AG/deployments?project=$P")"
check agent-config "$(pick 200 404)" "$(code "${A[@]}" "$GWR/agents/$AG/configurations?project=$P")"
check agent-config-old-path 404 "$(code "${A[@]}" "$GW/projects/$P/agents/$AG/configurations")"
check catalog "$(pick 200 404)" "$(code "${A[@]}" "$GW/catalog")"
# The bundled gateway's controller connects over the internal WebSocket.
if [ "$DIST" = amp ]; then
  connected=no
  for _ in $(seq 30); do
    code "${A[@]}" "$GW/gateways" >/dev/null
    connected=$(has '"status":"ACTIVE"')
    [ "$connected" = yes ] && break
    sleep 2
  done
  check gateway-connected yes "$connected"
fi
# No alias under the core's API.
check gateways-old-path 404 "$(code "${A[@]}" "$B/gateways")"
check mcp-endpoint 401 "$(code -X POST "$API/mcp" -d '{}')"
check ws-internal "$(pick 401 404)" "$(code -k "$INT/api/internal/v1/ws/gateways/connect")"
# Routes of features Lite does not serve are absent from both distributions.
check agent-kinds 404 "$(code "${A[@]}" "$B/agent-kinds")"
# Evaluation is not part of either distribution.
check monitors 404 "$(code "${A[@]}" "$B/projects/$P/agents/$AG/monitors")"
check evaluators 404 "$(code "${A[@]}" "$B/evaluators")"
check module-route-404 404 "$(code "${A[@]}" "$API/m/testmod/whoami")"
# The observability module is installable, not on by default: its trace routes
# are absent until `amp module install observability`.
OBS=$API/m/observability/api/v1/traces
WINDOW="startTime=2026-01-01T00:00:00Z&endTime=2026-01-02T00:00:00Z"
check traces-not-installed 404 "$(code "${A[@]}" "$OBS?project=$P&agent=$AG&$WINDOW")"
check traces-export-not-installed 404 "$(code "${A[@]}" "$OBS/export?project=$P&agent=$AG&$WINDOW")"
check trace-spans-not-installed 404 "$(code "${A[@]}" "$OBS/t/spans?$WINDOW")"
check trace-span-not-installed 404 "$(code "${A[@]}" "$OBS/t/spans/s")"
check workload-heartbeat-bad 401 "$(code -k -X POST "$INT/api/internal/v1/workloads/heartbeat" -d '{"credential":"x.y","protocol":"p/v1"}')"

check agent-delete 204 "$(code "${A[@]}" -X DELETE "$AGS/$AG?project=$P")"
check agent-gone 404 "$(code "${A[@]}" "$AGS/$AG?project=$P")"
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
  check config-scopes-gateway "$(pick yes no)" "$(has 'amp:ai-gateway:gateways.read')"
  check config-scopes-core yes "$(has 'amp:agent:create')"
else
  skip "console files (--no-console)"
fi

# --- UI manifest -------------------------------------------------------------------------
check ui-manifest-needs-token 401 "$(code "$API/api/v1/ui-manifest")"
check ui-manifest 200 "$(code "${A[@]}" "$API/api/v1/ui-manifest")"
CORE_PAGES=acp-core/external-agent
GW_PAGES=ai-gateway/gateways,ai-gateway/llm-providers,ai-gateway/mcp-proxies,ai-gateway/agent-security,ai-gateway/configure-agent,agent-identity/agent-identity
check ui-manifest-pages "$(pick "$CORE_PAGES,$GW_PAGES" "$CORE_PAGES")" "$(jq -r '.pages | join(",")' "$BODY")"
check ui-manifest-modules "$(pick core,ai-gateway core)" "$(jq -r '[.modules[].name] | join(",")' "$BODY")"

echo "== $DIST: $pass passed, $fail failed, $skipped skipped"
[ "$fail" -eq 0 ]
