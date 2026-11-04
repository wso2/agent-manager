#!/usr/bin/env bash
# Render assertions for wso2-amp-platform-resources-extension.
# Run: bash deployments/helm-charts/wso2-amp-platform-resources-extension/tests/render.sh
#
# Covers the shell quoting of the build steps. Argo escapes a substituted
# parameter for JSON, never for the shell, so a parameter carrying user text that
# is interpolated into a step's script becomes shell source: one apostrophe in a
# file mount closes the string early and the rest of the script is reparsed as
# code. The build then dies on a bare `(` far from the real cause and Argo
# reports only a missing output artifact (see issue #1639). A separator such as
# `;` needs no quote at all -- it just runs. The values have to reach the
# container as env vars, which the shell never parses.
#
# Three groups of assertions: the amp-generate-workload step it started with,
# then checkout-source, where the repository URL, branch and commit arrive the
# same way, then the whole chart, since every build step runs `sh -c` over a
# block scalar and the property has to hold for all of them.
set -uo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKLOAD_TMPL=templates/cluster-workflow-templates/generate-workload.yaml
FAILURES=0

if ! RENDERED="$(helm template test-release "$CHART_DIR" --show-only "$WORKLOAD_TMPL" 2>&1)"; then
  printf 'FAIL - helm template failed\n%s\n' "$RENDERED"
  exit 1
fi

# script_body — prints the container args block scalar, dedented. Everything in
# it is shell source, so a parameter reaching this text is a parameter the shell
# parses.
script_body() {
  awk '
    function ind(s) { match(s, /^ */); return RLENGTH }
    !found && $0 ~ /^ *- \|-?$/ { found = 1; bi = ind($0); next }
    found {
      if ($0 ~ /^[ ]*$/) { print ""; next }
      if (ind($0) <= bi) { exit }
      if (ci == 0) ci = ind($0)
      print substr($0, ci + 1)
    }
  ' <<<"$RENDERED"
}

SCRIPT="$(script_body)"

if [[ -z "$SCRIPT" ]]; then
  printf 'FAIL - could not extract the generate-workload script from %s\n' "$WORKLOAD_TMPL"
  exit 1
fi

# resolve — reads the script on stdin and substitutes the Argo placeholders the
# way the controller would, taking each parameter value from V_* in the
# environment. Placeholders with no V_* entry become an inert token, so a check
# reports the quoting defect and not an unrelated empty expansion.
resolve() {
  awk '
    function replace(s, from, to,   out, p) {
      while ((p = index(s, from)) > 0) {
        out = out substr(s, 1, p - 1) to
        s = substr(s, p + length(from))
      }
      return out s
    }
    BEGIN {
      n = 0
      keys[++n] = "{{workflow.parameters.file-mounts}}";           vals[n] = ENVIRON["V_FILE_MOUNTS"]
      keys[++n] = "{{workflow.parameters.environment-variables}}"; vals[n] = ENVIRON["V_ENV_VARS"]
      keys[++n] = "{{workflow.parameters.endpoints}}";             vals[n] = ENVIRON["V_ENDPOINTS"]
      keys[++n] = "{{workflow.parameters.app-path}}";              vals[n] = ENVIRON["V_APP_PATH"]
    }
    {
      line = $0
      for (i = 1; i <= n; i++) line = replace(line, keys[i], vals[i])
      while ((a = index(line, "{{")) > 0) {
        rest = substr(line, a + 2)
        b = index(rest, "}}")
        if (b == 0) break
        line = substr(line, 1, a - 1) "placeholder" substr(rest, b + 2)
      }
      print line
    }
  '
}

# Benign values for the parameters a case is not exercising. Exactly one hostile
# value per case: two stray apostrophes re-balance the quoting and the script
# parses again while silently carrying mangled data, which would report a defect
# as fixed.
BENIGN_ENDPOINTS='[]'
BENIGN_ENV_VARS='[{"name":"GREETING","value":"hello"}]'
BENIGN_FILE_MOUNTS='[{"key":"policy","mountPath":"/etc/policy.txt","value":"plain"}]'
BENIGN_APP_PATH='.'

