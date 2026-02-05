from __future__ import annotations

import logging
from typing import Any, Optional
import json

import requests
from datetime import datetime, timedelta, timezone
from pinecone import Pinecone
from langchain_core.messages import HumanMessage, SystemMessage
from langchain_core.tools import tool
from langchain_openai import ChatOpenAI, OpenAIEmbeddings
from pydantic import BaseModel, Field

from config import Settings

logger = logging.getLogger(__name__)


class RoomConfiguration(BaseModel):
    roomId: str = Field(..., description="Room ID to book.")
    numberOfRooms: int = Field(..., description="Number of rooms to book for this roomId.")
    pricePerNight: float | None = Field(
        None, description="Room price per night to pass to booking."
    )


class GuestDetails(BaseModel):
    firstName: str = Field(..., description="Primary guest first name.")
    lastName: str = Field(..., description="Primary guest last name.")
    email: str = Field(..., description="Primary guest email address.")
    phoneNumber: str = Field(..., description="Primary guest phone number.")
    nationality: Optional[str] = Field(None, description="Primary guest nationality, if available.")


class SpecialRequests(BaseModel):
    dietaryRequirements: Optional[str] = Field(None, description="Dietary requirements, if any.")
    accessibilityNeeds: Optional[str] = Field(None, description="Accessibility needs, if any.")
    bedPreference: Optional[str] = Field(None, description="Bed preference, if any.")
    petFriendly: bool | None = Field(None, description="Whether the booking should be pet friendly.")
    otherRequests: Optional[str] = Field(None, description="Other special requests.")


class BookingRequest(BaseModel):
    userId: Optional[str] = Field(None, description="User ID for the booking.")
    hotelId: str = Field(..., description="Hotel ID to book.")
    hotelName: Optional[str] = Field(None, description="Hotel name, if available.")
    rooms: list[RoomConfiguration] = Field(..., description="Room configuration(s) to book.")
    checkInDate: str = Field(..., description="Check-in date in YYYY-MM-DD format.")
    checkOutDate: str = Field(..., description="Check-out date in YYYY-MM-DD format.")
    numberOfGuests: int = Field(..., description="Total number of guests.")
    numberOfRooms: int = Field(..., description="Total number of rooms.")
    primaryGuest: GuestDetails = Field(..., description="Primary guest contact details.")
    specialRequests: SpecialRequests | None = Field(
        None, description="Optional special requests."
    )


class BookingUpdateRequest(BaseModel):
    userId: Optional[str] = Field(None, description="User ID for the booking.")
    bookingId: str = Field(..., description="Booking ID to update.")
    hotelId: Optional[str] = Field(None, description="Hotel ID to update.")
    hotelName: Optional[str] = Field(None, description="Hotel name to update.")
    rooms: list[RoomConfiguration] | None = Field(None, description="Updated room list.")
    checkInDate: Optional[str] = Field(None, description="Updated check-in date in YYYY-MM-DD format.")
    checkOutDate: Optional[str] = Field(None, description="Updated check-out date in YYYY-MM-DD format.")
    numberOfGuests: int | None = Field(None, description="Updated total number of guests.")
    numberOfRooms: int | None = Field(None, description="Updated total number of rooms.")
    primaryGuest: GuestDetails | None = Field(None, description="Updated primary guest details.")
    specialRequests: SpecialRequests | None = Field(None, description="Updated special requests.")


class BookingCancelRequest(BaseModel):
    userId: Optional[str] = Field(None, description="User ID for the booking.")
    bookingId: str = Field(..., description="Booking ID to cancel.")


class BookingListRequest(BaseModel):
    userId: Optional[str] = Field(None, description="User ID to list bookings for.")
    status: Optional[str] = Field(
        None,
        description="Optional booking status filter: CONFIRMED, CANCELLED, or ALL.",
    )


def _pinecone_index(settings: Settings):
    pc = Pinecone(api_key=settings.pinecone_api_key)
    return pc.Index(settings.pinecone_index_name, host=settings.pinecone_service_url)


def _embedder(settings: Settings) -> OpenAIEmbeddings:
    return OpenAIEmbeddings(
        model=settings.openai_embedding_model,
        api_key=settings.openai_api_key,
    )


def _policy_llm(settings: Settings) -> ChatOpenAI:
    return ChatOpenAI(
        model=settings.openai_model,
        api_key=settings.openai_api_key,
        temperature=0.2,
    )


