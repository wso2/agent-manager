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

"""A small HTTP client on the standard library, for the Git hosts, Agent Manager and the LLM.

It sends JSON requests and downloads archives with a timeout on every request,
retries what is safe to retry, and caps how much it reads. Redirects are followed
here rather than by urllib, so credentials never reach another origin and an
https request is never redirected to http.

Error messages never contain request or response headers, credentials, or a
URL's user info, query or fragment (signed download URLs carry a token in the
query). Nothing here logs.
"""

import http.client
import json
import random
import socket
import ssl
import time
import urllib.error
import urllib.request
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC
from email.utils import parsedate_to_datetime
from http import HTTPStatus
from pathlib import Path
from typing import Any, ClassVar
from urllib.parse import urljoin, urlsplit

from amp_shadow_runner._version import __version__
from amp_shadow_runner.exitcodes import ExitCode

USER_AGENT = f"amp-shadow-runner/{__version__}"
DEFAULT_TIMEOUT = 30.0
"""Seconds to wait to connect, and for each read."""
DEFAULT_MAX_ATTEMPTS = 4
"""Attempts in all for a retried request (the first try and three retries)."""
DEFAULT_MAX_JSON_BYTES = 16 * 1024 * 1024
DEFAULT_MAX_RETRY_AFTER = 60.0
"""The longest wait, in seconds, that a ``Retry-After`` header can ask for."""
MAX_REDIRECTS = 5

_BACKOFF_BASE = 1.0
_BACKOFF_CAP = 30.0
_CHUNK = 64 * 1024
_MAX_ERROR_BODY = 64 * 1024
_MAX_SERVER_MESSAGE = 200
_RETRY_STATUSES = frozenset({429, 500, 502, 503, 504})
_REDIRECT_STATUSES = frozenset({301, 302, 303, 307, 308})
# Dropped on a cross-origin redirect. Send credentials in Authorization: only GETs follow
# redirects, so another credential header (such as x-api-key) is safe only on a POST.
_CREDENTIAL_HEADERS = frozenset({"authorization", "proxy-authorization", "cookie"})


class HttpError(Exception):
    """A request failed. The message is safe to show.

    ``status`` is the HTTP status, or None when there was no usable response
    (network error, timeout, refused redirect, invalid URL). ``server_message``
    is the response's JSON ``message`` field, cleaned and truncated, when it has
    one. ``headers`` are the failed response's headers, for callers that need
    rate-limit headers; never print them. ``retryable`` says whether the same
    request may succeed later.
    """

    exit_code: ClassVar[ExitCode] = ExitCode.RUNTIME_ERROR

    def __init__(
        self,
        message: str,
        *,
        status: int | None = None,
        headers: http.client.HTTPMessage | None = None,
        server_message: str | None = None,
        retryable: bool = False,
    ) -> None:
        super().__init__(message)
        self.status = status
        self.headers = headers
        self.server_message = server_message
        self.retryable = retryable


@dataclass(frozen=True)
class JsonResponse:
    """A successful JSON response. ``data`` is None when the body was empty."""

    status: int
    headers: http.client.HTTPMessage
    """Case-insensitive, such as ``headers.get("Link")``."""
    data: Any


@dataclass(frozen=True)
class DownloadResponse:
    """A successful download, saved to the requested path."""

    status: int
    headers: http.client.HTTPMessage
    size: int
    """Bytes written."""


