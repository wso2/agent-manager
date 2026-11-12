#!/usr/bin/env bash
# resolve-source-commit.sh — find the unstamped commit a release was cut from.
#
# A release tag points at the "chore: update versions..." commit, where the
# standalone bundle's local amp-*:lite images have already been replaced. Re-running the version
# scripts there matches nothing and silently produces a release stamped with the
# OLD version, so a promotion must start from that commit's parent instead.
#
# The subject match picks the candidate; the placeholder check is what makes it
# safe. Run from a full checkout with HEAD at the source release tag.
#
# Prints the base commit SHA on stdout; all other output goes to stderr.
# Usage: resolve-source-commit.sh <source-version>
set -euo pipefail

SOURCE_VERSION="${1:?usage: resolve-source-commit.sh <source-version>}"
STAMP_SUBJECT="chore: update versions for release amp/v${SOURCE_VERSION}"

if [ "$(git log -1 --format=%s HEAD)" = "$STAMP_SUBJECT" ]; then
  BASE_COMMIT="$(git rev-parse HEAD^)"
  echo "HEAD is the version-stamp commit; using its parent ${BASE_COMMIT:0:7}" >&2
else
  BASE_COMMIT="$(git rev-parse HEAD)"
  echo "HEAD is not a version-stamp commit; using it directly (${BASE_COMMIT:0:7})" >&2
fi

if ! git grep -qE '^AMP_API_IMAGE=amp-api:lite$' "$BASE_COMMIT" -- deployments/standalone/.env.example; then
  {
    echo "Error: could not locate the unstamped source commit for amp/v${SOURCE_VERSION}."
    echo "Commit $BASE_COMMIT does not pin deployments/standalone/.env.example to the local"
    echo "amp-*:lite images, so the version-stamping scripts would stamp an already-stamped bundle."
  } >&2
  exit 1
fi

echo "$BASE_COMMIT"
