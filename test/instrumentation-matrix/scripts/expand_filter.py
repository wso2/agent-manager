#!/usr/bin/env python3
"""Expand the matrix and filter cells by comma-separated inclusion lists.

Reads filter inputs from env vars (avoids shell-quoting hazards when called
from a GitHub Actions step with `${{ inputs.X }}` interpolations). Writes
cell IDs and a cassette recording plan to GITHUB_OUTPUT.

Env vars:
- FILTER_TRACELOOP_VERSIONS, FILTER_FRAMEWORKS, FILTER_FRAMEWORK_VERSIONS,
  FILTER_PYTHON_VERSIONS — each comma-separated, or the literal string "all".

Used by .github/workflows/instrumentation-matrix-manual.yaml.
"""
from __future__ import annotations

import json
import os
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(HERE))

from harness.cassettes import cassette_directory  # noqa: E402
from harness.cell_filter import CellFilter  # noqa: E402
from harness.manifest import Cell, expand_matrix, load_manifest  # noqa: E402


def plan_recordings(cells: list[Cell], matrix_dir: Path) -> tuple[list[dict[str, str]], dict[str, str]]:
    """Assign each cassette to the first selected cell that uses it."""
    owners: dict[str, str] = {}
    cell_owners: dict[str, str] = {}
    for cell in cells:
        directory = cassette_directory(matrix_dir / "cassettes", cell.framework_name, cell.framework_version)
        cassette = (directory / "test_emission_cell.yaml").relative_to(matrix_dir).as_posix()
        cell_owners[cell.id] = owners.setdefault(cassette, cell.id)
    jobs = [{"id": owner, "cassette": cassette} for cassette, owner in owners.items()]
    return jobs, cell_owners


def main() -> int:
    cells = expand_matrix(load_manifest(HERE / "matrix.yaml"))
    filtered = CellFilter.from_env().apply(cells)

    ids = [c.id for c in filtered]
    payload = json.dumps(ids)
    recording_cells, cassette_owners = plan_recordings(filtered, HERE)

    gh_output = os.environ.get("GITHUB_OUTPUT")
    if gh_output:
        with open(gh_output, "a") as fh:
            fh.write(f"cell_ids={payload}\n")
            fh.write(f"count={len(ids)}\n")
            fh.write(f"recording_cells={json.dumps(recording_cells)}\n")
            fh.write(f"cassette_owners={json.dumps(cassette_owners)}\n")
    else:
        # Local dev: print so the operator can sanity-check filter inputs.
        print(payload)
    print(f"resolved {len(ids)} cell(s)", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
