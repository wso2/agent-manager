#!/bin/bash
# Local development script with hot-reload for all dependencies
# Usage: ./local-dev.sh

set -e

# Color output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}==> Starting development mode with hot-reload...${NC}"

# Store PIDs for cleanup
PIDS=()

# Cleanup function
cleanup() {
  echo -e "\n${YELLOW}==> Stopping all processes...${NC}"
  for pid in "${PIDS[@]}"; do
    echo -e "Stopping process ${pid}"
    kill -TERM "$pid" 2>/dev/null || true
  done
  exit 0
}

# Set up trap for cleanup on Ctrl+C
trap cleanup SIGINT SIGTERM

echo -e "${GREEN}==> Starting TypeScript watch mode for libraries...${NC}"

# Start watch mode for each library with tsconfig.lib.json
# Using process substitution to avoid subshell and preserve PIDS array
while read tsconfig; do
  pkg_dir=$(dirname "$tsconfig")
  pkg_name=$(basename "$pkg_dir")

  echo -e "  ${GREEN}✓${NC} Starting watch for: $pkg_name"
  (cd "$pkg_dir" && pnpm exec tsc -project tsconfig.lib.json --watch --preserveWatchOutput > /dev/null 2>&1) &
  PIDS+=($!)
done < <(find workspaces/libs -name "tsconfig.lib.json" -type f 2>/dev/null)

# Start watch mode for pages
while read tsconfig; do
  pkg_dir=$(dirname "$tsconfig")
  pkg_name=$(basename "$pkg_dir")

  # Skip template
  if echo "$pkg_dir" | grep -q ".template"; then
    continue
  fi

  echo -e "  ${GREEN}✓${NC} Starting watch for: $pkg_name"
  (cd "$pkg_dir" && pnpm exec tsc -project tsconfig.lib.json --watch --preserveWatchOutput > /dev/null 2>&1) &
  PIDS+=($!)
done < <(find workspaces/pages -name "tsconfig.lib.json" -type f 2>/dev/null)

echo -e "${YELLOW}==> Waiting for initial compilation (5s)...${NC}"
sleep 5

echo -e "${GREEN}==> Starting Vite dev server...${NC}"
echo -e "${GREEN}==> Development server will be available at http://localhost:3000${NC}"
echo -e "${YELLOW}==> Press Ctrl+C to stop all processes${NC}\n"

# Start Vite in foreground
cd apps/webapp
pnpm run dev

# This will run if Vite exits
cleanup
