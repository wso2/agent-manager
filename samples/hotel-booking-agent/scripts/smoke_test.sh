#!/usr/bin/env bash
# Installs both services, starts them without real API keys, and checks they serve requests.
set -euo pipefail

cd "$(dirname "$0")/.."
PYTHON="${PYTHON:-python3}"
HOTEL_PORT="${HOTEL_PORT:-9091}"
AGENT_PORT="${AGENT_PORT:-8000}"
WORK="$(mktemp -d)"
PIDS=()
cleanup() {
  for pid in "${PIDS[@]}"; do kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; done
  rm -rf "$WORK"
}
trap cleanup EXIT

wait_for() {
  for _ in $(seq 1 60); do
    curl -sf -o /dev/null "$1" && return 0
    sleep 1
  done
  echo "Timed out waiting for $1"
  cat "$2"
  return 1
}

echo "==> Installing hotel API"
"$PYTHON" -m venv "$WORK/hotel"
"$WORK/hotel/bin/pip" install -q -r services/hotel_api/requirements.txt

echo "==> Starting hotel API"
env -u OPENAI_API_KEY -u PINECONE_API_KEY \
  "$WORK/hotel/bin/python" -m uvicorn service:app --app-dir services/hotel_api --port "$HOTEL_PORT" \
  > "$WORK/hotel.log" 2>&1 &
PIDS+=($!)
wait_for http://127.0.0.1:$HOTEL_PORT/health "$WORK/hotel.log"

status=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$HOTEL_PORT/hotels/mock-002-tokyo/policies/search?q=pets")
[ "$status" = "503" ] || { echo "Expected 503 from unconfigured policy search, got $status"; exit 1; }

echo "==> Installing agent"
"$PYTHON" -m venv "$WORK/agent"
"$WORK/agent/bin/pip" install -q -r agent/requirements.txt

echo "==> Starting agent"
OPENAI_API_KEY=smoke-test HOTEL_API_BASE_URL=http://127.0.0.1:$HOTEL_PORT \
  "$WORK/agent/bin/python" -m uvicorn app:app --app-dir agent --port "$AGENT_PORT" \
  > "$WORK/agent.log" 2>&1 &
PIDS+=($!)
wait_for http://127.0.0.1:$AGENT_PORT/openapi.json "$WORK/agent.log"

echo "==> Calling agent tools against the hotel API"
(cd agent && OPENAI_API_KEY=smoke-test HOTEL_API_BASE_URL=http://127.0.0.1:$HOTEL_PORT "$WORK/agent/bin/python" - <<'EOF'
import tools

hotels = tools.search_hotels_tool.invoke({"destination": "Tokyo"})
assert hotels.get("hotels"), hotels
policy = tools.query_hotel_policy_tool.invoke({"question": "pets?", "hotel_id": "mock-002-tokyo"})
assert policy["found"] is False and policy.get("note"), policy
print(f"search returned {len(hotels['hotels'])} hotels; policy tool handled unavailable search")
EOF
)

echo "==> Smoke test passed"
