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

"""amp_shadow_runner.engine.match_ts and signatures.typescript: TS/JS agent signatures, names and facts."""

from pathlib import Path

import pytest

from amp_shadow_runner.engine.imports import extract_imports
from amp_shadow_runner.engine.languages import Language, parse
from amp_shadow_runner.engine.match import MAX_TOOLS, Match
from amp_shadow_runner.engine.match_ts import ScriptMatches, match_script_file
from amp_shadow_runner.engine.scan import scan_tree
from amp_shadow_runner.engine.signatures import Wiring


def _result(source: str, language: Language = Language.TYPESCRIPT) -> ScriptMatches:
    data = source.encode()
    tree = parse(language, data)
    assert tree is not None
    return match_script_file("src/agent.ts", tree, data, extract_imports(language, tree, data))


def _matches(source: str, language: Language = Language.TYPESCRIPT) -> tuple[Match, ...]:
    return _result(source, language).matches


def _found(source: str) -> list[tuple[str, int, str]]:
    return [(m.framework, m.line, m.name) for m in _matches(source)]


def _write(root: Path, files: dict[str, str]) -> None:
    for rel, text in files.items():
        path = root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")


# One case per always-proven signature, written in TypeScript: (source, framework).
SIGNATURE_CASES = [
    ('import { Agent } from "@mastra/core/agent";\nconst x = new Agent({ instructions: "i", model });', "mastra"),
    ('import { Agent } from "@mastra/core";\nconst x = new Agent({ model });', "mastra"),
    ('import { Agent } from "@openai/agents";\nconst x = new Agent({ instructions: "i" });', "openai-agents"),
    ('import { Agent } from "@openai/agents";\nconst x = Agent.create({ handoffs: [a, b] });', "openai-agents"),
    ('import { Agent } from "@openai/agents-core";\nconst x = new Agent({});', "openai-agents"),
    ('import { Experimental_Agent as Agent } from "ai";\nconst x = new Agent({ model, tools });', "vercel-ai-sdk"),
    ('import { ToolLoopAgent } from "ai";\nconst x = new ToolLoopAgent({ model, tools });', "vercel-ai-sdk"),
    (
        'import { createReactAgent } from "@langchain/langgraph/prebuilt";\nconst x = createReactAgent({ llm, tools });',
        "langgraph",
    ),
    ('import { createAgent } from "langchain";\nconst x = createAgent({ model: "gpt-4o", tools });', "langchain"),
    (
        'import { createReactAgent } from "langchain/agents";\nconst x = await createReactAgent({ llm, tools, prompt });',
        "langchain",
    ),
    (
        'import { createToolCallingAgent } from "langchain/agents";\nconst x = await createToolCallingAgent({ llm });',
        "langchain",
    ),
    (
        'import { createOpenAIToolsAgent } from "langchain/agents";\nconst x = await createOpenAIToolsAgent({ llm });',
        "langchain",
    ),
    (
        'import { AgentExecutor } from "langchain/agents";\nconst x = new AgentExecutor({ agent: a, tools });',
        "langchain",
    ),
    (
        'import { AgentExecutor } from "langchain/agents";\nconst x = AgentExecutor.fromAgentAndTools({ agent: a });',
        "langchain",
    ),
]


@pytest.mark.parametrize(("source", "framework"), SIGNATURE_CASES)
def test_signature(source: str, framework: str) -> None:
    (match,) = _matches(source)
    assert (match.framework, match.line, match.name, match.condition_met) == (framework, 2, "x", None)


@pytest.mark.parametrize(
    "source",
    [
        'import { Agent as MastraAgent } from "@mastra/core/agent";\nconst x = new MastraAgent({});',
        'import * as mastra from "@mastra/core/agent";\nconst x = new mastra.Agent({});',
        'import mastra = require("@mastra/core");\nconst x = new mastra.Agent({});',
        'const { Agent } = require("@mastra/core");\nconst x = new Agent({});',
        'const { Agent: A } = require("@mastra/core/agent");\nconst x = new A({});',
        'const m = require("@mastra/core");\nconst x = new m.Agent({});',
        'const { Agent } = await import("@mastra/core/agent");\nconst x = new Agent({});',
        'import { Agent } from "@mastra/core/agent";\nconst x = new Agent<Ctx>({});',
    ],
)
def test_resolves_through_imports(source: str) -> None:
    assert [(m.framework, m.name) for m in _matches(source)] == [("mastra", "x")]


