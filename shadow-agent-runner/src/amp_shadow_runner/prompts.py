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

"""Ask in the terminal for what a scan needs and Agent Manager never receives: the Git token and the LLM settings.

The command reads each value from its environment variable, or else asks for it:
secrets with hidden input, other values (the LLM provider, address and models)
with visible input. A secret is never printed, logged or put in an error message.
"""

import getpass
import os
import sys
import warnings
from collections.abc import Callable, Mapping
from typing import TextIO
from urllib.parse import urlsplit

from amp_shadow_runner.config import ConfigError, ScanConfig, _url
from amp_shadow_runner.llm import DEFAULT_LLM_URLS, LlmProvider, LlmSettings

Check = Callable[[str], str | None]
"""Returns an error message to show (never containing the secret), or None when the secret is fine."""

Parse = Callable[[str], str]
"""Returns the value to keep, or raises ConfigError with a message to show."""

GITHUB_TOKEN_ENV = "GITHUB_TOKEN"
AZURE_DEVOPS_TOKEN_ENV = "AZURE_DEVOPS_TOKEN"
LLM_PROVIDER_ENV = "AMP_LLM_PROVIDER"
LLM_URL_ENV = "AMP_LLM_URL"
LLM_MODEL_ENV = "AMP_LLM_MODEL"
LLM_JUDGE_MODEL_ENV = "AMP_LLM_JUDGE_MODEL"
LLM_API_KEY_ENV = "AMP_LLM_API_KEY"


_BAD_CHARACTERS = "The value contains spaces or control characters."


def _stdin_is_tty() -> bool:
    return sys.stdin is not None and sys.stdin.isatty()


def _has_bad_characters(value: str) -> bool:
    """True when a stripped value has inner whitespace or a control character (it could never be a header)."""
    return any(c.isspace() or not c.isprintable() for c in value)


def _read_hidden(read: Callable[[str], str], prompt: str, env_var: str) -> str:
    """Call ``read``, failing closed when getpass cannot hide the input and would echo it."""
    with warnings.catch_warnings():
        warnings.simplefilter("error", getpass.GetPassWarning)
        try:
            return read(prompt)
        except getpass.GetPassWarning:
            raise ConfigError(
                f"Cannot hide input on this terminal. Set {env_var} and pass it with -e {env_var} instead."
            ) from None


def ask_secret(
    label: str,
    env_var: str,
    *,
    need: str,
    optional: bool = False,
    check: Check | None = None,
    attempts: int = 3,
    env: Mapping[str, str] | None = None,
    is_tty: bool | None = None,
    read: Callable[[str], str] | None = None,
    out: TextIO | None = None,
) -> str | None:
    """Return a secret from ``env_var``, or ask for it with hidden input.

    ``need`` is the sentence shown when a required secret cannot be asked for,
    such as "A GitHub token is needed for private repositories." An optional
    secret returns None when it cannot be asked for, or when Enter is pressed.
    A value with inner spaces or control characters is refused, then ``check``
    runs. Refused answers, and blank answers to a required secret, are retried
    up to ``attempts`` answers in all. Raises ConfigError, whose message never
    contains the secret, when there is no usable secret or the prompt is cancelled.

    ``env``, ``is_tty`` and ``read`` default to ``os.environ``, ``sys.stdin.isatty()``
    and ``getpass.getpass`` (its prompt goes to the terminal). Other messages go
    to ``out``, by default ``sys.stderr``.
    """
    env = os.environ if env is None else env
    stream: TextIO = sys.stderr if out is None else out

    value = env.get(env_var, "").strip()
    if value:
        problem = _BAD_CHARACTERS if _has_bad_characters(value) else check(value) if check is not None else None
        if problem is not None:
            raise ConfigError(f"{env_var} is not usable: {problem}")
        return value

    if not (_stdin_is_tty() if is_tty is None else is_tty):
        if optional:
            return None
        raise ConfigError(f"{need} Run it with docker run -it, or set {env_var} and pass it with -e {env_var}.")

    reader = getpass.getpass if read is None else read
    prompt = f"{label} (press Enter to skip): " if optional else f"{label}: "
    final = no_value = f"No usable {label} was given."
    for _ in range(attempts):
        try:
            value = _read_hidden(reader, prompt, env_var).strip()
        except (EOFError, KeyboardInterrupt):
            print(file=stream)  # End the prompt line before the error is printed.
            raise ConfigError("Cancelled.") from None
        final = no_value
        if not value:
            if optional:
                return None
            problem = "A value is required. Press Ctrl-C to cancel."
        elif _has_bad_characters(value):
            problem = _BAD_CHARACTERS
        else:
            problem = check(value) if check is not None else None
            if problem is None:
                return value
            final = problem
        print(problem, file=stream)
    raise ConfigError(f"{final} Stopped after {attempts} attempts.")


