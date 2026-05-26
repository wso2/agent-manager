import builtins

import pytest

from harness.exporter_handle import ExporterNotInitialized, exporter_handle


def test_handle_returns_exporter_set_by_bootstrap():
    sentinel = object()
    builtins.__amp_matrix_exporter__ = sentinel
    try:
        assert exporter_handle() is sentinel
    finally:
        del builtins.__amp_matrix_exporter__


def test_handle_raises_when_not_set():
    if hasattr(builtins, "__amp_matrix_exporter__"):
        del builtins.__amp_matrix_exporter__
    with pytest.raises(ExporterNotInitialized):
        exporter_handle()
