#!/bin/bash
set -e

# Build Docker images from production Dockerfiles and import them into k3d.
# Usage:
#   ./build-and-import.sh              # Build and import all components
#   ./build-and-import.sh api          # Build and import API only
#   ./build-and-import.sh console api  # Build and import Console and API
#
# Supported components: api, console, traces-observer, evaluation-job

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

source "$SCRIPT_DIR/env.sh"

# Components and their build contexts (relative to ROOT_DIR)
declare -A COMPONENT_CONTEXT=(
    [api]="agent-manager-service"
    [console]="console"
    [traces-observer]="traces-observer-service"
    [evaluation-job]="evaluation-job"
)

declare -A COMPONENT_IMAGE=(
    [api]="amp-api"
    [console]="amp-console"
    [traces-observer]="amp-traces-observer"
    [evaluation-job]="amp-evaluation-job"
)

ALL_COMPONENTS="api console traces-observer evaluation-job"

# Determine which components to build
if [ $# -eq 0 ]; then
    COMPONENTS="$ALL_COMPONENTS"
else
    COMPONENTS="$*"
fi

# Validate component names
for comp in $COMPONENTS; do
    if [ -z "${COMPONENT_CONTEXT[$comp]}" ]; then
        echo "Unknown component: $comp"
        echo "Valid components: $ALL_COMPONENTS"
        exit 1
    fi
done

echo "=== Building and importing images into k3d ==="
echo ""

# Verify k3d cluster exists
if ! k3d cluster list 2>/dev/null | grep -q "${CLUSTER_NAME}"; then
    echo "k3d cluster '${CLUSTER_NAME}' not found. Run 'make setup-k3d' first."
    exit 1
fi

FAILED=""

for comp in $COMPONENTS; do
    IMAGE="${COMPONENT_IMAGE[$comp]}:${AMP_IMAGE_TAG}"
    CONTEXT="${ROOT_DIR}/${COMPONENT_CONTEXT[$comp]}"

    echo "Building ${comp} -> ${IMAGE}..."
    if docker build -t "$IMAGE" "$CONTEXT" --quiet; then
        echo "Importing ${IMAGE} into k3d cluster..."
        if k3d image import "$IMAGE" -c "${CLUSTER_NAME}"; then
            echo "${comp} ready."
        else
            echo "Failed to import ${comp}."
            FAILED="$FAILED $comp"
        fi
    else
        echo "Failed to build ${comp}."
        FAILED="$FAILED $comp"
    fi
    echo ""
done

if [ -n "$FAILED" ]; then
    echo "Failed components:${FAILED}"
    exit 1
fi

echo "All images built and imported successfully."
