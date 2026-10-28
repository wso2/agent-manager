#!/bin/bash
# Kills the console Vite dev server started from this checkout only.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Physical path: pnpm launches Vite from the resolved directory, not a symlink.
CONSOLE_DIR="$(cd "$SCRIPT_DIR/../../console" && pwd -P)"

# Under `pnpm dev` the only process carrying this path is Vite itself:
#   node $CONSOLE_DIR/apps/web-ui/node_modules/.bin/../../../../node_modules/.pnpm/vite@.../vite/bin/vite.js
# Matching vite/bin/vite.js (not just "vite") leaves vitest and editors alone.
# The pnpm wrappers exit once Vite does.
VITE_PATTERN="$CONSOLE_DIR/.*/vite/bin/vite\.js"

if pkill -f "$VITE_PATTERN" 2>/dev/null; then
    # Vite holds :3000 until it exits; wait (up to 5s) so a restart can bind it
    for _ in $(seq 50); do
        pgrep -f "$VITE_PATTERN" > /dev/null || break
        sleep 0.1
    done
    echo "✅ Stopped console dev server"
else
    echo "ℹ️  No console dev server running"
fi
