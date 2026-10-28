#!/usr/bin/env bash
# shellcheck source-path=SCRIPTDIR
set -euo pipefail

# Provisions a dedicated Thunder ID instance for ONE environment (the home of that
# environment's agent identities). The platform Thunder (amp-thunder, console login)
# is separate and untouched — env-Thunders are added alongside it.
#
# Unlike platform Thunder (which is installed via the agent-manager-owned
# wso2-amp-thunder-extension chart, including its full console/API/roles/groups
# bootstrap), env-Thunder installs the upstream ThunderID release chart DIRECTLY
# (oci://ghcr.io/thunder-id/helm-charts/thunderid — see
# https://thunderid.dev/docs/v1.0.x/getting-started/get-thunderid/). This
# decouples env-Thunder's version from whatever version platform Thunder happens to
# run (including platform Thunder being rolled back), and from the agent-manager
# release cadence — no wso2-amp-thunder-extension release is required to pick up a
# new ThunderID version here. Everything env-Thunder needs beyond the bare chart
# (trusted-issuer wiring, the amp-system-client OAuth app, external routing) is
# applied by this script directly, using the upstream chart's own native knobs
# (configuration.server.security.trustedIssuer, bootstrap.configMap, setup.admin,
# declarativeResources) or plain kubectl-applied manifests.
#
# All inputs are provided via environment variables so the script can be piped
# directly into bash:
#
#   curl -fsSL https://raw.githubusercontent.com/wso2/agent-manager/main/deployments/scripts/add-environment-thunder.sh \
#     | ENV_NAME=staging \
#       DISPLAY_NAME="Staging" \
#       bash
#
# Re-running with the same ENV_NAME is idempotent (helm upgrade --install; the
# system-client secret is reused, never rotated).
#
# Prerequisites:
#   - kubectl and helm must be configured
#   - ENV_NAME: resource name (lowercase alphanumeric with hyphens)
#   - DISPLAY_NAME: human-readable name
# Optional:
#   - ORG_NAME (default: default)
#   - THUNDER_CHART: override the chart ref (default: oci://ghcr.io/thunder-id/helm-charts/thunderid —
#     the upstream ThunderID release chart, pulled directly, NOT the agent-manager chart)
#   - CHART_VERSION: pin the chart version (default: 1.0.0; OCI charts only)
#   - THUNDER_HANDLE (default: unset) — an unguessable label that becomes this
#     environment's externally-reachable hostname segment:
#     "<handle>.<THUNDER_HOST_BASE_DOMAIN>". If unset,
#     agent-manager-service generates a 10-character one for you (see
#     register_thunder_url below) — there is no org/env-derived fallback.
#     Registered with AMS BEFORE any cluster mutation, so a handle already
#     taken by a DIFFERENT environment fails fast (exit 1) instead of leaving
#     an orphaned Helm release. Re-running with the same ENV_NAME reuses the
#     same stored handle.
#   - SYSTEM_CLIENT_SECRET (default: generated; reused if one already exists)
#   - THUNDER_ADMIN_PASSWORD (default: generated 10-char password w/ letters, digits,
#     and symbols; reused if one already exists) — native ThunderID superadmin password
#     for THIS env-Thunder's own /console. Printed at the end of this script's output;
#     not saved to disk. Stored server-side as a K8s Secret (<release>-admin-credentials,
#     key "password") so re-running the script reuses it instead of rotating it.
#   - PERSISTENCE_SIZE (default: 1Gi), STORAGE_CLASS (default: cluster default). Both
#     apply to the SQLite database volume only; the small certs/secrets volumes the
#     chart always creates use the cluster's default storage class.
#   - WAIT_TIMEOUT (default: 180s)
#   Storage (all optional). The defaults give a single-instance setup: SQLite on a
#   PVC, an in-memory cache and 1 replica. That is not highly available; ThunderID's
#   own production guide recommends PostgreSQL, and Redis once there is more than one
#   replica. Set these on the same command as this script (on `bash`, the last stage of
#   a curl | bash pipeline, not on curl):
#   - THUNDER_DB_TYPE (default: sqlite): sqlite | postgres. postgres needs an EXISTING
#     server with the four databases (see THUNDER_DB_NAME_PREFIX) already created AND
#     their schema loaded (each database's dbscripts/<db>/postgres.sql, taken from the
#     same Thunder image version as CHART_VERSION). Nothing in the chart creates
#     PostgreSQL tables, and against an empty database the setup hook fails with no
#     readable logs. Schema upgrades between ThunderID versions are also yours to apply.
#       THUNDER_DB_HOST, THUNDER_DB_USER, THUNDER_DB_PASSWORD, THUNDER_DB_NAME_PREFIX
#         are required for postgres. The databases used are <prefix>configdb,
#         <prefix>runtime_transient, <prefix>entitydb and <prefix>runtime_persistent.
#         There is no default prefix on purpose: two environments sharing one server
#         must not silently share databases. Use lowercase letters, digits and '_',
#         starting with a letter, at most 41 characters.
#       THUNDER_DB_PORT (default: 5432), THUNDER_DB_SSLMODE (default: require).
#   - THUNDER_CACHE_TYPE (default: inmemory): inmemory | redis. redis needs
#     THUNDER_REDIS_ADDRESS (host:port); optional THUNDER_REDIS_USERNAME,
#     THUNDER_REDIS_PASSWORD and THUNDER_REDIS_KEY_PREFIX (default: "<release>:", so
#     environments can share one Redis).
#   - THUNDER_REPLICAS (default: 1): more than 1 requires THUNDER_DB_TYPE=postgres AND
#     THUNDER_CACHE_TYPE=redis (SQLite is single-pod; per-pod in-memory caches
#     diverge). The chart's certs/secrets volumes are ReadWriteOnce by default; if the
#     replicas can land on different nodes, also set THUNDER_STATE_ACCESS_MODE=
#     ReadWriteMany (needs a storage class that supports it). The volumes are created
#     once, so this has no effect on an environment that already exists.
#   - THUNDER_FORCE_STORAGE_CHANGE (default: false): the script refuses to repoint an
#     already-installed environment at a different database, because Thunder seeds its
#     database only at first install. Set true only after migrating the data yourself.
#     The same refusal applies when an installed release's settings cannot be read, in
#     which case true skips that check. It never overrides a release that is mid-install,
#     mid-removal, or a failed first install whose setup job did not complete.
#   Re-running applies exactly the settings given on that run: a setting left out reverts
#   to its default (a re-run without THUNDER_REPLICAS scales back to 1), so pass the same
#   values every time. A run that fails its first install must be cleaned up with
#   remove-environment-thunder.sh before retrying: the chart's setup hook only runs on
#   first install, and the script refuses the retry unless the setup job did complete
#   (it can finish after helm's 5 minute hook timeout when an image pull is slow).
#   PostgreSQL: the role needs to own the four databases, or hold SELECT, INSERT, UPDATE
#   and DELETE on every table in them. The chart has no option for TLS to Redis, nor for
#   a private CA with THUNDER_DB_SSLMODE=verify-ca/verify-full. With non-default storage a
#   Thunder that is not ready after WAIT_TIMEOUT fails the run (with the defaults it only
#   warns), because the usual cause is an unreachable or unseeded database.
#   Passwords are stored in a Secret (<release>-storage-credentials) that the chart
#   reads by reference, so they never appear in the Helm release values. Changing a
#   password later updates the Secret but does not restart the pods; restart them
#   (kubectl rollout restart) to pick it up. Removing the environment
#   (remove-environment-thunder.sh) does NOT drop its PostgreSQL databases.
#   Delivering the system-client secret to agent-manager-service (AMS): the secret
#   is handed to AMS over HTTP, which encrypts it and stores it in its own database
#   (AMS decrypts it in-process when needed — it is never read back from a key
#   vault). AMS must be running and reachable when this step runs.
#   - AMP_API_URL (default: http://localhost:9000/api/v1) — AMS API base URL.
#     Local dev (docker-compose): http://localhost:9000/api/v1. k3d/quick-start:
#     http://api.amp.localhost:8080/api/v1. Set explicitly for other topologies.
#   - AGENT_MANAGER_TOKEN (default: unset) — a Bearer token with the
#     org:manage-service-account permission. If unset, this script obtains one via
#     a client_credentials grant using IDP_* below.
#   - IDP_TOKEN_URL (default: http://thunder.amp.localhost:8080/oauth2/token) —
#     platform Thunder token endpoint (used only when AGENT_MANAGER_TOKEN is unset).
#   - IDP_CLIENT_ID (default: amp-api-client), IDP_CLIENT_SECRET
#     (default: amp-api-client-secret) — the platform system client to grant as.
#     AMS derives the stored credential's OU ID from this token itself (never
#     from a client-supplied value — see models.EnvThunderSystemClient's doc
#     comment; org_name is not persisted for this credential at all).
#   Platform Thunder trusted-issuer (env-Thunder accepts platform Thunder tokens):
#   - PLATFORM_THUNDER_ISSUER   (default: http://thunder.amp.localhost:8080)
#   - PLATFORM_THUNDER_JWKS_URL (default: HTTPS JWKS endpoint of platform Thunder)
#   - PLATFORM_THUNDER_TOKEN_AUDIENCE (default: urn:wso2:amp — the aud claim platform
#     Thunder's tokens carry once any amp:* scope is requested, since ThunderID composes
#     aud from the resource server(s) resolved via the granted scopes. Must match the amp
#     resource server's identifier in the Thunder extension chart's
#     60-amp-resource-server.yaml. A scopeless client_credentials token instead
#     carries the calling client's own ID as aud.)
#   Non-local-dev deployments (e.g. a VM — see deployments/vm/lib-vm.sh, which sets
#   all three of these together, deployment-wide, whenever it provisions env-Thunder):
#   - THUNDER_HOST_BASE_DOMAIN (default: amp.localhost) — the domain suffix env-Thunder's
#     hostnames are built from ("<handle>.<this>"). MUST be set to the
#     identical value in agent-manager-service's own config (same env var name) on
#     any given deployment — see clients/thundersvc/naming.go's ThunderIssuerURL,
#     which independently computes the same value and has no way to learn about a
#     one-off override here.
#   - TLS_ENABLED (default: false) — when true, the issuer/publicUrl become
#     https://<host> with no explicit port (a VM's Caddy terminates TLS on the
#     standard HTTPS port) instead of http://<host>:8080 (the k3d gateway's
#     plain-HTTP port used in local dev). Same flag agent-manager-service and the
#     VM's own platform-Thunder Helm args already use for this exact purpose.
#   - SKIP_CA_BUNDLE_TRUST (default: false) — skip fetching/mounting a custom CA
#     bundle for the platform-Thunder trusted-issuer JWKS fetch. Set this when
#     platform Thunder's issuer is already backed by a real, publicly-trusted CA
#     (e.g. Let's Encrypt via a VM's Caddy) — the container's default trust store
#     already covers it, so there's nothing custom to mount. Leave false for local
#     dev, where platform Thunder's HTTPS gateway uses a self-signed CA that
#     nothing trusts by default.

