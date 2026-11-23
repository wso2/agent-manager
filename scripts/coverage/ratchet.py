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

"""Compare a coverage summary with its committed baseline.

Both files use one format, written by the per-language summary scripts:

    {"areas": {"<package or path>": {"covered": 120, "total": 160}, ...}}

The check fails when an area's coverage drops more than --tolerance
percentage points below its baseline. New areas and areas that grew are
reported, not enforced. --update rewrites the baseline from the current
summary, for a change that raises coverage or adds or removes areas.

Usage:
    ratchet.py CURRENT.json BASELINE.json [--title T] [--tolerance 0.5] [--update]

The report goes to stdout and, in GitHub Actions, to the job summary.
"""

import argparse
import json
import os
import sys


def pct(area):
    return 100.0 * area["covered"] / area["total"] if area["total"] else 100.0


def total(areas):
    covered = sum(a["covered"] for a in areas.values())
    stmts = sum(a["total"] for a in areas.values())
    return {"covered": covered, "total": stmts}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("current")
    parser.add_argument("baseline")
    parser.add_argument("--title", default="Coverage")
    parser.add_argument("--tolerance", type=float, default=0.5)
    parser.add_argument("--update", action="store_true")
    args = parser.parse_args()

    with open(args.current) as f:
        current = json.load(f)["areas"]

    if args.update:
        with open(args.baseline, "w") as f:
            json.dump({"areas": dict(sorted(current.items()))}, f, indent=2)
            f.write("\n")
        print(f"Wrote {args.baseline} ({len(current)} areas)")
        return 0

    try:
        with open(args.baseline) as f:
            baseline = json.load(f)["areas"]
    except FileNotFoundError:
        baseline = {}

    rows, drops = [], []
    for name in sorted(set(current) | set(baseline)):
        now, base = current.get(name), baseline.get(name)
        if now is None:
            rows.append((name, "—", f"{pct(base):.1f}%", "removed"))
            continue
        if base is None:
            rows.append((name, f"{pct(now):.1f}%", "—", "new"))
            continue
        delta = pct(now) - pct(base)
        if delta < -args.tolerance:
            status = f"**dropped {delta:.1f}**"
            drops.append(name)
        elif delta > args.tolerance:
            status = f"up {delta:+.1f}"
        else:
            status = "ok"
        rows.append((name, f"{pct(now):.1f}%", f"{pct(base):.1f}%", status))

    lines = [
        f"### {args.title}: {pct(total(current)):.1f}% "
        f"(baseline {pct(total(baseline)) if baseline else 0:.1f}%)",
        "",
        "| Area | Now | Baseline | |",
        "|---|---|---|---|",
    ]
    # Drops first, then the rest by name.
    rows.sort(key=lambda r: (not r[3].startswith("**"), r[0]))
    lines += [f"| `{n}` | {now} | {base} | {s} |" for n, now, base, s in rows]
    if drops:
        lines += [
            "",
            f"{len(drops)} area(s) dropped more than {args.tolerance} points. "
            "Add tests, or if the drop is intended, update the baseline "
            "(see docs/testing.md#coverage).",
        ]
    report = "\n".join(lines) + "\n"
    print(report)
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as f:
            f.write(report)
    return 1 if drops else 0


if __name__ == "__main__":
    sys.exit(main())
