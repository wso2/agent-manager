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

"""amp_shadow_runner.engine.match and signatures: Python agent signatures, names and facts."""

import time
from collections.abc import Callable

import pytest

from amp_shadow_runner.engine.imports import Symbol, extract_imports
from amp_shadow_runner.engine.languages import Language, parse
from amp_shadow_runner.engine.match import MAX_TOOLS, Match, PythonMatches, match_python_file
from amp_shadow_runner.engine.signatures import Condition, Framework, RuleSet, Shape, Signature, Wiring, agent_class
from amp_shadow_runner.engine.signatures.python import PYTHON


def _matches(source: str) -> tuple[Match, ...]:
    data = source.encode()
    tree = parse(Language.PYTHON, data)
    assert tree is not None
    return match_python_file("app/agent.py", tree, data, extract_imports(Language.PYTHON, tree, data)).matches


def _found(source: str) -> list[tuple[str, int, str]]:
    return [(m.framework, m.line, m.name) for m in _matches(source)]


# One case per always-proven signature shape: (source, framework).
SIGNATURE_CASES = [
    ("from langchain.agents import AgentExecutor\nx = AgentExecutor(agent=a, tools=t)", "langchain"),
    (
        "from langchain.agents import AgentExecutor\nx = AgentExecutor.from_agent_and_tools(agent=a, tools=t)",
        "langchain",
    ),
    ("from langchain.agents.agent import AgentExecutor\nx = AgentExecutor(agent=a)", "langchain"),
    ("from langchain_classic.agents import AgentExecutor\nx = AgentExecutor(agent=a)", "langchain"),
    ("from langchain.agents import create_agent\nx = create_agent(model, tools)", "langchain"),
    ("from langchain.agents import create_react_agent\nx = create_react_agent(llm, tools, prompt)", "langchain"),
    ("from langchain.agents import create_tool_calling_agent\nx = create_tool_calling_agent(l, t, p)", "langchain"),
    (
        "from langchain.agents import create_openai_functions_agent\nx = create_openai_functions_agent(l, t, p)",
        "langchain",
    ),
    ("from langchain.agents import create_openai_tools_agent\nx = create_openai_tools_agent(l, t, p)", "langchain"),
    (
        "from langchain.agents import create_structured_chat_agent\nx = create_structured_chat_agent(l, t, p)",
        "langchain",
    ),
    ("from langchain.agents import initialize_agent\nx = initialize_agent(tools, llm)", "langchain"),
    ("from langchain_classic.agents import create_react_agent\nx = create_react_agent(l, t, p)", "langchain"),
    ("from langchain.agents.react.agent import create_react_agent\nx = create_react_agent(l, t, p)", "langchain"),
    ("from langgraph.prebuilt import create_react_agent\nx = create_react_agent(model, tools)", "langgraph"),
    (
        "from langgraph.prebuilt.chat_agent_executor import create_react_agent\nx = create_react_agent(m, t)",
        "langgraph",
    ),
    ("from crewai import Agent\nx = Agent(role='r', goal='g', backstory='b')", "crewai"),
    ("from crewai.agent import Agent\nx = Agent(role='r')", "crewai"),
    ("from strands import Agent\nx = Agent(tools=[t])", "strands"),
    ("from strands.agent import Agent\nx = Agent()", "strands"),
    ("from strands.agent.agent import Agent\nx = Agent()", "strands"),
    ("from google.adk.agents import Agent\nx = Agent(name='x', model='gemini-2.0-flash')", "google-adk"),
    ("from google.adk.agents import LlmAgent\nx = LlmAgent(name='x')", "google-adk"),
    ("from google.adk.agents.llm_agent import LlmAgent\nx = LlmAgent(name='x')", "google-adk"),
    ("from google.adk import Agent\nx = Agent(name='x')", "google-adk"),
    ("from google.adk.agents import BaseAgent\nclass x(BaseAgent):\n    pass", "google-adk"),
    ("import autogen\nx = autogen.AssistantAgent('assistant', llm_config=c)", "autogen"),
    ("from autogen import ConversableAgent\nx = ConversableAgent('c', llm_config={'model': 'gpt-4o'})", "autogen"),
    ("from autogen import ConversableAgent\nx = ConversableAgent('c', llm_config=llm_config)", "autogen"),
    ("from autogen.agentchat import AssistantAgent\nx = AssistantAgent('a')", "autogen"),
    ("from autogen_agentchat.agents import AssistantAgent\nx = AssistantAgent('a', model_client=m)", "autogen"),
    ("from llama_index.core.agent import ReActAgent\nx = ReActAgent.from_tools(tools, llm=llm)", "llama-index"),
    ("from llama_index.core.agent import ReActAgent\nx = ReActAgent(tools=tools)", "llama-index"),
    ("from llama_index.core.agent.workflow import FunctionAgent\nx = FunctionAgent(tools=t)", "llama-index"),
    ("from llama_index.core.agent.workflow import ReActAgent\nx = ReActAgent(tools=t)", "llama-index"),
    (
        "from llama_index.core.agent.workflow import AgentWorkflow\nx = AgentWorkflow.from_tools_or_functions(t)",
        "llama-index",
    ),
    ("from llama_index.agent.openai import OpenAIAgent\nx = OpenAIAgent.from_tools(tools)", "llama-index"),
    ("from pydantic_ai import Agent\nx = Agent('openai:gpt-4o')", "pydantic-ai"),
    ("from pydantic_ai import Agent\nx = Agent[None, str]('openai:gpt-4o')", "pydantic-ai"),
    ("from smolagents import CodeAgent\nx = CodeAgent(tools=[], model=m)", "smolagents"),
    ("from smolagents import ToolCallingAgent\nx = ToolCallingAgent(tools=[], model=m)", "smolagents"),
]


