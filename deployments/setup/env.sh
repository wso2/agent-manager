# Shared cluster environment variables — sourced by all scripts in this directory.
OPENCHOREO_VERSION="1.1.1"
CLUSTER_NAME="${CLUSTER_NAME:-openchoreo-local-setup}"
CLUSTER_CONTEXT="k3d-${CLUSTER_NAME}"

# WSO2 API Platform / Gateway Operator versions
GATEWAY_OPERATOR_VERSION="0.10.1"
GATEWAY_CHART_VERSION="1.2.0-beta"
GATEWAY_IMAGE_VERSION="1.2.0-beta"


GATEWAY_ENCRYPTION_SECRET_NAME="gateway-encryption-keys"
GATEWAY_ENCRYPTION_SECRET_KEY="default-aesgcm256-v1.bin"

# OpenChoreo community module versions compatible with OpenChoreo 1.1.1
OBSERVABILITY_LOGS_OPENSEARCH_VERSION="0.4.1"
OBSERVABILITY_TRACING_OPENSEARCH_VERSION="0.4.1"
OBSERVABILITY_METRICS_PROMETHEUS_VERSION="0.6.1"

# Agent Sandbox community module
AGENT_SANDBOX_MODULE_VERSION="0.1.1"    # helm chart version — update when new releases land
AGENT_SANDBOX_UPSTREAM_VERSION="v0.4.6" # upstream controller version (default in chart)

# gVisor isolation tier — a dedicated runsc node added to the EXISTING cluster
# (see `make setup-gvisor`). runsc is installed once, on this new empty node, so
# existing runc agents are never disrupted. Agents in an environment whose
# isolationTier is "gvisor" get runtimeClassName=gvisor and land on this node.
GVISOR_NODE_NAME="gvisor"          # k3d agent node name → node/container "k3d-gvisor-0"
GVISOR_RUNTIME_CLASS="gvisor"      # RuntimeClass name (handler: runsc)
GVISOR_NODE_LABEL_KEY="gvisor"     # node label + RuntimeClass scheduling key + taint key
GVISOR_NODE_LABEL_VALUE="true"
# Set to "true" to start runsc in host-network passthrough mode instead of the
# default userspace netstack. Keeps gVisor syscall isolation but uses the host
# network stack inside the pod netns — use this only if cross-node Service traffic
# (traces/metrics/gateway) fails under the default netstack on your CNI/overlay.
GVISOR_NETWORK_HOST="${GVISOR_NETWORK_HOST:-false}"

# Kata Containers isolation tier — a real (non-Docker) node joined to the EXISTING cluster
# (see `make setup-kata`) on which kata-deploy installs the Kata runtime. Unlike
# gVisor (userspace kernel), Kata boots each agent in a lightweight VM with its own
# kernel, so the node MUST expose nested virtualization (/dev/kvm). Agents in an
# environment whose isolationTier is "kata" get runtimeClassName=kata-qemu and land
# on this node. kata-deploy + the install touch ONLY this new node — the server node
# and existing runc agents are never reconfigured, so there is no downtime.
KATA_RUNTIME_CLASS="kata-qemu"    # RuntimeClass name == handler kata-deploy registers
KATA_NODE_LABEL_KEY="kata"        # node label + RuntimeClass scheduling key + taint key
KATA_NODE_LABEL_VALUE="true"
KATA_VERSION="3.2.0"              # kata-containers release used by kata-deploy (RBAC + DaemonSet)
