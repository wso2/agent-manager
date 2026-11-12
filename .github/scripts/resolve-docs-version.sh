#!/usr/bin/env bash
# resolve-docs-version.sh — print the documentation version a release points at.
#
# The value is consumed as the path segment after /docs/ (see the console's
# DOCS_URL, set from DOCS_VERSION in deployments/standalone), so it must match a published version
# in wso2/docs-agent-platform exactly - a versioned_docs/version-<docs-version>
# directory and an entry in versions.json there. Docs versions are cut per
# release version, so this is just "v" plus the release version.
#
# Prints nothing (exit 0) for releases that cut no docs version: release
# candidates follow /docs/latest, and nightly.yml points nightlies at /docs/next.
#
# stamp-standalone-release.sh runs this to stamp the bundle, and the promotion
# workflow's validate job runs it to pre-flight the manifest before it cuts
# anything.
#
# Usage: resolve-docs-version.sh <target-version>
set -euo pipefail

TARGET_VERSION="${1:?usage: resolve-docs-version.sh <target-version>}"

if [[ "$TARGET_VERSION" =~ (-|\.)rc[0-9]+$ ]]; then
  : # release candidates follow /docs/latest
elif [[ "$TARGET_VERSION" == *-dev* ]]; then
  : # nightlies follow /docs/latest
else
  echo "v$TARGET_VERSION"
fi
