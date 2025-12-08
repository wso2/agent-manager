#!/usr/bin/env bash
# Update version references in documentation files
# Usage: update-docs-versions.sh <target-version> <registry-org>

set -euo pipefail

TARGET_VERSION="${1:-}"

if [ -z "$TARGET_VERSION" ]; then
  echo "Error: Version is required"
  echo "Usage: update-docs-versions.sh <target-version>"
  exit 1
fi

echo "Updating documentation files with version $TARGET_VERSION"

# Update quick-start.md - Docker image version
if [ -f "./docs/quick-start.md" ]; then
  # Update the amp-quick-start image version (handles wso2 registry)
  # Use # as delimiter to avoid conflict with | in the pattern
  sed -i.bak -E "s#v0.0.0-dev#v${TARGET_VERSION}#g" "./docs/quick-start.md"
  if grep -q "${TARGET_VERSION}" "./docs/quick-start.md"; then
    echo "✅ Updated docs/quick-start.md"
  else
    echo "⚠️ Warning: Version pattern may not have been found in ./docs/quick-start.md"
  fi
  rm -f "./docs/quick-start.md.bak"
else
  echo "⚠️ File not found: ./docs/quick-start.md, skipping"
fi

# Update single-cluster.md - Chart versions and registry
if [ -f "./docs/install/single-cluster.md" ]; then
  # Update HELM_CHART_REGISTRY (handles wso2 registry)
  sed -i.bak -E "s#0.0.0-dev#${TARGET_VERSION}#g" "./docs/install/single-cluster.md"
  if grep -q "${TARGET_VERSION}" "./docs/install/single-cluster.md"; then
    echo "✅ Updated docs/install/single-cluster.md"
  else
    echo "⚠️ Warning: Version pattern may not have been found in ./docs/install/single-cluster.md"
  fi
  rm -f "./docs/install/single-cluster.md.bak"
else
  echo "⚠️ File not found: ./docs/install/single-cluster.md, skipping"
fi

echo "✅ Updated all documentation files with version ${TARGET_VERSION}"