@pytest.mark.parametrize(
    "source",
    [
        # A same-named local class or function, with no import, never matches.
        "class Agent {}\nconst x = new Agent({});",
        "function createReactAgent() {}\nconst x = createReactAgent({});",
        'import { Agent } from "./agent";\nconst x = new Agent({});',
        'import { Agent } from "my-agents";\nconst x = new Agent({});',
        # A type-only import is no evidence of running code.
        'import type { Agent } from "@mastra/core/agent";\nconst x = new Agent({});',
        'import { type Agent } from "@mastra/core/agent";\nconst x = new Agent({});',
        # Using the symbol without constructing it.
        'import { Agent } from "@mastra/core";\nfunction f(a: Agent): Agent { return a; }',
        'import { Agent } from "@mastra/core";\nif (x instanceof Agent) {}',
        # run() only runs an agent; Runner is not one either.
        'import { run, Runner } from "@openai/agents";\nawait run(agent, "hi");\nconst r = new Runner();',
        # A same-named method on another object.
        'import { Agent } from "@mastra/core";\nconst x = this.Agent({});',
        # The AI SDK Agent type is an interface in AI SDK 6.
        'import type { Agent } from "ai";\nconst x = new Agent({ model, tools });',
        # StateGraph is from another package.
        'import { StateGraph } from "langgraph-lite";\nconst g = new StateGraph(s);',
    ],
)
def test_no_match(source: str) -> None:
    assert _matches(source) == ()


@pytest.mark.parametrize(
    ("options", "met"),
    [
        ("{ model, tools: { weather }, maxSteps: 5 }", True),
        ("{ model, tools, stopWhen: stepCountIs(5) }", True),
        ("{ model, tools, stopWhen }", True),
        ("{ model, tools: { weather }, maxSteps: 2 }", True),
        # Tools but no loop: Not sure.
        ("{ model, tools: { weather } }", False),
        ("{ model, tools, maxSteps: 1 }", False),
        ("{ model, tools, maxSteps: MAX_STEPS }", False),
        ("{ model, tools, maxSteps: 0x10 }", False),
        ("{ model, tools, maxSteps }", False),
        # Only an integer literal of 2 or more counts; a stopWhen of one step, or none, does not.
        ("{ model, tools, maxSteps: 1.5 }", False),
        ("{ model, tools, maxSteps: 2.0 }", False),
        ("{ model, tools, maxSteps: 1_0 }", True),
        ("{ model, tools, stopWhen: undefined }", False),
        ("{ model, tools, stopWhen: null, maxSteps: 1 }", False),
        ("{ model, tools, stopWhen: stepCountIs(1) }", False),
        ("{ model, tools, stopWhen: ai.stepCountIs(0) }", False),
        ("{ model, tools, stopWhen: stepCountIs(n) }", True),
        ("{ model, tools, stopWhen: [stepCountIs(1)] }", True),
    ],
)
def test_ai_sdk_tool_loop(options: str, met: bool) -> None:
    for function in ("generateText", "streamText"):
        source = (
            f'import {{ {function} }} from "ai";\nexport async function answer() {{\n  await {function}({options});\n}}'
        )
        (match,) = _matches(source)
        assert (match.framework, match.line, match.name, match.condition_met) == ("vercel-ai-sdk", 3, "answer", met)


@pytest.mark.parametrize(
    "source",
    [
        'import { generateText } from "ai";\nawait generateText({ model, prompt: "hi", maxSteps: 5 });',
        'import { streamText } from "ai";\nstreamText({ model, messages });',
        'import { generateText } from "ai";\ngenerateText(options);',
        # Tools that are not there.
        'import { generateText } from "ai";\ngenerateText({ model, tools: undefined, maxSteps: 5 });',
        'import { generateText } from "ai";\ngenerateText({ model, tools: null, maxSteps: 5 });',
        'import { generateText } from "ai";\ngenerateText({ model, tools: {}, stopWhen: stepCountIs(5) });',
        'import { generateText } from "ai";\ngenerateText({ model, tools: [ /* none */ ], maxSteps: 5 });',
    ],
)
def test_ai_sdk_call_without_tools_is_no_match(source: str) -> None:
    assert _matches(source) == ()


