"""Keep dependency resolution aligned with deployable sample runtime support."""
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path
from types import SimpleNamespace

import pytest


@pytest.fixture
def compat():
    """Load the compatibility checker without invoking its CLI."""
    path = Path(__file__).parents[1] / "scripts" / "check_sample_python_compat.py"
    spec = spec_from_file_location("check_sample_python_compat", path)
    module = module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_resolves_supported_pairs_and_preserves_other_sample_python310(compat, monkeypatch, capsys):
    """Only CrewAI drops Python 3.10, while all supported pairs resolve."""
    calls = []

    def resolve(args, **kwargs):
        calls.append((Path(args[6]).parent.name, args[5]))
        return SimpleNamespace(returncode=0)

    monkeypatch.setattr(compat.subprocess, "run", resolve)
    assert compat.main() == 0
    assert set(calls) == {
        ("customer-support-agent", version) for version in ("3.10", "3.11", "3.12", "3.13")
    } | {
        ("crewai-agent", version) for version in ("3.11", "3.12", "3.13")
    }
    assert len(calls) == 7
    assert "Skipping samples/crewai-agent on Python 3.10" in capsys.readouterr().out


def test_supported_runtime_resolution_failure_is_reported(compat, monkeypatch, capsys):
    """A supported interpreter failure still fails the compatibility check."""
    def resolve(args, **kwargs):
        failed = "crewai-agent" in args[6] and args[5] == "3.11"
        return SimpleNamespace(returncode=int(failed))

    monkeypatch.setattr(compat.subprocess, "run", resolve)
    assert compat.main() == 1
    assert "samples/crewai-agent/requirements.txt does not resolve on Python 3.11" in capsys.readouterr().err


def test_future_python_excluded_only_for_bounded_sample(compat, monkeypatch):
    """A future matrix runtime respects CrewAI's existing upper bound."""
    calls = []
    monkeypatch.setattr(compat.yaml, "safe_load", lambda _: {"python": {"versions": ["3.13", "3.14"]}})

    def resolve(args, **kwargs):
        calls.append((Path(args[6]).parent.name, args[5]))
        return SimpleNamespace(returncode=0)

    monkeypatch.setattr(compat.subprocess, "run", resolve)
    assert compat.main() == 0
    assert set(calls) == {
        ("crewai-agent", "3.13"),
        ("customer-support-agent", "3.13"),
        ("customer-support-agent", "3.14"),
    }
