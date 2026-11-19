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

"""amp_shadow_runner.engine.naming and manifests.project_name: the names found agents are shown with."""

import dataclasses
from pathlib import Path

import pytest

from amp_shadow_runner.engine.classify import Finding, Verdict
from amp_shadow_runner.engine.manifests import project_name
from amp_shadow_runner.engine.naming import is_weak
from amp_shadow_runner.engine.scan import scan_tree

STRANDS = "from strands import Agent\n"
LANGGRAPH = "from langgraph.graph import StateGraph\n"


def _write(root: Path, files: dict[str, str]) -> None:
    for rel, text in files.items():
        path = root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")


def _names(root: Path, files: dict[str, str], repository: str = "acme/r") -> dict[str, str]:
    """By external ID, the name of each agent ``scan_tree`` finds in ``files``."""
    _write(root, files)
    return {f.external_id: f.name for f in scan_tree(root, repository=repository).findings}


def _finding(name: str, framework: str = "strands", *, named: bool = False) -> Finding:
    return Finding("r:a.py:x", name, "r", "a.py", 1, framework, Verdict.PROVEN, None, {}, (), named=named)


@pytest.mark.parametrize(
    "name",
    ["agent", "Agents", "graph", "builder", "_workflow", "app", "agent_executor", "AgentExecutor", "crew", "main"],
)
def test_generic_words_are_weak(name: str) -> None:
    assert is_weak(_finding(name))


@pytest.mark.parametrize(
    "name",
    [
        "build_agent",
        "createAgent",
        "make_graph",
        "get_workflow",
        "setup_crew",
        "__init__",
        "g",
        "strands-agent",
        "graph_builder",
        "post",
        "get",
        "handler",
        "lambda_handler",
        "assistant",
        "CreateAgentAsync",
        "RunAsync",
        "MainAsync",
    ],
)
def test_factory_names_fallbacks_and_single_letters_are_weak(name: str) -> None:
    assert is_weak(_finding(name))


@pytest.mark.parametrize(
    "name", ["support_agent", "ResearchCrew", "researcher", "build_support_agent", "agent2", "async", "SupportAsync"]
)
def test_specific_names_are_strong(name: str) -> None:
    assert not is_weak(_finding(name))


def test_a_name_literal_is_never_weak() -> None:
    assert not is_weak(_finding("agent", named=True))


def test_weak_function_name_takes_the_pyproject_name(tmp_path: Path) -> None:
    names = _names(
        tmp_path,
        {
            "svc/pyproject.toml": '[project]\nname = "insurance-support"\n',
            "svc/agent/agent.py": STRANDS + "def build_agent(cfg):\n    return Agent(model=m)\n",
        },
    )
    # The external ID still holds the original name, so the agent is the same one in Agent Manager.
    assert names == {"acme/r:svc/agent/agent.py:build_agent": "insurance-support"}


def test_poetry_name_is_read_when_there_is_no_project_table(tmp_path: Path) -> None:
    names = _names(
        tmp_path, {"pyproject.toml": '[tool.poetry]\nname = "helpdesk"\n', "agent.py": STRANDS + "agent = Agent()\n"}
    )
    assert names == {"acme/r:agent.py:agent": "helpdesk"}


def test_weak_variable_takes_the_package_json_name_without_its_scope(tmp_path: Path) -> None:
    names = _names(
        tmp_path,
        {
            "web/package.json": '{"name": "@acme/travel-planner", "dependencies": {"@langchain/langgraph": "1"}}',
            "web/src/graph.ts": 'import { StateGraph } from "@langchain/langgraph";\n'
            "const graph = new StateGraph(S).compile();\n",
        },
    )
    assert names == {"acme/r:web/src/graph.ts:graph": "travel-planner"}


def test_csproj_file_name_names_the_project(tmp_path: Path) -> None:
    names = _names(
        tmp_path,
        {
            "Weather/Weather.Bot.csproj": "<Project />\n",
            "Weather/Program.cs": "using Microsoft.SemanticKernel.Agents;\nvar agent = new ChatCompletionAgent();\n",
        },
    )
    assert list(names.values()) == ["Weather.Bot"]