def test_ai_sdk_names_the_function_not_the_result() -> None:
    source = (
        'import { streamText } from "ai";\n'
        "export async function POST(req: Request) {\n"
        "  const result = streamText({ model, tools: { search, lookup: lookupTool }, stopWhen: s });\n"
        "  return result.toUIMessageStreamResponse();\n}\n"
        "const r = streamText({ model, tools, maxSteps: 3 });\n"
    )
    first, second = _matches(source)
    assert (first.name, first.qualified, first.tools) == ("POST", "POST", ("search", "lookup"))
    assert (second.name, second.qualified) == ("vercel-ai-sdk-agent", "vercel-ai-sdk-agent")


NAME_CASES = [
    ("const support = new Agent({});", ("support", "support")),
    ('const support = new Agent({ name: "Support Bot" });', ("Support Bot", "Support Bot")),
    ("const support = new Agent({ name: `Support` });", ("support", "support")),
    ('const support = new Agent({ name: "a\\tb" });', ("support", "support")),
    ("const support = new Agent({ name: NAME });", ("support", "support")),
    ('const support = new Agent({ "name": "Quoted" });', ("Quoted", "Quoted")),
    ("export default new Agent({});", ("mastra-agent", "mastra-agent")),
    ("const support = (await new Agent({})) as Agent;", ("support", "support")),
    ("let support;\nsupport = new Agent({});", ("support", "support")),
    ("function build() {\n  return new Agent({});\n}", ("build", "build")),
    ("function build() {\n  const a = new Agent({});\n  return a;\n}", ("a", "build.a")),
    ("export const build = async () => new Agent({});", ("build", "build")),
    ("const o = { make: () => new Agent({}) };", ("make", "make")),
    ("class Bot {\n  agent = new Agent({});\n}", ("agent", "Bot.agent")),
    ("class Bot {\n  private readonly agent = new Agent({});\n}", ("agent", "Bot.agent")),
    ("class Bot {\n  constructor() {\n    this.agent = new Agent({});\n  }\n}", ("agent", "Bot.constructor.agent")),
    ("class Bot {\n  make() {\n    return new Agent({});\n  }\n}", ("make", "Bot.make")),
    ("app.get('/', () => {\n  return new Agent({});\n});", ("mastra-agent", "mastra-agent")),
    ("function outer() {\n  app.get('/', () => new Agent({}));\n}", ("outer", "outer")),
    ("module.exports.make = function () {\n  return new Agent({});\n};", ("make", "make")),
    ("const [a, b] = [new Agent({}), 1];", ("mastra-agent", "mastra-agent")),
    # CommonJS export targets are no name: the agent is anonymous, as for ``export default``.
    ("module.exports = new Agent({});", ("mastra-agent", "mastra-agent")),
    ("exports = new Agent({});", ("mastra-agent", "mastra-agent")),
    ("exports.default = new Agent({});", ("mastra-agent", "mastra-agent")),
    ("module.exports.default = new Agent({});", ("mastra-agent", "mastra-agent")),
    ("exports.supportAgent = new Agent({});", ("supportAgent", "supportAgent")),
    ("module.exports.supportAgent = new Agent({});", ("supportAgent", "supportAgent")),
]


@pytest.mark.parametrize(("code", "expected"), NAME_CASES)
def test_name_and_qualified_name(code: str, expected: tuple[str, str]) -> None:
    (match,) = _matches(f'import {{ Agent }} from "@mastra/core/agent";\n{code}\n')
    assert (match.name, match.qualified) == expected


def test_a_builder_chain_is_named_after_its_variable() -> None:
    source = (
        'import { StateGraph } from "@langchain/langgraph";\nconst graph = new StateGraph(S).addNode("a", f).compile();'
    )
    assert _found(source) == [("langgraph", 2, "graph")]


