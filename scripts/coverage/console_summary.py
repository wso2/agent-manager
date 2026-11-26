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

"""Collect the console's per-package Vitest coverage into one summary.

Reads every console/**/coverage/coverage-summary.json written by
`pnpm test:coverage` and writes the gate.py summary format, one area per
package keyed by its path under console/, counting lines.

Usage:
    console_summary.py [--console DIR] OUTPUT.json
"""

import argparse
import json
import os
import sys


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("output")
    parser.add_argument(
        "--console",
        default=os.path.join(os.path.dirname(__file__), "..", "..", "console"),
    )
    args = parser.parse_args()
    root = os.path.abspath(args.console)

    areas = {}
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [d for d in dirnames if d != "node_modules"]
        if os.path.basename(dirpath) != "coverage":
            continue
        dirnames[:] = []
        if "coverage-summary.json" not in filenames:
            continue
        with open(os.path.join(dirpath, "coverage-summary.json")) as f:
            lines = json.load(f)["total"]["lines"]
        package = os.path.relpath(os.path.dirname(dirpath), root)
        areas[package.replace(os.sep, "/")] = {
            "covered": lines["covered"],
            "total": lines["total"],
        }

    if not areas:
        print(f"No coverage-summary.json under {root}", file=sys.stderr)
        return 1
    with open(args.output, "w") as f:
        json.dump({"areas": dict(sorted(areas.items()))}, f, indent=2)
        f.write("\n")
    print(f"Wrote {args.output} ({len(areas)} packages)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
