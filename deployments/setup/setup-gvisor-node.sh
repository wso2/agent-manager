#!/bin/bash

# Adds a single gVisor (runsc) node to the EXISTING k3d cluster and registers a
# `gvisor` RuntimeClass, so agents deployed to a gVisor-tier environment run under
# kernel isolation instead of the default runc.
#
# Why this is safe to run on a live cluster: installing runsc requires
# reconfiguring containerd and restarting the node. We do that ONLY on the new,
# empty node we just created — the server node and every running (runc) agent are
# never touched, so there is no downtime.
#
# Prerequisites:
#   - Main cluster running (make setup)
#   - curl, docker, kubectl, k3d available
#
# Idempotent: re-running skips node creation / runsc install / config when already done.
#
# Networking: by default the gVisor pods use runsc's userspace netstack. If
# cross-node Service traffic (traces, metrics, gateway/try-it) fails on your
# CNI/overlay, re-run with GVISOR_NETWORK_HOST=true to start runsc in host-network
# passthrough mode (keeps syscall isolation; see env.sh and gvisor-isolation-tier.md).
#
# Usage:  make setup-gvisor      (or:  ./setup-gvisor-node.sh)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"
source "$SCRIPT_DIR/env.sh"
source "$SCRIPT_DIR/utils.sh"

echo "=== Adding gVisor isolation node to cluster '${CLUSTER_NAME}' ==="

# --- Preconditions ---
if ! kubectl cluster-info --context "$CLUSTER_CONTEXT" &>/dev/null; then
    echo "❌ k3d cluster '$CLUSTER_CONTEXT' is not running. Run: make setup"
    exit 1
fi
kubectl config use-context "$CLUSTER_CONTEXT" >/dev/null

SERVER_CONTAINER="k3d-${CLUSTER_NAME}-server-0"

# --- Raise inotify limits on the host kernel BEFORE creating the node ---
# k3d nodes share the host kernel. After a full `make setup`, the server node's
# workload exhausts the default fs.inotify.max_user_instances (128), so a freshly
# created node's containerd CRI plugin fails ("too many open files") and
# `k3d node create --wait` HANGS forever. Bump the limit via the (privileged) server
# container — the write propagates to the shared kernel — so the new node's containerd
# can start. Must happen before node creation; idempotent. Applies to Colima, GCP,
# and bare Linux alike.
if docker ps --filter "name=${SERVER_CONTAINER}" --format '{{.Names}}' | grep -q "${SERVER_CONTAINER}"; then
    echo "⚙️  Raising inotify limits on the host kernel (needed by containerd CRI)..."
    docker exec "${SERVER_CONTAINER}" sysctl -w fs.inotify.max_user_instances=512 >/dev/null 2>&1 || true
    docker exec "${SERVER_CONTAINER}" sysctl -w fs.inotify.max_user_watches=524288 >/dev/null 2>&1 || true
fi

# --- 1. Create the gVisor agent node (skip if present) ---
# Detect by Docker container name — more reliable than k3d node list which can
# lag behind actual container state in some k3d versions.
if docker ps -a --filter "name=k3d-${GVISOR_NODE_NAME}-" --format '{{.Names}}' 2>/dev/null | grep -q "k3d-${GVISOR_NODE_NAME}-"; then
    echo "✅ gVisor node already exists — skipping creation"
else
    # Clear any stale registration from a previously deleted node of the same name.
    # When a k3d node is deleted, the container goes away but the k3s server keeps the
    # node-password secret and the Node object. A recreated node then generates a new
    # password and the server rejects it ("Node password rejected, duplicate hostname"),
    # leaving k3d node create --wait hanging. Safe here: we are in the create branch, so
    # no live container of this name exists and any lingering entries are stale.
    echo "🧹 Clearing any stale registration for k3d-${GVISOR_NODE_NAME}-0..."
    kubectl delete secret "k3d-${GVISOR_NODE_NAME}-0.node-password.k3s" -n kube-system --ignore-not-found >/dev/null 2>&1 || true
    kubectl delete node "k3d-${GVISOR_NODE_NAME}-0" --ignore-not-found >/dev/null 2>&1 || true

    echo "🚀 Creating gVisor agent node in cluster '${CLUSTER_NAME}'..."
    k3d node create "${GVISOR_NODE_NAME}" --cluster "${CLUSTER_NAME}" --role agent --wait
