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

The command reads each value from its environment variable, or else asks for it in its section of the
output (see ``terminal``): ``  ? LLM model``. On a terminal a secret is read key by key and shown as dots
(``keyboard.read_secret``), the LLM provider is chosen from a menu (``keyboard.choose``), and each question
is replaced by its answer's row once answered::

    ◆ AI review
      Provider   openai · https://api.openai.com/v1
      Model      gpt-4o-mini
      API key    ✓ received (164 characters)

Without a key-by-key terminal, secrets are read with getpass and the provider is typed. A secret is never
printed, logged or put in an error message: its row says only where it came from and how long it is.
"""

import getpass
import os
import sys
import warnings
from collections.abc import Callable, Mapping
from urllib.parse import urlsplit

from amp_shadow_runner.config import ConfigError, ScanConfig, _url
from amp_shadow_runner.keyboard import Keyboard, choose, keyboard_for, read_secret
from amp_shadow_runner.llm import DEFAULT_LLM_URLS, LlmProvider, LlmSettings
from amp_shadow_runner.terminal import Mark, Ui, clean, plural

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
PROVIDERS: tuple[LlmProvider, ...] = ("openai", "anthropic")
"""The menu's choices, the default first."""

_BAD_CHARACTERS = "The value contains spaces or control characters."
_NOTHING_ENTERED = "Nothing was entered. Press Ctrl-C to cancel."


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


def _cancelled(ui: Ui) -> ConfigError:
    ui.write("\n")  # End the question's line before the error is printed.
    return ConfigError("Cancelled.")


def ask_secret(
    label: str,
    env_var: str,
    *,
    row: str,
    need: str,
    optional: bool = False,
    check: Check | None = None,
    attempts: int = 3,
    env: Mapping[str, str] | None = None,
    is_tty: bool | None = None,
    read: Callable[[str], str] | None = None,
    keyboard: Keyboard | None = None,
    ui: Ui | None = None,
) -> str | None:
    """Return a secret from ``env_var``, or ask for it with hidden input, and print its ``row``:
    ``✓ from ENV_VAR`` or ``✓ received (40 characters)``.

    ``need`` is the sentence shown when a required secret cannot be asked for,
    such as "A GitHub token is needed for private repositories." An optional
    secret returns None when it cannot be asked for, or when Enter is pressed.
    Surrounding whitespace is stripped. A value with inner spaces or control
    characters is refused, then ``check`` runs. Refused answers, and blank
    answers to a required secret, are retried up to ``attempts`` answers in all.
    Raises ConfigError, whose message never contains the secret, when there is
    no usable secret or the question is cancelled.

    ``read`` (a getpass-like reader) wins over ``keyboard``; by default stdin is
    read key by key when it is a terminal and ``ui`` is live, else with
    ``getpass.getpass``. ``env`` and ``is_tty`` default to ``os.environ`` and
    ``sys.stdin.isatty()``; ``ui`` to a ``Ui`` on stderr.
    """
    env = os.environ if env is None else env
    ui = Ui() if ui is None else ui

    value = env.get(env_var, "").strip()
    if value:
        problem = _BAD_CHARACTERS if _has_bad_characters(value) else check(value) if check is not None else None
        if problem is not None:
            raise ConfigError(f"{env_var} is not usable: {problem}")
        ui.row(row, f"{ui.mark(Mark.OK)} from {env_var}")
        return value

    if not (_stdin_is_tty() if is_tty is None else is_tty):
        if optional:
            return None
        raise ConfigError(f"{need} Run it with docker run -it, or set {env_var} and pass it with -e {env_var}.")

    if read is None and keyboard is None and ui.live:
        keyboard = keyboard_for(sys.stdin)
    question = f"{label} (press Enter to skip)" if optional else label
    start = ui.rows
    final = no_value = f"No usable {label} was given."
    for _ in range(attempts):
        try:
            if read is None and keyboard is not None and ui.live:
                value = read_secret(ui, question, keyboard)
            else:
                prompt = ui.question(question)
                value = _read_hidden(getpass.getpass if read is None else read, prompt, env_var)
                ui.count(prompt)  # getpass ends the line on the terminal.
        except (EOFError, KeyboardInterrupt):
            raise _cancelled(ui) from None
        value = value.strip()
        final = no_value
        if not value:
            if optional:
                ui.erase_since(start)
                ui.row(row, ui.dim("skipped"))
                return None
            problem = _NOTHING_ENTERED
        elif _has_bad_characters(value):
            problem = _BAD_CHARACTERS
        else:
            problem = check(value) if check is not None else None
            if problem is None:
                ui.erase_since(start)
                ui.row(row, f"{ui.mark(Mark.OK)} received ({plural(len(value), 'character')})")
                return value
            final = problem
        ui.warn(problem)
    raise ConfigError(f"{final} Stopped after {attempts} attempts.")


