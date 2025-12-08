#!/usr/bin/env bash
# Update version references in documentation files
# Usage: update-docs-versions.sh <target-version> <registry-org>

set -euo pipefail

TARGET_VERSION="${1:-}"
REGISTRY_ORG="${2:-wso2}"

if [ -z "$TARGET_VERSION" ]; then
  echo "Error: Version is required"
  echo "Usage: update-docs-versions.sh <target-version> [registry-org]"
  exit 1
fi

echo "Updating documentation files with version $TARGET_VERSION (registry: ${REGISTRY_ORG})..."

# Update quick-start.md - Docker image version
if [ -f "./docs/quick-start.md" ]; then
  # Update the amp-quick-start image version (handles wso2 registry)
  # Use # as delimiter to avoid conflict with | in the pattern
  sed -i.bak -E "s#ghcr\.io/wso2/amp-quick-start:v[0-9]+\.[0-9]+\.[0-9]+#ghcr.io/${REGISTRY_ORG}/amp-quick-start:v${TARGET_VERSION}#g" "./docs/quick-start.md"
  rm -f "./docs/quick-start.md.bak"
  echo "✅ Updated docs/quick-start.md"
else
  echo "⚠️ File not found: ./docs/quick-start.md, skipping"
fi

# Update single-cluster.md - Chart versions and registry
if [ -f "./docs/install/single-cluster.md" ]; then
  # Update HELM_CHART_REGISTRY (handles wso2 registry)
  sed -i.bak -E "s#export HELM_CHART_REGISTRY=\"ghcr\.io/wso2\"#export HELM_CHART_REGISTRY=\"ghcr.io/${REGISTRY_ORG}\"#g" "./docs/install/single-cluster.md"
  
  # Update AMP_CHART_VERSION
  sed -i.bak "s#export AMP_CHART_VERSION=\"[^\"]*\"#export AMP_CHART_VERSION=\"${TARGET_VERSION}\"#g" "./docs/install/single-cluster.md"
  
  # Update OBSERVABILITY_CHART_VERSION
  sed -i.bak "s#export OBSERVABILITY_CHART_VERSION=\"[^\"]*\"#export OBSERVABILITY_CHART_VERSION=\"${TARGET_VERSION}\"#g" "./docs/install/single-cluster.md"
  
  # Update BUILD_CI_CHART_VERSION
  sed -i.bak "s#export BUILD_CI_CHART_VERSION=\"[^\"]*\"#export BUILD_CI_CHART_VERSION=\"${TARGET_VERSION}\"#g" "./docs/install/single-cluster.md"
  
  # Update registry reference in Default Configuration section
  sed -i.bak -E "s#- Registry: \`ghcr\.io/wso2\`#- Registry: \`ghcr.io/${REGISTRY_ORG}\`#g" "./docs/install/single-cluster.md"
  
  rm -f "./docs/install/single-cluster.md.bak"
  echo "✅ Updated docs/install/single-cluster.md"
else
  echo "⚠️ File not found: ./docs/install/single-cluster.md, skipping"
fi

echo "✅ Updated all documentation files with version ${TARGET_VERSION}"