fi

NODE_CONTAINER="$(docker ps --filter "name=k3d-${GVISOR_NODE_NAME}-" --format '{{.Names}}' | head -1)"
if [ -z "$NODE_CONTAINER" ]; then
    echo "❌ Could not find the gVisor node container (k3d-${GVISOR_NODE_NAME}-*)"
    exit 1
fi
NODE_NAME="$NODE_CONTAINER" # k3d's k8s node name == container name
echo "   Node: ${NODE_NAME}"

# --- 2. Match the cluster's registry mirror on the new node ---
# Copy registries.yaml from the server node so the gVisor node can pull agent
# images from the local workflow-plane registry (host.k3d.internal:10082).
if docker cp "${SERVER_CONTAINER}:/etc/rancher/k3s/registries.yaml" /tmp/k3d-registries.yaml 2>/dev/null; then
    docker exec "${NODE_CONTAINER}" mkdir -p /etc/rancher/k3s
    docker cp /tmp/k3d-registries.yaml "${NODE_CONTAINER}:/etc/rancher/k3s/registries.yaml"
    rm -f /tmp/k3d-registries.yaml
    echo "   ✅ Registry mirror config copied from server node"
fi

# --- 3. Install the runsc binaries (skip if already present) ---
# runsc needs its sidecar binaries in /usr/local/bin/gvisor-bin/ 
if docker exec "${NODE_CONTAINER}" test -f /usr/local/bin/runsc 2>/dev/null \
    && docker exec "${NODE_CONTAINER}" test -x /usr/local/bin/gvisor-bin/gvisor_sentry 2>/dev/null; then
    echo "✅ runsc binaries already present on ${NODE_NAME}"
else
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64)        GVISOR_ARCH="x86_64" ;;
        aarch64|arm64) GVISOR_ARCH="aarch64" ;; # macOS reports arm64; gVisor bucket uses aarch64
        *) echo "❌ Unsupported architecture: $ARCH"; exit 1 ;;
    esac

    if command -v zstd &>/dev/null; then
        TARBALL="gvisor.tar.zstd"; DECOMPRESS=(zstd -dc)
    elif command -v bzip2 &>/dev/null; then
        TARBALL="gvisor.tar.bz2"; DECOMPRESS=(bzip2 -dc)
    else
        echo "❌ zstd or bzip2 is required to unpack the gVisor release."
        exit 1
    fi
    # macOS has shasum but not sha512sum.
    if command -v sha512sum &>/dev/null; then SHA512=(sha512sum); else SHA512=(shasum -a 512); fi

    echo "📥 Downloading gVisor release (${GVISOR_ARCH})..."
    BASE="https://storage.googleapis.com/gvisor/releases/release/latest/${GVISOR_ARCH}"
    GVISOR_TMP="$(mktemp -d)"
    trap 'rm -rf "${GVISOR_TMP}"' EXIT
    curl -fsSL --retry 3 "${BASE}/${TARBALL}" -o "${GVISOR_TMP}/${TARBALL}"
    curl -fsSL --retry 3 "${BASE}/${TARBALL}.sha512" -o "${GVISOR_TMP}/${TARBALL}.sha512"
    ( cd "${GVISOR_TMP}" && "${SHA512[@]}" -c "${TARBALL}.sha512" >/dev/null ) || {
        echo "❌ gVisor release checksum verification failed — aborting."
        exit 1
    }
    "${DECOMPRESS[@]}" "${GVISOR_TMP}/${TARBALL}" \
        | tar -xf - -C "${GVISOR_TMP}" runsc containerd-shim-runsc-v1 gvisor-bin

    echo "📦 Installing runsc into ${NODE_NAME}..."
    docker exec "${NODE_CONTAINER}" mkdir -p /usr/local/bin
    docker cp "${GVISOR_TMP}/runsc" "${NODE_CONTAINER}:/usr/local/bin/runsc"
    docker cp "${GVISOR_TMP}/containerd-shim-runsc-v1" "${NODE_CONTAINER}:/usr/local/bin/containerd-shim-runsc-v1"
    # runsc looks for its sidecars in gvisor-bin/ next to itself
    docker exec "${NODE_CONTAINER}" rm -rf /usr/local/bin/gvisor-bin
    docker cp "${GVISOR_TMP}/gvisor-bin" "${NODE_CONTAINER}:/usr/local/bin/gvisor-bin"
    rm -rf "${GVISOR_TMP}"
    trap - EXIT
