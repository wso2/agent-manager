#!/bin/sh
set -e

# Capture the monorepo root (set by WORKDIR in Dockerfile.dev; matches host path)
MONOREPO_ROOT="$PWD"

echo "==> Generating runtime config..."
cd "$MONOREPO_ROOT/apps/webapp"
envsubst < public/config.template.js > public/config.js

echo "==> Starting TypeScript watch mode for workspace packages..."
cd "$MONOREPO_ROOT"

# Find all packages with tsconfig.lib.json and start tsc --watch for each
# Store the list first to avoid subshell issues
TSCONFIGS=$(find workspaces -name "tsconfig.lib.json" -type f 2>/dev/null)

echo "==> Waiting for initial compilation..."
sleep 5

echo "==> Starting Vite dev server..."
cd "$MONOREPO_ROOT/apps/webapp"
exec pnpm run dev --host 0.0.0.0