@pytest.mark.parametrize(
    ("options", "model", "tools"),
    [
        ('{ model: "gpt-4.1" }', "gpt-4.1", ()),
        ('{ model: "openai/gpt-4o-mini" }', "openai/gpt-4o-mini", ()),
        ('{ model: openai("gpt-4o") }', None, ()),
        ('{ model: "http://localhost:11434/llama3" }', None, ()),
        ('{ model: "user@example.com" }', None, ()),
        ('{ model: "gpt 4o" }', None, ()),
        ("{ model: `gpt-4o` }", None, ()),
        ("{ tools: [search, lookup, search] }", None, ("search", "lookup")),
        ('{ tools: [search, tool({ name: "x" }), mod.fn, "s"] }', None, ("search",)),
        ("{ tools: { weather, search: searchTool, [k]: v } }", None, ("weather", "search")),
        ("{ tools: TOOLS }", None, ()),
        ("{ tools }", None, ()),
        ('{ instructions: "You are a support agent", tools: [] }', None, ()),
    ],
)
def test_model_and_tools(options: str, model: str | None, tools: tuple[str, ...]) -> None:
    (match,) = _matches(f'import {{ Agent }} from "@openai/agents";\nconst x = new Agent({options});\n')
    assert (match.model, match.tools) == (model, tools)


def test_tools_are_capped() -> None:
    names = ", ".join(f"t{i}" for i in range(MAX_TOOLS + 5))
    (match,) = _matches(f'import {{ Agent }} from "@openai/agents";\nconst x = new Agent({{ tools: [{names}] }});\n')
    assert len(match.tools) == MAX_TOOLS


def test_instructions_are_never_read() -> None:
    (match,) = _matches(
        'import { Agent } from "@mastra/core";\nconst x = new Agent({ instructions: "secret prompt" });'
    )
    assert "secret prompt" not in repr(match)


def test_executor_absorbs_the_agent_it_runs() -> None:
    source = (
        'import { AgentExecutor, createOpenAIToolsAgent } from "langchain/agents";\n'
        "const agent = await createOpenAIToolsAgent({ llm, tools, prompt });\n"
        "const executor = new AgentExecutor({ agent, tools });\n"
        "const inline = AgentExecutor.fromAgentAndTools({ agent: await createOpenAIToolsAgent({ llm }), tools });\n"
        "const other = await createOpenAIToolsAgent({ llm, tools, prompt });\n"
        "class Svc {\n  async init() {\n"
        "    this.agent = await createOpenAIToolsAgent({ llm });\n"
        "    this.executor = new AgentExecutor({ agent: this.agent, tools });\n  }\n}\n"
    )
    assert _found(source) == [
        ("langchain", 3, "executor"),
        ("langchain", 4, "inline"),
        ("langchain", 5, "other"),
        ("langchain", 9, "executor"),
    ]


def test_absorbs_from_an_enclosing_scope() -> None:
    source = (
        'import { AgentExecutor, createReactAgent } from "langchain/agents";\n'
        "const agent = await createReactAgent({ llm });\n"
        "function f() {\n  return new AgentExecutor({ agent, tools });\n}\n"
        "class C {\n  agent = createReactAgent({ llm });\n"
        "  run() { return new AgentExecutor({ agent, tools }); }\n}\n"
    )
    # The class field is no variable of the method: the method's ``agent`` is the module's, which it absorbs.
    assert _found(source) == [("langchain", 4, "f"), ("langchain", 7, "agent"), ("langchain", 8, "run")]


def test_absorbs_only_from_its_own_or_an_enclosing_scope() -> None:
    source = (
        'import { AgentExecutor, createReactAgent } from "langchain/agents";\n'
        "function a() { const agent = createReactAgent({ llm }); }\n"
        "function b() { return new AgentExecutor({ agent, tools }); }\n"
    )
    assert _found(source) == [("langchain", 2, "agent"), ("langchain", 3, "b")]


GRAPH = 'import { StateGraph } from "@langchain/langgraph";\nconst workflow = new StateGraph(MessagesAnnotation);\n'


