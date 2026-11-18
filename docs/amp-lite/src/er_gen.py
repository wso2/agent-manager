#!/usr/bin/env python3
"""Render the core tables of db_migrations/schema/lite.yaml as a Mermaid ER diagram.

Usage: python3 er_gen.py <lite.yaml> <out.md>
Tables used only by the gateway module are left out; database-schema.md lists them.
Parses only the flow-style subset lite.yaml uses, so it needs no YAML library.
"""
import re
import sys

SRC, OUT = sys.argv[1], sys.argv[2]

GATEWAY = {
    "gateways", "gateway_environments", "environment_default_gateways", "gateway_tokens",
    "gateway_identity_providers", "gateway_event_states", "gateway_events", "artifacts",
    "artifact_deployments", "artifact_gateway_status", "llm_provider_templates", "llm_providers",
    "llm_proxies", "mcp_proxies", "mcp_proxy_mappings", "mcp_proxy_scopes", "mcp_proxy_scope_tools",
    "mcp_proxy_endpoints", "mcp_proxy_endpoint_environments", "agent_model_configs",
    "agent_model_config_llm", "agent_model_config_mcp", "agent_model_config_mcp_envs",
    "agent_model_config_vars", "ai_applications", "api_keys", "agent_exposures", "agent_cards",
    "a2a_proxies",
}


def flow(s):
    """Parse '{a: b, c: [x, y]}' into a dict of strings and lists."""
    out = {}
    for k, v in re.findall(r"(\w+):\s*(\[[^\]]*\]|[^,}]+)", s.strip()[1:-1]):
        v = v.strip()
        out[k] = [x.strip() for x in v[1:-1].split(",") if x.strip()] if v.startswith("[") else v.strip("'\"")
    return out


tables, cur, section = [], None, None
for line in open(SRC):
    m = re.match(r"^  - name: (\w+)", line)
    if m:
        cur = dict(name=m.group(1), cols=[], pk=[], fks=[])
        tables.append(cur)
        section = None
        continue
    if cur is None:
        continue
    m = re.match(r"^    (\w+):(.*)$", line)
    if m:
        section, rest = m.group(1), m.group(2).strip()
        if section == "primary_key":
            cur["pk"] = [x.strip() for x in rest.strip("[]").split(",")]
        continue
    m = re.match(r"^      - (\{.*\})\s*$", line)
    if m and section == "columns":
        cur["cols"].append(flow(m.group(1)))
    elif m and section == "foreign_keys":
        cur["fks"].append(flow(m.group(1)))

core = [t for t in tables if t["name"] not in GATEWAY]
names = {t["name"] for t in core}
lines = ["erDiagram"]
for t in core:
    fkcols = {c for fk in t["fks"] for c in fk["columns"]}
    lines.append(f"    {t['name']} {{")
    for c in t["cols"]:
        keys = [k for k, on in (("PK", c["name"] in t["pk"]), ("FK", c["name"] in fkcols)) if on]
        null = ' "nullable"' if c.get("nullable") == "true" else ""
        lines.append(f"        {c['type']} {c['name']}{' ' + ','.join(keys) if keys else ''}{null}")
    lines.append("    }")
seen = set()
for t in core:
    for fk in t["fks"]:
        ref = fk["references"]
        if ref not in names or (ref, t["name"]) in seen:
            continue
        seen.add((ref, t["name"]))
        label = "owns" if fk.get("on_delete") == "cascade" else "has"
        lines.append(f"    {ref} ||--o{{ {t['name']} : {label}")

doc = f"""# Database ER diagram

Core tables of AMP Lite, rendered from [`db_migrations/schema/lite.yaml`](../db_migrations/schema/lite.yaml) by `docs/amp-lite/src/er_gen.py` (run through `docs/amp-lite/src/render.sh`). "owns" means the child is deleted with its parent.

The {len(tables) - len(core)} tables the gateway module uses, module tables (`mod_<module>_<table>`), column rules and decisions are in the [AMP Lite database schema](../../docs/amp-lite/database-schema.md).

```mermaid
{chr(10).join(lines)}
```
"""
open(OUT, "w").write(doc)
print(f"{len(core)} core tables")
