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
# KIND, either express or implied. See the License for the
# specific language governing permissions and limitations
# under the License.

# Script to generate self-signed TLS certificates for agent-manager-service
# This script generates certificates matching the original format:
# - Organization: Platform API Dev
# - Validity: 1 year
# - Subject Alternative Names: localhost, 127.0.0.1, ::1

set -e

# Configuration (all configurable via environment variables)
CERT_DIR="${INTERNAL_SERVER_CERT_DIR:-../data/certs}"
CERT_FILE="${CERT_DIR}/cert.pem"
KEY_FILE="${CERT_DIR}/key.pem"
VALIDITY_DAYS="${CERT_VALIDITY_DAYS:-365}"
ORGANIZATION="${CERT_ORGANIZATION:-Agent Manager Dev}"
COUNTRY="${CERT_COUNTRY:-US}"

# Additional SANs from environment (comma-separated DNS names or IPs)
# Example: CERT_EXTRA_SANS="amp-api.wso2-amp.svc.cluster.local,amp-api.wso2-amp.svc"
EXTRA_SANS="${CERT_EXTRA_SANS:-}"

# Create certificate directory if it doesn't exist
mkdir -p "${CERT_DIR}"

# Check if certificates already exist
if [ -f "${CERT_FILE}" ] && [ -f "${KEY_FILE}" ]; then
    echo "Certificates already exist at ${CERT_DIR}, skipping generation"
    echo "  Certificate: ${CERT_FILE}"
    echo "  Private Key: ${KEY_FILE}"
    exit 0
fi

echo "Generating self-signed TLS certificate..."
echo "  Certificate directory: ${CERT_DIR}"
echo "  Validity: ${VALIDITY_DAYS} days"
echo "  Organization: ${ORGANIZATION}"

# Build Subject Alternative Names
SAN_DNS="DNS:localhost"
SAN_IP="IP:127.0.0.1,IP:::1"

# Add extra SANs if provided
if [ -n "${EXTRA_SANS}" ]; then
    echo "  Adding extra SANs: ${EXTRA_SANS}"
    IFS=',' read -ra SANS <<< "${EXTRA_SANS}"
    for san in "${SANS[@]}"; do
        san=$(echo "$san" | xargs)  # Trim whitespace
        # Check if it's an IP address or DNS name
        # IPv4: must match pattern N.N.N.N where N is 0-255
        # IPv6: must contain a colon (distinguishes from hex-only DNS labels)
        if [[ $san =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || [[ $san == *:* ]]; then
            SAN_IP="${SAN_IP},IP:${san}"
        else
            SAN_DNS="${SAN_DNS},DNS:${san}"
        fi
    done
fi

SUBJECT_ALT_NAME="${SAN_DNS},${SAN_IP}"
echo "  Subject Alternative Names: ${SUBJECT_ALT_NAME}"

# Generate private key
echo "Generating private key..."
openssl genrsa -out "${KEY_FILE}" 2048 2>/dev/null

# Generate self-signed certificate
echo "Generating certificate..."
openssl req -new -x509 -sha256 \
    -key "${KEY_FILE}" \
    -out "${CERT_FILE}" \
    -days "${VALIDITY_DAYS}" \
    -subj "/C=${COUNTRY}/O=${ORGANIZATION}" \
    -addext "keyUsage=critical,digitalSignature,keyEncipherment" \
    -addext "extendedKeyUsage=serverAuth" \
    -addext "subjectAltName=${SUBJECT_ALT_NAME}" \
    2>/dev/null

# Set appropriate permissions
chmod 644 "${CERT_FILE}"
chmod 600 "${KEY_FILE}"

echo "✓ Certificate generated successfully"
echo "  Certificate: ${CERT_FILE}"
echo "  Private Key: ${KEY_FILE}"
echo ""
echo "Certificate details:"
openssl x509 -in "${CERT_FILE}" -noout -subject -issuer -dates -ext subjectAltName 2>/dev/null || true
