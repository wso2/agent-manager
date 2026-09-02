#!/usr/bin/env bash
# Render assertions for the build-workflow egress NetworkPolicy.
# Run: bash deployments/helm-charts/wso2-amp-platform-resources-extension/tests/build-egress-render.sh
#
# The policy is matched post-DNAT, and is inert without the pod labels its podSelector
# targets — so both are asserted here. Kept separate from render.sh, which covers the
# shell quoting of the build steps.
set -uo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FAILURES=0

render() {
  local out
  if ! out="$(helm template test-release "$CHART_DIR" "$@" 2>&1)"; then
    printf 'helm template failed: %s\n' "$out" >&2
    return 1
  fi
  printf '%s' "$out"
}

# probe <python-expr-file> [helm --set args...]
probe() {
  local script="$1"; shift
  local rendered
  rendered="$(render "$@")" || return 1
  printf '%s' "$rendered" | python3 -c "$script"
}

assert() {
  local label="$1" expected="$2" script="$3"
  shift 3
  local actual
  # Without this, a render failure returns empty and reads as a passing "no rule" case.
  if ! actual="$(probe "$script" "$@")"; then
    printf 'FAIL - %s\n      render failed\n' "$label"
    FAILURES=$((FAILURES + 1))
    return
  fi
  if [[ "$expected" == "$actual" ]]; then
    printf 'ok   - %s\n' "$label"
  else
    printf 'FAIL - %s\n      expected: %q\n      actual:   %q\n' "$label" "$expected" "$actual"
    FAILURES=$((FAILURES + 1))
  fi
}

# --- the policy itself ---------------------------------------------------------------

READ_NETPOL='
import sys, yaml
policy = None
for doc in yaml.safe_load_all(sys.stdin):
    if doc and doc.get("kind") == "NetworkPolicy" and doc["metadata"]["name"] == "amp-build-workflow-egress":
        policy = doc
if policy is None:
    print("ABSENT")
    raise SystemExit
'

INTERNET_RULE=$READ_NETPOL'
for rule in policy["spec"]["egress"]:
    blocks = [t["ipBlock"] for t in rule.get("to") or [] if "ipBlock" in t]
    if any(b["cidr"] == "0.0.0.0/0" for b in blocks):
        # Reported, never skipped: a ports key here would break a non-standard-port mirror.
        ports = ",".join(str(p["port"]) for p in rule.get("ports") or []) or "ALL"
        print(ports + "|" + ",".join(blocks[0].get("except") or []))
'

# cidrs of the ipBlock rule carrying <port>
IPBLOCK_RULE=$READ_NETPOL'
import os
want = os.environ["WANT_PORT"]
for rule in policy["spec"]["egress"]:
    ports = [str(p["port"]) for p in rule.get("ports") or []]
    if want in ports:
        blocks = [t["ipBlock"]["cidr"] for t in rule.get("to") or [] if "ipBlock" in t]
        if blocks:
            print(",".join(blocks) + "|" + ",".join(ports))
'

NS_RULE=$READ_NETPOL'
import os
want = os.environ["WANT_NS_PORT"]
for rule in policy["spec"]["egress"]:
    ports = [str(p["port"]) for p in rule.get("ports") or []]
    if want not in ports:
        continue
    for t in rule.get("to") or []:
        ns = (t.get("namespaceSelector") or {}).get("matchLabels", {})
        if ns and not t.get("podSelector"):
            print(ns["kubernetes.io/metadata.name"] + "|" + ",".join(ports))
'

# Every rule carrying port 5000, so an unset in-cluster namespace can be shown to render none.
NS_RULE_PODS=$READ_NETPOL'
import os
want = os.environ["WANT_NS_PORT"]
for rule in policy["spec"]["egress"]:
    ports = [str(p["port"]) for p in rule.get("ports") or []]
    if want not in ports:
        continue
    for t in rule.get("to") or []:
        ns = (t.get("namespaceSelector") or {}).get("matchLabels", {})
        if not ns:
            continue
        pod = (t.get("podSelector") or {}).get("matchLabels", {})
        print(ns["kubernetes.io/metadata.name"] + "|" + ",".join(ports) + "|"
              + ",".join(f"{k}={v}" for k, v in sorted(pod.items())))
