#!/usr/bin/env bash
# build-vm-bundle.sh — package the combined install bundle attached to a release.
#
# The bundle is a single tarball with everything install-vm.sh / install-advanced.sh /
# start.sh read at runtime, so installers need ONE download instead of a git clone
# plus many raw.githubusercontent fetches (which GitHub rate-limits per IP). It
# reproduces the repo layout so install.sh resolves DEPLOYMENTS_DIR (=deployments/)
# and its single-cluster/values/k8s/scripts siblings exactly as in a checkout.
#
# Also copies bootstrap.sh, start.sh, k3d-install.sh and k3d-install-subscription.sh
# out alongside the tarball so each can be attached as its own release asset (the
# stable curl entry points for the VM, dev-container, host k3d and subscription
# host k3d install paths respectively — piping a tarball to bash doesn't work,
# so these stay standalone files even though a copy also lives inside the bundle).
#
# Usage: build-vm-bundle.sh <version> <out-dir>   (run from the repo root)
set -euo pipefail

VERSION="${1:?usage: build-vm-bundle.sh <version> <out-dir>}"
OUT_DIR="${2:?usage: build-vm-bundle.sh <version> <out-dir>}"
mkdir -p "$OUT_DIR"

BUNDLE="${OUT_DIR}/wso2-agent-manager-${VERSION}.tar.gz"

# Directories the installers read at runtime (see install.sh DEPLOYMENTS_DIR +
# install-helpers.sh ../scripts). setup/ is here because install-kata.sh and
# install-gvisor.sh resolve ../k8s/ relative to themselves, and fall back to
# fetching those manifests over the network when they are not shipped alongside.
# Charts are pulled from OCI registries at install time, so helm-charts/ is
# intentionally excluded.
tar -czf "$BUNDLE" \
  deployments/vm \
  deployments/quick-start \
  deployments/single-cluster \
  deployments/values \
  deployments/k8s \
  deployments/scripts \
  deployments/setup

cp deployments/vm/bootstrap.sh "${OUT_DIR}/bootstrap.sh"
cp deployments/quick-start/start.sh "${OUT_DIR}/start.sh"
cp deployments/quick-start/k3d-install.sh "${OUT_DIR}/k3d-install.sh"
cp deployments/quick-start/k3d-install-subscription.sh "${OUT_DIR}/k3d-install-subscription.sh"

# MANIFEST.json — what this bundle expects to find at install time, in a form a
# downstream publisher can read without parsing release notes. The download is
# distributed outside GitHub, so "which images does this expect, and from where"
# has to travel with the artifact rather than living in the workflow that built
# it. Image digests are not included: they are only knowable after the push, and
# recording a guess is worse than recording nothing.
CONFIG_FILE="${CONFIG_FILE:-.github/release-config.json}"
IMAGE_REGISTRY="${REGISTRY:-ghcr.io}/${REGISTRY_ORG:-wso2}"
CHART_REGISTRY="${HELM_REGISTRY:-oci://ghcr.io/wso2}"
BUILD_COMMIT="$(git rev-parse HEAD 2>/dev/null || echo unknown)"

if [ -f "$CONFIG_FILE" ] && command -v jq >/dev/null 2>&1; then
  jq -n \
    --arg version "$VERSION" \
    --arg buildCommit "$BUILD_COMMIT" \
    --arg imageRegistry "$IMAGE_REGISTRY" \
    --arg chartRegistry "$CHART_REGISTRY" \
    --slurpfile config "$CONFIG_FILE" \
    '{
       version: $version,
       buildCommit: $buildCommit,
       imageRegistry: $imageRegistry,
       chartRegistry: $chartRegistry,
       images: [$config[0].images[] | { name: .name, tag: ("v" + $version) }],
       charts: [$config[0].charts[] | { name: ., version: $version }]
     }' > "${OUT_DIR}/MANIFEST.json"
  echo "✅ Wrote manifest: ${OUT_DIR}/MANIFEST.json"
else
  echo "⚠️  Skipping MANIFEST.json (need jq and ${CONFIG_FILE})" >&2
fi

# SHA256SUMS — the integrity contract for a download served from somewhere other
# than the GitHub release it was built in. Written last so it covers everything,
# and in the format `sha256sum -c` reads, so verifying is one command.
(
  cd "$OUT_DIR"
  # Basenames only: the file is consumed next to the artifacts, wherever they
  # are republished, so an absolute build path would make it unusable.
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum ./* > SHA256SUMS.tmp 2>/dev/null || true
  else
    # macOS has shasum instead; -a 256 matches sha256sum's output format.
    shasum -a 256 ./* > SHA256SUMS.tmp 2>/dev/null || true
  fi
  grep -v 'SHA256SUMS' SHA256SUMS.tmp | sed 's#\./##' > SHA256SUMS
  rm -f SHA256SUMS.tmp
)
echo "✅ Wrote checksums: ${OUT_DIR}/SHA256SUMS"

echo "✅ Built install bundle: $BUNDLE"
echo "   Contents (top level):"
# head closes the pipe after 20 lines; under `set -o pipefail` the SIGPIPE tar/sed
# then take would fail the build, so cap first and swallow the expected broken pipe.
tar -tzf "$BUNDLE" | head -n 20 | sed 's/^/     /' || true
echo "✅ Copied bootstrap.sh: ${OUT_DIR}/bootstrap.sh"
echo "✅ Copied start.sh: ${OUT_DIR}/start.sh"
echo "✅ Copied k3d-install.sh: ${OUT_DIR}/k3d-install.sh"
echo "✅ Copied k3d-install-subscription.sh: ${OUT_DIR}/k3d-install-subscription.sh"