def test_without_a_manifest_the_folder_names_it_skipping_generic_folders(tmp_path: Path) -> None:
    names = _names(tmp_path, {"samples/it-helpdesk/agent/src/main.py": STRANDS + "agent = Agent()\n"})
    assert names == {"acme/r:samples/it-helpdesk/agent/src/main.py:agent": "it-helpdesk"}


def test_at_the_repository_root_the_repository_names_it(tmp_path: Path) -> None:
    names = _names(tmp_path, {"agent/main.py": STRANDS + "agent = Agent()\n"}, repository="acme/support-bot")
    assert names == {"acme/support-bot:agent/main.py:agent": "support-bot"}


@pytest.mark.parametrize(
    "manifest",
    [
        ("pyproject.toml", "[project]\nversion = '1'\n"),
        ("pyproject.toml", "[project]\nname = 3\n"),
        ("pyproject.toml", "[project\nname = 'broken'\n"),
        ("package.json", '{"name": ["x"]}'),
        ("package.json", '{"name": "@acme"}'),
        ("requirements.txt", "strands-agents\n"),
    ],
)
def test_a_manifest_without_a_plain_name_falls_back_to_the_folder(tmp_path: Path, manifest: tuple[str, str]) -> None:
    file, text = manifest
    names = _names(tmp_path, {f"billing/{file}": text, "billing/agent.py": STRANDS + "agent = Agent()\n"})
    assert names == {"acme/r:billing/agent.py:agent": "billing"}


def test_strong_names_are_kept(tmp_path: Path) -> None:
    names = _names(
        tmp_path,
        {
            "pyproject.toml": '[project]\nname = "proj"\n',
            "a.py": STRANDS + "x = Agent(name='Support Bot')\nsupport_agent = Agent()\nagent = Agent(name='agent')\n",
        },
    )
    assert names == {
        "acme/r:a.py:Support Bot": "Support Bot",
        "acme/r:a.py:support_agent": "support_agent",
        "acme/r:a.py:agent": "agent",
    }


def test_weak_agents_sharing_a_name_get_their_original_names(tmp_path: Path) -> None:
    names = _names(
        tmp_path,
        {
            "proj/requirements.txt": "langgraph\n",
            "proj/graph.py": LANGGRAPH + "builder = StateGraph(S)\ngraph = StateGraph(S)\n",
            "proj/agent.py": STRANDS + "proj = Agent()\n",
            "other/agent.py": STRANDS + "agent = Agent()\n",
        },
    )
    assert names == {
        "acme/r:other/agent.py:agent": "other",
        "acme/r:proj/agent.py:proj": "proj",
        "acme/r:proj/graph.py:builder": "proj (builder)",
        "acme/r:proj/graph.py:graph": "proj (graph)",
    }


def test_repeats_left_after_the_original_name_get_the_file_name(tmp_path: Path) -> None:
    names = _names(
        tmp_path,
        {
            "svc/pyproject.toml": '[project]\nname = "svc-bot"\n',
            "svc/a.py": STRANDS + "agent = Agent()\nagent = Agent()\n",
            "svc/b.py": STRANDS + "agent = Agent()\n",
            "svc/api/c.py": STRANDS + "agent = Agent()\n",
            "svc/web/c.py": STRANDS + "agent = Agent()\n",
        },
    )
    assert names == {
        "acme/r:svc/a.py:agent": "svc-bot (a.py)",
        "acme/r:svc/a.py:agent@2": "svc-bot (a.py)",  # A repeat inside one file stays.
        "acme/r:svc/b.py:agent": "svc-bot (b.py)",
        "acme/r:svc/api/c.py:agent": "svc-bot (api/c.py)",
        "acme/r:svc/web/c.py:agent": "svc-bot (web/c.py)",
    }


