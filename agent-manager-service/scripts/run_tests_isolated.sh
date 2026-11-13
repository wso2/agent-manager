#!/bin/bash

# Run tests with isolated test database
# This script sets up test database, runs migrations, and executes tests.
# The database is a fresh SQLite file unless DB_DRIVER=postgres, which uses the
# server named in .env.test.

set -e

PROJECT_ROOT=$(pwd)
ENV_TEST_FILE="$PROJECT_ROOT/.env.test"

if [ "${DB_DRIVER:-sqlite}" = "sqlite" ]; then
    mkdir -p localdata
    LOG_FILE="$PROJECT_ROOT/localdata/test_output_isolated.log"
    : > "$LOG_FILE"
    exec > >(tee -a "$LOG_FILE") 2>&1
    export DB_DRIVER=sqlite
    export DB_PATH="$PROJECT_ROOT/localdata/integration-test.db"
    export ENCRYPTION_KEY="${ENCRYPTION_KEY:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
    export JWT_SIGNING_PRIVATE_KEY_PATH="$PROJECT_ROOT/keys/private.pem"
    export JWT_SIGNING_PUBLIC_KEYS_CONFIG="$PROJECT_ROOT/keys/public-keys-config.json"
    unset ENV_FILE_PATH
    rm -f "$DB_PATH" "$DB_PATH-wal" "$DB_PATH-shm"
    echo "Running integration tests on SQLite ($DB_PATH)"
    bash scripts/gen_keys.sh
    go run . -migrate -server=false
    # One package at a time: every test binary opens the same file.
    COVERAGE_OUT="${COVERAGE_OUT:-$PROJECT_ROOT/localdata/coverage-integration.out}"
    go test -v --race -p 1 -tags=integration \
        -covermode=atomic \
        -coverpkg=./... \
        -coverprofile="$COVERAGE_OUT" \
        ./...
    exit $?
fi

# Check if .env.test exists
if [ ! -f "$ENV_TEST_FILE" ]; then
    echo "Error: $ENV_TEST_FILE not found"
    echo "Please create .env.test with test database configuration"
    exit 1
fi

# Load test database credentials from .env.test
set -a
source "$ENV_TEST_FILE"
set +a

# Test database credentials from .env.test
TEST_DB_HOST="${DB_HOST}"
TEST_DB_PORT="${DB_PORT}"
TEST_DB_USER="${DB_USER}"
TEST_DB_PASSWORD="${DB_PASSWORD}"
TEST_DB_NAME="${DB_NAME}"

# psql_exec runs a one-off SQL command against the server. It prefers a psql
# binary on the host; if none exists (common on dev machines that only run
# Postgres via Docker), it falls back to executing psql inside the running
# Postgres container. Set PG_CONTAINER to override container auto-detection.
# Args: <extra-psql-flags...> -c "<sql>"  — passed straight through to psql.
PG_CONTAINER="${PG_CONTAINER:-}"

detect_pg_container() {
    [ -n "$PG_CONTAINER" ] && { echo "$PG_CONTAINER"; return; }
    command -v docker >/dev/null 2>&1 || return
    # Find every Postgres container publishing the test port. We refuse to guess
    # when more than one matches: psql_exec drives destructive DROP/CREATE DATABASE
    # commands, so picking the wrong instance could wipe the wrong database.
    local matches
    matches="$(docker ps --filter "publish=${TEST_DB_PORT}" --format '{{.Names}} {{.Image}}' 2>/dev/null \
        | awk 'tolower($2) ~ /postgres/ {print $1}')"

    local count
    count="$(printf '%s\n' "$matches" | grep -c .)"
    if [ "$count" -gt 1 ]; then
        echo "✗ FAILED - Multiple Postgres containers publish port ${TEST_DB_PORT}; refusing to guess which to use." >&2
        echo "  Matching containers:" >&2
        printf '%s\n' "$matches" | sed 's/^/    /' >&2
        echo "  Set PG_CONTAINER=<name> to disambiguate." >&2
        return 1
    fi
    if [ "$count" -eq 0 ]; then
        # No match: let the caller (psql_exec) emit its install/PG_CONTAINER hint.
        return
    fi
    printf '%s\n' "$matches"
}

psql_exec() {
    if command -v psql >/dev/null 2>&1; then
        PGPASSWORD="$TEST_DB_PASSWORD" psql \
            -h "$TEST_DB_HOST" -p "$TEST_DB_PORT" -U "$TEST_DB_USER" "$@"
        return $?
    fi
    local container detect_rc
    container="$(detect_pg_container)"
    detect_rc=$?
    # detect_pg_container returns non-zero only when the match is ambiguous; it has
    # already printed the matching containers and the PG_CONTAINER hint to stderr.
    if [ "$detect_rc" -ne 0 ]; then
        return "$detect_rc"
    fi
    if [ -z "$container" ]; then
        echo "✗ FAILED - psql not found on host and no Postgres container detected." >&2
        echo "  Install psql (brew install libpq) or set PG_CONTAINER=<name>." >&2
        return 127
    fi
    # Inside the container, connect over the local socket as the configured user.
    docker exec -e PGPASSWORD="$TEST_DB_PASSWORD" -i "$container" \
        psql -U "$TEST_DB_USER" "$@"
}

