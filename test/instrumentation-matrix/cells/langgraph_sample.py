"""Minimal LangGraph agent sample for the matrix.

A two-node graph: an LLM with one bound tool, plus a ToolNode. The LLM is
prompted in a way that forces a tool call, the tool executes, then the LLM
produces a final answer. Covers the llm + tool span kinds in the same trace.

Cassette: cassettes/langgraph/agent_with_tools.yaml.
"""
from __future__ import annotations

from typing import Annotated, TypedDict

from langgraph.graph.message import add_messages


# Module-scope so LangGraph's get_type_hints() can resolve the forward reference
# when introspecting call_llm / should_continue.
# add_messages is a reducer that *appends* node outputs to the existing list;
# without it, each node's returned messages would replace the whole list.
class S(TypedDict):
    messages: Annotated[list, add_messages]


def run_scenario() -> str:
    from langchain_core.messages import HumanMessage
    from langchain_core.tools import tool
    from langchain_openai import ChatOpenAI
    from langgraph.graph import END, StateGraph
    from langgraph.prebuilt import ToolNode

    @tool
    def double(x: int) -> int:
        """Return twice the integer x."""
        return x * 2

    llm = ChatOpenAI(model="gpt-4o-mini", temperature=0).bind_tools([double])
    tools = ToolNode([double])

    def call_llm(state: S):
        return {"messages": [llm.invoke(state["messages"])]}

    def should_continue(state: S):
        last = state["messages"][-1]
        return "tools" if getattr(last, "tool_calls", None) else END

    g = StateGraph(S)
    g.add_node("llm", call_llm)
    g.add_node("tools", tools)
    g.set_entry_point("llm")
    g.add_conditional_edges("llm", should_continue, {"tools": "tools", END: END})
    g.add_edge("tools", "llm")
    app = g.compile()

    result = app.invoke(
        {"messages": [HumanMessage(content="What is double of 21? Use the tool.")]}
    )
    return str(result["messages"][-1].content)
