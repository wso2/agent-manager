#!/usr/bin/env bash
# Smoke-test a built amp-shadow-runner image with no network and a
# read-only root file system. Used by `make docker-smoke` and the PR workflow.
# It never contacts Agent Manager or GitHub: the scan checks it runs stop before any request.
# Usage: scripts/smoke-image.sh <image> <expected-version>
set -euo pipefail

image="${1:?usage: smoke-image.sh <image> <expected-version>}"
version="${2:?usage: smoke-image.sh <image> <expected-version>}"
# Read-only root, scratch on tmpfs, no network: the tightest way to run the image.
run=(docker run --rm --network none --read-only --tmpfs /tmp)

echo "--version prints the build VERSION"
got="$("${run[@]}" "$image" --version)"
if [ "$got" != "amp-shadow-runner $version" ]; then
  echo "expected 'amp-shadow-runner $version', got '$got'" >&2
  exit 1
fi

echo "--help lists the commands"
help="$("${run[@]}" "$image" --help)"
for command in scan upload version; do
  grep -q "$command" <<< "$help" || { echo "--help does not list '$command'" >&2; exit 1; }
done

echo "no arguments prints the help"
bare="$("${run[@]}" "$image")"
grep -q "usage: amp-shadow-runner" <<< "$bare" || { echo "no arguments does not print the help" >&2; exit 1; }

echo "version --json reports the runner version"
json="$("${run[@]}" "$image" version --json)"
grep -q "\"runner_version\": \"$version\"" <<< "$json" || { echo "version --json is wrong: $json" >&2; exit 1; }

echo "every grammar imports and loads"
"${run[@]}" --entrypoint python "$image" -c '
import tree_sitter, tree_sitter_c_sharp, tree_sitter_javascript, tree_sitter_python, tree_sitter_typescript
for fn in (tree_sitter_python.language, tree_sitter_javascript.language, tree_sitter_typescript.language_typescript,
           tree_sitter_typescript.language_tsx, tree_sitter_c_sharp.language):
    tree_sitter.Language(fn())
'

echo "runs as uid 10001"
uid="$("${run[@]}" --entrypoint id "$image" -u)"
if [ "$uid" != "10001" ]; then
  echo "expected uid 10001, got $uid" >&2
  exit 1
fi

echo "runs as the caller's uid and writes only to \$HOME (/tmp) and /out"
"${run[@]}" --user 4242:4242 --tmpfs /out:uid=4242,gid=4242 --entrypoint python "$image" -c '
import os, pathlib, tempfile
tempfile.TemporaryFile().close()
pathlib.Path(os.environ["HOME"], "probe").write_text("ok")
pathlib.Path("/out/probe").write_text("ok")
try:
    pathlib.Path("/opt/venv/probe").write_text("x")
except OSError:
    pass
else:
    raise SystemExit("the root file system is writable")
'

echo "scan --help and upload --help list their options"
scan_help="$("${run[@]}" "$image" scan --help)"
for option in --include-private --yes; do
  grep -q -- "$option" <<< "$scan_help" || { echo "scan --help does not list $option" >&2; exit 1; }
done
upload_help="$("${run[@]}" "$image" upload --help)"
grep -q -- "--scan-id" <<< "$upload_help" || { echo "upload --help does not list --scan-id" >&2; exit 1; }

# A syntactically valid, unexpired, unsigned JWT: enough for the runner's lifetime check, useless anywhere else.
b64url() { base64 | tr '+/' '-_' | tr -d '=\n'; }
fake_jwt="$(printf '{"alg":"none"}' | b64url).$(printf '{"exp":%d}' "$(( $(date +%s) + 3600 ))" | b64url).c2ln"
scan=("${run[@]}" --tmpfs /out:uid=10001,gid=10001 -e AMP_URL=https://agent-manager.invalid -e AMP_SCAN_TOKEN="$fake_jwt"
  "$image" scan --scan-id 3f2b6c1e-8d4a-4f5b-9c7e-1a2b3c4d5e6f --github-org acme --all-repos)

# expect_exit <code> <text> <command...>: the command exits <code> and its stderr names <text>.
expect_exit() {
  local want="$1" text="$2" got err
  shift 2
  set +e
  err="$("$@" 2>&1 >/dev/null)"
  got=$?
  set -e
  if [ "$got" != "$want" ] || ! grep -qF -- "$text" <<< "$err"; then
    echo "expected exit $want naming '$text', got exit $got: $err" >&2
    exit 1
  fi
}

echo "scan of private repositories with no terminal and no GITHUB_TOKEN exits 2 and names GITHUB_TOKEN"
expect_exit 2 GITHUB_TOKEN "${scan[@]}" --include-private

echo "scan with no terminal and without --yes exits 2 and names --yes"
expect_exit 2 "--yes" "${scan[@]}"

echo "pip is not shipped"
if "${run[@]}" --entrypoint python "$image" -c 'import pip' 2>/dev/null; then
  echo "pip is present in the image" >&2
  exit 1
fi

echo "Smoke test passed for $image"
