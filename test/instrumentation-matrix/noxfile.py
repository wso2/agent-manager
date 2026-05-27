"""nox driver — one session per emission cell.

Reads matrix.yaml, expands to cells, and runs each cell in its own venv with
the right pinned packages installed. Reports land in reports/cells/.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
from pathlib import Path

import nox

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from harness.manifest import expand_matrix, load_manifest  # noqa: E402
from providers import PROVIDERS  # noqa: E402


def _cells():
    manifest = load_manifest(HERE / "matrix.yaml")
    return expand_matrix(manifest)


CELLS = _cells()


@nox.session(python=False)
@nox.parametrize("cell", CELLS, ids=[c.id for c in CELLS])
def emission(session, cell):
    """Run one emission-tier cell."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--cell-id", default=None)
    args, _ = parser.parse_known_args(session.posargs)
    if args.cell_id and cell.id != args.cell_id:
        session.skip(f"filtered out by --cell-id={args.cell_id}")

    provider = PROVIDERS[cell.provider_name]
    venv_dir = HERE / ".nox" / cell.id
    pip = venv_dir / "bin" / "pip"
    py = venv_dir / "bin" / "python"

    if not py.exists():
        session.run(f"python{cell.python}", "-m", "venv", str(venv_dir), external=True)

    install_specs = [
        *provider.package_specs(cell.provider_version),
        f"{cell.framework_package}=={cell.framework_version}",
        # Per-framework runtime extras declared in matrix.yaml.
        *cell.extras,
        # Test infra.
        "pytest",
        "pytest-recording",
        "vcrpy",
        "jsonschema",
        "pyyaml",
    ]
    session.run(str(pip), "install", "--quiet", *install_specs, external=True)

    pythonpath = f"{HERE}:{provider.bootstrap_module().parent}"

    cell_manifest = {
        "id": cell.id,
        "framework_name": cell.framework_name,
        "framework_package": cell.framework_package,
        "framework_version": cell.framework_version,
        "sample_path": cell.sample_path,
        "span_kinds": cell.span_kinds,
        "contract_schema_id": provider.contract_schema_id(),
        "cassette": cell.cassette,
    }

    # Pass through real API keys when recording (VCR_RECORD_MODE != "none");
    # otherwise inject dummy values so SDK client constructors don't reject at
    # import time. VCR replays the HTTP before the key is ever used.
    record_mode = os.environ.get("VCR_RECORD_MODE", "none")
    openai_key = os.environ.get("OPENAI_API_KEY") or "test-key-not-used-vcr-replays"
    anthropic_key = (
        os.environ.get("ANTHROPIC_API_KEY") or "test-key-not-used-vcr-replays"
    )

    session.run(
        str(py),
        "-m",
        "pytest",
        "harness/test_cell.py",
        "-v",
        external=True,
        env={
            "PYTHONPATH": pythonpath,
            "VCR_RECORD_MODE": record_mode,
            "CELL_MANIFEST": json.dumps(cell_manifest),
            "REPORTS_DIR": str(HERE / "reports" / "cells"),
            "OPENAI_API_KEY": openai_key,
            "ANTHROPIC_API_KEY": anthropic_key,
        },
    )
