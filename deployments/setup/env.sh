# shellcheck shell=bash
# Shared cluster environment variables — sourced by all scripts in this directory.
CLUSTER_NAME="${CLUSTER_NAME:-amp-dev}"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"

# WSO2 API Platform / Gateway Operator versions
GATEWAY_OPERATOR_VERSION="0.11.3"
GATEWAY_CHART_VERSION="2026.09.24"
GATEWAY_IMAGE_VERSION="2026.09.24"
# Namespace the gateway operator runs in (it watches every namespace).
GATEWAY_OPERATOR_NAMESPACE="amp-gateway-operator"

GATEWAY_ENCRYPTION_SECRET_NAME="gateway-encryption-keys"
GATEWAY_ENCRYPTION_SECRET_KEY="default-aesgcm256-v1.bin"

# Namespace holding OpenSearch, the OpenTelemetry collector and the
# Agent Manager Observer (see ../scripts/lib-observability.sh).
OBSERVABILITY_NAMESPACE="amp-observability"
