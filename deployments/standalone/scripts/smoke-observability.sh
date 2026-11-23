#!/usr/bin/env bash
# smoke-observability.sh — smoke-test traces on a stack started with
# `make up-observability` or `make dev-up-observability`.
#
#   scripts/smoke-observability.sh
#
# Checks that ClickHouse and the collector run with no published ports, on the
# internal observability network with amp-api alone; that amp migrate created
# the span table; and that the observability module is installed. Then, from
# the amp-api container, as the module forwards spans, it posts a span whose
# attributes name another org: it must be stored under the org in x-org-id and
# read back through the module's trace route. A span with no x-org-id must not
# be stored.
#
# The API's host port is AMP_API_PORT if set, else the one Compose published
# for amp-api. The admin password is AMP_ADMIN_PASSWORD if set, else the one
# amp-init generated, read from the amp-api container. Sign-in uses the admin
# account, so the stack must run with file sign-in (auth.mode: file). Exits
# non-zero if any check fails.
set -uo pipefail

for tool in curl jq docker; do
  command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 2; }
done

DIR="$(cd "$(dirname "$0")/.." && pwd)"

pass=0 fail=0 skipped=0
check() { # NAME WANT GOT
  if [ "$2" = "$3" ]; then echo "PASS $1 ($3)"; pass=$((pass + 1))
  else echo "FAIL $1: want $2, got $3"; fail=$((fail + 1)); fi
}
skip() { echo "SKIP $1"; skipped=$((skipped + 1)); }
dc() { docker compose --project-directory "$DIR" --profile observability "$@"; }
ch() { dc exec -T clickhouse clickhouse-client -q "$1"; }
# amp CLI in amp-api: the release binary, or on the dev stack the one air builds.
amp_cli() {
  dc exec -T amp-api sh -c 'if [ -x /go/bin/amp ]; then exec /go/bin/amp "$@"; fi
    exec su-exec 1000:1000 /app/tmp/agent-manager-service "$@"' amp "$@"
}
hex() { od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'; }

# --- Containers and network ----------------------------------------------------------
CH=$(dc ps -q --status running clickhouse)
COL=$(dc ps -q --status running otel-collector)
API_ID=$(dc ps -q --status running amp-api)
if [ -z "$CH" ] || [ -z "$COL" ] || [ -z "$API_ID" ]; then
  echo "== amp-api, clickhouse and otel-collector must be running (make up-observability); stopping" >&2
  exit 1
fi
PASSWORD=${AMP_ADMIN_PASSWORD:-}
[ -n "$PASSWORD" ] || PASSWORD=$(dc exec -T amp-api cat /var/lib/agent-manager/secrets/admin-password 2>/dev/null)
[ -n "$PASSWORD" ] || { echo "AMP_ADMIN_PASSWORD is not set and the generated one could not be read" >&2; exit 2; }
PORT=${AMP_API_PORT:-}
if [ -z "$PORT" ]; then
  PORT=$(dc port amp-api 8080 2>/dev/null | head -1)
  PORT=${PORT##*:}
fi
[[ "$PORT" =~ ^[0-9]+$ ]] || { echo "amp-api's host port could not be read; set AMP_API_PORT" >&2; exit 2; }
API=http://localhost:$PORT
check clickhouse-published-ports none "$(docker port "$CH" | head -1 | grep . || echo none)"
check collector-published-ports none "$(docker port "$COL" | head -1 | grep . || echo none)"
PROJECT=$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$CH")
NET=${PROJECT}_observability
nets() { docker inspect -f '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' "$1" | xargs; }
check clickhouse-networks "$NET" "$(nets "$CH")"
check collector-networks "$NET" "$(nets "$COL")"
check network-internal true "$(docker network inspect -f '{{.Internal}}' "$NET")"
members=$(for id in $(docker network inspect -f '{{range $id, $c := .Containers}}{{$id}} {{end}}' "$NET"); do
  docker inspect -f '{{index .Config.Labels "com.docker.compose.service"}}' "$id"
done | sort | xargs)
check network-members "amp-api clickhouse otel-collector" "$members"

# From the host: nothing answers on the ClickHouse HTTP or collector ports,
# unless a container outside this stack publishes them.
for port in 8123 4318 13133; do
  curl -s -o /dev/null --max-time 3 "http://localhost:$port/"
  rc=$?
  if [ "$rc" -eq 7 ]; then check "host-port-$port" refused refused
  else
    owner=$(docker ps --filter "publish=$port" --format '{{.Names}}' | head -1)
    case "$owner" in
      "") check "host-port-$port" refused "curl exit $rc" ;;
      "$PROJECT"-*) check "host-port-$port" refused "published by $owner" ;;
      *) skip "host-port-$port (published by $owner, outside this stack)" ;;
    esac
  fi
done
check collector-health-from-amp-api ok \
  "$(dc exec -T amp-api wget -qO /dev/null http://otel-collector:13133/ && echo ok || echo unreachable)"

# --- Schema and module ---------------------------------------------------------------------
check span-table 1 "$(ch 'EXISTS TABLE otel.otel_traces')"
check schema-history 001_otel_traces "$(ch 'SELECT version FROM otel.amp_schema_migrations FINAL ORDER BY version' | xargs)"
check module-state active "$(amp_cli module list | awk '$1 == "observability" {print $3}')"

