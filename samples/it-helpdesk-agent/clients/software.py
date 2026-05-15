"""Mocked software access request client."""

from __future__ import annotations

import uuid
from datetime import datetime, timezone
from typing import Any

from ._data import load

_LOG: list[dict[str, Any]] = []


class SoftwareNotFound(Exception):
    pass


class DepartmentNotAllowed(Exception):
    def __init__(self, software_name: str, department: str):
        super().__init__(
            f"Access to {software_name!r} is not available for the {department!r} department. "
            f"Contact your manager if you believe this is an error."
        )


class ApprovalRequired(Exception):
    def __init__(self, software_name: str, manager_id: str):
        super().__init__(
            f"Access to {software_name!r} requires manager approval. "
            f"A request has been sent to manager {manager_id} for review."
        )


def get_catalog() -> list[dict[str, Any]]:
    return load("software_catalog.json")


def lookup(software_name: str) -> dict[str, Any]:
    for sw in load("software_catalog.json"):
        if sw["name"].lower() == software_name.lower():
            return dict(sw)
    raise SoftwareNotFound(f"No software named {software_name!r} in catalog")


def request_access(
    employee_id: str,
    department: str,
    manager_id: str | None,
    software_name: str,
) -> dict[str, Any]:
    sw = lookup(software_name)

    if sw["allowed_departments"] is not None and department not in sw["allowed_departments"]:
        raise DepartmentNotAllowed(software_name=sw["name"], department=department)

    if sw["requires_approval"]:
        ticket = {
            "request_id": f"SAR-{uuid.uuid4().hex[:6].upper()}",
            "employee_id": employee_id,
            "software": sw["name"],
            "status": "pending_approval",
            "approver": manager_id or "unknown",
            "requested_at": datetime.now(timezone.utc).isoformat(),
        }
        _LOG.append(ticket)
        raise ApprovalRequired(software_name=sw["name"], manager_id=manager_id or "unknown")

    ticket = {
        "request_id": f"SAR-{uuid.uuid4().hex[:6].upper()}",
        "employee_id": employee_id,
        "software": sw["name"],
        "status": "provisioned",
        "requested_at": datetime.now(timezone.utc).isoformat(),
    }
    _LOG.append(ticket)
    return dict(ticket)