# ---------------------------------------------------------------------------
# Pure helpers (sourced by the test suite; keep free of side effects).
# ---------------------------------------------------------------------------

# validate_name NAME -> 0 if a valid DNS-1123-ish label, non-zero otherwise.
validate_name() {
  printf '%s' "${1:-}" | grep -Eq '^[a-z0-9]([a-z0-9-]*[a-z0-9])?$'
}

# _sha256 FILE -> full SHA-256 hex of a file (portable: shasum or sha256sum).
_sha256() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}

# _storage_match VAR REGEX WHAT -> 0 if VAR is unset/empty or matches REGEX in full;
# otherwise prints what was expected and returns 1. The match uses [[ =~ ]] on
# purpose: grep would test a multi-line value line by line, so "ok<newline>--set x=y"
# would pass a check meant to keep every value a single helm argument. Never use this
# on a password: the offending value is echoed back.
_storage_match() {
  local name="$1" re="$2" what="$3" value
  value="${!name:-}"
  [ -z "$value" ] && return 0
  [[ "$value" =~ $re ]] && return 0
  echo "❌ ${name} must be ${what} (got '${value}')" >&2
  return 1
}

# validate_storage_config -> 0 if the THUNDER_DB_*/THUNDER_CACHE_TYPE/THUNDER_REDIS_*/
# THUNDER_REPLICAS/THUNDER_STATE_ACCESS_MODE settings are coherent; otherwise prints
# every problem to stderr and returns 1. The defaults (sqlite, inmemory, 1 replica)
# are always valid. Every value that ends up in a helm --set-string argument is held
# to a strict character set, because helm treats ',', '\' and '[' in a value as syntax.
validate_storage_config() {
  local db_type="${THUNDER_DB_TYPE:-sqlite}"
  local cache_type="${THUNDER_CACHE_TYPE:-inmemory}"
  local replicas="${THUNDER_REPLICAS:-1}"
  local bad=0 v

  case "$db_type" in
    sqlite|postgres) ;;
    *) echo "❌ THUNDER_DB_TYPE must be 'sqlite' or 'postgres' (got '${db_type}')" >&2; bad=1 ;;
  esac
  case "$cache_type" in
    inmemory|redis) ;;
    *) echo "❌ THUNDER_CACHE_TYPE must be 'inmemory' or 'redis' (got '${cache_type}')" >&2; bad=1 ;;
  esac
  case "${THUNDER_FORCE_STORAGE_CHANGE:-false}" in
    true|false) ;;
    *) echo "❌ THUNDER_FORCE_STORAGE_CHANGE must be 'true' or 'false' (got '${THUNDER_FORCE_STORAGE_CHANGE}')" >&2; bad=1 ;;
  esac

  # Settings for a backend that is not selected almost always mean THUNDER_DB_TYPE /
  # THUNDER_CACHE_TYPE was forgotten. Failing here beats silently provisioning SQLite
  # on a PVC when the operator meant an external PostgreSQL.
  if [ "$db_type" = "sqlite" ]; then
    for v in THUNDER_DB_HOST THUNDER_DB_PORT THUNDER_DB_USER THUNDER_DB_PASSWORD THUNDER_DB_SSLMODE THUNDER_DB_NAME_PREFIX; do
      if [ -n "${!v:-}" ]; then
        echo "❌ ${v} is set but THUNDER_DB_TYPE is 'sqlite'; set THUNDER_DB_TYPE=postgres or unset ${v}" >&2; bad=1
      fi
    done
  fi
  if [ "$cache_type" = "inmemory" ]; then
    for v in THUNDER_REDIS_ADDRESS THUNDER_REDIS_USERNAME THUNDER_REDIS_PASSWORD THUNDER_REDIS_KEY_PREFIX; do
      if [ -n "${!v:-}" ]; then
        echo "❌ ${v} is set but THUNDER_CACHE_TYPE is 'inmemory'; set THUNDER_CACHE_TYPE=redis or unset ${v}" >&2; bad=1
      fi
    done
  fi

  if ! [[ "$replicas" =~ ^[1-9][0-9]*$ ]]; then
    echo "❌ THUNDER_REPLICAS must be a positive integer (got '${replicas}')" >&2; bad=1
  elif [ "$replicas" != "1" ]; then
    if [ "$db_type" = "sqlite" ]; then
      echo "❌ THUNDER_REPLICAS=${replicas} needs THUNDER_DB_TYPE=postgres (SQLite is single-pod only)" >&2; bad=1
    fi
    if [ "$cache_type" = "inmemory" ]; then
      echo "❌ THUNDER_REPLICAS=${replicas} needs THUNDER_CACHE_TYPE=redis (each pod's in-memory cache would diverge)" >&2; bad=1
    fi
  fi
  # ReadWriteOncePod is deliberately not accepted: the chart's rolling update (maxSurge 1)
  # and this script's post-install patch both need a second pod to mount the volume.
  case "${THUNDER_STATE_ACCESS_MODE:-ReadWriteOnce}" in
    ReadWriteOnce|ReadWriteMany) ;;
    *) echo "❌ THUNDER_STATE_ACCESS_MODE must be ReadWriteOnce or ReadWriteMany (got '${THUNDER_STATE_ACCESS_MODE}')" >&2; bad=1 ;;
  esac
  if [ "$replicas" != "1" ] && [ -z "${THUNDER_STATE_ACCESS_MODE:-}" ]; then
    echo "⚠️  THUNDER_REPLICAS=${replicas} with the default ReadWriteOnce certs/secrets volumes: replicas scheduled" >&2
    echo "   on different nodes will not start. Set THUNDER_STATE_ACCESS_MODE=ReadWriteMany if your storage class supports it." >&2
  fi
  # PERSISTENCE_SIZE travels through storage_helm_args one argument per line, so it has to
  # stay a single-line Kubernetes quantity (it also catches a typo before the PVC does).
  _storage_match PERSISTENCE_SIZE '^[0-9]+(\.[0-9]+)?(Ki|Mi|Gi|Ti|Pi|Ei|k|M|G|T|P|E)?$' \
    "a Kubernetes quantity such as 1Gi" || bad=1

  if [ "$db_type" = "postgres" ]; then
    for v in THUNDER_DB_HOST THUNDER_DB_USER THUNDER_DB_PASSWORD THUNDER_DB_NAME_PREFIX; do
      if [ -z "${!v:-}" ]; then
        echo "❌ ${v} is required when THUNDER_DB_TYPE=postgres" >&2; bad=1
      fi
    done
    # Longest database name is "<prefix>runtime_persistent" (18 chars); PostgreSQL caps
    # identifiers at 63, hence a 41-character prefix at most.
    _storage_match THUNDER_DB_NAME_PREFIX '^[a-z][a-z0-9_]{0,40}$' \
      "1-41 lowercase letters, digits or '_', starting with a letter" || bad=1
    _storage_match THUNDER_DB_HOST '^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$' \
      "a hostname or IPv4 address" || bad=1
    _storage_match THUNDER_DB_USER '^[A-Za-z0-9_.@+-]+$' \
      "letters, digits or one of _ . @ + -" || bad=1
    if [ -n "${THUNDER_DB_PORT:-}" ]; then
      if ! [[ "$THUNDER_DB_PORT" =~ ^[1-9][0-9]{0,4}$ ]] || [ "$THUNDER_DB_PORT" -gt 65535 ]; then
        echo "❌ THUNDER_DB_PORT must be a port number from 1 to 65535 (got '${THUNDER_DB_PORT}')" >&2; bad=1
      fi
    fi
    case "${THUNDER_DB_SSLMODE:-require}" in
      disable|allow|prefer|require|verify-ca|verify-full) ;;
      *) echo "❌ THUNDER_DB_SSLMODE must be disable, allow, prefer, require, verify-ca or verify-full (got '${THUNDER_DB_SSLMODE}')" >&2; bad=1 ;;
    esac
    # The reverse mistake (postgres settings under sqlite) is an error above; these two
    # are only meaningful for the SQLite volume, so say they are being ignored.
    for v in PERSISTENCE_SIZE STORAGE_CLASS; do
      if [ -n "${!v:-}" ]; then
        echo "⚠️  ${v} only applies to the SQLite volume and is ignored with THUNDER_DB_TYPE=postgres." >&2
      fi
    done
  fi

  if [ "$cache_type" = "redis" ]; then
    if [ -z "${THUNDER_REDIS_ADDRESS:-}" ]; then
      echo "❌ THUNDER_REDIS_ADDRESS is required when THUNDER_CACHE_TYPE=redis" >&2; bad=1
    fi
    _storage_match THUNDER_REDIS_ADDRESS '^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?:[1-9][0-9]{0,4}$' \
      "host:port" || bad=1
    if [[ "${THUNDER_REDIS_ADDRESS:-}" =~ :([1-9][0-9]{0,4})$ ]] && [ "${BASH_REMATCH[1]}" -gt 65535 ]; then
      echo "❌ THUNDER_REDIS_ADDRESS port must be from 1 to 65535 (got '${THUNDER_REDIS_ADDRESS}')" >&2; bad=1
    fi
    _storage_match THUNDER_REDIS_USERNAME '^[A-Za-z0-9_.@+-]+$' \
      "letters, digits or one of _ . @ + -" || bad=1
    _storage_match THUNDER_REDIS_KEY_PREFIX '^[A-Za-z0-9_.:-]+$' \
      "letters, digits or one of _ . : -" || bad=1
  fi
  return "$bad"
}

