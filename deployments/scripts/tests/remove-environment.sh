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
# .body, or <method>-<segment>.<n>.code and .body for the n-th such call only.
# A missing .code file means 404, and a code of 000 fails like a refused
# connection. Every call is appended to $FAKE/calls with its arguments.
mkdir -p "$TMP/bin"
cat > "$TMP/bin/curl" <<'FAKE_CURL'
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
n=$(( $(cat "$FAKE/$key.count" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "$FAKE/$key.count"
[ -f "$FAKE/$key.$n.code" ] && key="$key.$n"
code=404
[ -f "$FAKE/$key.code" ] && code=$(cat "$FAKE/$key.code")
if [ "$code" = 000 ]; then printf '000'; exit 7; fi
body=""
[ -f "$FAKE/$key.body" ] && body=$(cat "$FAKE/$key.body")
if [ -n "$out" ]; then printf '%s' "$body" > "$out"; else printf '%s' "$body"; fi
printf '%s' "$code"
FAKE_CURL
# No helm releases and no namespaces: Steps 3 and 4 take their skip paths.
printf '#!/usr/bin/env bash\nexit 1\n' > "$TMP/bin/helm"
cat > "$TMP/bin/kubectl" <<'FAKE_KUBECTL'
#!/usr/bin/env bash
case "$1" in version|wait) exit 0 ;; *) exit 1 ;; esac
FAKE_KUBECTL
chmod +x "$TMP/bin/"*

# A gateway response as the API serializes it: keys in alphabetical order.
gateway_body() {
    local name="$1" uuid="$2" envs="$3"
    printf '{"createdAt":"t","displayName":"d","environments":%s,"gatewayType":"BOTH","isCritical":false,"name":"%s","status":"ACTIVE","updatedAt":"t","uuid":"%s","vhost":"v"}' \
        "$envs" "$name" "$uuid"
}
# Environment entries carry their own uuid, which the DELETE must not pick up.
STAGING='[{"createdAt":"t","displayName":"Staging","name":"staging","uuid":"env-1"}]'
PROD='[{"createdAt":"t","displayName":"Prod","name":"prod","uuid":"env-2"}]'

# new_case NAME — empties $FAKE and makes the environment DELETE in Step 1 succeed.
new_case() {
    CASE="$1"
    FAKE="$TMP/fake-$1"
    rm -rf "$FAKE"; mkdir -p "$FAKE"; : > "$FAKE/calls"
    echo 204 > "$FAKE/DELETE-staging.code"
}

# stub METHOD SEGMENT CODE [BODY] — the answer to every such call.
stub() {
    echo "$3" > "$FAKE/$1-$2.code"
    if [ -n "${4:-}" ]; then printf '%s' "$4" > "$FAKE/$1-$2.body"; fi
}

# stub_first METHOD SEGMENT CODE [BODY] — the answer to the first such call only.
stub_first() {
    echo "$3" > "$FAKE/$1-$2.1.code"
    if [ -n "${4:-}" ]; then printf '%s' "$4" > "$FAKE/$1-$2.1.body"; fi
}

# stub_owned NAME UUID — listed under staging before Step 1, and under nothing
# after it, as the environment DELETE drops the mapping.
stub_owned() {
    stub_first GET "$1" 200 "$(gateway_body "$1" "$2" "$STAGING")"
    stub GET "$1" 200 "$(gateway_body "$1" "$2" '[]')"
}

run_script() {
    OUT=$(FAKE="$FAKE" PATH="$TMP/bin:$PATH" ENV_NAME=staging AGENT_MANAGER_TOKEN=test-token \
        DEPROVISION_THUNDER=false bash "$SCRIPT" 2>&1)
    assert_eq "${CASE}: the script exits 0" 0 "$?"
    CALLS=$(cat "$FAKE/calls")
    GATEWAY_CALLS=$(grep '/gateways/' "$FAKE/calls")
}

# --- Cases ---

new_case owned
stub_owned "$GW" "$GW_UUID"
stub DELETE "$GW_UUID" 204
run_script
assert_contains "a gateway listed under this environment is deregistered" "Gateway '${GW}' deregistered" "$OUT"
assert_contains "the DELETE targets the gateway UUID" "-X DELETE http://api.amp.localhost:8080/api/v1/orgs/default/gateways/${GW_UUID}" "$CALLS"
assert_eq "the owner is read before the environment DELETE" "gateways/${GW}" \
    "$(grep -o 'gateways/[^ ]*\|environments/staging' "$FAKE/calls" | head -1)"
assert_contains "the script completes" "=== Environment 'staging' removed ===" "$OUT"

# The API returns an empty list when it cannot read the mappings, so an empty
# list is no proof the gateway is unused: a shared gateway would be deleted.
new_case empty-list
stub GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" '[]')"
run_script
assert_contains "a gateway listed under no environment is kept" "was not listed under 'staging', leaving it registered" "$OUT"
assert_not_contains "an unlisted gateway gets no DELETE" "-X DELETE" "$GATEWAY_CALLS"

new_case shared
stub GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" "$PROD")"
run_script
assert_contains "a gateway listed under another environment is kept" "was not listed under 'staging', leaving it registered" "$OUT"
assert_not_contains "a shared gateway gets no DELETE" "-X DELETE" "$GATEWAY_CALLS"

new_case reassigned
stub_first GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" "$STAGING")"
stub GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" "$PROD")"
run_script
assert_contains "a gateway assigned elsewhere during the removal is kept" "is now mapped to another environment" "$OUT"
assert_not_contains "a reassigned gateway gets no DELETE" "-X DELETE" "$GATEWAY_CALLS"

new_case deployments
stub_owned "$GW" "$GW_UUID"
stub DELETE "$GW_UUID" 409 '{"message":"gateway has active deployments","code":"CONFLICT"}'
run_script
assert_contains "a 409 on DELETE reports the API deployments" "still has API deployments" "$OUT"
assert_contains "the script completes after a 409" "=== Environment 'staging' removed ===" "$OUT"

new_case unregistered
run_script
assert_contains "a 404 lookup says the gateway is not registered" "Gateway '${GW}' is not registered, skipping" "$OUT"
assert_not_contains "a missing gateway gets no DELETE" "-X DELETE" "$GATEWAY_CALLS"

# A single-topology environment has no egress gateway, and its lookup must not
# print a "not registered" line for something that was never meant to exist.
assert_not_contains "an absent egress gateway is skipped quietly" "${EGRESS}'" "$OUT"

new_case split
stub_owned "$GW" "$GW_UUID"
stub DELETE "$GW_UUID" 204
stub_owned "$EGRESS" "$EGRESS_UUID"
stub DELETE "$EGRESS_UUID" 204
run_script
assert_contains "a split topology deregisters the ingress gateway" "Gateway '${GW}' deregistered" "$OUT"
assert_contains "a split topology deregisters the egress gateway" "Gateway '${EGRESS}' deregistered" "$OUT"

new_case unauthorized
stub GET "$GW" 401 '{"message":"Unauthorized","code":"UNAUTHORIZED"}'
run_script
assert_contains "a 401 lookup warns with the status" "Could not look up gateway '${GW}' before the removal (HTTP 401)" "$OUT"
assert_not_contains "a 401 lookup gets no DELETE" "-X DELETE" "$GATEWAY_CALLS"
assert_contains "the script completes after a 401" "=== Environment 'staging' removed ===" "$OUT"

new_case no-uuid
stub_first GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" "$STAGING")"
stub GET "$GW" 200 '{"environments":[],"name":"api-platform-default-staging"}'
run_script
assert_contains "a response without a UUID is reported" "Could not read the ID of gateway '${GW}'" "$OUT"
assert_not_contains "a response without a UUID gets no DELETE" "-X DELETE" "$GATEWAY_CALLS"

new_case refused
stub GET "$GW" 000
run_script
assert_contains "a refused lookup warns with HTTP 000" "Could not look up gateway '${GW}' before the removal (HTTP 000)" "$OUT"
assert_contains "the script completes after a refused lookup" "=== Environment 'staging' removed ===" "$OUT"

new_case lookup-refused-after
stub_first GET "$GW" 200 "$(gateway_body "$GW" "$GW_UUID" "$STAGING")"
stub GET "$GW" 000
run_script
assert_contains "a refused second lookup warns with HTTP 000" "Could not look up gateway '${GW}' (HTTP 000)" "$OUT"
assert_not_contains "a refused second lookup gets no DELETE" "-X DELETE" "$GATEWAY_CALLS"

new_case delete-refused
stub_owned "$GW" "$GW_UUID"
stub DELETE "$GW_UUID" 000
run_script
assert_contains "a refused DELETE warns with HTTP 000" "Failed to deregister gateway '${GW}' (HTTP 000)" "$OUT"
assert_contains "the script completes after a refused DELETE" "=== Environment 'staging' removed ===" "$OUT"

# Every gateway request is bounded, so a stalled connection reaches the
# warnings above instead of hanging the removal.
new_case bounded
stub_owned "$GW" "$GW_UUID"
stub DELETE "$GW_UUID" 204
run_script
# Four gateway requests: both owner lookups, then the gateway's lookup and DELETE.
assert_eq "every gateway request is made" 4 "$(printf '%s\n' "$GATEWAY_CALLS" | grep -c .)"
assert_eq "every gateway request carries --max-time" 4 \
    "$(printf '%s\n' "$GATEWAY_CALLS" | grep -c -- '--connect-timeout 10 --max-time 30')"

echo
if [ "$FAILURES" -gt 0 ]; then
    echo "${FAILURES} failure(s)"
    exit 1
fi
echo "all passed"