@pytest.mark.parametrize(
    ("files", "name"),
    [
        ({"travel/package.json": '{"name": "@acme/agent"}', "travel/graph.ts": "x"}, "travel"),
        ({"Travel/Agent.csproj": "<Project />", "Travel/Program.cs": "x"}, "Travel"),
        ({"services/workflow/main.py": "x"}, "services"),
        ({"pyproject.toml": '[project]\nname = "app"\n', "billing/src/main.py": "x"}, "billing"),
    ],
)
def test_weak_project_and_folder_names_are_skipped(tmp_path: Path, files: dict[str, str], name: str) -> None:
    code = {
        ".ts": 'import { StateGraph } from "@langchain/langgraph";\nconst graph = new StateGraph(S);\n',
        ".cs": "using Microsoft.SemanticKernel.Agents;\nvar agent = new ChatCompletionAgent();\n",
        ".py": STRANDS + "agent = Agent()\n",
    }
    files = {path: code[Path(path).suffix] if text == "x" else text for path, text in files.items()}
    assert list(_names(tmp_path, files).values()) == [name]


def test_the_repository_name_is_used_whatever_it_is(tmp_path: Path) -> None:
    names = _names(tmp_path, {"app/main.py": STRANDS + "agent = Agent()\n"}, repository="acme/agent")
    assert names == {"acme/agent:app/main.py:agent": "agent"}


def test_the_walk_goes_on_to_a_named_manifest_above(tmp_path: Path) -> None:
    names = _names(
        tmp_path,
        {
            "svc/pyproject.toml": '[project]\nname = "insurance"\n',
            "svc/agent/requirements.txt": "strands-agents\n",
            "svc/agent/agent.py": STRANDS + "agent = Agent()\n",
        },
    )
    assert names == {"acme/r:svc/agent/agent.py:agent": "insurance"}


def test_a_workspace_root_names_each_service_by_its_folder(tmp_path: Path) -> None:
    names = _names(
        tmp_path,
        {
            "package.json": '{"name": "mono", "workspaces": ["packages/*"]}',
            "packages/planner/src/graph.ts": 'import { StateGraph } from "@langchain/langgraph";\n'
            "const graph = new StateGraph(S);\n",
            "packages/booker/agent.py": STRANDS + "agent = Agent()\n",
        },
    )
    assert names == {
        "acme/r:packages/booker/agent.py:agent": "booker",
        "acme/r:packages/planner/src/graph.ts:graph": "planner",
    }


def test_a_test_double_does_not_change_a_published_name(tmp_path: Path) -> None:
    names = _names(
        tmp_path,
        {
            "bot/agent.py": STRANDS + "agent = Agent()\n",
            "bot/test_agent.py": STRANDS + "agent = Agent()\n",
        },
    )
    assert names == {"acme/r:bot/agent.py:agent": "bot", "acme/r:bot/test_agent.py:agent": "bot"}


def test_renaming_changes_only_the_name(tmp_path: Path) -> None:
    files = {"svc/a.py": STRANDS + "agent = Agent()\n"}
    _write(tmp_path, files)
    (before,) = scan_tree(tmp_path, repository="r").findings
    _write(tmp_path, {"svc/pyproject.toml": '[project]\nname = "svc-bot"\n'})
    (after,) = scan_tree(tmp_path, repository="r").findings
    assert (before.name, after.name) == ("svc", "svc-bot")
    assert before.external_id == after.external_id == "r:svc/a.py:agent"
    assert dataclasses.replace(after, name="svc", service_folder=None) == before


@pytest.mark.parametrize(
    ("path", "text", "name"),
    [
        ("pyproject.toml", '[project]\nname = " support "\n[tool.poetry]\nname = "other"\n', "support"),
        ("Ballerina.toml", '[package]\norg = "wso2"\nname = "leave_assistant"\n', "leave_assistant"),
        ("package.json", '{"name": "plain"}', "plain"),
        ("package.json", '{"name": "mono", "workspaces": ["packages/*"]}', None),
        ("src/My.App.CSPROJ", "<!DOCTYPE x>", "My.App"),
        ("pyproject.toml", f'[project]\nname = "{"n" * 101}"\n', None),
        ("pyproject.toml", '[project]\nname = "a\\u0007b"\n', None),
        ("package.json", "[1]", None),
        ("setup.cfg", "[metadata]\nname = x\n", None),
    ],
)
def test_project_name(path: str, text: str, name: str | None) -> None:
    assert project_name(path, text.encode()) == name
