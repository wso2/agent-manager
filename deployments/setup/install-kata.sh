#!/bin/bash
set -euo pipefail

# install-kata.sh — Install the Kata Containers isolation tier on a Linux Kubernetes cluster.
#
# Unlike gVisor (a single userspace `runsc` binary — see install-gvisor.sh), Kata boots each
# agent pod in a lightweight VM with its own guest kernel. The full stack (hypervisor + guest
# kernel + initrd + virtiofsd) is installed by upstream **kata-deploy**, which this script
# installs from kata-containers' Helm chart — run it from any machine with kubectl and helm
# access; you do NOT run this per-node like install-gvisor.sh.
#
# What it does (no downtime for existing runc agents):
#   1. Pre-flight: /dev/kvm must be available on the target node(s).
#   2. Label the target node(s) `kata=true` so the install + Kata pods land ONLY there.
#   3. Install the kata-deploy Helm chart at KATA_VERSION, with the image pinned to the same
#      version and the DaemonSet scoped to the labeled node(s). The chart wires containerd
#      itself on k8s, k3s and RKE2 (detected from the node's kubelet version).
#   4. Register the `kata-qemu` RuntimeClass (with scheduling) and boot a test pod on every
#      target node. The script only reports success once that pod has run in its own VM.
#   5. Taint the node(s) so only Kata pods schedule there (KATA_TAINT=false skips this).
#
# Usage (from a machine with kubectl pointed at the cluster):
#   KATA_NODES="node-a node-b" bash install-kata.sh
#   # or label the node(s) yourself first and run with no args (installs on every node
#   # already labeled kata=true).
#
# When this script runs on its own (downloaded with curl), it cannot see the repository's
# kata-runtimeclass.yaml, so it applies the RuntimeClass of the release it ships with. A
# copy taken from main has no release and stops before installing: set AMP_RELEASE_REF to
# the release you want (for example amp/v1.0.0), or AMP_MANIFEST_BASE_URL to a mirror of
# deployments/k8s.
#
# Requirements for each Kata node:
#   - /dev/kvm (bare metal, or a VM with nested virtualization; NOT E2/AMD-N2D/COS on GKE)
#   - Ubuntu/containerd (Container-Optimized OS is read-only and blocks the installer)
#   - x86_64 (Intel) strongly recommended; review upstream restrictions for arm64
#
# Idempotent: safe to re-run. Nodes set up by the earlier manifest-based version of this
# script are migrated to the Helm release on the first run.

echo "=== Installing Kata Containers (kata-qemu) isolation tier ==="

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Source shared vars when run from inside the repo; fall back to sane defaults when piped.
if [ -f "$SCRIPT_DIR/env.sh" ]; then source "$SCRIPT_DIR/env.sh"; fi
# The release this script ships with. The release workflow stamps the version it
# tags into this default, so a copy downloaded from a release tag knows its release.
AMP_RELEASE_REF="${AMP_RELEASE_REF:-amp/v2026.11.0}"
KATA_VERSION="${KATA_VERSION:-4.2.0}"
KATA_CHART="${KATA_CHART:-oci://ghcr.io/kata-containers/kata-deploy-charts/kata-deploy}"
KATA_HELM_RELEASE="${KATA_HELM_RELEASE:-kata-deploy}"
KATA_NAMESPACE="${KATA_NAMESPACE:-kube-system}"
KATA_RUNTIME_CLASS="${KATA_RUNTIME_CLASS:-kata-qemu}"
KATA_NODE_LABEL_KEY="${KATA_NODE_LABEL_KEY:-kata}"
KATA_NODE_LABEL_VALUE="${KATA_NODE_LABEL_VALUE:-true}"
KATA_NODES="${KATA_NODES:-}"   # space-separated node names; empty = use already-labeled nodes
KATA_TAINT="${KATA_TAINT:-true}"
KATA_SKIP_KVM_CHECK="${KATA_SKIP_KVM_CHECK:-false}"
KATA_SMOKE_TEST_IMAGE="${KATA_SMOKE_TEST_IMAGE:-busybox:1.36}"

