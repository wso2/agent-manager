#!/usr/bin/env bash
# shellcheck source-path=SCRIPTDIR
set -euo pipefail

# Removes the dedicated Thunder ID instance for ONE environment.
# Mirrors add-environment-thunder.sh — call this before removing the environment itself.
#
# All inputs are provided via environment variables so the script can be piped
# directly into bash:
#
#   curl -fsSL https://raw.githubusercontent.com/wso2/agent-manager/main/deployments/scripts/remove-environment-thunder.sh \
#     | ENV_NAME=staging \
#       bash
#
# Re-running is idempotent: if the release or namespace is already gone the step
# is skipped gracefully.
#
# Required:
#   - ENV_NAME: the environment name (lowercase alphanumeric with hyphens)
# Optional:
#   - ORG_NAME (default: default)
#   Best-effort cleanup of the system-client credential agent-manager-service
#   (AMS) stores in its own database (see add-environment-thunder.sh's
#   store_via_ams) — failure here does NOT abort this script, since environment
#   teardown must succeed even if AMS is unreachable; an orphaned row is
#   harmless (it's only ever read by ouId+env, and a deleted environment's name
#   is not expected to be reused for a Thunder instance with different secrets).
#   AMS looks the row up by this token's own OU ID (never client-supplied), so
#   the DELETE only removes the right row when this token belongs to the same
#   OU add-environment-thunder.sh ran as for this environment.
#   - AMP_API_URL (default: http://localhost:9000/api/v1)
#   - AGENT_MANAGER_TOKEN (default: unset) — if unset, obtains one via
#     client_credentials against IDP_TOKEN_URL/IDP_CLIENT_ID/IDP_CLIENT_SECRET
#     (defaults: http://thunder.amp.localhost:8080/oauth2/token, amp-api-client,
#     amp-api-client-secret) — same identity add-environment-thunder.sh uses.

# ---------------------------------------------------------------------------
# Pure helpers
# ---------------------------------------------------------------------------

validate_name() {
  printf '%s' "${1:-}" | grep -Eq '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'
}

# Load the shared AMS auth helpers (json_escape/get_ams_token) — see
# deployments/scripts/ams-auth.sh. Same prefer-local-sibling,
# fallback-to-curl-fetch pattern as the thunder-naming.sh load below.
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "$(dirname "${BASH_SOURCE[0]}")/ams-auth.sh" ]; then
  # shellcheck source=ams-auth.sh
  source "$(dirname "${BASH_SOURCE[0]}")/ams-auth.sh"
else
  _ams_auth_lib_url="${AMS_AUTH_LIB_URL:-${SCRIPT_BASE_URL:-https://raw.githubusercontent.com/wso2/agent-manager/main/deployments/scripts}/ams-auth.sh}"
  _ams_auth_lib_tmp="$(mktemp)"
  if ! curl -fsSL "${_ams_auth_lib_url}" -o "${_ams_auth_lib_tmp}"; then
    echo "❌ Failed to fetch AMS auth helpers from ${_ams_auth_lib_url}" >&2
    rm -f "${_ams_auth_lib_tmp}"
    exit 1
  fi
  # shellcheck source=/dev/null
  source "${_ams_auth_lib_tmp}"
  rm -f "${_ams_auth_lib_tmp}"
  unset _ams_auth_lib_url _ams_auth_lib_tmp
fi

# Load the shared Thunder naming helpers (thunder_release_name/thunder_namespace)
# — the single source of truth for this derivation, see
# deployments/scripts/thunder-naming.sh. Prefers a local sibling file
# (checked-out repo); falls back to fetching it from the same ref this script
# itself would be fetched from when piped via curl | bash.
if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "$(dirname "${BASH_SOURCE[0]}")/thunder-naming.sh" ]; then
  # shellcheck source=thunder-naming.sh
  source "$(dirname "${BASH_SOURCE[0]}")/thunder-naming.sh"
else
  _naming_lib_url="${THUNDER_NAMING_LIB_URL:-${SCRIPT_BASE_URL:-https://raw.githubusercontent.com/wso2/agent-manager/main/deployments/scripts}/thunder-naming.sh}"
  _naming_lib_tmp="$(mktemp)"
  if ! curl -fsSL "${_naming_lib_url}" -o "${_naming_lib_tmp}"; then
    echo "❌ Failed to fetch Thunder naming helpers from ${_naming_lib_url}" >&2
    rm -f "${_naming_lib_tmp}"
    exit 1
  fi
  # shellcheck source=/dev/null
  source "${_naming_lib_tmp}"
  rm -f "${_naming_lib_tmp}"
  unset _naming_lib_url _naming_lib_tmp
