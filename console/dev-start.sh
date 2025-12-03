#!/bin/sh
set -e

echo "==> Generating runtime config..."
cd /app/apps/webapp
envsubst < public/config.template.js > public/config.js

echo "==> Starting TypeScript watch mode for workspace packages..."
cd /app

# Find all packages with tsconfig.lib.json and start tsc --watch for each
# Store the list first to avoid subshell issues
TSCONFIGS=$(find workspaces -name "tsconfig.lib.json" -type f 2>/dev/null)

for tsconfig in $TSCONFIGS; do
  pkg_dir=$(dirname "$tsconfig")
  pkg_name=$(basename "$pkg_dir")
  
  # Skip packages that contain ".template" in their path to avoid watching yeoman packages
  if echo "$pkg_dir" | grep -q ".template"; then
    echo "  Skipping template package: $pkg_name at $pkg_dir"
    continue
  fi
  
  echo "  Starting watch for: $pkg_name at $pkg_dir"
  (cd "$pkg_dir" && pnpm exec tsc -project tsconfig.lib.json --watch --preserveWatchOutput) &
done

echo "==> Waiting for initial compilation..."
sleep 5

echo "==> Starting Vite dev server..."
cd /app/apps/webapp
exec pnpm run dev --host 0.0.0.0
