"""Per-cell JSON report writer."""
from __future__ import annotations

import base64
import gzip
import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


@dataclass
class CellResult:
    cell_id: str
    result: str  # "pass" | "fail" | "skipped"
    category: str | None
    skip_reason: str | None
    durations: dict[str, float] = field(default_factory=dict)
    coverage: dict[str, Any] = field(default_factory=dict)
    violations: list[dict[str, Any]] = field(default_factory=list)
    captured_spans: list[dict[str, Any]] = field(default_factory=list)


def write_cell_report(result: CellResult, reports_dir: Path) -> Path:
    reports_dir = Path(reports_dir)
    reports_dir.mkdir(parents=True, exist_ok=True)
    spans_blob = base64.b64encode(
        gzip.compress(json.dumps(result.captured_spans).encode("utf-8"))
    ).decode("ascii")
    payload = {
        "cellId": result.cell_id,
        "result": result.result,
        "category": result.category,
        "skipReason": result.skip_reason,
        "durations": result.durations,
        "coverage": result.coverage,
        "violations": result.violations,
        "capturedSpans": spans_blob,
    }
    out = reports_dir / f"{result.cell_id}.json"
    out.write_text(json.dumps(payload, indent=2))
    return out
