#!/bin/bash
set -euo pipefail

# Resolved the same way as install-kata.sh: the RuntimeClass manifest is applied
# from the sibling k8s/ directory when this script travels with it.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# The release this script ships with. The release workflow stamps the version it
# tags into this default, so a copy downloaded from a release tag knows its release.
AMP_RELEASE_REF="${AMP_RELEASE_REF:-amp/v2026.11.0}"

# install-gvisor.sh — Install gVisor (runsc) on a Linux Kubernetes node.
#
# Run this script directly on each node you want to use for gVisor agents.
# After running on each node, apply the RuntimeClass and register the node
# from any machine that has kubectl access to the cluster (see docs).
#
# Usage:
#   sudo bash install-gvisor.sh
#
# Or pipe directly from the release you are installing (replace <release-tag>, for
# example amp/v1.0.0):
#   curl -fsSL https://raw.githubusercontent.com/wso2/agent-manager/<release-tag>/deployments/setup/install-gvisor.sh \
#     | sudo bash
#
# When this script runs on its own (downloaded with curl), it cannot see the repository's
# gvisor-runtimeclass.yaml, so its printed next steps link to the RuntimeClass of the
# release it ships with. A copy taken from main has no release: set AMP_RELEASE_REF to
# the release you want (for example amp/v1.0.0), or AMP_MANIFEST_BASE_URL to a mirror
# of deployments/k8s:
#   sudo AMP_RELEASE_REF=amp/v1.0.0 bash install-gvisor.sh
#
# Optional: host-network passthrough for runsc (keeps syscall isolation but uses
# the host network stack inside the pod netns). Use only if the default userspace
# netstack cannot carry cross-node Service traffic on your CNI/overlay:
#   GVISOR_NETWORK_HOST=true sudo -E bash install-gvisor.sh
#
# Prerequisites:
#   - Ubuntu 20.04+, Debian 11+, RHEL 8+, Amazon Linux 2023, or any Linux with
#     containerd managed by systemd
#   - x86_64 or aarch64 (arm64) architecture
#   - containerd installed.
#   - Internet access to storage.googleapis.com
#   - zstd or bzip2 (gVisor releases ship as a compressed tarball)
#
# Idempotent: safe to re-run. Already-installed components are skipped.

echo "=== Installing gVisor (runsc) isolation tier ==="

GVISOR_NETWORK_HOST="${GVISOR_NETWORK_HOST:-false}"

# Must run as root
if [ "$(id -u)" != "0" ]; then
    echo "❌ This script must be run as root. Use: sudo bash install-gvisor.sh"
    exit 1
fi

# --- Detect architecture ---
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)        GVISOR_ARCH="x86_64" ;;
    aarch64|arm64) GVISOR_ARCH="aarch64" ;;
    *)
        echo "❌ Unsupported architecture: ${ARCH}"
        echo "   gVisor supports x86_64 and aarch64 only."
        exit 1
        ;;
esac
echo "   Architecture: ${ARCH}"

# --- Check containerd is present and running ---
if ! command -v containerd &>/dev/null; then
    echo "❌ containerd not found. Install containerd before running this script."
    echo "   See: https://docs.docker.com/engine/install/"
    exit 1
fi

# containerd may not have been started yet
if systemctl is-active containerd &>/dev/null; then
    CONTAINERD_RUNNING=true
else
    CONTAINERD_RUNNING=false
    echo "   containerd is installed but not running yet — configuring it for its first start"
fi

CONTAINERD_VERSION="$(containerd --version | grep -oE '[0-9]+\.[0-9]+' | head -1)"
echo "   containerd: v${CONTAINERD_VERSION}"

# --- Install runsc ---

INSTALL_DIR="/usr/local/bin"

runsc_installed() {
    command -v runsc &>/dev/null && runsc --version &>/dev/null || return 1
    if runsc flags 2>&1 | grep -- "-sidecar-usage-policy" >/dev/null; then
        [ -x "$(dirname "$(command -v runsc)")/gvisor-bin/gvisor_sentry" ] || return 1
    fi
}

if runsc_installed; then
    echo "✅ runsc already installed ($(runsc --version 2>&1 | head -1)) — skipping download"
