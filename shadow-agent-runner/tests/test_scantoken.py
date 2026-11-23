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

"""amp_shadow_runner.scantoken: reading the scan token's expiry without a signature check."""

import base64
import json
from datetime import UTC, datetime

import pytest

from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.scantoken import DEFAULT_MIN_SECONDS, ScanTokenError, require_lifetime, token_expiry

NOW = 1_800_000_000
INVALID = "AMP_SCAN_TOKEN is not a valid scan token. Copy the whole command from Agent Manager again."


def _b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")


def _token(payload: object, *, raw_payload: str | None = None) -> str:
    """A JWT-shaped token with a dummy signature. ``raw_payload`` replaces the encoded payload segment."""
    header = _b64url(json.dumps({"alg": "RS256", "typ": "JWT"}).encode())
    body = raw_payload if raw_payload is not None else _b64url(json.dumps(payload).encode())
    return f"{header}.{body}.{_b64url(b'dummy-signature')}"


def _clock() -> float:
    return NOW


def _refuse(token: str, min_seconds: int = DEFAULT_MIN_SECONDS) -> ScanTokenError:
    with pytest.raises(ScanTokenError) as info:
        require_lifetime(token, min_seconds, now=_clock)
    exc = info.value
    # Neither the token nor any of its segments may leak through the message, repr or a chained error.
    # Segments under 8 characters are skipped: they match ordinary words in the message by chance.
    for secret in [token, *token.split(".")]:
        if len(secret) >= 8:
            assert secret not in str(exc)
            assert secret not in repr(exc)
    assert exc.__cause__ is None
    assert exc.__context__ is None
    return exc


def test_error_maps_to_config_error() -> None:
    assert ScanTokenError.exit_code is ExitCode.CONFIG_ERROR


def test_valid_token_returns_expiry_in_utc() -> None:
    token = _token({"exp": NOW + 3600, "scope": "amp:shadow-agent:publish"})
    assert require_lifetime(token, now=_clock) == datetime.fromtimestamp(NOW + 3600, tz=UTC)


def test_default_clock_is_wall_time() -> None:
    expires_at = require_lifetime(_token({"exp": 4_000_000_000}))
    assert expires_at.tzinfo is UTC


def test_exactly_min_seconds_left_is_accepted() -> None:
    token = _token({"exp": NOW + DEFAULT_MIN_SECONDS})
    assert require_lifetime(token, now=_clock) == datetime.fromtimestamp(NOW + DEFAULT_MIN_SECONDS, tz=UTC)


def test_one_second_under_min_seconds_is_refused() -> None:
    exc = _refuse(_token({"exp": NOW + DEFAULT_MIN_SECONDS - 1}))
    assert str(exc) == (
        "The scan token expires in less than a minute, and an upload needs at least 1 minute. "
        "Upload the results file in Agent Manager instead (Shadow Agent Discovery, Scans, Upload results)."
    )


def test_custom_min_seconds() -> None:
    token = _token({"exp": NOW + 120})
    assert require_lifetime(token, 60, now=_clock) == datetime.fromtimestamp(NOW + 120, tz=UTC)
    assert "expires in 2 minutes, and an upload needs at least 3 minutes" in str(_refuse(token, 180))


def test_minimum_rounds_up_to_whole_minutes() -> None:
    # 90 seconds needed and 89 left: the minimum reads 2 minutes, never "less than a minute".
    exc = _refuse(_token({"exp": NOW + 89}), 90)
    assert "expires in 1 minute, and an upload needs at least 2 minutes." in str(exc)
    assert "needs at least 1 minute." in str(_refuse(_token({"exp": NOW + 10}), 30))


@pytest.mark.parametrize(
    ("left", "phrase"),
    [(240, "4 minutes"), (60, "1 minute,"), (30, "less than a minute")],
)
def test_less_than_the_minimum_left_is_refused(left: int, phrase: str) -> None:
    assert f"The scan token expires in {phrase}" in str(_refuse(_token({"exp": NOW + left}), 600))


@pytest.mark.parametrize("exp", [NOW, NOW - 1])
def test_expired_token_is_refused(exp: int) -> None:
    exc = _refuse(_token({"exp": exp}))
    assert str(exc).startswith("The scan token has expired. Upload the results file in Agent Manager instead")


@pytest.mark.parametrize("exp", [NOW - 3600, NOW + 3600])
def test_token_expiry_reads_exp_even_when_past(exp: int) -> None:
    assert token_expiry(_token({"exp": exp})) == datetime.fromtimestamp(exp, tz=UTC)


def test_token_expiry_refuses_an_unreadable_token() -> None:
    with pytest.raises(ScanTokenError, match="not a valid scan token"):
        token_expiry("not-a-jwt")


def test_fractional_exp_is_accepted() -> None:
    # RFC 7519 allows a NumericDate with a fraction; 1.9e9 also parses as a float.
    token = _token({"exp": float(NOW + 3600) + 0.5})
    assert require_lifetime(token, now=_clock) == datetime.fromtimestamp(NOW + 3600.5, tz=UTC)
    assert require_lifetime(_token(None, raw_payload=_b64url(b'{"exp": 1.9e9}')), now=_clock) == datetime(
        2030, 3, 17, 17, 46, 40, tzinfo=UTC
    )


def test_missing_exp_is_refused() -> None:
    assert str(_refuse(_token({"sub": "client", "scope": "amp:shadow-agent:publish"}))) == INVALID


@pytest.mark.parametrize(
    "raw_payload",
    [
        b'{"exp": true}',
        b'{"exp": "1800003600"}',
        b'{"exp": null}',
        b'{"exp": [1800003600]}',
        b'{"exp": {"value": 1800003600}}',
        b'{"exp": NaN}',
        b'{"exp": Infinity}',
        b'{"exp": 1e400}',
        b'{"exp": 1' + b"0" * 400 + b"}",
        b'{"exp": 99999999999999}',
    ],
)
def test_unusable_exp_is_refused(raw_payload: bytes) -> None:
    assert str(_refuse(_token(None, raw_payload=_b64url(raw_payload)))) == INVALID


@pytest.mark.parametrize(
    "token",
    [
        "",
        "not-a-jwt",
        "header.payload",
        "header12.payload1.signature.extra123",
        "header..signature",
        " " + _token({"exp": NOW + 3600}),
        _token({"exp": NOW + 3600}) + "\n",
        _token(None, raw_payload="bad+base64/"),
        _token(None, raw_payload="abcde"),  # 5 characters is never valid base64.
        _token(None, raw_payload=_b64url(b"not json")),
        _token(None, raw_payload=_b64url(b"\xff\xfe")),
        _token(None, raw_payload=_b64url(b"[1800003600]")),
        _token(None, raw_payload=_b64url(b"1800003600")),
        _token(None, raw_payload=_b64url(b"[" * 100_000)),
    ],
)
def test_malformed_token_is_refused(token: str) -> None:
    assert str(_refuse(token)) == INVALID
