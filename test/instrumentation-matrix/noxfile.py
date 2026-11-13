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
    # Shards the matrix by interpreter so CI can fan the cells out across
    # parallel jobs (one per Python version) instead of one long serial run.
    parser.add_argument("--python-version", default=None)
    args, _ = parser.parse_known_args(session.posargs)
    if args.cell_id and cell.id != args.cell_id:
        session.skip(f"filtered out by --cell-id={args.cell_id}")
    if args.python_version and cell.python != args.python_version:
        session.skip(f"filtered out by --python-version={args.python_version}")

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
    }

    # Pass through real API keys when recording (VCR_RECORD_MODE != "none");
    # otherwise inject dummy values so SDK client constructors don't reject at
    # import time. VCR replays the HTTP before the key is ever used.
    record_mode = os.environ.get("VCR_RECORD_MODE", "none")
    openai_key = os.environ.get("OPENAI_API_KEY") or "test-key-not-used-vcr-replays"
    anthropic_key = (
        os.environ.get("ANTHROPIC_API_KEY") or "test-key-not-used-vcr-replays"
    )

    # An incoming REPORTS_DIR lets nightly's revalidate-known-broken job
    # redirect output to `reports/revalidate/` without forking the session.
    # Relative paths resolve under the suite directory, mirroring how the
    # default behaves.
    reports_override = os.environ.get("REPORTS_DIR")
    if reports_override:
        reports_dir = (
            Path(reports_override)
            if Path(reports_override).is_absolute()
            else HERE / reports_override
        )
    else:
        reports_dir = HERE / "reports" / "cells"

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
            "REPORTS_DIR": str(reports_dir),
            "OPENAI_API_KEY": openai_key,
            "ANTHROPIC_API_KEY": anthropic_key,
        },
    )


@nox.session(python=False)
def heavy(session):
    """Run the heavy tier against a live AMP stack.

    Pre-condition (caller's responsibility — the nightly / manual workflows
    use the `amp-dev-stack` composite for this): Agent Manager is up on
    Docker Compose (deployments/standalone), built from the working tree, on
    its default ports, and each cell's `python<X.Y>` is on PATH. Service URLs +
    IDP creds default in heavy/driver.py; only OPENAI_API_KEY /
    ANTHROPIC_API_KEY need to be set (passed to each local agent).
    """
    # Use the interpreter that's running nox itself — that's the matrix
    # venv locally and the workflow's setup-python interpreter in CI. A bare
    # `python` resolves to Python 2 on macOS and may not exist on minimal
    # Ubuntu runners; either fails before reaching the env precheck.
    session.run(
        sys.executable,
        "heavy/driver.py",
        external=True,
        env={"PYTHONPATH": str(HERE)},
    )


@nox.session(python=False)
def report(session):
    """Aggregate per-cell reports into a summary + triage page set."""
    from harness.aggregator import build_summary
    from harness.manifest import load_manifest
    from harness.reports import load_cell_report
    from harness.triage import build_diff_markdown, required_keys_for_kinds

    reports = HERE / "reports"
    cells_dir = reports / "cells"
    heavy_dir = reports / "heavy"
    diffs_dir = reports / "diffs"
    contracts_dir = HERE / "contracts"
    reports.mkdir(parents=True, exist_ok=True)
    diffs_dir.mkdir(parents=True, exist_ok=True)

    cell_files = sorted(cells_dir.glob("*.json")) if cells_dir.exists() else []
    heavy_files = sorted(heavy_dir.glob("*.json")) if heavy_dir.exists() else []

    # When neither tier produced reports (most commonly: the outer nox
    # crashed before any cell ran), CI still wants a PR comment that makes
    # the absence visible. Write a placeholder summary instead of failing
    # the report job and silently leaving no comment.
    if not cell_files and not heavy_files:
        (reports / "summary.md").write_text(
            "## Instrumentation matrix\n"
            "\n"
            "No per-cell reports were produced. The emission job likely failed "
            "before any cell ran; check the workflow logs for the failing step.\n"
        )
        session.log(
            f"no per-cell reports under {cells_dir} or {heavy_dir}; "
            f"wrote placeholder summary to {reports / 'summary.md'}"
        )
        return

    m = load_manifest(HERE / "matrix.yaml")
    default_id = (
        f"{m.default_cell.provider}-{m.default_cell.provider_version}-"
        f"{m.default_cell.framework}-{m.default_cell.framework_version}-"
        f"py{m.default_cell.python}"
    )

    # cell-id → "<provider>/<contract-schema>" so each diff page derives its
    # required-key set from the schema the cell was validated against.
    cell_schema_id: dict[str, str] = {}
    for cell in expand_matrix(m):
        provider = PROVIDERS[cell.provider_name]
        cell_schema_id[cell.id] = provider.contract_schema_id()

    sections: list[str] = []
    if cell_files:
        sections.append(
            build_summary(cells_dir, default_cell_id=default_id, tier="emission")
        )
    if heavy_files:
        sections.append(
            build_summary(heavy_dir, default_cell_id=default_id, tier="heavy")
        )
    (reports / "summary.md").write_text("\n\n".join(sections))

    # Triage diff pages are produced for every failing cell across both tiers.
    for f in [*cells_dir.glob("*.json"), *heavy_dir.glob("*.json")]:
        r = load_cell_report(f)
        if r["result"] != "fail":
            continue
        expected_kinds = (r.get("coverage") or {}).get("expected") or []
        schema_id = cell_schema_id.get(r["cellId"])
        if schema_id is None:
            # Stale report from a cell no longer in the manifest; fall back to
            # the only schema we currently ship rather than skip the diff.
            schema_id = "traceloop/v1"
        required = required_keys_for_kinds(contracts_dir, schema_id, expected_kinds)
        diff = build_diff_markdown(r, schema_required=required)
        (diffs_dir / f"{r['cellId']}.diff.md").write_text(diff)

    session.log(f"summary written to {reports / 'summary.md'}")
