"""Test-only sitecustomize.

Mirrors python-instrumentation-provider/sitecustomize.py with one change: the
OTLP HTTP exporter is replaced by InMemorySpanExporter so the cell harness can
read spans synchronously.
"""
import logging
import os

logging.basicConfig(level=logging.INFO)
log = logging.getLogger(__name__)

try:
    import builtins

    from opentelemetry import trace as otel_trace
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import SimpleSpanProcessor
    from opentelemetry.sdk.trace.export.in_memory_span_exporter import (
        InMemorySpanExporter,
    )
    from traceloop.sdk import Traceloop

    os.environ.setdefault(
        "TRACELOOP_TRACE_CONTENT", os.getenv("AMP_TRACE_CONTENT", "true")
    )
    os.environ["TRACELOOP_METRICS_ENABLED"] = "false"

    _exporter = InMemorySpanExporter()
    _provider = TracerProvider()
    _provider.add_span_processor(SimpleSpanProcessor(_exporter))
    otel_trace.set_tracer_provider(_provider)

    Traceloop.init(
        telemetry_enabled=False,
        exporter=_exporter,
    )

    builtins.__amp_matrix_exporter__ = _exporter
    log.info("matrix-test sitecustomize initialized")

except Exception as e:  # pragma: no cover
    log.exception("matrix-test sitecustomize failed: %s", e)
