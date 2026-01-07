#!/bin/bash

export ENV_FILE_PATH=$(pwd)/.env
echo "Running tests"
echo "Using ENV_FILE_PATH: $ENV_FILE_PATH"

# Create localdata directory if it doesn't exist
mkdir -p localdata

# Record start time
start_time=$SECONDS

# Run tests with output to both terminal and log file
go test -v --race ./... 2>&1 | tee localdata/test_output.log

testExitCode=${PIPESTATUS[0]}

elapsed=$(( SECONDS - start_time ))
echo "Test completed in ${elapsed}s"

if [ $testExitCode -ne 0 ]; then
    echo "FAILED - Full output saved in localdata/test_output.log"
    exit ${testExitCode}
fi
echo "PASSED - Full output saved in localdata/test_output.log"