for tool in kubectl helm; do
    if ! command -v "$tool" &>/dev/null; then
        echo "❌ ${tool} not found. Run this from a machine with kubectl and helm access to the cluster."
        exit 1
    fi
done

# kata-deploy's manifests under tools/packaging/kata-deploy/{kata-rbac,kata-deploy}/base/ were
# removed after 3.21.0, and every 3.x manifest from 3.4.0 on runs `kata-deploy:latest` with a
# command that image no longer has. The Helm chart is the supported path, and its values took
# their current shape in 4.0.0, so that is the oldest release this script installs.
KATA_MAJOR="${KATA_VERSION%%.*}"
if ! [[ "$KATA_MAJOR" =~ ^[0-9]+$ ]] || [ "$KATA_MAJOR" -lt 4 ]; then
    echo "❌ KATA_VERSION=${KATA_VERSION} is not supported. This installer needs kata-containers 4.0.0 or later."
    echo "   kata-deploy is installed from its Helm chart, whose values changed shape in 4.0.0."
    exit 1
fi
# Fetch the chart now: the migration below removes an older Kata before Helm runs, so a
# version Helm cannot resolve must stop the script before anything changes.
if ! helm show chart "$KATA_CHART" --version "$KATA_VERSION" >/dev/null; then
    echo "❌ Cannot fetch the kata-deploy chart ${KATA_VERSION} from ${KATA_CHART}."
    echo "   Check KATA_VERSION against https://github.com/kata-containers/kata-containers/releases"
    exit 1
fi

# Helm 4 applies charts server-side, which makes re-runs fail with field-ownership conflicts.
HELM_ARGS=()
if helm version --template '{{.Version}}' 2>/dev/null | grep -q '^v4'; then
    HELM_ARGS+=(--server-side=false)
fi

# Resolve the RuntimeClass manifest before touching the cluster, so a standalone run without
# a release ref fails here rather than after Kata is already installed.
RUNTIMECLASS_MANIFEST="$SCRIPT_DIR/../k8s/kata-runtimeclass.yaml"
if [ ! -f "$RUNTIMECLASS_MANIFEST" ]; then
    # A copy from main still carries the unstamped ref. The pattern avoids spelling out
    # the placeholder, because the release workflow rewrites every occurrence of it here.
    UNSTAMPED=false
    case "$AMP_RELEASE_REF" in
        amp/v0.0.0-de[v]) UNSTAMPED=true ;;
    esac
    if [ -z "${AMP_MANIFEST_BASE_URL:-}" ] && [ "$UNSTAMPED" = true ]; then
        echo "❌ Cannot find kata-runtimeclass.yaml next to this script."
        echo "   Set AMP_RELEASE_REF to the release you downloaded the script from, for example:"
        echo "     AMP_RELEASE_REF=amp/v1.0.0 KATA_NODES=\"<node>\" bash install-kata.sh"
        exit 1
    fi
    RUNTIMECLASS_URL="${AMP_MANIFEST_BASE_URL:-https://raw.githubusercontent.com/wso2/agent-manager/${AMP_RELEASE_REF}/deployments/k8s}/kata-runtimeclass.yaml"
    if [ -f "$RUNTIMECLASS_URL" ]; then
        # AMP_MANIFEST_BASE_URL can also name a local copy of deployments/k8s.
        RUNTIMECLASS_MANIFEST="$RUNTIMECLASS_URL"
    else
        # Download it now and apply the downloaded copy later, so an unreachable URL stops
        # the script before any node is labeled or an older Kata is removed.
        if ! command -v curl &>/dev/null; then
            echo "❌ curl not found. It downloads kata-runtimeclass.yaml when this script runs on its own."
            exit 1
        fi
        RUNTIMECLASS_MANIFEST="$(mktemp)"
        trap 'rm -f "$RUNTIMECLASS_MANIFEST"' EXIT
        # Print the URL without any user:password@ a mirror URL may carry.
        RUNTIMECLASS_URL_SHOWN="$(printf '%s' "$RUNTIMECLASS_URL" | sed -E 's#^([A-Za-z][A-Za-z0-9+.-]*://)[^/@]*@#\1#')"
        echo "📥 Downloading the RuntimeClass manifest from ${RUNTIMECLASS_URL_SHOWN}..."
        if ! curl -fsSL --retry 3 "$RUNTIMECLASS_URL" -o "$RUNTIMECLASS_MANIFEST"; then
            echo "❌ Cannot download kata-runtimeclass.yaml from ${RUNTIMECLASS_URL_SHOWN}."
            echo "   Set AMP_MANIFEST_BASE_URL to a reachable copy of deployments/k8s, or run the script"
            echo "   from a checkout of the release so it finds ../k8s/kata-runtimeclass.yaml."
            exit 1
        fi
    fi
