#!/usr/bin/env python3
# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

"""Report which API routes a test run called, from amp-api's route-hit log.

amp-api, with LOG_ROUTE_HITS=true, logs every route it registers ("route
registered") and every request to one ("route hit") as JSON lines carrying
the route pattern, module, surface and status. This reads those lines, takes
the registered routes as the table and the hit ones as covered, leaves out
the exempted routes, and prints a markdown report: the coverage, then the
routes no request reached, grouped.

A route is named by its pattern, e.g. "POST /orgs/{orgName}/projects" (core,
below /api/v1) or "GET /m/ai-gateway/orgs/{orgName}/gateways" (a module's).
Routes of the internal surface are below /api/internal/v1 and are named with
that prefix. A request that matched no route is not logged, so it counts
nowhere.

Exemptions are a YAML list of route + reason, in this fixed form:

    exemptions:
      - route: "GET /api/internal/v1/deployments"
        reason: Called by the AI Gateway, not by users.

A route may use * as a wildcard (fnmatch).

--out writes the coverage summary format ({"areas": {group: {covered, total}}}, see
gate.py); --hits writes the hit routes as TSV (route, module, status,
count).

Usage:
    route_coverage.py LOG [LOG...] [--exemptions FILE] [--out JSON] [--hits TSV]
"""

import argparse
import fnmatch
import json
import os
import sys
from collections import Counter

INTERNAL_PREFIX = "/api/internal/v1"


def route_id(entry):
    pattern = entry.get("pattern", "")
    if entry.get("surface") == "internal":
        method, _, path = pattern.partition(" ")
        return f"{method} {INTERNAL_PREFIX}{path}"
    return pattern


def read_log(paths):
    """Returns ({route: module}, Counter((route, module, status)))."""
    registered, hits = {}, Counter()
    for path in paths:
        with open(path, encoding="utf-8", errors="replace") as f:
            for line in f:
                # docker compose logs may prefix each line with the container.
                start = line.find("{")
                if start < 0:
                    continue
                try:
                    entry = json.loads(line[start:])
                except ValueError:
                    continue
                if not isinstance(entry, dict):
                    continue
                msg = entry.get("msg")
                if msg not in ("route registered", "route hit"):
                    continue
                route, module = route_id(entry), entry.get("module", "core")
                if msg == "route registered":
                    registered[route] = module
                else:
                    hits[(route, module, entry.get("status", 0))] += 1
    return registered, hits


def read_exemptions(path):
    """Parses the fixed two-field list; returns [(route, reason)]."""
    entries, current = [], None
    with open(path, encoding="utf-8") as f:
        for n, raw in enumerate(f, 1):
            line = raw.split(" #", 1)[0].rstrip() if not raw.lstrip().startswith("#") else ""
            if not line.strip() or line.strip() == "exemptions:":
                continue
            text = line.strip()
            if text.startswith("- "):
                current = {}
                entries.append(current)
                text = text[2:].strip()
            key, sep, value = text.partition(":")
            if not sep or current is None or key.strip() not in ("route", "reason"):
                sys.exit(f"{path}:{n}: expected '- route: ...' or 'reason: ...'")
            value = value.strip()
            if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
                value = value[1:-1]
            current[key.strip()] = value
    out = []
    for e in entries:
        if not e.get("route") or not e.get("reason"):
            sys.exit(f"{path}: every exemption needs a route and a reason: {e}")
        out.append((e["route"], e["reason"]))
    return out


def group_of(route, module):
    """The route's area: its module, then its first path segment below the
    organization and project it is scoped to."""
    method, _, path = route.partition(" ")
    if path.startswith(INTERNAL_PREFIX + "/"):
        return f"{module}/internal"
    segs = [s for s in path.split("/") if s]
    if len(segs) >= 2 and segs[0] == "m":
        segs = segs[2:]
    last = None
    for literal in ("orgs", "projects"):
        if segs[:1] == [literal]:
            last = literal
            segs = segs[2:] if len(segs) > 1 and segs[1].startswith("{") else segs[1:]
    seg = segs[0] if segs else (last or "/")
    return f"{module}/{seg}"


def pct(covered, total):
    return 100.0 * covered / total if total else 100.0


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("logs", nargs="+")
    parser.add_argument("--exemptions")
    parser.add_argument("--out")
    parser.add_argument("--hits")
    parser.add_argument("--title", default="E2E route coverage")
    args = parser.parse_args()

    registered, hits = read_log(args.logs)
    if not registered:
        print("No registered routes in the log: was amp-api started with LOG_ROUTE_HITS=true?")
        return 1
    exemptions = read_exemptions(args.exemptions) if args.exemptions else []

    def exempt(route):
        return any(fnmatch.fnmatchcase(route, p) for p, _ in exemptions)

    hit_routes = {r for r, _, _ in hits}
    # A hit on a route missing from the table (its registration line was lost)
    # still names a route.
    unlisted = sorted(r for r in hit_routes if r not in registered)
    for r, m, _ in hits:
        registered.setdefault(r, m)

    table = {r: m for r, m in registered.items() if not exempt(r)}
    unused = [p for p, _ in exemptions if not any(fnmatch.fnmatchcase(r, p) for r in registered)]

    areas, uncovered = {}, {}
    for route, module in table.items():
        g = group_of(route, module)
        a = areas.setdefault(g, {"covered": 0, "total": 0})
        a["total"] += 1
        if route in hit_routes:
            a["covered"] += 1
        else:
            uncovered.setdefault(g, []).append(route)

    covered = sum(a["covered"] for a in areas.values())
    total = len(table)
    lines = [
        f"### {args.title}: {covered}/{total} routes ({pct(covered, total):.1f}%)",
        "",
        f"{len(registered)} routes registered, {len(registered) - total} exempted "
        f"({'see ' + args.exemptions if args.exemptions else 'none'}).",
        "",
        "| Area | Covered | Total | % |",
        "|---|---|---|---|",
    ]
    lines += [
        f"| `{g}` | {a['covered']} | {a['total']} | {pct(a['covered'], a['total']):.0f}% |"
        for g, a in sorted(areas.items())
    ]
    if uncovered:
        lines += ["", f"<details><summary>{total - covered} routes no request reached</summary>", ""]
        for g in sorted(uncovered):
            lines += [f"**{g}**", ""]
            lines += [f"- `{r}`" for r in sorted(uncovered[g], key=lambda r: (r.partition(' ')[2], r))]
            lines.append("")
        lines.append("</details>")
    if unlisted:
        lines += ["", "Hit but never logged as registered: " + ", ".join(f"`{r}`" for r in unlisted)]
    if unused:
        lines += ["", "Exemptions matching no registered route: " + ", ".join(f"`{p}`" for p in unused)]
    report = "\n".join(lines) + "\n"
    print(report)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as f:
            f.write(report)

    if args.out:
        with open(args.out, "w") as f:
            json.dump({"areas": dict(sorted(areas.items()))}, f, indent=2)
            f.write("\n")
    if args.hits:
        with open(args.hits, "w") as f:
            f.write("route\tmodule\tstatus\tcount\n")
            for (r, m, s), n in sorted(hits.items(), key=lambda kv: (kv[0][0], kv[0][2])):
                f.write(f"{r}\t{m}\t{s}\t{n}\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
