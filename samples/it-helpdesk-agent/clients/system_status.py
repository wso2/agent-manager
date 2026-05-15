"""Mocked system status client."""

from __future__ import annotations

from typing import Any

from ._data import load


def get_all() -> list[dict[str, Any]]:
    return load("system_status.json")


def check_service(service_query: str) -> list[dict[str, Any]]:
    """Return services matching a keyword search."""
    q = service_query.lower().strip()
    results = []
    for s in load("system_status.json"):
        if q in s["service"].lower() or q in s["message"].lower():
            results.append(dict(s))
    return results


def get_degraded() -> list[dict[str, Any]]:
    """Return only services that are not operational."""
    return [
        dict(s) for s in load("system_status.json")
        if s["status"] != "operational"
    ]
