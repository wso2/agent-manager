"""Discovery shim for the InMemorySpanExporter set by the provider bootstrap.

The provider's `_test_sitecustomize_*.py` runs at interpreter startup and
attaches the exporter to `builtins.__amp_matrix_exporter__`. The cell test
body retrieves it through this shim, keeping the coupling explicit (and
easily mockable).
"""
from __future__ import annotations

import builtins


class ExporterNotInitialized(RuntimeError):
    pass


def exporter_handle():
    e = getattr(builtins, "__amp_matrix_exporter__", None)
    if e is None:
        raise ExporterNotInitialized(
            "Provider bootstrap did not set __amp_matrix_exporter__; "
            "did the bootstrap module fail to import?"
        )
    return e
