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

"""The LLM that AI review uses: its settings, and a small client for OpenAI-compatible and Anthropic APIs.

The settings are asked for in the terminal (``prompts.llm_settings``) and never reach Agent Manager.
The client sends one chat request and returns the text of the answer. It runs outside the engine.

No error message contains the API key, the prompt or the code.
"""

import time
from collections.abc import Callable
from dataclasses import dataclass, field
from typing import Any, ClassVar, Literal
from urllib.parse import urlsplit

from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.httpclient import HttpClient, HttpError

LlmProvider = Literal["openai", "anthropic"]

DEFAULT_LLM_URLS: dict[LlmProvider, str] = {
    "openai": "https://api.openai.com/v1",
    "anthropic": "https://api.anthropic.com",
}
LLM_TIMEOUT = 120.0
"""Seconds to wait to connect, and for each read: a model can take a while to answer."""
LLM_MAX_ATTEMPTS = 3
ANTHROPIC_VERSION = "2023-06-01"
_KEY_SLICE = 8
"""A server message holding this many characters of the key in a row is not shown."""


class LlmError(Exception):
    """An LLM request failed. The message is safe to show.

    ``status`` is the HTTP status, or None when there was no usable response.
    """

    exit_code: ClassVar[ExitCode] = ExitCode.RUNTIME_ERROR

    def __init__(self, message: str, *, status: int | None = None) -> None:
        super().__init__(message)
        self.status = status


@dataclass(frozen=True)
class LlmSettings:
    """The LLM for AI review. ``repr`` never shows the key.

    ``url`` has no trailing slash. ``judge_model`` is None without ``--judge``.
    """

    provider: LlmProvider
    url: str
    model: str
    judge_model: str | None
    api_key: str = field(repr=False)


def llm_http_client(*, timeout: float = LLM_TIMEOUT, sleep: Callable[[float], None] = time.sleep) -> HttpClient:
    """The HTTP client for LLM calls: at most 3 attempts, and a timeout or network error is not retried.

    A request that timed out may still be running at the LLM, so sending it again would only pile up work.
    """
    return HttpClient(timeout=timeout, max_attempts=LLM_MAX_ATTEMPTS, retry_network_errors=False, sleep=sleep)


