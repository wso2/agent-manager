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

"""amp_shadow_runner.prompts: secrets and LLM settings from the environment or a terminal prompt."""

import getpass
import io
import warnings
from collections.abc import Callable
from dataclasses import replace
from datetime import UTC, datetime

import pytest

from amp_shadow_runner.config import ConfigError, ScanConfig
from amp_shadow_runner.llm import LlmSettings
from amp_shadow_runner.prompts import Check, ask_secret, ask_value, git_token, llm_settings
from amp_shadow_runner.terminal import Ui
from tests.test_keyboard import Keys
from tests.test_terminal import screen

SECRET = "ghp_s3cr3t-value"
RECEIVED = "  Token      ✓ received (16 characters)\n"


class Log(io.StringIO):
    """A UTF-8 stream that is not a terminal, like stderr in CI."""

    encoding = "utf-8"


class Tty(Log):
    def isatty(self) -> bool:
        return True


def _ui(out: io.StringIO | None = None) -> Ui:
    return Ui(Log() if out is None else out, env={"TERM": "xterm", "NO_COLOR": "1"}, columns=lambda: 100)


class Reader:
    """A fake hidden reader: returns the answers in turn and records each prompt."""

    def __init__(self, *answers: str | BaseException) -> None:
        self.answers = list(answers)
        self.prompts: list[str] = []

    def __call__(self, prompt: str) -> str:
        self.prompts.append(prompt)
        answer = self.answers.pop(0)
        if isinstance(answer, BaseException):
            raise answer
        return answer


def _ask(
    reader: Callable[[str], str] | None = None,
    *,
    env: dict[str, str] | None = None,
    tty: bool = True,
    optional: bool = False,
    out: io.StringIO | None = None,
    check: Check | None = None,
) -> str | None:
    return ask_secret(
        "Test token",
        "TEST_TOKEN",
        row="Token",
        need="A test token is needed.",
        optional=optional,
        check=check,
        env={} if env is None else env,
        is_tty=tty,
        read=reader if reader is not None else Reader(),
        ui=_ui(out),
    )


def _too_short(secret: str) -> str | None:
    return "The token is too short." if len(secret) < 10 else None


def test_env_var_wins_without_prompting() -> None:
    reader = Reader()
    out = Log()
    assert _ask(reader, env={"TEST_TOKEN": f"  {SECRET}\n"}, out=out) == SECRET
    assert reader.prompts == []
    assert out.getvalue() == "  Token      ✓ from TEST_TOKEN\n"


def test_blank_env_var_is_ignored() -> None:
    assert _ask(Reader(SECRET), env={"TEST_TOKEN": "  "}) == SECRET


def test_no_tty_required_names_the_variable() -> None:
    with pytest.raises(ConfigError) as info:
        _ask(tty=False)
    assert str(info.value) == (
        "A test token is needed. Run it with docker run -it, or set TEST_TOKEN and pass it with -e TEST_TOKEN."
    )


def test_no_tty_optional_returns_none() -> None:
    assert _ask(tty=False, optional=True) is None


def test_hidden_reader_is_used() -> None:
    reader = Reader(f" {SECRET} ")
    out = Log()
    assert _ask(reader, out=out) == SECRET
    assert reader.prompts == ["  ? Test token "]
    assert out.getvalue() == RECEIVED  # How long it is, never what it is.


