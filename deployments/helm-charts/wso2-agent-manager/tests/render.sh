#!/usr/bin/env bash
# Render assertions for wso2-agent-manager.
# Run: bash deployments/helm-charts/wso2-agent-manager/tests/render.sh
#
# Covers the derived auth values that plain `helm template` with default values
# cannot distinguish from hardcoded ones: OAUTH_AUTHORIZATION_SERVERS falling
# back to keyManager.issuer, and serverPublicURL being appended to
# keyManager.audience. Both are invisible at install time and surface only as a
# broken MCP login — `invalid_target` on authorize if the advertised
# authorization server is wrong, or 401 on every tool call if the audience is
# (see issues #1414 and #1424).
set -uo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FAILURES=0

# cm_value <key> [helm --set args...] -> the rendered value of ConfigMap data key <key>
# A render failure is reported rather than silently becoming an empty value, so a
# crash cannot look like a wrong value.
cm_value() {
  local key="$1" rendered
  shift
  if ! rendered="$(helm template test-release "$CHART_DIR" \
    --show-only templates/agent-manager-service/configmap.yaml "$@" 2>&1)"; then
    printf 'helm template failed: %s\n' "$rendered" >&2
    return 1
  fi
  awk -v k="$key" '
    $1 == k":" {
      sub(/^[[:space:]]*[^:]+:[[:space:]]*/, "")
      sub(/[[:space:]]+$/, "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  ' <<<"$rendered"
}

assert_cm() {
  local label="$1" key="$2" expected="$3"
  shift 3
  local actual
  actual="$(cm_value "$key" "$@")"
  if [[ "$expected" == "$actual" ]]; then
    printf 'ok   - %s\n' "$label"
  else
    printf 'FAIL - %s\n      expected %s: %q\n      actual   %s: %q\n' \
      "$label" "$key" "$expected" "$key" "$actual"
    FAILURES=$((FAILURES + 1))
  fi
}

CLIENTS="urn:wso2:amp,amp-console-client,amp-api-client,amp-publisher-*,amctl,am-mcp"

# Defaults must stay byte-identical to the pre-derivation literals, so existing
# installs and quick-start (which passes no overrides) are unaffected.
assert_cm "default audience keeps the gateway resource entry" \
  KEY_MANAGER_AUDIENCE "${CLIENTS},http://api.amp.localhost:8080/mcp"
assert_cm "default authorization servers fall back to the default issuer" \
  OAUTH_AUTHORIZATION_SERVERS "http://thunder.amp.localhost:8080"

# Issue #1424: one issuer override has to move the advertised authorization
# server too, or MCP clients discover an authorization server whose tokens this
# service then rejects.
assert_cm "issuer override moves the advertised authorization server" \
  OAUTH_AUTHORIZATION_SERVERS "https://thunder.example.com" \
  --set agentManagerService.config.keyManager.issuer=https://thunder.example.com
assert_cm "explicit authorization servers win over the issuer" \
  OAUTH_AUTHORIZATION_SERVERS "https://as.example.com" \
  --set agentManagerService.config.keyManager.issuer=https://thunder.example.com \
  --set agentManagerService.config.oauthAuthorizationServers=https://as.example.com

# Issue #1414: serverPublicURL is the RFC 8707 resource identifier MCP tokens are
# minted with, so it must reach the audience list with "/mcp" appended and no
# trailing slash (per the MCP spec's canonical-URI guidance).
assert_cm "serverPublicURL override is appended to the audience" \
  KEY_MANAGER_AUDIENCE "${CLIENTS},https://api.example.com/mcp" \
  --set agentManagerService.config.serverPublicURL=https://api.example.com
assert_cm "a serverPublicURL that already ends in a slash is not doubled" \
  KEY_MANAGER_AUDIENCE "${CLIENTS},https://api.example.com/mcp" \
  --set agentManagerService.config.serverPublicURL=https://api.example.com/
assert_cm "an audience that already lists the resource gains no duplicate" \
  KEY_MANAGER_AUDIENCE "amp,https://api.example.com/mcp" \
  --set agentManagerService.config.serverPublicURL=https://api.example.com \
  --set 'agentManagerService.config.keyManager.audience=amp\,https://api.example.com/mcp'
assert_cm "whitespace in the audience does not defeat the duplicate check" \
  KEY_MANAGER_AUDIENCE "amp,https://api.example.com/mcp" \
  --set agentManagerService.config.serverPublicURL=https://api.example.com \
  --set 'agentManagerService.config.keyManager.audience=amp\, https://api.example.com/mcp'
assert_cm "an empty serverPublicURL appends nothing and leaves no trailing comma" \
  KEY_MANAGER_AUDIENCE "$CLIENTS" \
  --set-string agentManagerService.config.serverPublicURL=
assert_cm "a stray comma in the audience does not produce an empty entry" \
  KEY_MANAGER_AUDIENCE "amp,https://api.example.com/mcp" \
  --set agentManagerService.config.serverPublicURL=https://api.example.com \
  --set 'agentManagerService.config.keyManager.audience=amp\,'

# env_value <template-path> <env-name> [helm --set args...] -> value of that env var,
# or empty when the variable is not rendered at all.
env_value() {
  local tmpl="$1" name="$2" rendered
  shift 2
  if ! rendered="$(helm template test-release "$CHART_DIR" \
    --show-only "$tmpl" "$@" 2>&1)"; then
    printf 'helm template failed: %s\n' "$rendered" >&2
    return 1
  fi
  awk -v n="$name" '
    $1 == "-" && $2 == "name:" { in_var = ($3 == n); next }
    in_var && $1 == "value:" {
      sub(/^[[:space:]]*value:[[:space:]]*/, "")
      sub(/[[:space:]]+$/, "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  ' <<<"$rendered"
}

assert_env() {
  local label="$1" tmpl="$2" key="$3" expected="$4"
  shift 4
  local actual
  actual="$(env_value "$tmpl" "$key" "$@")"
  if [[ "$expected" == "$actual" ]]; then
    printf 'ok   - %s\n' "$label"
  else
    printf 'FAIL - %s\n      expected %s: %q\n      actual   %s: %q\n' \
      "$label" "$key" "$expected" "$key" "$actual"
    FAILURES=$((FAILURES + 1))
  fi
}

API_TMPL=templates/agent-manager-service/deployment.yaml
MIG_TMPL=templates/jobs/db-migration-job.yaml
EXTERNAL=(--set postgresql.enabled=false --set postgresql.external.host=db.example.com)

# The database connection carries no TLS settings unless asked for, so the DSN
# stays byte-identical to pre-DB_SSL_MODE builds and pgx keeps its "prefer"
# default. Both workloads connect, so both must agree.
assert_env "default renders no SSL mode for the API" "$API_TMPL" DB_SSL_MODE ""
assert_env "default renders no SSL mode for the migration job" "$MIG_TMPL" DB_SSL_MODE ""
assert_env "external without an SSL mode renders none for the API" \
  "$API_TMPL" DB_SSL_MODE "" "${EXTERNAL[@]}"
assert_env "external without an SSL mode renders none for the migration job" \
  "$MIG_TMPL" DB_SSL_MODE "" "${EXTERNAL[@]}"

# A configured mode has to reach the migration job too: it runs the same binary
# against the same database, so a mode set only on the API would leave the
# post-install migration hook connecting in plaintext.
assert_env "sslMode reaches the API" "$API_TMPL" DB_SSL_MODE "require" \
  "${EXTERNAL[@]}" --set postgresql.external.sslMode=require
assert_env "sslMode reaches the migration job" "$MIG_TMPL" DB_SSL_MODE "require" \
  "${EXTERNAL[@]}" --set postgresql.external.sslMode=require
assert_env "sslRootCert reaches the API" "$API_TMPL" DB_SSL_ROOT_CERT "system" \
  "${EXTERNAL[@]}" --set postgresql.external.sslMode=verify-full \
  --set postgresql.external.sslRootCert=system
assert_env "sslRootCert reaches the migration job" "$MIG_TMPL" DB_SSL_ROOT_CERT "system" \
  "${EXTERNAL[@]}" --set postgresql.external.sslMode=verify-full \
  --set postgresql.external.sslRootCert=system

# Migration 043 derives a full URL for existing handle-only rows. The hook must
# receive the same deployment origin settings as the API or a TLS/custom-domain
# upgrade would backfill localhost URLs even though live writes are correct.
assert_env "IDP base domain reaches the migration job" \
  "$MIG_TMPL" IDP_HOST_BASE_DOMAIN "amp.example.com" \
  --set agentManagerService.config.idpHostBaseDomain=amp.example.com
assert_env "TLS setting reaches the migration job" \
  "$MIG_TMPL" TLS_ENABLED "true" \
  --set agentManagerService.config.tlsEnabled=true

# The bundled PostgreSQL serves plaintext, so an sslMode left over from an
# external configuration must not be applied to it — that would fail the API pod
# and the migration hook on an otherwise working default install.
assert_env "sslMode is ignored for the in-cluster database" \
  "$API_TMPL" DB_SSL_MODE "" --set postgresql.external.sslMode=require

# secret_ref <template-path> <env-name> <name|key> [helm --set args...] -> the
# requested field from that env var's secretKeyRef.
secret_ref() {
  local tmpl="$1" name="$2" field="$3" rendered
  shift 3
  if ! rendered="$(helm template test-release "$CHART_DIR" \
    --show-only "$tmpl" "$@" 2>&1)"; then
    printf 'helm template failed: %s\n' "$rendered" >&2
    return 1
  fi
  awk -v n="$name" -v f="$field" '
    $1 == "-" && $2 == "name:" { in_var = ($3 == n); next }
    in_var && $1 == f":" {
      value = $2
      gsub(/^"|"$/, "", value)
      print value
      exit
    }
  ' <<<"$rendered"
}

assert_secret_ref() {
  local label="$1" tmpl="$2" env_name="$3" field="$4" expected="$5"
  shift 5
  local actual
  actual="$(secret_ref "$tmpl" "$env_name" "$field" "$@")"
  if [[ "$expected" == "$actual" ]]; then
    printf 'ok   - %s\n' "$label"
  else
    printf 'FAIL - %s\n      expected secretKeyRef.%s: %q\n      actual   secretKeyRef.%s: %q\n' \
      "$label" "$field" "$expected" "$field" "$actual"
    FAILURES=$((FAILURES + 1))
  fi
}

# secret_has_key <key> [helm --set args...] -> yes when the chart-managed
# agent-manager Secret emits that stringData key.
secret_has_key() {
  local key="$1" rendered
  shift
  if ! rendered="$(helm template test-release "$CHART_DIR" \
    --show-only templates/agent-manager-service/secret.yaml "$@" 2>&1)"; then
    printf 'helm template failed: %s\n' "$rendered" >&2
    return 1
  fi
  if awk -v k="$key" '
      $1 == "stringData:" { in_data = 1; next }
      in_data && $1 == k":" { found = 1 }
      END { exit(found ? 0 : 1) }
    ' <<<"$rendered"; then
    printf 'yes\n'
  else
    printf 'no\n'
  fi
}

assert_secret_key() {
  local label="$1" key="$2" expected="$3"
  shift 3
  local actual
  actual="$(secret_has_key "$key" "$@")"
  if [[ "$expected" == "$actual" ]]; then
    printf 'ok   - %s\n' "$label"
  else
    printf 'FAIL - %s\n      expected key %q present: %s\n      actual: %s\n' \
      "$label" "$key" "$expected" "$actual"
    FAILURES=$((FAILURES + 1))
  fi
}

# existingSecretKey belongs to an external Secret. Without existingSecret, the
# chart-managed Secret retains its stable built-in keys and every consumer must
# reference those keys even if a stray custom key value is supplied.
assert_secret_key "generated Secret retains the fixed API key" api-key yes \
  --set agentManagerService.config.apiKey.existingSecretKey=custom-api-key
assert_secret_key "generated Secret does not emit the external-only custom API key" custom-api-key no \
  --set agentManagerService.config.apiKey.existingSecretKey=custom-api-key
assert_secret_ref "generated API Secret ignores an external-only custom key in the API" \
  "$API_TMPL" API_KEY_VALUE key api-key \
  --set agentManagerService.config.apiKey.existingSecretKey=custom-api-key
assert_secret_ref "generated API Secret ignores an external-only custom key in migrations" \
  "$MIG_TMPL" API_KEY_VALUE key api-key \
  --set agentManagerService.config.apiKey.existingSecretKey=custom-api-key
assert_secret_ref "external API Secret honors its custom key in the API" \
  "$API_TMPL" API_KEY_VALUE key custom-api-key \
  --set agentManagerService.config.apiKey.existingSecret=external-api \
  --set agentManagerService.config.apiKey.existingSecretKey=custom-api-key
assert_secret_ref "external API Secret honors its custom key in migrations" \
  "$MIG_TMPL" API_KEY_VALUE key custom-api-key \
  --set agentManagerService.config.apiKey.existingSecret=external-api \
  --set agentManagerService.config.apiKey.existingSecretKey=custom-api-key
assert_secret_ref "generated GitHub Secret ignores an external-only custom key" \
  "$API_TMPL" GITHUB_TOKEN key github-token \
  --set agentManagerService.config.github.existingSecretKey=custom-github-key
assert_secret_ref "external GitHub Secret honors its custom key" \
  "$API_TMPL" GITHUB_TOKEN key custom-github-key \
  --set agentManagerService.config.github.existingSecret=external-github \
  --set agentManagerService.config.github.existingSecretKey=custom-github-key
assert_secret_ref "generated encryption Secret ignores an external-only custom key" \
  "$API_TMPL" ENCRYPTION_KEY key encryption-key \
  --set agentManagerService.config.encryptionKey.existingSecretKey=custom-encryption-key
assert_secret_ref "external encryption Secret honors its custom key" \
  "$API_TMPL" ENCRYPTION_KEY key custom-encryption-key \
  --set agentManagerService.config.encryptionKey.existingSecret=external-encryption \
  --set agentManagerService.config.encryptionKey.existingSecretKey=custom-encryption-key

# flatten_items <block-key> — reads YAML on stdin and prints one line per list
# item in the first block with that key, fields joined by "; ". Flattening is
# what lets a check require two fields on the SAME item, which independent greps
# over the whole document cannot express.
flatten_items() {
  awk -v key="$1" '
    function ind(s) { match(s, /^ */); return RLENGTH }
    !inblk && $0 ~ "^ *"key":[ ]*$" { inblk = 1; bi = ind($0); next }
    inblk {
      if ($0 ~ /^[ ]*$/) { next }
      if (ind($0) <= bi) { if (buf != "") print buf; exit }
      if ($0 ~ /^ *- /) { if (buf != "") print buf; buf = "" }
      line = $0
      sub(/^ *-? */, "", line)
      sub(/ +$/, "", line)
      buf = (buf == "" ? line : buf "; " line)
    }
    END { if (buf != "") print buf }
  '
}

# mounts_ca <template-path> [helm --set args...] -> "yes" only when the container
# has a volumeMounts item carrying BOTH the volume name and the mount path, and
# the pod spec declares a volume of that name. A template emitting one without
# the other is broken, so both are required rather than matched independently.
mounts_ca() {
  local tmpl="$1" rendered
  shift
  if ! rendered="$(helm template test-release "$CHART_DIR" \
    --show-only "$tmpl" "$@" 2>&1)"; then
    printf 'helm template failed: %s\n' "$rendered" >&2
    return 1
  fi
  if flatten_items volumeMounts <<<"$rendered" \
       | grep 'name: db-ca' | grep -q 'mountPath: /etc/db-ca' \
     && flatten_items volumes <<<"$rendered" | grep -q 'name: db-ca'; then
    printf 'yes\n'
  else
    printf 'no\n'
  fi
}

assert_ca_mount() {
  local label="$1" tmpl="$2" expected="$3"
  shift 3
  local actual
  actual="$(mounts_ca "$tmpl" "$@")"
  if [[ "$expected" == "$actual" ]]; then
    printf 'ok   - %s\n' "$label"
  else
    printf 'FAIL - %s\n      expected %q, got %q\n' "$label" "$expected" "$actual"
    FAILURES=$((FAILURES + 1))
  fi
}

# verify-ca/verify-full against a private CA needs the PEM readable by every
# process that opens a connection. The migration job runs as a post-install hook,
# so a CA reaching only the API would fail the install with x509 "unknown
# authority" rather than at request time.
CA_VOLUME=(
  --set 'volumes[0].name=db-ca'
  --set 'volumes[0].secret.secretName=db-ca'
  --set 'volumeMounts[0].name=db-ca'
  --set 'volumeMounts[0].mountPath=/etc/db-ca'
  --set 'volumeMounts[0].readOnly=true'
)
assert_ca_mount "a private CA volume reaches the API" "$API_TMPL" yes "${CA_VOLUME[@]}"
assert_ca_mount "a private CA volume reaches the migration job" "$MIG_TMPL" yes "${CA_VOLUME[@]}"
assert_ca_mount "no CA volume is invented for the migration job by default" "$MIG_TMPL" no

if ((FAILURES > 0)); then
  printf '\n%d assertion(s) failed\n' "$FAILURES"
  exit 1
fi
printf '\nAll render assertions passed\n'