'

NS_RULE_PORTS=$READ_NETPOL'
for rule in policy["spec"]["egress"]:
    if "5000" in [str(p["port"]) for p in rule.get("ports") or []]:
        print("PRESENT")
'

DNS_RULE=$READ_NETPOL'
for rule in policy["spec"]["egress"]:
    for t in rule.get("to") or []:
        ns = (t.get("namespaceSelector") or {}).get("matchLabels", {})
        pod = (t.get("podSelector") or {}).get("matchLabels", {})
        if ns or pod:
            print(",".join(f"{k}={v}" for k, v in sorted({**ns, **pod}.items()))
                  + "|" + ",".join(f'"'"'{p["protocol"]}{p["port"]}'"'"' for p in rule.get("ports") or []))
'

# Guards a render that quietly attaches ports, or drops an except CIDR (169.254.0.0/16 is
# what blocks the cloud metadata endpoint).
assert "internet rule is portless and withholds every private range" \
  "ALL|10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16,100.64.0.0/10" \
  "$INTERNET_RULE"

# Emptying the list is the documented way to open the policy right up; a bare `except:`
# key would be rendered instead of dropping it.
assert "an empty excludedCidrs list drops the except key" \
  "ALL|" \
  "$INTERNET_RULE" --set "networkPolicy.buildWorkflows.internet.excludedCidrs={}"

# A policy with only an IPv4 block denies every IPv6 destination, so dual-stack builds fail.
assert "the internet rule covers IPv6 as well as IPv4" \
  "0.0.0.0/0,::/0" \
  "$READ_NETPOL"'
for rule in policy["spec"]["egress"]:
    blocks = [t["ipBlock"]["cidr"] for t in rule.get("to") or [] if "ipBlock" in t]
    if "0.0.0.0/0" in blocks:
        print(",".join(blocks))
'

# The chart must be installable before OpenChoreo has created workflows-<env>.
assert "a Namespace is rendered alongside the policy" \
  "workflows-default" \
  '
import sys, yaml
for doc in yaml.safe_load_all(sys.stdin):
    if doc and doc.get("kind") == "Namespace":
        print(doc["metadata"]["name"])
'

assert "DNS egress is scoped to the kube-dns workload" \
  'k8s-app=kube-dns,kubernetes.io/metadata.name=kube-system|UDP53,TCP53' \
  "$DNS_RULE"

WANT_PORT=6443 assert "default render carries the API-server rule on both control-plane ports" \
  "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16|443,6443" \
  "$IPBLOCK_RULE"

WANT_PORT=10082 assert "default render carries the registry rule" \
  "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16|10082" \
  "$IPBLOCK_RULE"

# --set cidrs[0]=X must replace the RFC1918 default, not merge into it.
WANT_PORT=6443 assert "apiServer.cidrs[0] override replaces the default list" \
  "172.19.0.0/16|443,6443" \
  "$IPBLOCK_RULE" --set "networkPolicy.buildWorkflows.apiServer.cidrs[0]=172.19.0.0/16"

WANT_PORT=10082 assert "registry.cidrs[0] override replaces the default list" \
  "172.19.0.0/16|10082" \
  "$IPBLOCK_RULE" --set "networkPolicy.buildWorkflows.registry.cidrs[0]=172.19.0.0/16"

# `cidr: null` is rejected by the API server, failing the whole install.
WANT_PORT=6443 assert "an empty apiServer CIDR drops the rule instead of rendering cidr: null" \
  "" \
  "$IPBLOCK_RULE" --set "networkPolicy.buildWorkflows.apiServer.cidrs[0]="

assert "extraEgress leaves the internet rule untouched" \
  "ALL|10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16,100.64.0.0/10" \
  "$INTERNET_RULE" \
  --set "networkPolicy.buildWorkflows.extraEgress[0].to[0].ipBlock.cidr=10.1.2.0/24"

