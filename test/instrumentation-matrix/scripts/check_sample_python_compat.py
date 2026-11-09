#!/usr/bin/env python3
"""Check deployable sample requirements resolve on supported matrix Pythons.

The heavy tier builds each sample in harness/deployable_samples.py on every
supported Python in matrix.yaml, but the samples' own PR checks only use one
version, so a pin that drops an older Python only surfaces as a nightly build
failure. This
runs `uv pip compile --python-version <v>` for each (sample, version) pair and
exits 1 naming every pair that doesn't resolve.
"""
from __future__ import annotations

import subprocess
import sys
import tempfile
from pathlib import Path

import yaml

HERE = Path(__file__).resolve().parent.parent
ROOT = HERE.parent.parent
sys.path.insert(0, str(HERE))

from harness.deployable_samples import DEPLOYABLE_SAMPLES  # noqa: E402


def main() -> int:
    """Resolve each sample on its supported matrix runtimes, reporting failures."""
    pythons = yaml.safe_load((HERE / "matrix.yaml").read_text())["python"]["versions"]
    samples = {"samples" + s.app_path: s for s in DEPLOYABLE_SAMPLES.values()}

    failures: list[str] = []
    with tempfile.TemporaryDirectory() as tmp:
        out = Path(tmp) / "requirements.lock"
        for sample, config in sorted(samples.items()):
            reqs = ROOT / sample / "requirements.txt"
            for py in pythons:
                if not config.supports_python(py):
                    print(f"Skipping {sample} on Python {py}: unsupported sample runtime")
                    continue
                print(f"::group::{sample} on Python {py}", flush=True)
                rc = subprocess.run(
                    ["uv", "pip", "compile", "--quiet", "--python-version", py, str(reqs), "-o", str(out)],
                    check=False,
                ).returncode
                print("::endgroup::", flush=True)
                if rc != 0:
                    failures.append(f"{sample}/requirements.txt does not resolve on Python {py}")

    if failures:
        print("Deployable sample dependencies are incompatible with the matrix Pythons:", file=sys.stderr)
        for f in failures:
            print(f"  {f}", file=sys.stderr)
        print("The heavy tier builds each sample on its supported Pythons in matrix.yaml.", file=sys.stderr)
        return 1

    print(f"All deployable samples resolve on their supported matrix Pythons: {', '.join(sorted(samples))}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
