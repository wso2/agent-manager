#!/bin/bash

# Run tests with isolated test database
# This script sets up test database, runs migrations, and executes tests

set -e

PROJECT_ROOT=$(pwd)
ENV_TEST_FILE="$PROJECT_ROOT/.env.test"

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

# Set up log file early so ALL output (including migration failures) is captured
mkdir -p localdata
LOG_FILE="$PROJECT_ROOT/localdata/test_output_isolated.log"
> "$LOG_FILE"

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
if ! PGPASSWORD="$TEST_DB_PASSWORD" psql -h "$TEST_DB_HOST" -p "$TEST_DB_PORT" -U "$TEST_DB_USER" -q -c "SELECT 1" 2>/dev/null; then
    echo "✗ FAILED - Cannot connect to PostgreSQL"
    echo ""
    echo "Please check:"
    echo "  1. PostgreSQL is running: pg_isready -h $TEST_DB_HOST -p $TEST_DB_PORT"
    echo "  2. Credentials are correct in .env.test"
    echo "  3. Main database user '$TEST_DB_USER' exists"
    exit 1
fi
echo "✓ PostgreSQL connection verified"

# Drop and recreate test database for clean state
echo "→ Dropping existing test database (if exists)..."
PGPASSWORD="$TEST_DB_PASSWORD" psql -h "$TEST_DB_HOST" -p "$TEST_DB_PORT" -U "$TEST_DB_USER" -q -c "DROP DATABASE IF EXISTS $TEST_DB_NAME;" 2>/dev/null || true

echo "→ Creating test database..."
if ! PGPASSWORD="$TEST_DB_PASSWORD" psql -h "$TEST_DB_HOST" -p "$TEST_DB_PORT" -U "$TEST_DB_USER" -q -c "CREATE DATABASE $TEST_DB_NAME OWNER $TEST_DB_USER;" 2>/dev/null; then
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
set +e
go test -v --race ./...
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
