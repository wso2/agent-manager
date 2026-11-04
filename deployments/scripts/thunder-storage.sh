#!/bin/bash
# Env-Thunder storage-settings validation: the SINGLE source of truth for the rules that
# decide whether the THUNDER_DB_*/THUNDER_CACHE_TYPE/THUNDER_REDIS_*/THUNDER_REPLICAS/
# THUNDER_STATE_ACCESS_MODE settings are coherent. Meant to be sourced, not executed.
#
# Both add-environment.sh (so wrong settings are rejected BEFORE the environment is
# created) and add-environment-thunder.sh (which applies them, and can be run on its own)
# source THIS file instead of carrying a copy of the rules. Do not add another copy
# anywhere else.
#
# Pure functions: sourcing this file defines them and does nothing else.

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