def _line_reader(ui: Ui) -> Callable[[str], str]:
    """Show the prompt on ``ui`` and read a line of visible input. Raises EOFError at the end of input."""

    def read(prompt: str) -> str:
        ui.write(prompt)
        line = sys.stdin.readline()
        if not line:
            raise EOFError
        if not ui.tty:  # Nothing echoed the answer's newline into the log.
            ui.write("\n")
        return line

    return read


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
    ui: Ui | None = None,
) -> str:
    """Return a value that is not secret from ``env_var``, or ask for it with visible input.

    Like ``ask_secret``, with these differences. With a ``default``, the question
    shows it dim, and Enter, or having no terminal, gives it. ``parse`` returns the
    value to keep (such as the provider in lowercase) or raises ConfigError.
    ``read`` defaults to a line read from stdin, with the prompt on ``ui``. No row
    is printed: on a terminal the question is erased once answered, for the
    caller to print the answer's row.
    """
    env = os.environ if env is None else env
    ui = Ui() if ui is None else ui

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

    reader = _line_reader(ui) if read is None else read
    prompt = ui.question(label, default=default)
    start = ui.rows
    final = no_value = f"No usable {label} was given."
    for _ in range(attempts):
        try:
            answer = reader(prompt)
        except (EOFError, KeyboardInterrupt):
            raise _cancelled(ui) from None
        ui.count(prompt + answer.rstrip("\r\n"))
        value = answer.strip()
        final = no_value
        if not value:
            if default is not None:
                ui.erase_since(start)
                return default
            problem = _NOTHING_ENTERED
        else:
            try:
                value = _parsed(value, parse)
            except ConfigError as exc:
                problem = final = str(exc)
            else:
                ui.erase_since(start)
                return value
        ui.warn(problem)
    raise ConfigError(f"{final} Stopped after {attempts} attempts.")


def _provider(value: str) -> str:
    provider = value.lower()
    if provider not in DEFAULT_LLM_URLS:
        raise ConfigError("The LLM provider must be openai (for any OpenAI-compatible API) or anthropic.")
    return provider


def _ascii_key(value: str) -> str | None:
    return None if value.isascii() else "The API key must contain only ASCII letters, digits and symbols."


def _ascii_token(value: str) -> str | None:
    return None if value.isascii() else "The token must contain only ASCII letters, digits and symbols."


def _llm_url(value: str) -> str:
    return _url(value, "The LLM address")