fi

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------
main() {
  : "${ENV_NAME:?ENV_NAME is required (e.g. ENV_NAME=staging)}"

  local org="${ORG_NAME:-default}"

  if ! validate_name "$ENV_NAME"; then
    echo "❌ Invalid ENV_NAME '${ENV_NAME}'"
    echo "   Must be lowercase alphanumeric with hyphens (no leading/trailing hyphen)."
    exit 1
  fi
  if ! validate_name "$org"; then
    echo "❌ Invalid ORG_NAME '${org}'"
    echo "   Must be lowercase alphanumeric with hyphens (no leading/trailing hyphen)."
    exit 1
  fi

  # Namespace is ALWAYS computed from (org, env) — never overridable — so removal
  # targets exactly the namespace add-environment-thunder.sh actually provisioned.
  local release ns
  release="$(thunder_release_name "$org" "$ENV_NAME")"
  ns="$(thunder_namespace "$org" "$ENV_NAME")"

  # An external PostgreSQL (THUNDER_DB_TYPE=postgres in add-environment-thunder.sh) is
  # not ours to drop. Detect it now, while the Secret that records it still exists
  # (it goes away with the namespace), and say so again at the end.
  local external_db=false external_prefix=""
  if [ -n "$(kubectl get secret "${release}-storage-credentials" -n "$ns" -o jsonpath='{.data.db-password}' 2>/dev/null)" ]; then
    external_db=true
    external_prefix="$(kubectl get secret "${release}-storage-credentials" -n "$ns" -o jsonpath='{.data.db-name-prefix}' 2>/dev/null | openssl base64 -d -A 2>/dev/null || true)"
  fi

  echo "=== Removing Thunder ID for environment '${ENV_NAME}' (org '${org}') ==="
  echo ""
  echo "  Release:   ${release}"
  echo "  Namespace: ${ns}"
  echo ""

  # --- Step 1: Uninstall the Thunder helm release ---
  echo "🗑️  Uninstalling Thunder helm release..."
  if helm status "$release" --namespace "$ns" > /dev/null 2>&1; then
    helm uninstall "$release" --namespace "$ns"
    echo "✅ Thunder helm release uninstalled"
  else
    echo "ℹ️  Thunder release '${release}' not found — already removed or never installed"
  fi

  # --- Step 1b: Delete the cross-namespace HTTPRoute ---
  # add-environment-thunder.sh applies this directly via kubectl (not part of the
  # Helm release, since it lives in openchoreo-control-plane rather than the
  # release's own namespace), so it is NOT removed by `helm uninstall` above.
  echo ""
  echo "🗑️  Deleting HTTPRoute in openchoreo-control-plane..."
  if kubectl get httproute "$release" -n openchoreo-control-plane > /dev/null 2>&1; then
    kubectl delete httproute "$release" -n openchoreo-control-plane
    echo "✅ HTTPRoute deleted"
  else
    echo "ℹ️  HTTPRoute '${release}' not found — already removed or never created"
  fi

  # --- Step 2: Delete the Thunder namespace ---
  # The namespace holds the system-client Secret (created outside Helm); deleting
  # the namespace ensures full cleanup even if Helm tracking was incomplete.
  echo ""
  echo "🗑️  Deleting Thunder namespace '${ns}'..."
  if kubectl get namespace "$ns" > /dev/null 2>&1; then
    kubectl delete namespace "$ns"
    echo "✅ Thunder namespace deleted"
  else
    echo "ℹ️  Namespace '${ns}' not found — already deleted"
  fi

  # --- Step 3: Best-effort cleanup of the AMS-stored system-client credential ---
  # Never fatal — environment teardown must succeed even if AMS is unreachable.
  echo ""
  echo "🗑️  Removing system-client credential from agent-manager-service (best-effort)..."
  local amp_api_url="${AMP_API_URL:-http://localhost:9000/api/v1}"
  local access_token
  local system_client_deleted=false
  if access_token="$(get_ams_token 3)"; then
    # AMS looks the credential up by this token's own OU ID (not org_name, and
    # never overridable) — see add-environment-thunder.sh's store_via_ams. The
    # DELETE only targets the right row if this token belongs to the same OU
    # the PUT ran as (true whenever AGENT_MANAGER_TOKEN/IDP_* here match what
    # add-environment-thunder.sh was given for this environment).
    local http_code
    http_code="$(curl -s -o /dev/null -w "%{http_code}" \
      --max-time 30 --retry 3 --retry-delay 5 \
      -X DELETE "${amp_api_url}/orgs/${org}/environments/${ENV_NAME}/thunder-system-client" \
      -H "Authorization: Bearer ${access_token}" 2>/dev/null || true)"
    # curl's own -w already writes "000" when no response is received; falling
    # back with `|| echo "000"` on top of that double-appends into "000000".
    http_code="${http_code:-000}"
    case "$http_code" in
      200|204)
        echo "✅ Removed system-client credential from agent-manager-service"
        system_client_deleted=true
        ;;
      *)
        echo "⚠️  Could not remove the system-client credential from agent-manager-service (HTTP ${http_code})."
        echo "   Harmless to skip — continuing."
        ;;
    esac
  else
    echo "⚠️  Could not obtain a token to clean up the system-client credential in agent-manager-service."
    echo "   Harmless to skip — continuing."
  fi

  # --- Step 4: Best-effort cleanup of the registered thunder url handle ---
  # Never fatal, for the same reason as Step 3. Freeing the handle lets a future
  # environment (or a retry of this same one, with a different THUNDER_HANDLE)
  # claim it — see add-environment-thunder.sh's register_thunder_url. Always
  # attempted even if THUNDER_HANDLE was never set for this environment: deleting
  # a non-existent handle is a no-op on the server side.
  #
  # Only runs once Step 3 has actually succeeded. If the system-client credential
  # is still there but the handle gets freed anyway, a future re-provisioning
  # attempt for this same environment would claim a NEW handle (SetThunderURL
  # has no existing row to reuse) while the still-live credential's Thunder
  # Helm release keeps its OLD, immutable issuer — a mismatch between what AMS
  # reports and what the instance actually answers on. Skipping here just
  # leaves both rows in place for a retry.
  echo ""
  if [ "$system_client_deleted" != true ]; then
    echo "⚠️  Skipping thunder url handle cleanup — the system-client credential"
    echo "   was not confirmed deleted, so freeing the handle now could leave a"
    echo "   future re-provision attempt pointing at a mismatched issuer."
    echo "   Re-run this script once the credential cleanup above succeeds."
  else
    echo "🗑️  Removing thunder url handle from agent-manager-service (best-effort)..."
    # Reuse Step 3's token — same OU, same call shape, already known-good since
    # Step 3 just succeeded with it.
    local url_http_code
    url_http_code="$(curl -s -o /dev/null -w "%{http_code}" \
      --max-time 30 --retry 3 --retry-delay 5 \
      -X DELETE "${amp_api_url}/orgs/${org}/environments/${ENV_NAME}/thunder-url" \
      -H "Authorization: Bearer ${access_token}" 2>/dev/null || true)"
    url_http_code="${url_http_code:-000}"
    case "$url_http_code" in
      200|204)
        echo "✅ Removed thunder url handle from agent-manager-service"
        ;;
      *)
        echo "⚠️  Could not remove the thunder url handle from agent-manager-service (HTTP ${url_http_code})."
        echo "   Harmless to skip — continuing."
        ;;
    esac
  fi

  echo ""
  echo "=== Thunder ID for '${ENV_NAME}' removed ==="
  echo ""
  echo "  Environment: ${ENV_NAME}"
  echo "  Release:     ${release}"
  echo "  Namespace:   ${ns}"
  echo ""
  if [ "$external_db" = true ]; then
    echo "ℹ️  This environment used an external PostgreSQL. Its databases were NOT dropped:"
    echo "   ${external_prefix:-<prefix>}configdb, ${external_prefix:-<prefix>}entitydb,"
    echo "   ${external_prefix:-<prefix>}runtime_transient, ${external_prefix:-<prefix>}runtime_persistent"
    echo "   They still hold this environment's identity data. Thunder seeds a database only once,"
    echo "   at first install, so adding the environment again against them will NOT re-seed them:"
    echo "   drop and recreate them (and reload the schema), or use a new THUNDER_DB_NAME_PREFIX."
    echo ""
  fi
}

# Run main only when executed directly — not when sourced (e.g. by tests).
# BASH_SOURCE[0] is unset when the script is piped to bash (curl ... | bash);
# ${BASH_SOURCE[0]:-$0} falls back to $0 (which equals "bash") so the condition
# stays true and main runs, while sourced execution still sees the script filename.
if [ "${BASH_SOURCE[0]:-$0}" = "${0}" ]; then
  main "$@"
fi
