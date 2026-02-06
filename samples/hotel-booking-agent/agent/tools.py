from __future__ import annotations

import logging
from typing import Any, Optional
import requests
from datetime import date, datetime, timedelta, timezone
from pinecone import Pinecone
from langchain_core.tools import tool
from langchain_openai import OpenAIEmbeddings
from pydantic import BaseModel, Field

from config import settings

logger = logging.getLogger(__name__)


class RoomConfiguration(BaseModel):
    room_id: str = Field(..., description="Room ID to book.")
    number_of_rooms: int = Field(..., description="Number of rooms to book for this room_id.")
    price_per_night: float | None = Field(
        None, description="Room price per night to pass to booking."
    )


class GuestDetails(BaseModel):
    first_name: str = Field(..., description="Primary guest first name.")
    last_name: str = Field(..., description="Primary guest last name.")
    email: str = Field(..., description="Primary guest email address.")
    phone_number: str = Field(..., description="Primary guest phone number.")
    nationality: Optional[str] = Field(None, description="Primary guest nationality, if available.")


class SpecialRequests(BaseModel):
    dietary_requirements: Optional[str] = Field(None, description="Dietary requirements, if any.")
    accessibility_needs: Optional[str] = Field(None, description="Accessibility needs, if any.")
    bed_preference: Optional[str] = Field(None, description="Bed preference, if any.")
    pet_friendly: bool | None = Field(None, description="Whether the booking should be pet friendly.")
    other_requests: Optional[str] = Field(None, description="Other special requests.")


class BookingRequest(BaseModel):
    user_id: Optional[str] = Field(None, description="User ID for the booking.")
    hotel_id: str = Field(..., description="Hotel ID to book.")
    hotel_name: Optional[str] = Field(None, description="Hotel name, if available.")
    rooms: list[RoomConfiguration] = Field(..., description="Room configuration(s) to book.")
    check_in_date: str = Field(..., description="Check-in date in YYYY-MM-DD format.")
    check_out_date: str = Field(..., description="Check-out date in YYYY-MM-DD format.")
    number_of_guests: int = Field(..., description="Total number of guests.")
    number_of_rooms: int = Field(..., description="Total number of rooms.")
    primary_guest: GuestDetails = Field(..., description="Primary guest contact details.")
    special_requests: SpecialRequests | None = Field(
        None, description="Optional special requests."
    )


class BookingUpdateRequest(BaseModel):
    user_id: Optional[str] = Field(None, description="User ID for the booking.")
    booking_id: str = Field(..., description="Booking ID to update.")
    hotel_id: Optional[str] = Field(None, description="Hotel ID to update.")
    hotel_name: Optional[str] = Field(None, description="Hotel name to update.")
    rooms: list[RoomConfiguration] | None = Field(None, description="Updated room list.")
    check_in_date: Optional[str] = Field(None, description="Updated check-in date in YYYY-MM-DD format.")
    check_out_date: Optional[str] = Field(None, description="Updated check-out date in YYYY-MM-DD format.")
    number_of_guests: int | None = Field(None, description="Updated total number of guests.")
    number_of_rooms: int | None = Field(None, description="Updated total number of rooms.")
    primary_guest: GuestDetails | None = Field(None, description="Updated primary guest details.")
    special_requests: SpecialRequests | None = Field(None, description="Updated special requests.")


class BookingCancelRequest(BaseModel):
    user_id: Optional[str] = Field(None, description="User ID for the booking.")
    booking_id: str = Field(..., description="Booking ID to cancel.")


class BookingListRequest(BaseModel):
    user_id: Optional[str] = Field(None, description="User ID to list bookings for.")
    status: Optional[str] = Field(
        None,
        description="Optional booking status filter: CONFIRMED, CANCELLED, or ALL.",
    )


def _pinecone_index():
    pc = Pinecone(api_key=settings.pinecone_api_key)
    return pc.Index(settings.pinecone_index_name, host=settings.pinecone_service_url)


def _embedder() -> OpenAIEmbeddings:
    return OpenAIEmbeddings(
        model=settings.openai_embedding_model,
        api_key=settings.openai_api_key,
    )


def _booking_api_url(path: str) -> str:
    return f"{settings.hotel_api_base_url.rstrip('/')}{path}"


def _call_hotel_api(
    method: str,
    path: str,
    *,
    params: dict[str, Any] | None = None,
    json_body: dict[str, Any] | None = None,
) -> dict[str, Any]:
    url = _booking_api_url(path)
    try:
        response = requests.request(method, url, params=params, json=json_body, timeout=30)
        response.raise_for_status()
    except requests.RequestException:
        logger.exception("Hotel API request failed: %s %s", method, url)
        return {"error": "Hotel API request failed."}
    try:
        payload = response.json()
    except ValueError:
        return {"error": "Hotel API returned non-JSON response."}
    if isinstance(payload, dict) and payload.get("error_code"):
        return {"error": payload.get("message") or "Hotel API error.", "details": payload}
    return payload


