import json
from pathlib import Path

from harness.reports import CellResult, write_cell_report


def test_write_cell_report_creates_json(tmp_path):
    result = CellResult(
        cell_id="traceloop-0.60.0-langchain-0.3.27-py3.11",
        result="pass",
        category=None,
        skip_reason=None,
        durations={"install": 4.2, "scenario": 1.1, "validate": 0.3},
        coverage={"expected": ["llm"], "actual": ["llm"], "missing": []},
        violations=[],
        captured_spans=[{"name": "openai.chat", "kind": "CLIENT", "attributes": {}}],
    )
    out = write_cell_report(result, reports_dir=tmp_path)
    data = json.loads(Path(out).read_text())
    assert data["cellId"] == result.cell_id
    assert data["result"] == "pass"
    assert "capturedSpans" in data
    assert data["coverage"]["missing"] == []


def test_write_cell_report_includes_violations(tmp_path):
    result = CellResult(
        cell_id="x",
        result="fail",
        category="schema-violation",
        skip_reason=None,
        durations={},
        coverage={"expected": ["llm"], "actual": ["llm"], "missing": []},
        violations=[
            {
                "spanName": "openai.chat",
                "kind": "llm",
                "rule": "required",
                "path": "/attributes/gen_ai.system",
                "message": "is required",
            }
        ],
        captured_spans=[],
    )
    out = write_cell_report(result, reports_dir=tmp_path)
    data = json.loads(Path(out).read_text())
    assert data["category"] == "schema-violation"
    assert data["violations"][0]["path"] == "/attributes/gen_ai.system"
