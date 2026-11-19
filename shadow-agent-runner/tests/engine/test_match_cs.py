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

"""amp_shadow_runner.engine.match_cs, match_bal and their signatures: C# and Ballerina agents."""

import time
from pathlib import Path
from typing import Any

import pytest

from amp_shadow_runner.engine.imports import extract_imports
from amp_shadow_runner.engine.languages import Language, parse
from amp_shadow_runner.engine.match import Match
from amp_shadow_runner.engine.match_bal import ballerina_imports, match_ballerina_file
from amp_shadow_runner.engine.match_cs import declared_types, match_csharp_file, mentions
from amp_shadow_runner.engine.scan import scan_tree

SK = "using Microsoft.SemanticKernel.Agents;\n"
SK_OPENAI = "using Microsoft.SemanticKernel.Agents.OpenAI;\n"
MAF = "using Microsoft.Agents.AI;\n"


def _matches(source: str) -> tuple[Match, ...]:
    data = source.encode()
    tree = parse(Language.C_SHARP, data)
    assert tree is not None
    return match_csharp_file("src/Agent.cs", tree, data, extract_imports(Language.C_SHARP, tree, data))


def _found(source: str) -> list[tuple[str, str, bool | None]]:
    return [(m.framework, m.name, m.condition_met) for m in _matches(source)]


def _write(root: Path, files: dict[str, str]) -> None:
    for rel, text in files.items():
        path = root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")


# One case per signature, with its namespace imported: (source, framework, condition_met). Each is named ``x``.
# Extension methods have no condition: without their using they are no match at all.
SIGNATURE_CASES = [
    (SK + 'var x = new ChatCompletionAgent { Instructions = "i", Kernel = kernel };', "semantic-kernel", True),
    (SK + "var x = new ChatCompletionAgent();", "semantic-kernel", True),
    (SK + "ChatCompletionAgent x = new() { Kernel = kernel };", "semantic-kernel", True),
    (SK_OPENAI + "var x = new OpenAIAssistantAgent(assistant, client);", "semantic-kernel", True),
    (
        SK_OPENAI + "var x = await OpenAIAssistantAgent.CreateAsync(client, definition, kernel);",
        "semantic-kernel",
        True,
    ),
    (SK_OPENAI + "var x = new OpenAIResponseAgent(client) { Kernel = kernel };", "semantic-kernel", True),
    ("using Microsoft.SemanticKernel.Agents.AzureAI;\nvar x = new AzureAIAgent(d, client);", "semantic-kernel", True),
    (MAF + 'var x = new ChatClientAgent(chatClient, instructions: "i");', "microsoft-agent-framework", True),
    (MAF + 'AIAgent x = client.GetChatClient(m).CreateAIAgent(instructions: "i");', "microsoft-agent-framework", None),
    (MAF + 'var x = chatClient.AsAIAgent(instructions: "i");', "microsoft-agent-framework", None),
    ("using Microsoft.Agents.AI.OpenAI;\nvar x = chatClient.AsAIAgent();", "microsoft-agent-framework", None),
]


@pytest.mark.parametrize(("source", "framework", "met"), SIGNATURE_CASES)
def test_signature_with_using_is_proven(source: str, framework: str, met: bool | None) -> None:
    assert _found(source) == [(framework, "x", met)]


@pytest.mark.parametrize(
    "source",
    [
        "using SKA = Microsoft.SemanticKernel.Agents;\nvar x = new SKA.ChatCompletionAgent();",
        "using CCA = Microsoft.SemanticKernel.Agents.ChatCompletionAgent;\nvar x = new CCA();",
        "var x = new Microsoft.SemanticKernel.Agents.ChatCompletionAgent();",
        "var x = new global::Microsoft.SemanticKernel.Agents.ChatCompletionAgent();",
        "namespace App { using Microsoft.SemanticKernel.Agents; class C { void M() { var x = new ChatCompletionAgent(); } } }",
        SK + "ChatCompletionAgent? x = new() { Kernel = k };",
    ],
)
def test_resolves_aliases_and_qualified_names(source: str) -> None:
    assert [(m.framework, m.condition_met) for m in _matches(source)] == [("semantic-kernel", True)]


