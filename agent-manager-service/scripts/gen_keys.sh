#!/bin/bash

# Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

# Script to generate RSA key pair for JWT signing
# Keys are generated in the keys/ directory

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
KEYS_DIR="${PROJECT_ROOT}/keys"
PRIVATE_KEY_PATH="${KEYS_DIR}/private.pem"
PUBLIC_KEY_PATH="${KEYS_DIR}/public.pem"

# Create keys directory if it doesn't exist
mkdir -p "${KEYS_DIR}"

# Check if keys already exist
if [ -f "${PRIVATE_KEY_PATH}" ] && [ -f "${PUBLIC_KEY_PATH}" ]; then
    echo "✓ JWT signing keys already exist at ${KEYS_DIR}"
    echo "  Private key: ${PRIVATE_KEY_PATH}"
    echo "  Public key: ${PUBLIC_KEY_PATH}"
    exit 0
fi

echo "Generating RSA key pair for JWT signing..."

# Generate private key (2048-bit RSA)
openssl genrsa -out "${PRIVATE_KEY_PATH}" 2048

# Generate public key from private key
openssl rsa -in "${PRIVATE_KEY_PATH}" -pubout -out "${PUBLIC_KEY_PATH}"

# Set appropriate permissions
chmod 600 "${PRIVATE_KEY_PATH}"
chmod 644 "${PUBLIC_KEY_PATH}"

echo "✓ Successfully generated RSA key pair"
echo "  Private key: ${PRIVATE_KEY_PATH}"
echo "  Public key: ${PUBLIC_KEY_PATH}"