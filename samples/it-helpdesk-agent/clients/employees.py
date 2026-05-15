"""Mocked employee lookup client."""

from __future__ import annotations

from copy import deepcopy
from typing import Any

from ._data import load

_OVERRIDES: dict[str, dict[str, Any]] = {}


class EmployeeNotFound(Exception):
    pass


def lookup_by_email(email: str) -> dict[str, Any]:
    for e in load("employees.json"):
        if e["email"].lower() == email.lower():
            emp = _OVERRIDES.get(e["id"], e)
            return deepcopy(emp)
    raise EmployeeNotFound(f"No employee with email {email!r}")


def get(employee_id: str) -> dict[str, Any]:
    if employee_id in _OVERRIDES:
        return deepcopy(_OVERRIDES[employee_id])
    for e in load("employees.json"):
        if e["id"] == employee_id:
            return deepcopy(e)
    raise EmployeeNotFound(f"No employee with id {employee_id!r}")


def verify(email: str, employee_id: str) -> dict[str, Any]:
    """Verify identity by matching email + employee ID. Returns the employee
    record with ``verified=True`` on success."""
    emp = lookup_by_email(email)
    if emp["id"] != employee_id:
        raise EmployeeNotFound(
            f"Employee ID {employee_id!r} does not match email {email!r}"
        )
    emp["verified"] = True
    _OVERRIDES[employee_id] = emp
    return deepcopy(emp)
