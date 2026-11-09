from datetime import date, datetime

from langchain_core.runnables import RunnableConfig
from langchain_core.tools import tool

from .demo import change_booking, search, valid_dates


@tool
def search_car_rentals(
    location: str | None = None,
    name: str | None = None,
    price_tier: str | None = None,
    start_date: date | datetime | None = None,
    end_date: date | datetime | None = None,
) -> list[dict]:
    """Search fictional car rental inventory by location, name and price tier."""
    if not valid_dates(start_date, end_date):
        return []
    return search("car rental", location=location, name=name, price_tier=price_tier)


@tool
def book_car_rental(rental_id: int, *, config: RunnableConfig) -> str:
    """Make a simulated car rental booking in this session."""
    return change_booking(config, "car rental", rental_id, "book")


@tool
def update_car_rental(
    rental_id: int,
    start_date: date | datetime | None = None,
    end_date: date | datetime | None = None,
    *,
    config: RunnableConfig,
) -> str:
    """Change the dates of this session's simulated car rental booking."""
    return change_booking(config, "car rental", rental_id, "update", start=start_date, end=end_date)


@tool
def cancel_car_rental(rental_id: int, *, config: RunnableConfig) -> str:
    """Cancel this session's simulated car rental booking."""
    return change_booking(config, "car rental", rental_id, "cancel")
