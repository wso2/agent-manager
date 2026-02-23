#!/bin/bash
set -e

# Install openssl and kubectl
apk add --no-cache openssl kubectl

# Check if secret already exists
if kubectl get secret {{ include "agent-management-platform.tlsCertsSecretName" . }} -n {{ .Release.Namespace }} 2>/dev/null; then
  echo "TLS certificates secret already exists, skipping generation"
  exit 0
fi

echo "Generating self-signed TLS certificate for agent-manager-service..."
# Configuration
VALIDITY_DAYS=365
ORGANIZATION="Platform API Dev"
COUNTRY="US"
# Build Subject Alternative Names
SAN_DNS="DNS:localhost,DNS:amp-api.{{ .Release.Namespace }}.svc.cluster.local,DNS:amp-api.{{ .Release.Namespace }}.svc,DNS:amp-api"
SAN_IP="IP:127.0.0.1,IP:::1"
SUBJECT_ALT_NAME="${SAN_DNS},${SAN_IP}"
echo "  Organization: ${ORGANIZATION}"
echo "  Validity: ${VALIDITY_DAYS} days"
echo "  Subject Alternative Names: ${SUBJECT_ALT_NAME}"
# Generate private key
echo "Generating private key..."
openssl genrsa -out /tmp/key.pem 2048
# Generate self-signed certificate
echo "Generating certificate..."
openssl req -new -x509 -sha256 \
    -key tmp/key.pem \
    -out tmp/cert.pem \
    -days "${VALIDITY_DAYS}" \
    -subj "/C=${COUNTRY}/O=${ORGANIZATION}" \
    -addext "keyUsage=critical,digitalSignature,keyEncipherment" \
    -addext "extendedKeyUsage=serverAuth" \
    -addext "subjectAltName=${SUBJECT_ALT_NAME}"
echo "✓ Certificate generated successfully"
# Create Kubernetes secret
echo "Creating Kubernetes secret..."

# Show certificate details
echo ""
echo "Certificate details:"
openssl x509 -in tmp/cert.pem -noout -subject -issuer -dates -ext subjectAltName || true