fi

# --- 1. Label the target node(s) so kata-deploy + Kata pods land only there ---
if [ -n "$KATA_NODES" ]; then
    for n in $KATA_NODES; do
        echo "🏷️  Labeling node ${n} ${KATA_NODE_LABEL_KEY}=${KATA_NODE_LABEL_VALUE}..."
        kubectl label node "$n" "${KATA_NODE_LABEL_KEY}=${KATA_NODE_LABEL_VALUE}" --overwrite
    done
fi

LABELED=$(kubectl get nodes -l "${KATA_NODE_LABEL_KEY}=${KATA_NODE_LABEL_VALUE}" -o name 2>/dev/null || true)
if [ -z "$LABELED" ]; then
    echo "❌ No nodes are labeled ${KATA_NODE_LABEL_KEY}=${KATA_NODE_LABEL_VALUE}."
    echo "   Pass KATA_NODES=\"<node> ...\" or label the Kata node(s) first:"
    echo "     kubectl label node <node> ${KATA_NODE_LABEL_KEY}=${KATA_NODE_LABEL_VALUE} --overwrite"
    exit 1
fi
NODES=$(echo "$LABELED" | sed 's#node/##')
echo "   Target Kata node(s):"
echo "$NODES" | sed 's/^/     /'

# --- Which containerd the chart must wire: RKE2 and k3s ship their own ---
# RKE2 reports a "-k3s" containerd too, so the kubelet version is what tells them apart.
if [ -z "${KATA_K8S_DISTRIBUTION:-}" ]; then
    for node in $NODES; do
        kubelet=$(kubectl get node "$node" -o jsonpath='{.status.nodeInfo.kubeletVersion}')
        case "$kubelet" in
            *+rke2*) dist=rke2 ;;
            *+k3s*)  dist=k3s ;;
            *)       dist=k8s ;;
        esac
        if [ -n "${KATA_K8S_DISTRIBUTION:-}" ] && [ "$dist" != "$KATA_K8S_DISTRIBUTION" ]; then
            echo "❌ Target nodes run different distributions (${KATA_K8S_DISTRIBUTION} and ${dist})."
            echo "   Install Kata on one distribution at a time, or set KATA_K8S_DISTRIBUTION explicitly."
            exit 1
        fi
        KATA_K8S_DISTRIBUTION="$dist"
    done
fi
echo "   Kubernetes distribution: ${KATA_K8S_DISTRIBUTION}"

# --- Pre-flight: KVM is MANDATORY on each target node ---
# Kata-qemu boots a real VM via QEMU/KVM, so a node without /dev/kvm cannot run it. A tiny
# privileged pod checks for /dev/kvm on each node. A check that cannot finish is reported as
# such, not as missing KVM. Set KATA_SKIP_KVM_CHECK=true only if you verified KVM yourself.
if [ "$KATA_SKIP_KVM_CHECK" = "true" ]; then
    echo "⏭️  KATA_SKIP_KVM_CHECK=true — skipping the /dev/kvm pre-flight (you asserted KVM is present)."
else
    echo "🔬 Verifying /dev/kvm on the target node(s)..."
    KVM_MISSING=()
    for node in $NODES; do
        POD="kata-kvm-check-${node//[^a-z0-9-]/-}"
        POD="${POD:0:63}"
        kubectl delete pod "$POD" --ignore-not-found >/dev/null
        cat <<EOF | kubectl apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: ${POD}
