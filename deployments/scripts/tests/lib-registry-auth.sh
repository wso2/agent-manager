#!/usr/bin/env bash
# Unit tests for deployments/scripts/lib-registry-auth.sh.
# Run: bash deployments/scripts/tests/lib-registry-auth.sh
#
# Covers the parts that are silent when wrong: the host derivation (a pull
# secret scoped to the wrong host is ignored rather than rejected), the
# credentials-file parser (which must not execute its input), and the public
# default emitting no helm flags at all.
set -uo pipefail

LIB="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib-registry-auth.sh"
FAILURES=0
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

assert_eq() {
    local label="$1" expected="$2" actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        printf 'ok   - %s\n' "$label"
    else
        printf 'FAIL - %s\n      expected: %q\n      actual:   %q\n' "$label" "$expected" "$actual"
        FAILURES=$((FAILURES + 1))
    fi
}

# Each case runs in a subshell so one case's exports cannot leak into the next.
host_for() { ( unset AMP_IMAGE_REGISTRY; export AMP_IMAGE_REGISTRY="$1"; . "$LIB"; amp_registry_host ); }

assert_eq "public default host" "ghcr.io" "$( ( unset AMP_IMAGE_REGISTRY; . "$LIB"; amp_registry_host ) )"
assert_eq "host is split from the organization" "registry.example.com" "$(host_for registry.example.com/my-org)"
assert_eq "a nested project path keeps only the host" "registry.example.com" "$(host_for registry.example.com/a/b/c)"
assert_eq "a host-only value is returned as-is" "registry.example.com" "$(host_for registry.example.com)"
assert_eq "a port survives the split" "localhost:5000" "$(host_for localhost:5000/org)"

# The public default must produce no helm flags, so every existing install
# command stays byte-identical.
assert_eq "public registry emits no helm args" "" \
  "$( ( unset AMP_IMAGE_REGISTRY; . "$LIB"; amp_pull_secret_helm_args ) )"
assert_eq "private registry emits both helm args" \
  "--set global.ampImageRegistry=registry.example.com/my-org --set global.imagePullSecrets[0]=wso2-registry-credentials" \
  "$( ( export AMP_IMAGE_REGISTRY=registry.example.com/my-org; . "$LIB"; amp_pull_secret_helm_args | tr '\n' ' ' | sed 's/ $//' ) )"
# The charts take imagePullSecrets as a list of plain strings, not objects, so
# the flag must be [0]=name. [0].name= renders an empty entry and no secret.
assert_eq "the pull-secret flag targets a string list, not an object" \
  "global.imagePullSecrets[0]=wso2-registry-credentials" \
  "$( ( export AMP_IMAGE_REGISTRY=r.example.com/org; . "$LIB"; amp_pull_secret_helm_args | tail -1 ) )"

# Credentials file: parsed, not sourced.
printf 'AMP_REGISTRY_USERNAME=robot$proj+ci\nAMP_REGISTRY_PASSWORD=tok=with=equals\n' > "$TMP/creds"
chmod 600 "$TMP/creds"
read -r U P < <( ( export AMP_REGISTRY_ALLOW_INSECURE=true AMP_REGISTRY_CREDENTIALS_FILE="$TMP/creds"
                   unset AMP_REGISTRY_USERNAME AMP_REGISTRY_PASSWORD
                   . "$LIB"; amp_registry_load_credentials >/dev/null 2>&1
                   printf '%s %s\n' "$AMP_REGISTRY_USERNAME" "$AMP_REGISTRY_PASSWORD" ) )
assert_eq "username is read from the credentials file" 'robot$proj+ci' "$U"
assert_eq "a password containing '=' survives parsing" "tok=with=equals" "$P"

# `read` returns non-zero on a final line with no trailing newline, so a naive
# loop drops it — and the dropped one is whichever the operator wrote last.
printf 'AMP_REGISTRY_USERNAME=u1\nAMP_REGISTRY_PASSWORD=p-no-newline' > "$TMP/nonl"
chmod 600 "$TMP/nonl"
assert_eq "a final line without a newline is still read" "p-no-newline" \
  "$( ( export AMP_REGISTRY_ALLOW_INSECURE=true AMP_REGISTRY_CREDENTIALS_FILE="$TMP/nonl"
        unset AMP_REGISTRY_USERNAME AMP_REGISTRY_PASSWORD
        . "$LIB"; amp_registry_load_credentials >/dev/null 2>&1
        printf '%s' "$AMP_REGISTRY_PASSWORD" ) )"

# The containerd fragment puts these into double-quoted YAML scalars, so a
# password containing a quote or a backslash would either break the document or
# silently reach the node altered. Asserted one character at a time: a combined
# case is unreadable through two layers of shell quoting.
frag_password() {
  ( export AMP_REGISTRY_ALLOW_INSECURE=true AMP_IMAGE_REGISTRY=registry.example.com/org \
           AMP_REGISTRY_USERNAME='u' AMP_REGISTRY_PASSWORD="$1"
    . "$LIB"; amp_registries_yaml_fragment ) | grep 'password:'
}
assert_eq "a double quote is escaped for YAML" \
  '          password: "a\"b"' "$(frag_password 'a"b')"
