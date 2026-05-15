"""Mocked password reset client."""

from __future__ import annotations

import uuid
from datetime import datetime, timezone
from typing import Any

_LOG: list[dict[str, Any]] = []


class AdminResetBlocked(Exception):
    def __init__(self, employee_id: str):
        super().__init__(
            f"Password reset for admin account {employee_id} is blocked. "
            f"Admin resets must be handled by L2 Security support."
        )
        self.employee_id = employee_id


class IdentityNotVerified(Exception):
    def __init__(self):
        super().__init__(
            "Identity has not been verified. The employee must provide their "
            "email and employee ID before a password reset can be performed."
        )


def reset(employee_id: str, is_admin: bool, verified: bool) -> dict[str, Any]:
    if not verified:
        raise IdentityNotVerified()
    if is_admin:
        raise AdminResetBlocked(employee_id=employee_id)
    receipt = {
        "reset_id": f"RST-{uuid.uuid4().hex[:8].upper()}",
        "employee_id": employee_id,
        "status": "completed",
        "temporary_password_sent": True,
        "reset_at": datetime.now(timezone.utc).isoformat(),
        "message": "Temporary password sent to registered email. Must be changed on next login.",
    }
    _LOG.append(receipt)
    return dict(receipt)