class HttpClient:
    """Sends requests with the runner's ``User-Agent``, verified TLS and the standard proxy variables.

    GET requests and downloads are retried on network errors and on HTTP 429,
    500, 502, 503 and 504, waiting with capped exponential backoff and jitter, or
    longer when ``Retry-After`` asks. A request whose ``Retry-After`` is longer
    than ``max_retry_after`` fails at once instead of waiting. A POST is
    retried only when the caller passes ``retry=True``. Only GET requests and
    downloads follow redirects.

    ``sleep``, ``now`` and ``jitter`` (a number in [0, 1)) default to
    ``time.sleep``, ``time.time`` and ``random.random``; tests replace them.
    """

    def __init__(
        self,
        *,
        timeout: float = DEFAULT_TIMEOUT,
        max_attempts: int = DEFAULT_MAX_ATTEMPTS,
        max_json_bytes: int = DEFAULT_MAX_JSON_BYTES,
        max_retry_after: float = DEFAULT_MAX_RETRY_AFTER,
        sleep: Callable[[float], None] = time.sleep,
        now: Callable[[], float] = time.time,
        jitter: Callable[[], float] = random.random,
    ) -> None:
        self._timeout = timeout
        self._max_attempts = max(1, max_attempts)
        self._max_json_bytes = max_json_bytes
        self._max_retry_after = max_retry_after
        self._sleep = sleep
        self._now = now
        self._jitter = jitter
        # Only these handlers: no urllib redirect or error processing (redirects are
        # handled in _attempt), and no file:, ftp: or data: URLs. ProxyHandler reads
        # HTTP(S)_PROXY and NO_PROXY; the default TLS context honours SSL_CERT_FILE and SSL_CERT_DIR.
        self._opener = urllib.request.OpenerDirector()
        self._opener.addheaders = []
        for handler in (
            urllib.request.ProxyHandler(),
            urllib.request.HTTPHandler(),
            urllib.request.HTTPSHandler(context=ssl.create_default_context()),
        ):
            self._opener.add_handler(handler)

    def get_json(self, url: str, *, headers: Mapping[str, str] | None = None) -> JsonResponse:
        """GET ``url`` and parse its JSON body. ``Accept`` defaults to ``application/json``."""
        return self._send("GET", url, None, _headers("application/json", headers), True, self._read_json)

    def post_json(
        self, url: str, body: Any, *, headers: Mapping[str, str] | None = None, retry: bool = False
    ) -> JsonResponse:
        """POST ``body`` as UTF-8 JSON and parse the JSON response.

        Not retried unless ``retry`` is True: only opt in when the server treats
        a repeated request as the same one.
        """
        data = json.dumps(body, ensure_ascii=False, allow_nan=False).encode("utf-8")
        sent = _headers("application/json", headers, content_type="application/json")
        return self._send("POST", url, data, sent, retry, self._read_json)

    def download(
        self, url: str, dest: Path, *, max_bytes: int, headers: Mapping[str, str] | None = None
    ) -> DownloadResponse:
        """GET ``url`` and stream the body to ``dest``, replacing it.

        Fails when the body is larger than ``max_bytes``. A partial file is
        removed on any failure. An error opening or writing ``dest`` is raised
        as the OSError itself.
        """

        def save(response: http.client.HTTPResponse, method: str, current: str) -> DownloadResponse:
            return _save(response, method, current, dest, max_bytes)

        return self._send("GET", url, None, _headers("*/*", headers), True, save)

    def _send[T](
        self,
        method: str,
        url: str,
        data: bytes | None,
        headers: dict[str, str],
        retry: bool,
        read: Callable[[http.client.HTTPResponse, str, str], T],
    ) -> T:
        _check_url(url)
        _check_headers(headers, method, url)
        attempts = self._max_attempts if retry else 1
        attempt = 1
        while True:
            try:
                return self._attempt(method, url, data, headers, read)
            except HttpError as exc:
                error = exc
            delay = self._delay(attempt, error) if error.retryable and attempt < attempts else None
            if delay is None:
                raise error  # Outside the except block, so nothing is chained onto it.
            self._sleep(delay)
            attempt += 1

    def _attempt[T](
        self,
        method: str,
        url: str,
        data: bytes | None,
        headers: dict[str, str],
        read: Callable[[http.client.HTTPResponse, str, str], T],
    ) -> T:
        origin = _origin(url)
        current, sent = url, headers
        for _ in range(MAX_REDIRECTS + 1):
            with self._open(method, current, data, sent) as response:
                status = response.status
                if method == "GET" and status in _REDIRECT_STATUSES:
                    location = response.headers.get("Location")
                    if not location:
                        raise HttpError(f"{method} {safe_url(current)} failed: HTTP {status} without a Location")
                    current = redirect_target(current, location)
                    if _origin(current) != origin:
                        # Once dropped, credentials stay dropped for the rest of the chain.
                        sent = {k: v for k, v in sent.items() if k.lower() not in _CREDENTIAL_HEADERS}
                    continue
                if 200 <= status < 300:
                    return read(response, method, current)
                raise _status_error(method, current, response)
        raise HttpError(f"{method} {safe_url(url)} failed: more than {MAX_REDIRECTS} redirects")

    def _open(self, method: str, url: str, data: bytes | None, headers: dict[str, str]) -> http.client.HTTPResponse:
        request = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            response: http.client.HTTPResponse = self._opener.open(request, timeout=self._timeout)
            return response
        except (OSError, http.client.HTTPException) as exc:
            failure = _network_error(method, url, exc)
        raise failure

    def _read_json(self, response: http.client.HTTPResponse, method: str, url: str) -> JsonResponse:
        body = _read_capped(response, self._max_json_bytes, method, url)
        if not body.strip():
            return JsonResponse(response.status, response.headers, None)
        try:
            data = json.loads(body)
        except (ValueError, RecursionError):  # UnicodeDecodeError and JSONDecodeError are ValueErrors.
            data = _NOT_JSON
        if data is _NOT_JSON:
            raise HttpError(f"{method} {safe_url(url)} failed: the response is not valid JSON", status=response.status)
        return JsonResponse(response.status, response.headers, data)

    def _delay(self, attempt: int, error: HttpError) -> float | None:
        """Seconds to wait before the next attempt, or None when the server asks for longer than the cap."""
        backoff = min(_BACKOFF_CAP, _BACKOFF_BASE * 2.0 ** (attempt - 1)) * (0.5 + 0.5 * self._jitter())
        asked = _retry_after(error.headers.get("Retry-After"), self._now) if error.headers is not None else None
        if asked is None:
            return backoff
        return None if asked > self._max_retry_after else max(asked, backoff)