@pytest.mark.parametrize(
    ("extra", "wiring"),
    [
        (
            'import { ToolNode } from "@langchain/langgraph/prebuilt";\nconst node = new ToolNode(tools);\n',
            {Wiring.TOOL_NODE},
        ),
        (
            'import { toolsCondition } from "@langchain/langgraph/prebuilt";\n'
            'workflow.addConditionalEdges("agent", toolsCondition);\n',
            {Wiring.TOOLS_CONDITION},
        ),
        (
            'import * as prebuilt from "@langchain/langgraph/prebuilt";\n'
            'workflow.addConditionalEdges("agent", prebuilt.toolsCondition);\n',
            {Wiring.TOOLS_CONDITION},
        ),
        ("const bound = model.bindTools(tools);\n", {Wiring.BIND_TOOLS}),
        # Only imported, never used; or a same-named local.
        ('import { toolsCondition } from "@langchain/langgraph/prebuilt";\n', set()),
        ("class ToolNode {}\nconst node = new ToolNode(tools);\n", set()),
        ("", set()),
    ],
)
def test_graph_wiring(extra: str, wiring: set[Wiring]) -> None:
    result = _result(GRAPH + extra)
    assert [(m.framework, m.name) for m in result.matches] == [("langgraph", "workflow")]
    assert result.wiring == wiring


def test_wiring_needs_a_langchain_import() -> None:
    assert _result("const bound = model.bindTools(tools);\n").wiring == frozenset()


# The same agent in every TS/JS file kind. ``.cjs`` uses require; ``.jsx`` and ``.tsx`` hold JSX.
EXTENSION_CASES = [
    ("agent.ts", 'import { Agent } from "@mastra/core/agent";\nexport const bot = new Agent({ name: "Bot" });\n'),
    ("agent.mts", 'import { Agent } from "@mastra/core/agent";\nexport const bot = new Agent({ name: "Bot" });\n'),
    ("agent.cts", 'import { Agent } from "@mastra/core/agent";\nexport const bot = new Agent({ name: "Bot" });\n'),
    ("agent.js", 'import { Agent } from "@mastra/core/agent";\nexport const bot = new Agent({ name: "Bot" });\n'),
    ("agent.mjs", 'import { Agent } from "@mastra/core/agent";\nexport const bot = new Agent({ name: "Bot" });\n'),
    ("agent.cjs", 'const { Agent } = require("@mastra/core/agent");\nconst bot = new Agent({ name: "Bot" });\n'),
    (
        "agent.jsx",
        'import { Agent } from "@mastra/core/agent";\nconst bot = new Agent({ name: "Bot" });\n'
        "export const View = () => <div className='x'>{bot.name}</div>;\n",
    ),
    (
        "agent.tsx",
        'import { Agent } from "@mastra/core/agent";\nconst bot = new Agent<Ctx>({ name: "Bot" });\n'
        "export const View = (): JSX.Element => <div>{bot.name}</div>;\n",
    ),
]


@pytest.mark.parametrize(("filename", "source"), EXTENSION_CASES)
def test_every_extension(tmp_path: Path, filename: str, source: str) -> None:
    _write(tmp_path, {f"src/{filename}": source})
    result = scan_tree(tmp_path, repository="web")
    assert [(f.external_id, f.framework, f.verdict, f.line) for f in result.findings] == [
        (f"web:src/{filename}:Bot", "mastra", "proven", 2)
    ]
    assert (result.files_scanned, result.parse_errors) == (1, 0)


