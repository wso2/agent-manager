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

import pytest

from amp_shadow_runner.engine.imports import Symbol, extract_imports
from amp_shadow_runner.engine.languages import Language, parse
from amp_shadow_runner.engine.match import MAX_TOOLS, Match, match_python
from amp_shadow_runner.engine.signatures import Framework, RuleSet, Shape, Signature, agent_class
from amp_shadow_runner.engine.signatures.python import PYTHON


def _matches(source: str) -> tuple[Match, ...]:
    data = source.encode()
    tree = parse(Language.PYTHON, data)
    assert tree is not None
    return match_python("app/agent.py", tree, data, extract_imports(Language.PYTHON, tree, data))


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
