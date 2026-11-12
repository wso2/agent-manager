"""OTel-GenAI-native provider.

Exercises the current OpenTelemetry GenAI semantic conventions as emitted by
OTel-native SDKs (e.g. Microsoft Agent Framework and the OpenAI .NET SDK):
span names shaped ``{operation} {model}`` (e.g. ``chat gpt-4o-mini``),
``gen_ai.provider.name`` rather than the legacy ``gen_ai.system``, structured
``parts[]`` messages, and ``gen_ai.tool.call.*`` tool I/O.

The .NET external-agent sample (``samples/dotnet-agent/``) emits exactly this
shape. This provider re-expresses it in Python so the shape is locked into the
compatibility matrix and validated against the same contract bundle the observer
reads. It is emission-only (no amp-instrumentation release / instrumentationVersions), like
the ``manual`` provider. See ``harness/classify.py`` — the classifier already
recognises this semconv.
"""
from __future__ import annotations

from pathlib import Path

_HERE = Path(__file__).parent


class OtelGenAIProvider:
    name = "otel-genai"

    def package_specs(self, version: str) -> list[str]:
        # Vanilla OpenTelemetry only — no monkey-patching SDK. The SDK itself is
        # pinned via the framework package in matrix.yaml so the cell venv gets a
        # concrete version; here we just ensure the API is present.
        return ["opentelemetry-api"]

    def bootstrap_module(self) -> Path:
        return _HERE / "bootstrap" / "otel-genai" / "sitecustomize.py"

    def contract_schema_id(self) -> str:
        # Same schema bundle as every other provider: the observer's contract is
        # source-agnostic.
        return "traceloop/v1"

    def normalize_span(self, raw_span):
        return raw_span
