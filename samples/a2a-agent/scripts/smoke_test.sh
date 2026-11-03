#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
exec "${PYTHON:-python3.11}" "$ROOT/test/sample-smoke/a2a/a2a_smoke.py" "$@"