@pytest.mark.parametrize(("source", "framework"), SIGNATURE_CASES)
def test_always_proven_signature(source: str, framework: str) -> None:
    assert _found(source) == [(framework, 2, "x")]


@pytest.mark.parametrize(
    "source",
    [
        # Aliases, module references and fallback imports.
        "from crewai import Agent as A\nx = A(role='r')",
        "import crewai\nx = crewai.Agent(role='r')",
        "import crewai as c\nx = c.Agent(role='r')",
        "import crewai.agent\nx = crewai.agent.Agent(role='r')",
        "from crewai import *\nx = Agent(role='r')",
        "try:\n    from crewai import Agent\nexcept ImportError:\n    Agent = None\nx = Agent(role='r')",
        "def build():\n    from crewai import Agent\n    x = Agent(role='r')\n    return x",
    ],
)
def test_resolves_through_imports(source: str) -> None:
    assert [(m.framework, m.name) for m in _matches(source)] == [("crewai", "x")]


@pytest.mark.parametrize(
    "source",
    [
        # A same-named local class or function, with no import, never matches.
        "class Agent:\n    pass\nx = Agent()",
        "def create_react_agent():\n    pass\nx = create_react_agent()",
        "from myapp.agents import Agent\nx = Agent()",
        "from .agents import Agent\nx = Agent()",
        # A type-only import is no evidence of running code.
        "from typing import TYPE_CHECKING\nif TYPE_CHECKING:\n    from crewai import Agent\nx = Agent()",
        # Using the symbol without calling or deriving from it.
        "from crewai import Agent\nisinstance(x, Agent)",
        "from crewai import Agent\nx = Agent.model_validate(data)",
        "from crewai import Agent\ndef f(a: Agent) -> Agent:\n    return a",
        # A base class that proves an agent only when derived from is not one when called.
        "from google.adk.agents import BaseAgent\nx = BaseAgent(name='b')",
        # Orchestrators: their sub-agents are found on their own.
        "from google.adk.agents import SequentialAgent\nx = SequentialAgent(name='s', sub_agents=[a])",
        "from google.adk.agents import ParallelAgent\nx = ParallelAgent(name='p', sub_agents=[a])",
        "from google.adk.agents import LoopAgent\nx = LoopAgent(name='l', sub_agents=[a])",
        "from llama_index.core.agent.workflow import AgentWorkflow\nx = AgentWorkflow(agents=[a, b])",
        "from llama_index.core.agent import AgentWorkflow\nx = AgentWorkflow(agents=[a, b])",
        "from llama_index.core.agent.workflow import AgentWorkflow\nclass W(AgentWorkflow):\n    pass",
        # AutoGen ConversableAgent without a model: a human proxy, or no llm_config at all.
        "from autogen import ConversableAgent\nhuman = ConversableAgent('human', llm_config=False, human_input_mode='ALWAYS')",
        "from autogen import ConversableAgent\nx = ConversableAgent('x', llm_config=None)",
        "from autogen import ConversableAgent\nx = ConversableAgent('x')",
        "from autogen import ConversableAgent\nclass X(ConversableAgent):\n    pass",
        # A subscripted value is not a dotted name.
        "import crewai\nx = agents[0].Agent()",
    ],
)
def test_no_match(source: str) -> None:
    assert _matches(source) == ()


def test_subclass_is_named_after_its_class() -> None:
    source = "from autogen import AssistantAgent\n\nclass Planner(AssistantAgent, metaclass=M):\n    pass\n"
    assert _found(source) == [("autogen", 3, "Planner")]


def test_subclass_of_generic_and_dotted_bases() -> None:
    source = (
        "import pydantic_ai\nfrom crewai import Agent\n"
        "class A(pydantic_ai.Agent[None, str]):\n    pass\nclass B(Base, Agent):\n    pass\n"
    )
    assert _found(source) == [("pydantic-ai", 3, "A"), ("crewai", 5, "B")]


