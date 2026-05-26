"""Provider Protocol. A 'provider' is an instrumentation stack (Traceloop today;
OpenInference / OpenLit / vanilla-OTel later) the matrix can test against.
"""
from __future__ import annotations

from pathlib import Path
from typing import Any, Protocol, runtime_checkable


@runtime_checkable
class InstrumentationProvider(Protocol):
    name: str

    def package_specs(self, version: str) -> list[str]: ...
    def bootstrap_module(self) -> Path: ...
    def contract_schema_id(self) -> str: ...
    def normalize_span(self, raw_span: Any) -> Any: ...
