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
    project_path,
)
from amp_shadow_runner.engine.files import Tag, walk
from amp_shadow_runner.engine.imports import extract_imports
from amp_shadow_runner.engine.languages import Language, parse
from amp_shadow_runner.engine.manifests import Dependencies
from amp_shadow_runner.engine.match import match_python_file
from amp_shadow_runner.engine.scan import scan_tree
from amp_shadow_runner.engine.signatures.python import PYTHON

NO_DEPENDENCIES = Dependencies({})


def _file(path: str, source: str, tag: Tag | None = None) -> FileMatches:
    data = source.encode()
    tree = parse(Language.PYTHON, data)
    assert tree is not None
    imports = extract_imports(Language.PYTHON, tree, data)
    return FileMatches(path, tag, imports, match_python_file(path, tree, data, imports).matches)


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
        "language": "python",
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


def test_one_agent_per_branch_of_a_conditional_gets_no_repeat_number() -> None:
    source = (
        "from langchain.agents import create_agent\n"
        "if is_fast_api:\n    graph = create_agent(model=m, checkpointer=memory)\n"
        "else:\n    graph = create_agent(model=m)\n"
        "agent = create_agent(model=m)\nagent = create_agent(model=n)\n"
    )
    assert [(f.external_id, f.line) for f in _classify(_file("agent.py", source))] == [
        ("support-bot:agent.py:graph", 3),
        ("support-bot:agent.py:agent", 6),
        ("support-bot:agent.py:agent@2", 7),
    ]


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
    assert finding.attributes == {"framework": "crewai", "language": "python", "labels": {"codeArea": "example"}}


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
    # web/index.ts is scanned too (it only imports): six files, one link skipped.
    assert (result.files_scanned, result.files_skipped, result.parse_errors) == (6, 1, 1)


def test_scan_tree_progress_counts_every_walked_file_and_changes_nothing(tmp_path: Path) -> None:
    _write(
        tmp_path,
        {
            "requirements.txt": "crewai\n",
            "a.py": "from crewai import Agent\nx = Agent()\n",
            "b.py": "x = 1\n",
            "c.ts": "import { Agent } from '@mastra/core';\n",
        },
    )
    calls: list[tuple[int, int]] = []
    result = scan_tree(tmp_path, repository="r", progress=lambda done, total: calls.append((done, total)))
    total = len(walk(tmp_path).files)
    assert calls == [(n, total) for n in range(total + 1)]
    assert result == scan_tree(tmp_path, repository="r")


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


GRAPH = "from langgraph.graph import StateGraph\nworkflow = StateGraph(State)\n"
TOOL_NODE = "from langgraph.prebuilt import ToolNode\ntool_node = ToolNode([search])\n"
NODE, CONDITION, BIND = {"toolNode": True}, {"toolsCondition": True}, {"bindTools": True}


