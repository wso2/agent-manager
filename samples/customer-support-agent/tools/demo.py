"""Fictional travel inventory and session-local booking state."""
from datetime import date, datetime, timedelta, timezone
from threading import Lock

DEMO_PASSENGER = "DEMO-001"
DEMO_TICKET = "DEMO-TICKET-001"
_DEPARTURE = datetime.now(timezone.utc).replace(hour=10, minute=0, second=0, microsecond=0) + timedelta(days=7)
FLIGHTS = {
    1: dict(flight_id=1, flight_no="DEMO101", departure_airport="ZRH", arrival_airport="GVA",
            scheduled_departure=_DEPARTURE, scheduled_arrival=_DEPARTURE + timedelta(hours=1)),
    2: dict(flight_id=2, flight_no="DEMO102", departure_airport="ZRH", arrival_airport="GVA",
            scheduled_departure=_DEPARTURE + timedelta(hours=3), scheduled_arrival=_DEPARTURE + timedelta(hours=4)),
}
CATALOG = {
    "hotel": {1: dict(id=1, name="Demo Lakeside Hotel", location="Geneva", price_tier="Midscale")},
    "car rental": {1: dict(id=1, name="Demo Compact Car", location="Geneva", price_tier="Economy")},
    "excursion": {1: dict(id=1, name="Demo Lake Walk", location="Geneva", keywords="lake walking sightseeing")},
}
_lock = Lock()
_tickets = {}
_bookings = {}


def session(config):
    return config["configurable"]["thread_id"]


def day(value: date | datetime) -> date:
    return value.date() if isinstance(value, datetime) else value


def valid_dates(start, end):
    return start is None or end is None or day(start) <= day(end)


def search(kind, **filters):
    return [
        {**item, "simulated": True}
        for item in CATALOG[kind].values()
        if all(value is None or value.casefold() in item[key].casefold() for key, value in filters.items())
    ]


def tickets(config):
    if config.get("configurable", {}).get("passenger_id") != DEMO_PASSENGER:
        return []
    with _lock:
        flight_id = _tickets.get(session(config), 1)
    if flight_id is None:
        return []
    return [dict(FLIGHTS[flight_id], ticket_no=DEMO_TICKET, passenger_id=DEMO_PASSENGER, simulated=True)]


def change_ticket(config, ticket_no, flight_id):
    if config.get("configurable", {}).get("passenger_id") != DEMO_PASSENGER or ticket_no != DEMO_TICKET:
        return "Demo ticket not found for this passenger."
    if flight_id is not None and flight_id not in FLIGHTS:
        return "Demo flight not found."
    with _lock:
        key = session(config)
        if _tickets.get(key, 1) is None:
            return "Demo ticket is already cancelled."
        _tickets[key] = flight_id
    if flight_id is None:
        return "Simulated cancellation complete."
    return f"Simulated ticket change to {FLIGHTS[flight_id]['flight_no']} complete."


def change_booking(config, kind, item_id, action, **fields):
    if item_id not in CATALOG[kind]:
        return f"Demo {kind} not found."
    key = (session(config), kind, item_id)
    with _lock:
        current = _bookings.get(key)
        if action == "book":
            if current is not None:
                return f"Demo {kind} is already booked in this session."
            _bookings[key] = {}
        elif current is None:
            return f"No demo {kind} booking in this session."
        elif action == "cancel":
            del _bookings[key]
        else:
            updated = {**current, **{name: value for name, value in fields.items() if value is not None}}
            if not valid_dates(updated.get("start"), updated.get("end")):
                return "The end date must be on or after the start date."
            _bookings[key] = updated
    return f"Simulated {kind} {action} complete."
