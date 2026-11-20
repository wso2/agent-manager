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

"""amp_shadow_runner.httpclient against local HTTP servers: retries, caps, redirects and safe errors."""

import json
import socket
import threading
import time
from collections.abc import Callable, Iterator
from dataclasses import dataclass
from email.utils import formatdate
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

import pytest

from amp_shadow_runner import __version__
from amp_shadow_runner.httpclient import HttpClient, HttpError, origin, redirect_target, safe_url

NOW = 1_800_000_000.0
TOKEN = "Bearer s3cr3t-scan-token-value"
Reply = tuple[int, dict[str, str], bytes]


@dataclass
class Seen:
    method: str
    path: str
    headers: dict[str, str]
    body: bytes


class _Handler(BaseHTTPRequestHandler):
    server: "_Server"

    def _handle(self) -> None:
        body = self.rfile.read(int(self.headers.get("Content-Length") or 0))
        self.server.seen.append(Seen(self.command, self.path, dict(self.headers.items()), body))
        replies = self.server.routes.get(self.path.split("?", 1)[0])
        if not replies:
            self.send_error(404)
            return
        reply = replies.pop(0) if len(replies) > 1 else replies[0]
        if callable(reply):
            reply = reply()
        status, headers, payload = reply
        self.send_response(status)
        for name, value in headers.items():
            self.send_header(name, value)
        self.end_headers()
        self.wfile.write(payload)

    do_GET = do_POST = _handle

    def log_message(self, format: str, *args: Any) -> None:  # noqa: A002 (the base class's name)
        pass


class _Server(ThreadingHTTPServer):
    """Answers each path with its replies in turn and records every request it receives."""

    def __init__(self) -> None:
        super().__init__(("127.0.0.1", 0), _Handler)
        self.routes: dict[str, list[Any]] = {}
        self.seen: list[Seen] = []

    def handle_error(self, request: Any, client_address: Any) -> None:
        pass  # A client that timed out leaves a broken pipe behind; not a test failure.

    def url(self, path: str) -> str:
        return f"http://127.0.0.1:{self.server_address[1]}{path}"

    def route(self, path: str, *replies: Reply | Callable[[], Reply]) -> str:
        """Answer ``path`` with each reply in turn, repeating the last one."""
        self.routes[path] = list(replies)
        return self.url(path)


def _start() -> Iterator[_Server]:
    server = _Server()
    thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.01}, daemon=True)
    thread.start()
    yield server
    server.shutdown()
    server.server_close()
    thread.join()


@pytest.fixture
def server() -> Iterator[_Server]:
    yield from _start()


@pytest.fixture
def other() -> Iterator[_Server]:
    """A second server on another port, so another origin."""
    yield from _start()


