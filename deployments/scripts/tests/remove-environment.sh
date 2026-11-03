#!/usr/bin/env bash
# Tests for the gateway deregistration in deployments/scripts/remove-environment.sh.
# Run: bash deployments/scripts/tests/remove-environment.sh
#
# Runs the whole script against fake curl, kubectl and helm, so the JSON
# parsing and HTTP branches of Step 3b are exercised without a cluster. Every
# case must also finish the script: the environment is already deleted by
# Step 3b, so a gateway failure that aborted it would skip the namespace cleanup.
set -uo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/remove-environment.sh"
FAILURES=0
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

GW="api-platform-default-staging"
EGRESS="${GW}-egress"
GW_UUID="11111111-2222-3333-4444-555555555555"
EGRESS_UUID="66666666-7777-8888-9999-000000000000"

assert_eq() {
    local label="$1" expected="$2" actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        printf 'ok   - %s\n' "$label"
    else
        printf 'FAIL - %s\n      expected: %s\n      actual:   %s\n' "$label" "$expected" "$actual"
        FAILURES=$((FAILURES + 1))
    fi
}

assert_contains() {
    local label="$1" needle="$2" haystack="$3"
    if [[ "$haystack" == *"$needle"* ]]; then
        printf 'ok   - %s\n' "$label"
    else
        printf 'FAIL - %s\n      missing: %s\n      output:\n%s\n' "$label" "$needle" "$haystack"
        FAILURES=$((FAILURES + 1))
    fi
}

assert_not_contains() {
    local label="$1" needle="$2" haystack="$3"
    if [[ "$haystack" != *"$needle"* ]]; then
        printf 'ok   - %s\n' "$label"
    else
        printf 'FAIL - %s\n      unexpected: %s\n      output:\n%s\n' "$label" "$needle" "$haystack"
        FAILURES=$((FAILURES + 1))
    fi
}

# --- Fakes ---
# curl answers from files under $FAKE: <method>-<last path segment>.code and
# .body. A missing .code file means 404, and a code of 000 fails like a refused
# connection. Every call is appended to $FAKE/calls with its arguments.
mkdir -p "$TMP/bin"
cat > "$TMP/bin/curl" <<'EOF'
#!/usr/bin/env bash
out="" method=GET url=""
echo "$*" >> "$FAKE/calls"
while [ $# -gt 0 ]; do
    case "$1" in
        -o) out="$2"; shift ;;
        -X) method="$2"; shift ;;
        -w|-H|--connect-timeout|--max-time) shift ;;
        http*) url="$1" ;;
    esac
    shift
done
case "$url" in */healthz) exit 0 ;; esac
key="${method}-${url##*/}"
code=404
[ -f "$FAKE/$key.code" ] && code=$(cat "$FAKE/$key.code")
if [ "$code" = 000 ]; then printf '000'; exit 7; fi
body=""
[ -f "$FAKE/$key.body" ] && body=$(cat "$FAKE/$key.body")
if [ -n "$out" ]; then printf '%s' "$body" > "$out"; else printf '%s' "$body"; fi
printf '%s' "$code"
EOF
# No helm releases and no namespaces: Steps 3 and 4 take their skip paths.
printf '#!/usr/bin/env bash\nexit 1\n' > "$TMP/bin/helm"
cat > "$TMP/bin/kubectl" <<'EOF'
#!/usr/bin/env bash
case "$1" in version|wait) exit 0 ;; *) exit 1 ;; esac
EOF
chmod +x "$TMP/bin/"*

# A gateway response as the API serializes it: keys in alphabetical order.
gateway_body() {
    local name="$1" uuid="$2" envs="$3"
    printf '{"createdAt":"t","displayName":"d","environments":%s,"gatewayType":"BOTH","isCritical":false,"name":"%s","status":"ACTIVE","updatedAt":"t","uuid":"%s","vhost":"v"}' \
        "$envs" "$name" "$uuid"
}

# new_case NAME — empties $FAKE and makes the environment DELETE in Step 1 succeed.
new_case() {
    CASE="$1"
    FAKE="$TMP/fake-$1"
    rm -rf "$FAKE"; mkdir -p "$FAKE"; : > "$FAKE/calls"
    echo 204 > "$FAKE/DELETE-staging.code"
}

# stub METHOD SEGMENT CODE [BODY]
stub() {
    echo "$3" > "$FAKE/$1-$2.code"
    if [ -n "${4:-}" ]; then printf '%s' "$4" > "$FAKE/$1-$2.body"; fi
}

