#!/usr/bin/env bash
# Update Helm chart versions and image tags
# Usage: update-helm-charts.sh <target-version> <release-tag>

set -euo pipefail

TARGET_VERSION="${1:-}"

if [ -z "$TARGET_VERSION" ]; then
  echo "Error: Missing required arguments"
  echo "Usage: update-helm-charts.sh <target-version>"
  exit 1
fi

# Find all Chart.yaml files and replace 0.0.0-dev
find ./deployments/helm-charts -name "Chart.yaml" -type f | while read -r chart_file; do
  # Replace version: 0.0.0-dev with vTARGET_VERSION (using | as delimiter to avoid conflicts with / in version)
  sed -i.bak "s|version: 0\.0\.0-dev|version: v$TARGET_VERSION|g" "$chart_file"
  # Replace appVersion: "0.0.0-dev" with vTARGET_VERSION (all versions must be vx.x.x format)
  sed -i.bak "s|appVersion: \"0\.0\.0-dev\"|appVersion: \"v$TARGET_VERSION\"|g" "$chart_file"
  # Remove backup files
  rm -f "${chart_file}.bak"
done

# Find all values.yaml files and replace 0.0.0-dev in image tags
find ./deployments/helm-charts -name "values.yaml" -type f | while read -r values_file; do
  # Replace tag: "0.0.0-dev" with vTARGET_VERSION (all image tags must be vx.x.x format)
  sed -i.bak "s|tag: \"0\.0\.0-dev\"|tag: \"v$TARGET_VERSION\"|g" "$values_file"
  # Remove backup files
  rm -f "${values_file}.bak"
done

echo "✅ Updated all Helm chart versions"