@pytest.mark.parametrize(
    ("source", "framework"),
    [
        # The lookalike: the class name with no using that imports its namespace.
        ('var agent = new ChatCompletionAgent { Name = "Bot" };', "semantic-kernel"),
        ("using MyCompany.Agents;\nvar agent = new ChatCompletionAgent();", "semantic-kernel"),
        ("var agent = await OpenAIAssistantAgent.CreateAsync(c, d);", "semantic-kernel"),
        ("ChatClientAgent agent = new();", "microsoft-agent-framework"),
        ("using Microsoft.SemanticKernel;\nvar agent = new ChatClientAgent(c);", "microsoft-agent-framework"),
    ],
)
def test_lookalike_without_using_is_not_sure(source: str, framework: str) -> None:
    (match,) = _matches(source)
    assert (match.framework, match.condition_met) == (framework, False)


@pytest.mark.parametrize(
    "source",
    [
        # Qualified with another namespace: not the framework's class.
        "var x = new MyCompany.Agents.ChatCompletionAgent();",
        "using Mine = MyCompany.Agents;\nvar x = new Mine.ChatCompletionAgent();",
        # Extension methods without their namespace in scope: any class may have such a method.
        'var x = chatClient.CreateAIAgent(instructions: "i");',
        "using Microsoft.Agents;\nvar x = chatClient.AsAIAgent();",
        "using MAI = Microsoft.Agents.AI;\nvar x = chatClient.AsAIAgent();",
        # Orchestration and other classes.
        SK + "var chat = new AgentGroupChat(a, b);",
        SK + "var kernel = Kernel.CreateBuilder().Build();",
        "var x = new();",
        SK + "var x = this.ChatCompletionAgent();",
        SK + "var x = ChatCompletionAgent.Parse(s);",
        # A using imports a namespace's types, not the namespaces under it.
        "using Microsoft.SemanticKernel;\nvar x = new Agents.ChatCompletionAgent();",
        "using Microsoft.SemanticKernel.Agents;\nvar x = await OpenAI.OpenAIAssistantAgent.CreateAsync(c, d);",
    ],
)
def test_no_match(source: str) -> None:
    assert _matches(source) == ()


NAME_CASES = [
    (SK + 'var agent = new ChatCompletionAgent { Name = "Host" };', ("Host", "Host")),
    (SK + 'var agent = new ChatCompletionAgent { Name = @"Host" };', ("agent", "agent")),
    (SK + 'var agent = new ChatCompletionAgent { Name = "a\\tb" };', ("agent", "agent")),
    (SK + 'var agent = new ChatCompletionAgent { Name = $"{x}" };', ("agent", "agent")),
    (SK + "var agent = new ChatCompletionAgent { Name = AgentName };", ("agent", "agent")),
    (MAF + 'var agent = new ChatClientAgent(c, name: "Joker");', ("Joker", "Joker")),
    (MAF + 'var agent = c.CreateAIAgent(instructions: "i", name: "Joker");', ("Joker", "Joker")),
    (SK + "agent = new ChatCompletionAgent();", ("agent", "agent")),
    (SK + "Use(new ChatCompletionAgent());", ("semantic-kernel-agent", "semantic-kernel-agent")),
    (
        SK + "namespace App.Agents;\nclass Bot {\n  void Run() { this.agent = new ChatCompletionAgent(); }\n}",
        ("agent", "Bot.Run.agent"),
    ),
    (SK + "class Bot {\n  private readonly ChatCompletionAgent _agent = new();\n}", ("_agent", "Bot._agent")),
    (SK + "class Bot {\n  ChatCompletionAgent Agent { get; } = new() { };\n}", ("Agent", "Bot.Agent")),
    (SK + "class Bot {\n  ChatCompletionAgent Make() => new ChatCompletionAgent();\n}", ("Make", "Bot.Make")),
    (SK + "class Bot {\n  public Bot() { Use(new ChatCompletionAgent()); }\n}", ("Bot", "Bot.Bot")),
    (SK + "class Bot {\n  class Inner { void M() { Use(new ChatCompletionAgent()); } }\n}", ("M", "Bot.Inner.M")),
    (
        SK + "class Bot {\n  void M() { ChatCompletionAgent Local() => new ChatCompletionAgent(); }\n}",
        ("Local", "Bot.M.Local"),
    ),
    (SK + "record Bot(Kernel K) {\n  object A = Use(new ChatCompletionAgent());\n}", ("Bot", "Bot")),
]


