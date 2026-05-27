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


def test_manual_provider_is_registered():
    assert "manual" in PROVIDERS
    p = PROVIDERS["manual"]
    assert isinstance(p, InstrumentationProvider)
    assert p.name == "manual"


def test_manual_package_specs_are_stdlib_only():
    # The manual bootstrap intentionally avoids amp-instrumentation; init_otel()
    # configures OTLP export to AMP, which is orthogonal to the contract the
    # matrix tests. See providers/bootstrap/manual/sitecustomize.py.
    specs = PROVIDERS["manual"].package_specs("any")
    assert "opentelemetry-sdk" in specs
    assert "opentelemetry-api" in specs
    assert not any("amp-instrumentation" in s for s in specs)


def test_manual_bootstrap_module_exists():
    path = PROVIDERS["manual"].bootstrap_module()
    assert path.exists()
    assert path.name == "sitecustomize.py"


def test_manual_shares_contract_schema_with_traceloop():
    # Both providers validate against traceloop/v1: the observer reads one
    # shape regardless of where the spans came from. See design §10.
    assert (
        PROVIDERS["manual"].contract_schema_id()
        == PROVIDERS["traceloop"].contract_schema_id()
    )