def test_scan_tree_classifies_typescript(tmp_path: Path) -> None:
    _write(
        tmp_path,
        {
            "package.json": '{"dependencies": {"@openai/agents": "^0.1"}, "devDependencies": {"ai": "^5"}}',
            "src/triage.ts": 'import { Agent } from "@openai/agents";\nimport { tool } from "@openai/agents";\n'
            'export const triage = new Agent({ name: "Triage", model: "gpt-4.1", tools: [lookup] });\n',
            "src/chat.ts": 'import { generateText } from "ai";\n'
            "export async function chat() { return generateText({ model, tools: { lookup } }); }\n",
            "src/graph.ts": GRAPH,
            "src/wired.ts": GRAPH + 'import { ToolNode } from "@langchain/langgraph/prebuilt";\nnew ToolNode(t);\n',
            "src/agent.test.ts": 'import { Agent } from "@mastra/core";\nconst fake = new Agent({});\n',
            "examples/demo.js": 'const { Agent } = require("@mastra/core");\nconst demo = new Agent({});\n',
            "web/node_modules/x/index.js": 'const { Agent } = require("@mastra/core");\nnew Agent({});\n',
        },
    )
    findings = {f.external_id: f for f in scan_tree(tmp_path, repository="web").findings}
    assert list(findings) == [
        "web:examples/demo.js:demo",
        "web:src/agent.test.ts:fake",
        "web:src/chat.ts:chat",
        "web:src/graph.ts:workflow",
        "web:src/triage.ts:Triage",
        "web:src/wired.ts:workflow",
    ]
    triage = findings["web:src/triage.ts:Triage"]
    assert triage.verdict == "proven"
    assert triage.attributes == {
        "framework": "openai-agents",
        "model": "gpt-4.1",
        "tools": [{"type": "function", "name": "lookup"}],
    }
    assert [(e.code, e.source, dict(e.observed)) for e in triage.evidence] == [
        ("d1_agent_dependency", "package.json", {"declared": True}),
        ("d2_agent_import", "src/triage.ts", {"imports": 2}),
    ]
    assert triage.service_folder == "."
    chat = findings["web:src/chat.ts:chat"]
    assert (chat.verdict, [e.code for e in chat.evidence]) == ("not_sure", ["d1_agent_dependency", "d2_agent_import"])
    graph, wired = findings["web:src/graph.ts:workflow"], findings["web:src/wired.ts:workflow"]
    assert (graph.verdict, [e.code for e in graph.evidence]) == ("not_sure", ["d2_agent_import"])
    assert wired.verdict == "proven"
    assert [(e.code, dict(e.observed)) for e in wired.evidence][-1] == ("d7_tool_calling", {"toolNode": True})
    assert findings["web:src/agent.test.ts:fake"].tag == "test"
    assert findings["web:examples/demo.js:demo"].attributes["labels"] == {"codeArea": "example"}


def test_python_and_typescript_keep_their_own_manifests(tmp_path: Path) -> None:
    # The npm package "langchain" is no evidence for a Python agent, nor requirements.txt for a TS one.
    _write(
        tmp_path,
        {
            "package.json": '{"dependencies": {"langchain": "^1"}}',
            "requirements.txt": "crewai\n",
            "agent.py": "from langchain.agents import create_agent\na = create_agent(m, t)\n",
            "agent.ts": 'import { createAgent } from "langchain";\nconst b = createAgent({ model });\n',
        },
    )
    findings = scan_tree(tmp_path, repository="r").findings
    assert [(f.path, [e.code for e in f.evidence]) for f in findings] == [
        ("agent.py", ["d2_agent_import"]),
        ("agent.ts", ["d1_agent_dependency", "d2_agent_import"]),
    ]
    assert [f.service_folder for f in findings] == [".", "."]


@pytest.mark.parametrize(
    "code",
    [
        # A binding of the imported name in a function, block or catch around the call: not the import.
        "function make(Agent) { return new Agent({}); }",
        "function make(Agent: any) { return new Agent({}); }",
        "function make({ Agent }) { return new Agent({}); }",
        "function make({ x: Agent }) { return new Agent({}); }",
        "function make([Agent]) { return new Agent({}); }",
        "function make(Agent = Base) { return new Agent({}); }",
        "function make(...Agent) { return new Agent({}); }",
        "const make = (Agent) => new Agent({});",
        "const make = Agent => new Agent({});",
        "class S { make(Agent: any) { return new Agent({}); } }",
        "function f() { class Agent {}\n  const x = new Agent({}); }",
        "function f() { function Agent() {}\n  const x = new Agent({}); }",
        "function f() { const Agent = class {}; const x = new Agent({}); }",
        "function f() { let { Agent } = local; const x = new Agent({}); }",
        "function f() { if (y) { var Agent = Other; } return new Agent({}); }",
        "try { run(); } catch (Agent) { new Agent({}); }",
        "for (const Agent of kinds) { new Agent({}); }",
        "{ class Agent {} new Agent({}); }",
        "function f(m) { return new m.Agent({}); }",
    ],
)
def test_a_local_binding_shadows_the_import(code: str) -> None:
    imports = 'import { Agent } from "@mastra/core/agent";\nimport * as m from "@mastra/core/agent";\n'
    assert _matches(imports + code + "\n") == ()


