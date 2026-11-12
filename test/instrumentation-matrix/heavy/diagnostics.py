"""Heavy-tier failure diagnostics: capture pipeline-boundary evidence when a
cell captures 0 spans, and classify which boundary the trail went cold at.

Evidence is gathered best-effort from the locally run agent's log and the
collector's `docker compose logs` (deployments/standalone); any failure leaves
the corresponding field None (indeterminate) and never raises. The classifier
is pure so it unit-tests without a stack.
"""
from __future__ import annotations

import os
from pathlib import Path
import re
import subprocess
from dataclasses import dataclass, field

from harness.categorize import FailureCategory

_COLLECTOR_SERVICE = "otel-collector"
_LOG_TAIL = "300"
_COMPOSE_DIR = Path(os.environ.get(
    "AMP_COMPOSE_DIR", Path(__file__).resolve().parents[3] / "deployments/standalone"))


@dataclass
class Evidence:
    agent_init: str | None = None          # "ok" | "failed" | None (marker not found)
    agent_export_status: int | None = None # HTTP status parsed from an OTLP exporter error line
    agent_export_error: str | None = None  # short excerpt of the exporter error
    collector_received: bool | None = None # True / False / None (indeterminate)
    raw: dict[str, str] = field(default_factory=dict)  # truncated log excerpts, for the report


def classify_no_spans(ev: Evidence) -> FailureCategory:
    """Map boundary evidence to a failure category. Order matters: a gateway
    rejection (401/403) is the most specific and actionable signal."""
    if ev.agent_export_status in (401, 403):
        return FailureCategory.INGEST_REJECTED
    if (
        ev.agent_init == "failed"
        or (ev.agent_export_status is not None and ev.agent_export_status >= 500)
        or (ev.agent_export_error is not None and ev.agent_export_status is None)
    ):
        return FailureCategory.EXPORT_FAILED
    if ev.agent_init == "ok" and ev.agent_export_error is None and ev.collector_received is False:
        return FailureCategory.COLLECTOR_NOT_RECEIVED
    return FailureCategory.NO_SPANS_CAPTURED


def _compose_logs(service: str) -> tuple[int, str]:
    """Tail a compose service's logs best-effort; return (returncode, output)."""
    try:
        proc = subprocess.run(
            ["docker", "compose", "logs", "--no-color", "--tail", _LOG_TAIL, service],
            cwd=_COMPOSE_DIR, capture_output=True, text=True, timeout=30,
        )
        return proc.returncode, (proc.stdout or "") + (proc.stderr or "")
    except Exception:  # noqa: BLE001 - diagnostics must never raise
        return 127, ""


def _agent_signals(log: str, ev: Evidence) -> None:
    if not log:
        return
    ev.raw["agent"] = log[-2000:]
    if "Automatic Tracing initialized successfully" in log:
        ev.agent_init = "ok"
    if "Failed to initialize Automatic Tracing" in log:
        ev.agent_init = "failed"
    for line in log.splitlines():
        if "failed to export" in line.lower():
            ev.agent_export_error = line.strip()[:300]
            # Only read a 3-digit number that appears in an HTTP-status context
            # (the OTLP/HTTP exporter logs "... code: 401, reason: ...").
            # A bare \d{3} would grab a span count/port and a bogus 401/403
            # would mis-classify as ingest-rejected — which aborts every cell.
            m = re.search(r"(?:code|status(?:\s*code)?|http)[:=\s]+(\d{3})\b", line, re.I)
            if m:
                ev.agent_export_status = int(m.group(1))
            break


def _collector_signals(ev: Evidence) -> None:
    rc, log = _compose_logs(_COLLECTOR_SERVICE)
    if rc != 0:
        return
    ev.raw["collector"] = log[-2000:]
    # The collector logs span receipt; absence of any receipt marker means it
    # never got the agent's spans. Keep the marker list narrow and reviewable.
    ev.collector_received = ("TracesExporter" in log) or ("spans" in log.lower())


def collect_failure_evidence(agent_log: str) -> Evidence:
    """Gather agent + collector log signals for a 0-span cell. Best-effort:
    each probe is independent and any failure leaves its fields None."""
    ev = Evidence()
    _agent_signals(agent_log, ev)
    _collector_signals(ev)
    return ev
