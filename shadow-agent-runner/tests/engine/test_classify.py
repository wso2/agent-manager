# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

"""amp_shadow_runner.engine.classify and scan: findings, evidence, external ids and scanning a tree."""

import hashlib
from pathlib import Path

import pytest

from amp_shadow_runner.engine.classify import (
    MAX_EXTERNAL_ID_CHARS,
    FileMatches,
    Finding,
    Verdict,
    cap_external_id,
    classify,
)
from amp_shadow_runner.engine.files import Tag
from amp_shadow_runner.engine.imports import extract_imports
from amp_shadow_runner.engine.languages import Language, parse
from amp_shadow_runner.engine.manifests import Dependencies
from amp_shadow_runner.engine.match import match_python
from amp_shadow_runner.engine.scan import scan_tree
from amp_shadow_runner.engine.signatures.python import PYTHON

NO_DEPENDENCIES = Dependencies({})


def _file(path: str, source: str, tag: Tag | None = None) -> FileMatches:
    data = source.encode()
    tree = parse(Language.PYTHON, data)
    assert tree is not None
    imports = extract_imports(Language.PYTHON, tree, data)
    return FileMatches(path, tag, imports, match_python(path, tree, data, imports))


def _classify(*files: FileMatches, dependencies: Dependencies = NO_DEPENDENCIES) -> tuple[Finding, ...]:
    return classify(files, repository="support-bot", dependencies=dependencies, rules=PYTHON)


def test_finding_fields() -> None:
    source = (
        "import os\nfrom google.adk.agents import Agent\nfrom google.adk.tools import google_search\n"
        "root = Agent(name='Support', model='gemini-2.0-flash', instruction='secret prompt', tools=[google_search])\n"
    )
    (finding,) = _classify(_file("src/agent.py", source))
    assert finding.external_id == "support-bot:src/agent.py:Support"
    assert (finding.name, finding.repository, finding.path, finding.line) == (
        "Support",
        "support-bot",
        "src/agent.py",
        4,
    )
    assert (finding.framework, finding.verdict, finding.tag) == ("google-adk", Verdict.PROVEN, None)
    assert finding.attributes == {
        "framework": "google-adk",
        "model": "gemini-2.0-flash",
        "tools": [{"type": "function", "name": "google_search"}],
    }
    assert [(e.code, e.source, dict(e.observed)) for e in finding.evidence] == [
        ("d2_agent_import", "src/agent.py", {"imports": 2}),
    ]
    assert "secret prompt" not in repr(finding)


def test_import_count_ignores_other_and_type_only_imports() -> None:
    source = (
        "from typing import TYPE_CHECKING\nimport crewai\nfrom crewai import Agent, Task\nfrom crewai_tools import X\n"
        "if TYPE_CHECKING:\n    from crewai import Crew\nfrom .crewai import y\nx = Agent(role='r')\n"
    )
    (finding,) = _classify(_file("a.py", source))
    assert finding.evidence[0].observed == {"imports": 3}


def test_dependency_evidence_comes_first() -> None:
    dependencies = Dependencies({"requirements.txt": frozenset({"crewai"})})
    (finding,) = _classify(_file("a.py", "from crewai import Agent\nx = Agent()\n"), dependencies=dependencies)
    assert [(e.code, e.source, dict(e.observed)) for e in finding.evidence] == [
        ("d1_agent_dependency", "requirements.txt", {"declared": True}),
        ("d2_agent_import", "a.py", {"imports": 1}),
    ]


def test_external_ids_are_unique_and_deterministic() -> None:
    files = (
        _file("b.py", "from crewai import Agent\nx = Agent()\nx = Agent()\n[Agent(), Agent()]\n"),
        _file("a.py", "from crewai import Agent\nx = Agent()\n"),
    )
    ids = [f.external_id for f in _classify(*files)]
    assert ids == [
        "support-bot:a.py:x",
        "support-bot:b.py:x",
        "support-bot:b.py:x@2",
        "support-bot:b.py:crewai-agent",
        "support-bot:b.py:crewai-agent@2",
    ]
    assert [f.external_id for f in _classify(*reversed(files))] == ids


def test_a_repeat_skips_a_number_already_taken() -> None:
    source = "from crewai import Agent\nAgent(name='x@2')\nx = Agent()\nx = Agent()\n"
    assert [f.external_id for f in _classify(_file("a.py", source))] == [
        "support-bot:a.py:x@2",
        "support-bot:a.py:x",
        "support-bot:a.py:x@3",
    ]


def test_long_external_ids_are_capped_stably_and_uniquely() -> None:
    path = "a/" * 600 + "agent.py"  # Over 1000 characters.
    files = _file(path, "from crewai import Agent\nx = Agent()\nx = Agent()\n")
    first, second = _classify(files)
    full = f"support-bot:{path}:x"
    digest = hashlib.sha256(full.encode()).hexdigest()[:12]
    assert first.external_id == f"{full[: MAX_EXTERNAL_ID_CHARS - 13]}#{digest}"
    assert len(first.external_id) == len(second.external_id) == MAX_EXTERNAL_ID_CHARS
    assert second.external_id != first.external_id
    assert second.external_id.endswith("#" + hashlib.sha256(f"{full}@2".encode()).hexdigest()[:12])
    assert [f.external_id for f in _classify(files)] == [first.external_id, second.external_id]
    assert cap_external_id("short") == "short"


