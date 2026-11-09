from datetime import date, datetime

from langchain_core.runnables import RunnableConfig
from langchain_core.tools import tool

from .demo import FLIGHTS, change_ticket, day, tickets


@tool
def fetch_user_flight_information(config: RunnableConfig) -> list[dict]:
    """Find the current demo passenger's simulated ticket."""
    return tickets(config)


@tool
def search_flights(
    departure_airport: str | None = None,
    arrival_airport: str | None = None,
    start_time: date | datetime | None = None,
    end_time: date | datetime | None = None,
    limit: int = 20,
) -> list[dict]:
    """Search fictional flights by airport code and departure date."""
    return [
        {**flight, "simulated": True}
        for flight in FLIGHTS.values()
        if (departure_airport is None or flight["departure_airport"] == departure_airport.upper())
        and (arrival_airport is None or flight["arrival_airport"] == arrival_airport.upper())
        and (start_time is None or day(start_time) <= day(flight["scheduled_departure"]))
        and (end_time is None or day(flight["scheduled_departure"]) <= day(end_time))
    ][:max(0, limit)]


@tool
def update_ticket_to_new_flight(ticket_no: str, new_flight_id: int, *, config: RunnableConfig) -> str:
    """Move the demo passenger's ticket to a fictional flight."""
    return change_ticket(config, ticket_no, new_flight_id)


@tool
def cancel_ticket(ticket_no: str, *, config: RunnableConfig) -> str:
    """Cancel the simulated ticket in this session."""
    return change_ticket(config, ticket_no, None)
