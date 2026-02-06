from __future__ import annotations

import logging
from typing import Annotated, TypedDict

from langchain_core.messages import BaseMessage, SystemMessage
from langchain_openai import ChatOpenAI
from langgraph.graph import StateGraph
from langgraph.graph.message import add_messages
from langgraph.prebuilt import ToolNode, tools_condition
from langgraph.checkpoint.memory import InMemorySaver


from config import settings
from tools import TOOLS

logger = logging.getLogger(__name__)

SYSTEM_PROMPT = """You are an assistant for planning trip itineraries of a hotel listing company.
Help users plan their perfect trip, considering preferences and available hotels.

Instructions:
- Match hotels near attractions with user interests when prioritizing hotels.
- You may plan itineraries with multiple hotels based on user interests and attractions.
- Include the hotel and things to do for each day in the itinerary.
- Response should be in markdown format. Include the photos of the hotels if available.
- Use the provided tools to search for hotels and availability, answer policy questions, and place, view, edit, or cancel bookings as needed.
- Present only information explicitly returned by tool outputs or provided by the user. If something isn't available, say so and offer to look it up.
"""


class AgentState(TypedDict):
    messages: Annotated[list[BaseMessage], add_messages]


def build_graph():
    tools = TOOLS
    llm = ChatOpenAI(
        model=settings.openai_model,
        api_key=settings.openai_api_key,
    ).bind_tools(tools)

    def agent_node(state: AgentState) -> AgentState:
        messages = [SystemMessage(content=SYSTEM_PROMPT)] + state["messages"]
        response = llm.invoke(messages)
        tool_calls = getattr(response, "tool_calls", None) or []
        if tool_calls:
            tool_names = [call.get("name") for call in tool_calls if isinstance(call, dict)]
            logger.debug("agent_node decided to call tools: %s", tool_names)
        else:
            logger.debug("agent_node returned a final response (no tool calls).")
        return {"messages": [response]}

    graph = StateGraph(AgentState) #add in memory server
    graph.add_node("agent", agent_node)
    graph.add_node("tools", ToolNode(tools)) 
    
    # Remove the mapping - tools_condition returns "tools" or END automatically
    graph.add_conditional_edges("agent", tools_condition)
    graph.add_edge("tools", "agent")
    graph.set_entry_point("agent")

    checkpointer = InMemorySaver()
    return graph.compile(checkpointer=checkpointer)
