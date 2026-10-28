#!/usr/bin/env bash
set -euo pipefail

CHART_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
rendered="$(mktemp)"
trap 'rm -f "$rendered"' EXIT

helm template test "$CHART_DIR" "$@" >"$rendered"

python3 - "$rendered" <<'PY'
import sys
import yaml

with open(sys.argv[1], encoding="utf-8") as stream:
    documents = list(yaml.safe_load_all(stream))

bootstrap = next(
    doc for doc in documents
    if isinstance(doc, dict)
    and doc.get("kind") == "ConfigMap"
    and doc.get("metadata", {}).get("name") == "amp-thunder-bootstrap"
)
data = bootstrap["data"]

amp = yaml.safe_load(data["60-amp-resource-server.yaml"])
assert amp["identifier"] == "urn:wso2:amp", amp["identifier"]

servers = list(yaml.safe_load_all(data["60-mcp-resource-servers.yaml"]))
assert all(isinstance(server, dict) for server in servers)
by_id = {server["id"]: server for server in servers}
main = by_id["amp-agent-manager-mcp-resource-srv"]
observer = by_id["amp-observer-mcp-resource-server"]
assert main["identifier"] == "http://api.amp.localhost:8080/mcp"
assert observer["identifier"] == "http://traces.amp.localhost:11080/mcp"
assert "amp-agent-manager-dev-mcp-resource-server" not in by_id

setup_job = next(
    doc for doc in documents
    if isinstance(doc, dict)
    and doc.get("kind") == "Job"
    and doc.get("metadata", {}).get("name", "").endswith("-thunder-setup")
)
mounts = setup_job["spec"]["template"]["spec"]["containers"][0]["volumeMounts"]
assert any(mount.get("subPath") == "60-mcp-resource-servers.yaml" for mount in mounts)

main_permissions = {
    action["handle"]
    for resource in main["resources"]
    for action in resource.get("actions", [])
}
assert main_permissions == {
    "create", "read", "build", "env-non-production", "suspend", "token-manage"
}
observer_permissions = {
    action["handle"]
    for resource in observer["resources"]
    for action in resource.get("actions", [])
}
assert observer_permissions == {"trace-read", "log-read", "build-log-read", "metric-read"}

for filename in (
    "61-amp-role-admin.yaml",
    "62-amp-role-developer.yaml",
    "63-amp-role-ai-lead.yaml",
    "64-amp-role-platform-engineer.yaml",
):
    role = yaml.safe_load(data[filename])
    ids = {entry["resourceServerId"] for entry in role["permissions"]}
    assert all(isinstance(i, str) for i in ids), ids
    assert "amp-resource-server" in ids
    assert "amp-agent-manager-mcp-resource-srv" in ids
    assert "amp-observer-mcp-resource-server" in ids

# Thunder's PostgreSQL schema declares RESOURCE_SERVER.ID VARCHAR(36). SQLite
# ignores the length, so a longer id only fails on PostgreSQL installs.
THUNDER_RESOURCE_SERVER_ID_MAX = 36
for server in [amp, *servers]:
    assert len(server["id"]) <= THUNDER_RESOURCE_SERVER_ID_MAX, server["id"]

print("Thunder MCP resource-server render checks passed")
PY
