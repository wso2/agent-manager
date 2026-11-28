from harness.cell_filter import CellFilter, parse_filter
from harness.manifest import Cell


def _cell(tl="0.61.0", fw="langchain", fwv="0.3.27", py="3.11") -> Cell:
    return Cell(
        provider_name="traceloop",
        provider_version=tl,
        framework_name=fw,
        framework_version=fwv,
        framework_package=fw,
        sample_path=f"cells/{fw}_sample.py",
        span_kinds=["llm"],
        python=py,
        contract_schema="v1",
    )


def test_parse_filter_all_or_blank_matches_everything():
    assert parse_filter("all") is None
    assert parse_filter("") is None
    assert parse_filter(" , ") is None
    # `all` anywhere means the union, not a restriction to the other tokens.
    assert parse_filter("all,langchain") is None


def test_parse_filter_splits_and_strips():
    assert parse_filter(" 0.61.0 ,0.62.4") == {"0.61.0", "0.62.4"}


def test_from_env_defaults_to_match_everything():
    f = CellFilter.from_env({})
    assert f == CellFilter()
    assert f.matches(_cell())


def test_from_env_reads_every_axis():
    f = CellFilter.from_env(
        {
            "FILTER_TRACELOOP_VERSIONS": "0.62.4",
            "FILTER_FRAMEWORKS": "crewai",
            "FILTER_FRAMEWORK_VERSIONS": "1.1.0",
            "FILTER_PYTHON_VERSIONS": "3.12",
        }
    )
    assert f.matches(_cell("0.62.4", "crewai", "1.1.0", "3.12"))
    assert not f.matches(_cell("0.62.3", "crewai", "1.1.0", "3.12"))
    assert not f.matches(_cell("0.62.4", "langchain", "1.1.0", "3.12"))
    assert not f.matches(_cell("0.62.4", "crewai", "1.2.0", "3.12"))
    assert not f.matches(_cell("0.62.4", "crewai", "1.1.0", "3.11"))


def test_recording_plan_assigns_one_owner_per_cassette(tmp_path):
    from scripts.expand_filter import plan_recordings

    first = _cell()
    another_python = _cell(py="3.12")
    another_traceloop = _cell(tl="0.62.4")
    shared_version = _cell(fwv="0.3.28")
    separate_version = _cell(fwv="1.0.0")
    (tmp_path / "cassettes" / "langchain" / separate_version.framework_version).mkdir(parents=True)

    jobs, owners = plan_recordings(
        [first, another_python, another_traceloop, shared_version, separate_version],
        tmp_path,
    )

    assert jobs == [
        {"id": first.id, "cassette": "cassettes/langchain/test_emission_cell.yaml"},
        {"id": separate_version.id, "cassette": "cassettes/langchain/1.0.0/test_emission_cell.yaml"},
    ]
    assert owners == {
        first.id: first.id,
        another_python.id: first.id,
        another_traceloop.id: first.id,
        shared_version.id: first.id,
        separate_version.id: separate_version.id,
    }