# --- Ingest ----------------------------------------------------------------------------------
TOKEN=$(jq -n --arg p "$PASSWORD" '{username: "admin", password: $p}' \
  | curl -s -X POST "$API/auth/login" -H 'Content-Type: application/json' -d @- \
  | jq -r '.access_token // empty')
if [ -z "$TOKEN" ]; then check login ok "no token (is auth.mode file?)"; echo "== stopping"; exit 1; fi
# The org the module scopes reads to: the token's ouId claim.
payload=$(cut -d. -f2 <<<"$TOKEN" | tr '_-' '/+')
while [ $((${#payload} % 4)) -ne 0 ]; do payload="$payload="; done
ORG=$(base64 -d <<<"$payload" 2>/dev/null | jq -r '.ouId // empty')
[ -n "$ORG" ] || { check org-id ok none; exit 1; }

# span TRACE SPAN: an OTLP/JSON span whose resource claims another org.
span() {
  local now; now=$(date +%s)
  jq -nc --arg t "$1" --arg s "$2" --arg start "${now}000000000" --arg end "${now}500000000" '{resourceSpans: [{
    resource: {attributes: [
      {key: "service.name", value: {stringValue: "smoke"}},
      {key: "agent-manager/org-id", value: {stringValue: "spoofed-org"}}]},
    scopeSpans: [{spans: [{traceId: $t, spanId: $s, name: "smoke", kind: 1,
      startTimeUnixNano: $start, endTimeUnixNano: $end}]}]}]}'
}
# post BODY [HEADER...]: from amp-api to the collector, as the module forwards.
post() {
  local body=$1; shift
  local h=(--header 'Content-Type: application/json')
  for x in "$@"; do h+=(--header "$x"); done
  dc exec -T amp-api wget -qO /dev/null "${h[@]}" --post-data "$body" http://otel-collector:4318/v1/traces \
    && echo ok || echo failed
}
# stored TRACE: the org of the trace's stored spans, waiting up to 30 seconds.
stored() {
  local got=""
  for _ in $(seq 30); do
    got=$(ch "SELECT org_id FROM otel.otel_traces WHERE TraceId = '$1'" | xargs)
    [ -n "$got" ] && break
    sleep 1
  done
  printf '%s' "${got:-none}"
}

T1=$(hex 16) S1=$(hex 8) T2=$(hex 16) T3=$(hex 16)
check post-with-org ok "$(post "$(span "$T1" "$S1")" "x-org-id: $ORG")"
check stored-under-header-org "$ORG" "$(stored "$T1")"
check post-without-org ok "$(post "$(span "$T2" "$(hex 8)")")"
# A later span with an org is stored once the earlier one would have been.
post "$(span "$T3" "$(hex 8)")" "x-org-id: $ORG" >/dev/null
stored "$T3" >/dev/null
check no-org-not-stored 0 "$(ch "SELECT count() FROM otel.otel_traces WHERE TraceId = '$T2'")"

WINDOW="startTime=$(jq -nr 'now - 3600 | todate')&endTime=$(jq -nr 'now + 3600 | todate')"
BODY=$(curl -s -w '\n%{http_code}' -H "Authorization: Bearer $TOKEN" "$API/m/observability/traces/$T1/spans?$WINDOW")
check trace-route 200 "$(tail -1 <<<"$BODY")"
check trace-route-has-span yes "$(grep -q "$S1" <<<"$BODY" && echo yes || echo no)"

# --- An agent's export through the module ---------------------------------------------------
# An external agent's key, then a span sent to the module with a spoofed org
# header and attribute: it is stored under the key's org and agent.
A=(-H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json')
HANDLE=$(curl -s "${A[@]}" "$API/api/v1/orgs" | jq -r '.organizations[0].name // empty')
B=$API/api/v1/orgs/${HANDLE:-default}
P=obs-$RANDOM AG=obs-agent-$RANDOM
EXT='"provisioning":{"type":"external"},"agentType":{"type":"external-agent-api","subType":"custom-api"}'
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
check project-create 202 "$(code "${A[@]}" -X POST "$B/projects" -d "{\"name\":\"$P\",\"displayName\":\"$P\"}")"
check agent-create 202 "$(code "${A[@]}" -X POST "$B/projects/$P/agents" -d "{\"name\":\"$AG\",\"displayName\":\"$AG\",$EXT}")"
KEY=$(curl -s "${A[@]}" -X POST "$B/projects/$P/agents/$AG/token" -d '{}' | jq -r '.token // empty')
[ -n "$KEY" ] || { check agent-key ok none; exit 1; }
INGEST=$API/m/observability/otel/v1/traces
T4=$(hex 16)
check export-without-key 401 "$(code -X POST "$INGEST" -H 'Content-Type: application/json' -d "$(span "$(hex 16)" "$(hex 8)")")"
check export-with-key 200 "$(code -X POST "$INGEST" -H 'Content-Type: application/json' \
  -H "X-Amp-Api-Key: $KEY" -H 'x-org-id: spoofed-org' -d "$(span "$T4" "$(hex 8)")")"
check export-stored-under-key-org "$ORG" "$(stored "$T4")"
check export-stored-under-agent "$AG" \
  "$(ch "SELECT ResourceAttributes['agent-manager/agent'] FROM otel.otel_traces WHERE TraceId = '$T4' LIMIT 1" | xargs)"

echo "== observability: $pass passed, $fail failed, $skipped skipped"
[ "$fail" -eq 0 ]