else
    # extract runsc, containerd-shim-runsc-v1 and gvisor-bin/ from the tarball
    if command -v zstd &>/dev/null; then
        TARBALL="gvisor.tar.zstd"; DECOMPRESS=(zstd -dc)
    elif command -v bzip2 &>/dev/null; then
        TARBALL="gvisor.tar.bz2"; DECOMPRESS=(bzip2 -dc)
    else
        echo "❌ zstd or bzip2 is required to unpack the gVisor release. Install one and re-run."
        exit 1
    fi

    echo "📥 Downloading gVisor release (${GVISOR_ARCH})..."
    BASE="https://storage.googleapis.com/gvisor/releases/release/latest/${GVISOR_ARCH}"
    # Download into a private temp dir (not predictable /tmp paths) and remove it on exit.
    # The tarball is ~130 MB (~330 MB unpacked) and /tmp may be RAM-backed.
    GVISOR_TMP="$(mktemp -d /var/tmp/gvisor.XXXXXX)"
    trap 'rm -rf "${GVISOR_TMP}"' EXIT
    curl -fsSL --retry 3 "${BASE}/${TARBALL}" -o "${GVISOR_TMP}/${TARBALL}"
    # gVisor publishes a .sha512 alongside the tarball; verify before trusting it.
    curl -fsSL --retry 3 "${BASE}/${TARBALL}.sha512" -o "${GVISOR_TMP}/${TARBALL}.sha512"
    ( cd "${GVISOR_TMP}" && sha512sum -c "${TARBALL}.sha512" ) || {
        echo "❌ gVisor release checksum verification failed — aborting."
        exit 1
    }
    "${DECOMPRESS[@]}" "${GVISOR_TMP}/${TARBALL}" \
        | tar -xf - -C "${GVISOR_TMP}" runsc containerd-shim-runsc-v1 gvisor-bin
    rm -f "${GVISOR_TMP}/${TARBALL}"

    rm -rf "${INSTALL_DIR}/gvisor-bin.new"
    mv "${GVISOR_TMP}/gvisor-bin" "${INSTALL_DIR}/gvisor-bin.new"
    rm -rf "${INSTALL_DIR}/gvisor-bin"
    mv "${INSTALL_DIR}/gvisor-bin.new" "${INSTALL_DIR}/gvisor-bin"
    chmod +x "${GVISOR_TMP}/runsc" "${GVISOR_TMP}/containerd-shim-runsc-v1"
    mv "${GVISOR_TMP}/runsc" "${INSTALL_DIR}/runsc"
    mv "${GVISOR_TMP}/containerd-shim-runsc-v1" "${INSTALL_DIR}/containerd-shim-runsc-v1"
    echo "   ✅ runsc $(runsc --version 2>&1 | head -1) installed, with sidecars in ${INSTALL_DIR}/gvisor-bin/"
fi

# --- Configure containerd ---
CONTAINERD_CONFIG="/etc/containerd/config.toml"

if [ ! -f "$CONTAINERD_CONFIG" ]; then
    mkdir -p "$(dirname "$CONTAINERD_CONFIG")"
    containerd config default > "$CONTAINERD_CONFIG"
    echo "   ✅ Generated default containerd config"
fi

# The CRI runtimes table has a different plugin ID per config version, and
# containerd silently ignores the wrong one (it only logs "Ignoring unknown key"):
#   version = 3 (containerd 2.x):  plugins."io.containerd.cri.v1.runtime"
#   version = 2 (containerd 1.x, or 2.x migrating a v2 file):  plugins."io.containerd.grpc.v1.cri"
CONFIG_VERSION="$(awk -F= '/^version[[:space:]]*=/ { gsub(/[[:space:]"]/, "", $2); print $2; exit }' "$CONTAINERD_CONFIG")"
case "$CONFIG_VERSION" in
    3) CRI_PLUGIN="io.containerd.cri.v1.runtime"; OTHER_CRI_PLUGIN="io.containerd.grpc.v1.cri" ;;
    2) CRI_PLUGIN="io.containerd.grpc.v1.cri";    OTHER_CRI_PLUGIN="io.containerd.cri.v1.runtime" ;;
    *)
        echo "❌ Unexpected containerd config version '${CONFIG_VERSION:-<none>}' in ${CONTAINERD_CONFIG}."
        echo "   Expected 'version = 3' (containerd 2.x) or 'version = 2' (containerd 1.x)."
        echo "   Not adding a runtime block: under the wrong plugin table containerd ignores it."
        exit 1
        ;;
