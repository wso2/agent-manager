#!/bin/bash

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

echo "PASSED - Full output in data/test_coverage_output.log"
echo ""
echo "Coverage Summary:"
go tool cover -func=coverage.out | tail -1
echo ""
echo "Generating HTML coverage report..."
go tool cover -html=coverage.out -o coverage.html
echo "HTML coverage report: coverage.html"
