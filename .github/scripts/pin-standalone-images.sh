#!/usr/bin/env bash
# pin-standalone-images.sh — point the standalone compose bundle at released images.
#
# Rewrites the AMP image lines in deployments/standalone/.env.example from the
# locally built `amp-*:lite` tags to <registry>/<image>:<tag>, so a release
# bundle runs the images published for that release without building anything.
#
# Usage: pin-standalone-images.sh <tag> <registry/org>   (run from the repo root)
#   e.g. pin-standalone-images.sh v1.2.0 ghcr.io/wso2
set -euo pipefail

TAG="${1:?usage: pin-standalone-images.sh <tag> <registry/org>}"
PREFIX="${2:?usage: pin-standalone-images.sh <tag> <registry/org>}"
ENV_FILE="deployments/standalone/.env.example"

[ -f "$ENV_FILE" ] || { echo "missing $ENV_FILE (run from the repo root)" >&2; exit 1; }

tmp="$(mktemp)"
sed -e "s#^AMP_API_IMAGE=.*#AMP_API_IMAGE=${PREFIX}/amp-api:${TAG}#" \
    -e "s#^AMP_OBSERVER_IMAGE=.*#AMP_OBSERVER_IMAGE=${PREFIX}/amp-observer:${TAG}#" \
    -e "s#^AMP_CONSOLE_IMAGE=.*#AMP_CONSOLE_IMAGE=${PREFIX}/amp-console:${TAG}#" \
    "$ENV_FILE" > "$tmp"
mv "$tmp" "$ENV_FILE"

for image in amp-api amp-observer amp-console; do
    grep -q "=${PREFIX}/${image}:${TAG}\$" "$ENV_FILE" \
        || { echo "failed to pin ${image} in $ENV_FILE" >&2; exit 1; }
done
echo "Pinned standalone images to ${PREFIX}/*:${TAG}"
