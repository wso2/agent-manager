#!/usr/bin/env bash
# assert-docs-version-published.sh — fail unless the docs version this release
# needs has already been cut and published.
#
# The documentation lives in wso2/docs-agent-platform, so the manifest is read
# from that repository's main branch rather than from a local checkout. 
#
# stamp-standalone-release.sh runs this too, but only once the release branch and
# tags already exist.
#
# Usage: assert-docs-version-published.sh <docs-version> <target-version>
set -euo pipefail

DOCS_VERSION="${1-}"
TARGET_VERSION="${2:?usage: assert-docs-version-published.sh <docs-version> <target-version>}"

#fetch the versions from the doc repo
DOCS_VERSIONS_URL="${DOCS_VERSIONS_URL:-https://raw.githubusercontent.com/wso2/docs-agent-platform/main/versions.json}"

if [ -z "$DOCS_VERSION" ]; then
  echo "No documentation version is required for $TARGET_VERSION; the console will follow /docs/latest"
  exit 0
fi

# A transport failure must not read as "version missing": that would report a
# network blip as an unpublished release and send someone off cutting docs that
# already exist. Separate the two outcomes on the HTTP status.
BODY_FILE="$(mktemp)"
trap 'rm -f "$BODY_FILE"' EXIT
HTTP_CODE="$(curl -sS -o "$BODY_FILE" -w '%{http_code}' \
  --retry 3 --retry-delay 2 --max-time 30 "$DOCS_VERSIONS_URL" || true)"
HTTP_CODE="${HTTP_CODE:-000}"

if [ "$HTTP_CODE" != "200" ]; then
  echo "Error: could not read the documentation manifest at $DOCS_VERSIONS_URL (HTTP $HTTP_CODE), so $DOCS_VERSION cannot be verified." >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq is required to read the documentation manifest." >&2
  exit 1
fi

if ! jq -e 'type == "array" and all(.[]; type == "string")' "$BODY_FILE" >/dev/null 2>&1; then
  echo "Error: $DOCS_VERSIONS_URL is not a JSON array of version strings, so $DOCS_VERSION cannot be verified." >&2
  exit 1
fi

if ! jq -e --arg v "$DOCS_VERSION" 'any(.[]; . == $v)' "$BODY_FILE" >/dev/null; then
  {
    echo "Error: documentation version $DOCS_VERSION is not published."
    echo
    echo "In wso2/docs-agent-platform, run the \"Documentation Release\" workflow with:"
    echo "  version=$DOCS_VERSION"
    echo "  docker_tag=v$TARGET_VERSION"
    echo "then merge the pull request it opens and re-run this promotion."
  } >&2
  exit 1
fi

echo "✅ Documentation version $DOCS_VERSION is published"
