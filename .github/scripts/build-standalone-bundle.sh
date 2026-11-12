#!/usr/bin/env bash
# build-standalone-bundle.sh — package the Docker Compose bundle attached to a release.
#
# The bundle is deployments/standalone without the overlays that build from
# source (docker-compose.build.yml, docker-compose.dev.yml), so
# `tar xzf … && cd standalone && cp .env.example .env && docker compose up -d --wait`
# runs the released images with nothing else to download from this repository.
# Run pin-standalone-images.sh first.
#
# Usage: build-standalone-bundle.sh <version> <out-dir>   (run from the repo root)
set -euo pipefail

VERSION="${1:?usage: build-standalone-bundle.sh <version> <out-dir>}"
OUT_DIR="${2:?usage: build-standalone-bundle.sh <version> <out-dir>}"
mkdir -p "$OUT_DIR"

if grep -qE '^AMP_[A-Z_]+_IMAGE=amp-[a-z]+:lite$' deployments/standalone/.env.example; then
    echo "deployments/standalone/.env.example still uses local images; run pin-standalone-images.sh first" >&2
    exit 1
fi

BUNDLE="${OUT_DIR}/wso2-agent-manager-standalone-${VERSION}.tar.gz"
tar -czf "$BUNDLE" \
    -C deployments \
    --exclude 'standalone/docker-compose.dev.yml' \
    --exclude 'standalone/docker-compose.build.yml' \
    --exclude 'standalone/.env' \
    standalone

( cd "$OUT_DIR" && sha256sum "$(basename "$BUNDLE")" > "$(basename "$BUNDLE").sha256" )
echo "Built ${BUNDLE}"