ORIGINAL = (
    "from crewai import Agent\n"
    "def support():\n    agent = Agent(role='s')\n    return agent\n"
    "def billing():\n    agent = Agent(role='b')\n    Agent(role='b2')\n    Agent(role='b3')\n"
    "class Bot:\n    def __init__(self):\n        self.agent = Agent(role='bot')\n"
)
EDITED = (
    "from crewai import Agent\nimport os\n\n"
    "def triage():\n    agent = Agent(role='t')\n    Agent(role='t2')\n    return agent\n\n"
    "def support():\n    # A comment and a blank line above the agent.\n\n    agent = Agent(role='s')\n    return agent\n"
    "def billing():\n    agent = Agent(role='b')\n    Agent(role='b2')\n    Agent(role='b3')\n"
    "class Bot:\n    def __init__(self):\n        self.agent = Agent(role='bot')\n"
)


def test_ids_do_not_change_when_code_is_added_elsewhere() -> None:
    before = [f.external_id for f in _classify(_file("app.py", ORIGINAL))]
    after = [f.external_id for f in _classify(_file("app.py", EDITED))]
    assert before == [
        "support-bot:app.py:support.agent",
        "support-bot:app.py:billing.agent",
        "support-bot:app.py:billing",
        "support-bot:app.py:billing@2",
        "support-bot:app.py:Bot.__init__.agent",
    ]
    assert after == ["support-bot:app.py:triage.agent", "support-bot:app.py:triage", *before]


def test_same_variable_in_different_functions() -> None:
    source = "from crewai import Agent\ndef a():\n    agent = Agent()\ndef b():\n    agent = Agent()\n"
    findings = _classify(_file("app.py", source))
    assert [(f.name, f.external_id) for f in findings] == [
        ("agent", "support-bot:app.py:a.agent"),
        ("agent", "support-bot:app.py:b.agent"),
    ]


def test_example_code_is_labelled() -> None:
    (finding,) = _classify(_file("examples/demo.py", "from crewai import Agent\nx = Agent()\n", tag="example"))
    assert finding.tag == "example"
    assert finding.attributes == {"framework": "crewai", "labels": {"codeArea": "example"}}


def test_tag_is_kept() -> None:
    (finding,) = _classify(_file("tests/test_a.py", "from crewai import Agent\nx = Agent()\n", tag="test"))
    assert finding.tag == "test"
    assert "labels" not in finding.attributes


def test_long_name_is_cut() -> None:
    (finding,) = _classify(_file("a.py", f"from crewai import Agent\n{'n' * 300} = Agent()\n"))
    assert finding.name == "n" * 200


def _write(root: Path, files: dict[str, str]) -> None:
    for rel, text in files.items():
        path = root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")


def test_scan_tree(tmp_path: Path) -> None:
    _write(
        tmp_path,
        {
            "requirements.txt": "crewai\n",
            "src/crew.py": "from crewai import Agent\nresearcher = Agent(role='r')\n",
            "src/broken.py": "from strands import Agent\nbot = Agent()\ndef broken(:\n",
            "src/plain.py": "print('no imports')\n",
            "src/local.py": "class Agent:\n    pass\nx = Agent()\n",
            "examples/package.json": "{}\n",
            "examples/demo.py": "from smolagents import CodeAgent\ndemo = CodeAgent(tools=[], model=m)\n",
            "web/index.ts": "import { Agent } from '@mastra/core';\n",
            "node_modules/x/a.py": "from crewai import Agent\nx = Agent()\n",
        },
    )
    (tmp_path / "src" / "link.py").symlink_to(tmp_path / "src" / "crew.py")
    result = scan_tree(tmp_path, repository="support-bot")
    assert [(f.external_id, f.framework, f.tag) for f in result.findings] == [
        ("support-bot:examples/demo.py:demo", "smolagents", "example"),
        ("support-bot:src/broken.py:bot", "strands", None),
        ("support-bot:src/crew.py:researcher", "crewai", None),
    ]
    crew = result.findings[2]
    assert [e.code for e in crew.evidence] == ["d1_agent_dependency", "d2_agent_import"]
    assert [f.service_folder for f in result.findings] == ["examples", ".", "."]
    assert (result.files_scanned, result.files_skipped, result.parse_errors) == (5, 1, 1)


def test_scan_tree_counts_an_unreadable_file_as_skipped(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    _write(tmp_path, {"a.py": "from crewai import Agent\nx = Agent()\n", "b.py": "x = 1\n"})
    original = Path.read_bytes

    def read_bytes(self: Path) -> bytes:
        if self.name == "a.py":
            raise PermissionError("denied")
        return original(self)

    monkeypatch.setattr(Path, "read_bytes", read_bytes)
    result = scan_tree(tmp_path, repository="r")
    assert (result.findings, result.files_scanned, result.files_skipped) == ((), 1, 1)
