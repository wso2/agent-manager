"""Manual-instrumentation provider.

The matrix's second provider — exercises the path where the *application*
emits spans against AMP's contract via the standard OpenTelemetry API, with
no monkey-patching SDK in the loop. Validates the same contract bundle as
the Traceloop provider: the observer reads one shape regardless of source.
"""
from __future__ import annotations

from pathlib import Path

_HERE = Path(__file__).parent


class ManualProvider:
    name = "manual"

    def package_specs(self, version: str) -> list[str]:
        # The manual path needs only stdlib OpenTelemetry — no amp-instrumentation
        # dependency for the test bootstrap. See providers/bootstrap/manual/
        # sitecustomize.py for the rationale.
        return [
            "opentelemetry-sdk",
            "opentelemetry-api",
        ]

    def bootstrap_module(self) -> Path:
        return _HERE / "bootstrap" / "manual" / "sitecustomize.py"

    def contract_schema_id(self) -> str:
        # Same schema bundle as the Traceloop provider: the observer's contract
        # is source-agnostic.
        return "traceloop/v1"

    def normalize_span(self, raw_span):
        return raw_span
