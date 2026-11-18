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

"""amp_shadow_runner.engine.manifests: package names from Python dependency manifests."""

import pytest

from amp_shadow_runner.engine.manifests import Dependencies, is_python_manifest, normalise, python_packages


@pytest.mark.parametrize(
    ("path", "text", "packages"),
    [
        (
            "requirements.txt",
            "# deps\nCrewAI[tools]>=0.80  # pinned\nllama_index.core==0.12\n-r base.txt\n-e .\n\n"
            "strands-agents @ https://example.com/s.whl\npydantic-ai; python_version >= '3.10'\n",
            {"crewai", "llama-index-core", "strands-agents", "pydantic-ai"},
        ),
        ("requirements/base.txt", "langchain\n", {"langchain"}),
        (
            "pyproject.toml",
            '[project]\nname = "app"\ndependencies = ["google-adk>=1.0", "httpx"]\n'
            '[project.optional-dependencies]\nai = ["smolagents"]\n'
            '[dependency-groups]\ndev = ["pytest", {include-group = "ai"}]\n',
            {"google-adk", "httpx", "smolagents", "pytest"},
        ),
        (
            "pyproject.toml",
            '[tool.poetry.dependencies]\npython = "^3.12"\nLangGraph = "^0.2"\n'
            '[tool.poetry.group.dev.dependencies]\nag2 = "*"\n',
            {"langgraph", "ag2"},
        ),
        ("pyproject.toml", "not = [valid toml", set()),
        ("Pipfile", '[packages]\ncrewai = "*"\n[dev-packages]\nPyAutoGen = "*"\n', {"crewai", "pyautogen"}),
        (
            "setup.cfg",
            "[options]\ninstall_requires =\n    langchain>=0.2\n    requests\n"
            "[options.extras_require]\nai = smolagents\n",
            {"langchain", "requests", "smolagents"},
        ),
        ("setup.cfg", "[options\nbroken", set()),
        ("package.json", '{"dependencies": {"crewai": "1"}}', set()),
    ],
)
def test_python_packages(path: str, text: str, packages: set[str]) -> None:
    assert python_packages(path, text.encode()) == packages


@pytest.mark.parametrize(
    ("path", "expected"),
    [
        ("requirements.txt", True),
        ("svc/requirements-dev.txt", True),
        ("pyproject.toml", True),
        ("Pipfile", True),
        ("setup.cfg", True),
        ("package.json", False),
        ("App.csproj", False),
        ("Ballerina.toml", False),
    ],
)
def test_is_python_manifest(path: str, expected: bool) -> None:
    assert is_python_manifest(path) == expected


def test_normalise() -> None:
    assert normalise("Llama_Index.Core") == normalise("llama-index--core") == "llama-index-core"


DEPENDENCIES = Dependencies(
    {
        "requirements.txt": frozenset({"crewai"}),
        "services/a/pyproject.toml": frozenset({"crewai", "strands-agents"}),
        "services/b/requirements.txt": frozenset({"langchain"}),
    }
)


@pytest.mark.parametrize(
    ("packages", "path", "manifest"),
    [
        (["crewai"], "services/a/src/agent.py", "services/a/pyproject.toml"),
        (["crewai"], "services/b/agent.py", "requirements.txt"),
        (["CrewAI"], "agent.py", "requirements.txt"),
        (["strands-agents"], "agent.py", "services/a/pyproject.toml"),
        (["langchain", "langchain-classic"], "services/a/agent.py", "services/b/requirements.txt"),
        (["smolagents"], "agent.py", None),
    ],
)
def test_declaring(packages: list[str], path: str, manifest: str | None) -> None:
    assert DEPENDENCIES.declaring(packages, path) == manifest
