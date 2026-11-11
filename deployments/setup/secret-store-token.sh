#!/bin/bash
# Generates the local in-house secret store token once per checkout and prints
# it. docker compose passes it to agent-manager-service (env_file
# deployments/.secret-store.env) and setup-secret-store.sh gives the same value
# to the ClusterSecretStore. The file is gitignored: never commit a token.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOKEN_FILE="${SCRIPT_DIR}/../.secret-store.env"

if [ ! -s "$TOKEN_FILE" ]; then
    umask 077
    printf 'SECRET_STORE_TOKEN=%s\n' "$(openssl rand -hex 32)" > "$TOKEN_FILE"
fi
sed -n 's/^SECRET_STORE_TOKEN=//p' "$TOKEN_FILE"
