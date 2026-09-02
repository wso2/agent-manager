#!/bin/sh
set -e

# Capture the monorepo root (set by WORKDIR in Dockerfile.dev; matches host path)
MONOREPO_ROOT="$PWD"

echo "==> Linking dependencies for container environment..."
cd "$MONOREPO_ROOT"
pnpm install --frozen-lockfile

echo "==> Generating runtime config..."
cd "$MONOREPO_ROOT/apps/web-ui"
envsubst < public/config.template.js > public/config.js

# No core-ui build step: apps/web-ui/vite.config.ts aliases every workspace package
# to its src/, so the dev server compiles them itself and picks up edits directly.
echo "==> Starting web-ui dev server..."
exec pnpm run dev --host 0.0.0.0
