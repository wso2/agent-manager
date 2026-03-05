#!/bin/bash

# Set environment file path if .env exists
if [ -f .env ]; then
    export ENV_FILE_PATH=$(pwd)/.env
    echo "Using ENV_FILE_PATH: $ENV_FILE_PATH"
fi

echo "Running tests with coverage for traces-observer-service"

# Create data directory if it doesn't exist
mkdir -p data

# Record start time
start_time=$SECONDS

# Save original stdout and stderr
exec 6>&1 7>&2

# Redirect both stdout and stderr to log file
exec > data/test_coverage_output.log 2>&1

go test -v --race -coverprofile=coverage.out ./...

testExitCode=$?

# Restore original stdout and stderr
exec 1>&6 2>&7 6>&- 7>&-

elapsed=$(( SECONDS - start_time ))
echo "Test completed in ${elapsed}s"

if [ $testExitCode -ne 0 ]; then
    echo "FAILED - Check data/test_coverage_output.log for details"
    exit ${testExitCode}
fi

echo ""
echo "Coverage Summary:"
if ! go tool cover -func=coverage.out | tail -1; then
    echo "FAILED - Could not generate coverage function summary"
    exit 1
fi

echo ""
echo "Generating HTML coverage report..."
if ! go tool cover -html=coverage.out -o coverage.html; then
    echo "FAILED - Could not generate HTML coverage report"
    exit 1
fi

echo ""
echo "PASSED - Full output in data/test_coverage_output.log"
echo "HTML coverage report: coverage.html"
