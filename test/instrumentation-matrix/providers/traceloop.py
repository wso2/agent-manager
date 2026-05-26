from __future__ import annotations

from pathlib import Path

_HERE = Path(__file__).parent


class TraceloopProvider:
    name = "traceloop"

    def package_specs(self, version: str) -> list[str]:
        return [
            f"traceloop-sdk=={version}",
            # Mirrors python-instrumentation-provider/requirements.txt: wrapt 2.x
            # removed the `module=` kwarg that opentelemetry-instrumentation-*
            # 0.60.0 still calls.
            "wrapt<2.0.0",
            "opentelemetry-sdk",
            "opentelemetry-api",
        ]

    def bootstrap_module(self) -> Path:
        # Auto-loaded by Python at interpreter startup when this file's parent
        # directory is on PYTHONPATH. Filename must literally be sitecustomize.py.
        return _HERE / "bootstrap" / "traceloop" / "sitecustomize.py"

    def contract_schema_id(self) -> str:
        return "traceloop/v1"

    def normalize_span(self, raw_span):
        return raw_span