def _read_line(prompt: str) -> str:
    """Show ``prompt`` on stderr and read a line of visible input. Raises EOFError at the end of input."""
    print(prompt, end="", file=sys.stderr, flush=True)
    line = sys.stdin.readline()
    if not line:
        raise EOFError
    return line


def _parsed(value: str, parse: Parse | None) -> str:
    if _has_bad_characters(value):
        raise ConfigError(_BAD_CHARACTERS)
    return value if parse is None else parse(value)


def ask_value(
    label: str,
    env_var: str,
    *,
    need: str,
    default: str | None = None,
    parse: Parse | None = None,
    attempts: int = 3,
    env: Mapping[str, str] | None = None,
    is_tty: bool | None = None,
    read: Callable[[str], str] | None = None,
    out: TextIO | None = None,
) -> str:
    """Return a value that is not secret from ``env_var``, or ask for it with visible input.

    Like ``ask_secret``, with these differences. With a ``default``, the prompt
    shows it, and Enter, or having no terminal, gives it. ``parse`` returns the
    value to keep (such as the provider in lowercase) or raises ConfigError.
    ``read`` defaults to a line read from stdin, with the prompt on stderr.
    """
    env = os.environ if env is None else env
    stream: TextIO = sys.stderr if out is None else out

    value = env.get(env_var, "").strip()
    if value:
        try:
            return _parsed(value, parse)
        except ConfigError as exc:
            raise ConfigError(f"{env_var} is not usable: {exc}") from None

    if not (_stdin_is_tty() if is_tty is None else is_tty):
        if default is not None:
            return default
        raise ConfigError(f"{need} Run it with docker run -it, or set {env_var} and pass it with -e {env_var}.")

    reader = _read_line if read is None else read
    prompt = f"{label} [{default}]: " if default is not None else f"{label}: "
    final = no_value = f"No usable {label} was given."
    for _ in range(attempts):
        try:
            value = reader(prompt).strip()
        except (EOFError, KeyboardInterrupt):
            print(file=stream)  # End the prompt line before the error is printed.
            raise ConfigError("Cancelled.") from None
        final = no_value
        if not value:
            if default is not None:
                return default
            problem = "A value is required. Press Ctrl-C to cancel."
        else:
            try:
                return _parsed(value, parse)
            except ConfigError as exc:
                problem = final = str(exc)
        print(problem, file=stream)
    raise ConfigError(f"{final} Stopped after {attempts} attempts.")


def _provider(value: str) -> str:
    provider = value.lower()
    if provider not in DEFAULT_LLM_URLS:
        raise ConfigError("The LLM provider must be openai (for any OpenAI-compatible API) or anthropic.")
    return provider


def _ascii_key(value: str) -> str | None:
    return None if value.isascii() else "The API key must contain only ASCII letters, digits and symbols."


def _llm_url(value: str) -> str:
    return _url(value, "The LLM address")


