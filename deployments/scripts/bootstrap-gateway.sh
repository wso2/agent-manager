#!/bin/sh
# bootstrap-gateway.sh
# Registers the AI gateway in Agent Manager, generates a registration token,
# and writes it to a shared file so the gateway-controller can read it at startup.
#
# Environment variables (set in docker-compose or Helm Job):
#   AMP_API_URL          - Agent Manager API base URL (e.g. http://agent-manager-service:9000/api/v1)
#   IDP_TOKEN_URL        - Thunder IDP token endpoint
#   IDP_CLIENT_ID        - OAuth2 client ID
#   IDP_CLIENT_SECRET    - OAuth2 client secret
#   ORG_NAME             - Organization name in Agent Manager
#   GATEWAY_NAME         - Logical name for the gateway (lowercase alphanumeric + hyphens)
#   GATEWAY_DISPLAY_NAME - Human-readable display name
#   GATEWAY_VHOST        - Virtual host (FQDN or IP) the gateway is reachable on
#   GATEWAY_TYPE         - Gateway type (must be "ai")
#   TOKEN_FILE           - Path to write the plaintext token (e.g. /shared/gateway-token)

set -eu

log_info()  { echo "[INFO]  $*"; }
log_error() { echo "[ERROR] $*" >&2; }

# Install curl if not present (alpine base image)
if ! command -v curl >/dev/null 2>&1; then
  apk add --no-cache curl
fi

# -------------------------------------------------------------------------
# Step 1: Wait for Agent Manager API to be healthy
# -------------------------------------------------------------------------
HEALTH_URL="${AMP_API_URL%/api/v1}/healthz"
log_info "Waiting for Agent Manager API at ${HEALTH_URL}..."
MAX_WAIT=120
ELAPSED=0
until curl -sf "${HEALTH_URL}" -o /dev/null; do
  if [ "${ELAPSED}" -ge "${MAX_WAIT}" ]; then
    log_error "Timed out waiting for Agent Manager API after ${MAX_WAIT}s"
    exit 1
  fi
  sleep 5
  ELAPSED=$((ELAPSED + 5))
done
log_info "Agent Manager API is healthy."

# -------------------------------------------------------------------------
# Step 2: Obtain a JWT from Thunder IDP using client_credentials
# -------------------------------------------------------------------------
log_info "Obtaining JWT from Thunder IDP at ${IDP_TOKEN_URL}..."
TOKEN_RESPONSE=$(curl -sf \
  --max-time 30 \
  --retry 5 \
  --retry-delay 5 \
  -X POST "${IDP_TOKEN_URL}" \
  -d "grant_type=client_credentials&client_id=${IDP_CLIENT_ID}&client_secret=${IDP_CLIENT_SECRET}")

ACCESS_TOKEN=$(echo "${TOKEN_RESPONSE}" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)
if [ -z "${ACCESS_TOKEN}" ]; then
  log_error "Failed to obtain access token from IDP. Response: ${TOKEN_RESPONSE}"
  exit 1
fi
log_info "JWT obtained successfully."

# -------------------------------------------------------------------------
# Step 3: Check if the gateway already exists (idempotency)
# -------------------------------------------------------------------------
log_info "Checking if gateway '${GATEWAY_NAME}' already exists in org '${ORG_NAME}'..."
LIST_RESPONSE=$(curl -sf \
  --max-time 30 \
  --retry 3 \
  --retry-delay 5 \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  "${AMP_API_URL}/orgs/${ORG_NAME}/gateways?limit=100")

# Extract gateway ID by matching on name field.
# Check "id" first; fall back to "uuid" to match Helm bootstrap contract.
GATEWAY_ID=$(echo "${LIST_RESPONSE}" | \
  grep -o '"name":"'"${GATEWAY_NAME}"'"[^}]*"id":"[^"]*"\|"id":"[^"]*"[^}]*"name":"'"${GATEWAY_NAME}"'"' | \
  grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "${GATEWAY_ID}" ]; then
  GATEWAY_ID=$(echo "${LIST_RESPONSE}" | \
    grep -o '"name":"'"${GATEWAY_NAME}"'"[^}]*"uuid":"[^"]*"\|"uuid":"[^"]*"[^}]*"name":"'"${GATEWAY_NAME}"'"' | \
    grep -o '"uuid":"[^"]*"' | head -1 | cut -d'"' -f4)