# assert_parses <label> <endpoints> <env-vars> <file-mounts> <app-path>
# Checked with sh, which is what the step's container runs the script under, so a
# bashism that a bash-backed image happens to tolerate cannot pass unnoticed.
assert_parses() {
  local label="$1" err
  err="$(V_ENDPOINTS="$2" V_ENV_VARS="$3" V_FILE_MOUNTS="$4" V_APP_PATH="$5" \
    resolve <<<"$SCRIPT" | sh -n 2>&1)"
  if [[ -z "$err" ]]; then
    printf 'ok   - generate-workload parses with %s\n' "$label"
  else
    printf 'FAIL - %s breaks generate-workload\n      %s\n' "$label" "$err"
    FAILURES=$((FAILURES + 1))
  fi
}

# The literal case that broke a real build was a cancellation policy reading
# "charged one night's rate". The other two carry the same user text through
# different parameters, and a build path is exercised with the double quote that
# would close the one string it is interpolated into.
assert_parses "an apostrophe in a file mount" \
  "$BENIGN_ENDPOINTS" "$BENIGN_ENV_VARS" \
  '[{"key":"policy","mountPath":"/etc/policy.txt","value":"charged one night'\''s rate"}]' \
  "$BENIGN_APP_PATH"
assert_parses "an apostrophe in an environment variable" \
  "$BENIGN_ENDPOINTS" '[{"name":"GREETING","value":"it'\''s here"}]' \
  "$BENIGN_FILE_MOUNTS" "$BENIGN_APP_PATH"
assert_parses "an apostrophe in an endpoint schema" \
  '[{"name":"api","port":8080,"basePath":"/","visibility":["Public"],"schemaContent":"summary: the user'\''s profile"}]' \
  "$BENIGN_ENV_VARS" "$BENIGN_FILE_MOUNTS" "$BENIGN_APP_PATH"
assert_parses "a double quote in the build path" \
  "$BENIGN_ENDPOINTS" "$BENIGN_ENV_VARS" "$BENIGN_FILE_MOUNTS" 'app"dir'

# env_value <name> — the container env value for that variable, empty when the
# variable is not rendered at all.
env_value() {
  awk -v n="$1" '
    $1 == "-" && $2 == "name:" { in_var = ($3 == n); next }
    in_var && $1 == "value:" {
      sub(/^[[:space:]]*value:[[:space:]]*/, "")
      sub(/[[:space:]]+$/, "")
      gsub(/^'\''|'\''$/, "")
      gsub(/^"|"$/, "")
      print
      exit
    }
  ' <<<"$RENDERED"
}

# The parameters that carry arbitrary user text — OpenAPI schemas, environment
# variable values, file mount contents, and a build path — must be delivered
# through the pod spec rather than pasted into the script. A syntax check alone
# would pass on an even number of stray quotes, so this is what keeps the values
# out of the shell's reach in the first place.
assert_param_via_env() {
  local var="$1" param="$2"
  if grep -qF "{{$param}}" <<<"$SCRIPT"; then
    printf 'FAIL - %s is interpolated into the script instead of reaching it as $%s\n' "$param" "$var"
    FAILURES=$((FAILURES + 1))
    return
  fi
  local actual
  actual="$(env_value "$var")"
  if [[ "$actual" == "{{$param}}" ]]; then
    printf 'ok   - %s reaches the container as $%s\n' "$param" "$var"
  else
    printf 'FAIL - $%s does not carry %s\n      actual: %q\n' "$var" "$param" "$actual"
    FAILURES=$((FAILURES + 1))
  fi
}

assert_param_via_env ENDPOINTS workflow.parameters.endpoints
assert_param_via_env ENV_VARS workflow.parameters.environment-variables
assert_param_via_env FILE_MOUNTS workflow.parameters.file-mounts
assert_param_via_env APP_PATH workflow.parameters.app-path

# The defect was one quoting style in one step; this is the pattern itself, so a
# parameter added later in the same shape is caught before it ships.
QUOTED_ASSIGNMENT="$(grep -nE "^[A-Z_]+='\{\{" <<<"$SCRIPT")"
if [[ -z "$QUOTED_ASSIGNMENT" ]]; then
  printf 'ok   - no Argo parameter is assigned inside a single-quoted string\n'
else
  printf 'FAIL - an Argo parameter is assigned inside a single-quoted string\n%s\n' "$QUOTED_ASSIGNMENT"
  FAILURES=$((FAILURES + 1))
fi