# Set up log file early so ALL output (including migration failures) is captured
mkdir -p localdata
LOG_FILE="$PROJECT_ROOT/localdata/test_output_isolated.log"
: > "$LOG_FILE"

# Tee all subsequent stdout/stderr to the log file while still showing on terminal
exec > >(tee -a "$LOG_FILE") 2>&1

echo "========================================"
echo "Running tests with isolated test database"
echo "========================================"
echo ""
echo "Test Database Configuration (from .env.test):"
echo "  Host: $TEST_DB_HOST"
echo "  Port: $TEST_DB_PORT"
echo "  Database: $TEST_DB_NAME"
echo "  User: $TEST_DB_USER"
echo ""

# Step 1: Setup test database
echo "Step 1: Setting up test database"
echo "→ Checking PostgreSQL connection..."
if ! psql_exec -q -c "SELECT 1" >/dev/null 2>&1; then
    echo "✗ FAILED - Cannot connect to PostgreSQL"
    echo ""
    echo "Please check:"
    echo "  1. PostgreSQL is running: pg_isready -h $TEST_DB_HOST -p $TEST_DB_PORT"
    echo "  2. Credentials are correct in .env.test"
    echo "  3. Main database user '$TEST_DB_USER' exists"
    echo "  4. If psql is not on the host, a Postgres container is running (or set PG_CONTAINER)"
    exit 1
fi
echo "✓ PostgreSQL connection verified"

# Drop and recreate test database for clean state
echo "→ Dropping existing test database (if exists)..."
psql_exec -q -c "DROP DATABASE IF EXISTS $TEST_DB_NAME;" 2>/dev/null || true

echo "→ Creating test database..."
if ! psql_exec -q -c "CREATE DATABASE $TEST_DB_NAME OWNER $TEST_DB_USER;" 2>/dev/null; then
    echo "✗ FAILED - Could not create test database"
    exit 1
fi
echo "✓ Test database created"
echo ""

# Step 2: Generate RSA keys if needed
echo "Step 2: Checking RSA keys"
bash scripts/gen_keys.sh
echo ""

# Step 3: Run migrations
echo "Step 3: Running migrations on test database"
export ENV_FILE_PATH="$ENV_TEST_FILE"
# .env.test intentionally omits JWT_SIGNING_PRIVATE_KEY_PATH/JWT_SIGNING_PUBLIC_KEYS_CONFIG
# and falls back to config_loader.go's relative defaults ("keys/private.pem", etc.), which
# only resolve from the repo root. `go test ./...` runs each package's test binary with its
# OWN directory as the working directory (not $PROJECT_ROOT), so any package other than the
# repo root — e.g. ./tests — would fail to find the keys at all. Force absolute paths so every
# package resolves the same real key files regardless of which directory its binary runs from.
export JWT_SIGNING_PRIVATE_KEY_PATH="$PROJECT_ROOT/keys/private.pem"
export JWT_SIGNING_PUBLIC_KEYS_CONFIG="$PROJECT_ROOT/keys/public-keys-config.json"
set +e
go run . -migrate -server=false 2>&1
migrationExitCode=$?
set -e
if [ $migrationExitCode -ne 0 ]; then
    echo "✗ FAILED - Database migrations failed (exit code $migrationExitCode)"
    echo "Full log: $LOG_FILE"
    exit $migrationExitCode
fi
echo "✓ Migrations completed"
echo ""

# Step 4: Run tests
echo "Step 4: Running tests"
echo "========================================"
echo ""

# Record start time
start_time=$SECONDS

# Run tests (output already tee'd to log file via exec above)
# -tags=integration includes the database-backed test files.
COVERAGE_OUT="${COVERAGE_OUT:-$PROJECT_ROOT/localdata/coverage-integration.out}"
set +e
go test -v --race -tags=integration \
    -covermode=atomic \
    -coverpkg=./... \
    -coverprofile="$COVERAGE_OUT" \
    ./...
testExitCode=$?
set -e

elapsed=$(( SECONDS - start_time ))
echo ""
echo "========================================"
echo "Test completed in ${elapsed}s"
echo "========================================"
echo ""

if [ $testExitCode -ne 0 ]; then
    echo "✗ FAILED"
    echo "Full log: $LOG_FILE"
    exit ${testExitCode}
fi

echo "✓ PASSED - Summary:"
echo "========================================"
grep -E "(^ok|^FAIL|^---)" "$LOG_FILE" | tail -20
echo "========================================"
echo ""
echo "Full log: $LOG_FILE"
echo ""