# storage_helm_args RELEASE CREDS_SECRET PERSISTENCE_SIZE -> prints, one per line, the
# helm args for the database, cache and replica settings (validated first by
# validate_storage_config). CREDS_SECRET is the Secret holding db-password /
# redis-password; passwords reach the chart by reference, never inline. The base args
# in main() already pin deployment.replicaCount=1 (the SQLite requirement), so it is
# only restated here when more replicas were asked for; helm applies a later --set
# over an earlier one.
storage_helm_args() {
  local release="$1" creds_secret="$2" persistence_size="$3"
  local db_type="${THUNDER_DB_TYPE:-sqlite}"
  local cache_type="${THUNDER_CACHE_TYPE:-inmemory}"
  local replicas="${THUNDER_REPLICAS:-1}"
  local db_host="${THUNDER_DB_HOST:-}" db_user="${THUNDER_DB_USER:-}"
  local db_prefix="${THUNDER_DB_NAME_PREFIX:-}" redis_address="${THUNDER_REDIS_ADDRESS:-}"
  local pair store base

  if [ "$replicas" != "1" ]; then
    printf '%s\n' --set "deployment.replicaCount=${replicas}"
  fi

  if [ "$db_type" = "postgres" ]; then
    # store:upstream-default-database-name; the name gets THUNDER_DB_NAME_PREFIX.
    for pair in config:configdb runtime_transient:runtime_transient entity:entitydb runtime_persistent:runtime_persistent; do
      store="${pair%%:*}"
      base="configuration.database.${store}"
      printf '%s\n' \
        --set "${base}.type=postgres" \
        --set-string "${base}.postgres.hostname=${db_host}" \
        --set-string "${base}.postgres.port=${THUNDER_DB_PORT:-5432}" \
        --set-string "${base}.postgres.name=${db_prefix}${pair#*:}" \
        --set-string "${base}.postgres.username=${db_user}" \
        --set-string "${base}.postgres.sslmode=${THUNDER_DB_SSLMODE:-require}" \
        --set-string "${base}.postgres.passwordRef.name=${creds_secret}" \
        --set-string "${base}.postgres.passwordRef.key=db-password"
    done
  else
    for store in config runtime_transient entity runtime_persistent; do
      printf '%s\n' --set "configuration.database.${store}.type=sqlite"
    done
    printf '%s\n' \
      --set "configuration.consent.database.type=sqlite" \
      --set "persistence.enabled=true" \
      --set "persistence.size=${persistence_size}"
  fi

  if [ "$cache_type" = "redis" ]; then
    printf '%s\n' \
      --set "configuration.cache.disabled=false" \
      --set "configuration.cache.type=redis" \
      --set-string "configuration.cache.redis.address=${redis_address}" \
      --set-string "configuration.cache.redis.keyPrefix=${THUNDER_REDIS_KEY_PREFIX:-${release}:}"
    if [ -n "${THUNDER_REDIS_USERNAME:-}" ]; then
      printf '%s\n' --set-string "configuration.cache.redis.username=${THUNDER_REDIS_USERNAME}"
    fi
    if [ -n "${THUNDER_REDIS_PASSWORD:-}" ]; then
      printf '%s\n' \
        --set-string "configuration.cache.redis.passwordRef.name=${creds_secret}" \
        --set-string "configuration.cache.redis.passwordRef.key=redis-password"
    fi
  else
    printf '%s\n' --set "configuration.cache.disabled=false"
  fi

  # Only matters when a replica could land on another node; the chart's PVCs are
  # created once (pre-install hook), so changing this on a re-run has no effect.
  if [ -n "${THUNDER_STATE_ACCESS_MODE:-}" ]; then
    printf '%s\n' \
      --set-string "certs.persistence.accessMode=${THUNDER_STATE_ACCESS_MODE}" \
      --set-string "secrets.persistence.accessMode=${THUNDER_STATE_ACCESS_MODE}"
  fi
}

# storage_is_default -> 0 when no storage setting differs from the single-instance
# default (SQLite, in-memory cache, 1 replica).
storage_is_default() {
  [ "${THUNDER_DB_TYPE:-sqlite}" = "sqlite" ] &&
    [ "${THUNDER_CACHE_TYPE:-inmemory}" = "inmemory" ] &&
    [ "${THUNDER_REPLICAS:-1}" = "1" ]
}

# existing_release_storage RELEASE NAMESPACE -> prints "<db-type> <config-database-name>"
# for an installed release (the name is only reported for postgres: the chart's own
# default postgres block is present in the values of a SQLite release too).
# Returns 2 when the release is not installed ("release: not found"), and 1 for anything
# else: any other helm error, or values this parser does not recognise. Treating those
# as "not installed" would let the guard below fail open exactly when it cannot see.
# Parses `helm get values --all` (plain YAML, sorted keys) with awk so no jq is needed.
existing_release_storage() {
  local release="$1" ns="$2" values rc=0
  values="$(helm get values "$release" --namespace "$ns" --all 2>&1)" || rc=$?
  if [ "$rc" -ne 0 ]; then
    case "$values" in
      *"release: not found"*) return 2 ;;
    esac
    printf '%s\n' "$values" >&2
    return 1
  fi
  [ -n "$values" ] || return 1
  printf '%s\n' "$values" | awk '
    { match($0, /^ */); indent = RLENGTH }
    /^  database:$/ { in_db = 1; in_cfg = 0; in_pg = 0; next }
    in_db && NF && indent <= 2 { in_db = 0; in_cfg = 0; in_pg = 0 }
    in_db && indent == 4 { in_cfg = ($0 == "    config:"); in_pg = 0 }
    in_cfg && indent == 6 {
      in_pg = ($0 == "      postgres:")
      if ($1 == "type:") type = $2
    }
    in_cfg && in_pg && indent == 8 && $1 == "name:" { name = $2 }
    END {
      gsub(/"/, "", type); gsub(/"/, "", name)
      if (type == "") exit 1
      if (type != "postgres") name = ""
      print type, name
    }'
}

# release_state RELEASE NAMESPACE -> prints "<status> <revision>" from `helm status`
# (for example "deployed 3"); returns 1 if it cannot be read.
release_state() {
  helm status "$1" --namespace "$2" 2>/dev/null |
    awk '/^STATUS:/ { s = $2 } /^REVISION:/ { r = $2 } END { if (s == "") exit 1; print s, r }'
}

# setup_job_succeeded RELEASE NAMESPACE -> 0 if the chart's setup Job (which seeds the
# database) has completed. The chart deletes that Job only when helm sees it succeed, so
# one that finished after helm gave up waiting (a slow image pull outlasting helm's 5
# minute hook timeout) is still there with a success count, and nothing needs redoing.
setup_job_succeeded() {
  [ "$(kubectl get job "${1}-setup" --namespace "$2" -o jsonpath='{.status.succeeded}' 2>/dev/null)" = "1" ]
}

# guard_storage_unchanged RELEASE NAMESPACE -> 0 if provisioning may proceed, 1 if it
# would repoint an installed environment at a different database, or cannot tell.
# Thunder seeds its database once, from a pre-install hook that `helm upgrade` never
# repeats, so a re-run against another database (SQLite to PostgreSQL, or another
# PostgreSQL database) leaves an empty one: no OAuth clients, no logins, and nothing
# says why. Moving the host of the same database is fine and is not blocked.
# THUNDER_FORCE_STORAGE_CHANGE=true overrides a mismatch or an unreadable release, for an
# operator who migrated the data; it never overrides a release that is mid-operation or
# a failed first install whose setup job did not complete.
guard_storage_unchanged() {
  local release="$1" ns="$2"
  local want_type="${THUNDER_DB_TYPE:-sqlite}" want_name="" current cur_type cur_name rc=0
  if [ "$want_type" = "postgres" ]; then
    want_name="${THUNDER_DB_NAME_PREFIX}configdb"
  fi

  current="$(existing_release_storage "$release" "$ns")" || rc=$?
  if [ "$rc" -eq 2 ]; then
    return 0 # not installed: nothing to protect
  fi

  # Checked before the force flag is consulted: it can waive a storage mismatch, never a
  # release that is mid-operation or a failed first install. An unreadable status is not
  # grounds to stop by itself (when the values were readable, so is the release).
  local state status="" revision=""
  state="$(release_state "$release" "$ns")" || state=""
  read -r status revision <<<"$state"
  case "$status" in
    pending-install|pending-upgrade|pending-rollback|uninstalling|unknown)
      {
        echo "❌ '${release}' is in state '${status}': an install, upgrade or removal is still running or was"
        echo "   interrupted, and helm would refuse this run partway through."
        echo "   Wait for it to finish, or run remove-environment-thunder.sh and then this script again."
      } >&2
      return 1
      ;;
    failed)
      # A failed first install is retried through helm's upgrade path, which skips the
      # chart's pre-install hooks (the setup Job and the volumes), so the database would
      # never be seeded, unless the setup Job did finish after helm gave up on it.
      if [ "$revision" = "1" ] && ! setup_job_succeeded "$release" "$ns"; then
        {
          echo "❌ '${release}' is a failed first install and its setup job has not completed. Re-running"
          echo "   would skip the chart's setup hook (it only runs on first install), so the database would"
          echo "   never be seeded."
          echo "   If the job is still running (kubectl get job ${release}-setup -n ${ns}), wait for it and re-run."
          echo "   Otherwise run remove-environment-thunder.sh, fix the cause, then run this script again."
        } >&2
        return 1
      fi
      ;;
  esac

  if [ "$rc" -ne 0 ]; then
    if [ "${THUNDER_FORCE_STORAGE_CHANGE:-false}" = "true" ]; then
      echo "⚠️  Could not read the storage settings of '${release}'; continuing (THUNDER_FORCE_STORAGE_CHANGE=true)."
      return 0
    fi
    {
      echo "❌ '${release}' is installed but its storage settings could not be read (see above), so this"
      echo "   run cannot tell whether it would repoint the environment at a different database."
      echo "   Fix the cause and re-run, or set THUNDER_FORCE_STORAGE_CHANGE=true to skip this check."
    } >&2
    return 1
  fi

  read -r cur_type cur_name <<<"$current"
  if [ "$cur_type" = "$want_type" ] && { [ "$want_type" != "postgres" ] || [ "$cur_name" = "$want_name" ]; }; then
    return 0
  fi

  if [ "${THUNDER_FORCE_STORAGE_CHANGE:-false}" = "true" ]; then
    echo "⚠️  Storage of '${release}' changes from ${cur_type}${cur_name:+ (${cur_name})} to ${want_type}${want_name:+ (${want_name})} (THUNDER_FORCE_STORAGE_CHANGE=true)."
    return 0
  fi

  {
    echo "❌ '${release}' is already installed with ${cur_type}${cur_name:+ (database ${cur_name})}; this run asks for ${want_type}${want_name:+ (database ${want_name})}."
    echo "   Thunder seeds its database once, at first install, and 'helm upgrade' never repeats it,"
    echo "   so pointing an existing environment at a different database leaves that database empty:"
    echo "   no OAuth clients and no logins."
    echo "   Remove this environment's Thunder first (remove-environment-thunder.sh), or set"
    echo "   THUNDER_FORCE_STORAGE_CHANGE=true if you have already migrated the data."
  } >&2
  return 1
}

