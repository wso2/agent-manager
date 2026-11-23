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

"""Summarise Go cover profiles per package for ratchet.py.

Reads one or more profiles (any mode). A block that appears in several
profiles is counted once, and counts as covered if any profile hit it.
Paths are made relative to --module; files matching an --exclude regex
(or a line of an --exclude-file) are dropped, so generated code, mocks
and test helpers don't count.

Usage:
    go_summary.py PROFILE... --module M --out SUMMARY.json
        [--exclude REGEX]... [--exclude-file FILE] [--profile-out MERGED.out]

--profile-out writes the kept blocks as one deduplicated profile, for
`go tool cover -html`.

Missing profiles are skipped with a warning, so a failed test job still
yields a report from the tiers that ran.
"""

import argparse
import json
import os
import re
import sys


def read_excludes(path):
    with open(path) as f:
        return [ln.strip() for ln in f if ln.strip() and not ln.startswith("#")]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("profiles", nargs="+")
    parser.add_argument("--module", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--exclude", action="append", default=[])
    parser.add_argument("--exclude-file")
    parser.add_argument("--profile-out")
    args = parser.parse_args()

    patterns = list(args.exclude)
    if args.exclude_file:
        patterns += read_excludes(args.exclude_file)
    excludes = [re.compile(p) for p in patterns]
    prefix = args.module.rstrip("/") + "/"

    # (file, "start,end") -> [statements, covered]
    blocks = {}
    read = 0
    for path in args.profiles:
        if not os.path.exists(path):
            print(f"warning: {path} not found, skipping", file=sys.stderr)
            continue
        read += 1
        with open(path) as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("mode:"):
                    continue
                loc, stmts, count = line.rsplit(" ", 2)
                full, span = loc.rsplit(":", 1)
                if not full.startswith(prefix):
                    continue
                name = full[len(prefix):]
                if any(rx.search(name) for rx in excludes):
                    continue
                block = blocks.setdefault((full, span), [int(stmts), False])
                block[1] = block[1] or int(count) > 0
    if not read:
        print("error: no coverage profiles found", file=sys.stderr)
        return 1

    areas = {}
    for (full, _), (stmts, hit) in blocks.items():
        pkg = os.path.dirname(full[len(prefix):]) or "."
        area = areas.setdefault(pkg, {"covered": 0, "total": 0})
        area["total"] += stmts
        area["covered"] += stmts if hit else 0

    with open(args.out, "w") as f:
        json.dump({"areas": dict(sorted(areas.items()))}, f, indent=2)
        f.write("\n")
    if args.profile_out:
        with open(args.profile_out, "w") as f:
            f.write("mode: set\n")
            for (full, span), (stmts, hit) in blocks.items():
                f.write(f"{full}:{span} {stmts} {int(hit)}\n")
    covered = sum(a["covered"] for a in areas.values())
    total = sum(a["total"] for a in areas.values())
    print(
        f"{len(areas)} packages, {covered}/{total} statements, "
        f"{100.0 * covered / total if total else 100.0:.1f}% covered"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