@pytest.mark.parametrize(("source", "expected"), NAME_CASES)
def test_name_and_qualified_name(source: str, expected: tuple[str, str]) -> None:
    (match,) = _matches(source)
    assert (match.name, match.qualified) == expected


def test_instructions_are_never_read() -> None:
    (match,) = _matches(SK + 'var a = new ChatCompletionAgent { Instructions = "secret prompt" };')
    assert "secret prompt" not in repr(match)


def test_mentions() -> None:
    assert mentions(b"var a = new ChatCompletionAgent();")
    assert mentions(b"x.CreateAIAgent()")
    assert mentions(b"OpenAIAssistantAgent.CreateAsync")
    assert not mentions(b"var kernel = Kernel.CreateBuilder();")


CSPROJ = (
    '<Project Sdk="Microsoft.NET.Sdk">\n  <ItemGroup>\n'
    '    <PackageReference Include="Microsoft.SemanticKernel.Agents.Core" Version="1.60.0" />\n'
    "  </ItemGroup>\n</Project>\n"
)
LOOKALIKE = "namespace App;\nclass Host {\n  void Run() { var agent = new ChatCompletionAgent(); }\n}\n"


@pytest.mark.parametrize(
    ("files", "verdict"),
    [
        # The file's own using.
        ({"src/App/App.csproj": CSPROJ, "src/App/Host.cs": SK + LOOKALIKE}, "proven"),
        # A global using in another file of the same project.
        ({"src/App/App.csproj": CSPROJ, "src/App/Usings.cs": "global " + SK, "src/App/Host.cs": LOOKALIKE}, "proven"),
        (
            {"src/App/App.csproj": CSPROJ, "src/App/Sub/Usings.cs": "global " + SK, "src/App/Host.cs": LOOKALIKE},
            "proven",
        ),
        # A <Using Include> item of the project's csproj.
        (
            {
                "src/App/App.csproj": CSPROJ.replace(
                    "</ItemGroup>", '<Using Include="Microsoft.SemanticKernel.Agents" /></ItemGroup>'
                ),
                "src/App/Host.cs": LOOKALIKE,
            },
            "proven",
        ),
        (
            {
                "src/App/App.csproj": '<Project><ItemGroup><Using Include="Microsoft.SemanticKernel.Agents" Alias="A"/>'
                "</ItemGroup></Project>",
                "src/App/Host.cs": LOOKALIKE.replace("new ChatCompletionAgent", "new A.ChatCompletionAgent"),
            },
            "proven",
        ),
        # A global using of another project does not apply; nor does a plain using of another file.
        (
            {
                "src/App/App.csproj": CSPROJ,
                "src/Lib/Lib.csproj": CSPROJ,
                "src/Lib/Usings.cs": "global " + SK,
                "src/App/Host.cs": LOOKALIKE,
            },
            "not_sure",
        ),
        ({"src/App/App.csproj": CSPROJ, "src/App/Other.cs": SK, "src/App/Host.cs": LOOKALIKE}, "not_sure"),
        # With no csproj, the repository is the project.
        ({"Usings.cs": "global " + SK, "src/Host.cs": LOOKALIKE}, "proven"),
        ({"src/App/Host.cs": LOOKALIKE}, "not_sure"),
        # A global alias in another file: the file that uses only the alias is matched too.
        (
            {
                "src/App/App.csproj": CSPROJ,
                "src/App/Usings.cs": "global using CCA = Microsoft.SemanticKernel.Agents.ChatCompletionAgent;\n",
                "src/App/Host.cs": LOOKALIKE.replace("new ChatCompletionAgent", "new CCA"),
            },
            "proven",
        ),
        # The <Using> items of the nearest Directory.Build.props at or above the project, and only those.
        (
            {
                "Directory.Build.props": '<Project><ItemGroup><Using Include="Microsoft.SemanticKernel.Agents" />'
                "</ItemGroup></Project>",
                "src/App/App.csproj": CSPROJ,
                "src/App/Host.cs": LOOKALIKE,
            },
            "proven",
        ),
        (
            {
                "src/Directory.Build.props": '<Project><ItemGroup><Using Alias="CCA" '
                'Include="Microsoft.SemanticKernel.Agents.ChatCompletionAgent" /></ItemGroup></Project>',
                "src/App/App.csproj": CSPROJ,
                "src/App/Host.cs": LOOKALIKE.replace("new ChatCompletionAgent", "new CCA"),
            },
            "proven",
        ),
        (
            {
                "Directory.Build.props": '<Project><ItemGroup><Using Include="Microsoft.SemanticKernel.Agents" />'
                "</ItemGroup></Project>",
                "src/Directory.Build.props": "<Project />",
                "src/App/App.csproj": CSPROJ,
                "src/App/Host.cs": LOOKALIKE,
            },
            "not_sure",
        ),
        # A type of that name declared in the project may be the one meant.
        (
            {
                "src/App/App.csproj": CSPROJ,
                "src/App/Host.cs": SK + LOOKALIKE,
                "src/App/Fakes.cs": "namespace App;\ninternal sealed class ChatCompletionAgent { }\n",
            },
            "not_sure",
        ),
        # A csproj with a DOCTYPE is refused: no usings, and the scan goes on.
        (
            {
                "src/App/App.csproj": '<?xml version="1.0"?>\n<!DOCTYPE p [<!ENTITY a "Microsoft.SemanticKernel.Agents">]>'
                '\n<Project><ItemGroup><Using Include="&a;" /></ItemGroup></Project>',
                "src/App/Host.cs": LOOKALIKE,
            },
            "not_sure",
        ),
    ],
)
def test_project_usings(tmp_path: Path, files: dict[str, str], verdict: str) -> None:
    _write(tmp_path, files)
    (finding,) = scan_tree(tmp_path, repository="r").findings
    assert finding.path.endswith("Host.cs")
    assert (finding.external_id, finding.framework, finding.verdict) == (
        f"r:{finding.path}:Host.Run.agent",
        "semantic-kernel",
        verdict,
    )