NAME_CASES = [
    ("support = Agent(role='r')", "support"),
    ("support: Agent = Agent(role='r')", "support"),
    ("self.support = Agent(role='r')", "support"),
    ("support = Agent(name='Support Bot')", "Support Bot"),
    ('support = Agent(name="  padded  ")', "padded"),
    ("Agent(name='Support')", "Support"),
    ("agents = [Agent(role='r')]", "strands-agent"),
    ("Agent()", "strands-agent"),
    ("a, b = Agent(), Agent()", "strands-agent"),
    ("support = Agent(name=NAME)", "support"),
    ("support = Agent(name=f'{x}-bot')", "support"),
    ("support = Agent(name=b'bot')", "support"),
    ("support = Agent(name='a' 'b')", "support"),
    ("support = Agent(name='''multi\nline''')", "support"),
    ("support = Agent(name='" + "n" * 101 + "')", "support"),
    ("support = Agent(name='')", "support"),
    ("support = Agent(name='a\\tb')", "support"),
    ("support = Agent(name=f'plain')", "plain"),
]


@pytest.mark.parametrize(("line", "name"), NAME_CASES)
def test_name(line: str, name: str) -> None:
    assert [m.name for m in _matches(f"from strands import Agent\n{line}\n")][:1] == [name]


@pytest.mark.parametrize(
    ("arguments", "model", "tools"),
    [
        ("model='gemini-2.0-flash'", "gemini-2.0-flash", ()),
        ("llm='gpt-4o'", "gpt-4o", ()),
        ("model=MODEL", None, ()),
        ("model=LiteLlm(model='x')", None, ()),
        ("tools=[search, lookup, search]", None, ("search", "lookup")),
        ("tools=[search, Tool(name='x'), mod.fn, 'str']", None, ("search",)),
        ("tools=TOOLS", None, ()),
        ("model='http://localhost:11434/llama3'", None, ()),
        ("model='user@example.com'", None, ()),
        ("model='gpt 4o'", None, ()),
        ("model='openai:gpt-4o'", "openai:gpt-4o", ()),
        ("instruction='You are a support agent', tools=[]", None, ()),
    ],
)
def test_model_and_tools(arguments: str, model: str | None, tools: tuple[str, ...]) -> None:
    (match,) = _matches(f"from google.adk.agents import Agent\nx = Agent({arguments})\n")
    assert (match.model, match.tools) == (model, tools)


def test_tools_are_capped() -> None:
    names = ", ".join(f"t{i}" for i in range(MAX_TOOLS + 5))
    (match,) = _matches(f"from strands import Agent\nx = Agent(tools=[{names}])\n")
    assert len(match.tools) == MAX_TOOLS


def test_executor_absorbs_the_agent_it_runs() -> None:
    source = (
        "from langchain.agents import AgentExecutor, create_react_agent\n"
        "agent = create_react_agent(llm, tools, prompt)\n"
        "executor = AgentExecutor(agent=agent, tools=tools)\n"
        "inline = AgentExecutor.from_agent_and_tools(agent=create_react_agent(llm, tools, prompt), tools=tools)\n"
        "other = create_react_agent(llm, tools, prompt)\n"
    )
    assert _found(source) == [("langchain", 3, "executor"), ("langchain", 4, "inline"), ("langchain", 5, "other")]


def test_absorbs_the_nearest_earlier_assignment_in_the_same_scope() -> None:
    source = (
        "from langchain.agents import AgentExecutor, create_react_agent\n"
        "def build():\n"
        "    agent = create_react_agent(llm, tools, first)\n"
        "    agent = create_react_agent(llm, tools, second)\n"
        "    executor = AgentExecutor(agent=agent, tools=tools)\n"
        "    agent = create_react_agent(llm, tools, later)\n"
        "    return executor\n"
        "def other():\n"
        "    agent = create_react_agent(llm, tools, prompt)\n"
    )
    assert _found(source) == [
        ("langchain", 3, "agent"),
        ("langchain", 5, "executor"),
        ("langchain", 6, "agent"),
        ("langchain", 9, "agent"),
    ]


def test_absorbs_from_an_enclosing_function_or_the_module() -> None:
    source = (
        "from langchain.agents import AgentExecutor, create_react_agent\n"
        "agent = create_react_agent(llm, tools, prompt)\n"
        "def run():\n"
        "    return AgentExecutor(agent=agent, tools=tools)\n"
        "class Svc:\n"
        "    agent = create_react_agent(llm, tools, prompt)\n"
        "    def run(self):\n"
        "        return AgentExecutor(agent=agent, tools=tools)\n"
        "def other():\n"
        "    agent = create_react_agent(llm, tools, prompt)\n"
        "def last():\n"
        "    return AgentExecutor(agent=agent, tools=tools)\n"
    )
    # A class attribute is no variable of its methods; another function's variable is not in scope.
    assert _found(source) == [
        ("langchain", 4, "run"),
        ("langchain", 6, "agent"),
        ("langchain", 8, "run"),
        ("langchain", 10, "agent"),
        ("langchain", 12, "last"),
    ]


