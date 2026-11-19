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

"""Ask for the secrets a scan needs (the Git token, and later the LLM key) in the terminal.

Agent Manager never receives these secrets: the command reads each one from its
environment variable, or else asks for it with hidden input. A secret is never
printed, logged or put in an error message.
"""

import getpass
import os
import sys
import warnings
from collections.abc import Callable, Mapping
from typing import TextIO
from urllib.parse import urlsplit

from amp_shadow_runner.config import ConfigError, ScanConfig

Check = Callable[[str], str | None]
"""Returns an error message to show (never containing the secret), or None when the secret is fine."""

GITHUB_TOKEN_ENV = "GITHUB_TOKEN"


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
    for. Before a token is used over plain http, warn that it will be sent
    unencrypted.
    """
    stream: TextIO = sys.stderr if out is None else out
    label, env_var, noun = "GitHub token (fine-grained, Contents read-only)", GITHUB_TOKEN_ENV, "GitHub token"
    need = "A GitHub token is needed for private repositories."
    if not config.include_private:
        token = ask_secret(label, env_var, need=need, optional=True, env=env, is_tty=False, out=stream)
        if token is not None:
            _warn_if_plain_http(config, noun, stream)
        return token
    _warn_if_plain_http(config, noun, stream)
    return ask_secret(label, env_var, need=need, env=env, is_tty=is_tty, read=read, out=stream)


def _warn_if_plain_http(config: ScanConfig, noun: str, out: TextIO) -> None:
    if config.api_url.startswith("http://"):
        host = urlsplit(config.api_url).netloc
        print(f"Warning: {host} uses plain http, so the {noun} will be sent to it unencrypted.", file=out)
