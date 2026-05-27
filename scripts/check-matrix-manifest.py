#!/usr/bin/env python3
"""Verify the instrumentation-matrix manifest covers everything AMP ships.

Every (traceloop_version × python_version) combo declared in
.github/release-config.json under `python-instrumentation-provider` must be
exercised by the matrix. The matrix is allowed to be a strict superset — it
can test versions AMP hasn't baselined yet — but it must never be a subset of
the baselined set, otherwise the baseline ships untested combos.

Run from CI on every PR that touches release-config.json or matrix.yaml.
Exits 0 (covered) or 1 (gap found).
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parent.parent
RC_PATH = ROOT / ".github" / "release-config.json"
MX_PATH = ROOT / "test" / "instrumentation-matrix" / "matrix.yaml"


def main() -> int:
    rc = json.loads(RC_PATH.read_text())
    mx = yaml.safe_load(MX_PATH.read_text())

    rc_entries = rc.get("python-instrumentation-provider", [])
    if not rc_entries:
        print("release-config.json has no python-instrumentation-provider entries; nothing to check")
        return 0

    mx_traceloop = set(mx["providers"]["traceloop"]["versions"])
    mx_pythons = set(mx["python"]["versions"])

    missing: list[str] = []
    for entry in rc_entries:
        tl = entry["traceloop_version"]
        if tl not in mx_traceloop:
            missing.append(
                f"matrix.yaml.providers.traceloop.versions missing {tl} (baselined in release-config.json)"
            )
        for py in entry.get("python_versions", []):
            if py not in mx_pythons:
                missing.append(
                    f"matrix.yaml.python.versions missing {py} (baselined for traceloop {tl})"
                )

    if missing:
        print("matrix.yaml does not cover release-config.json:", file=sys.stderr)
        for m in missing:
            print(f"  {m}", file=sys.stderr)
        return 1

    print(
        f"matrix.yaml covers release-config.json "
        f"(traceloop: {sorted(mx_traceloop)}, python: {sorted(mx_pythons)})"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
