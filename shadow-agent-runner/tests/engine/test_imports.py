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

"""amp_shadow_runner.engine.imports: Python, TS/JS and C# imports, and resolving aliases through them."""

import pytest

from amp_shadow_runner.engine.imports import Import, ImportKind, ImportTable, Symbol, extract_imports, package_root
from amp_shadow_runner.engine.languages import Language, parse

S = Symbol
SCRIPT_LANGUAGES = [Language.JAVASCRIPT, Language.TYPESCRIPT, Language.TSX]


def _table(language: Language, source: str) -> ImportTable:
    data = source.encode()
    tree = parse(language, data)
    assert tree is not None
    return extract_imports(language, tree, data)


PYTHON_CASES = [
    ("import a", "a", (S("a", None),)),
    ("import a", "a.B", (S("a", "B"),)),
    ("import a.b", "a.b", (S("a.b", None),)),
    ("import a.b", "a.b.C", (S("a.b", "C"),)),
    ("import a.b", "a.C", (S("a", "C"),)),
    ("import a.b\nimport a.c", "a.c.D", (S("a.c", "D"),)),
    ("import a . b", "a.b.C", (S("a.b", "C"),)),
    ("import a.b as c", "c", (S("a.b", None),)),
    ("import a.b as c", "c.D", (S("a.b", "D"),)),
    ("import a.b as c", "c.x.D", (S("a.b.x", "D"),)),
    ("import a.b as c", "a.b.D", ()),
    ("import x, a.b as c", "x.Y", (S("x", "Y"),)),
    ("from a.b import C", "C", (S("a.b", "C"),)),
    ("from a.b import C", "C.create", (S("a.b.C", "create"),)),
    ("from a import b", "b.C", (S("a.b", "C"),)),
    ("from a.b import C as D", "D", (S("a.b", "C"),)),
    ("from a.b import C as D", "C", ()),
    ("from a import (\n    x,\n    y as z,\n)", "x", (S("a", "x"),)),
    ("from a import (\n    x,\n    y as z,\n)", "z", (S("a", "y"),)),
    ("from a import *", "Agent", (S("a", "Agent"),)),
    ("from a import *\nfrom b import *", "Agent", (S("a", "Agent"), S("b", "Agent"))),
    ("from a import *\nfrom b import Agent", "Agent", (S("b", "Agent"),)),
    ("from a import *", "Agent.run", (S("a.Agent", "run"),)),
    ("from . import x", "x", (S(".", "x"),)),
    ("from . import x", "x.y", (S(".x", "y"),)),
    ("from .tools import y", "y", (S(".tools", "y"),)),
    ("from ..pkg.mod import z", "z", (S("..pkg.mod", "z"),)),
    ("from ... import z as w", "w", (S("...", "z"),)),
    ("from . import *", "q", (S(".", "q"),)),
    ("from __future__ import annotations", "annotations", ()),
    ("from a import Agent\nfrom b import Agent", "Agent", (S("a", "Agent"), S("b", "Agent"))),
    ("from a import Agent\nfrom a import Agent", "Agent", (S("a", "Agent"),)),
    ("import b as Agent\nfrom a import Agent", "Agent", (S("b", None), S("a", "Agent"))),
    (
        "try:\n    from llama_index.core.agent import ReActAgent\n"
        "except ImportError:\n    from llama_index.agent import ReActAgent\n",
        "ReActAgent",
        (S("llama_index.core.agent", "ReActAgent"), S("llama_index.agent", "ReActAgent")),
    ),
    ("from typing import TYPE_CHECKING\nif TYPE_CHECKING:\n    from strands import Agent\n", "Agent", ()),
    ("import typing\nif typing.TYPE_CHECKING:\n    import strands\n", "strands.Agent", ()),
    ("if TYPE_CHECKING:\n    def f():\n        from strands import Agent\n", "Agent", ()),
    ("if TYPE_CHECKING:\n    pass\nelse:\n    from strands import Agent\n", "Agent", (S("strands", "Agent"),)),
    ("if TYPE_CHECKING:\n    pass\nelif x:\n    from strands import Agent\n", "Agent", (S("strands", "Agent"),)),
    ("if not TYPE_CHECKING:\n    from strands import Agent\n", "Agent", (S("strands", "Agent"),)),
    (
        "if TYPE_CHECKING:\n    from a import Agent\nelse:\n    from b import Agent\n",
        "Agent",
        (S("b", "Agent"),),
    ),
    ("if TYPE_CHECKING:\n    from a import Agent\nfrom b import *\n", "Agent", (S("b", "Agent"),)),
    ("def build():\n    from strands import Agent\n    return Agent()", "Agent", (S("strands", "Agent"),)),
    ("try:\n    import a\nexcept ImportError:\n    a = None", "a.B", (S("a", "B"),)),
    ("from strands import Agent\n", "Tool", ()),
    ("class Agent:\n    pass\n", "Agent", ()),
    ("import a", "a..B", ()),
    ("import a", "", ()),
]


