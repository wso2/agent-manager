#!/usr/bin/env bash
# stamp-standalone-release.sh — stamp a release version into the standalone bundle.
#
# Pins deployments/standalone/.env.example to the release's images and points
# the console's documentation links at the docs version this release ships
# with (release candidates and nightlies keep following /docs/latest), after
# checking that docs version is published.
#
# Usage: stamp-standalone-release.sh <target-version> <registry/org>   (run from the repo root)
set -euo pipefail

TARGET_VERSION="${1:?usage: stamp-standalone-release.sh <target-version> <registry/org>}"
PREFIX="${2:?usage: stamp-standalone-release.sh <target-version> <registry/org>}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="deployments/standalone/.env.example"

DOCS_VERSION="$(bash "$SCRIPT_DIR/resolve-docs-version.sh" "$TARGET_VERSION")"
bash "$SCRIPT_DIR/assert-docs-version-published.sh" "$DOCS_VERSION" "$TARGET_VERSION"

bash "$SCRIPT_DIR/pin-standalone-images.sh" "v${TARGET_VERSION}" "$PREFIX"

tmp="$(mktemp)"
sed "s#^DOCS_VERSION=.*#DOCS_VERSION=${DOCS_VERSION:-latest}#" "$ENV_FILE" > "$tmp"
mv "$tmp" "$ENV_FILE"
echo "Console documentation links follow /docs/${DOCS_VERSION:-latest}"