fi

if [ -n "${GATEWAY_ID}" ]; then
  log_info "Gateway '${GATEWAY_NAME}' already exists with ID: ${GATEWAY_ID}"
else
  # -------------------------------------------------------------------------
  # Step 4: Fetch environments and pick the first one
  # -------------------------------------------------------------------------
  log_info "Fetching environments for org '${ORG_NAME}'..."
  ENV_RESPONSE=$(curl -sf \
    --max-time 30 \
    --retry 3 \
    --retry-delay 5 \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    "${AMP_API_URL}/orgs/${ORG_NAME}/environments")

  ENVIRONMENT_ID=$(echo "${ENV_RESPONSE}" | \
    grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

  if [ -z "${ENVIRONMENT_ID}" ]; then
    log_error "No environments found for org '${ORG_NAME}'. Cannot register gateway."
    exit 1
  fi
  log_info "Using environment ID: ${ENVIRONMENT_ID}"

  # -------------------------------------------------------------------------
  # Step 5: Register the gateway with the environment
  # -------------------------------------------------------------------------
  log_info "Registering gateway '${GATEWAY_NAME}'..."
  REGISTER_RESPONSE=$(curl -sf \
    --max-time 30 \
    --retry 3 \
    --retry-delay 5 \
    -X POST \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    "${AMP_API_URL}/orgs/${ORG_NAME}/gateways" \
    -d '{
      "name": "'"${GATEWAY_NAME}"'",
      "displayName": "'"${GATEWAY_DISPLAY_NAME}"'",
      "gatewayType": "'"${GATEWAY_TYPE}"'",
      "vhost": "'"${GATEWAY_VHOST}"'",
      "environmentIds": ["'"${ENVIRONMENT_ID}"'"]
    }')

  GATEWAY_ID=$(echo "${REGISTER_RESPONSE}" | \
    grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)

  if [ -z "${GATEWAY_ID}" ]; then
    log_error "Failed to register gateway. Response: ${REGISTER_RESPONSE}"
    exit 1
  fi
  log_info "Gateway registered with ID: ${GATEWAY_ID}"
fi

# -------------------------------------------------------------------------
# Step 6: Check if token file already exists (skip token rotation on restart
# to avoid invalidating a running gateway-controller)
# -------------------------------------------------------------------------
if [ -s "${TOKEN_FILE}" ]; then
  log_info "Token file '${TOKEN_FILE}' already exists and is non-empty — skipping token rotation."
  log_info "Bootstrap complete. Gateway '${GATEWAY_NAME}' ID: ${GATEWAY_ID}"
  exit 0
fi

# -------------------------------------------------------------------------
# Step 7: Generate a gateway registration token
# -------------------------------------------------------------------------
log_info "Generating registration token for gateway '${GATEWAY_ID}'..."
ROTATE_RESPONSE=$(curl -sf \
  --max-time 30 \
  --retry 3 \
  --retry-delay 5 \
  -X POST \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  "${AMP_API_URL}/orgs/${ORG_NAME}/gateways/${GATEWAY_ID}/tokens")

GATEWAY_TOKEN=$(echo "${ROTATE_RESPONSE}" | \
  grep -o '"token":"[^"]*"' | head -1 | cut -d'"' -f4)

if [ -z "${GATEWAY_TOKEN}" ]; then
  log_error "Failed to generate gateway token. Response: ${ROTATE_RESPONSE}"
  exit 1
fi
log_info "Gateway token generated successfully."

# -------------------------------------------------------------------------
# Step 8: Write the token to the shared volume (atomic, mode 0600)
# -------------------------------------------------------------------------
mkdir -p "$(dirname "${TOKEN_FILE}")"
TOKEN_TMP=$(mktemp "$(dirname "${TOKEN_FILE}")/.token.XXXXXX")
chmod 600 "${TOKEN_TMP}"
printf '%s' "${GATEWAY_TOKEN}" > "${TOKEN_TMP}"
mv "${TOKEN_TMP}" "${TOKEN_FILE}"
log_info "Token written to '${TOKEN_FILE}'."

log_info "Bootstrap complete. Gateway '${GATEWAY_NAME}' registered with ID: ${GATEWAY_ID}"
