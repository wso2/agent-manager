#!/usr/bin/env bash
# Package and push a Helm chart
# Usage: package-helm-chart.sh <chart-name> <chart-dir> <version> <helm-registry> <github-token>

set -euo pipefail

CHART_DIR="${1:-}"
VERSION="${2:-}"
HELM_REGISTRY="${3:-}"
GITHUB_TOKEN="${4:-}"

if [ -z "$CHART_DIR" ] || [ -z "$VERSION" ] || [ -z "$HELM_REGISTRY" ] || [ -z "$GITHUB_TOKEN" ]; then
  echo "Error: Missing required arguments"
  echo "Usage: package-helm-chart.sh <chart-dir> <version> <helm-registry> <github-token>"
  exit 1
fi

if [ ! -d "$CHART_DIR" ]; then
  echo "Error: Chart directory not found: $CHART_DIR"
  exit 1
fi

# Log in to registry
ACTOR="${GITHUB_ACTOR:-github-actions}"
echo "$GITHUB_TOKEN" | helm registry login -u "$ACTOR" --password-stdin "${HELM_REGISTRY#oci://}"

# Update dependencies only if tarballs are missing
if [ -f "$CHART_DIR/Chart.lock" ]; then
  CHARTS_SUBDIR="$CHART_DIR/charts"
  MISSING=false
  CURRENT_NAME=""

  while IFS= read -r line; do
    case "$line" in
      *"name:"*)
        CURRENT_NAME=$(echo "$line" | sed 's/.*name: *//')
        ;;
      *"version:"*)
        DEP_VERSION=$(echo "$line" | sed 's/.*version: *//')
        if [ -n "$CURRENT_NAME" ] && [ -n "$DEP_VERSION" ]; then
          if [ ! -f "$CHARTS_SUBDIR/$CURRENT_NAME-$DEP_VERSION.tgz" ]; then
            echo "Missing dependency: $CURRENT_NAME-$DEP_VERSION.tgz"
            MISSING=true
          fi
          CURRENT_NAME=""
        fi
        ;;
    esac
  done < "$CHART_DIR/Chart.lock"

  if [ "$MISSING" = "true" ]; then
    echo "Downloading missing dependencies..."
    helm repo add bitnami https://charts.bitnami.com/bitnami --force-update
    helm repo update
    helm dependency update "$CHART_DIR"
  else
    echo "All chart dependencies are already present, skipping download"
  fi
fi

# Package and push
# Capture the output from helm package which prints the created filename
# Format: "Successfully packaged chart and saved it to: chart-name-version.tgz"
PACKAGE_OUTPUT=$(helm package "$CHART_DIR" --version "$VERSION" 2>&1)

# Extract the filename from the output (works with both relative and absolute paths)
PACKAGED_FILE=$(echo "$PACKAGE_OUTPUT" | sed -n 's/.*Successfully packaged chart and saved it to: //p')

if [ -z "$PACKAGED_FILE" ]; then
  echo "Error: Failed to determine packaged chart filename from helm package output"
  echo "helm package output: $PACKAGE_OUTPUT"
  exit 1
fi

# Extract just the filename (without path) if helm package printed a full path
PACKAGED_FILENAME=$(basename "$PACKAGED_FILE")

# Verify the file exists
if [ ! -f "$PACKAGED_FILENAME" ]; then
  echo "Error: Packaged chart file not found: $PACKAGED_FILENAME"
  echo "helm package output: $PACKAGE_OUTPUT"
  exit 1
fi

helm push "$PACKAGED_FILENAME" "$HELM_REGISTRY/"

echo "✅ Pushed $PACKAGED_FILENAME chart version $VERSION"