spec:
  nodeName: ${node}
  restartPolicy: Never
  tolerations:
    - operator: Exists
  containers:
    - name: check
      image: busybox:1.36
      securityContext: { privileged: true }
      command: ["sh","-c","test -e /dev/kvm && echo KVM_OK || echo KVM_MISSING"]
      volumeMounts: [{ name: dev, mountPath: /dev }]
  volumes:
    - name: dev
      hostPath: { path: /dev }
EOF
        result=""
        for _ in $(seq 1 30); do
            phase=$(kubectl get pod "$POD" -o jsonpath='{.status.phase}' 2>/dev/null || true)
            if [ "$phase" = "Succeeded" ] || [ "$phase" = "Failed" ]; then
                result=$(kubectl logs "$POD" 2>/dev/null || true)
                break
            fi
            sleep 3
        done
        case "$result" in
            *KVM_OK*)
                echo "   ✅ ${node}: /dev/kvm present" ;;
            *KVM_MISSING*)
                echo "   ❌ ${node}: /dev/kvm NOT available"
                KVM_MISSING+=("$node") ;;
            *)
                echo "   ❌ ${node}: the KVM check did not finish (pod phase: ${phase:-unknown}). This is not a KVM result."
                kubectl get events --field-selector "involvedObject.name=${POD}" 2>/dev/null | tail -5 | sed 's/^/      /'
                kubectl delete pod "$POD" --ignore-not-found >/dev/null
                echo "   Re-run the script, or set KATA_SKIP_KVM_CHECK=true if you verified /dev/kvm yourself."
                exit 1 ;;
        esac
        kubectl delete pod "$POD" --ignore-not-found >/dev/null
    done

    if [ "${#KVM_MISSING[@]}" -gt 0 ]; then
        echo ""
        echo "❌ Kata cannot be installed: ${KVM_MISSING[*]} expose no /dev/kvm."
        echo "   Kata-qemu requires hardware virtualization (VT-x/AMD-V) exposed as /dev/kvm. Use:"
        echo "     - a bare-metal node (e.g. an EKS *.metal instance), or"
        echo "     - a nested-virt cloud VM (GCP Intel N2 + --enable-nested-virtualization, Ubuntu/containerd)."
        exit 1
    fi
fi

# --- Migrate a node set up by the manifest-based installer ---
# That version applied a kata-deploy DaemonSet and RBAC under the same names the chart uses,
# so Helm would refuse to adopt them. Deleting the DaemonSet runs its cleanup hook, which
# removes the old Kata stack from the node before the chart installs the new one. The hook
# needs the old RBAC, so the pods must be gone before the RBAC is deleted or Helm runs.
# The old manifests always deployed to kube-system, whatever KATA_NAMESPACE is now.
LEGACY_NAMESPACE="kube-system"
if kubectl -n "$LEGACY_NAMESPACE" get daemonset kata-deploy &>/dev/null \
    && [ -z "$(kubectl -n "$LEGACY_NAMESPACE" get daemonset kata-deploy -o jsonpath='{.metadata.annotations.meta\.helm\.sh/release-name}')" ]; then
    echo "🔁 Removing the manifest-based kata-deploy installed by an earlier version of this script..."
    kubectl -n "$LEGACY_NAMESPACE" delete daemonset kata-deploy --cascade=foreground --wait=true --timeout=10m
    for _ in $(seq 1 200); do
        [ -z "$(kubectl -n "$LEGACY_NAMESPACE" get pods -l name=kata-deploy -o name 2>/dev/null)" ] && break
        sleep 3
    done
    if [ -n "$(kubectl -n "$LEGACY_NAMESPACE" get pods -l name=kata-deploy -o name 2>/dev/null)" ]; then
        echo "❌ The old kata-deploy pods did not finish their cleanup. Check them before re-running:"
        echo "   kubectl -n ${LEGACY_NAMESPACE} get pods -l name=kata-deploy"
        exit 1
    fi
    kubectl -n "$LEGACY_NAMESPACE" delete serviceaccount kata-deploy-sa --ignore-not-found
    kubectl delete clusterrolebinding kata-deploy-rb --ignore-not-found
    kubectl delete clusterrole kata-deploy-role --ignore-not-found