_NOT_JSON = object()


def safe_url(url: str) -> str:
    """Return ``url`` without user info, query or fragment, safe to show in a message."""
    try:
        parts = urlsplit(url)
    except ValueError:  # An invalid IPv6 address.
        return "an invalid address"
    host = parts.hostname or ""
    if ":" in host:
        host = f"[{host}]"
    try:
        port = f":{parts.port}" if parts.port is not None else ""
    except ValueError:
        port = ""
    return _printable(f"{parts.scheme}://{host}{port}{parts.path}")


def redirect_target(current: str, location: str) -> str:
    """Return the absolute URL a redirect points to, refusing https to http and non-http schemes."""
    try:
        target = urljoin(current, location.strip()).split("#", 1)[0]
        scheme = urlsplit(target).scheme.lower()
    except ValueError:  # An invalid IPv6 address.
        scheme = ""
    if scheme not in ("http", "https"):
        raise HttpError(f"GET {safe_url(current)} failed: redirected to an unsupported address")
    if urlsplit(current).scheme.lower() == "https" and scheme == "http":
        raise HttpError(f"GET {safe_url(current)} failed: refused a redirect from https to http")
    _check_url(target)
    return target


def _origin(url: str) -> tuple[str, str, int]:
    parts = urlsplit(url)
    scheme = parts.scheme.lower()
    return scheme, parts.hostname or "", parts.port or (443 if scheme == "https" else 80)


def _printable(text: str) -> str:
    return "".join(c if c.isprintable() else " " for c in text)


def _url_problem(url: str) -> str | None:
    if not url.isascii() or any(c.isspace() or not c.isprintable() for c in url):
        return "it contains spaces, control characters or non-ASCII characters"
    try:
        parts = urlsplit(url)
        parts.port  # noqa: B018 (raises ValueError for an invalid port)
    except ValueError:
        return "it is not a valid address"
    if parts.scheme.lower() not in ("http", "https") or not parts.hostname:
        return "only http and https addresses are supported"
    if parts.username is not None or parts.password is not None:
        return "it must not contain a user name or password"
    return None


def _check_url(url: str) -> None:
    problem = _url_problem(url)
    if problem is not None:
        raise HttpError(f"Refused the address {safe_url(url)}: {problem}")


def _check_headers(headers: Mapping[str, str], method: str, url: str) -> None:
    for value in headers.values():
        # http.client would raise with the value in the message; refuse first, without it.
        if not all(c == "\t" or " " <= c <= "~" for c in value):
            raise HttpError(f"{method} {safe_url(url)} refused: a request header has an invalid value")


def _headers(accept: str, extra: Mapping[str, str] | None, *, content_type: str | None = None) -> dict[str, str]:
    """Merge the caller's headers over the defaults, case-insensitively. ``User-Agent`` is always ours."""
    merged = {"Accept": accept}
    if content_type is not None:
        merged["Content-Type"] = content_type
    for name, value in {**(extra or {}), "User-Agent": USER_AGENT}.items():
        merged = {k: v for k, v in merged.items() if k.lower() != name.lower()}
        merged[name] = value
    return merged