@pytest.mark.parametrize(("source", "reference", "expected"), PYTHON_CASES)
def test_python_resolve(source: str, reference: str, expected: tuple[Symbol, ...]) -> None:
    assert _table(Language.PYTHON, source).resolve(reference) == expected


SCRIPT_CASES = [
    ('import { Agent } from "@mastra/core";', "Agent", (S("@mastra/core", "Agent"),)),
    ('import { Agent } from "@mastra/core";', "Agent.create", (S("@mastra/core", "Agent.create"),)),
    ('import { Agent as A } from "@mastra/core";', "A", (S("@mastra/core", "Agent"),)),
    ('import { Agent as A } from "@mastra/core";', "Agent", ()),
    ('import { "an-agent" as A } from "m";', "A", (S("m", "an-agent"),)),
    ('import * as m from "@openai/agents";', "m", (S("@openai/agents", None),)),
    ('import * as m from "@openai/agents";', "m.Agent", (S("@openai/agents", "Agent"),)),
    ('import * as m from "./lib.js";', "m.a.b", (S("./lib.js", "a.b"),)),
    ('import ai from "ai";', "ai", (S("ai", "default"),)),
    ('import ai from "ai";', "ai.generateText", (S("ai", "default.generateText"),)),
    ('import { default as ai } from "ai";', "ai", (S("ai", "default"),)),
    ('import x, { y } from "m";', "x", (S("m", "default"),)),
    ('import x, { y } from "m";', "y", (S("m", "y"),)),
    ('import x, * as ns from "m";', "ns.z", (S("m", "z"),)),
    ('import { StateGraph } from "@langchain/langgraph"', "StateGraph", (S("@langchain/langgraph", "StateGraph"),)),
    ("import { tool } from 'ai';", "tool", (S("ai", "tool"),)),
    ('import "dotenv/config";', "config", ()),
    ('const m = require("langchain");', "m.Agent", (S("langchain", "Agent"),)),
    ('var m = require("langchain");', "m", (S("langchain", None),)),
    ("let m = require(`langchain`);", "m.x", (S("langchain", "x"),)),
    ('const { Agent, Tool: T } = require("m");', "Agent", (S("m", "Agent"),)),
    ('const { Agent, Tool: T } = require("m");', "T", (S("m", "Tool"),)),
    ('const { Agent, Tool: T } = require("m");', "Tool", ()),
    ('const { Agent, ...rest } = require("m");', "rest", ()),
    ('const m = require("m").sub;', "m", ()),
    ('const { ["Agent"]: A } = require("m");', "A", ()),
    (
        'async function f() { const { Agent, run } = await import("@openai/agents"); }',
        "Agent",
        (S("@openai/agents", "Agent"),),
    ),
    (
        'async function f() { const { Agent, run } = await import("@openai/agents"); }',
        "run",
        (S("@openai/agents", "run"),),
    ),
    ('const m = await import("m");', "m.Agent", (S("m", "Agent"),)),
    ("const m = await import(name);", "m", ()),
    ('const m = import("m");', "m.Agent", ()),
    ('const m = await load("m");', "m", ()),
    ("const m = require(name);", "m", ()),
    ('const m = load("m");', "m", ()),
    ('function f() { const { Agent } = require("m"); }', "Agent", (S("m", "Agent"),)),
    ('export { Agent } from "m";', "Agent", ()),
    ('export * from "m";', "Agent", ()),
    ('import { Agent } from "a";\nimport { Agent } from "b";', "Agent", (S("b", "Agent"),)),
    ('import { Agent } from "@mastra/core";', "Workflow", ()),
    ("class Agent {}", "Agent", ()),
]


