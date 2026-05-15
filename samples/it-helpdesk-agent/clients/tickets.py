"""Mocked ticket management client."""

from __future__ import annotations

import uuid
from copy import deepcopy
from datetime import datetime, timezone
from typing import Any

from ._data import load

_CREATED: list[dict[str, Any]] = []


class TicketNotFound(Exception):
    pass


def get_by_employee(employee_id: str) -> list[dict[str, Any]]:
    """Return all tickets for an employee (from seed data + runtime created)."""
    seed = [deepcopy(t) for t in load("tickets.json") if t["employee_id"] == employee_id]
    runtime = [deepcopy(t) for t in _CREATED if t["employee_id"] == employee_id]
    all_tickets = seed + runtime
    all_tickets.sort(key=lambda t: t["created_at"], reverse=True)
    return all_tickets


def get(ticket_id: str) -> dict[str, Any]:
    for t in load("tickets.json"):
        if t["id"] == ticket_id:
            return deepcopy(t)
    for t in _CREATED:
        if t["id"] == ticket_id:
            return deepcopy(t)
    raise TicketNotFound(f"No ticket with id {ticket_id!r}")


def create(
    employee_id: str,
    subject: str,
    category: str,
    priority: str,
    description: str,
) -> dict[str, Any]:
    ticket = {
        "id": f"TKT-{uuid.uuid4().hex[:8].upper()}",
        "employee_id": employee_id,
        "subject": subject,
        "category": category,
        "priority": priority,
        "status": "open",
        "assigned_to": "L1",
        "description": description,
        "created_at": datetime.now(timezone.utc).isoformat(),
    }
    _CREATED.append(ticket)
    return deepcopy(ticket)


def find_duplicates(employee_id: str, category: str) -> list[dict[str, Any]]:
    """Find open tickets for the same employee and category."""
    all_tickets = get_by_employee(employee_id)
    return [
        t for t in all_tickets
        if t["category"] == category and t["status"] in ("open", "in_progress")
    ]