def test_scan_tree_classifies_csharp(tmp_path: Path) -> None:
    _write(
        tmp_path,
        {
            "Directory.Packages.props": '<Project><ItemGroup><PackageVersion Include="Microsoft.Agents.AI.OpenAI" '
            'Version="1.0.0" /></ItemGroup></Project>',
            "src/App/App.csproj": CSPROJ,
            "src/App/Host.cs": SK + LOOKALIKE,
            "tools/Joker.cs": MAF + 'var joker = client.CreateAIAgent(name: "Joker");\n',
            "src/App.Tests/HostTests.cs": SK + "class HostTests { void T() { var a = new ChatCompletionAgent(); } }",
            "src/App/obj/Generated.cs": SK + "var g = new ChatCompletionAgent();",
        },
    )
    result = scan_tree(tmp_path, repository="r")
    findings = {f.external_id: f for f in result.findings}
    assert list(findings) == [
        "r:src/App.Tests/HostTests.cs:HostTests.T.a",
        "r:src/App/Host.cs:Host.Run.agent",
        "r:tools/Joker.cs:Joker",
    ]
    host = findings["r:src/App/Host.cs:Host.Run.agent"]
    assert (host.verdict, host.attributes, host.service_folder) == (
        "proven",
        {"framework": "semantic-kernel"},
        "src/App",
    )
    assert [(e.code, e.source, dict(e.observed)) for e in host.evidence] == [
        ("d1_agent_dependency", "src/App/App.csproj", {"declared": True}),
        ("d2_agent_import", "src/App/Host.cs", {"imports": 1}),
    ]
    joker = findings["r:tools/Joker.cs:Joker"]
    assert (joker.verdict, joker.framework) == ("proven", "microsoft-agent-framework")
    assert [(e.code, e.source) for e in joker.evidence] == [
        ("d1_agent_dependency", "Directory.Packages.props"),
        ("d2_agent_import", "tools/Joker.cs"),
    ]
    assert findings["r:src/App.Tests/HostTests.cs:HostTests.T.a"].tag == "test"
    assert (result.files_scanned, result.parse_errors) == (3, 0)


