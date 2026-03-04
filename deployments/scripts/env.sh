# Shared cluster environment variables — sourced by all scripts in this directory.
OPENCHOREO_VERSION="0.14.0"
OPENCHOREO_PATCH_VERSION="0.0.0-b53c6dc3"
CLUSTER_NAME="openchoreo-local-v${OPENCHOREO_VERSION}"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"

# AMP (Agent Management Platform) variables
AMP_NAMESPACE="wso2-amp"
AMP_RELEASE_NAME="amp"
AMP_IMAGE_TAG="0.0.0-dev"
