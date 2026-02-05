from __future__ import annotations

import json
import logging
import re
import threading
from difflib import SequenceMatcher
from math import ceil
from pathlib import Path
from typing import Any

from fastapi import APIRouter

logger = logging.getLogger(__name__)

router = APIRouter()


class HotelSearchError(RuntimeError):
    pass


class HotelNotFoundError(HotelSearchError):
    pass


_dataset_lock = threading.Lock()
_dataset_cache: dict[str, Any] | None = None
_dataset_path = Path(__file__).resolve().parent / "resources" / "hotel_data.json"


def _load_dataset() -> dict[str, Any]:
    global _dataset_cache
    with _dataset_lock:
        if _dataset_cache is None:
            _dataset_cache = json.loads(_dataset_path.read_text())
    return _dataset_cache or {}


def _normalize_name(value: str) -> str:
    return re.sub(r"[^a-z0-9]+", "", value.lower())


def resolve_hotel_id_by_name(name: str, threshold: float = 0.75) -> str | None:
    data = _load_dataset()
    hotels = data.get("hotels") or []
    target = _normalize_name(name)
    if not target:
        return None
    best_id = None
    best_score = 0.0
    for hotel in hotels:
        hotel_name = str(hotel.get("hotelName") or hotel.get("name") or "")
        candidate = _normalize_name(hotel_name)
        if not candidate:
            continue
        score = SequenceMatcher(None, target, candidate).ratio()
        if score > best_score:
            best_score = score
            best_id = hotel.get("hotelId")
    if best_id and best_score >= threshold:
        return str(best_id)
    return None


def _rooms_for_hotel(hotel_id: str) -> list[dict[str, Any]]:
    data = _load_dataset()
    rooms = data.get("rooms") or []
    return [room for room in rooms if room.get("hotelId") == hotel_id]


def _sort_hotels_by_price(items: list[dict[str, Any]], ascending: bool) -> list[dict[str, Any]]:
    return sorted(
        items,
        key=lambda hotel: hotel.get("lowestPrice", 0),
        reverse=not ascending,
    )