# --- generate-workload path containment (arbitrary file read) ----------------
# A schema file is read with jq --rawfile and its bytes land in the Workload CR,
# which the caller can read back, so the read must be confined to the checkout.
# The confinement has to be anchored to the fixed checkout directory: anchoring it
# to a realpath of the caller-supplied appPath let a symlink committed in the
# repository move the trusted root (e.g. to /proc) so a schema path under that
# target passed the "inside the checkout" check and read a file outside it. These
# guard that the root stays fixed and both appPath and the schema file are
# confined to it.
if grep -qE 'CHECKOUT_ROOT=\$\(realpath /mnt/vol/source' <<<"$SCRIPT"; then
  printf 'ok   - the checkout root is anchored to the fixed source directory\n'
else
  printf 'FAIL - the checkout root is not anchored to /mnt/vol/source\n'
  FAILURES=$((FAILURES + 1))
fi

if grep -qF '"$SOURCE_ROOT"' <<<"$SCRIPT"; then
  printf 'FAIL - containment still compares against a root derived from appPath\n'
  FAILURES=$((FAILURES + 1))
else
  printf 'ok   - containment does not use an appPath-derived root\n'
fi

if [[ "$(grep -cF '"$CHECKOUT_ROOT"/*' <<<"$SCRIPT")" -ge 2 ]]; then
  printf 'ok   - appPath and the schema file are confined to the fixed checkout root\n'
else
  printf 'FAIL - appPath and the schema file are not both confined to the fixed checkout root\n'
  FAILURES=$((FAILURES + 1))
fi

# --- generate-workload Step 7: the build's health checks on the binding --------
# Health checks belong to the build: Step 7 reads them from the build's own
# WorkflowRun and writes them in the same update as the release pin, for a new
# binding and an existing one alike, removing an earlier build's when this build
# has none. Everything else an existing binding holds (env vars, runtime class)
# belongs to the environment and must come through untouched. These run the real
# script under sh against a stubbed API server and check the binding body it sends.
STEP7_DIR="$(mktemp -d)"
trap 'rm -rf "$STEP7_DIR"' EXIT
mkdir -p "$STEP7_DIR/bin" "$STEP7_DIR/vol/source"

# The script hardcodes its scratch volume; point it at the temp dir instead.
sed "s#/mnt/vol#$STEP7_DIR/vol#g" <<<"$SCRIPT" >"$STEP7_DIR/script.sh"

printf '#!/bin/sh\nexit 0\n' >"$STEP7_DIR/bin/apk"
printf '#!/bin/sh\nexit 0\n' >"$STEP7_DIR/bin/podman"
# curl stub: answers each API route the script calls and records the body of the
# ReleaseBinding write. BUILD_RECORD names the file holding this build's
# WorkflowRun; EXISTING_BINDING names the file holding the binding the GET
# returns, or is empty when the binding does not exist yet.
cat >"$STEP7_DIR/bin/curl" <<'STUB'
#!/bin/sh
method=GET url="" data="" code_fmt=""
while [ $# -gt 0 ]; do
  case "$1" in
    -X) method="$2"; shift ;;
    -d) data="$2"; shift ;;
    -w) code_fmt=1; shift ;;
    -H) shift ;;
    http*) url="$1" ;;
  esac
  shift
done
reply() { printf '%s' "$2"; [ -n "$code_fmt" ] && printf '\n%s' "$1"; exit 0; }
case "$method $url" in
  "POST http://token"*) reply 200 '{"access_token":"t"}' ;;
  "GET "*"/workflowruns/"*) reply 200 "$(cat "$BUILD_RECORD")" ;;
  "PUT "*"/workflowruns/"*) reply 200 '{}' ;;
  "POST "*"/workloads") reply 201 '{}' ;;
  "POST "*"/generate-release") reply 201 '{}' ;;
  "GET "*"/projects/"*) reply 200 '{"spec":{"deploymentPipelineRef":{"name":"pipeline"}}}' ;;
  "GET "*"/deploymentpipelines/"*)
    reply 200 '{"spec":{"promotionPaths":[{"sourceEnvironmentRef":{"name":"dev"},"targetEnvironmentRefs":[{"name":"prod"}]}]}}' ;;
  "GET "*"/releasebindings/"*)
    if [ -n "$EXISTING_BINDING" ]; then reply 200 "$(cat "$EXISTING_BINDING")"; fi
    reply 404 '{}' ;;
  "POST "*"/releasebindings"|"PUT "*"/releasebindings/"*)
    cp "${data#@}" "$RECORDED_BINDING"
    reply 200 '{}' ;;
