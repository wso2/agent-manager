"""Per-cell pytest body. Invoked inside a cell venv by noxfile.py.

Reads the cell manifest from the CELL_MANIFEST env var (JSON-encoded by the
harness), runs the sample, captures spans, validates them against the contract,
and writes a per-cell JSON report.
"""
from __future__ import annotations

import importlib.util
import json
import os
import time
from pathlib import Path

import pytest
from opentelemetry import trace as otel_trace

from harness.exporter_handle import exporter_handle
from harness.reports import CellResult, write_cell_report
from harness.validator import ContractValidator

CELL_MANIFEST = json.loads(os.environ["CELL_MANIFEST"])
REPORTS_DIR = Path(os.environ.get("REPORTS_DIR", "reports/cells"))

_FRAMEWORK = CELL_MANIFEST["framework_name"]
_SCENARIO = CELL_MANIFEST.get("cassette", "llm_chat_completion")
_CASSETTE = f"../cassettes/{_FRAMEWORK}/{_SCENARIO}.yaml"


@pytest.fixture(scope="session")
def vcr_config():
    return {
        "filter_headers": [
            ("authorization", "REDACTED"),
            ("x-api-key", "REDACTED"),
            ("openai-organization", "REDACTED"),
        ],
        "filter_post_data_parameters": [("api_key", "REDACTED")],
        "decode_compressed_response": True,
        "record_mode": os.getenv("VCR_RECORD_MODE", "none"),
    }


@pytest.mark.vcr(_CASSETTE)
def test_emission_cell():
    cell_id = CELL_MANIFEST["id"]
    schema_id = CELL_MANIFEST["contract_schema_id"]
    expected_kinds = CELL_MANIFEST["span_kinds"]

    sample_path = Path(CELL_MANIFEST["sample_path"])
    if not sample_path.is_absolute():
        sample_path = Path(__file__).resolve().parent.parent / sample_path
    spec = importlib.util.spec_from_file_location("cell_sample", sample_path)
    sample = importlib.util.module_from_spec(spec)
    assert spec is not None and spec.loader is not None

    t0 = time.monotonic()
    spec.loader.exec_module(sample)
    sample.run_scenario()
    t_scenario = time.monotonic() - t0

    otel_trace.get_tracer_provider().force_flush(timeout_millis=5000)
    exporter = exporter_handle()
    raw_spans = exporter.get_finished_spans()
    exporter.clear()

    spans = [_to_dict(s) for s in raw_spans]

    t0 = time.monotonic()
    validator = ContractValidator.load(schema_id)
    coverage = validator.assert_coverage(spans, expected_kinds=expected_kinds)
    shape_results = validator.validate_all(spans)
    t_validate = time.monotonic() - t0

    violations = [
        {
            "spanName": r.span_name,
            "kind": r.kind,
            "rule": "schema",
            "path": r.path,
            "message": r.message,
        }
        for r in shape_results
        if not r.ok
    ]

    if not spans:
        result, category = "fail", "no-spans-captured"
    elif not coverage.ok:
        result, category = "fail", "missing-span-kind"
    elif violations:
        result, category = "fail", "schema-violation"
    else:
        result, category = "pass", None

    write_cell_report(
        CellResult(
            cell_id=cell_id,
            result=result,
            category=category,
            skip_reason=None,
            durations={
                "scenario": round(t_scenario, 3),
                "validate": round(t_validate, 3),
            },
            coverage={
                "expected": expected_kinds,
                "actual": sorted(coverage.actual),
                "missing": sorted(coverage.missing),
            },
            violations=violations,
            captured_spans=spans,
        ),
        reports_dir=REPORTS_DIR,
    )

    assert result == "pass", f"{cell_id}: {category} — {violations or coverage.missing}"


def _to_dict(s) -> dict:
    """Coerce an OpenTelemetry ReadableSpan into a plain dict."""
    ctx = s.get_span_context()
    return {
        "name": s.name,
        "kind": s.kind.name if hasattr(s.kind, "name") else str(s.kind),
        "attributes": dict(s.attributes or {}),
        "traceId": format(ctx.trace_id, "032x"),
        "spanId": format(ctx.span_id, "016x"),
        "parentSpanId": format(s.parent.span_id, "016x") if s.parent else None,
        "resource": dict(s.resource.attributes or {}) if s.resource else {},
    }
