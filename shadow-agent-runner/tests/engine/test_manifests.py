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

from amp_shadow_runner.engine.imports import ImportKind
from amp_shadow_runner.engine.manifests import (
    Dependencies,
    Ecosystem,
    ballerina_packages,
    csproj_usings,
    ecosystem,
    is_python_manifest,
    normalise,
    npm_packages,
    nuget_packages,
    python_packages,
    read_packages,
    service_folder,
)


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


@pytest.mark.parametrize(
    ("text", "packages"),
    [
        (
            '{"name": "web", "dependencies": {"@mastra/core": "^0.10", "zod": "3"}, '
            '"devDependencies": {"@openai/agents": "*"}, "optionalDependencies": {"ai": "5"}, '
            '"peerDependencies": {"@langchain/langgraph": "*"}, "scripts": {"langchain": "x"}}',
            {"@mastra/core", "zod", "@openai/agents", "ai"},
        ),
        ('{"dependencies": ["ai"]}', set()),
        ("[1, 2]", set()),
        ("{not json", set()),
        ("[" * 100_000, set()),
        ("", set()),
    ],
)
def test_npm_packages(text: str, packages: set[str]) -> None:
    assert npm_packages(text.encode()) == packages


@pytest.mark.parametrize(
    ("path", "kind"),
    [
        ("requirements.txt", Ecosystem.PYTHON),
        ("svc/pyproject.toml", Ecosystem.PYTHON),
        ("web/package.json", Ecosystem.NPM),
        ("src/App/App.csproj", Ecosystem.NUGET),
        ("src/App/APP.CSPROJ", Ecosystem.NUGET),
        ("Directory.Packages.props", Ecosystem.NUGET),
        ("legacy/packages.config", Ecosystem.NUGET),
        ("Ballerina.toml", Ecosystem.BALLERINA),
        ("Dependencies.toml", None),
    ],
)
def test_ecosystem(path: str, kind: Ecosystem | None) -> None:
    assert ecosystem(path) == kind


def test_read_packages_by_ecosystem() -> None:
    assert read_packages("web/package.json", b'{"dependencies": {"AI": "5"}}') == frozenset({"ai"})
    assert read_packages("requirements.txt", b"crewai\n") == frozenset({"crewai"})
    assert read_packages(
        "App.csproj", b'<Project><ItemGroup><PackageReference Include="A.B"/></ItemGroup></Project>'
    ) == frozenset({"a-b"})
    assert read_packages("App.csproj", b"<!DOCTYPE x><Project/>") is None
    assert read_packages("Ballerina.toml", b'[[dependency]]\norg = "ballerina"\nname = "ai"\n') == frozenset(
        {"ballerina/ai"}
    )


SDK_CSPROJ = """<Project Sdk="Microsoft.NET.Sdk">
  <ItemGroup>
    <PackageReference Include="Microsoft.SemanticKernel.Agents.Core" Version="1.60.0" />
    <PackageReference Include="Azure.Identity;Microsoft.Agents.AI" />
    <PackageReference Include="$(SharedPackage)" />
    <PackageReference Update="Ignored.Update" />
    <Using Include="Microsoft.SemanticKernel.Agents" />
    <Using Include="Microsoft.Agents.AI.ChatClientAgent" Alias="Agent" />
    <Using Include="System.Math" Static="True" />
    <Using Remove="System.Linq" />
  </ItemGroup>
</Project>
"""
LEGACY_CSPROJ = (
    '\ufeff<?xml version="1.0" encoding="utf-8"?>\n'
    '<Project ToolsVersion="15.0" xmlns="http://schemas.microsoft.com/developer/msbuild/2003">\n'
    '  <ItemGroup><PackageReference Include="Microsoft.SemanticKernel"><Version>1.0</Version></PackageReference>'
    "</ItemGroup>\n</Project>\n"
)


@pytest.mark.parametrize(
    ("data", "packages"),
    [
        (
            SDK_CSPROJ.encode(),
            {"microsoft-semantickernel-agents-core", "azure-identity", "microsoft-agents-ai"},
        ),
        (LEGACY_CSPROJ.encode(), {"microsoft-semantickernel"}),
        (
            b'<Project><ItemGroup><PackageVersion Include="Microsoft.Agents.AI.OpenAI" Version="1"/>'
            b"</ItemGroup></Project>",
            {"microsoft-agents-ai-openai"},
        ),
        (b'<packages><package id="Microsoft.SemanticKernel" version="1.0" /></packages>', {"microsoft-semantickernel"}),
        (b"<Project></Project>", set()),
    ],
)
def test_nuget_packages(data: bytes, packages: set[str]) -> None:
    assert nuget_packages(data) == frozenset(packages)