esac
reply 500 "unexpected $method $url"
STUB
chmod +x "$STEP7_DIR/bin/"*

# run_step7 <build's healthChecks JSON, or "" for none> <existing binding JSON, or ""
# for none> — runs the script and prints the binding body it wrote, or nothing if the
# run failed (the script's output is then in $STEP7_DIR/out.log).
run_step7() {
  local existing=""
  rm -f "$STEP7_DIR/recorded.json"
  if [[ -n "$1" ]]; then
    printf '{"metadata":{"name":"run-2"},"spec":{"workflow":{"parameters":{"healthChecks":%s}}}}' "$1" >"$STEP7_DIR/record.json"
  else
    printf '{"metadata":{"name":"run-2"},"spec":{"workflow":{"parameters":{}}}}' >"$STEP7_DIR/record.json"
  fi
  if [[ -n "$2" ]]; then
    existing="$STEP7_DIR/existing.json"
    printf '%s' "$2" >"$existing"
  fi
  if ! env PATH="$STEP7_DIR/bin:$PATH" \
      BUILD_RECORD="$STEP7_DIR/record.json" EXISTING_BINDING="$existing" RECORDED_BINDING="$STEP7_DIR/recorded.json" \
      IMAGE=registry/agent:v2 PROJECT_NAME=proj COMPONENT_NAME=agent NAMESPACE=ns APP_PATH=. \
      ENDPOINTS='[{"name":"agent-endpoint","port":8000}]' ENV_VARS='[]' FILE_MOUNTS='[]' \
      RUN_NAME=run-2 OAUTH_URL=http://token OAUTH_HOST=h CLIENT_ID=c OAUTH_CLIENT_SECRET=s \
      API_URL=http://api API_HOST=h \
      sh "$STEP7_DIR/script.sh" >"$STEP7_DIR/out.log" 2>&1; then
    return
  fi
  cat "$STEP7_DIR/recorded.json"
}

# assert_binding <label> <jq expression that must be true of the written body> <body>
assert_binding() {
  if [[ -n "$3" ]] && jq -e "$2" <<<"$3" >/dev/null 2>&1; then
    printf 'ok   - step 7 %s\n' "$1"
  else
    printf 'FAIL - step 7 %s\n      check: %s\n      body:  %s\n' "$1" "$2" "$3"
    FAILURES=$((FAILURES + 1))
  fi
}

BUILD_CHECKS='{"readiness":{"type":"HTTP","path":"/health","periodSeconds":3}}'
EXISTING='{"metadata":{"name":"agent-dev"},"spec":{"environment":"dev","releaseName":"run-1",
  "workloadOverrides":{"container":{"env":[{"key":"LOG_LEVEL","value":"debug"}]}},
  "componentTypeEnvironmentConfigs":{"runtimeClassName":"gvisor",
    "healthChecks":{"readiness":{"type":"TCP"}}}}}'

BODY="$(run_step7 "$BUILD_CHECKS" "")"
assert_binding "writes the build's health checks on a new binding" \
  ".spec.releaseName == \"run-2\" and .spec.componentTypeEnvironmentConfigs.healthChecks == $BUILD_CHECKS" "$BODY"

BODY="$(run_step7 "" "")"
assert_binding "leaves health checks off a new binding when the build has none" \
  '.spec.releaseName == "run-2" and (.spec | has("componentTypeEnvironmentConfigs") | not)' "$BODY"

BODY="$(run_step7 '{}' "")"
assert_binding "treats the build's empty health checks as none" \
  '.spec.releaseName == "run-2" and (.spec | has("componentTypeEnvironmentConfigs") | not)' "$BODY"

BODY="$(run_step7 "$BUILD_CHECKS" "$EXISTING")"
assert_binding "replaces an existing binding's health checks with the build's, with the pin" \
  ".spec.releaseName == \"run-2\" and .spec.componentTypeEnvironmentConfigs.healthChecks == $BUILD_CHECKS" "$BODY"
assert_binding "keeps an existing binding's environment settings" \
  '.spec.workloadOverrides.container.env == [{"key":"LOG_LEVEL","value":"debug"}]
   and .spec.componentTypeEnvironmentConfigs.runtimeClassName == "gvisor"' "$BODY"

BODY="$(run_step7 "" "$EXISTING")"
assert_binding "removes an earlier build's health checks when this build has none" \
  '.spec.releaseName == "run-2"
   and (.spec.componentTypeEnvironmentConfigs | has("healthChecks") | not)
   and .spec.componentTypeEnvironmentConfigs.runtimeClassName == "gvisor"' "$BODY"