run_script() {
    OUT=$(FAKE="$FAKE" PATH="$TMP/bin:$PATH" ENV_NAME=staging AGENT_MANAGER_TOKEN=test-token \
        DEPROVISION_THUNDER=false bash "$SCRIPT" 2>&1)
    assert_eq "${CASE}: the script exits 0" 0 "$?"
    CALLS=$(cat "$FAKE/calls")
}

# --- Cases ---

new_case orphan
stub GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" '[]')"
stub DELETE "$GW_UUID" 204
run_script
assert_contains "an unmapped gateway is deregistered" "Gateway '${GW}' deregistered" "$OUT"
assert_contains "the DELETE targets the gateway UUID" "-X DELETE http://api.amp.localhost:8080/api/v1/orgs/default/gateways/${GW_UUID}" "$CALLS"
assert_contains "the script completes" "=== Environment 'staging' removed ===" "$OUT"

new_case shared
stub GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" '[{"createdAt":"t","id":"env-2","name":"prod"}]')"
run_script
assert_contains "a gateway mapped to another environment is kept" "still mapped to another environment" "$OUT"
assert_not_contains "a kept gateway gets no DELETE" "-X DELETE" "$(grep gateways "$FAKE/calls")"

new_case deployments
stub GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" '[]')"
stub DELETE "$GW_UUID" 409 '{"message":"gateway has active deployments","code":"CONFLICT"}'
run_script
assert_contains "a 409 on DELETE reports the API deployments" "still has API deployments" "$OUT"
assert_contains "the script completes after a 409" "=== Environment 'staging' removed ===" "$OUT"

new_case unregistered
run_script
assert_contains "a 404 lookup says the gateway is not registered" "Gateway '${GW}' is not registered, skipping" "$OUT"
assert_not_contains "a missing gateway gets no DELETE" "-X DELETE" "$(grep gateways "$FAKE/calls")"

# A single-topology environment has no egress gateway, and its lookup must not
# print a "not registered" line for something that was never meant to exist.
assert_not_contains "an absent egress gateway is skipped quietly" "${EGRESS}' is not registered" "$OUT"

new_case split
stub GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" '[]')"
stub DELETE "$GW_UUID" 204
stub GET "$EGRESS" 200 "$(gateway_body "$EGRESS" "$EGRESS_UUID" '[]')"
stub DELETE "$EGRESS_UUID" 204
run_script
assert_contains "a split topology deregisters the ingress gateway" "Gateway '${GW}' deregistered" "$OUT"
assert_contains "a split topology deregisters the egress gateway" "Gateway '${EGRESS}' deregistered" "$OUT"

new_case unauthorized
stub GET "$GW" 401 '{"message":"Unauthorized","code":"UNAUTHORIZED"}'
run_script
assert_contains "a 401 lookup warns with the status" "Could not look up gateway '${GW}' (HTTP 401)" "$OUT"
assert_not_contains "a 401 lookup gets no DELETE" "-X DELETE" "$(grep gateways "$FAKE/calls")"
assert_contains "the script completes after a 401" "=== Environment 'staging' removed ===" "$OUT"

new_case no-uuid
stub GET "$GW" 200 '{"environments":[],"name":"api-platform-default-staging"}'
run_script
assert_contains "a response without a UUID is reported" "Could not read the ID of gateway '${GW}'" "$OUT"
assert_not_contains "a response without a UUID gets no DELETE" "-X DELETE" "$(grep gateways "$FAKE/calls")"

new_case refused
stub GET "$GW" 000
run_script
assert_contains "a refused connection warns with HTTP 000" "Could not look up gateway '${GW}' (HTTP 000)" "$OUT"
assert_contains "the script completes after a refused connection" "=== Environment 'staging' removed ===" "$OUT"

new_case delete-refused
stub GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" '[]')"
stub DELETE "$GW_UUID" 000
run_script
assert_contains "a refused DELETE warns with HTTP 000" "Failed to deregister gateway '${GW}' (HTTP 000)" "$OUT"
assert_contains "the script completes after a refused DELETE" "=== Environment 'staging' removed ===" "$OUT"

# Both deregistration requests are bounded, so a stalled connection reaches the
# warnings above instead of hanging the namespace cleanup.
new_case bounded
stub GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" '[]')"
stub DELETE "$GW_UUID" 204
run_script
# Three gateway requests: the lookup and DELETE for the gateway, the lookup for egress.
assert_eq "every gateway request carries --max-time" 3 \
    "$(grep '/gateways/' "$FAKE/calls" | grep -c -- '--connect-timeout 10 --max-time 30')"

echo
if [ "$FAILURES" -gt 0 ]; then
    echo "${FAILURES} failure(s)"
    exit 1
fi
echo "all passed"