def test_absorbs_an_attribute_by_its_chain() -> None:
    source = (
        "from langchain.agents import AgentExecutor, create_react_agent\n"
        "class Bot:\n"
        "    def __init__(self):\n"
        "        agent = create_react_agent(llm, tools, local)\n"
        "        self.agent = create_react_agent(llm, tools, prompt)\n"
        "        self.executor = AgentExecutor(agent=self.agent, tools=tools)\n"
    )
    assert _found(source) == [("langchain", 4, "agent"), ("langchain", 6, "executor")]


@pytest.mark.parametrize(
    "call",
    [
        "AgentExecutor.from_agent_and_tools(agent, tools)",
        "AgentExecutor.from_agent_and_tools(agent, tools=tools)",
        "AgentExecutor.from_agent_and_tools(agent=agent, tools=tools)",
    ],
)
def test_factory_absorbs_its_first_positional_argument(call: str) -> None:
    source = (
        "from langchain.agents import AgentExecutor, create_react_agent\n"
        f"agent = create_react_agent(llm, tools, prompt)\nexecutor = {call}\n"
    )
    assert _found(source) == [("langchain", 3, "executor")]


def test_constructor_does_not_absorb_a_positional_argument() -> None:
    source = (
        "from langchain.agents import AgentExecutor, create_react_agent\n"
        "agent = create_react_agent(llm, tools, prompt)\nexecutor = AgentExecutor(agent)\n"
    )
    assert _found(source) == [("langchain", 2, "agent"), ("langchain", 3, "executor")]


QUALIFIED_CASES = [
    ("agent = Agent()", ("agent", "agent")),
    ("def support():\n    agent = Agent()", ("agent", "support.agent")),
    ("class Bot:\n    def __init__(self):\n        self.agent = Agent()", ("agent", "Bot.__init__.agent")),
    ("def build():\n    return Agent(name='Support')", ("Support", "build.Support")),
    ("def researcher():\n    return Agent()", ("researcher", "researcher")),
    ("def team():\n    return [Agent(), 1]", ("team", "team")),
    ("class Crew:\n    def writer(self):\n        Agent()", ("writer", "Crew.writer")),
    ("def outer():\n    class Inner:\n        x = [Agent()]", ("outer", "outer")),
    ("class Holder:\n    items = [Agent()]", ("strands-agent", "Holder.strands-agent")),
    ("Agent()", ("strands-agent", "strands-agent")),
    ("def f():\n    class Mine(Agent):\n        pass", ("Mine", "f.Mine")),
]


@pytest.mark.parametrize(("code", "expected"), QUALIFIED_CASES)
def test_name_and_qualified_name(code: str, expected: tuple[str, str]) -> None:
    (match,) = _matches(f"from strands import Agent\n{code}\n")
    assert (match.name, match.qualified) == expected


def test_crewai_project_template() -> None:
    source = (
        "from crewai import Agent, Crew, Task\n"
        "from crewai.project import CrewBase, agent, crew\n\n"
        "@CrewBase\n"
        "class ResearchCrew:\n"
        "    @agent\n"
        "    def researcher(self) -> Agent:\n"
        "        return Agent(config=self.agents_config['researcher'], verbose=True)\n\n"
        "    @agent\n"
        "    def reporting_analyst(self) -> Agent:\n"
        "        return Agent(config=self.agents_config['reporting_analyst'])\n"
    )
    assert [(m.name, m.qualified, m.line) for m in _matches(source)] == [
        ("researcher", "ResearchCrew.researcher", 8),
        ("reporting_analyst", "ResearchCrew.reporting_analyst", 12),
    ]


def test_every_match_in_source_order() -> None:
    source = (
        "from crewai import Agent\nfrom crewai import Agent as A\n"
        "researcher = Agent(role='r')\nwriter = A(role='w')\n[Agent(), Agent()]\n"
    )
    assert _found(source) == [
        ("crewai", 3, "researcher"),
        ("crewai", 4, "writer"),
        ("crewai", 5, "crewai-agent"),
        ("crewai", 5, "crewai-agent"),
    ]


def test_signature_helpers() -> None:
    sigs = agent_class("f", "pkg", "Agent", factories=("build",), absorbs="agent")
    assert {(s.symbol, s.shape, s.absorbs) for s in sigs} == {
        (Symbol("pkg", "Agent"), Shape.CALL, "agent"),
        (Symbol("pkg", "Agent"), Shape.SUBCLASS, None),
        (Symbol("pkg.Agent", "build"), Shape.FACTORY, "agent"),
    }


