#!/bin/bash
#
# Run one test name/pattern (go test -run) with the same unit-tier config env
# var defaults as run_unit_tests.sh. Config is loaded at import time, so
# these must be present even though no real DB connection is opened.
#
# Usage: scripts/run_single_test.sh <RUN_PATTERN> [PKG]
#   RUN_PATTERN  required, passed to go test -run (regex, e.g. TestAgentKindService)
#   PKG          optional, defaults to ./... (e.g. ./services/)
#
# Normally invoked via: make test-run RUN=TestAgentKindService PKG=./services/

set -euo pipefail

RUN_PATTERN="${1:-}"
PKG="${2:-./...}"

if [ -z "$RUN_PATTERN" ]; then
    echo "Usage: make test-run RUN=<TestNamePattern> [PKG=./path/to/package/]"
    exit 1
fi

export DB_HOST="${DB_HOST:-localhost}"
export DB_PORT="${DB_PORT:-5432}"
export DB_USER="${DB_USER:-unit}"
export DB_PASSWORD="${DB_PASSWORD:-unit}"
export DB_NAME="${DB_NAME:-unit}"
export ENCRYPTION_KEY="${ENCRYPTION_KEY:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
export SERVER_PORT="${SERVER_PORT:-8080}"

if [ -n "${ENV_FILE_PATH:-}" ] && [ ! -f "${ENV_FILE_PATH}" ]; then
    echo "ENV_FILE_PATH points to a missing file (${ENV_FILE_PATH}); unsetting for this run."
    unset ENV_FILE_PATH
fi

echo "Running tests matching '${RUN_PATTERN}' in ${PKG}"
go test -run "$RUN_PATTERN" -v "$PKG"
