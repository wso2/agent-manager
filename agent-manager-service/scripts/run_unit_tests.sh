#!/bin/bash
#
# Run the fast unit-test tier: every package EXCEPT the database-backed
# integration tests (which are guarded by `//go:build integration`).
#
# No PostgreSQL is required. The db package connects lazily (db/db.go), so
# packages that never touch the database run without one. Config is still
# loaded at import time, so the required config env vars must be present —
# dummy values are fine because no real connection is opened.

set -euo pipefail

PROJECT_ROOT=$(pwd)

# Provide harmless defaults for the config vars that config.loadEnvs() marks
# required. These are NEVER used to open a connection in the unit tier.
export DB_HOST="${DB_HOST:-localhost}"
export DB_PORT="${DB_PORT:-5432}"
export DB_USER="${DB_USER:-unit}"
export DB_PASSWORD="${DB_PASSWORD:-unit}"
export DB_NAME="${DB_NAME:-unit}"
export ENCRYPTION_KEY="${ENCRYPTION_KEY:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
export SERVER_PORT="${SERVER_PORT:-8080}"

# Ensure ENV_FILE_PATH is unset/valid so config.init() does not panic.
if [ -n "${ENV_FILE_PATH:-}" ] && [ ! -f "${ENV_FILE_PATH}" ]; then
    echo "ENV_FILE_PATH points to a missing file (${ENV_FILE_PATH}); unsetting for unit run."
    unset ENV_FILE_PATH
fi

# RSA keys (used by a few middleware tests).
bash scripts/gen_keys.sh

mkdir -p localdata
COVERAGE_OUT="${COVERAGE_OUT:-$PROJECT_ROOT/localdata/coverage-unit.out}"

echo "========================================"
echo "Running UNIT tests (no database)"
echo "========================================"

start_time=$SECONDS

# Coverage is measured PER PACKAGE (no -coverpkg). Each package's % counts only
# its own tests against its own code — the standard `go test` behavior. We
# deliberately avoid -coverpkg=./... here: the unit tier excludes every
# //go:build integration package, so a repo-wide denominator collapses the
# aggregate to a meaningless ~2%. Several packages (services, repositories,
# controllers, ...) also have most of their tests integration-gated, so their
# real coverage lives in the integration tier; a per-package number keeps the
# unit report honest without a single misleading total.
set +e
go test -race \
    -covermode=atomic \
    -coverprofile="$COVERAGE_OUT" \
    ./... 2>&1 | tee localdata/unit_test_output.log
testExitCode=${PIPESTATUS[0]}
set -e

elapsed=$(( SECONDS - start_time ))
echo "Unit tests completed in ${elapsed}s"

if [ $testExitCode -ne 0 ]; then
    echo "✗ UNIT TESTS FAILED - see localdata/unit_test_output.log"
    exit $testExitCode
fi

echo "✓ UNIT TESTS PASSED"
echo "Coverage profile: $COVERAGE_OUT"
# Per-package coverage (each package vs. its own tests). The total below is the
# aggregate over only the packages that have unit tests — NOT the whole repo.
go tool cover -func="$COVERAGE_OUT" | tail -1