@pytest.mark.parametrize("language", SCRIPT_LANGUAGES, ids=str)
@pytest.mark.parametrize(("source", "reference", "expected"), SCRIPT_CASES)
def test_script_resolve(language: Language, source: str, reference: str, expected: tuple[Symbol, ...]) -> None:
    assert _table(language, source).resolve(reference) == expected


TYPESCRIPT_CASES = [
    ('import type { Agent } from "@mastra/core";', "Agent", ()),
    ('import { type Agent, Mastra } from "@mastra/core";', "Agent", ()),
    ('import { type Agent, Mastra } from "@mastra/core";', "Mastra", (S("@mastra/core", "Mastra"),)),
    ('import type * as m from "@mastra/core";', "m.Agent", ()),
    ('import fs = require("fs");', "fs.readFile", (S("fs", "readFile"),)),
    ('const { Agent } = require("@openai/agents");', "Agent", (S("@openai/agents", "Agent"),)),
]


@pytest.mark.parametrize("language", [Language.TYPESCRIPT, Language.TSX], ids=str)
@pytest.mark.parametrize(("source", "reference", "expected"), TYPESCRIPT_CASES)
def test_typescript_resolve(language: Language, source: str, reference: str, expected: tuple[Symbol, ...]) -> None:
    assert _table(language, source).resolve(reference) == expected


@pytest.mark.parametrize(
    ("specifier", "root"),
    [
        ("@mastra/core/agent", "@mastra/core"),
        ("@mastra/core", "@mastra/core"),
        ("langchain/agents", "langchain"),
        ("ai", "ai"),
        ("./agent", "."),
        ("@scope", "@scope"),
    ],
)
def test_package_root(specifier: str, root: str) -> None:
    assert package_root(specifier) == root


SK = "Microsoft.SemanticKernel"

C_SHARP_CASES = [
    (f"using {SK};", "Kernel", (S(SK, "Kernel"),)),
    # A using imports types, not the namespaces under it: a dotted name is only taken as fully qualified.
    (f"using {SK};", "Kernel.CreateBuilder", (S("Kernel", "CreateBuilder"),)),
    (f"using {SK};", "Agents.ChatCompletionAgent", (S("Agents", "ChatCompletionAgent"),)),
    ("using static System.Math;", "Math.Max", (S("Math", "Max"),)),
    (f"global using {SK};", "Kernel", (S(SK, "Kernel"),)),
    (f"using global::{SK};", "Kernel", (S(SK, "Kernel"),)),
    (f"using SK = {SK};", "SK.Kernel", (S(SK, "Kernel"),)),
    (f"using SK = {SK};", "Kernel", ()),
    ("using K = A.Kernel;\nusing K = B.Kernel;", "K", (S("B", "Kernel"),)),
    (f"using K = {SK}.Kernel;", "K", (S(SK, "Kernel"),)),
    (f"using K = {SK}.Kernel;", "K.CreateBuilder", (S(f"{SK}.Kernel", "CreateBuilder"),)),
    ("using L = System.Collections.Generic.List<int>;", "L", (S("System.Collections.Generic", "List"),)),
    ("using P = (int, int);", "P", ()),
    ("using static System.Math;", "Max", (S("System.Math", "Max"),)),
    ("using A;\nusing B;", "Agent", (S("A", "Agent"), S("B", "Agent"))),
    ("using A;\nusing A;", "Agent", (S("A", "Agent"),)),
    ("class C {}", f"{SK}.Kernel", (S(SK, "Kernel"),)),
    ("class C {}", f"global::{SK}.Kernel", (S(SK, "Kernel"),)),
    ("class C {}", "Kernel", ()),
    (f"namespace App;\nusing {SK};\nclass C {{}}", "Kernel", (S(SK, "Kernel"),)),
    (f"namespace App {{\n    using {SK};\n    class C {{}}\n}}", "Kernel", (S(SK, "Kernel"),)),
    (f"using SK = {SK};\nusing Other;", "SK.Kernel", (S(SK, "Kernel"),)),
    ("class C { void M() { using (var k = new Kernel()) {} } }", "Kernel", ()),
    ("class C { void M() { using var k = new Kernel(); } }", "Kernel", ()),
]


