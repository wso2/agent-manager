from __future__ import annotations

import base64
import json
import logging
import os
import uuid
from datetime import date, datetime, timezone
from pathlib import Path
from typing import Any

from dotenv import load_dotenv
from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware

logger = logging.getLogger(__name__)

load_dotenv()

users: list[dict[str, Any]] = []

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


def _calculate_nights(check_in: str | None, check_out: str | None) -> int:
    if not check_in or not check_out:
        return 0
    try:
        start = date.fromisoformat(check_in)
        end = date.fromisoformat(check_out)
        return max((end - start).days, 0)
    except Exception:
        return 0


def _build_pricing(payload: dict[str, Any]) -> list[dict[str, Any]]:
    if os.getenv("BOOKING_DISABLE_PRICING_LOOKUP", "").lower() in {"1", "true", "yes"}:
        return []
    nights = _calculate_nights(payload.get("checkInDate"), payload.get("checkOutDate"))
    if nights <= 0:
        return []

    requested_rooms = payload.get("rooms") or []
    if not requested_rooms:
        return []

    total_per_night = 0.0
    for room in requested_rooms:
        count = room.get("numberOfRooms") or 1
        rate = room.get("pricePerNight")
        if rate is None:
            return []
        try:
            total_per_night += float(rate) * int(count)
        except (TypeError, ValueError):
            return []

    if total_per_night <= 0:
        return []

    total_amount = round(total_per_night * nights, 2)
    return [
        {
            "roomRate": round(total_per_night, 2),
            "totalAmount": total_amount,
            "nights": nights,
            "currency": "USD",
        }
    ]


def _decode_jwt_payload(token: str) -> dict[str, Any]:
    parts = token.split(".")
    if len(parts) < 2:
        raise ValueError("Invalid JWT format")
    payload = parts[1]
    padding = "=" * (-len(payload) % 4)
    decoded = base64.urlsafe_b64decode(payload + padding)
    return json.loads(decoded)


def _determine_user_type(claims: dict[str, Any]) -> str:
    roles = claims.get("roles") or []
    for role in roles if isinstance(roles, list) else []:
        if "premium" in role.lower() or "vip" in role.lower():
            return "PREMIUM"

    groups = claims.get("groups") or []
    for group in groups if isinstance(groups, list) else []:
        if "premium" in group.lower() or "vip" in group.lower():
            return "PREMIUM"

    return "GUEST"


def _create_user_from_claims(claims: dict[str, Any]) -> dict[str, Any]:
    return {
        "userId": claims.get("sub"),
        "email": claims.get("email"),
        "firstName": claims.get("given_name"),
        "lastName": claims.get("family_name"),
        "phoneNumber": claims.get("phone_number"),
        "profilePicture": claims.get("picture"),
        "registrationDate": _get_current_timestamp(),
        "userType": _determine_user_type(claims),
        "authClaims": claims,
    }


def _find_or_create_user(user_id: str, claims: dict[str, Any]) -> dict[str, Any]:
    for user in users:
        if user.get("userId") == user_id:
            return {
                "userId": user.get("userId"),
                "email": claims.get("email"),
                "firstName": claims.get("given_name"),
                "lastName": claims.get("family_name"),
                "phoneNumber": claims.get("phone_number"),
                "profilePicture": claims.get("picture"),
                "registrationDate": user.get("registrationDate"),
                "userType": _determine_user_type(claims),
                "authClaims": claims,
            }

    new_user = _create_user_from_claims(claims)
    users.append(new_user)
    return new_user


def _extract_auth_context(request: Request) -> tuple[dict[str, Any] | None, dict[str, Any] | None]:
    assertion = request.headers.get("x-jwt-assertion")
    if not assertion:
        return None, None
    try:
        payload = _decode_jwt_payload(assertion)
    except Exception:
        return _error_response("Authentication required", "AUTH_REQUIRED"), None

    sub = payload.get("sub")
    if not sub:
        return _error_response("Authentication required", "AUTH_REQUIRED"), None

    claims = {
        "sub": sub,
        "email": payload.get("email"),
        "given_name": payload.get("given_name"),
        "family_name": payload.get("family_name"),
        "preferred_username": payload.get("preferred_username"),
        "phone_number": payload.get("phone_number"),
        "picture": payload.get("picture"),
        "roles": payload.get("roles"),
        "groups": payload.get("groups"),
    }

    return None, {"userId": sub, "userClaims": claims}


def _resolve_user_id(request: Request, payload_user_id: str | None = None) -> tuple[dict[str, Any] | None, str | None]:
    error, context = _extract_auth_context(request)
    if context:
        return None, context.get("userId")
    if error:
        return error, None
    header_user_id = request.headers.get("x-user-id")
    if header_user_id:
        return None, header_user_id
    if payload_user_id:
        return None, payload_user_id
    return _error_response("Authentication required", "AUTH_REQUIRED"), None


def _load_bookings() -> list[dict[str, Any]]:
    if not DATA_PATH.exists():
        return []
    try:
        return json.loads(DATA_PATH.read_text())
    except json.JSONDecodeError:
        logger.warning("booking data corrupted; starting fresh")
        return []


def _save_bookings(bookings: list[dict[str, Any]]) -> None:
    DATA_PATH.write_text(json.dumps(bookings, indent=2))


def _update_booking_record(bookings: list[dict[str, Any]], booking_id: str, updates: dict[str, Any]) -> dict[str, Any] | None:
    for booking in bookings:
        if booking.get("bookingId") == booking_id:
            booking.update(updates)
            return booking
    return None


@app.get("/auth/profile")
def get_profile(request: Request):
    error, context = _extract_auth_context(request)
    if error or not context:
        return error or _error_response("Authentication required", "AUTH_REQUIRED")

    user = _find_or_create_user(context["userId"], context["userClaims"])
    return user


@app.get("/health")
def health():
    return {"status": "ok"}


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
