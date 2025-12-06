#!/usr/bin/env bash
# Package and push a Helm chart
# Usage: package-helm-chart.sh <chart-name> <chart-dir> <version> <helm-registry> <github-token>

set -euo pipefail

CHART_NAME="${1:-}"
CHART_DIR="${2:-}"
VERSION="${3:-}"
HELM_REGISTRY="${4:-}"
GITHUB_TOKEN="${5:-}"

if [ -z "$CHART_NAME" ] || [ -z "$CHART_DIR" ] || [ -z "$VERSION" ] || [ -z "$HELM_REGISTRY" ] || [ -z "$GITHUB_TOKEN" ]; then
  echo "Error: Missing required arguments"
  echo "Usage: package-helm-chart.sh <chart-name> <chart-dir> <version> <helm-registry> <github-token>"
  exit 1
fi

if [ ! -d "$CHART_DIR" ]; then
  echo "Error: Chart directory not found: $CHART_DIR"
  exit 1
fi

# Log in to registry
ACTOR="${GITHUB_ACTOR:-github-actions}"
echo "$GITHUB_TOKEN" | helm registry login -u "$ACTOR" --password-stdin "${HELM_REGISTRY#oci://}"

# Package and push
helm package "$CHART_DIR" --version "$VERSION"
helm push "${CHART_NAME}-${VERSION}.tgz" "$HELM_REGISTRY/"

echo "✅ Pushed $CHART_NAME chart version $VERSION"