@pytest.fixture(autouse=True)
def _no_proxy(monkeypatch: pytest.MonkeyPatch) -> None:
    for name in ("http_proxy", "https_proxy", "all_proxy", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"):
        monkeypatch.delenv(name, raising=False)
    monkeypatch.setenv("no_proxy", "*")


@pytest.fixture
def sleeps() -> list[float]:
    return []


@pytest.fixture
def client(sleeps: list[float]) -> HttpClient:
    return HttpClient(timeout=2, sleep=sleeps.append, now=lambda: NOW, jitter=lambda: 0.0)


def _json(status: int, data: object, **headers: str) -> Reply:
    body = json.dumps(data).encode()
    return status, {"Content-Type": "application/json", "Content-Length": str(len(body)), **headers}, body


def _fails(call: Callable[[], object]) -> HttpError:
    with pytest.raises(HttpError) as info:
        call()
    exc = info.value
    for secret in ("s3cr3t", "sig=", "pw@", "query-secret"):
        assert secret not in str(exc)
        assert secret not in repr(exc)
    assert exc.__cause__ is None
    assert exc.__context__ is None
    return exc


def test_get_json_returns_status_headers_and_data(server: _Server, client: HttpClient) -> None:
    url = server.route("/repos", _json(200, [{"name": "a"}], Link='<https://x/repos?page=2>; rel="next"'))
    response = client.get_json(url, headers={"Accept": "application/vnd.github+json", "Authorization": TOKEN})
    assert (response.status, response.data) == (200, [{"name": "a"}])
    assert response.headers.get("link") == '<https://x/repos?page=2>; rel="next"'
    seen = server.seen[0]
    assert seen.headers["User-Agent"] == f"amp-shadow-runner/{__version__}"
    assert seen.headers["Accept"] == "application/vnd.github+json"
    assert seen.headers["Authorization"] == TOKEN


def test_user_agent_cannot_be_replaced(server: _Server, client: HttpClient) -> None:
    client.get_json(server.route("/x", _json(200, {})), headers={"user-agent": "other"})
    assert server.seen[0].headers["User-Agent"] == f"amp-shadow-runner/{__version__}"


def test_post_json_sends_utf8_json(server: _Server, client: HttpClient) -> None:
    url = server.route("/results", (204, {}, b""))
    response = client.post_json(url, {"name": "agent é"})
    assert (response.status, response.data) == (204, None)
    seen = server.seen[0]
    assert seen.method == "POST"
    assert seen.headers["Content-Type"] == "application/json"
    assert json.loads(seen.body.decode("utf-8")) == {"name": "agent é"}
    assert "é".encode() in seen.body


def test_503_is_retried_then_succeeds(server: _Server, client: HttpClient, sleeps: list[float]) -> None:
    url = server.route("/x", _json(503, {}), _json(200, {"ok": True}))
    assert client.get_json(url).data == {"ok": True}
    assert len(server.seen) == 2
    assert sleeps == [0.5]  # 1 s backoff with the lowest jitter.


def test_429_is_retried_honouring_retry_after_seconds(server: _Server, client: HttpClient, sleeps: list[float]) -> None:
    url = server.route("/x", _json(429, {}, **{"Retry-After": "7"}), _json(200, {}))
    client.get_json(url)
    assert sleeps == [7.0]


def test_retry_after_http_date_is_honoured(server: _Server, client: HttpClient, sleeps: list[float]) -> None:
    url = server.route("/x", _json(503, {}, **{"Retry-After": formatdate(NOW + 12, usegmt=True)}), _json(200, {}))
    client.get_json(url)
    assert sleeps == [12.0]


def test_retry_after_over_the_cap_fails_at_once(server: _Server, client: HttpClient, sleeps: list[float]) -> None:
    url = server.route("/x", _json(429, {}, **{"Retry-After": "3600"}), _json(200, {}))
    exc = _fails(lambda: client.get_json(url))
    assert (exc.status, len(server.seen), sleeps) == (429, 1, [])


def test_retry_after_shorter_than_backoff_waits_the_backoff(
    server: _Server, client: HttpClient, sleeps: list[float]
) -> None:
    url = server.route("/x", _json(503, {}, **{"Retry-After": "0"}), _json(200, {}))
    client.get_json(url)
    assert sleeps == [0.5]


def test_retries_are_exhausted(server: _Server, client: HttpClient, sleeps: list[float]) -> None:
    url = server.route("/x", _json(502, {"message": "upstream down"}))
    exc = _fails(lambda: client.get_json(url))
    assert exc.status == 502
    assert exc.retryable
    assert exc.server_message == "upstream down"
    assert str(exc) == f"GET {url} failed: HTTP 502 Bad Gateway: upstream down"
    assert len(server.seen) == 4
    assert sleeps == [0.5, 1.0, 2.0]


@pytest.mark.parametrize(
    ("body", "expected"),
    [
        ({"error": {"type": "invalid_request_error", "message": " bad\n model "}}, "bad model"),
        ({"message": "top", "error": {"message": "nested"}}, "top"),
        ({"error": {"message": "x" * 300}}, "x" * 197 + "..."),
        ({"error": "a string"}, None),
        ({"error": {"message": 3}}, None),
    ],
)
def test_server_message_reads_a_nested_error_message(
    server: _Server, client: HttpClient, body: object, expected: str | None
) -> None:
    exc = _fails(lambda: client.post_json(server.route("/x", _json(400, body)), {}))
    assert exc.server_message == expected


def test_client_errors_are_not_retried(server: _Server, client: HttpClient) -> None:
    exc = _fails(lambda: client.get_json(server.route("/x", _json(404, {"message": "Not Found"}))))
    assert (exc.status, exc.retryable, len(server.seen)) == (404, False, 1)


def test_post_is_not_retried_unless_asked(server: _Server, client: HttpClient) -> None:
    url = server.route("/results", _json(503, {}), _json(200, {"id": 1}))
    assert _fails(lambda: client.post_json(url, {})).status == 503
    assert len(server.seen) == 1
    assert client.post_json(url, {}, retry=True).data == {"id": 1}


def test_body_that_is_not_json_fails(server: _Server, client: HttpClient) -> None:
    url = server.route("/x", (200, {"Content-Type": "text/html"}, b"<html>"))
    exc = _fails(lambda: client.get_json(url))
    assert (exc.status, len(server.seen)) == (200, 1)
    assert "not valid JSON" in str(exc)


@pytest.mark.parametrize("with_length", [True, False])
def test_oversize_json_fails(server: _Server, sleeps: list[float], with_length: bool) -> None:
    body = json.dumps({"data": "x" * 100}).encode()
    headers = {"Content-Length": str(len(body))} if with_length else {}
    url = server.route("/x", (200, headers, body))
    exc = _fails(lambda: HttpClient(max_json_bytes=50, sleep=sleeps.append).get_json(url))
    assert "larger than 50 bytes" in str(exc)


def test_download_writes_the_file(server: _Server, client: HttpClient, tmp_path: Path) -> None:
    dest = tmp_path / "repo.tar.gz"
    url = server.route("/archive", (200, {"Content-Length": "200000"}, b"z" * 200_000))
    response = client.download(url, dest, max_bytes=1_000_000)
    assert (response.status, response.size) == (200, 200_000)
    assert dest.read_bytes() == b"z" * 200_000


@pytest.mark.parametrize("with_length", [True, False])
def test_download_reports_progress(server: _Server, client: HttpClient, tmp_path: Path, with_length: bool) -> None:
    headers = {"Content-Length": "200000"} if with_length else {}
    url = server.route("/archive", (200, headers, b"z" * 200_000))
    calls: list[tuple[int, int | None]] = []
    client.download(url, tmp_path / "a", max_bytes=1_000_000, on_progress=lambda n, length: calls.append((n, length)))
    received = [n for n, _ in calls]
    assert received[0] == 0 and received[-1] == 200_000 and len(received) > 2
    assert received == sorted(set(received))
    assert {length for _, length in calls} == {200_000 if with_length else None}


@pytest.mark.parametrize("with_length", [True, False])
def test_download_over_max_bytes_leaves_no_file(
    server: _Server, client: HttpClient, tmp_path: Path, with_length: bool
) -> None:
    dest = tmp_path / "repo.zip"
    headers = {"Content-Length": "200000"} if with_length else {}
    url = server.route("/archive", (200, headers, b"z" * 200_000))
    exc = _fails(lambda: client.download(url, dest, max_bytes=100_000))
    assert "larger than 100000 bytes" in str(exc)
    assert not dest.exists()


def test_short_json_body_is_retried(server: _Server, client: HttpClient, sleeps: list[float]) -> None:
    url = server.route("/x", (200, {"Content-Length": "100"}, b'{"a": 1}'), _json(200, {"a": 2}))
    assert client.get_json(url).data == {"a": 2}
    assert len(sleeps) == 1


def test_short_body_fails_when_retries_run_out(server: _Server, client: HttpClient) -> None:
    url = server.route("/x", (200, {"Content-Length": "100"}, b'{"a": 1}'))
    exc = _fails(lambda: client.get_json(url))
    assert (exc.status, exc.retryable, str(exc)) == (None, True, f"GET {url} failed: invalid or incomplete response")


def test_short_download_is_retried_and_leaves_the_right_file(
    server: _Server, client: HttpClient, sleeps: list[float], tmp_path: Path
) -> None:
    dest = tmp_path / "repo.zip"
    url = server.route(
        "/archive", (200, {"Content-Length": "1000"}, b"partial"), (200, {"Content-Length": "4"}, b"full")
    )
    assert client.download(url, dest, max_bytes=10_000).size == 4
    assert dest.read_bytes() == b"full"
    assert len(sleeps) == 1


def test_short_download_leaves_no_file(server: _Server, tmp_path: Path) -> None:
    dest = tmp_path / "repo.zip"
    url = server.route("/archive", (200, {"Content-Length": "1000"}, b"partial"))
    _fails(lambda: HttpClient(max_attempts=1).download(url, dest, max_bytes=10_000))
    assert not dest.exists()


def test_sameorigin_redirect_keeps_authorization(server: _Server, client: HttpClient, tmp_path: Path) -> None:
    server.route("/land", (200, {}, b"data"))
    url = server.route("/tarball", (302, {"Location": "/land?sig=abc"}, b""))
    client.download(url, tmp_path / "a", max_bytes=100, headers={"Authorization": TOKEN})
    assert [s.path for s in server.seen] == ["/tarball", "/land?sig=abc"]
    assert server.seen[1].headers["Authorization"] == TOKEN


@pytest.mark.parametrize("name", ["Authorization", "authorization"])
def test_crossorigin_redirect_drops_authorization(
    server: _Server, other: _Server, client: HttpClient, tmp_path: Path, name: str
) -> None:
    landing = other.route("/codeload", (200, {}, b"data"))
    url = server.route("/tarball", (302, {"Location": landing + "?sig=abc"}, b""))
    client.download(url, tmp_path / "a", max_bytes=100, headers={name: TOKEN, "X-Other": "kept"})
    assert server.seen[0].headers["Authorization"] == TOKEN
    received = other.seen[0].headers
    assert "authorization" not in {k.lower() for k in received}
    assert received["X-Other"] == "kept"
    assert received["User-Agent"] == f"amp-shadow-runner/{__version__}"


def test_authorization_stays_dropped_when_redirected_back(server: _Server, other: _Server, client: HttpClient) -> None:
    server.route("/land", _json(200, {}))
    hop = other.route("/hop", (302, {"Location": server.url("/land")}, b""))
    url = server.route("/start", (302, {"Location": hop}, b""))
    client.get_json(url, headers={"Authorization": TOKEN})
    assert [s.path for s in server.seen] == ["/start", "/land"]
    assert "Authorization" in server.seen[0].headers
    assert "Authorization" not in other.seen[0].headers
    assert "Authorization" not in server.seen[1].headers


def testorigin_compares_scheme_host_and_port() -> None:
    assert origin("http://h:8443/a") != origin("https://h:8443/a")
    assert origin("https://H/a") == origin("https://h:443/b?c")
    assert origin("https://h/a") != origin("https://h:8443/a")


def test_too_many_redirects_fail(server: _Server, client: HttpClient) -> None:
    url = server.route("/loop", (302, {"Location": "/loop"}, b""))
    assert "more than 5 redirects" in str(_fails(lambda: client.get_json(url)))
    assert len(server.seen) == 6


def test_post_does_not_follow_redirects(server: _Server, client: HttpClient) -> None:
    server.route("/elsewhere", _json(200, {}))
    url = server.route("/results", (307, {"Location": "/elsewhere"}, b""))
    assert _fails(lambda: client.post_json(url, {})).status == 307
    assert len(server.seen) == 1


def test_redirect_target() -> None:
    assert redirect_target("https://api.github.com/a/b", "/c?x=1#frag") == "https://api.github.com/c?x=1"
    assert redirect_target("http://ghe.local/a", "https://codeload.example/x") == "https://codeload.example/x"
    assert "from https to http" in str(_fails(lambda: redirect_target("https://h/a", "http://h/a")))


@pytest.mark.parametrize("location", ["http://codeload.example/x", "ftp://x/y", "file:///etc/passwd", "http://[::1/x"])
def test_redirect_target_refuses(location: str) -> None:
    assert _fails(lambda: redirect_target("https://api.github.com/a?query-secret=1", location)).status is None


def test_error_message_has_no_credentials_or_query(server: _Server, other: _Server, client: HttpClient) -> None:
    landing = other.route("/codeload", _json(500, {"message": "Something\n  went\x1b[31m wrong " + "x" * 300}))
    url = server.route("/tarball", (302, {"Location": landing + "?sig=abc&query-secret=1"}, b""))
    exc = _fails(lambda: client.get_json(url, headers={"Authorization": TOKEN}))
    assert exc.server_message is not None and len(exc.server_message) == 200
    assert exc.server_message.startswith("Something went [31m wrong xxx")
    assert str(exc).startswith(f"GET {landing} failed: HTTP 500 Internal Server Error: Something went")


@pytest.mark.parametrize(
    "url", ["ftp://host/x", "http://user:pw@host/x", "http://host/a b", "http://host:99999/x", "/relative"]
)
def test_refused_urls(client: HttpClient, url: str) -> None:
    assert _fails(lambda: client.get_json(url)).status is None


def test_safe_url_strips_user_info_query_and_fragment() -> None:
    assert safe_url("https://user:pw@Host:8443/p/a?query-secret=1#f") == "https://host:8443/p/a"


def test_header_with_a_line_break_is_refused_without_its_value(client: HttpClient) -> None:
    exc = _fails(lambda: client.get_json("http://127.0.0.1:1/x", headers={"Authorization": "Bearer s3cr3t\r\nX: y"}))
    assert "invalid value" in str(exc)


def _free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port: int = sock.getsockname()[1]
    return port


def test_connection_refused_is_retried_then_fails(client: HttpClient, sleeps: list[float]) -> None:
    url = f"http://127.0.0.1:{_free_port()}/x"
    exc = _fails(lambda: client.get_json(url))
    assert (exc.status, exc.retryable) == (None, True)
    assert str(exc) == f"GET {url} failed: connection refused"
    assert len(sleeps) == 3


def test_timeout_is_applied(server: _Server, sleeps: list[float]) -> None:
    def slow() -> Reply:
        time.sleep(2)
        return _json(200, {})

    url = server.route("/slow", slow)
    started = time.monotonic()
    exc = _fails(lambda: HttpClient(timeout=0.1, max_attempts=2, sleep=sleeps.append).get_json(url))
    assert time.monotonic() - started < 1.5
    assert str(exc) == f"GET {url} failed: timed out"
    assert len(server.seen) == 2
