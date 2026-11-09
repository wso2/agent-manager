from datetime import date, datetime

from langchain_core.runnables import RunnableConfig
from langchain_core.tools import tool

from .demo import change_booking, search, valid_dates


@tool
def search_hotels(
    location: str | None = None,
    name: str | None = None,
    price_tier: str | None = None,
    checkin_date: date | datetime | None = None,
    checkout_date: date | datetime | None = None,
) -> list[dict]:
    """Search fictional hotel inventory by location, name and price tier."""
    if not valid_dates(checkin_date, checkout_date):
        return []
    return search("hotel", location=location, name=name, price_tier=price_tier)


@tool
def book_hotel(hotel_id: int, *, config: RunnableConfig) -> str:
    """Make a simulated hotel booking in this session."""
    return change_booking(config, "hotel", hotel_id, "book")


@tool
def update_hotel(
    hotel_id: int,
    checkin_date: date | datetime | None = None,
    checkout_date: date | datetime | None = None,
    *,
    config: RunnableConfig,
) -> str:
    """Change the dates of this session's simulated hotel booking."""
    return change_booking(config, "hotel", hotel_id, "update", start=checkin_date, end=checkout_date)


@tool
def cancel_hotel(hotel_id: int, *, config: RunnableConfig) -> str:
    """Cancel this session's simulated hotel booking."""
    return change_booking(config, "hotel", hotel_id, "cancel")