esac
echo "   Config:     ${CONTAINERD_CONFIG} (version ${CONFIG_VERSION}, plugin ${CRI_PLUGIN})"

# True if the config defines the runsc runtime under the given plugin table.
has_runsc_table() {
    grep -F -e "[plugins.\"$1\".containerd.runtimes.runsc]" \
            -e "[plugins.'$1'.containerd.runtimes.runsc]" "$CONTAINERD_CONFIG" >/dev/null 2>&1
}

# Only a block under the table this config version reads counts as configured.
# A block under the other table (e.g. left by an earlier version of this script)
# is dead config; skipping on it would leave an already-broken node broken.
if has_runsc_table "$CRI_PLUGIN"; then
    echo "✅ containerd already configured for runsc — skipping"
else
    if has_runsc_table "$OTHER_CRI_PLUGIN"; then
        echo "   ⚠️  ${CONTAINERD_CONFIG} defines runsc under plugins.\"${OTHER_CRI_PLUGIN}\","
        echo "       which containerd ignores at config version ${CONFIG_VERSION}. Leaving it in place"
        echo "       and adding the runtime under plugins.\"${CRI_PLUGIN}\"; remove the stale block by hand."
    fi
    echo "⚙️  Adding runsc runtime to containerd config (${CONTAINERD_CONFIG})..."

    if [ "$GVISOR_NETWORK_HOST" = "true" ]; then
        cat >> "$CONTAINERD_CONFIG" <<BLOCK

# gVisor (runsc) runtime — added by install-gvisor.sh (host-network passthrough)
[plugins."${CRI_PLUGIN}".containerd.runtimes.runsc]
  runtime_type = "io.containerd.runsc.v1"
  [plugins."${CRI_PLUGIN}".containerd.runtimes.runsc.options]
    TypeUrl = "io.containerd.runsc.v1.options"
    ConfigPath = "/etc/containerd/runsc.toml"
BLOCK
        printf '[runsc_config]\n  network = "host"\n' > /etc/containerd/runsc.toml
        echo "   ✅ containerd config updated (runsc --network=host)"
    else
        cat >> "$CONTAINERD_CONFIG" <<BLOCK

# gVisor (runsc) runtime — added by install-gvisor.sh
[plugins."${CRI_PLUGIN}".containerd.runtimes.runsc]
  runtime_type = "io.containerd.runsc.v1"
BLOCK
        echo "   ✅ containerd config updated"
    fi
fi

# --- Restart containerd ---
# Skipped when containerd has not started yet:
if [ "$CONTAINERD_RUNNING" = "true" ]; then
    # Only containerd restarts — kubelet and running pods are unaffected.
    echo "🔄 Restarting containerd to load the new runtime..."
    systemctl restart containerd
fi

# A runtime block containerd ignores still leaves the node Ready, so this is the
# only check that catches it. crictl cannot reach a containerd that has not started.
if [ "$CONTAINERD_RUNNING" != "true" ]; then
    echo "   ℹ️  containerd is not running yet — skipped the restart and the registration check."
    echo "       runsc loads when containerd starts. Confirm then with:"
    echo "       crictl --runtime-endpoint unix:///run/containerd/containerd.sock info | grep runsc"