assert_eq "a backslash is doubled for YAML" \
  '          password: "a\\b"' "$(frag_password 'a\b')"

# A double-quoted YAML scalar cannot carry these literally: one in a credential
# would change the value the node receives, or invalidate the k3d config.
assert_eq "a tab is escaped for YAML" \
  '          password: "a\tb"' "$(frag_password "$(printf 'a\tb')")"
assert_eq "a carriage return is escaped for YAML" \
  '          password: "a\rb"' "$(frag_password "$(printf 'a\rb')")"

# A credentials file written on Windows leaves a trailing CR, which would
# otherwise be an invisible character in the password that simply fails to
# authenticate.
printf 'AMP_REGISTRY_USERNAME=u\r\nAMP_REGISTRY_PASSWORD=p-crlf\r\n' > "$TMP/crlf"
chmod 600 "$TMP/crlf"
assert_eq "a CRLF credentials file yields a clean password" "p-crlf" \
  "$( ( export AMP_REGISTRY_ALLOW_INSECURE=true AMP_REGISTRY_CREDENTIALS_FILE="$TMP/crlf"
        unset AMP_REGISTRY_USERNAME AMP_REGISTRY_PASSWORD
        . "$LIB"; amp_registry_load_credentials >/dev/null 2>&1
        printf '%s' "$AMP_REGISTRY_PASSWORD" ) )"


# A credentials file must never be able to run code.
printf 'AMP_REGISTRY_USERNAME=safe\nAMP_REGISTRY_PASSWORD=safe\n' > "$TMP/evil"
printf 'touch %q/PWNED\n' "$TMP" >> "$TMP/evil"
chmod 600 "$TMP/evil"
( export AMP_REGISTRY_ALLOW_INSECURE=true AMP_REGISTRY_CREDENTIALS_FILE="$TMP/evil"
  unset AMP_REGISTRY_USERNAME AMP_REGISTRY_PASSWORD
  . "$LIB"; amp_registry_load_credentials >/dev/null 2>&1 ) || true
if [[ -e "$TMP/PWNED" ]]; then
    printf 'FAIL - a credentials file executed its contents\n'
    FAILURES=$((FAILURES + 1))
else
    printf 'ok   - a credentials file is parsed, not executed\n'
fi

# The environment wins over the file, so a CI secret is not silently overridden.
assert_eq "environment credentials win over the file" "from-env" \
  "$( ( export AMP_REGISTRY_ALLOW_INSECURE=true AMP_REGISTRY_CREDENTIALS_FILE="$TMP/creds" AMP_REGISTRY_USERNAME=from-env AMP_REGISTRY_PASSWORD=x
        . "$LIB"; amp_registry_load_credentials >/dev/null 2>&1; printf '%s' "$AMP_REGISTRY_USERNAME" ) )"

# Non-interactive and no credentials must fail, not block on a prompt.
if ( unset AMP_REGISTRY_USERNAME AMP_REGISTRY_PASSWORD AMP_REGISTRY_CREDENTIALS_FILE
     . "$LIB"; amp_registry_load_credentials </dev/null >/dev/null 2>&1 ); then
    printf 'FAIL - missing credentials should fail without a terminal\n'
    FAILURES=$((FAILURES + 1))
else
    printf 'ok   - missing credentials fail without a terminal\n'
fi

# The containerd fragment must sit at the indentation of the k3d config's
# `config: |` literal, or the node's registries.yaml is malformed.
FRAG="$( ( export AMP_REGISTRY_ALLOW_INSECURE=true AMP_IMAGE_REGISTRY=registry.example.com/my-org \
                  AMP_REGISTRY_USERNAME='robot$proj+ci' AMP_REGISTRY_PASSWORD=tok
           . "$LIB"; amp_registries_yaml_fragment ) )"
assert_eq "the fragment is keyed on the host alone" '    configs:
      "registry.example.com":
        auth:
          username: "robot$proj+ci"
          password: "tok"' "$FRAG"

# Both installers test amp_create_pull_secret in an `if`, which disables errexit
# inside it, so a cleanup step as the last command would report success over a
# Secret that was never created. Stub kubectl to fail and require a non-zero rc.
STUB="$TMP/stub"; mkdir -p "$STUB"
printf '#!/bin/sh\nexit 1\n' > "$STUB/kubectl"; chmod +x "$STUB/kubectl"
if ( export PATH="$STUB:$PATH" AMP_REGISTRY_ALLOW_INSECURE=true \
            AMP_IMAGE_REGISTRY=registry.example.com/org \
            AMP_REGISTRY_USERNAME=u AMP_REGISTRY_PASSWORD=p
     . "$LIB"; amp_create_pull_secret ns-a ns-b >/dev/null 2>&1 ); then
    printf 'FAIL - a failed kubectl was reported as success\n'
    FAILURES=$((FAILURES + 1))