@pytest.mark.parametrize(
    "data",
    [
        # A DTD or entity is refused before parsing, so no entity is ever expanded.
        b'<?xml version="1.0"?>\n<!DOCTYPE lolz [<!ENTITY lol "lol"><!ENTITY lol2 "&lol;&lol;">]>\n'
        b'<Project><ItemGroup><PackageReference Include="&lol2;" /></ItemGroup></Project>',
        b'<!doctype Project SYSTEM "file:///etc/passwd"><Project/>',
        b"<Project><!ENTITY x></Project>",
        # Not UTF-8 (a DTD could hide in UTF-16), or not XML.
        '<?xml version="1.0" encoding="utf-16"?><Project/>'.encode("utf-16"),
        b"<Project><ItemGroup>",
        b"\xff\xfe",
        b"",
    ],
)
def test_unsafe_or_broken_msbuild_is_unreadable(data: bytes) -> None:
    assert nuget_packages(data) is None
    assert csproj_usings(data) == ()


def test_csproj_usings() -> None:
    usings = csproj_usings(SDK_CSPROJ.encode())
    assert [(u.module, u.local, u.kind, u.global_) for u in usings] == [
        ("Microsoft.SemanticKernel.Agents", None, ImportKind.NAMESPACE, True),
        ("Microsoft.Agents.AI.ChatClientAgent", "Agent", ImportKind.ALIAS, True),
        ("System.Math", None, ImportKind.STATIC, True),
    ]


@pytest.mark.parametrize(
    ("text", "packages"),
    [
        (
            '[package]\norg = "acme"\nname = "bot"\n[[dependency]]\norg = "ballerinax"\nname = "ai.agent"\n'
            '[[dependency]]\norg = "ballerina"\nname = "http"\n[[dependency]]\nname = "no-org"\n',
            {"ballerinax/ai-agent", "ballerina/http"},
        ),
        ('[package]\nname = "bot"\n', set()),
        ('dependency = "x"\n', set()),
        ("not = [toml", set()),
    ],
)
def test_ballerina_packages(text: str, packages: set[str]) -> None:
    assert ballerina_packages(text.encode()) == packages


def test_declaring_matches_a_package_prefix() -> None:
    dependencies = Dependencies(
        {
            "src/App/App.csproj": frozenset({normalise("Microsoft.SemanticKernel.Agents.Core")}),
            "src/Other/Other.csproj": frozenset({normalise("Microsoft.SemanticKernelX")}),
        }
    )
    wanted = ["Microsoft.SemanticKernel", "Microsoft.SemanticKernel.*"]
    assert dependencies.declaring(wanted, "src/App/Host.cs") == "src/App/App.csproj"
    assert dependencies.declaring(wanted, "src/Other/Host.cs") is None


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
        (["CrewAI"], "agent.py", "requirements.txt"),
        # Only the nearest manifest folder above the file counts, even when it does not declare the package.
        (["crewai"], "services/b/agent.py", None),
        (["strands-agents"], "agent.py", None),
        (["langchain", "langchain-classic"], "services/a/agent.py", None),
        (["smolagents"], "agent.py", None),
    ],
)
def test_declaring(packages: list[str], path: str, manifest: str | None) -> None:
    assert DEPENDENCIES.declaring(packages, path) == manifest


def test_declaring_falls_back_only_with_no_manifest_above() -> None:
    dependencies = Dependencies(
        {"services/b/requirements.txt": frozenset({"crewai"}), "services/a/pyproject.toml": frozenset({"crewai"})},
        unread=frozenset({"tools/package.json"}),
    )
    # tools/ has only an unread manifest, so no read manifest is above the file: the first by path wins.
    assert dependencies.declaring(["crewai"], "tools/agent.py") == "services/a/pyproject.toml"
    assert dependencies.paths == ("services/b/requirements.txt", "services/a/pyproject.toml", "tools/package.json")


def test_declaring_reads_every_manifest_in_the_nearest_folder() -> None:
    dependencies = Dependencies(
        {
            "svc/pyproject.toml": frozenset({"httpx"}),
            "svc/requirements.txt": frozenset({"crewai"}),
            "requirements.txt": frozenset({"crewai"}),
        }
    )
    assert dependencies.declaring(["crewai"], "svc/app/agent.py") == "svc/requirements.txt"


SERVICE_MANIFESTS = (
    "package.json",
    "services/api/pyproject.toml",
    "services/api/requirements.txt",
    "services/web/package.json",
    "services/api-v2/Api.csproj",
)


@pytest.mark.parametrize(
    ("path", "manifests", "folder"),
    [
        ("services/api/src/agent.py", SERVICE_MANIFESTS, "services/api"),
        ("services/api/agent.py", SERVICE_MANIFESTS, "services/api"),
        ("services/api-v2/Agent.cs", SERVICE_MANIFESTS, "services/api-v2"),
        ("services/apiary/agent.py", SERVICE_MANIFESTS, "."),
        ("agent.py", SERVICE_MANIFESTS, "."),
        ("services/api/agent.py", (), None),
        ("tools/agent.py", ("services/api/pyproject.toml",), None),
    ],
)
def test_service_folder(path: str, manifests: tuple[str, ...], folder: str | None) -> None:
    assert service_folder(path, manifests) == folder