class LlmClient:
    """Sends a system and a user message to the LLM and returns the answer's text.

    OpenAI-compatible: ``POST {url}/chat/completions`` with ``Authorization: Bearer``, sending ``max_tokens``
    and temperature 0, which OpenAI-compatible servers (vLLM, Ollama, LiteLLM and others) accept. Azure
    OpenAI works only through its ``/openai/v1`` address with a key it accepts as a Bearer token; classic
    deployments (an ``api-key`` header and an ``api-version`` query) do not. OpenAI's reasoning models (the
    gpt-5 family and the o-series) refuse both: on that HTTP 400 the request is sent once more with
    ``max_completion_tokens`` and no temperature, and the model is remembered so later requests start
    that way. For them, reasoning tokens count against ``max_tokens``, so leave headroom above the answer.

    Anthropic: ``POST {url}/v1/messages`` (``{url}/messages`` when the address ends in ``/v1``) with
    ``x-api-key`` and temperature 0.

    Requests are retried on HTTP 429, 5xx and 529 (overloaded), at most 3 attempts in all, but not after a
    timeout or network error. A POST never follows a redirect, so the key is only ever sent to ``url``.
    An answer cut off at the token limit is an error, not partial text.
    """

    def __init__(self, settings: LlmSettings, *, http: HttpClient | None = None) -> None:
        self._settings = settings
        self._http = llm_http_client() if http is None else http
        self._reasoning_models: set[str] = set()
        """OpenAI models that take ``max_completion_tokens`` and no temperature."""

    def complete(self, system: str, user: str, *, model: str, max_tokens: int) -> str:
        """Return the text the LLM answers. Raises LlmError when the request fails or the answer is unusable."""
        if self._settings.provider == "anthropic":
            return self._anthropic(system, user, model, max_tokens)
        try:
            return self._openai(system, user, model, max_tokens)
        except _ReasoningModel:
            self._reasoning_models.add(model)
        return self._openai(system, user, model, max_tokens)

    def _openai(self, system: str, user: str, model: str, max_tokens: int) -> str:
        body: dict[str, Any] = {
            "model": model,
            "messages": [{"role": "system", "content": system}, {"role": "user", "content": user}],
        }
        reasoning = model in self._reasoning_models
        if reasoning:
            body["max_completion_tokens"] = max_tokens
        else:
            body["max_tokens"] = max_tokens
            body["temperature"] = 0
        headers = {"Authorization": f"Bearer {self._settings.api_key}"}
        data = self._post(f"{self._settings.url}/chat/completions", body, headers, fallback=not reasoning)
        choices = data.get("choices") if isinstance(data, dict) else None
        first = choices[0] if isinstance(choices, list) and choices else None
        if isinstance(first, dict) and first.get("finish_reason") == "length":
            raise self._cut_off(max_tokens)
        message = first.get("message") if isinstance(first, dict) else None
        content = message.get("content") if isinstance(message, dict) else None
        if not isinstance(content, str):
            raise self._unexpected()
        return content

    def _anthropic(self, system: str, user: str, model: str, max_tokens: int) -> str:
        url = self._settings.url
        url += "/messages" if urlsplit(url).path.endswith("/v1") else "/v1/messages"
        body = {
            "model": model,
            "system": system,
            "messages": [{"role": "user", "content": user}],
            "max_tokens": max_tokens,
            "temperature": 0,
        }
        headers = {"x-api-key": self._settings.api_key, "anthropic-version": ANTHROPIC_VERSION}
        data = self._post(url, body, headers, fallback=False)
        if isinstance(data, dict) and data.get("stop_reason") == "max_tokens":
            raise self._cut_off(max_tokens)
        content = data.get("content") if isinstance(data, dict) else None
        texts = [
            block["text"]
            for block in (content if isinstance(content, list) else ())
            if isinstance(block, dict) and block.get("type") == "text" and isinstance(block.get("text"), str)
        ]
        if not texts:
            raise self._unexpected()
        return "".join(texts)

    def _post(self, url: str, body: dict[str, Any], headers: dict[str, str], *, fallback: bool) -> Any:
        """POST and return the JSON answer. ``fallback`` raises _ReasoningModel on the reasoning-model 400."""
        try:
            return self._http.post_json(url, body, headers=headers, retry=True).data
        except HttpError as exc:
            if fallback and exc.status == 400 and _names_openai_only_parameter(exc.server_message):
                error: Exception = _ReasoningModel()
            else:
                error = self._error(exc)
        raise error  # Outside the except block, so the HttpError is not chained onto it.

    def _host(self) -> str:
        return urlsplit(self._settings.url).netloc

    def _unexpected(self) -> LlmError:
        return LlmError(f"The LLM at {self._host()} gave an unexpected response.")

    def _cut_off(self, max_tokens: int) -> LlmError:
        return LlmError(f"The LLM's answer was cut off at the token limit ({max_tokens} tokens).")

    def _error(self, exc: HttpError) -> LlmError:
        host = self._host()
        status = exc.status
        if status is None:  # A network error, a timeout or a refused redirect: the text has no secrets.
            return LlmError(f"Could not get an answer from the LLM at {host}: {exc}")
        if 200 <= status < 300:  # Not JSON, or too large.
            return LlmError(
                f"The LLM at {host} gave an unexpected response (not JSON). "
                "Check the LLM address: it may point at a web page rather than the API.",
                status=status,
            )
        if status == 401:  # OpenAI's message quotes part of the key.
            return LlmError(f"The LLM at {host} refused the API key (HTTP 401).", status=401)
        text = f"The LLM at {host} answered HTTP {status}"
        message = exc.server_message
        if message and not _holds_key(message, self._settings.api_key):
            text += f": {message}"
        if (
            status == 404
            and self._settings.provider == "openai"
            and not urlsplit(self._settings.url).path.endswith("/v1")
        ):
            text += " (the address usually ends in /v1)"
        return LlmError(text + ".", status=status)


class _ReasoningModel(Exception):
    """The model refused ``max_tokens`` or ``temperature``: send it the reasoning-model form."""


def _names_openai_only_parameter(message: str | None) -> bool:
    return message is not None and ("max_tokens" in message or "temperature" in message)


def _holds_key(message: str, key: str) -> bool:
    """True when ``message`` holds the key, or any 8 characters of it in a row (a cut can split the key)."""
    size = min(_KEY_SLICE, len(key))
    return any(key[i : i + size] in message for i in range(len(key) - size + 1))