def llm_settings(
    config: ScanConfig,
    *,
    env: Mapping[str, str] | None = None,
    is_tty: bool | None = None,
    read: Callable[[str], str] | None = None,
    read_secret: Callable[[str], str] | None = None,
    out: TextIO | None = None,
) -> LlmSettings:
    """Return the LLM settings for ``--ai-review``: provider, address, model, judge model and API key.

    Each comes from its ``AMP_LLM_*`` variable or is asked for. At the prompt
    the address defaults to the provider's own; without a terminal
    ``AMP_LLM_URL`` is required, so a gateway's key is never sent to the public
    API by mistake. The judge model (only with ``--judge``) defaults to the
    review model; the judge uses the same provider, address and key. Before the
    key is asked for, print the provider, host and models (never the key), and
    warn when the address is plain http. ``read`` reads visible answers and
    ``read_secret`` the key (see ``ask_value`` and ``ask_secret``).
    """
    stream: TextIO = sys.stderr if out is None else out
    tty = _stdin_is_tty() if is_tty is None else is_tty

    def ask(label: str, env_var: str, need: str, *, default: str | None = None, parse: Parse | None = None) -> str:
        return ask_value(
            label, env_var, need=need, default=default, parse=parse, env=env, is_tty=tty, read=read, out=stream
        )

    need = "AI review needs"
    chosen = ask("LLM provider (openai or anthropic)", LLM_PROVIDER_ENV, f"{need} an LLM provider.", parse=_provider)
    provider: LlmProvider = "anthropic" if chosen == "anthropic" else "openai"
    default_url = DEFAULT_LLM_URLS[provider]
    url_need = f"{need} the LLM address, such as {default_url}."
    url = ask("LLM address", LLM_URL_ENV, url_need, default=default_url if tty else None, parse=_llm_url)
    model = ask("LLM model", LLM_MODEL_ENV, f"{need} an LLM model name.")
    judge_model = ask("Judge model", LLM_JUDGE_MODEL_ENV, "", default=model) if config.judge else None
    judge = f" (judge {judge_model})" if judge_model is not None else ""
    print(f"AI review: {provider} at {urlsplit(url).netloc}, model {model}{judge}.", file=stream)
    _warn_if_plain_http(url, "LLM API key", stream)
    key_need = f"{need} an LLM API key."
    api_key = ask_secret(
        "LLM API key",
        LLM_API_KEY_ENV,
        need=key_need,
        check=_ascii_key,
        env=env,
        is_tty=tty,
        read=read_secret,
        out=stream,
    )
    if api_key is None:  # Only an optional secret is ever None.
        raise ConfigError(key_need)
    return LlmSettings(provider=provider, url=url, model=model, judge_model=judge_model, api_key=api_key)


def git_token(
    config: ScanConfig,
    *,
    env: Mapping[str, str] | None = None,
    is_tty: bool | None = None,
    read: Callable[[str], str] | None = None,
    out: TextIO | None = None,
) -> str | None:
    """Return the Git token for the scan, or None for a public GitHub scan without ``GITHUB_TOKEN``.

    GitHub asks for a token only with ``--include-private``. Without it, a set
    ``GITHUB_TOKEN`` is still used (it raises the API rate limit) but never asked
    for. Azure DevOps always needs a token. Before a token is used over plain
    http, warn that it will be sent unencrypted.
    """
    stream: TextIO = sys.stderr if out is None else out
    if config.provider == "github":
        label, env_var, noun = "GitHub token (fine-grained, Contents read-only)", GITHUB_TOKEN_ENV, "GitHub token"
        need = "A GitHub token is needed for private repositories."
        if not config.include_private:
            token = ask_secret(label, env_var, need=need, optional=True, env=env, is_tty=False, out=stream)
            if token is not None:
                _warn_if_plain_http(config.api_url, noun, stream)
            return token
    else:
        label = "Azure DevOps personal access token (Code read, Project and team read)"
        env_var, noun = AZURE_DEVOPS_TOKEN_ENV, "Azure DevOps token"
        need = "An Azure DevOps personal access token is needed."
    _warn_if_plain_http(config.api_url, noun, stream)
    return ask_secret(label, env_var, need=need, env=env, is_tty=is_tty, read=read, out=stream)


def _warn_if_plain_http(url: str, noun: str, out: TextIO) -> None:
    if url.startswith("http://"):
        host = urlsplit(url).netloc
        print(f"Warning: {host} uses plain http, so the {noun} will be sent to it unencrypted.", file=out)