@pytest.mark.parametrize(
    ("frameworks", "signatures"),
    [
        ((Framework("f", ("f",), ("f",)),), (Signature("g", Symbol("f", "A"), Shape.CALL),)),
        ((Framework("f", ("f",), ("f",)), Framework("f", ("f",), ("f",))), ()),
        (
            (Framework("f", ("f",), ("f",)),),
            (Signature("f", Symbol("f", "A"), Shape.CALL), Signature("f", Symbol("f", "A"), Shape.CALL)),
        ),
    ],
)
def test_rule_set_rejects_bad_data(frameworks: tuple[Framework, ...], signatures: tuple[Signature, ...]) -> None:
    with pytest.raises(ValueError):
        RuleSet(frameworks, signatures)


def test_python_framework_ids_are_lowercase_hyphenated() -> None:
    for framework in PYTHON.frameworks:
        assert framework.id == framework.id.lower()
        assert "_" not in framework.id


def _file(source: str, is_local: Callable[[str], bool] | None = None) -> PythonMatches:
    data = source.encode()
    tree = parse(Language.PYTHON, data)
    assert tree is not None
    return match_python_file(
        "app/graph.py", tree, data, extract_imports(Language.PYTHON, tree, data), is_local=is_local
    )


@pytest.mark.parametrize(
    "source",
    [
        "from langgraph.graph import StateGraph\nworkflow = StateGraph(State)",
        "from langgraph.graph.state import StateGraph\nworkflow = StateGraph(State)",
        "from langgraph.graph import MessageGraph\nworkflow = MessageGraph()",
        "import langgraph.graph as lg\nworkflow = lg.StateGraph(State)",
    ],
)
def test_graph_is_a_conditional_match(source: str) -> None:
    (match,) = _matches(source)
    assert (match.framework, match.name, match.signature.condition) == ("langgraph", "workflow", Condition.TOOLS)


NODE, CONDITION, BIND = Wiring.TOOL_NODE, Wiring.TOOLS_CONDITION, Wiring.BIND_TOOLS


@pytest.mark.parametrize(
    ("source", "wired"),
    [
        ("from langgraph.prebuilt import ToolNode\nnode = ToolNode(tools)", {NODE}),
        ("from langgraph.prebuilt.tool_node import ToolNode as TN\nnode = TN(tools)", {NODE}),
        ("from langgraph import prebuilt\nnode = prebuilt.ToolNode(tools)", {NODE}),
        (
            "from langgraph.prebuilt import tools_condition\ng.add_conditional_edges('agent', tools_condition)",
            {CONDITION},
        ),
        ("from langgraph.prebuilt import tools_condition as tc\ng.add_conditional_edges('agent', tc)", {CONDITION}),
        (
            "import langgraph.prebuilt\ng.add_conditional_edges('agent', langgraph.prebuilt.tools_condition)",
            {CONDITION},
        ),
        ("from langgraph.prebuilt import *\ng.add_conditional_edges('agent', tools_condition)", {CONDITION}),
        (
            "from langgraph.prebuilt import ToolNode, tools_condition\n"
            "node = ToolNode(tools)\ng.add_conditional_edges('agent', tools_condition)",
            {NODE, CONDITION},
        ),
        ("from langchain_openai import ChatOpenAI\nllm = ChatOpenAI().bind_tools(TOOLS)", {BIND}),
        ("from langgraph.graph import StateGraph\ndef call(state, model):\n    return model.bind_tools(T)", {BIND}),
        # .bind_tools needs a langchain or langgraph import: any class may have such a method.
        ("def call(state, model):\n    return model.bind_tools(TOOLS).invoke(state)", set()),
        ("from myapp.registry import Registry\nRegistry().bind_tools(handlers)", set()),
        # Importing is not using; a name that no import binds, or a local class of the same name, is not wiring.
        ("from langgraph.prebuilt import tools_condition, ToolNode", set()),
        ("from langgraph.prebuilt import ToolNode\nisinstance(n, ToolNode)", set()),
        ("from langgraph.graph import StateGraph\nclass ToolNode:\n    pass\nnode = ToolNode(tools)", set()),
        ("from langgraph.graph import StateGraph\ng.add_conditional_edges('agent', tools_condition)", set()),
        ("from langgraph.graph import END\nfrom myapp import router\ng.add_edge('a', router.tools_condition)", set()),
        (
            "from typing import TYPE_CHECKING\nif TYPE_CHECKING:\n    from langgraph.prebuilt import ToolNode\nToolNode(t)",
            set(),
        ),
        ("from langchain_core.tools import tool\nmodel.bind(tools=TOOLS)", set()),
    ],
)
def test_tool_wiring(source: str, wired: set[Wiring]) -> None:
    assert _file(source).wiring == wired


OPENAI_AGENT = "from agents import Agent, Runner\nsupport = Agent(name='Support', instructions='x')\n"


