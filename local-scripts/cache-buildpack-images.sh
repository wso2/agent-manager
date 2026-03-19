#!/bin/bash
# Cache buildpack images in the local k3d registry to speed up builds.
# Run this once — after that, builds will pull images locally instead of from the internet.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOCAL_REGISTRY="localhost:10082"

IMAGES=(
  "docker.io/buildpacksio/lifecycle:0.20.5|buildpacks-cache/lifecycle:0.20.5"
  "gcr.io/buildpacks/builder@sha256:5977b4bd47d3e9ff729eefe9eb99d321d4bba7aa3b14986323133f40b622aef1|buildpacks-cache/google-builder:latest"
  "gcr.io/buildpacks/google-22/run:latest|buildpacks-cache/google-run:latest"
)

echo "Caching buildpack images to ${LOCAL_REGISTRY}..."
echo ""

for entry in "${IMAGES[@]}"; do
  remote="${entry%%|*}"
  cached="${entry##*|}"
  local_tag="${LOCAL_REGISTRY}/${cached}"

  echo "==> Pulling ${remote}"
  docker pull "${remote}"

  echo "==> Tagging as ${local_tag}"
  docker tag "${remote}" "${local_tag}"

  echo "==> Pushing to local registry"
  docker push "${local_tag}"

  echo ""
done

echo "Images cached. Enabling buildpack cache in Helm..."
helm upgrade --install amp-custom-build-ci-workflows "${SCRIPT_DIR}/../deployments/helm-charts/wso2-amp-build-extension" \
    --namespace openchoreo-build-plane \
    --set global.defaultResources.buildpackCache.enabled=true

echo ""
echo "Done! Buildpack cache is now enabled. Builds will use local images."
