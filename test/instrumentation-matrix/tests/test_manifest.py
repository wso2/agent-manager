from pathlib import Path

from harness.manifest import Cell, expand_matrix, load_manifest

FIXTURE = Path(__file__).parent / "fixtures" / "manifest_minimal.yaml"


def test_load_manifest_parses_minimal_fixture():
    m = load_manifest(FIXTURE)
    assert m.providers["traceloop"].versions == ["0.60.0"]
    assert m.providers["traceloop"].contract_schema == "v1"
    assert m.frameworks[0].name == "langchain"
    assert m.frameworks[0].span_kinds == ["llm"]
    assert m.default_cell.framework == "langchain"


def test_expand_matrix_minimal_yields_one_cell():
    m = load_manifest(FIXTURE)
    cells = expand_matrix(m)
    assert len(cells) == 1
    c = cells[0]
    assert c.id == "traceloop-0.60.0-langchain-0.3.27-py3.11"
    assert c.provider_name == "traceloop"
    assert c.provider_version == "0.60.0"
    assert c.framework_name == "langchain"
    assert c.framework_version == "0.3.27"
    assert c.python == "3.11"
    assert c.span_kinds == ["llm"]
    assert c.sample_path == "cells/langchain_sample.py"


def test_expand_matrix_uses_provider_restriction():
    m = load_manifest(FIXTURE)
    m.frameworks[0].provider_restriction = "manual"  # no manual provider in fixture
    cells = expand_matrix(m)
    assert cells == []  # framework restricted to a provider that is not declared