def test_openai_agent_is_a_conditional_match() -> None:
    (match,) = _file(OPENAI_AGENT).matches
    assert (match.framework, match.name, match.signature.condition) == (
        "openai-agents",
        "Support",
        Condition.DEPENDENCY,
    )
    (other,) = _matches("import agents\nbot = agents.Agent(name='Bot')\n")
    assert other.framework == "openai-agents"


@pytest.mark.parametrize(
    ("source", "local", "matched", "asked"),
    [
        (OPENAI_AGENT, {"agents"}, False, ["agents"]),
        (OPENAI_AGENT, set(), True, ["agents"]),
        # The whole module and every module above it are asked about: a local agents package, or a local
        # agents/agent.py in a namespace folder.
        ("from agents.agent import Agent\nx = Agent(name='x')\n", {"agents"}, False, ["agents"]),
        ("from agents.agent import Agent\nx = Agent(name='x')\n", {"agents.agent"}, False, ["agents", "agents.agent"]),
        ("from agents.agent import Agent\nx = Agent(name='x')\n", set(), True, ["agents", "agents.agent"]),
    ],
)
def test_openai_agent_from_a_local_module_is_no_match(
    source: str, local: set[str], matched: bool, asked: list[str]
) -> None:
    seen: list[str] = []

    def is_local(module: str) -> bool:
        seen.append(module)
        return module in local

    assert bool(_file(source, is_local).matches) is matched
    assert seen == asked


def test_only_the_dependency_condition_asks_whether_a_module_is_local() -> None:
    assert len(_file("from crewai import Agent\nx = Agent()\n", lambda _: True).matches) == 1


def test_runner_is_not_an_agent() -> None:
    assert _matches("from agents import Runner\nresult = Runner.run_sync(starting_agent, 'hi')\n") == ()
    assert _matches("from agents import Runner\nawait Runner.run(agent, 'hi')\n") == ()


def _crews(source: str) -> list[tuple[str, str, str | None]]:
    return [(m.framework if m.signature.condition is None else "crew", m.name, m.crew) for m in _matches(source)]


CREW_IMPORTS = "from crewai import Agent, Crew, Task\nfrom crewai.project import CrewBase, agent, crew, task\n"


def test_crew_links_variables_and_inline_agents() -> None:
    source = (
        "from crewai import Agent, Crew\n"
        "researcher = Agent(role='r')\n"
        "def main():\n"
        "    writer = Agent(role='w')\n"
        "    team = Crew(agents=[researcher, writer, Agent(role='inline')], tasks=[])\n"
        "    other = Agent(role='o')\n"
    )
    assert _crews(source) == [
        ("crewai", "researcher", "team"),
        ("crewai", "writer", "team"),
        ("crewai", "main", "team"),
        ("crewai", "other", None),
    ]


@pytest.mark.parametrize(
    ("crew", "name"),
    [
        ("Crew(name='Research Team', agents=[researcher])", "Research Team"),
        ("team = Crew(agents=[researcher])", "team"),
        ("def build():\n    return Crew(agents=[researcher])", "build"),
        # A crew built in a method is named after its class, unless named or assigned.
        ("class Team:\n    def crew(self):\n        return Crew(agents=[researcher])", "Team"),
        (
            "class Team:\n    def crew(self):\n        def inner():\n            return Crew(agents=[researcher])",
            "inner",
        ),
        ("class Team:\n    def crew(self):\n        return Crew(name='Research', agents=[researcher])", "Research"),
        ("class Team:\n    def crew(self):\n        team = Crew(agents=[researcher])", "team"),
        ("class Holder:\n    crews = [Crew(agents=[researcher])]", "Holder"),
        ("Crew(agents=[researcher])", "crewai-crew"),
    ],
)
def test_crew_name(crew: str, name: str) -> None:
    source = f"from crewai import Agent, Crew\nresearcher = Agent(role='r')\n{crew}\n"
    assert _crews(source) == [("crewai", "researcher", name)]


def test_crew_links_self_attributes_and_methods() -> None:
    source = (
        CREW_IMPORTS + "class Team:\n"
        "    def __init__(self):\n"
        "        self.researcher = Agent(role='r')\n"
        "    @agent\n"
        "    def writer(self) -> Agent:\n"
        "        return Agent(role='w')\n"
        "    def helper(self) -> Agent:\n"
        "        return Agent(role='h')\n"
        "    def crew(self) -> Crew:\n"
        "        return Crew(agents=[self.researcher, self.writer(), self.helper()], tasks=[])\n"
    )
    assert _crews(source) == [
        ("crewai", "researcher", "Team"),
        ("crewai", "writer", "Team"),
        ("crewai", "helper", None),
    ]