WANT_PORT=8443 assert "extraEgress rules reach the rendered policy" \
  "10.1.2.0/24|8443" \
  "$IPBLOCK_RULE" \
  --set "networkPolicy.buildWorkflows.extraEgress[0].to[0].ipBlock.cidr=10.1.2.0/24" \
  --set "networkPolicy.buildWorkflows.extraEgress[0].ports[0].port=8443" \
  --set "networkPolicy.buildWorkflows.extraEgress[0].ports[0].protocol=TCP"

# The in-cluster registry rule is off by default: an unset namespace must not render a
# namespaceSelector matching every namespace.
assert "no in-cluster registry rule without a namespace" \
  "" \
  "$NS_RULE_PORTS"

WANT_NS_PORT=5000 assert "in-cluster registry rule names the containerPort, not the Service port" \
  "openchoreo-workflow-plane|5000" \
  "$NS_RULE" --set "networkPolicy.buildWorkflows.registry.inCluster.namespace=openchoreo-workflow-plane"

assert "enabled=false renders no policy at all" \
  "ABSENT" \
  "$READ_NETPOL"'print("PRESENT")' \
  --set "networkPolicy.buildWorkflows.enabled=false"

# One policy per namespace: a multi-environment install must not leave the environments it
# did not name unprotected.
assert "one policy and one namespace render per configured namespace" \
  "NP:workflows-default,workflows-staging NS:workflows-default,workflows-staging" \
  '
import sys, yaml
np, ns = [], []
for doc in yaml.safe_load_all(sys.stdin):
    if not doc:
        continue
    if doc.get("kind") == "NetworkPolicy" and doc["metadata"]["name"] == "amp-build-workflow-egress":
        np.append(doc["metadata"]["namespace"])
    if doc.get("kind") == "Namespace":
        ns.append(doc["metadata"]["name"])
print("NP:" + ",".join(sorted(np)) + " NS:" + ",".join(sorted(ns)))
' \
  --set "networkPolicy.buildWorkflows.namespaces={workflows-default,workflows-staging}"

# Without a podSelector the rule reaches every pod in the registry's namespace.
WANT_NS_PORT=5000 assert "in-cluster registry rule carries the registry podSelector" \
  "openchoreo-workflow-plane|5000|app=docker-registry" \
  "$NS_RULE_PODS" \
  --set "networkPolicy.buildWorkflows.registry.inCluster.namespace=openchoreo-workflow-plane" \
  --set "networkPolicy.buildWorkflows.registry.inCluster.podLabels.app=docker-registry"

assert "the policy lands in workflows-<environment.name>" \
  "workflows-staging" \
  "$READ_NETPOL"'print(policy["metadata"]["namespace"])' \
  --set "environment.name=staging"

# --- the pod labels the podSelector depends on ---------------------------------------

# The policy is inert if these drift apart, and nothing else would catch it.
LABELLED_TEMPLATES='
import sys, yaml
selector = None
labelled = []
for doc in yaml.safe_load_all(sys.stdin):
    if not doc:
        continue
    if doc.get("kind") == "NetworkPolicy" and doc["metadata"]["name"] == "amp-build-workflow-egress":
        selector = doc["spec"]["podSelector"]["matchLabels"]
    if doc.get("kind") == "ClusterWorkflowTemplate":
        for t in doc["spec"].get("templates") or []:
            if (t.get("metadata") or {}).get("labels"):
                labelled.append((doc["metadata"]["name"], t["name"], t["metadata"]["labels"]))
print(",".join(sorted(f"{c}/{t}" for c, t, _ in labelled)))
assert all(l == selector for _, _, l in labelled), f"pod labels do not match podSelector {selector}"
'

assert "exactly the untrusted build stages carry the podSelector labels" \
  "ballerina-buildpack-build/build-image,checkout-source/checkout,containerfile-build/build-image,gcp-buildpacks-build/build-image,publish-image/publish-image" \
  "$LABELLED_TEMPLATES"

if (( FAILURES )); then
  printf '\n%d assertion(s) failed\n' "$FAILURES"
  exit 1
fi
printf '\nall assertions passed\n'
