from harness.categorize import FailureCategory
from heavy import diagnostics
from heavy.diagnostics import Evidence, classify_no_spans


def test_boundary_categories_exist():
    assert FailureCategory.INGEST_REJECTED.value == "ingest-rejected"
    assert FailureCategory.EXPORT_FAILED.value == "export-failed"
    assert FailureCategory.COLLECTOR_NOT_RECEIVED.value == "collector-not-received"


def test_classify_ingest_rejected_on_401():
    ev = Evidence(agent_init="ok", agent_export_status=401, agent_export_error="401 Unauthorized")
    assert classify_no_spans(ev) is FailureCategory.INGEST_REJECTED


def test_classify_ingest_rejected_on_403():
    ev = Evidence(agent_init="ok", agent_export_status=403)
    assert classify_no_spans(ev) is FailureCategory.INGEST_REJECTED


def test_classify_export_failed_on_init_failure():
    ev = Evidence(agent_init="failed")
    assert classify_no_spans(ev) is FailureCategory.EXPORT_FAILED


def test_classify_export_failed_on_5xx():
    ev = Evidence(agent_init="ok", agent_export_status=503, agent_export_error="503")
    assert classify_no_spans(ev) is FailureCategory.EXPORT_FAILED


def test_classify_export_failed_on_error_without_status():
    ev = Evidence(agent_init="ok", agent_export_error="connection refused")
    assert classify_no_spans(ev) is FailureCategory.EXPORT_FAILED


def test_classify_collector_not_received_when_agent_ok_but_collector_empty():
    ev = Evidence(agent_init="ok", collector_received=False)
    assert classify_no_spans(ev) is FailureCategory.COLLECTOR_NOT_RECEIVED


def test_classify_falls_back_to_no_spans_when_inconclusive():
    assert classify_no_spans(Evidence()) is FailureCategory.NO_SPANS_CAPTURED


def test_collect_evidence_parses_401_from_agent_log(monkeypatch):
    monkeypatch.setattr(diagnostics, "_compose_logs", lambda service: (0, "no spans here\n"))
    ev = diagnostics.collect_failure_evidence(
        "Automatic Tracing initialized successfully.\n"
        "Failed to export batch code: 401, reason: Unauthorized\n")
    assert ev.agent_init == "ok"
    assert ev.agent_export_status == 401


def test_collect_evidence_detects_init_failure(monkeypatch):
    monkeypatch.setattr(diagnostics, "_compose_logs", lambda service: (1, ""))
    ev = diagnostics.collect_failure_evidence("Failed to initialize Automatic Tracing: boom\n")
    assert ev.agent_init == "failed"


def test_collect_evidence_ignores_non_status_3digit(monkeypatch):
    # A 3-digit number that is NOT an HTTP status (a span count here) must not
    # be read as a status — otherwise a bogus 401/403 would mis-classify as
    # ingest-rejected and abort every remaining cell.
    monkeypatch.setattr(diagnostics, "_compose_logs", lambda service: (1, ""))
    ev = diagnostics.collect_failure_evidence("Failed to export 250 spans to the collector: timed out\n")
    assert ev.agent_export_error is not None
    assert ev.agent_export_status is None


def test_collect_evidence_reads_the_collector_service(monkeypatch):
    asked = []
    monkeypatch.setattr(diagnostics, "_compose_logs", lambda service: (asked.append(service), (0, "TracesExporter ok"))[1])
    ev = diagnostics.collect_failure_evidence("")
    assert asked == ["otel-collector"]
    assert ev.collector_received is True


def test_collect_evidence_never_raises_when_docker_unavailable(monkeypatch):
    monkeypatch.setattr(diagnostics, "_compose_logs", lambda service: (127, ""))
    ev = diagnostics.collect_failure_evidence("")
    assert ev.agent_init is None and ev.agent_export_status is None


def test_systemic_categories_trigger_abort():
    from heavy import driver
    assert driver._is_systemic("ingest-rejected") is True
    assert driver._is_systemic("collector-not-received") is True


def test_per_cell_categories_do_not_abort():
    from heavy import driver
    for c in ("export-failed", "missing-span-kind", "pipeline-error", "no-spans-captured"):
        assert driver._is_systemic(c) is False
