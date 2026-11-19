#!/usr/bin/env bash
# Regenerates the AMP Lite diagrams (draw.io, HTML and PNG) in docs/amp-lite, and the core
# ER diagram in agent-manager-service/docs/database-er.md from the schema's lite.yaml.
# Needs python3 and Google Chrome (set CHROME to its binary on non-macOS hosts).
set -euo pipefail

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="$(dirname "$SRC")"
CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
PROFILE="$(mktemp -d)"
trap 'rm -rf "$PROFILE"' EXIT

shot() { # <html> <png> <width> <height> <scale>
  "$CHROME" --headless --disable-gpu --hide-scrollbars --user-data-dir="$PROFILE" \
    --force-device-scale-factor="$5" --window-size="$3,$4" \
    --screenshot="$2" "file://$1" 2>/dev/null
}

python3 "$SRC/architecture_gen.py" "$OUT"
shot "$OUT/architecture.html" "$OUT/architecture.png" 2330 1090 2

size="$(python3 "$SRC/data_model_gen.py" "$OUT/data-model.html" | head -1)"
shot "$OUT/data-model.html" "$OUT/data-model.png" "${size%x*}" "${size#*x}" 1

REPO="$(cd "$OUT/../.." && pwd)"
python3 "$SRC/er_gen.py" "$REPO/agent-manager-service/db_migrations/schema/lite.yaml" \
  "$REPO/agent-manager-service/docs/database-er.md" >/dev/null

echo "Wrote architecture.{drawio,html,png} and data-model.{html,png} to $OUT"