def llm_settings(
    config: ScanConfig,
    ui: Ui | None = None,
    *,
    env: Mapping[str, str] | None = None,
    is_tty: bool | None = None,
    read: Callable[[str], str] | None = None,
    read_secret: Callable[[str], str] | None = None,
    keyboard: Keyboard | None = None,
) -> LlmSettings:
    """Return the LLM settings for ``--ai-review``: provider, address, model, judge model and API key.

    Prints the AI review section. Each value comes from its ``AMP_LLM_*``
    variable or is asked for. At the prompt the address defaults to the
    provider's own; without a terminal ``AMP_LLM_URL`` is required, so a
    gateway's key is never sent to the public API by mistake. The judge model
    (only with ``--judge``) defaults to the review model; the judge uses the
    same provider, address and key. The provider, address and models are shown
    before the key is asked for, with a warning when the address is plain http.
    ``read`` reads visible answers and ``read_secret`` the key (see
    ``ask_value`` and ``ask_secret``); with neither, a terminal on stdin is read
    key by key (``keyboard``), and the provider is chosen from a menu.
    """
    ui = Ui() if ui is None else ui
    env = os.environ if env is None else env
    tty = _stdin_is_tty() if is_tty is None else is_tty
    if keyboard is None and read is None and read_secret is None and tty and ui.live:
        keyboard = keyboard_for(sys.stdin)

    def ask(label: str, env_var: str, need: str, *, default: str | None = None, parse: Parse | None = None) -> str:
        return ask_value(label, env_var, need=need, default=default, parse=parse, env=env, is_tty=tty, read=read, ui=ui)

    need = "AI review needs"
    ui.section("AI review")
    start = ui.rows
    if not env.get(LLM_PROVIDER_ENV, "").strip() and tty and keyboard is not None and ui.live:
        try:
            chosen: str = PROVIDERS[choose(ui, "LLM provider", PROVIDERS, keyboard)]
        except (EOFError, KeyboardInterrupt):
            raise ConfigError("Cancelled.") from None
    else:
        chosen = ask(
            "LLM provider (openai or anthropic)", LLM_PROVIDER_ENV, f"{need} an LLM provider.", parse=_provider
        )
    provider: LlmProvider = "anthropic" if chosen == "anthropic" else "openai"
    if ui.live:
        ui.row("Provider", provider)  # Shown while the address is asked; then replaced with both.
    default_url = DEFAULT_LLM_URLS[provider]
    url_need = f"{need} the LLM address, such as {default_url}."
    url = ask("LLM address", LLM_URL_ENV, url_need, default=default_url if tty else None, parse=_llm_url)
    ui.erase_since(start)
    ui.row("Provider", ui.join(provider, clean(url)))
    model = ask("LLM model", LLM_MODEL_ENV, f"{need} an LLM model name.")
    ui.row("Model", clean(model))
    judge_model = None
    if config.judge:
        judge_model = ask("Judge model", LLM_JUDGE_MODEL_ENV, "", default=model)
        ui.row("Judge", clean(judge_model))
    _warn_if_plain_http(url, "LLM API key", ui)
    key_need = f"{need} an LLM API key."
    api_key = ask_secret(
        "LLM API key",
        LLM_API_KEY_ENV,
        row="API key",
        need=key_need,
        check=_ascii_key,
        env=env,
        is_tty=tty,
        read=read_secret,
        keyboard=keyboard,
        ui=ui,
    )
    if api_key is None:  # Only an optional secret is ever None.
        raise ConfigError(key_need)
    return LlmSettings(provider=provider, url=url, model=model, judge_model=judge_model, api_key=api_key)


def git_token(
    config: ScanConfig,
    ui: Ui | None = None,
    *,
    env: Mapping[str, str] | None = None,
    is_tty: bool | None = None,
    read: Callable[[str], str] | None = None,
    keyboard: Keyboard | None = None,
) -> str | None:
    """Return the Git token for the scan, or None for a public GitHub scan without ``GITHUB_TOKEN``.

    Prints the GitHub (or Azure DevOps) section, unless there is no token to
    show. GitHub asks for a token only with ``--include-private``. Without it, a
    set ``GITHUB_TOKEN`` is still used (it raises the API rate limit) but never
    asked for. Azure DevOps always needs a token. Before a token is used over
    plain http, warn that it will be sent unencrypted.
    """
    ui = Ui() if ui is None else ui
    env = os.environ if env is None else env
    optional = False
    if config.provider == "github":
        title, label, env_var, noun = (
            "GitHub",
            "GitHub token (fine-grained, Contents read-only)",
            GITHUB_TOKEN_ENV,
            "GitHub token",
        )
        need = "A GitHub token is needed for private repositories."
        if not config.include_private:
            if not env.get(env_var, "").strip():
                return None
            optional, is_tty = True, False  # Used when set, never asked for.
    else:
        title, label = "Azure DevOps", "Azure DevOps personal access token (Code read, Project and team read)"
        env_var, noun = AZURE_DEVOPS_TOKEN_ENV, "Azure DevOps token"
        need = "An Azure DevOps personal access token is needed."
    ui.section(title)
    _warn_if_plain_http(config.api_url, noun, ui)
    return ask_secret(
        label,
        env_var,
        row="Token",
        need=need,
        optional=optional,
        check=_ascii_token,
        env=env,
        is_tty=is_tty,
        read=read,
        keyboard=keyboard,
        ui=ui,
    )


def _warn_if_plain_http(url: str, noun: str, ui: Ui) -> None:
    if url.startswith("http://"):
        host = clean(urlsplit(url).netloc)
        ui.warn(f"{host} uses plain http, so the {noun} will be sent to it unencrypted.")