def test_crew_base_class_links_every_agent_method() -> None:
    source = (
        CREW_IMPORTS + "@CrewBase\n"
        "class ResearchCrew:\n"
        "    @agent\n"
        "    def researcher(self) -> Agent:\n"
        "        return Agent(config=self.agents_config['researcher'])\n"
        "    @agent\n"
        "    def reporting_analyst(self) -> Agent:\n"
        "        return Agent(config=self.agents_config['reporting_analyst'])\n"
        "    def not_an_agent_method(self) -> Agent:\n"
        "        return Agent(role='x')\n"
        "    @crew\n"
        "    def crew(self) -> Crew:\n"
        "        return Crew(agents=self.agents, tasks=self.tasks)\n"
    )
    assert _crews(source) == [
        ("crewai", "researcher", "ResearchCrew"),
        ("crewai", "reporting_analyst", "ResearchCrew"),
        ("crewai", "not_an_agent_method", None),
    ]


@pytest.mark.parametrize(
    "source",
    [
        # self.agents outside a @CrewBase class links nothing.
        "from crewai import Agent, Crew\nfrom crewai.project import agent\nclass T:\n"
        "    @agent\n    def a(self):\n        return Agent()\n    def crew(self):\n        return Crew(agents=self.agents)\n",
        # Agents built elsewhere, or dynamically.
        "from crewai import Crew\nfrom myapp.agents import researcher\nteam = Crew(agents=[researcher])\n",
        "from crewai import Crew\nteam = Crew(agents=build_agents(), tasks=[])\n",
        "from crewai import Agent, Crew\ndef f():\n    a = Agent()\ndef g():\n    team = Crew(agents=[a])\n",
    ],
)
def test_crew_linking_no_agent_is_a_match(source: str) -> None:
    assert [m for m in _matches(source) if m.signature.condition == Condition.CREW][0].crew is None
    assert all(m.crew is None for m in _matches(source))


def test_an_agent_in_two_crews_keeps_the_first() -> None:
    source = (
        "from crewai import Agent, Crew\nresearcher = Agent(role='r')\n"
        "first = Crew(agents=[researcher])\nsecond = Crew(agents=[researcher])\n"
    )
    assert _crews(source) == [("crewai", "researcher", "first")]


def test_crew_links_a_list_variable_of_its_scope() -> None:
    source = (
        "from crewai import Agent, Crew\n"
        "def main():\n"
        "    researcher = Agent(role='r')\n"
        "    writer = Agent(role='w')\n"
        "    members = [writer]\n"
        "    members = [researcher]\n"
        "    team = Crew(agents=members)\n"
        "    members = [writer]\n"
        "def other():\n"
        "    members = [x]\n"
    )
    assert _crews(source) == [("crewai", "researcher", "team"), ("crewai", "writer", None)]


def test_crew_list_variable_from_another_scope_links_nothing() -> None:
    source = (
        "from crewai import Agent, Crew\nresearcher = Agent(role='r')\nmembers = [researcher]\n"
        "def main():\n    team = Crew(agents=members)\n"
    )
    assert _crews(source) == [("crewai", "researcher", None), ("crew", "team", None)]


@pytest.mark.parametrize(
    "manager",
    ["manager_agent=boss", "manager_agent=Agent(role='boss')"],
)
def test_crew_links_its_manager_agent(manager: str) -> None:
    source = (
        "from crewai import Agent, Crew\nboss = Agent(role='m')\n"
        f"team = Crew(agents=build(), {manager}, process=Process.hierarchical)\n"
    )
    found = _crews(source)
    assert found[-1][2] == "team"
    assert ("crew", "team", None) not in found


@pytest.mark.parametrize("parts", [1000, 5000])
def test_a_long_dotted_name_is_read_without_recursion(parts: int) -> None:
    assert _matches("import x\nfoo" + ".x" * parts + "()\n") == ()
    assert _matches("import strands\nbot = strands" + ".x" * parts + ".Agent()\n") == ()


def test_a_dotted_name_of_up_to_64_parts_resolves() -> None:
    longest = "import a\nbot = a" + ".b" * 62 + ".Agent()\n"  # 64 parts with ``a`` and ``Agent``.
    rules = RuleSet((Framework("f", ("a",), ("a",)),), (Signature("f", Symbol("a" + ".b" * 62, "Agent"), Shape.CALL),))
    data = longest.encode()
    tree = parse(Language.PYTHON, data)
    assert tree is not None
    imports = extract_imports(Language.PYTHON, tree, data)
    assert [m.name for m in match_python_file("a.py", tree, data, imports, rules).matches] == ["bot"]
    # One part more (65) is no reference, so it never resolves.
    rules = RuleSet((Framework("f", ("a",), ("a",)),), (Signature("f", Symbol("a" + ".b" * 63, "Agent"), Shape.CALL),))
    data = longest.replace(".Agent", ".b.Agent").encode()
    tree = parse(Language.PYTHON, data)
    assert tree is not None
    assert match_python_file("a.py", tree, data, imports, rules).matches == ()


