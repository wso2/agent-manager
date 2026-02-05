from __future__ import annotations

import json
import logging
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from fastapi import APIRouter

logger = logging.getLogger(__name__)

DATA_PATH = Path(__file__).resolve().parent / "storage" / "bookings.json"
DATA_PATH.parent.mkdir(parents=True, exist_ok=True)

router = APIRouter()


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


@router.post("/bookings", status_code=201)
def create_booking(payload: dict[str, Any]):
    user_id = payload.get("userId", "guest")
    pricing: list[dict[str, Any]] = []

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


@router.get("/bookings")
def get_bookings(userId: str):
    user_id = userId
    try:
        bookings = _load_bookings()
        return [booking for booking in bookings if booking.get("userId") == user_id]
    except Exception:
        logger.exception("get_bookings: failed to fetch bookings")
        return _error_response("Storage unavailable", "STORAGE_UNAVAILABLE")


@router.get("/bookings/{booking_id}")
def get_booking(booking_id: str, userId: str):
    user_id = userId
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


@router.put("/bookings/{booking_id}")
def update_booking(booking_id: str, payload: dict[str, Any]):
    user_id = payload.get("userId", "guest")
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
        updated_fields["pricing"] = []

        updated_booking = _update_booking_record(bookings, booking_id, updated_fields)
        _save_bookings(bookings)
        return {
            "message": "Booking updated successfully",
            "bookingDetails": updated_booking,
        }
    except Exception:
        logger.exception("update_booking: failed to update booking")
        return _error_response("Booking update failed", "BOOKING_UPDATE_FAILED")


@router.delete("/bookings/{booking_id}")
def cancel_booking(booking_id: str, userId: str):
    user_id = userId
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
