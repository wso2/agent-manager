from langchain_core.runnables import RunnableConfig
from langchain_core.tools import tool

from .demo import change_booking, search


@tool
def search_trip_recommendations(
    location: str | None = None,
    name: str | None = None,
    keywords: str | None = None,
) -> list[dict]:
    """Search fictional excursions by location, name or keyword."""
    return search("excursion", location=location, name=name, keywords=keywords)


@tool
def book_excursion(recommendation_id: int, *, config: RunnableConfig) -> str:
    """Book a simulated excursion in this session."""
    return change_booking(config, "excursion", recommendation_id, "book")


@tool
def update_excursion(recommendation_id: int, details: str, *, config: RunnableConfig) -> str:
    """Update the details of this session's simulated excursion."""
    return change_booking(config, "excursion", recommendation_id, "update", details=details)


@tool
def cancel_excursion(recommendation_id: int, *, config: RunnableConfig) -> str:
    """Cancel this session's simulated excursion."""
    return change_booking(config, "excursion", recommendation_id, "cancel")