# Ballerina


BAL_CASES = [
    (
        "import ballerina/ai;\nfinal ai:Agent supportAgent = check new (systemPrompt = {}, model = m, tools = []);\n",
        [("supportAgent", 2)],
    ),
    ("import ballerinax/ai;\nai:Agent bot = new (x);\n", [("bot", 2)]),
    ("import ballerina/ai;\n\nfunction f() returns error? {\n    var a = check new ai:Agent(x);\n}\n", [("a", 4)]),
    ("import ballerina/ai;\nfinal ai:Agent a = check new ai:Agent(x);\n", [("a", 2)]),
    ("import ballerina/ai;\nUse(check new ai:Agent(x));\n", [("ballerina-agent", 2)]),
    ("import ballerina/ai as llm;\nfinal llm:Agent helper = check new (x);\n", [("helper", 2)]),
    ("import ballerinax/ai.agent;\nagent:FunctionCallAgent fc = check new (model, tools);\n", [("fc", 2)]),
    ("import ballerinax/ai.agent;\nagent:ReActAgent r = check new (model, tools);\n", [("r", 2)]),
    # No import of the module, another prefix, a comment, or no construction: no match.
    ("final ai:Agent a = check new (x);\n", []),
    ("import ballerina/ai as llm;\nfinal ai:Agent a = check new (x);\n", []),
    ("import ballerina/ai;\n// final ai:Agent a = check new (x);\n", []),
    ("import ballerina/ai;\nfunction run(ai:Agent agent) returns string {\n    return agent.run(q);\n}\n", []),
    ("import ballerina/ai;\nfinal ai:Agent a = make();\n", []),
    ("import ballerina/http;\nfinal http:Client c = check new (url);\n", []),
    ("import ballerina/ai;\nfinal myai:Agent a = check new (x);\n", []),
]


@pytest.mark.parametrize(("text", "expected"), BAL_CASES)
def test_ballerina_text_match(text: str, expected: list[tuple[str, int]]) -> None:
    matches, _ = match_ballerina_file("main.bal", text)
    assert [(m.name, m.line) for m in matches] == expected
    for m in matches:
        assert (m.framework, m.qualified, m.condition_met) == ("ballerina-ai", m.name, None)


def test_ballerina_imports() -> None:
    table = ballerina_imports("import ballerina/ai;\nimport ballerinax/ai.agent as a;\n  import ballerina/http;\n")
    assert [(i.module, i.local, i.line) for i in table.imports] == [
        ("ballerina/ai", "ai", 1),
        ("ballerinax/ai.agent", "a", 2),
        ("ballerina/http", "http", 3),
    ]


