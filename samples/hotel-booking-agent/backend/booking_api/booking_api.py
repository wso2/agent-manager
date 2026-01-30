from __future__ import annotations

import json
import logging
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from hotel_search import (
    HotelSearchError,
    check_availability as hotel_check_availability,
    get_hotel_details as hotel_get_details,
    resolve_hotel_id_by_name as hotel_resolve_id_by_name,
    search_hotels as hotel_search_hotels,
)

logger = logging.getLogger(__name__)

DATA_PATH = Path(__file__).resolve().parent / "data" / "bookings.json"
DATA_PATH.parent.mkdir(parents=True, exist_ok=True)

app = FastAPI(title="Hotel Booking API")
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
    max_age=86400,
)


def _error_response(message: str, code: str) -> dict[str, Any]:
    return {
        "message": message,
        "errorCode": code,
        "timestamp": _get_current_timestamp(),
    }


def _get_current_timestamp() -> str:
    return datetime.now(timezone.utc).isoformat()


def _generate_booking_id() -> str:
    return f"BK{uuid.uuid4().hex[:8].upper()}"


def _generate_confirmation_number() -> str:
    return f"CONF{uuid.uuid4()}"


def _build_pricing(payload: dict[str, Any]) -> list[dict[str, Any]]:
    return []


def _resolve_user_id(request: Request, payload_user_id: str | None = None) -> tuple[dict[str, Any] | None, str | None]:
    header_user_id = request.headers.get("x-user-id")
    if header_user_id:
        return None, header_user_id
    if payload_user_id:
        return None, payload_user_id
    return None, "guest"


def _load_bookings() -> list[dict[str, Any]]:
    if not DATA_PATH.exists():
        DATA_PATH.write_text("[]")
        return []
    try:
        return json.loads(DATA_PATH.read_text())
    except json.JSONDecodeError:
        logger.warning("booking data corrupted; starting fresh")
        DATA_PATH.write_text("[]")
        return []


def _save_bookings(bookings: list[dict[str, Any]]) -> None:
    DATA_PATH.write_text(json.dumps(bookings, indent=2))


def _update_booking_record(bookings: list[dict[str, Any]], booking_id: str, updates: dict[str, Any]) -> dict[str, Any] | None:
    for booking in bookings:
        if booking.get("bookingId") == booking_id:
            booking.update(updates)
            return booking
    return None


@app.get("/health")
def health():
    return {"status": "ok"}


@app.get("/hotels/search")
def search_hotels(
    destination: str | None = None,
    checkInDate: str | None = None,
    checkOutDate: str | None = None,
    guests: int = 2,
    rooms: int = 1,
    minPrice: float | None = None,
    maxPrice: float | None = None,
    minRating: float | None = None,
    sortBy: str | None = None,
    page: int = 1,
    pageSize: int = 10,
):
    try:
        return hotel_search_hotels(
            None,
            destination=destination,
            check_in_date=checkInDate,
            check_out_date=checkOutDate,
            guests=guests,
            rooms=rooms,
            min_price=minPrice,
            max_price=maxPrice,
            min_rating=minRating,
            amenities=None,
            sort_by=sortBy,
            page=page,
            page_size=pageSize,
        )
    except HotelSearchError:
        logger.exception("search_hotels failed")
        return _error_response("Hotel search failed", "HOTEL_SEARCH_FAILED")


@app.get("/hotels/resolve")
def resolve_hotel_id(name: str):
    try:
        hotel_id = hotel_resolve_id_by_name(name)
        return {"hotelId": hotel_id}
    except Exception:
        logger.exception("resolve_hotel_id failed")
        return _error_response("Hotel resolve failed", "HOTEL_RESOLVE_FAILED")


@app.get("/hotels/{hotel_id}")
def get_hotel_details(
    hotel_id: str,
    checkInDate: str | None = None,
    checkOutDate: str | None = None,
    guests: int = 2,
):
    try:
        return hotel_get_details(
            None,
            hotel_id=hotel_id,
            check_in_date=checkInDate,
            check_out_date=checkOutDate,
            guests=guests,
        )
    except HotelSearchError:
        logger.exception("get_hotel_details failed")
        return _error_response("Hotel details unavailable", "HOTEL_DETAILS_FAILED")


@app.get("/hotels/{hotel_id}/availability")
def get_hotel_availability(
    hotel_id: str,
    checkInDate: str,
    checkOutDate: str,
    guests: int = 2,
    roomCount: int = 1,
):
    try:
        return hotel_check_availability(
            None,
            hotel_id=hotel_id,
            check_in_date=checkInDate,
            check_out_date=checkOutDate,
            guests=guests,
            room_count=roomCount,
        )
    except HotelSearchError:
        logger.exception("get_hotel_availability failed")
        return _error_response("Hotel availability unavailable", "HOTEL_AVAILABILITY_FAILED")