def _resolve_hotel_id(hotel_name: Optional[str]) -> Optional[str]:
    candidate_name = (hotel_name or "").strip()
    if not candidate_name:
        return None
    logger.info("Resolving hotel id from name: %s", candidate_name)
    resolve_payload = _call_hotel_api(
        "GET",
        "/hotels/resolve",
        params={"name": candidate_name},
    )
    if isinstance(resolve_payload, dict):
        resolved_id = resolve_payload.get("hotel_id")
        return resolved_id if resolved_id else None
    return None


@tool
def query_hotel_policy_tool(
    question: str,
    hotel_id: Optional[str],
    hotel_name: Optional[str],
) -> dict[str, Any]:
    """
    Answer hotel policy questions for a specific hotel using policy documents.

    Args:
        question (str): The policy question to answer.
        hotel_id (Optional[str]): The hotel identifier, if known.
        hotel_name (Optional[str]): The hotel name, if known.

    Returns:
        dict[str, Any]: Retrieved context or a not-found note.
    """
    logger.info(
        "query_hotel_policy_tool called: hotel_id=%s hotel_name=%s question=%s",
        hotel_id,
        hotel_name,
        question,
    )
    clean_id = (hotel_id or "").strip()
    if clean_id and " " not in clean_id:
        resolved_id = clean_id
    else:
        resolved_id = _resolve_hotel_id(hotel_name or hotel_id)
    if resolved_id:
        index = _pinecone_index()
        embedder = _embedder()
        query_vector = embedder.embed_query(question)
        response = index.query(
            vector=query_vector,
            top_k=5,
            include_metadata=True,
            filter={"hotel_id": {"$eq": resolved_id}},
        )
        matches = response.get("matches", [])
        context_chunks = [m.get("metadata", {}).get("content", "") for m in matches]
        context = "\n\n".join([c for c in context_chunks if c])
        if context:
            return {
                "found": True,
                "source": "pinecone",
                "hotel_id": resolved_id,
                "context": context,
            }

    if not hotel_name and not resolved_id:
        return {
            "found": False,
            "source": "pinecone",
            "hotel_id": resolved_id,
            "context": "",
            "note": "Hotel name or ID required.",
        }

    return {
        "found": False,
        "source": "pinecone",
        "hotel_id": resolved_id,
        "context": "",
    }


@tool
def search_hotels_tool(
    check_in_date: Optional[str] = None,
    check_out_date: Optional[str] = None,
    destination: Optional[str] = None,
    guests: int = 1,
    max_price: float | None = None,
    min_price: float | None = None,
    min_rating: float | None = None,
    page: int = 1,
    page_size: int = 10,
    rooms: int = 1,
    sort_by: Optional[str] = None,
) -> dict[str, Any]:
    """
    Search hotels by destination with optional filters.

    Args:
        check_in_date (Optional[str]): Check-in date (YYYY-MM-DD).
        check_out_date (Optional[str]): Check-out date (YYYY-MM-DD).
        destination (Optional[str]): City or destination name.
        guests (int): Number of guests.
        max_price (float | None): Maximum nightly price.
        min_price (float | None): Minimum nightly price.
        min_rating (float | None): Minimum rating.
        page (int): Page number.
        page_size (int): Page size.
        rooms (int): Number of rooms.
        sort_by (Optional[str]): Sort key.

    Returns:
        dict[str, Any]: Hotel search results and metadata.
    """
    logger.info(
        "search_hotels_tool called: destination=%s check_in_date=%s check_out_date=%s guests=%s rooms=%s",
        destination,
        check_in_date,
        check_out_date,
        guests,
        rooms,
    )
    params: dict[str, Any] = {
        "check_in_date": check_in_date,
        "check_out_date": check_out_date,
        "destination": destination,
        "guests": guests,
        "max_price": max_price,
        "min_price": min_price,
        "min_rating": min_rating,
        "page": page,
        "page_size": page_size,
        "rooms": rooms,
        "sort_by": sort_by,
    }
    params = {k: v for k, v in params.items() if v is not None}
    response = _call_hotel_api("GET", "/hotels/search", params=params)
    if isinstance(response, dict) and response.get("error"):
        return response
    return response