# Load the shared Thunder naming helpers (_sha6/thunder_release_name/
# thunder_namespace/thunder_host/thunder_issuer) — the single source of truth
# for this derivation, see deployments/scripts/thunder-naming.sh. Prefers a
# local sibling file (checked-out repo, or the test suite sourcing this
# script); falls back to fetching it from the same ref this script itself
# would be fetched from when piped via curl | bash.
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

# platform_thunder_issuer -> OIDC issuer of the shared platform Thunder instance.
# Env-Thunder trusts tokens bearing this issuer so callers can authenticate with
# a platform Thunder token instead of env-Thunder system-client credentials.
platform_thunder_issuer() {
  printf 'http://thunder.amp.localhost:8080'
}

# platform_thunder_jwks_url -> HTTPS JWKS URL that env-Thunder pods use to verify
# incoming platform Thunder tokens. Routed via the dedicated HTTPS Gateway on port
# 8443 (cert-manager-issued TLS, CA trusted via SSL_CERT_FILE inside the pod).
platform_thunder_jwks_url() {
  printf 'https://thunder.amp.localhost:8443/oauth2/jwks'
}

# platform_thunder_ca_cert -> prints the PEM CA cert that signed the certificate
# platform Thunder's issuer is served with, or returns 1 if not yet provisioned.
# Sources, in order:
#   1. PLATFORM_THUNDER_CA_PEM  — explicit injection (tests/CI, and the installer's
#      own in-process call).
#   2. ConfigMap amp-platform-ca — the operator-supplied CA persisted at install time
#      by a TLS_MODE=byoc advanced VM install (see deployments/vm/install-advanced.sh).
#   3. amp-local-root-ca-secret — the cert-manager self-signed root used by local/k3d
#      and any install where platform Thunder is served with the in-cluster CA.
#
# Source 2 exists because environments are created LONG AFTER the install exits. On a
# byoc install the public listener is served with the operator's certificate, so the
# self-signed root in (3) is the wrong CA: env-Thunder would mount a bundle that cannot
# validate the JWKS URL, and this script would still exit 0 — a silent failure that only
# surfaces later as a broken login into that environment. Persisting the CA in-cluster
# makes the trust decision durable instead of living in the installer's process env.
platform_thunder_ca_cert() {
  if [ -n "${PLATFORM_THUNDER_CA_PEM:-}" ]; then
    printf '%s' "$PLATFORM_THUNDER_CA_PEM"
    return 0
  fi

  # Operator-supplied CA persisted by a byoc install. Absent on every other install
  # path, so this lookup is a no-op there and the behaviour below is unchanged.
  local operator_ca
  operator_ca="$(kubectl get configmap amp-platform-ca -n openchoreo-control-plane \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null || true)"
  if [ -n "$operator_ca" ]; then
    # Any non-empty value would otherwise be accepted and folded into the trust bundle,
    # so a truncated or non-PEM ConfigMap would reproduce the exact silent failure this
    # lookup exists to prevent. The Mozilla bundle download applies the same check.
    if ! printf '%s' "$operator_ca" | grep -q "BEGIN CERTIFICATE"; then
      echo "❌ ConfigMap amp-platform-ca exists but ca.crt is not a PEM certificate." >&2
      echo "   Replace it with the CA that signed platform Thunder's certificate:" >&2
      echo "     kubectl create configmap amp-platform-ca -n openchoreo-control-plane \\" >&2
      echo "       --from-file=ca.crt=<ca.pem> --dry-run=client -o yaml | kubectl apply -f -" >&2
      return 1
    fi
    echo "🔐 Using the operator-supplied platform CA (ConfigMap amp-platform-ca)" >&2
    printf '%s' "$operator_ca"
    return 0
  fi

  # Wait for platform Thunder's TLS cert to be ready so we avoid racing with cert-manager.
  # Redirect output to stderr (>&2) to prevent polluting the stdout captured by the caller.
  if kubectl get certificate amp-thunder-extension-local-tls -n openchoreo-control-plane >/dev/null 2>&1; then
    echo "⏳ Waiting for platform Thunder TLS certificate to be issued by cert-manager..." >&2
    kubectl wait --for=condition=Ready certificate/amp-thunder-extension-local-tls \
      -n openchoreo-control-plane --timeout=300s >&2 || {
        echo "⚠️  Platform Thunder TLS certificate not yet ready — trusted issuer may not be configured." >&2
      }
  fi

  local b64
  b64="$(kubectl get secret amp-local-root-ca-secret -n cert-manager \
    -o jsonpath='{.data.ca\.crt}' 2>/dev/null || true)"
  [ -z "$b64" ] && return 1
  printf '%s' "$b64" | _b64decode
}

# _b64decode (stdin) -> decoded bytes (openssl is portable across macOS/Linux).
_b64decode() {
  openssl base64 -d -A
}

# generate_admin_password -> a 10-character random password with letters, digits,
# and special characters (avoids ambiguous chars like 0/O/1/l/I). Bash builtins only
# (no external tools) for portability across macOS/Linux.
generate_admin_password() {
  local alnum='ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789'
  local specials='!@%_='
  local chars=() i j tmp
  for ((i = 0; i < 8; i++)); do
    chars+=("${alnum:RANDOM % ${#alnum}:1}")
  done
  for ((i = 0; i < 2; i++)); do
    chars+=("${specials:RANDOM % ${#specials}:1}")
  done
  # Fisher-Yates shuffle so the two special characters aren't always at fixed positions.
  for ((i = ${#chars[@]} - 1; i > 0; i--)); do
    j=$((RANDOM % (i + 1)))
    tmp="${chars[i]}"; chars[i]="${chars[j]}"; chars[j]="$tmp"
  done
  printf '%s' "${chars[@]}"
}

# read_existing_secret NS NAME [KEY] -> prints the stored secret value (key
# defaults to "client-secret"), or returns 1 if the secret/key doesn't exist.
read_existing_secret() {
  local ns="$1" name="$2" key="${3:-client-secret}" b64
  b64="$(kubectl get secret "$name" -n "$ns" -o jsonpath="{.data.${key}}" 2>/dev/null || true)"
  [ -z "$b64" ] && return 1
  printf '%s' "$b64" | _b64decode
}

# Load the shared AMS auth helpers (json_escape/get_ams_token) — see
# deployments/scripts/ams-auth.sh. Same prefer-local-sibling,
# fallback-to-curl-fetch pattern as the thunder-naming.sh load above.
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

# store_via_ams ORG ENV CLIENT_ID SECRET — hands the credential to AMS over
# HTTP; AMS encrypts and stores it (never read back from a vault). Idempotent (PUT).
store_via_ams() {
  local org="$1" env_name="$2" client_id="$3" secret_val="$4"
  local amp_api_url="${AMP_API_URL:-http://localhost:9000/api/v1}"

  local access_token
  if ! access_token="$(get_ams_token)"; then
    echo "⚠️  Could not obtain an access token to call agent-manager-service."
    echo "   Set AGENT_MANAGER_TOKEN, or check IDP_TOKEN_URL/IDP_CLIENT_ID/IDP_CLIENT_SECRET"
    echo "   and that platform Thunder is reachable."
    return 1
  fi

  # AMS derives the credential's OU ID from this token itself (never from a
  # client-supplied value, to prevent one org's token from writing another
  # org's credential) — nothing to resolve or send here beyond the token.
  # json_escape guards against a custom SYSTEM_CLIENT_SECRET breaking the JSON body.
  local body
  body="$(printf '{"clientId":"%s","clientSecret":"%s"}' "$(json_escape "$client_id")" "$(json_escape "$secret_val")")"

  local http_code
  http_code="$(curl -s -o /dev/null -w "%{http_code}" \
    --max-time 30 --retry 5 --retry-delay 5 \
    -X PUT "${amp_api_url}/orgs/${org}/environments/${env_name}/thunder-system-client" \
    -H "Authorization: Bearer ${access_token}" \
    -H "Content-Type: application/json" \
    -d "${body}" 2>/dev/null)"
  # curl's own -w already writes "000" when no response is received; falling
  # back with `|| echo "000"` on top of that double-appends into "000000".
  http_code="${http_code:-000}"

  case "$http_code" in
    200|204)
      echo "🔐 Stored system-client secret in agent-manager-service (org=${org}, env=${env_name})"
      return 0
      ;;
  esac

  echo "⚠️  Could not store the system-client secret in agent-manager-service (HTTP ${http_code})."
  echo "   Check that AMP_API_URL (${amp_api_url}) is reachable and the token has the"
  echo "   org:manage-service-account permission, then re-run add-environment-thunder.sh."
  return 1
}

# register_thunder_url ORG ENV [HANDLE] -> prints the RESOLVED handle to stdout
# (the one that ended up stored — either HANDLE itself, or, when HANDLE is
# empty, a 10-character handle agent-manager-service generated on our behalf),
# or returns 1 on failure.
#
# ALWAYS called, even with an empty HANDLE — every environment needs a
# registered handle before a host can be computed at all (see thunder_host in
# thunder-naming.sh; there's no pattern to fall back to). This runs
# BEFORE anything is deployed, so a taken/invalid HANDLE fails fast instead of
# leaving an orphaned Helm release or a colliding HTTPRoute. Idempotent for the
# SAME (org, env) (PUT upserts, re-run reuses the same stored handle); a 409
# only happens when HANDLE is caller-supplied and a DIFFERENT environment
# already owns it — that is fatal, not a skip, since two environments must
# never share an external hostname.
register_thunder_url() {
  local org="$1" env_name="$2" handle="${3:-}"
  local amp_api_url="${AMP_API_URL:-http://localhost:9000/api/v1}"

  local access_token
  if ! access_token="$(get_ams_token)"; then
    echo "⚠️  Could not obtain an access token to call agent-manager-service." >&2
    echo "   Set AGENT_MANAGER_TOKEN, or check IDP_TOKEN_URL/IDP_CLIENT_ID/IDP_CLIENT_SECRET" >&2
    echo "   and that platform Thunder is reachable." >&2
    return 1
  fi

  # An empty body (no "handle" key) tells agent-manager-service to generate one —
  # sending {"handle":""} would instead be rejected as an invalid empty handle.
  local body
  if [ -n "$handle" ]; then
    body="$(printf '{"handle":"%s"}' "$(json_escape "$handle")")"
  else
    body='{}'
  fi

  local response http_code resp_body
  response="$(curl -s -w '\n%{http_code}' \
    --max-time 30 --retry 5 --retry-delay 5 \
    -X PUT "${amp_api_url}/orgs/${org}/environments/${env_name}/thunder-url" \
    -H "Authorization: Bearer ${access_token}" \
    -H "Content-Type: application/json" \
    -d "${body}" 2>/dev/null)"
  http_code="$(printf '%s' "$response" | tail -n1)"
  http_code="${http_code:-000}"
  resp_body="$(printf '%s' "$response" | sed '$d')"

  if [ "$http_code" = "200" ]; then
    local resolved
    resolved="$(printf '%s' "$resp_body" | grep -o '"handle":"[^"]*"' | cut -d'"' -f4)"
    if [ -z "$resolved" ]; then
      echo "⚠️  agent-manager-service returned 200 but no handle in the response body: ${resp_body}" >&2
      return 1
    fi
    if [ -n "$handle" ]; then
      echo "🔐 Registered thunder url handle '${resolved}' (org=${org}, env=${env_name})" >&2
    else
      # Not necessarily freshly generated: an empty HANDLE also resolves here when
      # agent-manager-service reuses an already-registered row (see
      # ResolveThunderHandle) — the response has no way to distinguish "new" from
      # "reused", so this message must not claim "generated" when it may not have been.
      echo "🔐 Resolved thunder url handle '${resolved}' (org=${org}, env=${env_name})" >&2
    fi
    printf '%s' "$resolved"
    return 0
  fi

  case "$http_code" in
    409)
      echo "❌ Thunder url handle '${handle}' is already taken." >&2
      echo "   Either a different environment already owns it, or THIS environment already has a" >&2
      echo "   DIFFERENT handle registered (Thunder's issuer is immutable once minted, so it's never" >&2
      echo "   silently changed) — remove it first with remove-environment-thunder.sh if you really" >&2
      echo "   want to switch, or choose a different THUNDER_HANDLE and re-run." >&2
      ;;
    400)
      echo "❌ Thunder url handle '${handle}' was rejected as invalid by agent-manager-service." >&2
      echo "   Must be lowercase alphanumeric with hyphens (no leading/trailing hyphen), 3-63 chars." >&2
      ;;
    *)
      echo "⚠️  Could not register the thunder url handle in agent-manager-service (HTTP ${http_code})." >&2
      echo "   Check that AMP_API_URL (${amp_api_url}) is reachable and the token has the" >&2
      echo "   org:manage-service-account permission, then re-run add-environment-thunder.sh." >&2
      ;;
  esac
  return 1
}