def _network_error(method: str, url: str, exc: BaseException) -> HttpError:
    """Describe a network failure by its type only: exception text can hold proxy or host details."""
    reason: object = exc.reason if isinstance(exc, urllib.error.URLError) else exc
    retryable = True
    if isinstance(reason, ssl.SSLCertVerificationError):
        text, retryable = "the server's TLS certificate could not be verified", False
    elif isinstance(reason, ssl.SSLError):  # Such as SSLEOFError when a proxy drops the connection.
        text = "TLS error"
    elif isinstance(reason, TimeoutError):
        text = "timed out"
    elif isinstance(reason, socket.gaierror):
        text = "host not found"
    elif isinstance(reason, ConnectionRefusedError):
        text = "connection refused"
    elif isinstance(reason, ConnectionError):
        text = "connection lost"
    elif isinstance(reason, http.client.HTTPException):
        text = "invalid or incomplete response"
    else:
        text = "network error"
    return HttpError(f"{method} {safe_url(url)} failed: {text}", retryable=retryable)


def _read(response: http.client.HTTPResponse, size: int, method: str, url: str) -> bytes:
    try:
        return response.read(size)
    except (OSError, http.client.HTTPException) as exc:
        failure = _network_error(method, url, exc)
    raise failure


def _content_length(response: http.client.HTTPResponse) -> int | None:
    if response.chunked:  # http.client ignores Content-Length on a chunked body.
        return None
    value = response.headers.get("Content-Length", "").strip()
    return int(value) if value.isdecimal() and value.isascii() else None


def _incomplete(method: str, url: str) -> HttpError:
    # http.client's read(amt) returns b"" at a short body instead of raising IncompleteRead.
    return HttpError(f"{method} {safe_url(url)} failed: invalid or incomplete response", retryable=True)


def _too_large(method: str, url: str, limit: int, status: int) -> HttpError:
    return HttpError(f"{method} {safe_url(url)} failed: the response is larger than {limit} bytes", status=status)


def _read_capped(response: http.client.HTTPResponse, limit: int, method: str, url: str) -> bytes:
    length = _content_length(response)
    if length is not None and length > limit:
        raise _too_large(method, url, limit, response.status)
    chunks: list[bytes] = []
    total = 0
    while chunk := _read(response, _CHUNK, method, url):
        total += len(chunk)
        if total > limit:
            raise _too_large(method, url, limit, response.status)
        chunks.append(chunk)
    if length is not None and total != length:
        raise _incomplete(method, url)
    return b"".join(chunks)


def _save(response: http.client.HTTPResponse, method: str, url: str, dest: Path, max_bytes: int) -> DownloadResponse:
    length = _content_length(response)
    if length is not None and length > max_bytes:
        raise _too_large(method, url, max_bytes, response.status)
    size = 0
    try:
        with dest.open("wb") as out:
            while chunk := _read(response, _CHUNK, method, url):
                size += len(chunk)
                if size > max_bytes:
                    raise _too_large(method, url, max_bytes, response.status)
                out.write(chunk)
        if length is not None and size != length:
            raise _incomplete(method, url)
    except BaseException:
        dest.unlink(missing_ok=True)
        raise
    return DownloadResponse(response.status, response.headers, size)


def _status_error(method: str, url: str, response: http.client.HTTPResponse) -> HttpError:
    status = response.status
    try:
        phrase = f" {HTTPStatus(status).phrase}"
    except ValueError:
        phrase = ""
    server_message = _server_message(response)
    text = f"{method} {safe_url(url)} failed: HTTP {status}{phrase}"
    if server_message:
        text += f": {server_message}"
    return HttpError(
        text,
        status=status,
        headers=response.headers,
        server_message=server_message,
        retryable=status in _RETRY_STATUSES,
    )


def _server_message(response: http.client.HTTPResponse) -> str | None:
    """The error body's JSON ``message``, printable, whitespace collapsed, at most 200 characters."""
    try:
        body = json.loads(response.read(_MAX_ERROR_BODY))
    except (OSError, http.client.HTTPException, ValueError, RecursionError):
        return None
    message = body.get("message") if isinstance(body, dict) else None
    if not isinstance(message, str) or not message.strip():
        return None
    message = " ".join(_printable(message).split())
    if len(message) > _MAX_SERVER_MESSAGE:
        message = message[: _MAX_SERVER_MESSAGE - 3] + "..."
    return message


def _retry_after(value: str | None, now: Callable[[], float]) -> float | None:
    """Seconds a ``Retry-After`` header asks to wait (delay seconds or an HTTP date), or None when unreadable."""
    if value is None:
        return None
    value = value.strip()
    if value.isdecimal() and value.isascii():
        return float(value)  # A very long number reads as infinity, which the caller caps.
    try:
        when = parsedate_to_datetime(value)
    except (TypeError, ValueError, IndexError):
        return None
    if when.tzinfo is None:
        when = when.replace(tzinfo=UTC)
    return max(0.0, when.timestamp() - now())
