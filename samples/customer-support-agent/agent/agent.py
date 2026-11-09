from datetime import datetime

from langchain_core.prompts import ChatPromptTemplate
from langchain_core.runnables import Runnable, RunnableConfig
from langchain_openai import ChatOpenAI

from .state import State
from .utils import create_tool_node_with_fallback
from tools.car_rentals import book_car_rental, cancel_car_rental, search_car_rentals, update_car_rental
from tools.excursions import book_excursion, cancel_excursion, search_trip_recommendations, update_excursion
from tools.flights import cancel_ticket, fetch_user_flight_information, search_flights, update_ticket_to_new_flight
from tools.hotels import book_hotel, cancel_hotel, search_hotels, update_hotel
from tools.policies import lookup_policy
from tools.web_search import tavily_tool


class Assistant:
    def __init__(self, runnable: Runnable):
        self.runnable = runnable

    def __call__(self, state: State, config: RunnableConfig):
        while True:
            configuration = config.get("configurable", {})
            passenger_id = configuration.get("passenger_id", None)
            state = {**state, "user_info": passenger_id}
            result = self.runnable.invoke(state)
            # If the LLM happens to return an empty response, we will re-prompt it
            # for an actual response.
            if not result.tool_calls and (
                not result.content
                or isinstance(result.content, list)
                and not result.content[0].get("text")
            ):
                messages = state["messages"] + [("user", "Respond with a real output.")]
                state = {**state, "messages": messages}
            else:
                break
        return {"messages": result}


def create_agent():
    llm = ChatOpenAI(model="gpt-4o", temperature=1)

    primary_assistant_prompt = ChatPromptTemplate.from_messages(
        [
            (
                "system",
                "You are a fictional airline customer support assistant. "
                " Use the provided tools to search for flights, company policies, and other information to assist the user's queries. "
                " Use Tavily, when available, for public travel information. Keep passenger and booking details out of web searches."
                " Use the demo tools for bookings, ticket changes, and cancellations. Clearly label those actions and results as simulated."
                " When searching, be persistent. Expand your query bounds if the first search returns no results. "
                " If a search comes up empty, expand your search before giving up."
                "\n\nCurrent user:\n<User>\n{user_info}\n</User>"
                "\nCurrent time: {time}.",
            ),
            ("placeholder", "{messages}"),
        ]
    ).partial(time=datetime.now)

    _tools = [
        fetch_user_flight_information,
        search_flights,
        lookup_policy,
        update_ticket_to_new_flight,
        cancel_ticket,
        search_car_rentals,
        book_car_rental,
        update_car_rental,
        cancel_car_rental,
        search_hotels,
        book_hotel,
        update_hotel,
        cancel_hotel,
        search_trip_recommendations,
        book_excursion,
        update_excursion,
        cancel_excursion,
    ]

    web_search = tavily_tool()
    if web_search is not None:
        _tools.append(web_search)

    _assistant_runnable = primary_assistant_prompt | llm.bind_tools(_tools)

    # %%
    from langgraph.checkpoint.memory import InMemorySaver
    from langgraph.graph import StateGraph, START
    from langgraph.prebuilt import tools_condition

    builder = StateGraph(State)

    # Define nodes: these do the work
    builder.add_node("assistant", Assistant(_assistant_runnable))
    builder.add_node("tools", create_tool_node_with_fallback(_tools))
    # Define edges: these determine how the control flow moves
    builder.add_edge(START, "assistant")
    builder.add_conditional_edges(
        "assistant",
        tools_condition,
    )
    builder.add_edge("tools", "assistant")

    # The checkpointer lets the graph persist its state
    # this is a complete memory for the entire graph.
    memory = InMemorySaver()
    _graph = builder.compile(checkpointer=memory)
    return _graph
