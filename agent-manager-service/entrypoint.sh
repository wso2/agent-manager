#!/bin/sh

# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

set -e

# Entrypoint script for agent-manager-service
# Generates JWT signing keys at runtime if they don't exist

echo "Starting agent-manager-service..."

# Generate JWT signing keys using the gen_keys.sh script
# This script will only generate keys if they don't already exist
if [ -f /app/scripts/gen_keys.sh ]; then
    echo "Running key generation script..."
    bash /app/scripts/gen_keys.sh "${JWT_SIGNING_ACTIVE_KEY_ID:-key-1}"
else
    echo "Warning: gen_keys.sh script not found, skipping key generation"
fi

# Start the application
echo "Starting agent-manager-service..."
exec /go/bin/agent-manager-service "$@"
