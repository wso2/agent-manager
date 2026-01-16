#!/bin/bash

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

# Script to generate RSA key pair for JWT signing
# Keys are generated in the keys/ directory
# You must manually create/update public-keys-config.json
#
# Usage:
#   ./gen_keys.sh           # Generates keys for key-1 (default)
#   ./gen_keys.sh key-2     # Generates keys for key-2 (rotation)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
KEYS_DIR="${PROJECT_ROOT}/keys"
JSON_CONFIG="${KEYS_DIR}/public-keys-config.json"

# Parse optional key ID parameter
KEY_ID="${1:-key-1}"

# Key file paths
if [ "${KEY_ID}" = "key-1" ]; then
    # Default key names for initial setup
    PRIVATE_KEY_PATH="${KEYS_DIR}/private.pem"
    PUBLIC_KEY_PATH="${KEYS_DIR}/public.pem"
else
    # Custom key names with key ID for rotation
    PRIVATE_KEY_PATH="${KEYS_DIR}/private-${KEY_ID}.pem"
    PUBLIC_KEY_PATH="${KEYS_DIR}/public-${KEY_ID}.pem"
fi

# Create keys directory if it doesn't exist
mkdir -p "${KEYS_DIR}"

# Check if keys already exist
if [ -f "${PRIVATE_KEY_PATH}" ] && [ -f "${PUBLIC_KEY_PATH}" ]; then
    echo "✓ JWT signing keys already exist for '${KEY_ID}'"
    echo "  Private key: ${PRIVATE_KEY_PATH}"
    echo "  Public key: ${PUBLIC_KEY_PATH}"
    exit 0
fi

echo "Generating RSA key pair for JWT signing (key: ${KEY_ID})..."

# Generate private key (2048-bit RSA)
openssl genrsa -out "${PRIVATE_KEY_PATH}" 2048

# Generate public key from private key
openssl rsa -in "${PRIVATE_KEY_PATH}" -pubout -out "${PUBLIC_KEY_PATH}"

# Set appropriate permissions
chmod 600 "${PRIVATE_KEY_PATH}"
chmod 644 "${PUBLIC_KEY_PATH}"

echo "✓ Successfully generated RSA key pair for '${KEY_ID}'"
echo "  Private key: ${PRIVATE_KEY_PATH}"
echo "  Public key: ${PUBLIC_KEY_PATH}"

echo ""

# Check if JSON config exists
if [ ! -f "${JSON_CONFIG}" ]; then
    # Create initial JSON config
    cat > "${JSON_CONFIG}" <<EOF
{
  "keys": [
    {
      "kid": "${KEY_ID}",
      "algorithm": "RS256",
      "publicKeyPath": "${PUBLIC_KEY_PATH}",
      "description": "Initial JWT signing key",
      "createdAt": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    }
  ]
}
EOF
    echo "✓ Created ${JSON_CONFIG}"
    echo ""
else
    # JSON config exists, provide instructions to add new key
    echo "⚠ JSON config already exists at ${JSON_CONFIG}"
    echo ""
    echo "Please add the new key to the 'keys' array in ${JSON_CONFIG}:"
    echo ""
    echo "   {"
    echo "     \"kid\": \"${KEY_ID}\","
    echo "     \"algorithm\": \"RS256\","
    echo "     \"publicKeyPath\": \"${PUBLIC_KEY_PATH}\","
    echo "     \"description\": \"Key for rotation\","
    echo "     \"createdAt\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\""
    echo "   }"
    echo ""
fi

echo "Next steps:"
echo "============"
echo ""
echo "Update your .env file with:"
echo "  JWT_SIGNING_PRIVATE_KEY_PATH=${PRIVATE_KEY_PATH}"
echo "  JWT_SIGNING_ACTIVE_KEY_ID=${KEY_ID}"
echo "  JWT_SIGNING_PUBLIC_KEYS_CONFIG=${JSON_CONFIG}"
echo ""