def _sort_hotels_by_rating(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return sorted(
        items,
        key=lambda hotel: hotel.get("rating", 0),
        reverse=True,
    )


def _paginate(items: list[dict[str, Any]], page: int, page_size: int) -> list[dict[str, Any]]:
    start = (page - 1) * page_size
    end = start + page_size
    return items[start:end]


def _apply_filters(
    items: list[dict[str, Any]],
    destination: str | None,
    min_price: float | None,
    max_price: float | None,
    min_rating: float | None,
    amenities: list[str] | None,
    sort_by: str | None,
) -> list[dict[str, Any]]:
    filtered = items[:]

    if destination:
        tokens = [t.strip().lower() for t in destination.split(",") if t.strip()]

        def _searchable_text(hotel: dict[str, Any]) -> str:
            return " ".join(
                str(value)
                for value in (
                    hotel.get("city"),
                    hotel.get("hotelName"),
                    hotel.get("name"),
                    hotel.get("place_name"),
                    hotel.get("short_place_name"),
                )
                if value
            ).lower()

        filtered = [h for h in filtered if any(token in _searchable_text(h) for token in tokens)]

    if min_price is not None or max_price is not None:
        tmp = []
        for h in filtered:
            price = h.get("lowestPrice", 0)
            if price == 0:
                tmp.append(h)
                continue
            if min_price is not None and price < min_price:
                continue
            if max_price is not None and price > max_price:
                continue
            tmp.append(h)
        filtered = tmp

    if min_rating is not None:
        filtered = [
            h
            for h in filtered
            if h.get("rating", 0) == 0 or h.get("rating", 0) >= min_rating
        ]

    if amenities:
        filtered = [
            h
            for h in filtered
            if all(
                any(a.lower() in str(ha).lower() for ha in h.get("amenities", []))
                for a in amenities
            )
        ]

    if sort_by == "price_low":
        filtered = _sort_hotels_by_price(filtered, True)
    elif sort_by == "price_high":
        filtered = _sort_hotels_by_price(filtered, False)
    elif sort_by == "rating":
        filtered = _sort_hotels_by_rating(filtered)

    return filtered


def search_hotels(
    api_key: str | None,
    destination: str | None = None,
    check_in_date: str | None = None,
    check_out_date: str | None = None,
    guests: int = 2,
    rooms: int = 1,
    min_price: float | None = None,
    max_price: float | None = None,
    min_rating: float | None = None,
    amenities: list[str] | None = None,
    sort_by: str | None = None,
    page: int = 1,
    page_size: int = 10,
) -> dict[str, Any]:
    has_filters = any(
        value is not None
        for value in (
            min_price,
            max_price,
            min_rating,
            amenities,
            sort_by,
        )
    )
    if not destination and not has_filters:
        return {
            "hotels": [],
            "metadata": {
                "totalResults": 0,
                "page": page,
                "pageSize": page_size,
                "dataSource": "mock",
            },
        }

    data = _load_dataset()
    hotels = data.get("hotels") or []
    filtered = _apply_filters(hotels, destination, min_price, max_price, min_rating, amenities, sort_by)
    paginated = _paginate(filtered, page, page_size)

    return {
        "hotels": paginated,
        "metadata": {
            "totalResults": len(filtered),
            "page": page,
            "pageSize": page_size,
            "dataSource": "mock",
        },
    }


def get_hotel_details(
    api_key: str | None,
    hotel_id: str,
    check_in_date: str | None = None,
    check_out_date: str | None = None,
    guests: int = 2,
) -> dict[str, Any]:
    if check_in_date and check_out_date:
        rooms_out = _rooms_for_hotel(hotel_id)
        hotel = None
        data = _load_dataset()
        match = next(
            (item for item in data.get("hotels", []) if item.get("hotelId") == hotel_id),
            None,
        )
        if match:
            hotel = dict(match)
        if not hotel:
            hotel = {
                "hotelId": hotel_id,
                "hotelName": "Unknown Hotel",
                "description": "",
                "city": "",
                "country": "",
            }
        return {
            "hotel": hotel,
            "rooms": rooms_out,
            "recentReviews": [],
            "nearbyAttractions": [],
        }

    data = _load_dataset()
    match = next(
        (item for item in data.get("hotels", []) if item.get("hotelId") == hotel_id),
        None,
    )
    if match:
        hotel = dict(match)
        return {
            "hotel": hotel,
            "rooms": [],
            "recentReviews": [],
            "nearbyAttractions": [],
        }

    raise HotelNotFoundError("Hotel not found.")


def _rooms_for_guests(
    rooms: list[dict[str, Any]],
    guests: int,
    room_count: int,
) -> list[dict[str, Any]]:
    guests_int = max(int(guests), 0)
    requested_rooms = max(int(room_count), 1)
    filtered: list[dict[str, Any]] = []
    for room in rooms:
        max_occupancy = room.get("maxOccupancy") or 0
        occupancy_int = max(int(max_occupancy), 0)
        if occupancy_int <= 0:
            continue
        required_for_guests = max(1, ceil(guests_int / occupancy_int))
        required_rooms = max(required_for_guests, requested_rooms)
        available_count = int(room.get("availableCount", 1))
        if available_count < required_rooms:
            continue
        room_copy = dict(room)
        room_copy["requiredRooms"] = required_rooms
        filtered.append(room_copy)
    return filtered


def check_availability(
    api_key: str | None,
    hotel_id: str,
    check_in_date: str,
    check_out_date: str,
    guests: int = 2,
    room_count: int = 1,
) -> dict[str, Any]:
    rooms_out = _rooms_for_guests(_rooms_for_hotel(hotel_id), guests, room_count)
    return {
        "hotelId": hotel_id,
        "checkInDate": check_in_date,
        "checkOutDate": check_out_date,
        "availableRooms": rooms_out,
        "totalAvailable": len(rooms_out),
    }


def _error_response(message: str, code: str) -> dict[str, Any]:
    return {
        "message": message,
        "errorCode": code,
    }


@router.get("/hotels/search")
def search_hotels_route(
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
        return search_hotels(
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


@router.get("/hotels/resolve")
def resolve_hotel_id_route(name: str):
    try:
        hotel_id = resolve_hotel_id_by_name(name)
        return {"hotelId": hotel_id}
    except Exception:
        logger.exception("resolve_hotel_id failed")
        return _error_response("Hotel resolve failed", "HOTEL_RESOLVE_FAILED")


@router.get("/hotels/{hotel_id}")
def get_hotel_details_route(
    hotel_id: str,
    checkInDate: str | None = None,
    checkOutDate: str | None = None,
    guests: int = 2,
):
    try:
        return get_hotel_details(
            None,
            hotel_id=hotel_id,
            check_in_date=checkInDate,
            check_out_date=checkOutDate,
            guests=guests,
        )
    except HotelSearchError:
        logger.exception("get_hotel_details failed")
        return _error_response("Hotel details unavailable", "HOTEL_DETAILS_FAILED")


@router.get("/hotels/{hotel_id}/availability")
def get_hotel_availability_route(
    hotel_id: str,
    checkInDate: str,
    checkOutDate: str,
    guests: int = 2,
    roomCount: int = 1,
):
    try:
        return check_availability(
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