@pytest.mark.parametrize(
    ("files", "verdict", "wired"),
    [
        # Wired in the graph's own file.
        (
            {"app/graph.py": GRAPH + "from langgraph.prebuilt import ToolNode\nnode = ToolNode(tools)\n"},
            "proven",
            ("app/graph.py", NODE),
        ),
        (
            {
                "app/graph.py": GRAPH
                + "from langgraph.prebuilt import tools_condition\nworkflow.add_conditional_edges('a', tools_condition)\n"
            },
            "proven",
            ("app/graph.py", CONDITION),
        ),
        (
            {"app/graph.py": GRAPH + "def agent(state):\n    return llm.bind_tools(TOOLS).invoke(state)\n"},
            "proven",
            ("app/graph.py", BIND),
        ),
        (
            {
                "app/graph.py": GRAPH
                + "from langgraph.prebuilt import ToolNode, tools_condition\nn = ToolNode(t)\n"
                + "workflow.add_conditional_edges('a', tools_condition)\n"
            },
            "proven",
            ("app/graph.py", {"toolNode": True, "toolsCondition": True}),
        ),
        # One relative import hop.
        (
            {
                "app/__init__.py": "",
                "app/graph.py": "from .tools import tool_node\n" + GRAPH,
                "app/tools.py": TOOL_NODE,
            },
            "proven",
            ("app/tools.py", NODE),
        ),
        (
            {
                "app/__init__.py": "",
                "app/graph.py": "from . import nodes\n" + GRAPH,
                "app/nodes.py": "from langgraph.prebuilt import tools_condition\nroute = tools_condition\n",
            },
            "proven",
            ("app/nodes.py", CONDITION),
        ),
        # One absolute import hop, from the package root (src layout), or from a folder above it.
        (
            {
                "src/myapp/__init__.py": "",
                "src/myapp/graph.py": "from myapp.tools import tool_node\n" + GRAPH,
                "src/myapp/tools.py": TOOL_NODE,
            },
            "proven",
            ("src/myapp/tools.py", NODE),
        ),
        (
            {"backend/app/graph.py": "from app.nodes import tool_node\n" + GRAPH, "backend/app/nodes.py": TOOL_NODE},
            "proven",
            ("backend/app/nodes.py", NODE),
        ),
        # A plain workflow graph, or one wired only two hops away, is Not sure.
        ({"app/graph.py": GRAPH + "workflow.add_node('step', step)\n"}, "not_sure", None),
        (
            {
                "app/__init__.py": "",
                "app/graph.py": "from .nodes import step\n" + GRAPH,
                "app/nodes.py": "from .tools import tool_node\nstep = 1\n",
                "app/tools.py": TOOL_NODE,
            },
            "not_sure",
            None,
        ),
        # A .bind_tools one hop away does not count, even with a langchain import there.
        (
            {
                "app/graph.py": "from utils import call_model\n" + GRAPH,
                "app/utils.py": "from langchain_openai import ChatOpenAI\n"
                "def call_model(state):\n    return llm.bind_tools(TOOLS).invoke(state)\n",
            },
            "not_sure",
            None,
        ),
        # Nor does a Registry.bind_tools in an imported file that has no langchain or langgraph import.
        (
            {
                "app/graph.py": "from registry import registry\n" + GRAPH,
                "app/registry.py": "from myapp.core import Registry\nregistry = Registry()\nregistry.bind_tools(handlers)\n",
            },
            "not_sure",
            None,
        ),
        # An import of a module that is not in the repository is no hop.
        (
            {"app/graph.py": "from otherapp.tools import tool_node\n" + GRAPH, "lib/otherapp/tools.py": TOOL_NODE},
            "not_sure",
            None,
        ),
    ],
)
def test_graph_is_proven_when_wired_to_tools(
    tmp_path: Path, files: dict[str, str], verdict: str, wired: tuple[str, dict[str, bool]] | None
) -> None:
    _write(tmp_path, files)
    findings = [f for f in scan_tree(tmp_path, repository="r").findings if f.framework == "langgraph"]
    (finding,) = findings
    assert (finding.external_id.rsplit(":", 1)[-1], finding.verdict) == ("workflow", verdict)
    tool_evidence = [(e.source, dict(e.observed)) for e in finding.evidence if e.code == "d7_tool_calling"]
    assert tool_evidence == ([] if wired is None else [wired])


OPENAI = "from agents import Agent, Runner\nsupport = Agent(name='Support', instructions='secret')\n"


@pytest.mark.parametrize(
    ("files", "expected"),
    [
        ({"requirements.txt": "openai-agents>=0.2\n", "main.py": OPENAI}, [("Support", "proven")]),
        (
            {"pyproject.toml": '[project]\ndependencies = ["openai_agents"]\n', "main.py": OPENAI},
            [("Support", "proven")],
        ),
        # No manifest declares it, or the nearest manifest does not.
        ({"main.py": OPENAI}, [("Support", "not_sure")]),
        (
            {"requirements.txt": "openai-agents\n", "svc/requirements.txt": "openai\n", "svc/main.py": OPENAI},
            [("Support", "not_sure")],
        ),
        # A local agents package or module: no match at all.
        ({"requirements.txt": "openai-agents\n", "main.py": OPENAI, "agents/__init__.py": "class Agent: ...\n"}, []),
        ({"app/main.py": OPENAI, "app/agents.py": "class Agent: ...\n"}, []),
        # A namespace folder holding the project's own agents/agent.py: no match for agents.agent.
        (
            {
                "requirements.txt": "openai-agents\n",
                "main.py": "from agents.agent import Agent\nbot = Agent(name='Bot')\n",
                "agents/agent.py": "class Agent: ...\n",
            },
            [],
        ),
        # A file named agents.py importing the package does not import itself.
        ({"requirements.txt": "openai-agents\n", "app/agents.py": OPENAI}, [("Support", "proven")]),
        # With no manifest above the file, only a root requirements/ folder proves it, not another service.
        ({"requirements/base.txt": "openai-agents\n", "app/main.py": OPENAI}, [("Support", "proven")]),
        ({"svc/requirements.txt": "openai-agents\n", "app/main.py": OPENAI}, [("Support", "not_sure")]),
        # A namespace folder named agents does not hide the installed package.
        (
            {"requirements.txt": "openai-agents\n", "main.py": OPENAI, "agents/impl.py": "x = 1\n"},
            [("Support", "proven")],
        ),
    ],
)
def test_openai_agent_is_proven_by_its_dependency(
    tmp_path: Path, files: dict[str, str], expected: list[tuple[str, str]]
) -> None:
    _write(tmp_path, files)
    findings = scan_tree(tmp_path, repository="r").findings
    assert [(f.name, f.verdict) for f in findings] == expected
    for finding in findings:
        assert finding.framework == "openai-agents"
        codes = [e.code for e in finding.evidence]
        assert codes == (
            ["d1_agent_dependency", "d2_agent_import"] if finding.verdict == "proven" else ["d2_agent_import"]
        )


