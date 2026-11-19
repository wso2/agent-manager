# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.

"""Read the expiry of the scan token (``AMP_SCAN_TOKEN``).

The token is a JWT that Agent Manager verifies when the runner publishes. The
runner cannot check its signature and does not try: it only reads ``exp`` so it
can refuse to start a scan that would outlive the token.

The token, or any part of it, never appears in an error message.
"""

import base64
import json
import math
import re
import time
from collections.abc import Callable
from datetime import UTC, datetime
from typing import Any, ClassVar

from amp_shadow_runner.exitcodes import ExitCode

DEFAULT_MIN_SECONDS = 600
"""A scan needs at least this many seconds of token lifetime left (10 minutes)."""

_JWT = re.compile(r"[A-Za-z0-9_-]+\.([A-Za-z0-9_-]+)\.[A-Za-z0-9_-]+")
_REGENERATE = "Generate a new command in Agent Manager and run it again."
_INVALID = "AMP_SCAN_TOKEN is not a valid scan token. Copy the whole command from Agent Manager again."


class ScanTokenError(Exception):
    """The scan token is unusable. The message is safe to show and never contains the token."""

    exit_code: ClassVar[ExitCode] = ExitCode.CONFIG_ERROR


def _payload(token: str) -> Any:
    """Return the decoded JWT payload, or None when the token is not a readable JWT."""
    match = _JWT.fullmatch(token)
    if match is None:
        return None
    segment = match.group(1)
    try:
        return json.loads(base64.urlsafe_b64decode(segment + "=" * (-len(segment) % 4)))
    except (ValueError, RecursionError):  # binascii.Error, UnicodeDecodeError and JSONDecodeError are ValueErrors.
        return None


def _minutes(minutes: int) -> str:
    if minutes < 1:
        return "less than a minute"
    return "1 minute" if minutes == 1 else f"{minutes} minutes"


def require_lifetime(
    token: str, min_seconds: int = DEFAULT_MIN_SECONDS, *, now: Callable[[], float] = time.time
) -> datetime:
    """Return the token's expiry (UTC) when at least ``min_seconds`` remain.

    The token is used exactly as given: surrounding whitespace is refused, so
    the caller decides whether to strip it, and checks for an unset or empty
    variable itself to give a more specific message.

    ``exp`` may be an integer or a finite number with a fraction (RFC 7519
    NumericDate); a boolean or any other type is refused. A token with exactly
    ``min_seconds`` left is accepted.

    Raises ScanTokenError (exit code CONFIG_ERROR) when the token is not a JWT,
    its payload is not a JSON object, ``exp`` is missing or not a number, or
    too little time remains.
    """
    # Errors are raised outside except blocks so no decoding exception, which
    # can hold part of the token, is chained onto them.
    payload = _payload(token)
    if not isinstance(payload, dict):
        raise ScanTokenError(_INVALID)
    exp = payload.get("exp")
    if isinstance(exp, bool) or not isinstance(exp, int | float):
        raise ScanTokenError(_INVALID)
    try:
        expires_at: datetime | None = datetime.fromtimestamp(exp, tz=UTC)
    except (OverflowError, OSError, ValueError):  # Out of datetime's range, or NaN or Infinity.
        expires_at = None
    if expires_at is None:
        raise ScanTokenError(_INVALID)
    remaining = exp - now()
    if remaining <= 0:
        raise ScanTokenError(
            f"The scan token has expired. {_REGENERATE} If you generated it just now, check this machine's clock."
        )
    if remaining < min_seconds:
        # Time left rounds down and the minimum rounds up, so the message never contradicts itself.
        left = _minutes(int(remaining // 60))
        needed = _minutes(max(1, math.ceil(min_seconds / 60)))
        raise ScanTokenError(f"The scan token expires in {left}, and a scan needs at least {needed}. {_REGENERATE}")
    return expires_at
