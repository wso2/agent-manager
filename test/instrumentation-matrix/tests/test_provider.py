from pathlib import Path

from harness.provider import InstrumentationProvider
from providers import PROVIDERS


def test_traceloop_provider_is_registered():
    assert "traceloop" in PROVIDERS
    p = PROVIDERS["traceloop"]
    assert isinstance(p, InstrumentationProvider)
    assert p.name == "traceloop"


def test_traceloop_package_specs_include_pinned_sdk():
    p = PROVIDERS["traceloop"]
    specs = p.package_specs("0.60.0")
    assert "traceloop-sdk==0.60.0" in specs
    assert any(s.startswith("wrapt") for s in specs)


def test_traceloop_bootstrap_module_exists():
    p = PROVIDERS["traceloop"]
    path = p.bootstrap_module()
    assert isinstance(path, Path)
    assert path.exists()
    assert path.suffix == ".py"


def test_traceloop_contract_schema_id_is_v1():
    assert PROVIDERS["traceloop"].contract_schema_id() == "traceloop/v1"