else
    printf 'ok   - a failed secret create returns non-zero\n'
fi

# A credential must not be sent to an endpoint whose TLS this host does not
# trust. docker cannot be relied on for that: a daemon with insecure-registries
# configured sends it to plain HTTP without complaint.
STUB2="$TMP/stub2"; mkdir -p "$STUB2"
# curl reports 000 when the handshake itself failed.
printf '#!/bin/sh\nprintf 000\nexit 35\n' > "$STUB2/curl"; chmod +x "$STUB2/curl"
if ( export PATH="$STUB2:$PATH" AMP_IMAGE_REGISTRY=registry.example.com/org
     . "$LIB"; amp_registry_verify_tls >/dev/null 2>&1 ); then
    printf 'FAIL - an untrusted TLS endpoint was accepted\n'
    FAILURES=$((FAILURES + 1))
else
    printf 'ok   - an untrusted TLS endpoint is refused\n'
fi
# 401 is what an authenticated registry answers; the handshake still verified.
printf '#!/bin/sh\nprintf 401\nexit 22\n' > "$STUB2/curl"; chmod +x "$STUB2/curl"
if ( export PATH="$STUB2:$PATH" AMP_IMAGE_REGISTRY=registry.example.com/org
     . "$LIB"; amp_registry_verify_tls >/dev/null 2>&1 ); then
    printf 'ok   - a 401 over verified TLS is accepted\n'
else
    printf 'FAIL - a 401 over verified TLS was refused\n'
    FAILURES=$((FAILURES + 1))
fi
# The opt-out exists for a mirror deliberately served over plain HTTP.
printf '#!/bin/sh\nprintf 000\nexit 35\n' > "$STUB2/curl"; chmod +x "$STUB2/curl"
if ( export PATH="$STUB2:$PATH" AMP_IMAGE_REGISTRY=registry.example.com/org \
            AMP_REGISTRY_ALLOW_INSECURE=true
     . "$LIB"; amp_registry_verify_tls >/dev/null 2>&1 ); then
    printf 'ok   - the explicit insecure opt-out is honoured\n'
else
    printf 'FAIL - the insecure opt-out did not work\n'
    FAILURES=$((FAILURES + 1))
fi

# --- amp_registry_helm_login --------------------------------------------------
# helm is stubbed to record its argv and stdin, so the cases below show both
# when it is called and that the token travels on stdin rather than argv.
STUB3="$TMP/stub3"; mkdir -p "$STUB3"
cat > "$STUB3/helm" <<EOF
#!/bin/sh
printf '%s\n' "\$*" > "$TMP/helm.args"
cat > "$TMP/helm.stdin"
EOF
chmod +x "$STUB3/helm"
helm_login() {
    rm -f "$TMP/helm.args" "$TMP/helm.stdin"
    ( export PATH="$STUB3:$PATH" AMP_REGISTRY_ALLOW_INSECURE=true \
             AMP_REGISTRY_USERNAME='robot$proj+ci' AMP_REGISTRY_PASSWORD=s3cret "$@"
      . "$LIB"; amp_registry_helm_login >/dev/null 2>&1 )
}
helm_login AMP_IMAGE_REGISTRY=registry.example.com/proj HELM_CHART_REGISTRY=registry.example.com/proj
assert_eq "charts on the image host log helm in to that host" \
    'registry login registry.example.com --username robot$proj+ci --password-stdin' "$(cat "$TMP/helm.args" 2>/dev/null)"
assert_eq "the token reaches helm on stdin" "s3cret" "$(cat "$TMP/helm.stdin" 2>/dev/null)"
helm_login AMP_IMAGE_REGISTRY=registry.example.com/proj HELM_CHART_REGISTRY=oci://registry.example.com/proj
assert_eq "an oci:// chart registry resolves to the same host" \
    'registry login registry.example.com --username robot$proj+ci --password-stdin' "$(cat "$TMP/helm.args" 2>/dev/null)"
helm_login AMP_IMAGE_REGISTRY=registry.example.com/proj HELM_CHART_REGISTRY=ghcr.io/wso2
assert_eq "charts on another host are left alone" "" "$(cat "$TMP/helm.args" 2>/dev/null)"
helm_login HELM_CHART_REGISTRY=ghcr.io/wso2
assert_eq "a public install never calls helm" "" "$(cat "$TMP/helm.args" 2>/dev/null)"

if ((FAILURES > 0)); then
    printf '\n%d assertion(s) failed\n' "$FAILURES"
    exit 1
fi
printf '\nAll lib-registry-auth assertions passed\n'