def build_tools(settings: Settings):
    def _booking_api_url(path: str) -> str:
        return f"{settings.booking_api_base_url.rstrip('/')}{path}"

    def _call_hotel_api(method: str, path: str, *, params: dict[str, Any] | None = None, json_body: dict[str, Any] | None = None) -> dict[str, Any]:
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
        if isinstance(payload, dict) and payload.get("errorCode"):
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
            resolved_id = resolve_payload.get("hotelId")
            return resolved_id if resolved_id else None
        return None

    @tool
    def query_hotel_policy_tool(
        question: str,
        hotel_id: Optional[str],
        hotel_name: Optional[str],
    ) -> str:
        """Retrieve hotel policy details from Pinecone."""
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
            index = _pinecone_index(settings)
            embedder = _embedder(settings)
            query_vector = embedder.embed_query(question)
            response = index.query(
                vector=query_vector,
                top_k=5,
                include_metadata=True,
                filter={"hotelId": {"$eq": resolved_id}},
            )
            matches = response.get("matches", [])
            context_chunks = [m.get("metadata", {}).get("content", "") for m in matches]
            context = "\n\n".join([c for c in context_chunks if c])
            if context:
                llm = _policy_llm(settings)
                system = SystemMessage(
                    content=(
                        "You are a hotel policy assistant. Answer only using the provided context. "
                        "If the answer is not in the context, say so."
                    )
                )
                user = HumanMessage(content=f"Question: {question}\n\nContext:\n{context}")
                result = llm.invoke([system, user])
                return json.dumps(
                    {
                        "found": True,
                        "source": "pinecone",
                        "hotelId": resolved_id,
                        "answer": result.content,
                    },
                    ensure_ascii=True,
                )

        if not hotel_name and not resolved_id:
            return json.dumps(
                {
                    "found": False,
                    "source": "pinecone",
                    "hotelId": resolved_id,
                    "answer": "",
                    "note": "Hotel name or ID required.",
                },
                ensure_ascii=True,
            )

        return json.dumps(
            {
                "found": False,
                "source": "pinecone",
                "hotelId": resolved_id,
                "answer": "",
            },
            ensure_ascii=True,
        )

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
        """Search hotels with filtering options."""
        logger.info(
            "search_hotels_tool called: destination=%s check_in_date=%s check_out_date=%s guests=%s rooms=%s",
            destination,
            check_in_date,
            check_out_date,
            guests,
            rooms,
        )
        params: dict[str, Any] = {
            "checkInDate": check_in_date,
            "checkOutDate": check_out_date,
            "destination": destination,
            "guests": guests,
            "maxPrice": max_price,
            "minPrice": min_price,
            "minRating": min_rating,
            "page": page,
            "pageSize": page_size,
            "rooms": rooms,
            "sortBy": sort_by,
        }
        params = {k: v for k, v in params.items() if v is not None}
        response = _call_hotel_api("GET", "/hotels/search", params=params)
        if isinstance(response, dict) and response.get("error"):
            return response
        return response

    @tool
    def get_hotel_info_tool(hotel_id: Optional[str] = None, hotel_name: Optional[str] = None) -> dict[str, Any]:
        """Retrieve detailed information about a hotel."""
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
        """Check availability of a hotel for given dates and guest count"""
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
            "checkInDate": check_in_date,
            "checkOutDate": check_out_date,
            "guests": guests,
            "roomCount": room_count,
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
        userId: str,
        hotelId: str,
        rooms: list[RoomConfiguration],
        checkInDate: str,
        checkOutDate: str,
        numberOfGuests: int,
        numberOfRooms: int,
        primaryGuest: GuestDetails,
        specialRequests: SpecialRequests | None = None,
        hotelName: Optional[str] = None,
    ) -> dict[str, Any]:
        """Create a booking via the hotel API."""
        resolved_user_id = userId or "guest"
        logger.info(
            "create_booking_tool called: user_id=%s hotel_id=%s check_in_date=%s check_out_date=%s number_of_rooms=%s",
            resolved_user_id,
            hotelId,
            checkInDate,
            checkOutDate,
            numberOfRooms,
        )
        payload = {
            "userId": resolved_user_id,
            "hotelId": hotelId,
            "hotelName": hotelName,
            "rooms": [room.model_dump() for room in rooms],
            "checkInDate": checkInDate,
            "checkOutDate": checkOutDate,
            "numberOfGuests": numberOfGuests,
            "numberOfRooms": numberOfRooms,
            "primaryGuest": primaryGuest.model_dump(),
            "specialRequests": specialRequests.model_dump() if specialRequests else None,
        }
        endpoint = f"{settings.booking_api_base_url.rstrip('/')}/bookings"
        try:
            response = requests.post(endpoint, json=payload, timeout=30)
            response.raise_for_status()
        except requests.RequestException:
            logger.exception("create_booking_tool failed calling booking API")
            return {"error": "Booking API request failed."}
        return response.json()

    @tool(args_schema=BookingUpdateRequest)
    def edit_booking_tool(
        userId: Optional[str],
        bookingId: str,
        hotelId: Optional[str] = None,
        hotelName: Optional[str] = None,
        rooms: list[RoomConfiguration] | None = None,
        checkInDate: Optional[str] = None,
        checkOutDate: Optional[str] = None,
        numberOfGuests: int | None = None,
        numberOfRooms: int | None = None,
        primaryGuest: GuestDetails | None = None,
        specialRequests: SpecialRequests | None = None,
    ) -> dict[str, Any]:
        """Edit an existing booking via the hotel API."""
        logger.info(
            "edit_booking_tool called: booking_id=%s user_id=%s hotel_id=%s",
            bookingId,
            userId,
            hotelId,
        )
        payload: dict[str, Any] = {"bookingId": bookingId}
        if userId:
            payload["userId"] = userId
        if hotelId is not None:
            payload["hotelId"] = hotelId
        if hotelName is not None:
            payload["hotelName"] = hotelName
        if rooms is not None:
            payload["rooms"] = [room.model_dump() for room in rooms]
        if checkInDate is not None:
            payload["checkInDate"] = checkInDate
        if checkOutDate is not None:
            payload["checkOutDate"] = checkOutDate
        if numberOfGuests is not None:
            payload["numberOfGuests"] = numberOfGuests
        if numberOfRooms is not None:
            payload["numberOfRooms"] = numberOfRooms
        if primaryGuest is not None:
            payload["primaryGuest"] = primaryGuest.model_dump()
        if specialRequests is not None:
            payload["specialRequests"] = specialRequests.model_dump()

        endpoint = f"{settings.booking_api_base_url.rstrip('/')}/bookings/{bookingId}"
        try:
            response = requests.put(endpoint, json=payload, timeout=30)
            response.raise_for_status()
        except requests.RequestException:
            logger.exception("edit_booking_tool failed calling booking API")
            return {"error": "Booking API request failed."}
        return response.json()

    @tool(args_schema=BookingCancelRequest)
    def cancel_booking_tool(bookingId: str, userId: Optional[str] = None) -> dict[str, Any]:
        """Cancel a booking via the hotel API."""
        logger.info("cancel_booking_tool called: booking_id=%s user_id=%s", bookingId, userId)
        endpoint = f"{settings.booking_api_base_url.rstrip('/')}/bookings/{bookingId}"
        try:
            params = {"userId": userId} if userId else None
            response = requests.delete(endpoint, params=params, timeout=30)
            response.raise_for_status()
        except requests.RequestException:
            logger.exception("cancel_booking_tool failed calling booking API")
            return {"error": "Booking API request failed."}
        return response.json()

    @tool(args_schema=BookingListRequest)
    def list_bookings_tool(userId: Optional[str] = None, status: Optional[str] = None) -> dict[str, Any]:
        """List bookings for a user via the booking API."""
        logger.info("list_bookings_tool called: user_id=%s status=%s", userId, status)
        endpoint = f"{settings.booking_api_base_url.rstrip('/')}/bookings"
        try:
            params = {"userId": userId} if userId else None
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
                if str(booking.get("bookingStatus", "")).upper() == normalized_status
            ]
        elif not normalized_status:
            bookings = [
                booking
                for booking in bookings
                if str(booking.get("bookingStatus", "")).upper() == "CONFIRMED"
            ]
        return {"bookings": bookings}

    @tool
    def get_weather_forecast_tool(location: str, date: Optional[str] = None) -> str:
        """Retrieve weather using WeatherAPI.com."""
        if not settings.weather_api_key:
            return "Weather service is not configured."
        logger.info("get_weather_forecast_tool called: location=%s date=%s", location, date)
        base_url = settings.weather_api_base_url.rstrip("/")
        if date:
            endpoint = f"{base_url}/forecast.json"
            params = {"key": settings.weather_api_key, "q": location, "dt": date}
        else:
            endpoint = f"{base_url}/current.json"
            params = {"key": settings.weather_api_key, "q": location}
        response = requests.get(endpoint, params=params, timeout=30)
        response.raise_for_status()
        return response.text

    @tool
    def resolve_relative_dates_tool(text: str) -> dict[str, Any]:
        """Resolve relative date phrases (UTC) into ISO dates."""
        logger.info("resolve_relative_dates_tool called: text=%s", text)
        now = datetime.now(timezone.utc).date()
        lowered = text.lower()
        resolved: list[dict[str, Any]] = []

        def _add(label: str, date_value):
            resolved.append({"label": label, "date": date_value.isoformat()})

        if "today" in lowered:
            _add("today", now)
        if "tomorrow" in lowered:
            _add("tomorrow", now + timedelta(days=1))
        if "day after tomorrow" in lowered:
            _add("day_after_tomorrow", now + timedelta(days=2))

        weekdays = {
            "monday": 0,
            "tuesday": 1,
            "wednesday": 2,
            "thursday": 3,
            "friday": 4,
            "saturday": 5,
            "sunday": 6,
        }

        def _next_weekday(target: int, base: datetime.date) -> datetime.date:
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

        return {"utcToday": now.isoformat(), "resolved": resolved}

    return [
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
