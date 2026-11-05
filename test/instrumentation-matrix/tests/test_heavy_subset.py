from pathlib import Path

from harness.cell_filter import CellFilter
from harness.heavy_subset import filter_shard, heavy_shards, select_heavy_run, select_heavy_subset
from harness.manifest import (
    DefaultCell,
    FrameworkEntry,
    HeavyTier,
    Manifest,
    ProviderEntry,
    expand_matrix,
    load_manifest,
)

FIXTURE = Path(__file__).parent / "fixtures" / "manifest_minimal.yaml"


def test_subset_is_one_cell_for_minimal_fixture():
    # The minimal fixture declares only langchain, so the deployable-framework
    # axis collapses to a single cell.
    m = load_manifest(FIXTURE)
    cells = expand_matrix(m)
    sub = select_heavy_subset(cells, m)
    assert len(sub) == 1
    assert sub[0].framework_name == "langchain"


def _multi_manifest() -> Manifest:
    """Two traceloop versions x two deployable frameworks x one python."""
    return Manifest(
        schema_version=1,
        providers={
            "traceloop": ProviderEntry(
                name="traceloop",
                versions=["0.60.0", "0.61.0"],
                contract_schema="v1",
                instrumentation_versions={"0.60.0": "0.2.1", "0.61.0": "0.3.0"},
            )
        },
        frameworks=[
            FrameworkEntry(
                name="langchain",
                package="langchain",
                versions=["0.3.27"],
                sample_path="cells/langchain_sample.py",
                span_kinds=["llm"],
            ),
            FrameworkEntry(
                name="crewai",
                package="crewai",
                versions=["1.1.0"],
                sample_path="cells/crewai_sample.py",
                span_kinds=["llm", "agent", "crewaitask"],
            ),
        ],
        python_versions=["3.11"],
        default_cell=DefaultCell("traceloop", "0.60.0", "langchain", "0.3.27", "3.11"),
        heavy_tier=HeavyTier(1, 1),
    )


def test_subset_covers_each_deployable_framework_per_provider_version():
    m = _multi_manifest()
    cells = expand_matrix(m)
    sub = select_heavy_subset(cells, m)
    pairs = {(c.provider_version, c.framework_name) for c in sub}
    # Both deployable frameworks appear under both provider versions: the
    # restored per-framework heavy axis, crossed with the init-container axis.
    assert pairs == {
        ("0.60.0", "langchain"),
        ("0.61.0", "langchain"),
        ("0.60.0", "crewai"),
        ("0.61.0", "crewai"),
    }


def test_subset_deduplicates_cell_ids():
    m = _multi_manifest()
    cells = expand_matrix(m)
    sub = select_heavy_subset(cells, m)
    ids = [c.id for c in sub]
    assert len(ids) == len(set(ids))


def test_subset_excludes_non_deployable_frameworks():
    m = _multi_manifest()
    # llama-index has no deployable sample, so it must never enter the subset.
    m.frameworks.append(
        FrameworkEntry(
            name="llama-index",
            package="llama-index",
            versions=["0.12.0"],
            sample_path="cells/llama_index_sample.py",
            span_kinds=["llm", "embedding"],
        )
    )
    cells = expand_matrix(m)
    sub = select_heavy_subset(cells, m)
    assert all(c.framework_name in {"langchain", "crewai"} for c in sub)
    assert "llama-index" not in {c.framework_name for c in sub}


def test_subset_preserves_python310_for_langchain_and_requires_python311_for_crewai():
    """Bound CrewAI runtime selection without reducing other samples' coverage."""
    m = _multi_manifest()
    m.python_versions = ["3.10", "3.11", "3.12", "3.13", "3.14"]
    cells = expand_matrix(m)
    sub = select_heavy_subset(cells, m)
    # Python is a heavy axis: the agent is rebuilt on each interpreter.
    assert {c.python for c in sub if c.framework_name == "langchain"} == {"3.10", "3.11", "3.12", "3.13", "3.14"}
    assert {c.python for c in sub if c.framework_name == "crewai"} == {"3.11", "3.12", "3.13"}
    # 2 provider versions x (5 LangChain Pythons + 3 CrewAI Pythons).
    assert len(sub) == 16


def test_per_provider_axis_pins_each_framework_to_its_representative_version():
    """The default framework pins to the default_cell version; other deployable
    frameworks pin to their first declared version. One cell per provider
    version per framework, never one per (provider_version x framework_version)."""
    m = _multi_manifest()
    m.frameworks[0].versions = ["0.3.27", "0.4.0"]  # langchain (default fw)
    m.frameworks[1].versions = ["1.1.0", "1.2.0"]  # crewai
    cells = expand_matrix(m)
    sub = select_heavy_subset(cells, m)
    langchain = [c for c in sub if c.framework_name == "langchain"]
    crewai = [c for c in sub if c.framework_name == "crewai"]
    assert {c.framework_version for c in langchain} == {"0.3.27"}  # default_cell
    assert {c.framework_version for c in crewai} == {"1.1.0"}  # first declared
    assert {c.provider_version for c in langchain} == {"0.60.0", "0.61.0"}
    assert {c.provider_version for c in crewai} == {"0.60.0", "0.61.0"}


def test_heavy_shards_are_the_provider_versions_in_order():
    m = _multi_manifest()
    m.providers["traceloop"].versions = ["0.61.0", "0.60.0", "0.62.10", "0.62.4"]
    m.providers["traceloop"].instrumentation_versions.update({"0.62.10": "0.5.0", "0.62.4": "0.4.2"})
    sub = select_heavy_subset(expand_matrix(m), m)
    # Numeric, not lexical: 0.62.10 sorts after 0.62.4.
    assert heavy_shards(sub) == ["0.60.0", "0.61.0", "0.62.4", "0.62.10"]


def test_filter_shard_keeps_only_that_provider_version():
    m = _multi_manifest()
    sub = select_heavy_subset(expand_matrix(m), m)
    shard = filter_shard(sub, "0.61.0")
    assert {c.provider_version for c in shard} == {"0.61.0"}
    assert {c.framework_name for c in shard} == {"langchain", "crewai"}
    # Every cell lands in exactly one shard.
    assert sorted(c.id for s in heavy_shards(sub) for c in filter_shard(sub, s)) == sorted(
        c.id for c in sub
    )


def test_select_heavy_run_applies_filters_to_the_subset():
    m = _multi_manifest()
    m.python_versions = ["3.10", "3.11"]
    run = select_heavy_run(m, CellFilter(traceloop_versions={"0.61.0"}, frameworks={"crewai"}))
    assert {(c.provider_version, c.framework_name) for c in run} == {("0.61.0", "crewai")}
    assert {c.python for c in run} == {"3.11"}


def test_select_heavy_run_filter_to_non_deployable_framework_is_empty():
    m = _multi_manifest()
    assert select_heavy_run(m, CellFilter(frameworks={"llama-index"})) == []


def test_select_heavy_run_cell_id_ignores_filters():
    m = _multi_manifest()
    target = "traceloop-0.60.0-crewai-1.1.0-py3.11"
    run = select_heavy_run(m, CellFilter(traceloop_versions={"0.61.0"}), cell_id=target)
    assert [c.id for c in run] == [target]


def test_select_heavy_run_cannot_force_unsupported_python_by_cell_id():
    """An explicit cell ID cannot bypass sample runtime constraints."""
    m = _multi_manifest()
    m.python_versions = ["3.10", "3.11"]
    assert select_heavy_run(m, cell_id="traceloop-0.60.0-crewai-1.1.0-py3.10") == []