CREW_BASE = (
    "from crewai import Agent, Crew\nfrom crewai.project import CrewBase, agent, crew\n"
    "@CrewBase\nclass ResearchCrew:\n"
    "    @agent\n    def researcher(self) -> Agent:\n        return Agent(config=self.agents_config['researcher'])\n"
    "    @crew\n    def crew(self) -> Crew:\n        return Crew(agents=self.agents, tasks=self.tasks)\n"
)


def test_crew_labels_its_agents() -> None:
    (finding,) = _classify(_file("src/crew.py", CREW_BASE))
    assert (finding.name, finding.verdict) == ("researcher", Verdict.PROVEN)
    assert finding.attributes == {"framework": "crewai", "language": "python", "labels": {"crew": "ResearchCrew"}}


def test_crew_label_merges_with_the_example_label() -> None:
    (finding,) = _classify(_file("examples/crew.py", CREW_BASE, tag="example"))
    assert finding.attributes["labels"] == {"codeArea": "example", "crew": "ResearchCrew"}


def test_a_crew_linking_no_agent_is_not_sure() -> None:
    source = "from crewai import Crew\nfrom myapp.agents import build\nteam = Crew(agents=build(), tasks=[])\n"
    (finding,) = _classify(_file("src/crew.py", source))
    assert (finding.name, finding.framework, finding.verdict) == ("team", "crewai", Verdict.NOT_SURE)
    assert finding.external_id == "support-bot:src/crew.py:team"
    assert "labels" not in finding.attributes


def test_other_findings_keep_the_lenient_dependency_evidence() -> None:
    # R10's D1 evidence still falls back to any declaring manifest; only the DEPENDENCY proof is strict.
    dependencies = Dependencies({"svc/requirements.txt": frozenset({"crewai", "openai-agents"})})
    crew, openai = _classify(
        _file("app/a.py", "from crewai import Agent\nx = Agent()\n"),
        _file("app/b.py", "from agents import Agent\ny = Agent()\n"),
        dependencies=dependencies,
    )
    assert [e.code for e in crew.evidence] == ["d1_agent_dependency", "d2_agent_import"]
    assert (openai.verdict, [e.code for e in openai.evidence]) == (Verdict.NOT_SURE, ["d2_agent_import"])


def test_language_and_project_path_attributes(tmp_path: Path) -> None:
    _write(
        tmp_path,
        {
            "pyproject.toml": '[project]\nname = "support"\ndependencies = ["crewai"]\n',
            "app.py": "from crewai import Agent\nroot = Agent()\n",
            "samples/web/package.json": '{"dependencies": {"@mastra/core": "1"}}',
            "samples/web/src/chat.tsx": 'import { Agent } from "@mastra/core";\nconst chat = new Agent({});\n',
            "samples/web/src/lib.mjs": 'import { Agent } from "@mastra/core";\nconst lib = new Agent({});\n',
            "samples/bal/Ballerina.toml": '[package]\nname = "leave"\n',
            "samples/bal/main.bal": "import ballerina/ai;\nfinal ai:Agent leave = check new (model);\n",
        },
    )
    found = {f.path: f.attributes for f in scan_tree(tmp_path, repository="r").findings}
    assert {path: (a["language"], a["projectPath"]) for path, a in found.items()} == {
        "app.py": ("python", "/"),
        "samples/bal/main.bal": ("ballerina", "/samples/bal"),
        "samples/web/src/chat.tsx": ("typescript", "/samples/web"),
        "samples/web/src/lib.mjs": ("javascript", "/samples/web"),
    }


def test_no_project_path_without_a_manifest_above() -> None:
    (finding,) = _classify(_file("src/agent.py", "from crewai import Agent\nx = Agent()\n"))
    assert finding.service_folder is None
    assert "projectPath" not in finding.attributes
    assert finding.attributes["language"] == "python"


@pytest.mark.parametrize(
    ("folder", "path"), [(".", "/"), ("svc", "/svc"), ("samples/customer-agent", "/samples/customer-agent")]
)
def test_project_path(folder: str, path: str) -> None:
    assert project_path(folder) == path