fi

# --- 2. Install kata-deploy from its Helm chart, scoped to the labeled node(s) ---
# Only the qemu shim is installed: kata-qemu is the handler Agent Manager's RuntimeClass names.
# The chart's own RuntimeClasses are off because ours carries the scheduling stanza agents
# rely on, and no extra snapshotter is set up.
# Helm's --set reads every dot in a key as nesting, so a domain-prefixed label key such as
# example.com/kata has its dots escaped.
KATA_NODE_LABEL_KEY_HELM="${KATA_NODE_LABEL_KEY//./\\.}"
echo "📦 Installing kata-deploy ${KATA_VERSION} (Helm release ${KATA_HELM_RELEASE} in ${KATA_NAMESPACE})..."
helm upgrade --install "$KATA_HELM_RELEASE" "$KATA_CHART" \
    --version "$KATA_VERSION" \
    --namespace "$KATA_NAMESPACE" \
    --set image.tag="$KATA_VERSION" \
    --set k8sDistribution="$KATA_K8S_DISTRIBUTION" \
    --set-string "nodeSelector.${KATA_NODE_LABEL_KEY_HELM}=${KATA_NODE_LABEL_VALUE}" \
    --set "tolerations[0].key=${KATA_NODE_LABEL_KEY}" \
    --set "tolerations[0].operator=Equal" \
    --set-string "tolerations[0].value=${KATA_NODE_LABEL_VALUE}" \
    --set "tolerations[0].effect=NoSchedule" \
    --set shims.disableAll=true \
    --set shims.qemu.enabled=true \
    --set defaultShim.amd64=qemu \
    --set defaultShim.arm64=qemu \
    --set runtimeClasses.enabled=false \
    --set 'snapshotter.setup={}' \
    --wait --timeout 15m \
    ${HELM_ARGS[@]+"${HELM_ARGS[@]}"}

echo "⏳ Waiting for kata-deploy to finish on the target node(s)..."
for node in $NODES; do
    if ! kubectl wait --for=jsonpath='{.metadata.labels.katacontainers\.io/kata-runtime}'=true \
        "node/${node}" --timeout=10m >/dev/null; then
        echo "❌ ${node}: kata-deploy did not report the Kata runtime as installed."
        echo "   kubectl -n ${KATA_NAMESPACE} logs -l name=kata-deploy --tail=50"
        exit 1
    fi
    kubectl wait --for=condition=Ready "node/${node}" --timeout=5m >/dev/null
    echo "   ✅ ${node}: Kata runtime installed"
done

