from __future__ import annotations

import logging
from typing import Annotated, TypedDict

from langchain_core.messages import BaseMessage, SystemMessage
from langchain_openai import ChatOpenAI
from langgraph.graph import StateGraph, END
from langgraph.graph.message import add_messages
from langgraph.prebuilt import ToolNode, tools_condition
from langgraph.checkpoint.memory import InMemorySaver


from config import Settings
from tools import build_tools

logger = logging.getLogger(__name__)

SYSTEM_PROMPT = (
    "You are an assistant for planning trip itineraries of a hotel listing company. "
    "Help users plan their perfect trip, considering preferences and available hotels.\n\n"
    "Instructions:\n"
    "- Match hotels near attractions with user interests when prioritizing hotels.\n"
    "- You may plan itineraries with multiple hotels based on user interests and attractions.\n"
    "- Include the hotel and things to do for each day in the itinerary.\n"
    "- Use markdown formatting in non-hotel-search answers. Include hotel photos if available.\n"
    "- Always call get_user_profile_tool first to retrieve personalization data.\n"
    "- If the user explicitly asks to book, call create_booking_tool using available hotel/room data.\n"
    "- When calling create_booking_tool, include pricePerNight for each room from availability results.\n"
    "- If the user asks to edit or modify a booking, call edit_booking_tool with the bookingId.\n"
    "- If the user asks to cancel a booking, call cancel_booking_tool with the bookingId.\n"
    "- If the user asks to list or view bookings, call list_bookings_tool with the userId from context. "
    "Filter by status when asked (available/my bookings => CONFIRMED, cancelled => CANCELLED, all => ALL).\n"
    "- If booking details are missing (hotelId, roomId, dates, guests, or primary guest contact info), "
    "ask a concise follow-up question instead of making up data. Use bullet points for the missing fields "
    "and list available room options as bullets when asking for a room selection.\n"
    "- Do not claim a booking failed unless the booking tool returns an error.\n"
    "- If a booking attempt fails, ask a concise follow-up to retry with corrected details or an alternative hotel.\n"
    "- After a successful booking tool response, provide the final user response and do not call more tools.\n"
    "- When listing past bookings, use hotelName when available; otherwise fall back to hotelId.\n"
    "- For hotel policy questions, call query_hotel_policy_tool with the hotel name or id and stop.\n"
    "- Use resolve_relative_dates_tool to resolve phrases like tomorrow, this weekend, next Friday into ISO dates. "
    "If ambiguity remains, ask a clarifying question and do not guess.\n"
    "- For availability responses, format each room with: Room Name, Price per night, Max Occupancy.\n"
    "- Prefer this discovery flow for hotels: call search_hotels_tool even if dates are missing, "
    "rank/summarize, ask for dates if missing.\n"
    "- When the user asks about a specific hotel, resolve hotelId then call get_hotel_info_tool.\n"
    "- For hotel search results or single-hotel details, return only HOTEL_RESULTS_JSON followed by valid JSON.\n"
    "- Do not output raw tool traces, internal reasoning, markdown headings, or code fences."
)


class AgentState(TypedDict):
    messages: Annotated[list[BaseMessage], add_messages]


def build_graph(settings: Settings):
    tools = build_tools(settings)
    llm = ChatOpenAI(
        model=settings.openai_model,
        api_key=settings.openai_api_key,
        temperature=0.3,
    ).bind_tools(tools)

    def agent_node(state: AgentState) -> AgentState:
        messages = [SystemMessage(content=SYSTEM_PROMPT)] + state["messages"]
        response = llm.invoke(messages)
        tool_calls = getattr(response, "tool_calls", None) or []
        if tool_calls:
            tool_names = [call.get("name") for call in tool_calls if isinstance(call, dict)]
            logger.info("agent_node decided to call tools: %s", tool_names)
        else:
            logger.info("agent_node returned a final response (no tool calls).")
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
