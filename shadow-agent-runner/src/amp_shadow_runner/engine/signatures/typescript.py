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

"""TypeScript and JavaScript agent signatures: one rule set for ``.js``, ``.jsx``, ``.mjs``, ``.cjs``, ``.ts``,
``.mts``, ``.cts`` and ``.tsx`` files.

A symbol's module is an npm package root and covers its subpaths (``@mastra/core`` covers
``@mastra/core/agent``; see ``match_ts``). ``CALL`` covers ``new X(...)`` and ``X(...)`` alike.

Most signatures always prove an agent. The conditional ones (see ``Condition``):

- an AI SDK ``generateText`` or ``streamText`` call is proven only when it passes ``tools`` and loops
  (``maxSteps`` above 1, or ``stopWhen``); with tools but no loop it is Not sure, and with no tools no match;
- a LangGraph ``StateGraph`` is proven only when its own file wires tools: a ``new ToolNode(...)``, a
  ``toolsCondition`` reference or a ``.bindTools(...)`` call. Unlike Python, files it imports are not
  looked at: TypeScript imports name files by relative path and bundler aliases, which the engine does not
  resolve.

Left out on purpose: classes deriving from an agent class (rare in TypeScript), ``run(...)`` of the
OpenAI Agents SDK and ``Runner``, which only run an agent, and the AI SDK ``Agent`` type, an interface since
AI SDK 6 (its classes are ``Experimental_Agent`` in AI SDK 5 and ``ToolLoopAgent`` in AI SDK 6).
"""

from amp_shadow_runner.engine.imports import Symbol
from amp_shadow_runner.engine.signatures import (
    Condition,
    Framework,
    RuleSet,
    Shape,
    Signature,
    ToolWiring,
    conditional_calls,
    factories,
)

TYPESCRIPT = RuleSet(
    frameworks=(
        Framework("mastra", ("@mastra/core",), ("@mastra/core",)),
        Framework(
            "openai-agents",
            ("@openai/agents", "@openai/agents-core", "@openai/agents-openai"),
            ("@openai/agents", "@openai/agents-core"),
        ),
        Framework("vercel-ai-sdk", ("ai",), ("ai",)),
        Framework("langgraph", ("@langchain/langgraph",), ("@langchain/langgraph",)),
        Framework("langchain", ("langchain", "@langchain/core"), ("langchain",)),
    ),
    signatures=(
        # Mastra: ``new Agent({ name, instructions, model, tools })`` from @mastra/core or @mastra/core/agent.
        *factories("mastra", "@mastra/core", "Agent"),
        # OpenAI Agents SDK for JavaScript.
        *(
            sig
            for module in ("@openai/agents", "@openai/agents-core")
            for sig in (
                Signature("openai-agents", Symbol(module, "Agent"), Shape.CALL),
                Signature("openai-agents", Symbol(module, "Agent.create"), Shape.FACTORY),
            )
        ),
        # Vercel AI SDK: a tool loop (conditional), and the agent classes of AI SDK 5 and 6.
        Signature("vercel-ai-sdk", Symbol("ai", "generateText"), Shape.CALL, condition=Condition.TOOL_LOOP, runs=True),
        Signature("vercel-ai-sdk", Symbol("ai", "streamText"), Shape.CALL, condition=Condition.TOOL_LOOP, runs=True),
        *factories("vercel-ai-sdk", "ai", "Experimental_Agent", "ToolLoopAgent"),
        # LangGraph: the prebuilt ReAct agent, and graphs (proven only when wired to tools).
        *factories("langgraph", "@langchain/langgraph", "createReactAgent"),
        *conditional_calls("langgraph", "@langchain/langgraph", "StateGraph", condition=Condition.TOOLS),
        # LangChain: the legacy agents (langchain/agents) and createAgent (LangChain 1.0).
        *factories(
            "langchain",
            "langchain",
            "createAgent",
            "createReactAgent",
            "createToolCallingAgent",
            "createOpenAIToolsAgent",
        ),
        Signature("langchain", Symbol("langchain", "AgentExecutor"), Shape.CALL, absorbs="agent"),
        Signature("langchain", Symbol("langchain", "AgentExecutor.fromAgentAndTools"), Shape.FACTORY, absorbs="agent"),
    ),
    wiring=ToolWiring(
        calls=frozenset({Symbol("@langchain/langgraph", "ToolNode")}),
        references=frozenset({Symbol("@langchain/langgraph", "toolsCondition")}),
        methods=frozenset({"bindTools"}),
        modules=("@langchain/", "langchain"),
    ),
)
"""The TypeScript and JavaScript signatures, and what wires a graph to tools."""