# --- 3. Register the RuntimeClass (with scheduling) ---
echo "🧩 Registering the '${KATA_RUNTIME_CLASS}' RuntimeClass..."
kubectl apply -f "$RUNTIMECLASS_MANIFEST"
# The manifest schedules onto kata=true. Point it at the label pair this run used, or Kata
# pods could not reach the nodes labeled above.
if [ "$KATA_NODE_LABEL_KEY" != "kata" ] || [ "$KATA_NODE_LABEL_VALUE" != "true" ]; then
    kubectl patch runtimeclass "$KATA_RUNTIME_CLASS" --type=json -p="[
      {\"op\":\"replace\",\"path\":\"/scheduling/nodeSelector\",\"value\":{\"${KATA_NODE_LABEL_KEY}\":\"${KATA_NODE_LABEL_VALUE}\"}},
      {\"op\":\"replace\",\"path\":\"/scheduling/tolerations\",\"value\":[{\"key\":\"${KATA_NODE_LABEL_KEY}\",\"operator\":\"Equal\",\"value\":\"${KATA_NODE_LABEL_VALUE}\",\"effect\":\"NoSchedule\"}]}
    ]"
fi

# --- 4. Prove it: boot a Kata pod on every target node ---
# A pod that runs under the RuntimeClass and reports a kernel other than the node's own is
# the only evidence that containerd has the handler and the VM boots. The pod goes through
# the scheduler with only the RuntimeClass's nodeSelector and tolerations, as an agent does,
# so a node agents cannot reach fails here too.
echo "🧪 Booting a test pod under '${KATA_RUNTIME_CLASS}' on each target node..."
for node in $NODES; do
    POD="kata-smoke-test-${node//[^a-z0-9-]/-}"
    POD="${POD:0:63}"
    kubectl delete pod "$POD" --ignore-not-found >/dev/null
    cat <<EOF | kubectl apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: ${POD}
spec:
  runtimeClassName: ${KATA_RUNTIME_CLASS}
  restartPolicy: Never
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchFields:
              - { key: metadata.name, operator: In, values: ["${node}"] }
  containers:
    - name: test
      image: ${KATA_SMOKE_TEST_IMAGE}
      command: ["uname", "-r"]
EOF
    phase=""
    for _ in $(seq 1 60); do
        phase=$(kubectl get pod "$POD" -o jsonpath='{.status.phase}' 2>/dev/null || true)
        if [ "$phase" = "Succeeded" ] || [ "$phase" = "Failed" ]; then break; fi
        sleep 3
    done
    guest_kernel=$(kubectl logs "$POD" 2>/dev/null || true)
    host_kernel=$(kubectl get node "$node" -o jsonpath='{.status.nodeInfo.kernelVersion}')
    if [ "$phase" != "Succeeded" ] || [ -z "$guest_kernel" ] || [ "$guest_kernel" = "$host_kernel" ]; then
        echo "❌ ${node}: a pod under '${KATA_RUNTIME_CLASS}' did not boot in its own VM (phase: ${phase:-unknown})."
        kubectl get events --field-selector "involvedObject.name=${POD}" 2>/dev/null | tail -5 | sed 's/^/      /'
        kubectl delete pod "$POD" --ignore-not-found >/dev/null
        exit 1
    fi
    kubectl delete pod "$POD" --ignore-not-found >/dev/null
    echo "   ✅ ${node}: guest kernel ${guest_kernel} (node runs ${host_kernel})"
done

# --- 5. Taint the node(s) so only Kata pods schedule there ---
if [ "$KATA_TAINT" = "true" ]; then
    echo "🏷️  Tainting the Kata node(s) so only Kata pods schedule there..."
    for node in $NODES; do
        kubectl taint node "$node" "${KATA_NODE_LABEL_KEY}=${KATA_NODE_LABEL_VALUE}:NoSchedule" --overwrite
    done

    # Fluent Bit (log DaemonSet) must tolerate the taint so agent logs are collected.
    # The DaemonSet is rendered by the Helm-owned observability-logs-opensearch release, so
    # the toleration is set as the chart value fluent-bit.tolerations (what
    # setup-openchoreo.sh installs with), not patched onto the object: a patch is lost
    # whenever the DaemonSet is recreated (reinstall, namespace teardown), and agent logs on
    # the Kata node then stop with nothing naming the cause. The upgrade pins the release's
    # own installed chart version, because --reuse-values across a chart version change can
    # render values the new chart does not expect.
    FB_NAMESPACE="openchoreo-observability-plane"
    FB_RELEASE="observability-logs-opensearch"
    FB_CHART="oci://ghcr.io/openchoreo/helm-charts/observability-logs-opensearch"
    # Tolerations as operator/key/effect lines. `Exists//` tolerates every taint and
    # `Exists//NoSchedule` every NoSchedule taint, which covers the Kata taint. The value set
    # below is `Exists//`, so it may replace other entries without losing any node.
    FB_TOLERATIONS_JSONPATH='{range .spec.template.spec.tolerations[*]}{.operator}/{.key}/{.effect}{"\n"}{end}'
    fb_tolerates_all() { grep -qxE 'Exists//(NoSchedule)?' <<<"$1"; }
    fb_print_helm_command() {
        echo "         helm upgrade ${FB_RELEASE} ${FB_CHART} \\"
        echo "           --namespace ${FB_NAMESPACE} \\"
        echo "           --version <installed chart version, from: helm list -n ${FB_NAMESPACE}> \\"
        echo "           --reuse-values \\"
        echo "           --set \"fluent-bit.tolerations[0].operator=Exists\""
    }
    if kubectl get daemonset fluent-bit -n "$FB_NAMESPACE" &>/dev/null; then
        echo "🪵 Ensuring Fluent Bit tolerates the Kata taint (so agent logs are collected)..."
        # `|| true`: under pipefail a failing helm would otherwise abort the script here
        # instead of reaching the patch fallback below.
        FB_CHART_VERSION="$(helm list -n "$FB_NAMESPACE" --filter "^${FB_RELEASE}\$" -o yaml 2>/dev/null \
            | sed -n "s/^[- ]*chart: ${FB_RELEASE}-//p" || true)"
        FB_RELEASE_TOLERATIONS=""
        if [ -n "$FB_CHART_VERSION" ]; then
            # Read the toleration from the release's stored manifest, not the live DaemonSet: a
            # toleration patched onto the live object (as earlier versions of this script did)
            # passes a live check but is gone once Helm recreates the DaemonSet.
            FB_RELEASE_TOLERATIONS="$(helm get manifest "$FB_RELEASE" -n "$FB_NAMESPACE" 2>/dev/null \
                | awk '/^---/{if(ds)printf "%s",buf; buf="";ds=0;next} {buf=buf $0 "\n"} /^kind: DaemonSet$/{ds=1} END{if(ds)printf "%s",buf}' \
                | kubectl create --dry-run=client -f - -o jsonpath="$FB_TOLERATIONS_JSONPATH" 2>/dev/null || true)"
        fi
        if [ -n "$FB_CHART_VERSION" ] && fb_tolerates_all "$FB_RELEASE_TOLERATIONS"; then
            echo "   ✅ fluent-bit.tolerations is already set on the ${FB_RELEASE} release"
        elif [ -n "$FB_CHART_VERSION" ] && helm upgrade "$FB_RELEASE" "$FB_CHART" \
                --namespace "$FB_NAMESPACE" \
                --version "$FB_CHART_VERSION" \
                --reuse-values \
                --set "fluent-bit.tolerations[0].operator=Exists" \
                ${HELM_ARGS[@]+"${HELM_ARGS[@]}"} >/dev/null; then
            echo "   ✅ Set fluent-bit.tolerations on the ${FB_RELEASE} release (chart ${FB_CHART_VERSION})"
        elif fb_tolerates_all "$(kubectl get daemonset fluent-bit -n "$FB_NAMESPACE" -o jsonpath="$FB_TOLERATIONS_JSONPATH")"; then
            echo "   ⚠️  Fluent Bit tolerates the taint only through a patch on the DaemonSet, which is"
            echo "       lost if the DaemonSet is recreated. Make it durable with:"
            fb_print_helm_command
        # Fallback for a log module that is not a Helm release we can upgrade from here. The
        # patch works now but does not survive the DaemonSet being recreated.
        elif kubectl patch daemonset fluent-bit -n "$FB_NAMESPACE" --type=json \
                -p='[{"op":"add","path":"/spec/template/spec/tolerations","value":[{"operator":"Exists"}]}]' >/dev/null; then
            echo "   ⚠️  Could not set the toleration through Helm; patched the DaemonSet instead."
            echo "       The patch is lost if the DaemonSet is recreated. Make it durable with:"
            fb_print_helm_command
        else
            echo "   ❌ Could not set the Fluent Bit toleration through Helm or patch the DaemonSet."
            echo "       Agents on the Kata node(s) produce no runtime logs until it is set with:"
            fb_print_helm_command
        fi
    fi
else
    echo "⏭️  KATA_TAINT=false — leaving the Kata node(s) schedulable for other pods."
fi

echo ""
echo "✅ Kata isolation tier installed and verified."
echo ""
echo "Next, create a Kata environment and deploy/promote an agent to it:"
echo "  ISOLATION_TIER=kata ENV_NAME=kata-dev DISPLAY_NAME=\"Kata Dev\" \\"
echo "    AGENT_MANAGER_TOKEN=<token> bash deployments/scripts/add-environment.sh"
