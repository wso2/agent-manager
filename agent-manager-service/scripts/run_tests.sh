#!/bin/bash

export ENV_FILE_PATH=$(pwd)/.env
echo "Running tests"
echo "Using ENV_FILE_PATH: $ENV_FILE_PATH"

# Create localdata directory if it doesn't exist
mkdir -p localdata

# Record start time
start_time=$SECONDS

# Save original stdout and stderr
exec 6>&1 7>&2

# Redirect both stdout and stderr to log file
exec > localdata/test_output.log 2>&1

go test -v  --race  ./...

testExitCode=$?

# Restore original stdout and stderr
exec 1>&6 2>&7 6>&- 7>&-

elapsed=$(( SECONDS - start_time ))
echo "Test completed in ${elapsed}s"

if [ $testExitCode -ne 0 ]; then
    echo "FAILED - Check localdata/test_output.log for details"
    exit ${testExitCode}
fi
echo "PASSED - Full output in localdata/test_output.log"