BODY="$(run_step7 '[1,2]' "")"
if [[ -z "$BODY" ]] && grep -q "healthChecks is not an object" "$STEP7_DIR/out.log"; then
  printf 'ok   - step 7 refuses a build whose health checks are not an object\n'
else
  printf 'FAIL - step 7 accepted a build whose health checks are not an object\n'
  FAILURES=$((FAILURES + 1))
fi

# --- checkout-source ---------------------------------------------------------
# The repository URL, branch and commit are user input on the same path, and this
# step holds the git credential secret, so a spliced value runs next to it.
# script_body and env_value both read $RENDERED, so pointing that at the next
# template reuses every assertion above.
CHECKOUT_TMPL=templates/cluster-workflow-templates/checkout-source.yaml

if ! RENDERED="$(helm template test-release "$CHART_DIR" --show-only "$CHECKOUT_TMPL" 2>&1)"; then
  printf 'FAIL - helm template failed for %s\n%s\n' "$CHECKOUT_TMPL" "$RENDERED"
  exit 1
fi

SCRIPT="$(script_body)"

if [[ -z "$SCRIPT" ]]; then
  printf 'FAIL - could not extract the checkout script from %s\n' "$CHECKOUT_TMPL"
  exit 1
fi

assert_param_via_env BRANCH workflow.parameters.branch
assert_param_via_env REPO workflow.parameters.git-repo
assert_param_via_env COMMIT workflow.parameters.commit

if [[ -z "$(sh -n <<<"$SCRIPT" 2>&1)" ]]; then
  printf 'ok   - checkout-source parses under sh\n'
else
  printf 'FAIL - checkout-source does not parse under sh\n      %s\n' "$(sh -n <<<"$SCRIPT" 2>&1)"
  FAILURES=$((FAILURES + 1))
fi

# Reaching git as an env var removes the shell, but not the option parser: an
# unquoted expansion still word-splits, and a ref opening with a dash is read as
# a flag rather than a branch.
if grep -qF -- '--branch "$BRANCH"' <<<"$SCRIPT"; then
  printf 'ok   - the clone quotes the branch it expands\n'
else
  printf 'FAIL - the branch expansion in the clone is unquoted\n'
  FAILURES=$((FAILURES + 1))
fi

if grep -qE '^ *-\*\)' <<<"$SCRIPT"; then
  printf 'ok   - a ref opening with a dash is rejected before it reaches git\n'
else
  printf 'FAIL - nothing stops a ref opening with a dash from becoming a git option\n'
  FAILURES=$((FAILURES + 1))
fi

# --- Every build step in the chart -------------------------------------------
# The defect was one quoting style in one step, then the same style in another.
# This is the property itself, so a step added later cannot reintroduce it: no
# Argo parameter may appear anywhere inside a script a shell parses. Scoped to
# block scalars under args:, because an argv list is passed to exec, not to sh.
if ! CHART_RENDERED="$(helm template test-release "$CHART_DIR" 2>&1)"; then
  printf 'FAIL - helm template failed for the chart\n%s\n' "$CHART_RENDERED"
  exit 1
fi

SPLICED="$(awk '
  function ind(s) { match(s, /^ */); return RLENGTH }
  /^# Source: / { src = $3; next }
  !in_script && $0 ~ /^ *args:$/ { pending = 1; ai = ind($0); next }
  pending {
    if ($0 ~ /^[ ]*$/) next
    pending = 0
    if ($0 ~ /^ *- \|-?$/) in_script = 1
    next
  }
  in_script {
    if ($0 !~ /^[ ]*$/ && ind($0) <= ai) { in_script = 0; next }
    if ($0 ~ /\{\{(workflow|inputs)\.parameters/) printf "      %s: %s\n", src, $0
  }
' <<<"$CHART_RENDERED")"

if [[ -z "$SPLICED" ]]; then
  printf 'ok   - no Argo parameter is spliced into a shell script anywhere in the chart\n'
else
  printf 'FAIL - Argo parameters are spliced into shell scripts\n%s\n' "$SPLICED"
  FAILURES=$((FAILURES + 1))
fi

if ((FAILURES > 0)); then
  printf '\n%d assertion(s) failed\n' "$FAILURES"
  exit 1
fi
printf '\nAll render assertions passed\n'