def test_ballerina_is_always_not_sure(tmp_path: Path) -> None:
    _write(
        tmp_path,
        {
            "Ballerina.toml": '[package]\norg = "acme"\nname = "support"\n\n[[dependency]]\norg = "ballerina"\n'
            'name = "ai"\nversion = "1.0.0"\n',
            "agents.bal": "import ballerina/ai;\nfinal ai:Agent supportAgent = check new (model = m);\n",
        },
    )
    result = scan_tree(tmp_path, repository="r")
    (finding,) = result.findings
    assert (finding.external_id, finding.framework, finding.verdict) == (
        "r:agents.bal:supportAgent",
        "ballerina-ai",
        "not_sure",
    )
    assert [(e.code, e.source, dict(e.observed)) for e in finding.evidence] == [
        ("d1_agent_dependency", "Ballerina.toml", {"declared": True}),
        ("d2_agent_import", "agents.bal", {"imports": 1}),
    ]
    assert (result.files_scanned, result.parse_errors) == (1, 0)


@pytest.mark.parametrize(
    "declaration",
    [
        "class ChatCompletionAgent { }",
        "struct ChatCompletionAgent { }",
        "record ChatCompletionAgent(int A);",
        "interface ChatCompletionAgent { }",
        "enum ChatCompletionAgent { A }",
        "delegate void ChatCompletionAgent();",
    ],
)
def test_a_type_declared_in_the_project_makes_it_not_sure(declaration: str) -> None:
    source = SK + "namespace MyApp { " + declaration + " class P { void M() { var x = new ChatCompletionAgent(); } } }"
    data = source.encode()
    tree = parse(Language.C_SHARP, data)
    assert tree is not None
    assert declared_types(tree, data) >= {"ChatCompletionAgent"}
    imports = extract_imports(Language.C_SHARP, tree, data)
    (match,) = match_csharp_file("a.cs", tree, data, imports, local_types=declared_types(tree, data))
    assert (match.framework, match.condition_met) == ("semantic-kernel", False)
    # A fully qualified name is not shadowed by it.
    qualified = SK + "class ChatCompletionAgent { }\nvar x = new Microsoft.SemanticKernel.Agents.ChatCompletionAgent();"
    (match,) = match_csharp_file("a.cs", *_parsed(qualified), local_types=frozenset({"ChatCompletionAgent"}))
    assert match.condition_met is True


def _parsed(source: str) -> tuple[Any, bytes, Any]:
    data = source.encode()
    tree = parse(Language.C_SHARP, data)
    assert tree is not None
    return tree, data, extract_imports(Language.C_SHARP, tree, data)


@pytest.mark.parametrize("parts", [1000, 5000])
def test_a_long_qualified_name_is_read_without_recursion(parts: int) -> None:
    assert _matches("class P { void M() { var y = foo" + ".x" * parts + "(); var z = ChatClientAgent.X; } }") == ()
    assert _matches("class P { void M() { var y = new " + "A." * parts + "ChatCompletionAgent(); } }") == ()


def test_a_name_literal_that_is_not_utf8_is_no_name() -> None:
    data = MAF.encode() + b'var writer = new ChatClientAgent(c, name: "W\xffriter");\n'
    tree = parse(Language.C_SHARP, data)
    assert tree is not None
    (match,) = match_csharp_file("a.cs", tree, data, extract_imports(Language.C_SHARP, tree, data))
    assert (match.name, match.named) == ("writer", False)


