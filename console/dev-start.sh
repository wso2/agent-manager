#!/bin/sh
set -e

# Capture the monorepo root (set by WORKDIR in Dockerfile.dev; matches host path)
MONOREPO_ROOT="$PWD"

echo "==> Linking dependencies for container environment..."
cd "$MONOREPO_ROOT"
rush update

echo "==> Generating runtime config..."
cd "$MONOREPO_ROOT/apps/web-ui"
envsubst < public/config.template.js > public/config.js

echo "==> Starting core-ui in watch mode..."
cd "$MONOREPO_ROOT/workspaces/core-ui"
rushx dev &
CORE_UI_PID=$!

echo "==> Waiting for initial core-ui build..."
sleep 10

echo "==> Starting web-ui dev server..."
cd "$MONOREPO_ROOT/apps/web-ui"
exec rushx dev --host 0.0.0.0
