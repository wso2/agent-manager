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
        hotel_name = str(hotel.get("hotel_name") or hotel.get("name") or "")
        candidate = _normalize_name(hotel_name)
        if not candidate:
            continue
        score = SequenceMatcher(None, target, candidate).ratio()
        if score > best_score:
            best_score = score
            best_id = hotel.get("hotel_id")
    if best_id and best_score >= threshold:
        return str(best_id)
    return None


def _rooms_for_hotel(hotel_id: str) -> list[dict[str, Any]]:
    data = _load_dataset()
    rooms = data.get("rooms") or []
    return [room for room in rooms if room.get("hotel_id") == hotel_id]


def _sort_hotels_by_price(items: list[dict[str, Any]], ascending: bool) -> list[dict[str, Any]]:
    return sorted(
        items,
        key=lambda hotel: hotel.get("lowest_price", 0),
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
                    hotel.get("hotel_name"),
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
            price = h.get("lowest_price", 0)
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
                "total_results": 0,
                "page": page,
                "page_size": page_size,
                "data_source": "mock",
            },
        }

    data = _load_dataset()
    hotels = data.get("hotels") or []
    filtered = _apply_filters(hotels, destination, min_price, max_price, min_rating, amenities, sort_by)
    paginated = _paginate(filtered, page, page_size)

    return {
        "hotels": paginated,
        "metadata": {
            "total_results": len(filtered),
            "page": page,
            "page_size": page_size,
            "data_source": "mock",
        },
    }


def get_hotel_details(
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
            (item for item in data.get("hotels", []) if item.get("hotel_id") == hotel_id),
            None,
        )
        if match:
            hotel = dict(match)
        if not hotel:
            hotel = {
                "hotel_id": hotel_id,
                "hotel_name": "Unknown Hotel",
                "description": "",
                "city": "",
                "country": "",
            }
        return {
            "hotel": hotel,
            "rooms": rooms_out,
            "recent_reviews": [],
            "nearby_attractions": [],
        }

    data = _load_dataset()
    match = next(
        (item for item in data.get("hotels", []) if item.get("hotel_id") == hotel_id),
        None,
    )
    if match:
        hotel = dict(match)
        return {
            "hotel": hotel,
            "rooms": [],
            "recent_reviews": [],
            "nearby_attractions": [],
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
        max_occupancy = room.get("max_occupancy") or 0
        occupancy_int = max(int(max_occupancy), 0)
        if occupancy_int <= 0:
            continue
        required_for_guests = max(1, ceil(guests_int / occupancy_int))
        required_rooms = max(required_for_guests, requested_rooms)
        available_count = int(room.get("available_count", 1))
        if available_count < required_rooms:
            continue
        room_copy = dict(room)
        room_copy["required_rooms"] = required_rooms
        filtered.append(room_copy)
    return filtered


def check_availability(
    hotel_id: str,
    check_in_date: str,
    check_out_date: str,
    guests: int = 2,
    room_count: int = 1,
) -> dict[str, Any]:
    data = _load_dataset()
    match = next(
        (item for item in data.get("hotels", []) if item.get("hotel_id") == hotel_id),
        None,
    )
    hotel_name = str(match.get("hotel_name") or match.get("name") or "") if match else ""
    rooms_out = _rooms_for_guests(_rooms_for_hotel(hotel_id), guests, room_count)
    return {
        "hotel_name": hotel_name,
        "check_in_date": check_in_date,
        "check_out_date": check_out_date,
        "available_rooms": rooms_out,
        "total_available": len(rooms_out),
    }


def _error_response(message: str, code: str) -> dict[str, Any]:
    return {
        "message": message,
        "error_code": code,
    }


@router.get("/hotels/search")
def search_hotels_route(
    destination: str | None = None,
    check_in_date: str | None = None,
    check_out_date: str | None = None,
    guests: int = 2,
    rooms: int = 1,
    min_price: float | None = None,
    max_price: float | None = None,
    min_rating: float | None = None,
    sort_by: str | None = None,
    page: int = 1,
    page_size: int = 10,
):
    try:
        return search_hotels(
            destination=destination,
            check_in_date=check_in_date,
            check_out_date=check_out_date,
            guests=guests,
            rooms=rooms,
            min_price=min_price,
            max_price=max_price,
            min_rating=min_rating,
            amenities=None,
            sort_by=sort_by,
            page=page,
            page_size=page_size,
        )
    except HotelSearchError:
        logger.exception("search_hotels failed")
        return _error_response("Hotel search failed", "HOTEL_SEARCH_FAILED")


@router.get("/hotels/resolve")
def resolve_hotel_id_route(name: str):
    try:
        hotel_id = resolve_hotel_id_by_name(name)
        return {"hotel_id": hotel_id}
    except Exception:
        logger.exception("resolve_hotel_id failed")
        return _error_response("Hotel resolve failed", "HOTEL_RESOLVE_FAILED")


@router.get("/hotels/{hotel_id}")
def get_hotel_details_route(
    hotel_id: str,
    check_in_date: str | None = None,
    check_out_date: str | None = None,
    guests: int = 2,
):
    try:
        return get_hotel_details(
            hotel_id=hotel_id,
            check_in_date=check_in_date,
            check_out_date=check_out_date,
            guests=guests,
        )
    except HotelSearchError:
        logger.exception("get_hotel_details failed")
        return _error_response("Hotel details unavailable", "HOTEL_DETAILS_FAILED")


@router.get("/hotels/{hotel_id}/availability")
def get_hotel_availability_route(
    hotel_id: str,
    check_in_date: str,
    check_out_date: str,
    guests: int = 2,
    room_count: int = 1,
):
    try:
        return check_availability(
            hotel_id=hotel_id,
            check_in_date=check_in_date,
            check_out_date=check_out_date,
            guests=guests,
            room_count=room_count,
        )
    except HotelSearchError:
        logger.exception("get_hotel_availability failed")
        return _error_response("Hotel availability unavailable", "HOTEL_AVAILABILITY_FAILED")
