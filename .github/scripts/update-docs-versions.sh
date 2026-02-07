#!/usr/bin/env bash
# Update version references in documentation files
# Usage: update-docs-versions.sh <target-version>

set -euo pipefail

TARGET_VERSION="${1:-}"

if [ -z "$TARGET_VERSION" ]; then
  echo "Error: Version is required"
  echo "Usage: update-docs-versions.sh <target-version>"
  exit 1
fi

echo "Updating documentation files with version $TARGET_VERSION"

# Counter for tracking updates
updated_count=0

# Find all markdown files in docs directory and update version references
# Use process substitution to avoid subshell issues with updated_count
while read -r doc_file; do
  # Check if file contains version patterns before attempting replacement
  if grep -q "0\.0\.0-dev\|v0\.0\.0-dev" "$doc_file" 2>/dev/null; then
    # Replace version: 0.0.0-dev with TARGET_VERSION (without v prefix)
    sed -i.bak "s|0\.0\.0-dev|${TARGET_VERSION}|g" "$doc_file"

    # Replace version: v0.0.0-dev with vTARGET_VERSION (with v prefix)
    sed -i.bak "s|v0\.0\.0-dev|v${TARGET_VERSION}|g" "$doc_file"

    # Remove backup file
    rm -f "${doc_file}.bak"

    echo "✅ Updated $(basename "$doc_file")"
    updated_count=$((updated_count + 1))
  fi
done < <(find ./docs -name "*.md" -type f)

if [ $updated_count -eq 0 ]; then
  echo "⚠️ No files with version patterns found in ./docs"
else
  echo "✅ Updated $updated_count documentation file(s) with version ${TARGET_VERSION}"
fi