elif command -v crictl &>/dev/null; then
    CRICTL=(crictl --runtime-endpoint unix:///run/containerd/containerd.sock)
    REGISTERED=false
    for _ in $(seq 1 30); do
        # grep without -q: an early exit would SIGPIPE crictl and fail the pipe under pipefail.
        if "${CRICTL[@]}" info 2>/dev/null | grep "runsc" >/dev/null; then
            REGISTERED=true
            break
        fi
        sleep 2
    done
    if [ "$REGISTERED" != "true" ]; then
        echo "❌ containerd did not register the runsc runtime after restarting."
        echo "   gVisor pods on this node would fail with: no runtime for \"runsc\" is configured"
        IGNORED="$(journalctl -u containerd --since "-10min" --no-pager 2>/dev/null \
            | grep "unknown key" | grep "runsc" | tail -3 || true)"
        if [ -n "$IGNORED" ]; then
            echo "   containerd ignored the runtime block:"
            printf '%s\n' "$IGNORED" | sed 's/^/     /'
        fi
        echo "   Inspect: ${CRICTL[*]} info | grep -A3 runsc"
        echo "            grep -B1 -A3 runsc ${CONTAINERD_CONFIG}"
        exit 1
    fi
    echo "   ✅ runsc runtime registered in containerd"
else
    echo "   ⚠️  crictl not found — could not confirm containerd registered runsc."
    echo "       Confirm by running a pod that sets runtimeClassName: gvisor on this node."
fi

echo ""
echo "✅ gVisor installed on this node."
echo ""
echo "Next steps — from a machine with kubectl access to your cluster:"
echo ""
echo "  1. Apply the RuntimeClass (once per cluster):"
if [ -f "$SCRIPT_DIR/../k8s/gvisor-runtimeclass.yaml" ]; then
    # Point at the copy already on disk: telling the reader to curl a different
    # one invites a version mismatch with the install they just ran.
    echo "     kubectl apply -f \"$SCRIPT_DIR/../k8s/gvisor-runtimeclass.yaml\""
else
    # A copy from main still carries the unstamped ref: print a placeholder rather
    # than main. The pattern avoids spelling out the placeholder, because the
    # release workflow rewrites every occurrence of it in this file.
    case "$AMP_RELEASE_REF" in
        amp/v0.0.0-de[v]) RELEASE_REF="<release-tag>" ;;
        *)                RELEASE_REF="$AMP_RELEASE_REF" ;;
    esac
    echo "     kubectl apply -f ${AMP_MANIFEST_BASE_URL:-https://raw.githubusercontent.com/wso2/agent-manager/${RELEASE_REF}/deployments/k8s}/gvisor-runtimeclass.yaml"
    if [ -z "${AMP_MANIFEST_BASE_URL:-}" ] && [ "$RELEASE_REF" = "<release-tag>" ]; then
        echo "     Replace <release-tag> with the release you downloaded this script from (for example"
        echo "     amp/v1.0.0), or re-run with AMP_RELEASE_REF set to have it filled in."
    fi
fi
echo ""
echo "  2. Label and taint this node (replace <node-name> with: kubectl get nodes):"
echo "     kubectl label node <node-name> gvisor=true --overwrite"
echo "     kubectl taint node <node-name> gvisor=true:NoSchedule --overwrite"
echo ""
echo "  3. Ensure the Fluent Bit log DaemonSet tolerates the taint (so agent logs are collected)."
echo "     Check the logs module's Helm values, not the live DaemonSet — a toleration patched"
echo "     onto the DaemonSet is lost when it is recreated:"
echo "     helm get values observability-logs-opensearch -n openchoreo-observability-plane"
echo "     If fluent-bit.tolerations does not list '- operator: Exists' (with no key or"
echo "     effect), set it (keep the release's installed chart version, shown by:"
echo "     helm list -n openchoreo-observability-plane):"
echo "     helm upgrade observability-logs-opensearch \\"
echo "       oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch \\"
echo "       --namespace openchoreo-observability-plane \\"
echo "       --version <installed chart version> \\"
echo "       --reuse-values \\"
echo "       --set \"fluent-bit.tolerations[0].operator=Exists\""
echo "     (Only if the logs module is not a Helm release: patch the DaemonSet instead, knowing"
echo "     the patch is lost whenever the DaemonSet is recreated:"
echo "     kubectl patch daemonset fluent-bit -n openchoreo-observability-plane --type=json \\"
echo "       -p='[{\"op\":\"add\",\"path\":\"/spec/template/spec/tolerations\",\"value\":[{\"operator\":\"Exists\"}]}]')"
echo ""
echo "  4. Verify:"
echo "     kubectl get runtimeclass gvisor"
echo "     kubectl get node <node-name> --show-labels"
