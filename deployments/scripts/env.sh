# Shared cluster environment variables — sourced by all scripts in this directory.
OPENCHOREO_VERSION="0.16.0"
CLUSTER_NAME="openchoreo-local-setup"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"

# Source utility functions
SCRIPT_DIR_ENV="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR_ENV/utils.sh"