def test_default_reader_is_getpass_with_its_own_prompt_stream(monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[tuple[str, object]] = []

    def fake_getpass(prompt: str = "Password: ", stream: object = None) -> str:
        calls.append((prompt, stream))
        return SECRET

    monkeypatch.setattr(getpass, "getpass", fake_getpass)
    assert ask_secret("Test token", "TEST_TOKEN", row="Token", need="", env={}, is_tty=True, ui=_ui()) == SECRET
    assert calls == [("  ? Test token ", None)]  # getpass writes the prompt to /dev/tty.


def test_input_that_cannot_be_hidden_is_refused() -> None:
    def echoing_reader(prompt: str) -> str:
        warnings.warn("Can not control echo on the terminal.", getpass.GetPassWarning, stacklevel=1)
        return SECRET

    with pytest.raises(ConfigError) as info:
        _ask(echoing_reader)
    assert str(info.value) == (
        "Cannot hide input on this terminal. Set TEST_TOKEN and pass it with -e TEST_TOKEN instead."
    )


def test_blank_on_optional_returns_none() -> None:
    reader = Reader("")
    out = Log()
    assert _ask(reader, optional=True, out=out) is None
    assert reader.prompts == ["  ? Test token (press Enter to skip) "]
    assert out.getvalue() == "  Token      skipped\n"


def test_blank_on_required_asks_again() -> None:
    reader = Reader("", SECRET)
    out = Log()
    assert _ask(reader, out=out) == SECRET
    assert len(reader.prompts) == 2
    assert out.getvalue() == "  ! Nothing was entered. Press Ctrl-C to cancel.\n" + RECEIVED


def test_blank_every_attempt_raises_without_the_ctrl_c_hint() -> None:
    with pytest.raises(ConfigError) as info:
        _ask(Reader("", "", ""))
    assert str(info.value) == "No usable Test token was given. Stopped after 3 attempts."


def test_spaces_or_control_characters_are_refused() -> None:
    out = Log()
    assert _ask(Reader("ghp_a b", "ghp_\x1b[2J", SECRET), out=out) == SECRET
    assert out.getvalue() == "  ! The value contains spaces or control characters.\n" * 2 + RECEIVED
    with pytest.raises(ConfigError) as info:
        _ask(Reader("a\tb", "a\x00b", "a\nb"))
    assert str(info.value) == "No usable Test token was given. Stopped after 3 attempts."
    with pytest.raises(ConfigError) as info:
        _ask(env={"TEST_TOKEN": " ghp_a\r\nX-Evil: 1 "})
    assert str(info.value) == "TEST_TOKEN is not usable: The value contains spaces or control characters."


def test_check_failure_then_success() -> None:
    reader = Reader("short", SECRET)
    out = Log()
    assert _ask(reader, out=out, check=_too_short) == SECRET
    assert out.getvalue() == "  ! The token is too short.\n" + RECEIVED


def test_check_failing_every_attempt_raises() -> None:
    reader = Reader("a", "b", "c")
    with pytest.raises(ConfigError) as info:
        _ask(reader, check=_too_short)
    assert str(info.value) == "The token is too short. Stopped after 3 attempts."
    assert len(reader.prompts) == 3


def test_env_value_failing_check_raises_without_prompting() -> None:
    reader = Reader()
    with pytest.raises(ConfigError) as info:
        _ask(reader, env={"TEST_TOKEN": "short"}, check=_too_short)
    assert str(info.value) == "TEST_TOKEN is not usable: The token is too short."
    assert reader.prompts == []


@pytest.mark.parametrize("interrupt", [EOFError(), KeyboardInterrupt()])
def test_cancel_raises_config_error(interrupt: BaseException) -> None:
    out = Log()
    with pytest.raises(ConfigError) as info:
        _ask(Reader(interrupt), out=out)
    assert str(info.value) == "Cancelled."
    assert info.value.__suppress_context__
    assert out.getvalue() == "\n"  # The question's line is ended before the error is printed.


def test_secret_never_shown(capsys: pytest.CaptureFixture[str]) -> None:
    out = Log()
    leaks: list[str] = []
    for env, answers in (({"TEST_TOKEN": SECRET}, ()), ({}, (SECRET, SECRET, SECRET))):
        with pytest.raises(ConfigError) as info:
            _ask(Reader(*answers), env=env, out=out, check=lambda _: "Not accepted.")
        leaks += [str(info.value), repr(info.value)]
    captured = capsys.readouterr()
    assert all(SECRET not in text for text in [*leaks, out.getvalue(), captured.out, captured.err])


CONFIG = ScanConfig(
    amp_url="https://agent-manager.example.com",
    token="scan-token",
    token_expires_at=datetime(2030, 1, 1, tzinfo=UTC),
    scan_id="3f2b6c1e-8d4a-4f5b-9c7e-1a2b3c4d5e6f",
    provider="github",
    organization="my-org",
    all_repositories=True,
    repositories=(),
    include_private=False,
    read_ci=False,
    api_url="https://api.github.com",
    ai_review=False,
    judge=False,
)
ADO = replace(CONFIG, provider="azure_devops", api_url="https://dev.azure.com")


def test_github_public_uses_a_set_token_but_never_asks() -> None:
    reader = Reader()
    out = Log()
    assert git_token(CONFIG, _ui(out), env={}, is_tty=True, read=reader) is None
    assert out.getvalue() == ""  # No section: nothing to show.
    assert git_token(CONFIG, _ui(out), env={"GITHUB_TOKEN": SECRET}, is_tty=True, read=reader) == SECRET
    assert reader.prompts == []
    assert out.getvalue() == "\n◆ GitHub\n  Token      ✓ from GITHUB_TOKEN\n"


def test_github_public_warns_about_plain_http_only_with_a_token() -> None:
    config = replace(CONFIG, api_url="http://ghe.internal/api/v3")
    out = Log()
    assert git_token(config, _ui(out), env={}) is None
    assert out.getvalue() == ""
    assert git_token(config, _ui(out), env={"GITHUB_TOKEN": SECRET}) == SECRET
    assert out.getvalue().startswith("\n◆ GitHub\n  ! ghe.internal uses plain http")


def test_github_private_asks_for_the_github_token() -> None:
    config = replace(CONFIG, include_private=True)
    reader = Reader(SECRET)
    assert git_token(config, _ui(), env={}, is_tty=True, read=reader) == SECRET
    assert reader.prompts == ["  ? GitHub token (fine-grained, Contents read-only) "]
    assert git_token(config, _ui(), env={"GITHUB_TOKEN": SECRET}, is_tty=False) == SECRET
    with pytest.raises(ConfigError, match="private repositories.*-e GITHUB_TOKEN"):
        git_token(config, _ui(), env={}, is_tty=False)


def test_azure_devops_always_asks() -> None:
    reader = Reader(SECRET)
    assert git_token(ADO, _ui(), env={}, is_tty=True, read=reader) == SECRET
    assert reader.prompts == ["  ? Azure DevOps personal access token (Code read, Project and team read) "]
    assert git_token(ADO, _ui(), env={"AZURE_DEVOPS_TOKEN": SECRET}, is_tty=False) == SECRET


def test_plain_http_warns_before_the_prompt() -> None:
    out = Log()

    def read(prompt: str) -> str:
        out.write("<prompt>")
        return SECRET

    config = replace(ADO, api_url="http://ado.internal:8080/tfs")
    assert git_token(config, _ui(out), env={}, is_tty=True, read=read) == SECRET
    assert out.getvalue() == (
        "\n◆ Azure DevOps\n"
        "  ! ado.internal:8080 uses plain http, so the Azure DevOps token will be sent to it unencrypted.\n"
        "<prompt>" + RECEIVED
    )
    assert SECRET not in out.getvalue()


def test_https_does_not_warn() -> None:
    out = Log()
    git_token(replace(CONFIG, include_private=True), _ui(out), env={}, is_tty=True, read=Reader(SECRET))
    assert out.getvalue() == "\n◆ GitHub\n" + RECEIVED


def _value(
    reader: Callable[[str], str] | None = None,
    *,
    env: dict[str, str] | None = None,
    tty: bool = True,
    default: str | None = None,
    out: io.StringIO | None = None,
) -> str:
    return ask_value(
        "Test value",
        "TEST_VALUE",
        need="A test value is needed.",
        default=default,
        parse=lambda v: v.lower() if v.lower() in ("a", "b") else _raise("Choose a or b."),
        env={} if env is None else env,
        is_tty=tty,
        read=reader if reader is not None else Reader(),
        ui=_ui(out),
    )


def _raise(message: str) -> str:
    raise ConfigError(message)


def test_value_from_env_is_parsed_without_asking() -> None:
    reader = Reader()
    assert _value(reader, env={"TEST_VALUE": " B "}) == "b"
    assert reader.prompts == []
    with pytest.raises(ConfigError) as info:
        _value(env={"TEST_VALUE": "c"})
    assert str(info.value) == "TEST_VALUE is not usable: Choose a or b."


def test_value_is_asked_with_visible_input_and_retried() -> None:
    reader = Reader("c", "", "A")
    out = Log()
    assert _value(reader, out=out) == "a"
    assert reader.prompts == ["  ? Test value "] * 3
    assert out.getvalue() == "  ! Choose a or b.\n  ! Nothing was entered. Press Ctrl-C to cancel.\n"
    with pytest.raises(ConfigError) as info:
        _value(Reader("c", "x y", "d"))
    assert str(info.value) == "Choose a or b. Stopped after 3 attempts."


def test_value_default_is_shown_and_taken_on_enter_or_without_a_terminal() -> None:
    reader = Reader("")
    assert _value(reader, default="b") == "b"
    assert reader.prompts == ["  ? Test value (b) "]
    assert _value(tty=False, default="b") == "b"


def test_value_without_a_terminal_names_the_variable() -> None:
    with pytest.raises(ConfigError) as info:
        _value(tty=False)
    assert str(info.value) == (
        "A test value is needed. Run it with docker run -it, or set TEST_VALUE and pass it with -e TEST_VALUE."
    )


@pytest.mark.parametrize("interrupt", [EOFError(), KeyboardInterrupt()])
def test_value_cancel_raises_config_error(interrupt: BaseException) -> None:
    with pytest.raises(ConfigError, match="^Cancelled.$"):
        _value(Reader(interrupt))


def test_default_value_reader_prompts_on_stderr(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    monkeypatch.setattr("sys.stdin", io.StringIO("a\n"))
    assert ask_value("Test value", "TEST_VALUE", need="", env={}, is_tty=True) == "a"
    assert capsys.readouterr().err == "  ? Test value \n"  # Not a terminal: the line is ended for the log.
    monkeypatch.setattr("sys.stdin", io.StringIO(""))
    with pytest.raises(ConfigError, match="^Cancelled.$"):
        ask_value("Test value", "TEST_VALUE", need="", env={}, is_tty=True)


LLM_KEY = "sk-s3cr3t-llm-key"
AI = replace(CONFIG, ai_review=True)
LLM_ENV = {
    "AMP_LLM_PROVIDER": "Anthropic",
    "AMP_LLM_URL": "https://api.anthropic.com",
    "AMP_LLM_MODEL": "claude-x",
    "AMP_LLM_API_KEY": LLM_KEY,
}


def _llm(
    config: ScanConfig = AI,
    *,
    env: dict[str, str] | None = None,
    answers: tuple[str, ...] = (),
    secret: tuple[str, ...] = (),
    tty: bool = True,
    out: io.StringIO | None = None,
) -> tuple[LlmSettings, Reader, Reader]:
    read, read_secret = Reader(*answers), Reader(*secret)
    settings = llm_settings(
        config,
        _ui(out),
        env={} if env is None else env,
        is_tty=tty,
        read=read,
        read_secret=read_secret,
    )
    return settings, read, read_secret


def test_llm_settings_from_env_ask_nothing() -> None:
    env = {**LLM_ENV, "AMP_LLM_URL": "https://llm.example.com/", "AMP_LLM_JUDGE_MODEL": "judge-x"}
    settings, read, read_secret = _llm(replace(AI, judge=True), env=env, tty=False)
    assert settings == LlmSettings("anthropic", "https://llm.example.com", "claude-x", "judge-x", LLM_KEY)
    assert read.prompts == read_secret.prompts == []


def test_llm_settings_without_a_terminal_default_the_judge_model_but_not_the_address() -> None:
    settings, _, _ = _llm(replace(AI, judge=True), env=LLM_ENV, tty=False)
    assert settings.judge_model == "claude-x"
    env = {name: value for name, value in LLM_ENV.items() if name != "AMP_LLM_URL"}
    with pytest.raises(ConfigError) as info:
        _llm(env=env, tty=False)
    assert str(info.value) == (
        "AI-enhanced detection needs the LLM address, such as https://api.anthropic.com. "
        "Run it with docker run -it, or set AMP_LLM_URL and pass it with -e AMP_LLM_URL."
    )


def test_llm_settings_questions_with_defaults() -> None:
    settings, read, read_secret = _llm(
        replace(AI, judge=True), answers=("openai", "", "gpt-x", ""), secret=(f" {LLM_KEY} ",)
    )
    assert settings == LlmSettings("openai", "https://api.openai.com/v1", "gpt-x", "gpt-x", LLM_KEY)
    assert read.prompts == [
        "  ? LLM provider (openai or anthropic) ",
        "  ? LLM address (https://api.openai.com/v1) ",
        "  ? LLM model ",
        "  ? Judge model (gpt-x) ",
    ]
    assert read_secret.prompts == ["  ? LLM API key "]


def test_llm_settings_without_judge_do_not_ask_for_a_judge_model() -> None:
    settings, read, _ = _llm(answers=("anthropic", "https://proxy.example.com/llm/", "claude-x"), secret=(LLM_KEY,))
    assert (settings.url, settings.judge_model) == ("https://proxy.example.com/llm", None)
    assert len(read.prompts) == 3


def test_llm_settings_refuse_a_bad_provider_or_address() -> None:
    with pytest.raises(ConfigError) as info:
        _llm(env={**LLM_ENV, "AMP_LLM_PROVIDER": "gemini"})
    assert str(info.value) == (
        "AMP_LLM_PROVIDER is not usable: The LLM provider must be openai (for any OpenAI-compatible API) or anthropic."
    )
    for url, problem in (
        ("ftp://llm.example.com", "must start with http:// or https://."),
        ("https://user:pw@llm.example.com", "must not contain a user name or password."),
        ("https://llm.example.com/?key=pw", "must not contain a query or fragment."),
    ):
        with pytest.raises(ConfigError) as info:
            _llm(env={**LLM_ENV, "AMP_LLM_URL": url})
        assert str(info.value) == f"AMP_LLM_URL is not usable: The LLM address {problem}"
        assert "pw" not in str(info.value)
    out = Log()
    settings, _, _ = _llm(answers=("x", "openai", "ftp://h", "https://h", "m"), secret=(LLM_KEY,), out=out)
    assert settings.url == "https://h"
    assert out.getvalue() == (
        "\n◆ AI-enhanced detection\n"
        "  ! The LLM provider must be openai (for any OpenAI-compatible API) or anthropic.\n"
        "  ! The LLM address must start with http:// or https://.\n"
        "  Provider   openai · https://h\n"
        "  Model      m\n"
        "  API key    ✓ received (17 characters)\n"
    )


@pytest.mark.parametrize(
    ("env", "variable"),
    [
        ({}, "AMP_LLM_PROVIDER"),
        ({"AMP_LLM_PROVIDER": "openai"}, "AMP_LLM_URL"),
        ({"AMP_LLM_PROVIDER": "openai", "AMP_LLM_URL": "https://h"}, "AMP_LLM_MODEL"),
        ({"AMP_LLM_PROVIDER": "openai", "AMP_LLM_URL": "https://h", "AMP_LLM_MODEL": "m"}, "AMP_LLM_API_KEY"),
    ],
)
def test_llm_settings_without_a_terminal_name_the_missing_variable(env: dict[str, str], variable: str) -> None:
    with pytest.raises(ConfigError) as info:
        _llm(env=env, tty=False)
    assert str(info.value).startswith("AI-enhanced detection needs ")
    assert str(info.value).endswith(f"set {variable} and pass it with -e {variable}.")


def test_llm_settings_warn_before_sending_the_key_over_plain_http() -> None:
    out = Log()

    def read_secret(prompt: str) -> str:
        out.write("<prompt>")
        return LLM_KEY

    settings = llm_settings(
        AI,
        _ui(out),
        env={"AMP_LLM_PROVIDER": "openai", "AMP_LLM_URL": "http://ollama.internal:11434/v1", "AMP_LLM_MODEL": "m"},
        is_tty=True,
        read=Reader(),
        read_secret=read_secret,
    )
    assert settings.url == "http://ollama.internal:11434/v1"
    assert out.getvalue() == (
        "\n◆ AI-enhanced detection\n"
        "  Provider   openai · http://ollama.internal:11434/v1\n"
        "  Model      m\n"
        "  ! ollama.internal:11434 uses plain http, so the LLM API key will be sent to it unencrypted.\n"
        "<prompt>  API key    ✓ received (17 characters)\n"
    )


def test_llm_key_is_never_shown(capsys: pytest.CaptureFixture[str]) -> None:
    out = Log()
    settings, _, _ = _llm(env=LLM_ENV, tty=False, out=out)
    texts = [repr(settings), str(settings), out.getvalue()]
    with pytest.raises(ConfigError) as info:
        _llm(env={**LLM_ENV, "AMP_LLM_API_KEY": f"{LLM_KEY} x"}, tty=False, out=out)
    texts += [str(info.value), repr(info.value), out.getvalue(), *capsys.readouterr()]
    assert all(LLM_KEY not in text for text in texts)


def test_llm_settings_rows_are_printed_before_the_key_and_never_hold_it() -> None:
    out = Log()

    def read_secret(prompt: str) -> str:
        out.write("<prompt>")
        return LLM_KEY

    env = {"AMP_LLM_PROVIDER": "openai", "AMP_LLM_URL": "https://gateway.example.com/llm/v1", "AMP_LLM_MODEL": "gpt-x"}
    llm_settings(
        replace(AI, judge=True),
        _ui(out),
        env={**env, "AMP_LLM_JUDGE_MODEL": "gpt-y"},
        is_tty=True,
        read=Reader(),
        read_secret=read_secret,
    )
    assert out.getvalue() == (
        "\n◆ AI-enhanced detection\n"
        "  Provider   openai · https://gateway.example.com/llm/v1\n"
        "  Model      gpt-x\n"
        "  Judge      gpt-y\n"
        "<prompt>  API key    ✓ received (17 characters)\n"
    )
    out = Log()
    _llm(env=LLM_ENV, tty=False, out=out)
    assert out.getvalue() == (
        "\n◆ AI-enhanced detection\n"
        "  Provider   anthropic · https://api.anthropic.com\n"
        "  Model      claude-x\n"
        "  API key    ✓ from AMP_LLM_API_KEY\n"
    )
    assert LLM_KEY not in out.getvalue()


def test_a_key_that_is_not_ascii_is_refused() -> None:
    message = "The API key must contain only ASCII letters, digits and symbols."
    with pytest.raises(ConfigError) as info:
        _llm(env={**LLM_ENV, "AMP_LLM_API_KEY": "sk-\u00e9t\u00e9"}, tty=False)
    assert str(info.value) == f"AMP_LLM_API_KEY is not usable: {message}"
    out = Log()
    env = {name: value for name, value in LLM_ENV.items() if name != "AMP_LLM_API_KEY"}
    settings, _, _ = _llm(env=env, secret=("sk-\u2013key", LLM_KEY), out=out)
    assert settings.api_key == LLM_KEY
    assert out.getvalue().endswith(f"  ! {message}\n  API key    ✓ received (17 characters)\n")


def _terminal() -> tuple[Tty, Ui]:
    out = Tty()
    return out, Ui(out, env={"TERM": "xterm", "NO_COLOR": "1"}, columns=lambda: 100)


def _typed(out: Tty, *answers: str) -> Callable[[str], str]:
    """Answers typed at a terminal: the prompt and the echoed answer are on the screen."""
    pending = list(answers)

    def read(prompt: str) -> str:
        answer = pending.pop(0)
        out.write(prompt + answer)
        return answer

    return read


def test_on_a_terminal_the_provider_is_a_menu_and_each_answer_replaces_its_question() -> None:
    out, ui = _terminal()
    keys = Keys("\x1b[B", "\r", f" {LLM_KEY} \r")
    read = _typed(out, "ftp://wrong\n", "\n", "claude-x\n")
    settings = llm_settings(AI, ui, env={}, is_tty=True, read=read, keyboard=keys)
    assert settings == LlmSettings("anthropic", "https://api.anthropic.com", "claude-x", None, LLM_KEY)
    assert screen(out.getvalue()) == (
        "\n◆ AI-enhanced detection\n"
        "  Provider   anthropic · https://api.anthropic.com\n"  # The refused address is erased too.
        "  Model      claude-x\n"
        "  API key    ✓ received (17 characters)\n"
    )
    assert "  ! The LLM address must start with http:// or https://." in out.getvalue()
    assert keys.modes == ["raw", "discarded", "restored"] * 2
    assert LLM_KEY not in out.getvalue()


def test_on_a_terminal_a_set_provider_skips_the_menu() -> None:
    out, ui = _terminal()
    keys = Keys(f"{LLM_KEY}\r")
    env = {"AMP_LLM_PROVIDER": "openai", "AMP_LLM_MODEL": "gpt-x"}
    settings = llm_settings(AI, ui, env=env, is_tty=True, read=_typed(out, "\n"), keyboard=keys)
    assert (settings.provider, settings.url) == ("openai", "https://api.openai.com/v1")
    assert screen(out.getvalue()).startswith(
        "\n◆ AI-enhanced detection\n  Provider   openai · https://api.openai.com/v1\n"
    )


def test_ctrl_c_in_the_menu_cancels() -> None:
    out, ui = _terminal()
    keys = Keys("\x03")
    with pytest.raises(ConfigError, match="^Cancelled.$"):
        llm_settings(AI, ui, env={}, is_tty=True, read=_typed(out), keyboard=keys)
    assert keys.modes == ["raw", "restored"]


def test_on_a_terminal_the_token_is_read_as_dots_and_retried_when_blank() -> None:
    out, ui = _terminal()
    keys = Keys("\r", SECRET, "\r")
    config = replace(CONFIG, include_private=True)
    assert git_token(config, ui, env={}, is_tty=True, keyboard=keys) == SECRET
    assert "  ! Nothing was entered. Press Ctrl-C to cancel." in out.getvalue()
    assert screen(out.getvalue()) == "\n◆ GitHub\n" + RECEIVED
    assert SECRET not in out.getvalue()


def test_ctrl_c_at_the_token_restores_the_terminal_and_ends_the_line() -> None:
    out, ui = _terminal()
    keys = Keys("ghp_", "\x03")
    with pytest.raises(ConfigError, match="^Cancelled.$"):
        git_token(replace(CONFIG, include_private=True), ui, env={}, is_tty=True, keyboard=keys)
    assert keys.modes == ["raw", "restored"]
    assert out.getvalue().endswith("••••\n")


def test_on_a_terminal_a_token_with_control_characters_or_other_than_ascii_is_asked_again() -> None:
    out, ui = _terminal()
    keys = Keys("ghp_a\tb\r", "ghp_\u00e9\r", f"{SECRET}\r")
    assert git_token(replace(CONFIG, include_private=True), ui, env={}, is_tty=True, keyboard=keys) == SECRET
    assert "  ! The value contains spaces or control characters.\n" in out.getvalue()
    assert "  ! The token must contain only ASCII letters, digits and symbols.\n" in out.getvalue()
    assert screen(out.getvalue()) == "\n◆ GitHub\n" + RECEIVED
    with pytest.raises(ConfigError, match="GITHUB_TOKEN is not usable: The token must contain only ASCII"):
        git_token(CONFIG, _ui(), env={"GITHUB_TOKEN": "ghp_\u00e9"})