# render_system_client_bootstrap_resource SECRET -> prints a declarative ThunderID
# resource document (resource_type: application) registering the amp-system-client
# OAuth2 app with the "system" OAuth scope.
#
# Was previously a bash+curl script that registered the app then assigned it to
# ThunderID's own native "Administrator" role via /roles/{id}/assignments/add.
# ThunderID's bootstrap no longer executes custom scripts at all — it only imports
# declarative YAML documents via its own `thunderid bootstrap` subcommand — so this
# renders a plain YAML document instead of calling the API directly.
#
# Also switched the actual admin-API grant mechanism from role-assignment to the
# "system" OAuth scope: Thunder's admin API is gated by security.HasSystemPermission,
# which reads the "system" scope directly off the token's OAuth scope claim — NOT by
# a live role-assignment lookup. A client_credentials request that includes any
# explicit scope also requires a resolvable resource indicator, so
# render_default_resource_server_config below pairs with this to point at Thunder's
# own built-in "System" resource server.
#
# This is the ONLY bootstrap env-Thunder needs: agent-manager-service uses this one
# client_credentials app (see agent-manager-service/clients/thundersvc/naming.go) to
# call env-Thunder's admin API and create per-agent OAuth2 apps at runtime. The
# console/CLI/MCP/workload-publisher/observer-reader clients and the AMP-specific
# roles/groups bootstrapped for platform Thunder are human-console concerns that
# env-Thunder (agent identities only) does not need.
render_system_client_bootstrap_resource() {
  local secret="$1" doc squote escaped_secret
  # clientSecret below is single-quoted YAML, whose only escape rule is a
  # doubled literal quote — unlike the double-quoted form this replaced, it
  # has no backslash-escape sequences to misinterpret. Guards a custom
  # SYSTEM_CLIENT_SECRET (env var, see this script's header) the same way
  # json_escape guards the store_via_ams JSON path above, so a value
  # containing a quote can't break the document or silently diverge from
  # what store_via_ams already sent to agent-manager-service.
  squote="'"
  escaped_secret="${secret//$squote/$squote$squote}"
  # ouId (not ouHandle): same declarative-importer gap as platform Thunder's
  # bootstrap (amp-thunder-bootstrap.yaml) — the importer never resolves
  # ouHandle for `application` documents, so this app would otherwise end up
  # with no OU at all. "01900000-0000-7000-8000-000000000001" is ThunderID's
  # own built-in "default" OU, fixed on every install (platform or env-Thunder
  # alike), not something generated per instance.
  #
  # clientConfig.attributes opts this client_credentials app into embedding
  # ouId/ouHandle as token claims (Thunder never does this by default, even
  # once the app has a resolved OU) — required wherever this token is used
  # against org-scoped endpoints.
  doc="$(cat <<'BOOTSTRAP_RESOURCE'
resource_type: application
id: amp-system-client
type: m2m
name: AMP System Client
description: System client for agent-manager to provision per-org OAuth apps
ouId: "01900000-0000-7000-8000-000000000001"
inboundAuthConfig:
  - type: oauth2
    config:
      clientId: amp-system-client
      clientSecret: '__SYSTEM_CLIENT_SECRET__'
      grantTypes: [client_credentials]
      tokenEndpointAuthMethod: client_secret_basic
      pkceRequired: false
      publicClient: false
      scopes: [system]
      token:
        accessToken:
          clientConfig:
            validityPeriod: 3600
            attributes: ["ouId", "ouHandle"]
BOOTSTRAP_RESOURCE
)"
  printf '%s' "${doc//__SYSTEM_CLIENT_SECRET__/$escaped_secret}"
}

# render_default_resource_server_config -> prints a declarative server_config
# document setting ThunderID's own built-in "System" resource server (id fixed by
# ThunderID's own default bootstrap, backend/cmd/server/bootstrap/01-default-resources.yaml)
# as the default resource indicator. Required so amp-system-client's scoped token
# request (see render_system_client_bootstrap_resource above) resolves without an
# explicit "resource" parameter — see resourceindicators.go's defaultResourceServer
# fallback. env-Thunder has no AMP-specific resource server of its own (unlike
# platform Thunder's amp-resource-server), so this points at ThunderID's native one.
render_default_resource_server_config() {
  cat <<'BOOTSTRAP_RESOURCE'
resource_type: server_config
name: defaultResourceServer
value:
  resourceServerId: "01900000-0000-7000-8000-000000000020"
BOOTSTRAP_RESOURCE
}

# render_system_client_role -> prints a declarative role granting amp-system-client
# the "system" permission on ThunderID's built-in System resource server, via a
# DIRECT app assignment (not a group). Registering "system" as a scope on the app
# (render_system_client_bootstrap_resource) only makes the client eligible to
# request it — granthandlers/client_credentials.go's client-credentials flow still
# runs every requested scope through an AuthZEN-style entitlement evaluation
# (EvaluateAccessBatch, keyed off the app's own ID and its group memberships) before
# including it in the issued token, so eligibility alone isn't enough. Deliberately a
# new role, not a direct edit to ThunderID's own native "Administrator" role: role
# assignments are additive (roleAssignmentService.AddAssignments), so it would be
# safe to extend that role too, but keeping our grant in a role we own is clearer to
# audit and reason about later.
render_system_client_role() {
  cat <<'BOOTSTRAP_RESOURCE'
resource_type: role
id: amp-system-client-thunder-admin
name: "AMP System Client Thunder Admin"
description: "Grants the env-Thunder system client access to ThunderID's admin API"
ouHandle: default
permissions:
  - resourceServerId: "01900000-0000-7000-8000-000000000020"
    permissions:
      - system
assignments:
  - id: amp-system-client
    type: app
BOOTSTRAP_RESOURCE
}

# render_system_rs_identifier_fix ISSUER -> prints a declarative resource_server document
# that re-declares ThunderID's own built-in "System" resource server (same id, so this
# updates rather than duplicates it) with its identifier corrected to this env-Thunder
# instance's actual issuer. ThunderID's own default bootstrap
# (backend/cmd/server/bootstrap/01-default-resources.yaml) hardcodes
# identifier: https://localhost:8090/mcp, not templated to the deployment's real public
# URL, so the native /console app's own OAuth resource_identifier (correctly derived from
# configuration.server.publicUrl) never matches it — every native-console login redirects
# back with "invalid_target: The resource parameter does not match any registered
# resource server", and the console silently renders nothing.
# Preserves the full resources/actions tree verbatim so nothing ThunderID's own native
# Administrator role depends on gets dropped.
render_system_rs_identifier_fix() {
  local issuer="$1"
  cat <<BOOTSTRAP_RESOURCE
resource_type: resource_server
id: "01900000-0000-7000-8000-000000000020"
name: System
description: System resource server
identifier: "${issuer}/mcp"
ouHandle: default
resources:
  - name: System
    handle: system
    description: System resource
  - name: Organization Unit
    handle: ou
    description: Organization unit resource
    parent: system
    actions:
      - name: View
        handle: view
        description: Read-only access to organization units
  - name: User
    handle: user
    description: User resource
    parent: system
    actions:
      - name: View
        handle: view
        description: Read-only access to users
  - name: Group
    handle: group
    description: Group resource
    parent: system
    actions:
      - name: View
        handle: view
        description: Read-only access to groups
  - name: User Type
    handle: usertype
    description: User type resource
    parent: system
    actions:
      - name: View
        handle: view
        description: Read-only access to user types
BOOTSTRAP_RESOURCE
}

