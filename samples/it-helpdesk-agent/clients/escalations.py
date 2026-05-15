"""Mocked L2 escalation queue."""

from __future__ import annotations

import uuid
from datetime import datetime, timezone
from typing import Any

_QUEUE: list[dict[str, Any]] = []


def create(employee_id: str, summary: str, priority: str, category: str) -> dict[str, Any]:
    ticket = {
        "ticket_id": f"ESC-{uuid.uuid4().hex[:6].upper()}",
        "employee_id": employee_id,
        "summary": summary,
        "priority": priority,
        "category": category,
        "assigned_to": "L2",
        "created_at": datetime.now(timezone.utc).isoformat(),
        "status": "queued",
    }
    _QUEUE.append(ticket)
    return dict(ticket)
