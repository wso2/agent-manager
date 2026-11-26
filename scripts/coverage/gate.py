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

"""Check a coverage summary against a part's coverage-gate.json.

Two rules, both from the config:

- Fixed minimums: the part's overall coverage must be at least
  overall_min, and every area at least area_min, or its own floor in
  area_exceptions. Go areas count statements, console areas lines.
- Changed lines: of the lines the pull request adds or changes that
  the coverage report counts, at least changed_min percent must be
  covered. Under changed_min_lines coverable lines, or with --exempt
  (the coverage-exception label), the result is reported, not enforced.

Usage:
    gate.py SUMMARY.json CONFIG.json --title T [--base REF]
        [--repo-root DIR] [--require-base] [--exempt] [--report-only]
        (--go-profile P... --module M --module-dir D --exclude-file F
         [--require-profiles] | --console-root DIR)

SUMMARY.json is go_summary.py or console_summary.py output. The report
goes to stdout and, in GitHub Actions, to the job summary.
--report-only always exits 0, for local runs.
"""

import argparse
import json
import os
import re
import sys

from changed_lines import (
    console_line_coverage,
    evaluate_console,
    evaluate_go,
    git_changed_lines,
    go_line_coverage,
)


def pct(a):
    return 100.0 * a["covered"] / a["total"] if a["total"] else 100.0


def check_minimums(areas, config):
    """Return (failures, warnings, rows); rows are (area, now, floor, status)."""
    failures, warnings, rows = [], [], []
    exceptions = config.get("area_exceptions", {})
    area_min = config["area_min"]
    overall = {
        "covered": sum(a["covered"] for a in areas.values()),
        "total": sum(a["total"] for a in areas.values()),
    }
    if pct(overall) < config["overall_min"]:
        failures.append(
            f"overall {pct(overall):.1f}% is below its minimum {config['overall_min']:.1f}%"
        )
    for name in sorted(areas):
        floor = exceptions.get(name, area_min)
        now = pct(areas[name])
        status = "ok"
        if now < floor:
            status = "**below**"
            failures.append(f"`{name}` {now:.1f}% is below its minimum {floor:.1f}%")
        rows.append((name, f"{now:.1f}%", f"{floor:.1f}%", status))
    for name in sorted(exceptions):
        if name not in areas:
            warnings.append(f"exception `{name}` names an area that no longer exists; remove it")
        elif pct(areas[name]) >= area_min:
            warnings.append(
                f"`{name}` is at {pct(areas[name]):.1f}%, above the area minimum "
                f"{area_min:.1f}%; remove its exception"
            )
    return failures, warnings, rows


def ranges(lines):
    """[12, 13, 14, 40] -> '12-14, 40'."""
    out, start, prev = [], None, None
    for n in lines + [None]:
        if start is not None and (n is None or n != prev + 1):
            out.append(str(start) if start == prev else f"{start}-{prev}")
            start = None
        if n is not None and start is None:
            start = n
        prev = n
    return ", ".join(out)


def check_changed(result, config, exempt):
    """Return (failures, report lines) for the changed-line rule."""
    if result is None:
        return [], ["changed-line check skipped (base not found)"]
    if not result.total:
        return [], ["No coverable changed lines."]
    now = 100.0 * result.covered / result.total
    head = f"{result.covered}/{result.total} coverable changed lines covered ({now:.1f}%, minimum {config['changed_min']}%)"
    report = [head]
    failures = []
    if now < config["changed_min"]:
        if exempt:
            report.append("Not enforced: coverage-exception label set.")
        elif result.total < config["changed_min_lines"]:
            report.append(f"Not enforced: fewer than {config['changed_min_lines']} coverable changed lines.")
        else:
            failures.append(
                f"changed lines {now:.1f}% covered ({result.covered}/{result.total}), "
                f"below {config['changed_min']}%"
            )
    if result.uncovered:
        report += ["", "Uncovered changed lines:", ""]
        report += [f"- `{path}`: {ranges(lines)}" for path, lines in sorted(result.uncovered.items())]
    return failures, report


def read_excludes(path):
    with open(path) as f:
        return [re.compile(ln.strip()) for ln in f if ln.strip() and not ln.startswith("#")]


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("summary")
    parser.add_argument("config")
    parser.add_argument("--title", default="Coverage")
    parser.add_argument("--base", default="upstream/amp-2.x")
    parser.add_argument("--repo-root", default=".")
    parser.add_argument("--exempt", action="store_true")
    parser.add_argument("--report-only", action="store_true")
    parser.add_argument("--go-profile", action="append", default=[])
    parser.add_argument("--module")
    parser.add_argument("--module-dir")
    parser.add_argument("--exclude-file")
    parser.add_argument("--require-profiles", action="store_true")
    # CI always has the base; a skipped changed-line check there is a bug.
    parser.add_argument("--require-base", action="store_true")
    parser.add_argument("--console-root")
    args = parser.parse_args(argv)
    go = bool(args.go_profile)
    if go == bool(args.console_root):
        parser.error("give either --go-profile or --console-root")
    if go and not (args.module and args.module_dir):
        parser.error("--go-profile needs --module and --module-dir")

    with open(args.summary) as f:
        areas = json.load(f)["areas"]
    with open(args.config) as f:
        config = json.load(f)

    failures = []
    if args.require_profiles:
        failures += [f"coverage profile missing: {p}" for p in args.go_profile if not os.path.exists(p)]

    min_failures, warnings, rows = check_minimums(areas, config)
    failures += min_failures

    changed = git_changed_lines(args.base, args.repo_root)
    if changed is None and args.require_base:
        failures.append(f"base `{args.base}` not found, so changed lines can't be checked")
    result = None
    if changed is not None and go:
        excludes = read_excludes(args.exclude_file) if args.exclude_file else []
        coverage = go_line_coverage(args.go_profile, args.module, args.module_dir, excludes)
        result = evaluate_go(changed, coverage, args.module_dir, excludes)
    elif changed is not None:
        coverage, reported = console_line_coverage(args.console_root, args.repo_root)
        result = evaluate_console(
            changed, coverage, reported, config.get("no_coverage_packages", []), args.repo_root
        )
    changed_failures, changed_report = check_changed(result, config, args.exempt)
    failures += changed_failures

    unit = "statements" if go else "lines"
    overall = {k: sum(a[k] for a in areas.values()) for k in ("covered", "total")}
    lines = [
        f"### {args.title}: {pct(overall):.1f}% of {unit} (minimum {config['overall_min']:.1f}%)",
        "",
        f"| Area | Now ({unit}) | Minimum | |",
        "|---|---|---|---|",
    ]
    rows.sort(key=lambda r: (r[3] == "ok", r[0]))
    lines += [f"| `{n}` | {now} | {floor} | {s} |" for n, now, floor, s in rows]
    if warnings:
        lines += [""] + [f"- Warning: {w}" for w in warnings]
    lines += ["", f"#### Changed lines (against `{args.base}`)", ""] + changed_report
    if failures:
        lines += ["", "**Failed:**", ""] + [f"- {f}" for f in failures]
        lines += ["", "See docs/testing.md#coverage."]
    report = "\n".join(lines) + "\n"
    print(report)
    step = os.environ.get("GITHUB_STEP_SUMMARY")
    if step:
        with open(step, "a") as f:
            f.write(report)
    return 1 if failures and not args.report_only else 0


if __name__ == "__main__":
    sys.exit(main())