def test_a_name_literal_that_is_not_utf8_is_no_name() -> None:
    data = b'from strands import Agent\nsupport = Agent(name="B\xffot", model="gpt\xff-4o")\n'
    tree = parse(Language.PYTHON, data)
    assert tree is not None
    (match,) = match_python_file("a.py", tree, data, extract_imports(Language.PYTHON, tree, data)).matches
    assert (match.name, match.named, match.model) == ("support", False, None)


# One agent written once in each branch of a conditional: only the first is kept. (code, its line)
ONE_AGENT_IN_BRANCHES = [
    ("if fast:\n    graph = Agent(model=a)\nelse:\n    graph = Agent(model=b)\n", 3),
    ("if a:\n    graph = Agent()\nelif b:\n    graph = Agent()\nelse:\n    graph = Agent()\n", 3),
    ("if a:\n    graph = Agent()\nelse:\n    if b:\n        graph = Agent()\n    else:\n        graph = Agent()\n", 3),
    (
        "try:\n    graph = Agent()\nexcept ImportError:\n    graph = Agent()\nexcept Exception:\n    graph = Agent()\n",
        3,
    ),
    ("graph = Agent() if fast else Agent()\n", 2),
    ("match mode:\n    case 'fast':\n        graph = Agent()\n    case _:\n        graph = Agent()\n", 4),
    (
        "class Bot:\n    def __init__(self, fast):\n        if fast:\n            self.graph = Agent()\n"
        "        else:\n            self.graph = Agent()\n",
        5,
    ),
]


@pytest.mark.parametrize(("code", "line"), ONE_AGENT_IN_BRANCHES)
def test_one_variable_in_exclusive_branches_is_one_agent(code: str, line: int) -> None:
    (match,) = _matches(f"from strands import Agent\n{code}")
    assert (match.name, match.line) == ("graph", line)


def test_a_conditional_expression_is_named_after_its_variable() -> None:
    (match,) = _matches("from strands import Agent\ngraph = Agent() if fast else Agent()\n")
    assert (match.name, match.qualified, match.named) == ("graph", "graph", False)


# Still two agents: a reassignment in one block, two scopes, ``try`` and ``else`` (both run), two
# frameworks, ``name=`` literals, or no variable at all.
TWO_AGENTS = [
    "graph = Agent()\ngraph = Agent()\n",
    "if fast:\n    graph = Agent()\n    graph = Agent()\n",
    "def a():\n    graph = Agent()\ndef b():\n    graph = Agent()\n",
    "if x:\n    def a():\n        graph = Agent()\nelse:\n    def a():\n        graph = Agent()\n",
    "try:\n    graph = Agent()\nexcept E:\n    pass\nelse:\n    graph = Agent()\n",
    "if fast:\n    graph = Agent()\nelse:\n    graph = crewai.Agent(role='r')\n",
    "if fast:\n    Agent(name='Fast')\nelse:\n    Agent(name='Slow')\n",
    "if fast:\n    graph = Agent(name='Bot')\nelse:\n    graph = Agent(name='Bot')\n",
    "if fast:\n    Agent()\nelse:\n    Agent()\n",
    # The ``else`` path builds two agents.
    "if a:\n    graph = Agent()\nelse:\n    graph = Agent()\n    graph = Agent()\n",
]


@pytest.mark.parametrize("code", TWO_AGENTS)
def test_two_agents_stay_two(code: str) -> None:
    assert len(_matches(f"from strands import Agent\nimport crewai\n{code}")) == 2


def test_a_reassignment_after_a_conditional_is_another_agent() -> None:
    source = (
        "from strands import Agent\n"
        "if fast:\n    graph = Agent()\nelse:\n    graph = Agent()\n"
        "graph = Agent()\n"
        "if slow:\n    graph = Agent()\nelse:\n    graph = Agent()\n"
    )
    assert [m.line for m in _matches(source)] == [3, 6, 8]


def test_the_kept_branch_agent_is_the_one_an_executor_absorbs() -> None:
    source = (
        "from langchain.agents import AgentExecutor, create_react_agent\n"
        "if fast:\n    agent = create_react_agent(llm, tools, a)\n"
        "else:\n    agent = create_react_agent(llm, tools, b)\n"
        "executor = AgentExecutor(agent=agent, tools=tools)\n"
    )
    assert _found(source) == [("langchain", 6, "executor")]


def test_many_reassignments_are_matched_quickly() -> None:
    source = "from strands import Agent\ndef f():\n" + "    graph = Agent()\n" * 2000
    started = time.monotonic()
    assert len(_matches(source)) == 2000
    assert time.monotonic() - started < 2


def test_crews_in_branches_are_never_merged() -> None:
    source = (
        "from crewai import Agent, Crew\n"
        "researcher = Agent(role='r')\nwriter = Agent(role='w')\n"
        "if fast:\n    crew = Crew(agents=[researcher])\nelse:\n    crew = Crew(agents=[writer])\n"
    )
    assert _crews(source) == [("crewai", "researcher", "crew"), ("crewai", "writer", "crew")]