fi

# --- 3b. Configure containerd's runsc runtime + network mode (IDEMPOTENT, every run) ---
# Applied on every run — NOT gated on first install — so toggling GVISOR_NETWORK_HOST
# takes effect without recreating the node. Restarts the node only when the config
# actually changes. The '{{ template "base" . }}' prefix is REQUIRED so k3s renders its
# default config (CNI + registry mirrors) first; without it CNI fails (pods NotReady).
#
# GVISOR_NETWORK_HOST=true → runsc runs with --network=host (host network stack inside
# the pod's netns). Needed on k3d/flannel because runsc's default userspace netstack
# refuses DIRECT pod-IP ingress (kgateway/Envoy dials pod IPs, not the ClusterIP), which
# breaks Try-It while egress/Service traffic still works. Host mode fixes pod-IP ingress
# and keeps full syscall isolation. Marker for host mode = "ConfigPath" in the template.
echo "⚙️  Ensuring containerd runsc config (network host: ${GVISOR_NETWORK_HOST})..."
RUNSC_CFG_RESULT=$(docker exec -e GVISOR_NETWORK_HOST="${GVISOR_NETWORK_HOST}" "${NODE_CONTAINER}" /bin/sh -c '
    set -e
    chmod +x /usr/local/bin/runsc /usr/local/bin/containerd-shim-runsc-v1 2>/dev/null || true
    TMPL=/var/lib/rancher/k3s/agent/etc/containerd/config.toml.tmpl
    mkdir -p "$(dirname "$TMPL")"
    HAS_RUNSC=no; if grep -q "runtimes.runsc" "$TMPL" 2>/dev/null; then HAS_RUNSC=yes; fi
    HAS_HOST=no;  if grep -q "ConfigPath"     "$TMPL" 2>/dev/null; then HAS_HOST=yes; fi
    NEED=no
    if [ "$HAS_RUNSC" = no ]; then NEED=yes; fi
    if [ "$GVISOR_NETWORK_HOST" = true ] && [ "$HAS_HOST" = no ];  then NEED=yes; fi
    if [ "$GVISOR_NETWORK_HOST" != true ] && [ "$HAS_HOST" = yes ]; then NEED=yes; fi
    if [ "$NEED" = yes ]; then
        if [ "$GVISOR_NETWORK_HOST" = true ]; then
            printf "{{ template \"base\" . }}\n\n[plugins.\"io.containerd.grpc.v1.cri\".containerd.runtimes.runsc]\n  runtime_type = \"io.containerd.runsc.v1\"\n  [plugins.\"io.containerd.grpc.v1.cri\".containerd.runtimes.runsc.options]\n    TypeUrl = \"io.containerd.runsc.v1.options\"\n    ConfigPath = \"/etc/containerd/runsc.toml\"\n" > "$TMPL"
            mkdir -p /etc/containerd
            printf "[runsc_config]\n  network = \"host\"\n" > /etc/containerd/runsc.toml
        else
            printf "{{ template \"base\" . }}\n\n[plugins.\"io.containerd.grpc.v1.cri\".containerd.runtimes.runsc]\n  runtime_type = \"io.containerd.runsc.v1\"\n" > "$TMPL"
            rm -f /etc/containerd/runsc.toml
        fi
        echo CHANGED
    fi
')

if echo "$RUNSC_CFG_RESULT" | grep -q CHANGED; then
    # docker restart (not SIGHUP): k3s reads the containerd config only at startup.
    echo "🔄 containerd runsc config changed — restarting ${NODE_NAME} to apply..."
    docker restart "${NODE_CONTAINER}" >/dev/null
else
    echo "✅ containerd runsc config already correct — no restart needed"
fi

echo "⏳ Waiting for the gVisor node to be Ready..."
kubectl wait --context "$CLUSTER_CONTEXT" --for=condition=Ready "node/${NODE_NAME}" --timeout=180s

# Generate a machine-id for Fluent Bit (DaemonSet mounts /etc/machine-id as a
# hostPath file; k3d containers don't have one by default). The shared
# generate_machine_ids helper only covers k3d-${CLUSTER_NAME}-* nodes, so the
# manually-named gVisor node (k3d-gvisor-0) is handled here.
if ! docker exec "${NODE_CONTAINER}" test -s /etc/machine-id 2>/dev/null; then
    docker exec "${NODE_CONTAINER}" sh -c \
        "cat /proc/sys/kernel/random/uuid | tr -d '-' > /etc/machine-id"
    echo "   ✅ machine-id generated for ${NODE_NAME}"
fi

# --- Ensure host.k3d.internal resolves on the new node ---
# k3d injects host.k3d.internal into the server node's /etc/hosts but NOT into
# manually-created agent nodes. Without it, kubelet can't pull agent images from
# the local workflow-plane registry (host.k3d.internal:10082).
# Note: this entry is lost if the node container is restarted; re-run this script
# to restore it (the check makes it idempotent).
HOST_K3D_IP=$(docker exec "${SERVER_CONTAINER}" /bin/sh -c \
    "grep 'host.k3d.internal' /etc/hosts | awk '{print \$1}' | head -1" 2>/dev/null || true)
if [ -n "$HOST_K3D_IP" ]; then
    if ! docker exec "${NODE_CONTAINER}" grep -q "host.k3d.internal" /etc/hosts 2>/dev/null; then
        docker exec "${NODE_CONTAINER}" /bin/sh -c \
            "echo '${HOST_K3D_IP} host.k3d.internal' >> /etc/hosts"
        echo "   ✅ host.k3d.internal (${HOST_K3D_IP}) added to ${NODE_NAME} /etc/hosts"
    else
        echo "   ✅ host.k3d.internal already present in ${NODE_NAME} /etc/hosts"
    fi
fi

# --- 4. RuntimeClass with scheduling ---
# `scheduling` makes the RuntimeClass admission controller automatically inject the
# nodeSelector and toleration into any pod that sets runtimeClassName: gvisor, so
# the agent-api ComponentType only has to set runtimeClassName.
echo "🧩 Creating/updating the '${GVISOR_RUNTIME_CLASS}' RuntimeClass..."
kubectl apply --context "$CLUSTER_CONTEXT" -f "$SCRIPT_DIR/../k8s/gvisor-runtimeclass.yaml"

# --- 5. Label + taint the node ---
# Label: matches the RuntimeClass nodeSelector. Taint: keeps non-gVisor pods off
# this node (gVisor pods tolerate it via the RuntimeClass scheduling stanza).
echo "🏷️  Labeling and tainting ${NODE_NAME}..."
kubectl label --context "$CLUSTER_CONTEXT" node "${NODE_NAME}" \
    "${GVISOR_NODE_LABEL_KEY}=${GVISOR_NODE_LABEL_VALUE}" --overwrite
kubectl taint --context "$CLUSTER_CONTEXT" node "${NODE_NAME}" \
    "${GVISOR_NODE_LABEL_KEY}=${GVISOR_NODE_LABEL_VALUE}:NoSchedule" --overwrite

# --- 6. Ensure the Fluent Bit log collector tolerates the gVisor taint ---
# Fluent Bit is a DaemonSet (one pod per node) that tails each node's container
# logs. The gVisor taint repels it, so without a toleration agents on the gVisor
# node produce NO runtime logs. setup-openchoreo.sh installs it with the chart value
# fluent-bit.tolerations[operator=Exists]; a log module installed without it gets the
# same value here through Helm rather than a patch on the Helm-owned DaemonSet, which
# would be lost whenever the DaemonSet is recreated. The upgrade pins the release's
# own installed chart version so --reuse-values never crosses a chart version change.
FB_NAMESPACE="openchoreo-observability-plane"
FB_RELEASE="observability-logs-opensearch"
FB_CHART="oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch"
# Tolerations as operator/key/effect lines. `Exists//` tolerates every taint and
# `Exists//NoSchedule` every NoSchedule taint, which covers the gVisor taint. The value set
# below is `Exists//`, so it may replace other entries without losing any node.
FB_TOLERATIONS_JSONPATH='{range .spec.template.spec.tolerations[*]}{.operator}/{.key}/{.effect}{"\n"}{end}'
fb_tolerates_all() { grep -qxE 'Exists//(NoSchedule)?' <<<"$1"; }
fb_print_helm_command() {
    echo "         helm upgrade ${FB_RELEASE} ${FB_CHART} \\"
    echo "           --kube-context ${CLUSTER_CONTEXT} --namespace ${FB_NAMESPACE} \\"
    echo "           --version <installed chart version, from: helm list -n ${FB_NAMESPACE}> \\"
    echo "           --reuse-values \\"
    echo "           --set \"fluent-bit.tolerations[0].operator=Exists\" \\"
    echo "           --set \"fluentBitCustomizations.clusterInstance=${CLUSTER_NAME}\""
}
if kubectl get daemonset fluent-bit -n "$FB_NAMESPACE" --context "$CLUSTER_CONTEXT" &>/dev/null; then
    echo "🪵 Ensuring Fluent Bit tolerates the gVisor taint (so logs are collected here)..."
    # `|| true`: under pipefail a missing or failing helm would otherwise abort the
    # script here instead of reaching the patch fallback below.
    FB_CHART_VERSION=""
    FB_RELEASE_TOLERATIONS=""
    if command -v helm &>/dev/null; then
        FB_CHART_VERSION="$(helm list -n "$FB_NAMESPACE" --kube-context "$CLUSTER_CONTEXT" \
            --filter "^${FB_RELEASE}\$" -o yaml 2>/dev/null | sed -n "s/^[- ]*chart: ${FB_RELEASE}-//p" || true)"
    fi
    if [ -n "$FB_CHART_VERSION" ]; then
        # Read the toleration from the release's stored manifest, not the live DaemonSet: a
        # toleration patched onto the live object (as earlier versions of this script did)
        # passes a live check but is gone once Helm recreates the DaemonSet.
        FB_RELEASE_TOLERATIONS="$(helm get manifest "$FB_RELEASE" -n "$FB_NAMESPACE" --kube-context "$CLUSTER_CONTEXT" 2>/dev/null \
            | awk '/^---/{if(ds)printf "%s",buf; buf="";ds=0;next} {buf=buf $0 "\n"} /^kind: DaemonSet$/{ds=1} END{if(ds)printf "%s",buf}' \
            | kubectl create --context "$CLUSTER_CONTEXT" --dry-run=client -f - -o jsonpath="$FB_TOLERATIONS_JSONPATH" 2>/dev/null || true)"
    fi
    if [ -n "$FB_CHART_VERSION" ] && fb_tolerates_all "$FB_RELEASE_TOLERATIONS"; then
        echo "   ✅ fluent-bit.tolerations is already set on the ${FB_RELEASE} release"
    elif [ -n "$FB_CHART_VERSION" ] && helm upgrade "$FB_RELEASE" "$FB_CHART" \
            --kube-context "$CLUSTER_CONTEXT" \
            --namespace "$FB_NAMESPACE" \
            --version "$FB_CHART_VERSION" \
            --reuse-values \
            --set "fluent-bit.tolerations[0].operator=Exists" \
            --set "fluentBitCustomizations.clusterInstance=${CLUSTER_NAME}" >/dev/null; then
        echo "   ✅ Set fluent-bit.tolerations on the ${FB_RELEASE} release (chart ${FB_CHART_VERSION})"
    elif fb_tolerates_all "$(kubectl get daemonset fluent-bit -n "$FB_NAMESPACE" --context "$CLUSTER_CONTEXT" \
            -o jsonpath="$FB_TOLERATIONS_JSONPATH")"; then
        echo "   ⚠️  Fluent Bit tolerates the taint only through a patch on the DaemonSet, which is"
        echo "       lost if the DaemonSet is recreated. Make it durable with:"
        fb_print_helm_command
    # Fallback for a log module that is not a Helm release we can upgrade from here. The
    # patch works now but does not survive the DaemonSet being recreated.
    elif kubectl patch daemonset fluent-bit -n "$FB_NAMESPACE" --context "$CLUSTER_CONTEXT" --type=json \
            -p='[{"op":"add","path":"/spec/template/spec/tolerations","value":[{"operator":"Exists"}]}]' >/dev/null; then
        echo "   ⚠️  Could not set the toleration through Helm; patched the DaemonSet instead."
        echo "       The patch is lost if the DaemonSet is recreated. Make it durable with:"
        fb_print_helm_command
    else
        echo "   ❌ Could not set the Fluent Bit toleration through Helm or patch the DaemonSet."
        echo "       Agents on this node produce no runtime logs until it is set with:"
        fb_print_helm_command
    fi
fi

# --- Status + networking sanity check ---
echo ""
echo "✅ gVisor isolation node ready."
kubectl get --context "$CLUSTER_CONTEXT" node "${NODE_NAME}" -o wide
kubectl get --context "$CLUSTER_CONTEXT" runtimeclass "${GVISOR_RUNTIME_CLASS}"

echo ""
echo "🔍 Cross-node networking sanity check (runsc pod → cluster DNS)..."
# This catches the most common gVisor-on-overlay failure: the pod is Ready but
# cannot reach Services on other nodes (so traces/metrics/gateway silently fail).
# It runs a tiny runsc pod on the gVisor node and resolves the kubernetes Service.
SANITY_POD="gvisor-netcheck"
kubectl delete pod "$SANITY_POD" --context "$CLUSTER_CONTEXT" --ignore-not-found >/dev/null 2>&1 || true
cat <<EOF | kubectl apply --context "$CLUSTER_CONTEXT" -f - >/dev/null 2>&1 || true
apiVersion: v1
kind: Pod
metadata:
  name: ${SANITY_POD}
spec:
  runtimeClassName: ${GVISOR_RUNTIME_CLASS}
  restartPolicy: Never
  containers:
    - name: netcheck
      image: busybox:1.36
      command: ["sh","-c","nslookup kubernetes.default.svc.cluster.local && wget -qO- --timeout=5 https://kubernetes.default.svc.cluster.local --no-check-certificate >/dev/null 2>&1; echo NETCHECK_DONE"]
EOF
if kubectl wait --context "$CLUSTER_CONTEXT" --for=condition=Ready "pod/${SANITY_POD}" --timeout=60s >/dev/null 2>&1 \
    || kubectl wait --context "$CLUSTER_CONTEXT" --for=jsonpath='{.status.phase}'=Succeeded "pod/${SANITY_POD}" --timeout=60s >/dev/null 2>&1; then
    sleep 3
    if kubectl logs "$SANITY_POD" --context "$CLUSTER_CONTEXT" 2>/dev/null | grep -q "Address"; then
        echo "   ✅ gVisor pod resolved cluster DNS — cross-node networking looks healthy."
    else
        echo "   ⚠️  gVisor pod could NOT resolve cluster DNS — cross-node networking is broken."
        echo "      This is the usual cause of empty traces/metrics and 'try-it not deployed'."
        echo "      Re-run with host-network passthrough:  GVISOR_NETWORK_HOST=true make setup-gvisor"
        echo "      (see deployments/gvisor-isolation-tier.md → Troubleshooting)."
    fi
else
    echo "   ⚠️  Sanity pod did not become Ready; inspect:  kubectl describe pod ${SANITY_POD}"
fi
kubectl delete pod "$SANITY_POD" --context "$CLUSTER_CONTEXT" --ignore-not-found >/dev/null 2>&1 || true

echo ""
echo "Next steps:"
echo "  1. Create a gVisor environment (e.g. ISOLATION_TIER=gvisor ... bash ../scripts/add-environment.sh)."
echo "  2. Deploy or promote an agent to that environment."
echo "  3. Verify it landed on the gVisor node and runs under runsc:"
echo "       kubectl get pod <pod> -n <ns> -o wide            # NODE == ${NODE_NAME}"
echo "       kubectl get pod <pod> -n <ns> -o jsonpath='{.spec.runtimeClassName}'   # gvisor"