@tool
def get_hotel_info_tool(hotel_id: Optional[str] = None, hotel_name: Optional[str] = None) -> dict[str, Any]:
    """
    Get details for one hotel by id or name.

    Args:
        hotel_id (Optional[str]): Hotel identifier.
        hotel_name (Optional[str]): Hotel name.

    Returns:
        dict[str, Any]: Hotel details including rooms and nearby attractions.
    """
    candidate = hotel_id or hotel_name or ""
    if candidate.lower().startswith("user_"):
        return {"error": "Invalid hotel_id provided. Ask for a hotel name or destination."}
    clean_id = (hotel_id or "").strip()
    if clean_id and " " not in clean_id:
        resolved_id = clean_id
    else:
        resolved_id = _resolve_hotel_id(hotel_name or hotel_id)
    if not resolved_id:
        return {"error": "Hotel not found. Provide a valid hotel_id or hotel_name."}
    logger.info("get_hotel_info_tool called: hotel_id=%s", resolved_id)
    response = _call_hotel_api("GET", f"/hotels/{resolved_id}")
    if isinstance(response, dict) and response.get("error"):
        return response
    return response


@tool
def check_hotel_availability_tool(
    check_in_date: str,
    check_out_date: str,
    guests: int,
    hotel_id: str,
    room_count: int,
    hotel_name: Optional[str] = None,
) -> dict[str, Any]:
    """
    Check room availability for a hotel for given dates and guest/room counts.

    Args:
        check_in_date (str): Check-in date (YYYY-MM-DD).
        check_out_date (str): Check-out date (YYYY-MM-DD).
        guests (int): Number of guests.
        hotel_id (str): Hotel identifier.
        room_count (int): Number of rooms requested.
        hotel_name (Optional[str]): Hotel name, if id is unknown.

    Returns:
        dict[str, Any]: Availability results and available rooms.
    """
    clean_id = (hotel_id or "").strip()
    if clean_id and " " not in clean_id:
        resolved_id = clean_id
    else:
        resolved_id = _resolve_hotel_id(hotel_name or hotel_id)
    if not resolved_id:
        return {"error": "Hotel not found. Provide a valid hotel_id or hotel_name."}
    logger.info(
        "check_hotel_availability_tool called: hotel_id=%s check_in_date=%s check_out_date=%s guests=%s room_count=%s",
        resolved_id,
        check_in_date,
        check_out_date,
        guests,
        room_count,
    )
    params = {
        "check_in_date": check_in_date,
        "check_out_date": check_out_date,
        "guests": guests,
        "room_count": room_count,
    }
    response = _call_hotel_api(
        "GET",
        f"/hotels/{resolved_id}/availability",
        params=params,
    )
    if isinstance(response, dict) and response.get("error"):
        return response
    return response


@tool(args_schema=BookingRequest)
def create_booking_tool(
    user_id: str,
    hotel_id: str,
    rooms: list[RoomConfiguration],
    check_in_date: str,
    check_out_date: str,
    number_of_guests: int,
    number_of_rooms: int,
    primary_guest: GuestDetails,
    special_requests: SpecialRequests | None = None,
    hotel_name: Optional[str] = None,
) -> dict[str, Any]:
    """
    Create a booking with hotel, dates, rooms, and guest details.

    Args:
        user_id (str): User identifier.
        hotel_id (str): Hotel identifier.
        rooms (list[RoomConfiguration]): Room configurations to book.
        check_in_date (str): Check-in date (YYYY-MM-DD).
        check_out_date (str): Check-out date (YYYY-MM-DD).
        number_of_guests (int): Total number of guests.
        number_of_rooms (int): Total number of rooms.
        primary_guest (GuestDetails): Primary guest contact details.
        special_requests (SpecialRequests | None): Optional special requests.
        hotel_name (Optional[str]): Hotel name, if available.

    Returns:
        dict[str, Any]: Booking confirmation details.
    """
    resolved_user_id = user_id or "guest"
    logger.info(
        "create_booking_tool called: user_id=%s hotel_id=%s check_in_date=%s check_out_date=%s number_of_rooms=%s",
        resolved_user_id,
        hotel_id,
        check_in_date,
        check_out_date,
        number_of_rooms,
    )
    payload = {
        "user_id": resolved_user_id,
        "hotel_id": hotel_id,
        "hotel_name": hotel_name,
        "rooms": [room.model_dump() for room in rooms],
        "check_in_date": check_in_date,
        "check_out_date": check_out_date,
        "number_of_guests": number_of_guests,
        "number_of_rooms": number_of_rooms,
        "primary_guest": primary_guest.model_dump(),
        "special_requests": special_requests.model_dump() if special_requests else None,
    }
    endpoint = f"{settings.hotel_api_base_url.rstrip('/')}/bookings"
    try:
        response = requests.post(endpoint, json=payload, timeout=30)
        response.raise_for_status()
    except requests.RequestException:
        logger.exception("create_booking_tool failed calling booking API")
        return {"error": "Booking API request failed."}
    return response.json()


