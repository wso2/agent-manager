from __future__ import annotations

import json
import re
import threading
from difflib import SequenceMatcher
from pathlib import Path
from typing import Any


class HotelSearchError(RuntimeError):
    pass


class HotelNotFoundError(HotelSearchError):
    pass


_hotel_cache_lock = threading.Lock()
_hotel_cache: dict[str, dict[str, Any]] = {}
_dataset_lock = threading.Lock()
_dataset_cache: dict[str, Any] | None = None
_dataset_path = Path(__file__).resolve().parent / "data" / "mock_dataset.json"


def _parse_float(value: Any) -> float:
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def _normalize_hotel(hotel: dict[str, Any]) -> dict[str, Any]:
    normalized = dict(hotel)
    hotel_id = normalized.get("hotelId")
    if not hotel_id:
        for key in ("hotel_id", "id", "hotel_key", "key"):
            value = normalized.get(key)
            if value:
                hotel_id = str(value)
                break
    if hotel_id:
        normalized["hotelId"] = hotel_id
    if not normalized.get("hotelName"):
        for key in ("name", "hotel_name", "hotel"):
            value = normalized.get(key)
            if value:
                normalized["hotelName"] = value
                break
    return normalized


def _cache_hotels(hotels: list[dict[str, Any]]) -> None:
    with _hotel_cache_lock:
        for hotel in hotels:
            hotel_id = hotel.get("hotelId")
            if hotel_id:
                _hotel_cache[hotel_id] = hotel


def get_cached_hotel(hotel_id: str) -> dict[str, Any] | None:
    with _hotel_cache_lock:
        return dict(_hotel_cache.get(hotel_id)) if hotel_id in _hotel_cache else None


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


def _mock_rates_for_hotel(hotel_id: str) -> list[dict[str, Any]]:
    data = _load_dataset()
    rooms = data.get("rooms") or []
    rates: list[dict[str, Any]] = []
    for room in rooms:
        if room.get("hotelId") != hotel_id:
            continue
        rates.append(
            {
                "code": room.get("roomId"),
                "name": room.get("roomName"),
                "rate": room.get("pricePerNight"),
                "url": room.get("bookingUrl") or "",
            }
        )
    return rates


def _rooms_for_hotel(hotel_id: str) -> list[dict[str, Any]]:
    data = _load_dataset()
    rooms = data.get("rooms") or []
    return [room for room in rooms if room.get("hotelId") == hotel_id]


def _build_rooms_from_rates(
    hotel_id: str,
    rates: list[dict[str, Any]],
    guests: int,
) -> list[dict[str, Any]]:
    rooms_out: list[dict[str, Any]] = []
    for rate in rates:
        rate_code = rate.get("code") or "OTA"
        rate_name = rate.get("name") or "OTA"
        booking_url = rate.get("link") or rate.get("url") or ""
        rooms_out.append(
            {
                "roomId": f"{hotel_id}_{rate_code}",
                "hotelId": hotel_id,
                "roomType": "Standard Room",
                "roomName": f"Room via {rate_name}",
                "description": f"Book through {rate_name}",
                "maxOccupancy": guests,
                "pricePerNight": _parse_float(rate.get("rate")),
                "bookingUrl": booking_url,
                "images": [],
                "amenities": [],
                "availableCount": 1,
            }
        )
    return rooms_out


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
    normalized = [_normalize_hotel(hotel) for hotel in hotels]
    _cache_hotels(normalized)

    filtered = _apply_filters(normalized, destination, min_price, max_price, min_rating, amenities, sort_by)
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
    cached = get_cached_hotel(hotel_id)
    if check_in_date and check_out_date:
        rooms_out = _rooms_for_hotel(hotel_id)
        hotel = cached or get_cached_hotel(hotel_id)
        if not hotel:
            data = _load_dataset()
            match = next(
                (item for item in data.get("hotels", []) if item.get("hotelId") == hotel_id),
                None,
            )
            if match:
                hotel = _normalize_hotel(match)
                _cache_hotels([hotel])
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

    if cached:
        return {
            "hotel": cached,
            "rooms": [],
            "recentReviews": [],
            "nearbyAttractions": [],
        }
    data = _load_dataset()
    match = next(
        (item for item in data.get("hotels", []) if item.get("hotelId") == hotel_id),
        None,
    )
    if match:
        hotel = _normalize_hotel(match)
        _cache_hotels([hotel])
        return {
            "hotel": hotel,
            "rooms": [],
            "recentReviews": [],
            "nearbyAttractions": [],
        }

    raise HotelNotFoundError("Hotel not found.")


def check_availability(
    api_key: str | None,
    hotel_id: str,
    check_in_date: str,
    check_out_date: str,
    guests: int = 2,
    room_count: int = 1,
) -> dict[str, Any]:
    rooms_out = _rooms_for_hotel(hotel_id)
    return {
        "hotelId": hotel_id,
        "checkInDate": check_in_date,
        "checkOutDate": check_out_date,
        "availableRooms": rooms_out,
        "totalAvailable": len(rooms_out),
    }


def fetch_room_rates(
    api_key: str | None,
    hotel_id: str,
    check_in_date: str | None,
    check_out_date: str | None,
    guests: int | None,
    room_count: int | None,
) -> list[dict[str, Any]]:
    if not hotel_id or not check_in_date or not check_out_date:
        return []
    return _mock_rates_for_hotel(hotel_id)


def build_rooms_from_rates(
    hotel_id: str,
    rates: list[dict[str, Any]],
    guests: int,
) -> list[dict[str, Any]]:
    return _build_rooms_from_rates(hotel_id, rates, guests)