# apply_httproute RELEASE NAMESPACE HOST PORT — routes ${HOST}:8080 to the env-Thunder
# Service via the shared `gateway-default` Gateway in openchoreo-control-plane.
# gateway-default only allows HTTPRoutes from its own namespace, so the route (and a
# ReferenceGrant authorizing it to reach a Service in another namespace) are created
# there directly. Kept as plain manifests (not a Helm chart) since the upstream
# thunderid chart's own httproute/gateway support assumes a same-namespace Gateway,
# and this is the same routing platform Thunder relies on.
apply_httproute() {
  local release="$1" ns="$2" host="$3" port="$4"
  cat <<EOF | kubectl apply -f - >/dev/null
---
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: amp-thunder-backend
  namespace: ${ns}
  labels:
    app.kubernetes.io/instance: ${release}
    app.kubernetes.io/managed-by: add-environment-thunder.sh
    app.kubernetes.io/name: thunderid
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: openchoreo-control-plane
  to:
    - group: ""
      kind: Service
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ${release}
  namespace: openchoreo-control-plane
  labels:
    app.kubernetes.io/instance: ${release}
    app.kubernetes.io/managed-by: add-environment-thunder.sh
    app.kubernetes.io/name: thunderid
spec:
  hostnames:
    - ${host}
  parentRefs:
    - group: gateway.networking.k8s.io
      kind: Gateway
      name: gateway-default
  rules:
    - backendRefs:
        - group: ""
          kind: Service
          name: ${release}-service
          namespace: ${ns}
          port: ${port}
          weight: 1
      matches:
        - path:
            type: PathPrefix
            value: /
EOF
}