@pytest.mark.parametrize(("source", "reference", "expected"), C_SHARP_CASES)
def test_c_sharp_resolve(source: str, reference: str, expected: tuple[Symbol, ...]) -> None:
    assert _table(Language.C_SHARP, source).resolve(reference) == expected


def test_python_records() -> None:
    table = _table(
        Language.PYTHON,
        "from __future__ import annotations\nimport a.b, c as d\nfrom ..pkg import (\n    x,\n    y as z,\n)\n"
        "from m import *\n",
    )
    assert table.imports == (
        Import("a.b", None, "a", ImportKind.MODULE, 2),
        Import("c", None, "d", ImportKind.ALIAS, 2),
        Import("pkg", "x", "x", ImportKind.MEMBER, 4, level=2),
        Import("pkg", "y", "z", ImportKind.MEMBER, 5, level=2),
        Import("m", None, None, ImportKind.WILDCARD, 7),
    )


@pytest.mark.parametrize("language", [Language.TYPESCRIPT, Language.TSX], ids=str)
def test_typescript_records(language: Language) -> None:
    table = _table(
        language,
        'import "side";\nimport type D from "t";\nimport x, { type T, y as Y } from "m";\n'
        'const { a, b: B } = require("r");\nexport { e } from "e";\n',
    )
    assert table.imports == (
        Import("side", None, None, ImportKind.SIDE_EFFECT, 1),
        Import("t", "default", "D", ImportKind.DEFAULT, 2, type_only=True),
        Import("m", "default", "x", ImportKind.DEFAULT, 3),
        Import("m", "T", "T", ImportKind.MEMBER, 3, type_only=True),
        Import("m", "y", "Y", ImportKind.MEMBER, 3),
        Import("r", "a", "a", ImportKind.MEMBER, 4),
        Import("r", "b", "B", ImportKind.MEMBER, 4),
    )


def test_c_sharp_records() -> None:
    table = _table(
        Language.C_SHARP,
        f"global using {SK};\nusing SK = {SK};\nusing static System.Math;\nusing X;\n"
        "global using static System.Console;\nglobal using G = System.Text;\nusing global::System.IO;\n"
        "class C { void M() { using (var x = Make()) {} } }\n",
    )
    assert table.imports == (
        Import(SK, None, None, ImportKind.NAMESPACE, 1, global_=True),
        Import(SK, None, "SK", ImportKind.ALIAS, 2),
        Import("System.Math", None, None, ImportKind.STATIC, 3),
        Import("X", None, None, ImportKind.NAMESPACE, 4),
        Import("System.Console", None, None, ImportKind.STATIC, 5, global_=True),
        Import("System.Text", None, "G", ImportKind.ALIAS, 6, global_=True),
        Import("System.IO", None, None, ImportKind.NAMESPACE, 7),
    )


def test_python_type_checking_records() -> None:
    table = _table(
        Language.PYTHON,
        "import typing\nif typing.TYPE_CHECKING:\n    from a import B\nelse:\n    from c import D\n",
    )
    assert table.imports == (
        Import("typing", None, "typing", ImportKind.MODULE, 1),
        Import("a", "B", "B", ImportKind.MEMBER, 3, type_only=True),
        Import("c", "D", "D", ImportKind.MEMBER, 5),
    )


def test_broken_file_still_gives_imports() -> None:
    assert _table(Language.PYTHON, "from strands import Agent\ndef broken(:\n").resolve("Agent") == (
        S("strands", "Agent"),
    )


def test_ballerina_is_not_supported() -> None:
    tree = parse(Language.PYTHON, b"")
    assert tree is not None
    with pytest.raises(ValueError, match="Ballerina"):
        extract_imports(Language.BALLERINA, tree, b"import ballerina/http;\n")