@tool(args_schema=BookingUpdateRequest)
def edit_booking_tool(
    user_id: Optional[str],
    booking_id: str,
    hotel_id: Optional[str] = None,
    hotel_name: Optional[str] = None,
    rooms: list[RoomConfiguration] | None = None,
    check_in_date: Optional[str] = None,
    check_out_date: Optional[str] = None,
    number_of_guests: int | None = None,
    number_of_rooms: int | None = None,
    primary_guest: GuestDetails | None = None,
    special_requests: SpecialRequests | None = None,
) -> dict[str, Any]:
    """
    Update an existing booking by booking_id.

    Args:
        user_id (Optional[str]): User identifier.
        booking_id (str): Booking identifier.
        hotel_id (Optional[str]): Hotel identifier.
        hotel_name (Optional[str]): Hotel name.
        rooms (list[RoomConfiguration] | None): Updated rooms.
        check_in_date (Optional[str]): Updated check-in date (YYYY-MM-DD).
        check_out_date (Optional[str]): Updated check-out date (YYYY-MM-DD).
        number_of_guests (int | None): Updated guest count.
        number_of_rooms (int | None): Updated room count.
        primary_guest (GuestDetails | None): Updated guest details.
        special_requests (SpecialRequests | None): Updated special requests.

    Returns:
        dict[str, Any]: Updated booking details.
    """
    logger.info(
        "edit_booking_tool called: booking_id=%s user_id=%s hotel_id=%s",
        booking_id,
        user_id,
        hotel_id,
    )
    payload: dict[str, Any] = {"booking_id": booking_id}
    if user_id:
        payload["user_id"] = user_id
    if hotel_id is not None:
        payload["hotel_id"] = hotel_id
    if hotel_name is not None:
        payload["hotel_name"] = hotel_name
    if rooms is not None:
        payload["rooms"] = [room.model_dump() for room in rooms]
    if check_in_date is not None:
        payload["check_in_date"] = check_in_date
    if check_out_date is not None:
        payload["check_out_date"] = check_out_date
    if number_of_guests is not None:
        payload["number_of_guests"] = number_of_guests
    if number_of_rooms is not None:
        payload["number_of_rooms"] = number_of_rooms
    if primary_guest is not None:
        payload["primary_guest"] = primary_guest.model_dump()
    if special_requests is not None:
        payload["special_requests"] = special_requests.model_dump()

    endpoint = f"{settings.hotel_api_base_url.rstrip('/')}/bookings/{booking_id}"
    try:
        response = requests.put(endpoint, json=payload, timeout=30)
        response.raise_for_status()
    except requests.RequestException:
        logger.exception("edit_booking_tool failed calling booking API")
        return {"error": "Booking API request failed."}
    return response.json()


@tool(args_schema=BookingCancelRequest)
def cancel_booking_tool(booking_id: str, user_id: Optional[str] = None) -> dict[str, Any]:
    """
    Cancel a booking by booking_id.

    Args:
        booking_id (str): Booking identifier.
        user_id (Optional[str]): User identifier.

    Returns:
        dict[str, Any]: Cancellation status/details.
    """
    logger.info("cancel_booking_tool called: booking_id=%s user_id=%s", booking_id, user_id)
    endpoint = f"{settings.hotel_api_base_url.rstrip('/')}/bookings/{booking_id}"
    try:
        params = {"user_id": user_id} if user_id else None
        response = requests.delete(endpoint, params=params, timeout=30)
        response.raise_for_status()
    except requests.RequestException:
        logger.exception("cancel_booking_tool failed calling booking API")
        return {"error": "Booking API request failed."}
    return response.json()