# patch_ca_bundle_mount RELEASE NAMESPACE CA_CONFIGMAP -> mounts the platform-CA
# ConfigMap into the env-Thunder Deployment via a strategic-merge kubectl patch,
# and sets SSL_CERT_FILE to point at it.
#
# NOT done via the chart's declarativeResources support: enabling it doesn't just
# mount extra files — it flips a GLOBAL server-side "declarative mode" flag that
# makes i18n translations read-only. ThunderID's OWN setup Job always tries to
# POST-seed default i18n translations regardless, so it fails with HTTP 400
# DCR-1002 "declarative_resource.update_operation_not_allowed" and the whole
# pre-install hook fails. The setup Job never needs this CA bundle anyway (its
# bootstrap scripts only call the LOCAL server on localhost:8090, never platform
# Thunder), so patching the Deployment after install — instead of setting a chart
# value before install — avoids the global flag entirely. Idempotent:
# `containers`/`volumes`/`env` merge by their name/mountPath key, so re-applying
# the same patch on every re-run is a no-op.
patch_ca_bundle_mount() {
  local release="$1" ns="$2" ca_cm_name="$3"
  kubectl patch deployment "${release}-deployment" -n "$ns" --type=strategic -p "$(cat <<EOF
{
  "spec": {
    "template": {
      "spec": {
        "volumes": [
          {"name": "platform-ca", "configMap": {"name": "${ca_cm_name}"}}
        ],
        "containers": [
          {
            "name": "thunderid",
            "volumeMounts": [
              {"name": "platform-ca", "mountPath": "/etc/ssl/amp/ca-bundle.crt", "subPath": "ca-bundle.crt", "readOnly": true}
            ],
            "env": [
              {"name": "SSL_CERT_FILE", "value": "/etc/ssl/amp/ca-bundle.crt"}
            ]
          }
        ]
      }
    }
  }
}
EOF
)" >/dev/null
}

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------
main() {
  : "${ENV_NAME:?ENV_NAME is required (e.g. ENV_NAME=staging)}"
  : "${DISPLAY_NAME:?DISPLAY_NAME is required (e.g. DISPLAY_NAME=\"Staging\")}"

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

  # Reject an incoherent storage combination (e.g. 2 replicas on SQLite), and one
  # that would repoint an installed environment at a different database, before
  # anything is registered or deployed.
  if ! validate_storage_config; then
    echo "   See the Storage section in this script's header for the allowed combinations." >&2
    exit 1
  fi
  if ! guard_storage_unchanged "$(thunder_release_name "$org" "$ENV_NAME")" "$(thunder_namespace "$org" "$ENV_NAME")"; then
    exit 1
  fi

  # THUNDER_HANDLE, when set, is the caller's chosen unguessable label for this
  # environment's externally-reachable host/issuer — see thunder_host's doc
  # comment and agent-manager-service/models/env_thunder_url.go. When unset,
  # agent-manager-service generates a 10-character one for us. Either way,
  # every environment must have a registered handle before a host can be
  # computed — there's no pattern to fall back to — so register_thunder_url is
  # always called, before any cluster mutation, so a taken/invalid handle fails
  # fast. Release name and namespace are UNAFFECTED (see below) — they never
  # honored THUNDER_HANDLE, since they're internal-only.
  local requested_handle="${THUNDER_HANDLE:-}"
  if [ -n "$requested_handle" ]; then
    # Length bounds mirror agent-manager-service's minThunderHandleLen/
    # maxThunderHandleLen — checked here too so a bad length fails fast
    # locally instead of a round trip that only fails at the 400 above.
    if ! validate_name "$requested_handle" || [ "${#requested_handle}" -lt 3 ] || [ "${#requested_handle}" -gt 63 ]; then
      echo "❌ Invalid THUNDER_HANDLE '${requested_handle}'"
      echo "   Must be lowercase alphanumeric with hyphens (no leading/trailing hyphen), 3-63 chars."
      exit 1
    fi
  fi
  local handle
  if ! handle="$(register_thunder_url "$org" "$ENV_NAME" "$requested_handle")"; then
    exit 1
  fi

  # Release name/namespace stay ALWAYS computed from (org, env) — they're
  # internal-only (Helm release name, K8s namespace), never externally
  # network-reachable, so not part of the guessable-URL attack surface the
  # handle closes. host/issuer, by contrast, ARE externally reachable and are
  # built ONLY from the registered handle now. Every consumer that resolves
  # host/issuer at runtime (agent-manager-service's ListThunderInstances/
  # EnvThunderResolver) reads this SAME registered handle back from its own DB —
  # see naming.go's ThunderExternalTokenURL doc comment.
  local release ns host issuer chart secret_name thunder_port
  release="$(thunder_release_name "$org" "$ENV_NAME")"
  ns="$(thunder_namespace "$org" "$ENV_NAME")"
  host="$(thunder_host "$handle")"
  issuer="$(thunder_issuer "$handle")"
  chart="${THUNDER_CHART:-oci://ghcr.io/thunder-id/helm-charts/thunderid}"
  secret_name="${release}-system-client"
  thunder_port=8090

  # gateClient controls the scheme/port Thunder's own login ("gate") flow bakes
  # into its sign-in redirect URLs — it does NOT automatically follow TLS_ENABLED
  # the way thunder_issuer() above does, so it must be derived the same way here,
  # or every sign-in link keeps pointing at plain HTTP:8080 even once TLS_ENABLED
  # has switched the issuer/publicUrl over to HTTPS.
  local gate_client_port gate_client_scheme
  if [ "${TLS_ENABLED:-false}" = "true" ]; then
    gate_client_port=443
    gate_client_scheme=https
  else
    gate_client_port=8080
    gate_client_scheme=http
  fi

  local persistence_size="${PERSISTENCE_SIZE:-1Gi}"
  local storage_class="${STORAGE_CLASS:-}"
  local wait_timeout="${WAIT_TIMEOUT:-180s}"

  # Platform Thunder coordinates — CORS origin + trusted-issuer JWKS (HTTPS via port 8443).
  local pt_issuer pt_jwks pt_audience
  pt_issuer="${PLATFORM_THUNDER_ISSUER:-$(platform_thunder_issuer)}"
  pt_jwks="${PLATFORM_THUNDER_JWKS_URL:-$(platform_thunder_jwks_url)}"
  pt_audience="${PLATFORM_THUNDER_TOKEN_AUDIENCE:-urn:wso2:amp}"

  # pt_issuer feeds both cors_origins below and trustedIssuer.issuer further down.
  # If TLS_ENABLED=true but PLATFORM_THUNDER_ISSUER was never explicitly set,
  # pt_issuer silently falls back to platform_thunder_issuer()'s plain-http
  # local-dev default, wiring an insecure origin/issuer into an otherwise
  # TLS-enabled install. That mismatch doesn't fail at startup, it just makes
  # every real token's iss claim silently stop matching later. Only check for
  # the unset case, not the scheme of an explicitly-passed value: a caller can
  # legitimately pass a plain-http PLATFORM_THUNDER_ISSUER alongside
  # TLS_ENABLED=true (e.g. platform Thunder reachable only via a local
  # port-forward while env-Thunder's own gateway routing still uses TLS).
  if [ "${TLS_ENABLED:-false}" = "true" ] && [ -z "${PLATFORM_THUNDER_ISSUER:-}" ]; then
    echo "❌ TLS_ENABLED=true but PLATFORM_THUNDER_ISSUER was not set — refusing to fall back to the"
    echo "   local-dev default ('$(platform_thunder_issuer)')."
    echo "   Set PLATFORM_THUNDER_ISSUER explicitly to platform Thunder's real issuer."
    exit 1
  fi

  # CORS origins for the AMP console reaching env-Thunder's own APIs directly
  # from the browser. localhost:3000/console.amp.localhost:8080 are quick-setup
  # (Rancher Desktop / amp-install-rancher.sh) console addresses — they don't
  # exist in a real deployment, so TLS_ENABLED=true (on-your-environment.mdx's
  # production flow) drops them and allows only the real platform Thunder
  # origin.
  local cors_origins
  if [ "${TLS_ENABLED:-false}" = "true" ]; then
    cors_origins="{${pt_issuer}}"
  else
    cors_origins="{http://localhost:3000,http://console.amp.localhost:8080,${pt_issuer}}"
  fi

  echo "=== Provisioning Thunder ID for environment '${ENV_NAME}' (org '${org}') ==="
  echo ""
  echo "  Release:   ${release}"
  echo "  Namespace: ${ns}"
  echo "  Issuer:    ${issuer}"
  echo "  Chart:     ${chart}"
  echo "  Storage:   db=${THUNDER_DB_TYPE:-sqlite} cache=${THUNDER_CACHE_TYPE:-inmemory} replicas=${THUNDER_REPLICAS:-1}"
  if [ "${THUNDER_DB_TYPE:-sqlite}" = "postgres" ]; then
    echo "             Expects these databases to exist on ${THUNDER_DB_HOST} with their schema loaded:"
    echo "             ${THUNDER_DB_NAME_PREFIX}configdb, ${THUNDER_DB_NAME_PREFIX}runtime_transient,"
    echo "             ${THUNDER_DB_NAME_PREFIX}entitydb, ${THUNDER_DB_NAME_PREFIX}runtime_persistent"
  fi
  echo ""

  # Resolve chart version for OCI charts (local chart paths skip this). Pinned to the
  # upstream ThunderID release we've validated env-Thunder against, independent of
  # whatever version platform Thunder happens to run.
  local version_args=()
  if printf '%s' "$chart" | grep -q '^oci://'; then
    local chart_version="${CHART_VERSION:-1.0.0}"
    echo "📌 Using Thunder chart version: ${chart_version}"
    version_args=(--version "$chart_version")
  fi

  # Ensure the namespace exists (idempotent) so the secrets can live in it.
  kubectl create namespace "$ns" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

  # Postgres/Redis passwords live in a Secret the chart reads via passwordRef (see
  # storage_helm_args), so they never land in the Helm release values. Re-applied on
  # every run, so a changed THUNDER_DB_PASSWORD / THUNDER_REDIS_PASSWORD takes effect.
  local storage_secret="${release}-storage-credentials"
  local storage_secret_args=()
  if [ "${THUNDER_DB_TYPE:-sqlite}" = "postgres" ]; then
    storage_secret_args+=(--from-literal="db-password=${THUNDER_DB_PASSWORD}")
    # Not a secret: kept here so remove-environment-thunder.sh can name the databases it leaves behind.
    storage_secret_args+=(--from-literal="db-name-prefix=${THUNDER_DB_NAME_PREFIX}")
  fi
  if [ "${THUNDER_CACHE_TYPE:-inmemory}" = "redis" ] && [ -n "${THUNDER_REDIS_PASSWORD:-}" ]; then
    storage_secret_args+=(--from-literal="redis-password=${THUNDER_REDIS_PASSWORD}")
  fi
  if [ "${#storage_secret_args[@]}" -gt 0 ]; then
    kubectl create secret generic "$storage_secret" -n "$ns" \
      "${storage_secret_args[@]}" \
      --dry-run=client -o yaml | kubectl apply -f - >/dev/null
    echo "🔐 Stored storage credentials (${storage_secret})"
  fi

  # Resolve the system-client secret: reuse an existing one (NO rotation), else
  # mint a unique one and store it as a K8s Secret in the env-Thunder namespace.
  local system_secret
  if system_secret="$(read_existing_secret "$ns" "$secret_name")" && [ -n "$system_secret" ]; then
    echo "🔐 Reusing existing system-client secret (${secret_name})"
  else
    system_secret="${SYSTEM_CLIENT_SECRET:-$(openssl rand -hex 24)}"
    kubectl create secret generic "$secret_name" -n "$ns" \
      --from-literal=client-secret="$system_secret" \
      --dry-run=client -o yaml | kubectl apply -f - >/dev/null
    echo "🔐 Stored new system-client secret (${secret_name})"
  fi
  # Deliver the secret to AMS (see store_via_ams). client_id here must match
  # clientId in render_system_client_bootstrap_resource below (both "amp-system-client").
  if ! store_via_ams "$org" "$ENV_NAME" "amp-system-client" "$system_secret"; then
    exit 1
  fi

  # Resolve this env-Thunder's own native admin password: reuse an existing one (NO
  # rotation — logging in with the old password must keep working across re-runs),
  # else mint a unique one and store it as a K8s Secret in the env-Thunder namespace.
  local admin_secret_name="${release}-admin-credentials"
  local admin_password
  if admin_password="$(read_existing_secret "$ns" "$admin_secret_name" "password")" && [ -n "$admin_password" ]; then
    echo "🔐 Reusing existing admin password (${admin_secret_name})"
  else
    admin_password="${THUNDER_ADMIN_PASSWORD:-$(generate_admin_password)}"
    kubectl create secret generic "$admin_secret_name" -n "$ns" \
      --from-literal=password="$admin_password" \
      --dry-run=client -o yaml | kubectl apply -f - >/dev/null
    echo "🔐 Stored new admin password (${admin_secret_name})"
  fi

  # Bootstrap ConfigMap: the amp-system-client OAuth2 app plus the default-resource-
  # server config it needs (see render_system_client_bootstrap_resource and
  # render_default_resource_server_config above). Pattern 2 (configMap.name + files)
  # preserves ThunderID's own default bootstrap resources (org unit, default user
  # schema, native Administrator role, etc.).
  local bootstrap_cm_name="${release}-bootstrap"
  kubectl create configmap "$bootstrap_cm_name" -n "$ns" \
    --from-literal="10-amp-system-client.yaml=$(render_system_client_bootstrap_resource "$system_secret")" \
    --from-literal="11-default-resource-server-config.yaml=$(render_default_resource_server_config)" \
    --from-literal="12-amp-system-client-thunder-admin-role.yaml=$(render_system_client_role)" \
    --from-literal="13-fix-thunder-system-rs-identifier.yaml=$(render_system_rs_identifier_fix "$issuer")" \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  echo "🔐 Bootstrap ConfigMap (${bootstrap_cm_name}) prepared"

  # Per-env overrides on top of upstream ThunderID chart defaults, using the
  # chart's own top-level values schema (no wrapper-chart value prefix).
  local set_args=(
    # Pin the K8s resource names to the release name (matches the naming every other
    # AMP component assumes, e.g. agent-manager-service/clients/thundersvc/naming.go's
    # "<release>-service" convention) instead of the chart's default fullname suffix.
    --set-string "fullnameOverride=${release}"
    --set-string "deployment.image.tag=${CHART_VERSION:-1.0.0}"
    # Single replica + writable root FS: required for SQLite (single-pod, local file DB).
    # THUNDER_REPLICAS and the database/cache settings are layered on below by
    # storage_helm_args; the root FS stays writable in every storage mode.
    --set "deployment.replicaCount=1"
    --set "deployment.securityContext.readOnlyRootFilesystem=false"
    --set "hpa.enabled=false"
    --set "ingress.enabled=false"
    --set-string "configuration.server.publicUrl=${issuer}"
    --set "configuration.server.httpOnly=true"
    --set-string "configuration.jwt.issuer=${issuer}"
    --set-string "configuration.gateClient.hostname=${host}"
    --set "configuration.gateClient.port=${gate_client_port}"
    --set-string "configuration.gateClient.scheme=${gate_client_scheme}"
    # CORS: allow the platform Thunder origin so its console can reach env-Thunder APIs
    # (plus quick-setup's local console addresses, only outside TLS_ENABLED — see
    # cors_origins above).
    --set "configuration.cors.allowedOrigins=${cors_origins}"
    # Native ThunderID superadmin (distinct from the AMP product's own admin user on
    # platform Thunder) — used to log into this env-Thunder's own /console. Password
    # is a per-env random secret (see admin_password resolution above), not "admin".
    --set-string "setup.admin.username=admin"
    --set-string "setup.admin.password=${admin_password}"
    --set-string "bootstrap.configMap.name=${bootstrap_cm_name}"
    --set-json 'bootstrap.configMap.files=["10-amp-system-client.yaml","11-default-resource-server-config.yaml","12-amp-system-client-thunder-admin-role.yaml","13-fix-thunder-system-rs-identifier.yaml"]'
  )
  # Database + cache + replicas (SQLite, in-memory cache and 1 replica unless
  # configured). Captured before use, not read from a process substitution: a failure
  # inside `< <(...)` is invisible to `set -e` and would install with a partial set.
  local storage_args _storage_arg
  storage_args="$(storage_helm_args "$release" "$storage_secret" "$persistence_size")"
  while IFS= read -r _storage_arg; do
    set_args+=("$_storage_arg")
  done <<<"$storage_args"
  if [ -n "$storage_class" ]; then
    set_args+=(--set-string "persistence.storageClass=${storage_class}")
  fi

  # ---------------------------------------------------------------------------
  # Trusted issuer: env-Thunder accepts tokens issued by platform Thunder.
  #
  # Flow:
  #   1. Fetch the self-signed CA cert that signed the platform Thunder TLS cert
  #      from cert-manager (or from PLATFORM_THUNDER_CA_PEM if injected by the
  #      caller — useful in tests/CI where cert-manager is not running).
  #   2. Fetch the Mozilla CA bundle (the same set shipped by Alpine / Debian
  #      ca-certificates packages) so the combined file is a complete trust store.
  #   3. Store the combined PEM bundle as a ConfigMap in the env-Thunder namespace,
  #      and queue --set trustedIssuer.issuer/jwksUrl/audience for the helm install
  #      below — that's the actual trust decision (which issuer to accept, and
  #      where to fetch its JWKS from); the CA bundle only exists so env-Thunder's
  #      Go TLS stack can reach that (HTTPS) JWKS URL in the first place.
  #   4. After install, mount the ConfigMap into the env-Thunder Deployment (via a
  #      post-install kubectl patch — NOT via the chart's declarativeResources
  #      support, see patch_ca_bundle_mount below for why) and set SSL_CERT_FILE
  #      to the combined file so Go's TLS stack trusts both commercial CAs and
  #      the self-signed CA.
  #
  # Set SKIP_CA_BUNDLE_TRUST=true to skip this whole flow when platform Thunder's
  # issuer is ALREADY backed by a real, publicly-trusted CA (e.g. a VM deployment
  # where Caddy terminates TLS with a Let's Encrypt cert — see
  # deployments/vm/lib-vm.sh) — the container's default system trust store already
  # trusts that CA, so there is nothing custom to fetch or mount. The trustedIssuer
  # --set-string args below (the actual trust decision) still apply either way;
  # only the custom-CA-bundle mechanics are skipped.
  # ---------------------------------------------------------------------------
  local ca_pem ca_cm_name=""
  if [ "${SKIP_CA_BUNDLE_TRUST:-false}" = "true" ]; then
    echo "🔐 SKIP_CA_BUNDLE_TRUST=true — platform Thunder's issuer is assumed to already be"
    echo "   backed by a publicly-trusted CA; skipping the custom CA bundle fetch/mount."
  else

  # Fetch platform Thunder CA. Missing cert is fatal to prevent silent auth failures.
  if ! ca_pem="$(platform_thunder_ca_cert)" || [ -z "$ca_pem" ]; then
    echo "❌ Platform Thunder CA cert is not available."
    echo "   Ensure cert-manager has issued amp-local-root-ca in the cert-manager namespace."
    echo "   Check status: kubectl get certificate -n cert-manager amp-local-root-ca"
    echo "   Alternatively, set PLATFORM_THUNDER_CA_PEM to inject the CA cert directly."
    exit 1
  fi

  # Fetch Mozilla root CA bundle. Appending our Root CA ensures the trust store
  # remains additive and compatible with any base image (Debian/Alpine).
  # The bundle is built on the operator's machine (not inside the pod), so we
  # cannot rely on the pod's /etc/ssl — we need a portable external source.
  local ca_bundle mozilla_bundle
  ca_cm_name="amp-thunder-platform-ca"

  # Short-circuit: if the ConfigMap already exists in the cluster (e.g. a re-run
  # or an idempotent apply), skip the 230KB download entirely.
  if kubectl get configmap "$ca_cm_name" -n "$ns" &>/dev/null 2>&1; then
    echo "🔐 CA bundle ConfigMap ${ns}/${ca_cm_name} already exists — skipping download"
    ca_bundle=""  # not used below when ConfigMap already present; set_args still queued
  else
    # Allow pre-downloading the bundle and pointing at it via MOZILLA_CA_BUNDLE.
    # Useful in air-gapped envs or when curl.se is unreachable.
    #   curl -fsSL https://curl.se/ca/cacert.pem -o /tmp/cacert.pem
    #   MOZILLA_CA_BUNDLE=/tmp/cacert.pem ENV_NAME=... bash add-environment-thunder.sh
    if [ -n "${MOZILLA_CA_BUNDLE:-}" ] && [ -f "$MOZILLA_CA_BUNDLE" ]; then
      echo "🔐 Using pre-downloaded Mozilla CA bundle from ${MOZILLA_CA_BUNDLE}"
      mozilla_bundle="$(cat "$MOZILLA_CA_BUNDLE")"
      if ! grep -q "BEGIN CERTIFICATE" <<< "$mozilla_bundle"; then
        echo "❌ MOZILLA_CA_BUNDLE file does not look like a PEM certificate bundle: ${MOZILLA_CA_BUNDLE}"
        exit 1
      fi
    else
      echo "🔐 Fetching Mozilla CA bundle from curl.se..."
      local tmp_bundle attempt download_ok
      tmp_bundle="$(mktemp)"
      download_ok=false
      for attempt in 1 2 3; do
        # Retry both download failures AND checksum mismatches — the old code only
        # retried the latter, causing the loop to be dead code on a network failure.
        if ! curl -fsSL --connect-timeout 30 https://curl.se/ca/cacert.pem -o "$tmp_bundle" 2>/dev/null; then
          if [ "$attempt" -lt 3 ]; then
            echo "⚠️  Download failed on attempt ${attempt}/3 — retrying in 5s..."
            sleep 5
            continue
          fi
          rm -f "$tmp_bundle"
          echo "❌ Could not fetch Mozilla CA bundle from https://curl.se/ca/cacert.pem after 3 attempts."
          echo "   Download it on a machine with internet access and set the path via MOZILLA_CA_BUNDLE:"
          echo "     curl -fsSL https://curl.se/ca/cacert.pem -o /tmp/cacert.pem"
          echo "     MOZILLA_CA_BUNDLE=/tmp/cacert.pem ENV_NAME=${ENV_NAME} ORG_NAME=${ORG_NAME} bash $(basename "$0")"
          exit 1
        fi
        if ! grep -q "BEGIN CERTIFICATE" "$tmp_bundle"; then
          if [ "$attempt" -lt 3 ]; then
            echo "⚠️  Downloaded file does not look like a PEM bundle on attempt ${attempt}/3 — retrying in 5s..."
            sleep 5
            continue
          fi
          rm -f "$tmp_bundle"
          echo "❌ Mozilla CA bundle download produced an unexpected response after 3 attempts."
          exit 1
        fi
        # Verify against the published checksum to detect download corruption.
        local expected_sha actual_sha
        if expected_sha="$(curl -fsSL --connect-timeout 15 https://curl.se/ca/cacert.pem.sha256 2>/dev/null | awk '{print $1}')" \
            && [ -n "$expected_sha" ]; then
          actual_sha="$(_sha256 "$tmp_bundle")"
          if [ "$expected_sha" != "$actual_sha" ]; then
            if [ "$attempt" -lt 3 ]; then
              echo "⚠️  Checksum mismatch on attempt ${attempt}/3 — retrying in 5s..."
              sleep 5
              continue
            fi
            rm -f "$tmp_bundle"
            echo "❌ Mozilla CA bundle checksum mismatch after 3 attempts — download may be corrupt."
            echo "   Expected: $expected_sha"
            echo "   Got:      $actual_sha"
            exit 1
          fi
          echo "   ✓ Checksum verified (SHA-256: ${actual_sha:0:16}...)"
        fi
        download_ok=true
        break
      done
      mozilla_bundle="$(cat "$tmp_bundle")"
      rm -f "$tmp_bundle"
      [ "$download_ok" = "true" ] || { echo "❌ CA bundle fetch failed unexpectedly"; exit 1; }
    fi

    ca_bundle="${mozilla_bundle}
${ca_pem}"

    # Store the combined bundle as a ConfigMap. Pass the bundle through a temp
    # file with --from-file rather than --from-literal: the Mozilla CA bundle is
    # ~230KB, which exceeds the Linux per-argument limit (MAX_ARG_STRLEN, 128KB)
    # and makes kubectl exit with "Argument list too long" before it emits any
    # YAML, so the piped `kubectl apply` then fails with "no objects passed to
    # apply". (macOS has a larger limit, so this only bites on Linux/CI.)
    local ca_bundle_file
    ca_bundle_file="$(mktemp)"
    printf '%s' "$ca_bundle" >"$ca_bundle_file"
    kubectl create configmap "$ca_cm_name" -n "$ns" \
      --from-file=ca-bundle.crt="$ca_bundle_file" \
      --dry-run=client -o yaml | kubectl apply -f - >/dev/null
    rm -f "$ca_bundle_file"
    echo "🔐 Combined CA bundle (Mozilla + platform Thunder CA) stored in ${ns}/${ca_cm_name}"
  fi
  fi # SKIP_CA_BUNDLE_TRUST

  set_args+=(
    # Configure the trusted issuer endpoints. (CA bundle mounting is done via a
    # post-install kubectl patch below, NOT declarativeResources — see patch_ca_bundle_mount.)
    --set-string "configuration.server.security.trustedIssuer.issuer=${pt_issuer}"
    --set-string "configuration.server.security.trustedIssuer.jwksUrl=${pt_jwks}"
    --set-string "configuration.server.security.trustedIssuer.audience=${pt_audience}"
  )

  echo ""
  echo "📦 Installing Thunder (${release}) from ${chart}..."
  helm upgrade --install "$release" "$chart" \
    ${version_args[@]+"${version_args[@]}"} \
    --namespace "$ns" --create-namespace \
    "${set_args[@]}"

  if [ "${SKIP_CA_BUNDLE_TRUST:-false}" != "true" ]; then
    echo ""
    echo "🔐 Mounting platform CA bundle into the Deployment (post-install patch)..."
    patch_ca_bundle_mount "$release" "$ns" "$ca_cm_name"
  fi

  echo ""
  echo "🌐 Routing ${host}:8080 to ${release}..."
  apply_httproute "$release" "$ns" "$host" "$thunder_port"

  echo ""
  echo "⏳ Waiting for Thunder '${release}' to be ready..."
  if kubectl wait --for=condition=available --timeout="$wait_timeout" \
      deployment -l "app.kubernetes.io/instance=${release}" -n "$ns" 2>/dev/null; then
    echo "✅ Thunder is ready"
  elif storage_is_default; then
    echo "⚠️  Thunder did not become ready in time — check: kubectl get pods -n ${ns}"
  else
    # With an external database or cache, "not ready" usually means it is unreachable or
    # its schema is missing. Do not go on to print a console URL and password that lead
    # nowhere, and do not let add-environment.sh report success.
    echo "❌ Thunder did not become ready in time — check: kubectl get pods -n ${ns}"
    echo "   With non-default storage the usual causes are an unreachable PostgreSQL/Redis, wrong"
    echo "   credentials, or a database whose schema was not loaded."
    exit 1
  fi

  echo ""
  echo "=== Thunder ID for '${ENV_NAME}' provisioned ==="
  echo ""
  echo "  Environment:     ${ENV_NAME}"
  echo "  Namespace:       ${ns}"
  echo "  Release:         ${release}"
  echo "  Chart:           ${chart} (${CHART_VERSION:-1.0.0})"
  echo "  Issuer:          ${issuer}"
  echo "  JWKS:            ${issuer}/oauth2/jwks"
  echo "  Trusted issuer:  ${pt_issuer}"
  echo "  Storage:         db=${THUNDER_DB_TYPE:-sqlite} cache=${THUNDER_CACHE_TYPE:-inmemory} replicas=${THUNDER_REPLICAS:-1}"
  echo ""
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  Thunder ID Console — ${ENV_NAME}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  URL:      ${issuer}/console"
  echo "  Username: admin"
  echo "  Password: ${admin_password}"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "  ⚠️  Save this password now to access the '${ENV_NAME}' Environment ThunderID"
  echo "     console later, you'll need it, and it won't be shown again."
  echo ""
}

# Run main only when executed directly — not when sourced (e.g. by tests).
# BASH_SOURCE[0] is unset when the script is piped to bash (curl ... | bash);
# ${BASH_SOURCE[0]:-$0} falls back to $0 (which equals "bash") so the condition
# stays true and main runs, while sourced execution still sees the script filename.
if [ "${BASH_SOURCE[0]:-$0}" = "${0}" ]; then
  main "$@"
fi
