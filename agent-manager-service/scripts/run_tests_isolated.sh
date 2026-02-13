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
if ! go run . -migrate -server=false 2>&1 | grep -q "migration completed"; then
    echo "✗ FAILED - Database migrations failed"
    exit 1
fi
echo "✓ Migrations completed"
echo ""

# Step 4: Run tests
echo "Step 4: Running tests"
echo "========================================"
echo ""

# Create localdata directory if it doesn't exist
mkdir -p localdata

# Record start time
start_time=$SECONDS

# Save original stdout and stderr
exec 6>&1 7>&2

# Redirect both stdout and stderr to log file
exec > localdata/test_output_isolated.log 2>&1

# Run tests with test database (disable -e to capture exit code)
set +e
go test -v --race ./...
testExitCode=$?
set -e

# Restore original stdout and stderr
exec 1>&6 2>&7 6>&- 7>&-

elapsed=$(( SECONDS - start_time ))
echo ""
echo "========================================"
echo "Test completed in ${elapsed}s"
echo "========================================"
echo ""

if [ $testExitCode -ne 0 ]; then
    echo "✗ FAILED - Showing last 50 lines of test output:"
    echo "========================================"
    tail -50 "$PROJECT_ROOT/localdata/test_output_isolated.log"
    echo "========================================"
    echo ""
    echo "Full log: localdata/test_output_isolated.log"
    exit ${testExitCode}
fi

echo "✓ PASSED - Showing summary:"
echo "========================================"
grep -E "(PASS|FAIL|ok|FAIL)" "$PROJECT_ROOT/localdata/test_output_isolated.log" | tail -20
echo "========================================"
echo ""
echo "Full log: localdata/test_output_isolated.log"
echo ""