@pytest.mark.parametrize(
    "code",
    [
        # Bindings elsewhere do not shadow the import where it is used.
        "function other(Agent) {}\nfunction make() { return new Agent({}); }",
        "function make() { const helper = (Agent) => 1; return new Agent({}); }",
        "function make() { const o = { Agent: 1 }; return new Agent({}); }",
        "function make({ Agent: other }) { return new Agent({}); }",
        "function make(x = Agent) { return new Agent({}); }",
        "class Agent2 { Agent() { return new Agent({}); } }",
    ],
)
def test_a_binding_elsewhere_does_not_shadow(code: str) -> None:
    (match,) = _matches('import { Agent } from "@mastra/core/agent";\n' + code + "\n")
    assert match.framework == "mastra"


def test_an_import_inside_a_function_is_not_a_shadow() -> None:
    source = (
        "function make() {\n"
        '  const { Agent } = require("@openai/agents");\n'
        '  const oa = require("@openai/agents");\n'
        "  return [new Agent({}), new oa.Agent({})];\n}\n"
    )
    assert [m.framework for m in _matches(source, Language.JAVASCRIPT)] == ["openai-agents", "openai-agents"]


@pytest.mark.parametrize("parts", [1000, 5000])
def test_a_long_member_chain_is_read_without_recursion(parts: int) -> None:
    assert _matches('import * as z from "x";\nfoo' + ".x" * parts + "();\n") == ()
    assert _matches('import * as oa from "@openai/agents";\nconst b = new oa' + ".x" * parts + ".Agent({});\n") == ()


def test_a_name_literal_that_is_not_utf8_is_no_name() -> None:
    data = b'import { Agent } from "@openai/agents";\nconst bot = new Agent({ name: "B\xffot", model: "g\xff" });\n'
    tree = parse(Language.TYPESCRIPT, data)
    assert tree is not None
    (match,) = match_script_file("a.ts", tree, data, extract_imports(Language.TYPESCRIPT, tree, data)).matches
    assert (match.name, match.named, match.model) == ("bot", False, None)


def test_a_file_too_deep_to_match_is_skipped_not_fatal(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    real = match_script_file

    def deep(path: str, *args: object, **kwargs: object) -> ScriptMatches:
        if path == "src/deep.ts":
            raise RecursionError("maximum recursion depth exceeded")
        return real(path, *args, **kwargs)  # type: ignore[arg-type]

    monkeypatch.setattr("amp_shadow_runner.engine.scan.match_script_file", deep)
    _write(
        tmp_path,
        {
            "src/deep.ts": 'import { Agent } from "@openai/agents";\nconst a = new Agent({});\n',
            "src/bot.ts": 'import { Agent } from "@openai/agents";\nconst bot = new Agent({ name: "Bot" });\n',
        },
    )
    result = scan_tree(tmp_path, repository="r")
    assert [f.external_id for f in result.findings] == ["r:src/bot.ts:Bot"]
    assert (result.files_scanned, result.files_skipped) == (1, 1)


def test_a_commonjs_export_is_named_after_its_project(tmp_path: Path) -> None:
    _write(
        tmp_path,
        {
            "support-bot/package.json": '{"name": "support-bot"}',
            "support-bot/index.js": 'const { Agent } = require("@openai/agents");\nmodule.exports = new Agent({});\n',
            "support-bot/other.js": 'const { Agent } = require("@openai/agents");\nexports.default = new Agent({});\n',
        },
    )
    findings = scan_tree(tmp_path, repository="r").findings
    assert [(f.external_id, f.name) for f in findings] == [
        ("r:support-bot/index.js:openai-agents-agent", "support-bot (index.js)"),
        ("r:support-bot/other.js:openai-agents-agent", "support-bot (other.js)"),
    ]