def test_ballerina_lines_and_a_byte_order_mark(tmp_path: Path) -> None:
    text = "import ballerina/ai;\n\nfinal ai:Agent a = check new (m);\n\n\nai:Agent b = new (m);\nx = new ai:Agent();\n"
    matches, imports = match_ballerina_file("main.bal", "import ballerina/http;\n" + text)
    assert [i.line for i in imports.imports] == [1, 2]
    assert [(m.name, m.line) for m in matches] == [("a", 4), ("b", 7), ("x", 8)]
    (tmp_path / "main.bal").write_bytes(b"\xef\xbb\xbf" + text.encode())
    findings = scan_tree(tmp_path, repository="r").findings
    assert [(f.external_id, f.line) for f in findings] == [
        ("r:main.bal:a", 3),
        ("r:main.bal:b", 6),
        ("r:main.bal:x", 7),
    ]


def test_ballerina_line_numbers_are_counted_once() -> None:
    # Counting from the file's start for every match took about 7 s here; counting on takes milliseconds.
    started = time.monotonic()
    matches, _ = match_ballerina_file("m.bal", "import ballerina/ai;\n" + "x = new ai:Agent()\n" * 50_000)
    assert (len(matches), matches[-1].line) == (50_000, 50_001)
    assert time.monotonic() - started < 2


# One agent written once in each branch of a conditional: only the first is kept. (body, its line)
ONE_AGENT_IN_BRANCHES = [
    ("if (fast) { graph = new ChatCompletionAgent(); } else { graph = new ChatCompletionAgent(); }", 3),
    (
        "if (a) graph = new ChatCompletionAgent();\nelse if (b) graph = new ChatCompletionAgent();\n"
        "else graph = new ChatCompletionAgent();",
        3,
    ),
    (
        "try { graph = new ChatCompletionAgent(); }\ncatch (IOException) { graph = new ChatCompletionAgent(); }\n"
        "catch { graph = new ChatCompletionAgent(); }",
        3,
    ),
    ("var graph = fast ? new ChatCompletionAgent() : new ChatCompletionAgent();", 3),
    ("graph = await (fast ? OpenAIAssistantAgent.CreateAsync(c) : OpenAIAssistantAgent.CreateAsync(d));", 3),
    (
        "switch (mode) {\ncase 1: graph = new ChatCompletionAgent(); break;\n"
        "default: graph = new ChatCompletionAgent(); break;\n}",
        4,
    ),
    ("if (fast) { this.graph = new ChatCompletionAgent(); } else { this.graph = new ChatCompletionAgent(); }", 3),
]


@pytest.mark.parametrize(("body", "line"), ONE_AGENT_IN_BRANCHES)
def test_one_variable_in_exclusive_branches_is_one_agent(body: str, line: int) -> None:
    (match,) = _matches(f"{SK}class Bot {{ async Task M() {{\n{body}\n}} }}\n")
    assert (match.name, match.qualified, match.line) == ("graph", "Bot.M.graph", line)


# Still two agents: a reassignment in one block, two scopes, ``try`` and ``finally`` (both run), ``Name``
# literals, no variable at all, or two in one path.
TWO_AGENTS = [
    "void M() { var graph = new ChatCompletionAgent(); graph = new ChatCompletionAgent(); }",
    "void M() { if (fast) { graph = new ChatCompletionAgent(); graph = new ChatCompletionAgent(); } }",
    "void A() { var graph = new ChatCompletionAgent(); }\nvoid B() { var graph = new ChatCompletionAgent(); }",
    "void M() { try { graph = new ChatCompletionAgent(); } finally { graph = new ChatCompletionAgent(); } }",
    'void M() { if (f) { new ChatCompletionAgent { Name = "A" }; } else { new ChatCompletionAgent { Name = "B" }; } }',
    "void M() { if (fast) { Use(new ChatCompletionAgent()); } else { Use(new ChatCompletionAgent()); } }",
    "void M() { if (a) { g = new ChatCompletionAgent(); }\n"
    "else { g = new ChatCompletionAgent(); g = new ChatCompletionAgent(); } }",
]


@pytest.mark.parametrize("members", TWO_AGENTS)
def test_two_agents_stay_two(members: str) -> None:
    assert len(_matches(f"{SK}class Bot {{\n{members}\n}}\n")) == 2
