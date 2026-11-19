#!/usr/bin/env python3
"""Render the core tables of db_migrations/schema/lite.yaml as a Mermaid ER diagram.

Usage: python3 er_gen.py <lite.yaml> <out.md>
Module tables (mod_<module>_<table>) come from each module's own migrations; database-schema.md lists them.
Parses only the flow-style subset lite.yaml uses, so it needs no YAML library.
"""
import re
import sys

SRC, OUT = sys.argv[1], sys.argv[2]


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

core = tables
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

Module tables (`mod_<module>_<table>`, such as the AI Gateway's), column rules and decisions are in the [AMP Lite database schema](../../docs/amp-lite/database-schema.md).

```mermaid
{chr(10).join(lines)}
```
"""
open(OUT, "w").write(doc)
print(f"{len(core)} core tables")