@app.post("/bookings", status_code=201)
def create_booking(payload: dict[str, Any], request: Request):
    error, user_id = _resolve_user_id(request, payload.get("userId"))
    if error or not user_id:
        return error or _error_response("Authentication required", "AUTH_REQUIRED")
    pricing = _build_pricing(payload)

    booking_id = _generate_booking_id()
    confirmation_number = _generate_confirmation_number()

    new_booking = {
        "bookingId": booking_id,
        "hotelId": payload.get("hotelId"),
        "hotelName": payload.get("hotelName"),
        "rooms": payload.get("rooms"),
        "userId": user_id,
        "checkInDate": payload.get("checkInDate"),
        "checkOutDate": payload.get("checkOutDate"),
        "numberOfGuests": payload.get("numberOfGuests"),
        "primaryGuest": payload.get("primaryGuest"),
        "pricing": pricing,
        "bookingStatus": "CONFIRMED",
        "bookingDate": _get_current_timestamp(),
        "confirmationNumber": confirmation_number,
        "specialRequests": payload.get("specialRequests"),
    }

    try:
        bookings = _load_bookings()
        bookings.append(new_booking)
        _save_bookings(bookings)
    except Exception:
        logger.exception("create_booking: failed to persist booking")
        return _error_response("Booking persistence failed", "BOOKING_PERSIST_FAILED")

    return {
        "bookingId": booking_id,
        "confirmationNumber": confirmation_number,
        "message": "Booking confirmed successfully",
        "bookingDetails": new_booking,
    }


@app.get("/bookings")
def get_bookings(request: Request):
    error, user_id = _resolve_user_id(request)
    if error or not user_id:
        return error or _error_response("Authentication required", "AUTH_REQUIRED")
    try:
        bookings = _load_bookings()
        return [booking for booking in bookings if booking.get("userId") == user_id]
    except Exception:
        logger.exception("get_bookings: failed to fetch bookings")
        return _error_response("Storage unavailable", "STORAGE_UNAVAILABLE")


@app.get("/bookings/{booking_id}")
def get_booking(booking_id: str, request: Request):
    error, user_id = _resolve_user_id(request)
    if error or not user_id:
        return error or _error_response("Authentication required", "AUTH_REQUIRED")
    try:
        bookings = _load_bookings()
        booking = next(
            (
                item
                for item in bookings
                if item.get("bookingId") == booking_id and item.get("userId") == user_id
            ),
            None,
        )
        if not booking:
            return _error_response("Booking not found", "BOOKING_NOT_FOUND")
        return booking
    except Exception:
        logger.exception("get_booking: failed to fetch booking")
        return _error_response("Storage unavailable", "STORAGE_UNAVAILABLE")


@app.put("/bookings/{booking_id}")
def update_booking(booking_id: str, payload: dict[str, Any], request: Request):
    error, user_id = _resolve_user_id(request, payload.get("userId"))
    if error or not user_id:
        return error or _error_response("Authentication required", "AUTH_REQUIRED")
    try:
        bookings = _load_bookings()
        booking = next(
            (
                item
                for item in bookings
                if item.get("bookingId") == booking_id and item.get("userId") == user_id
            ),
            None,
        )
        if not booking:
            return _error_response("Booking not found", "BOOKING_NOT_FOUND")

        updated_fields = {
            "hotelId": payload.get("hotelId", booking.get("hotelId")),
            "hotelName": payload.get("hotelName", booking.get("hotelName")),
            "rooms": payload.get("rooms", booking.get("rooms")),
            "checkInDate": payload.get("checkInDate", booking.get("checkInDate")),
            "checkOutDate": payload.get("checkOutDate", booking.get("checkOutDate")),
            "numberOfGuests": payload.get("numberOfGuests", booking.get("numberOfGuests")),
            "primaryGuest": payload.get("primaryGuest", booking.get("primaryGuest")),
            "specialRequests": payload.get("specialRequests", booking.get("specialRequests")),
            "updatedAt": _get_current_timestamp(),
        }
        updated_fields["pricing"] = _build_pricing({
            **booking,
            **updated_fields,
        })

        updated_booking = _update_booking_record(bookings, booking_id, updated_fields)
        _save_bookings(bookings)
        return {
            "message": "Booking updated successfully",
            "bookingDetails": updated_booking,
        }
    except Exception:
        logger.exception("update_booking: failed to update booking")
        return _error_response("Booking update failed", "BOOKING_UPDATE_FAILED")


@app.delete("/bookings/{booking_id}")
def cancel_booking(booking_id: str, request: Request):
    error, user_id = _resolve_user_id(request)
    if error or not user_id:
        return error or _error_response("Authentication required", "AUTH_REQUIRED")
    try:
        bookings = _load_bookings()
        booking = next(
            (
                item
                for item in bookings
                if item.get("bookingId") == booking_id and item.get("userId") == user_id
            ),
            None,
        )
        if not booking:
            return _error_response("Booking not found", "BOOKING_NOT_FOUND")

        updated_booking = _update_booking_record(
            bookings,
            booking_id,
            {
                "bookingStatus": "CANCELLED",
                "cancelledAt": _get_current_timestamp(),
            },
        )
        _save_bookings(bookings)
        return {
            "message": "Booking cancelled successfully",
            "bookingDetails": updated_booking,
        }
    except Exception:
        logger.exception("cancel_booking: failed to cancel booking")
        return _error_response("Booking cancel failed", "BOOKING_CANCEL_FAILED")