@tool(args_schema=BookingListRequest)
def list_bookings_tool(user_id: Optional[str] = None, status: Optional[str] = None) -> dict[str, Any]:
    """
    List bookings for a user, optionally filtered by status.

    Args:
        user_id (Optional[str]): User identifier.
        status (Optional[str]): Status filter (CONFIRMED, CANCELLED, or ALL).

    Returns:
        dict[str, Any]: List of bookings.
    """
    logger.info("list_bookings_tool called: user_id=%s status=%s", user_id, status)
    endpoint = f"{settings.hotel_api_base_url.rstrip('/')}/bookings"
    try:
        params = {"user_id": user_id} if user_id else None
        response = requests.get(endpoint, params=params, timeout=30)
        response.raise_for_status()
    except requests.RequestException:
        logger.exception("list_bookings_tool failed calling booking API")
        return {"error": "Booking API request failed."}
    bookings = response.json() or []
    normalized_status = (status or "").strip().upper()
    if normalized_status in {"AVAILABLE", "ACTIVE"}:
        normalized_status = "CONFIRMED"
    if normalized_status and normalized_status != "ALL":
        bookings = [
            booking
            for booking in bookings
            if str(booking.get("booking_status", "")).upper() == normalized_status
        ]
    elif not normalized_status:
        bookings = [
            booking
            for booking in bookings
            if str(booking.get("booking_status", "")).upper() == "CONFIRMED"
        ]
    return {"bookings": bookings}


@tool
def get_weather_forecast_tool(location: str, date: Optional[str] = None) -> dict[str, Any]:
    """
    Get weather for a location (current or specific date).

    Args:
        location (str): City or location name.
        date (Optional[str]): Date in YYYY-MM-DD format.

    Returns:
        dict[str, Any]: WeatherAPI JSON response or an error.
    """
    if not settings.weather_api_key:
        return {"error": "Weather service is not configured."}
    logger.info("get_weather_forecast_tool called: location=%s date=%s", location, date)
    base_url = settings.weather_api_base_url.rstrip("/")
    if date:
        endpoint = f"{base_url}/forecast.json"
        params = {"key": settings.weather_api_key, "q": location, "dt": date}
    else:
        endpoint = f"{base_url}/current.json"
        params = {"key": settings.weather_api_key, "q": location}
    try:
        response = requests.get(endpoint, params=params, timeout=30)
        response.raise_for_status()
    except requests.RequestException:
        logger.exception("get_weather_forecast_tool failed calling Weather API")
        return {"error": "Weather API request failed."}
    try:
        return response.json()
    except ValueError:
        return {"error": "Weather API returned non-JSON response."}


@tool
def resolve_relative_dates_tool(text: str) -> dict[str, Any]:
    """
    Resolve relative date phrases in text into ISO dates (UTC).

    Args:
        text (str): Input text that may contain relative dates.

    Returns:
        dict[str, Any]: Resolved dates with labels and ISO strings.
    """
    logger.info("resolve_relative_dates_tool called: text=%s", text)
    now = datetime.now(timezone.utc).date()
    lowered = text.lower()
    resolved: list[dict[str, Any]] = []

    def _add(label: str, date_value):
        resolved.append({"label": label, "date": date_value.isoformat()})

    if "day after tomorrow" in lowered:
        _add("day_after_tomorrow", now + timedelta(days=2))
    if "today" in lowered:
        _add("today", now)
    if "tomorrow" in lowered and "day after tomorrow" not in lowered:
        _add("tomorrow", now + timedelta(days=1))

    weekdays = {
        "monday": 0,
        "tuesday": 1,
        "wednesday": 2,
        "thursday": 3,
        "friday": 4,
        "saturday": 5,
        "sunday": 6,
    }

    def _next_weekday(target: int, base: date) -> date:
        days_ahead = (target - base.weekday() + 7) % 7
        if days_ahead == 0:
            days_ahead = 7
        return base + timedelta(days=days_ahead)

    for name, idx in weekdays.items():
        if f"next {name}" in lowered:
            _add(f"next_{name}", _next_weekday(idx, now))
        elif f"this {name}" in lowered:
            # If today is that weekday, keep today; else next occurrence within this week.
            days_ahead = (idx - now.weekday() + 7) % 7
            _add(f"this_{name}", now + timedelta(days=days_ahead))

    if "this weekend" in lowered:
        # Upcoming Saturday/Sunday based on current week.
        saturday = _next_weekday(5, now) if now.weekday() > 5 else now + timedelta(days=(5 - now.weekday()))
        sunday = saturday + timedelta(days=1)
        _add("this_weekend_start", saturday)
        _add("this_weekend_end", sunday)
    if "next weekend" in lowered:
        saturday = _next_weekday(5, now) + timedelta(days=7)
        sunday = saturday + timedelta(days=1)
        _add("next_weekend_start", saturday)
        _add("next_weekend_end", sunday)

    return {"utc_today": now.isoformat(), "resolved": resolved}

TOOLS = [
    query_hotel_policy_tool,
    search_hotels_tool,
    get_hotel_info_tool,
    create_booking_tool,
    edit_booking_tool,
    cancel_booking_tool,
    list_bookings_tool,
    resolve_relative_dates_tool,
    check_hotel_availability_tool,
    get_weather_forecast_tool,
]
