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

"""amp_shadow_runner.llm against a local HTTP server: both providers' requests, answers and safe errors."""

import json
import time
from collections.abc import Callable, Iterator
from typing import Any

import pytest

from amp_shadow_runner.llm import ANTHROPIC_VERSION, LlmClient, LlmError, LlmProvider, LlmSettings, llm_http_client
from tests.test_httpclient import Reply, _json, _Server, _start

KEY = "sk-s3cr3t-llm-key-value"
SYSTEM = "You review code."
USER = "def secret_code(): return 'prompt-body'"
OK = _json(200, {"choices": [{"message": {"content": "ok"}, "finish_reason": "stop"}]})
REASONING_400 = _json(
    400,
    {
        "error": {
            "message": "Unsupported parameter: 'max_tokens' is not supported with this model. "
            "Use 'max_completion_tokens' instead.",
            "type": "invalid_request_error",
        }
    },
)


@pytest.fixture
def server() -> Iterator[_Server]:
    yield from _start()


@pytest.fixture
def other() -> Iterator[_Server]:
    yield from _start()


@pytest.fixture(autouse=True)
def _no_proxy(monkeypatch: pytest.MonkeyPatch) -> None:
    for name in ("http_proxy", "https_proxy", "all_proxy", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"):
        monkeypatch.delenv(name, raising=False)
    monkeypatch.setenv("no_proxy", "*")


@pytest.fixture
def sleeps() -> list[float]:
    return []


def _client(server: _Server, provider: LlmProvider, sleeps: list[float], *, path: str = "") -> LlmClient:
    settings = LlmSettings(provider, server.url(path), "review-model", None, KEY)
    return LlmClient(settings, http=llm_http_client(timeout=2, sleep=sleeps.append))


def _complete(client: LlmClient) -> str:
    return client.complete(SYSTEM, USER, model="review-model", max_tokens=300)


def _fails(call: Callable[[], object]) -> LlmError:
    with pytest.raises(LlmError) as info:
        call()
    exc = info.value
    for text in (str(exc), repr(exc)):
        for secret in ("s3cr3t", "prompt-body", SYSTEM):
            assert secret not in text
    assert exc.__cause__ is None
    assert exc.__context__ is None
    return exc


def test_openai_request_and_answer(server: _Server, sleeps: list[float]) -> None:
    server.route("/v1/chat/completions", _json(200, {"choices": [{"message": {"role": "assistant", "content": "ok"}}]}))
    assert _complete(_client(server, "openai", sleeps, path="/v1")) == "ok"
    [seen] = server.seen
    headers = {name.lower(): value for name, value in seen.headers.items()}
    assert (seen.method, seen.path) == ("POST", "/v1/chat/completions")
    assert headers["authorization"] == f"Bearer {KEY}"
    assert "x-api-key" not in headers
    assert json.loads(seen.body) == {
        "model": "review-model",
        "messages": [{"role": "system", "content": SYSTEM}, {"role": "user", "content": USER}],
        "max_tokens": 300,
        "temperature": 0,
    }


def test_anthropic_request_and_answer(server: _Server, sleeps: list[float]) -> None:
    content = [
        {"type": "text", "text": "con"},
        {"type": "thinking", "thinking": "x"},
        {"type": "text", "text": "firmed"},
    ]
    server.route("/v1/messages", _json(200, {"content": content}))
    assert _complete(_client(server, "anthropic", sleeps)) == "confirmed"
    [seen] = server.seen
    headers = {name.lower(): value for name, value in seen.headers.items()}
    assert (seen.method, seen.path) == ("POST", "/v1/messages")
    assert headers["x-api-key"] == KEY
    assert headers["anthropic-version"] == ANTHROPIC_VERSION
    assert "authorization" not in headers
    assert json.loads(seen.body) == {
        "model": "review-model",
        "system": SYSTEM,
        "messages": [{"role": "user", "content": USER}],
        "max_tokens": 300,
        "temperature": 0,
    }


def test_the_model_is_the_one_asked_for(server: _Server, sleeps: list[float]) -> None:
    server.route("/chat/completions", _json(200, {"choices": [{"message": {"content": "ok"}}]}))
    _client(server, "openai", sleeps).complete(SYSTEM, USER, model="judge-model", max_tokens=10)
    assert json.loads(server.seen[0].body)["model"] == "judge-model"


def test_429_is_retried(server: _Server, sleeps: list[float]) -> None:
    server.route(
        "/chat/completions",
        _json(429, {"error": {"message": "Rate limit"}}, **{"Retry-After": "3"}),
        _json(200, {"choices": [{"message": {"content": "ok"}}]}),
    )
    assert _complete(_client(server, "openai", sleeps)) == "ok"
    assert (len(server.seen), sleeps) == (2, [3.0])


@pytest.mark.parametrize("provider", ["openai", "anthropic"])
def test_a_client_error_shows_the_nested_error_message(
    server: _Server, sleeps: list[float], provider: LlmProvider
) -> None:
    body = {"type": "error", "error": {"type": "invalid_request_error", "message": "model not found"}}
    server.route("/v1/messages" if provider == "anthropic" else "/chat/completions", _json(400, body))
    exc = _fails(lambda: _complete(_client(server, provider, sleeps)))
    assert exc.status == 400
    assert str(exc) == f"The LLM at 127.0.0.1:{server.server_address[1]} answered HTTP 400: model not found."
    assert len(server.seen) == 1


@pytest.mark.parametrize("leaked", [KEY, KEY[:8], KEY[-8:], f"{KEY[5:13]}..."])
def test_a_server_message_with_part_of_the_key_is_not_shown(server: _Server, sleeps: list[float], leaked: str) -> None:
    server.route("/v1/messages", _json(403, {"error": {"message": f"key {leaked} has no access"}}))
    exc = _fails(lambda: _complete(_client(server, "anthropic", sleeps)))
    assert str(exc) == f"The LLM at 127.0.0.1:{server.server_address[1]} answered HTTP 403."


def test_a_refused_key_does_not_show_the_server_message(server: _Server, sleeps: list[float]) -> None:
    masked = {"error": {"message": f"Incorrect API key provided: {KEY[:6]}****{KEY[-4:]}."}}
    server.route("/chat/completions", _json(401, masked))
    exc = _fails(lambda: _complete(_client(server, "openai", sleeps)))
    assert exc.status == 401
    assert "refused the API key (HTTP 401)" in str(exc)
    assert KEY[-4:] not in str(exc)


def test_a_redirect_on_post_is_refused_and_the_key_goes_nowhere_else(
    server: _Server, other: _Server, sleeps: list[float]
) -> None:
    elsewhere = other.route("/chat/completions", _json(200, {"choices": [{"message": {"content": "ok"}}]}))
    server.route("/chat/completions", (307, {"Location": elsewhere}, b""))
    exc = _fails(lambda: _complete(_client(server, "openai", sleeps)))
    assert exc.status == 307
    assert (len(server.seen), other.seen) == (1, [])


def test_a_network_error_is_an_llm_error(sleeps: list[float]) -> None:
    settings = LlmSettings("openai", "http://127.0.0.1:9", "m", None, KEY)
    client = LlmClient(settings, http=llm_http_client(timeout=1, sleep=sleeps.append))
    exc = _fails(lambda: _complete(client))
    assert exc.status is None
    assert str(exc).startswith("Could not get an answer from the LLM at 127.0.0.1:9: POST")
    assert sleeps == []  # A network error is not retried.


@pytest.mark.parametrize(
    ("provider", "data"),
    [
        ("openai", {}),
        ("openai", {"choices": []}),
        ("openai", {"choices": [{"message": {"content": None, "refusal": "no"}}]}),
        ("openai", [1, 2]),
        ("anthropic", {"content": []}),
        ("anthropic", {"content": [{"type": "tool_use"}]}),
        ("anthropic", {"content": "text"}),
        ("anthropic", None),
    ],
)
def test_unexpected_shapes_are_llm_errors(
    server: _Server, sleeps: list[float], provider: LlmProvider, data: Any
) -> None:
    reply = (200, {}, b"") if data is None else _json(200, data)
    server.route("/v1/messages" if provider == "anthropic" else "/chat/completions", reply)
    exc = _fails(lambda: _complete(_client(server, provider, sleeps)))
    assert "unexpected response" in str(exc)


def test_settings_repr_hides_the_key() -> None:
    assert KEY not in repr(LlmSettings("openai", "https://api.openai.com/v1", "m", "j", KEY))


def test_529_overloaded_is_retried_at_most_three_times(server: _Server, sleeps: list[float]) -> None:
    overloaded = _json(529, {"type": "error", "error": {"type": "overloaded_error", "message": "Overloaded"}})
    server.route("/v1/messages", overloaded, _json(200, {"content": [{"type": "text", "text": "ok"}]}))
    assert _complete(_client(server, "anthropic", sleeps)) == "ok"
    assert len(server.seen) == 2
    server.seen.clear()
    server.route("/v1/messages", overloaded)
    assert _fails(lambda: _complete(_client(server, "anthropic", sleeps))).status == 529
    assert len(server.seen) == 3


def test_a_timeout_is_not_retried(server: _Server, sleeps: list[float]) -> None:
    def slow() -> Reply:
        time.sleep(0.5)
        return OK

    server.route("/chat/completions", slow)
    settings = LlmSettings("openai", server.url(""), "m", None, KEY)
    client = LlmClient(settings, http=llm_http_client(timeout=0.1, sleep=sleeps.append))
    exc = _fails(lambda: _complete(client))
    assert exc.status is None
    assert "timed out" in str(exc)
    assert (len(server.seen), sleeps) == (1, [])


@pytest.mark.parametrize(
    ("provider", "data"),
    [
        ("openai", {"choices": [{"message": {"content": "partial"}, "finish_reason": "length"}]}),
        ("anthropic", {"content": [{"type": "text", "text": "partial"}], "stop_reason": "max_tokens"}),
    ],
)
def test_an_answer_cut_off_at_the_limit_is_an_error(
    server: _Server, sleeps: list[float], provider: LlmProvider, data: Any
) -> None:
    server.route("/v1/messages" if provider == "anthropic" else "/chat/completions", _json(200, data))
    exc = _fails(lambda: _complete(_client(server, provider, sleeps)))
    assert str(exc) == "The LLM's answer was cut off at the token limit (300 tokens)."


@pytest.mark.parametrize(
    "reply",
    [
        (200, {"Content-Type": "text/html"}, b"<html>sign in</html>"),
        (200, {"Content-Length": str(17 * 1024 * 1024)}, b""),
    ],
)
def test_a_2xx_answer_that_is_not_json_is_an_unexpected_response(
    server: _Server, sleeps: list[float], reply: Reply
) -> None:
    server.route("/chat/completions", reply)
    exc = _fails(lambda: _complete(_client(server, "openai", sleeps)))
    assert "gave an unexpected response (not JSON)" in str(exc)
    assert "may point at a web page" in str(exc)
    assert "HTTP 200" not in str(exc)


def test_an_anthropic_address_ending_in_v1_is_not_doubled(server: _Server, sleeps: list[float]) -> None:
    server.route("/v1/messages", _json(200, {"content": [{"type": "text", "text": "ok"}]}))
    assert _complete(_client(server, "anthropic", sleeps, path="/v1")) == "ok"
    assert server.seen[0].path == "/v1/messages"


@pytest.mark.parametrize(("path", "hint"), [("", True), ("/v1", False)])
def test_an_openai_404_hints_at_v1(server: _Server, sleeps: list[float], path: str, hint: bool) -> None:
    exc = _fails(lambda: _complete(_client(server, "openai", sleeps, path=path)))  # Every path is a 404.
    assert exc.status == 404
    assert str(exc).endswith("HTTP 404 (the address usually ends in /v1).") is hint


def test_a_reasoning_model_is_sent_max_completion_tokens_and_remembered(server: _Server, sleeps: list[float]) -> None:
    server.route("/chat/completions", REASONING_400, OK)
    client = _client(server, "openai", sleeps)
    assert _complete(client) == "ok"
    assert _complete(client) == "ok"
    first, retried, later = (json.loads(seen.body) for seen in server.seen)
    assert (first["max_tokens"], first["temperature"]) == (300, 0)
    for body in (retried, later):
        assert body["max_completion_tokens"] == 300
        assert "max_tokens" not in body
        assert "temperature" not in body
    assert sleeps == []
    client.complete(SYSTEM, USER, model="other-model", max_tokens=5)
    assert json.loads(server.seen[-1].body)["max_tokens"] == 5  # Remembered for that model only.


def test_the_reasoning_form_is_tried_only_once(server: _Server, sleeps: list[float]) -> None:
    server.route("/chat/completions", REASONING_400)
    exc = _fails(lambda: _complete(_client(server, "openai", sleeps)))
    assert exc.status == 400
    assert "max_completion_tokens" in str(exc)
    assert len(server.seen) == 2


def test_other_400s_are_not_sent_again(server: _Server, sleeps: list[float]) -> None:
    server.route("/chat/completions", _json(400, {"error": {"message": "context too long"}}))
    _fails(lambda: _complete(_client(server, "openai", sleeps)))
    assert len(server.seen) == 1
