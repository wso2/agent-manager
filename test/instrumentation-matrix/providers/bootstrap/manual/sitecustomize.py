"""Test-only sitecustomize for the manual-instrumentation provider.

Wires a stdlib OpenTelemetry SDK with InMemorySpanExporter so the cell harness
can read captured spans synchronously. We intentionally do NOT call
amp_instrumentation.init_otel() here — that function's job is to ship spans
over OTLP to AMP, which is a separate concern from "does the manual sample
emit spans that satisfy AMP's contract?" The matrix tests the contract; the
OTLP path has its own tests in libs/amp-instrumentation.

**Scope reminder for readers:** this verifies attribute-shape parity, not
delivery-path parity. The matrix does NOT exercise BatchSpanProcessor,
OTLP-HTTP serialisation, baggage propagation, or any other behaviour of the
production init_otel(). Coverage for those lives in libs/amp-instrumentation's
own test suite and (in v2) the heavy tier.
"""
import logging

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

    _exporter = InMemorySpanExporter()
    _provider = TracerProvider()
    _provider.add_span_processor(SimpleSpanProcessor(_exporter))
    otel_trace.set_tracer_provider(_provider)

    builtins.__amp_matrix_exporter__ = _exporter
    log.info("matrix-test sitecustomize (manual) initialized")

except Exception as e:  # pragma: no cover
    log.exception("matrix-test sitecustomize (manual) failed: %s", e)
