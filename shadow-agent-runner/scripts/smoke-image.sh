#!/usr/bin/env bash
# Smoke-test a built amp-shadow-runner image with no network and a
# read-only root file system. Used by `make docker-smoke` and the PR workflow.
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
"${run[@]}" "$image" | grep -q "usage: amp-shadow-runner"

echo "version --json reports the runner version"
"${run[@]}" "$image" version --json | grep -q "\"runner_version\": \"$version\""

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

echo "pip is not shipped"
if "${run[@]}" --entrypoint python "$image" -c 'import pip' 2>/dev/null; then
  echo "pip is present in the image" >&2
  exit 1
fi

echo "Smoke test passed for $image"
