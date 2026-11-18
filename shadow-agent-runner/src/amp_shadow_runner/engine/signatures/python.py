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

"""Python agent signatures that always prove an agent.

Each symbol is listed at the paths code imports it from: the public path, and the defining module where
code commonly imports that instead. Old and new paths are both listed, since ``ImportTable`` keeps every
fallback import. Conditional rules (a plain LangGraph ``StateGraph``, OpenAI Agents) come later.

Left out on purpose:

- classes that do not need a model: AutoGen ``UserProxyAgent`` and ``CodeExecutorAgent``, and AutoGen
  ``ConversableAgent`` unless it is constructed with an ``llm_config`` (a human proxy has none or ``False``);
- orchestrators whose sub-agents are found on their own: ADK ``SequentialAgent``, ``ParallelAgent`` and
  ``LoopAgent``, and the LlamaIndex ``AgentWorkflow`` constructor. ``AgentWorkflow.from_tools_or_functions``
  stays: it builds its own agent from the tools and an LLM;
- the LangChain ``create_*_agent`` functions other than the common ones.
"""

from amp_shadow_runner.engine.signatures import (
    Framework,
    RuleSet,
    agent_class,
    agent_classes,
    base_classes,
    classmethod_factories,
    conditional_class,
    factories,
)

_LANGCHAIN_FACTORIES = (
    "create_react_agent",
    "create_tool_calling_agent",
    "create_openai_functions_agent",
    "create_openai_tools_agent",
    "create_structured_chat_agent",
    "initialize_agent",
)

PYTHON = RuleSet(
    frameworks=(
        Framework(
            "langchain", ("langchain", "langchain_classic", "langchain_core"), ("langchain", "langchain-classic")
        ),
        Framework("langgraph", ("langgraph",), ("langgraph", "langgraph-prebuilt")),
        Framework("crewai", ("crewai",), ("crewai",)),
        Framework("strands", ("strands",), ("strands-agents",)),
        Framework("google-adk", ("google.adk",), ("google-adk",)),
        Framework("autogen", ("autogen", "autogen_agentchat"), ("pyautogen", "autogen", "ag2", "autogen-agentchat")),
        Framework("llama-index", ("llama_index",), ("llama-index", "llama-index-core", "llama-index-agent-openai")),
        Framework("pydantic-ai", ("pydantic_ai",), ("pydantic-ai", "pydantic-ai-slim")),
        Framework("smolagents", ("smolagents",), ("smolagents",)),
    ),
    signatures=(
        # LangChain: the legacy agents (langchain 0.x, and langchain_classic since 1.0), and create_agent (1.0).
        *agent_class(
            "langchain", "langchain.agents", "AgentExecutor", factories=("from_agent_and_tools",), absorbs="agent"
        ),
        *agent_class(
            "langchain", "langchain.agents.agent", "AgentExecutor", factories=("from_agent_and_tools",), absorbs="agent"
        ),
        *agent_class(
            "langchain",
            "langchain_classic.agents",
            "AgentExecutor",
            factories=("from_agent_and_tools",),
            absorbs="agent",
        ),
        *factories("langchain", "langchain.agents", "create_agent", *_LANGCHAIN_FACTORIES),
        *factories("langchain", "langchain_classic.agents", *_LANGCHAIN_FACTORIES),
        *factories("langchain", "langchain.agents.react.agent", "create_react_agent"),
        *factories("langchain", "langchain.agents.tool_calling_agent.base", "create_tool_calling_agent"),
        *factories("langchain", "langchain.agents.openai_functions_agent.base", "create_openai_functions_agent"),
        *factories("langchain", "langchain.agents.openai_tools.base", "create_openai_tools_agent"),
        *factories("langchain", "langchain.agents.structured_chat.base", "create_structured_chat_agent"),
        *factories("langchain", "langchain.agents.initialize", "initialize_agent"),
        # LangGraph's prebuilt ReAct agent. A plain StateGraph is a conditional rule.
        *factories("langgraph", "langgraph.prebuilt", "create_react_agent"),
        *factories("langgraph", "langgraph.prebuilt.chat_agent_executor", "create_react_agent"),
        # CrewAI
        *agent_classes("crewai", "crewai", "Agent"),
        *agent_classes("crewai", "crewai.agent", "Agent"),
        # Strands Agents
        *agent_classes("strands", "strands", "Agent"),
        *agent_classes("strands", "strands.agent", "Agent"),
        *agent_classes("strands", "strands.agent.agent", "Agent"),
        # Google ADK. ``Agent`` is ``LlmAgent``; a custom agent derives from ``BaseAgent``.
        *agent_classes("google-adk", "google.adk", "Agent"),
        *agent_classes("google-adk", "google.adk.agents", "Agent", "LlmAgent"),
        *agent_classes("google-adk", "google.adk.agents.llm_agent", "Agent", "LlmAgent"),
        *base_classes("google-adk", "google.adk.agents", "BaseAgent"),
        *base_classes("google-adk", "google.adk.agents.base_agent", "BaseAgent"),
        # AutoGen 0.2 (pyautogen, ag2) and AgentChat 0.4+.
        *agent_classes("autogen", "autogen", "AssistantAgent"),
        *agent_classes("autogen", "autogen.agentchat", "AssistantAgent"),
        *agent_classes("autogen", "autogen.agentchat.assistant_agent", "AssistantAgent"),
        *conditional_class("autogen", "autogen", "ConversableAgent", requires="llm_config"),
        *conditional_class("autogen", "autogen.agentchat", "ConversableAgent", requires="llm_config"),
        *conditional_class("autogen", "autogen.agentchat.conversable_agent", "ConversableAgent", requires="llm_config"),
        *agent_classes("autogen", "autogen_agentchat.agents", "AssistantAgent"),
        # LlamaIndex: the workflow agents (0.12+, exported from llama_index.core.agent since 0.13), the
        # earlier ReActAgent and FunctionCallingAgent, and the legacy OpenAIAgent.
        *agent_class("llama-index", "llama_index.core.agent", "ReActAgent", factories=("from_tools",)),
        *agent_class("llama-index", "llama_index.core.agent", "FunctionCallingAgent", factories=("from_tools",)),
        *agent_classes("llama-index", "llama_index.core.agent", "FunctionAgent"),
        *classmethod_factories("llama-index", "llama_index.core.agent", "AgentWorkflow", "from_tools_or_functions"),
        *agent_class("llama-index", "llama_index.core.agent.react", "ReActAgent", factories=("from_tools",)),
        *agent_classes("llama-index", "llama_index.core.agent.workflow", "FunctionAgent", "ReActAgent", "CodeActAgent"),
        *classmethod_factories(
            "llama-index", "llama_index.core.agent.workflow", "AgentWorkflow", "from_tools_or_functions"
        ),
        *agent_class("llama-index", "llama_index.agent.openai", "OpenAIAgent", factories=("from_tools",)),
        *agent_class("llama-index", "llama_index.agent", "OpenAIAgent", factories=("from_tools",)),
        *agent_class("llama-index", "llama_index.agent", "ReActAgent", factories=("from_tools",)),
        # Pydantic AI
        *agent_classes("pydantic-ai", "pydantic_ai", "Agent"),
        *agent_classes("pydantic-ai", "pydantic_ai.agent", "Agent"),
        # smolagents
        *agent_classes("smolagents", "smolagents", "CodeAgent", "ToolCallingAgent"),
        *agent_classes("smolagents", "smolagents.agents", "CodeAgent", "ToolCallingAgent"),
    ),
)
"""The always-proven Python signatures."""
