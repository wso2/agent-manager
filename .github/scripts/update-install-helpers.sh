#!/usr/bin/env bash
# Update chart versions in install-helpers.sh
# Usage: update-install-helpers.sh <version> <file_path>

set -euo pipefail

VERSION="${1:-}"
FILE="${2:-./deployments/quick-start/install-helpers.sh}"

if [ -z "$VERSION" ]; then
  echo "Error: Version is required"
  exit 1
fi

if [ ! -f "$FILE" ]; then
  echo "⚠️ File not found: $FILE, skipping"
  exit 0
fi

# Replace all remaining 0.0.0-dev references with the new version
sed -i.bak "s/0\.0\.0-dev/${VERSION}/g" "$FILE"

# Remove backup files
rm -f "${FILE}.bak"

echo "✅ Updated chart versions in $FILE to ${VERSION}"
