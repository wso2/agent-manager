#!/bin/bash

# Set environment file path if .env exists
if [ -f .env ]; then
    export ENV_FILE_PATH=$(pwd)/.env
    echo "Using ENV_FILE_PATH: $ENV_FILE_PATH"
fi

echo "Running tests for traces-observer-service"

# Create data directory if it doesn't exist
mkdir -p data

# Record start time
start_time=$SECONDS

# Save original stdout and stderr
exec 6>&1 7>&2

# Redirect both stdout and stderr to log file
exec > data/test_output.log 2>&1

go test -v --race ./...

testExitCode=$?

# Restore original stdout and stderr
exec 1>&6 2>&7 6>&- 7>&-

elapsed=$(( SECONDS - start_time ))
echo "Test completed in ${elapsed}s"

if [ $testExitCode -ne 0 ]; then
    echo "FAILED - Check data/test_output.log for details"
    exit ${testExitCode}
fi
echo "PASSED - Full output in data/test_output.log"
