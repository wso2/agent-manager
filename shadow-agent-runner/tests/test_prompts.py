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

SECRET = "ghp_s3cr3t-value"


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
        need="A test token is needed.",
        optional=optional,
        check=check,
        env={} if env is None else env,
        is_tty=tty,
        read=reader if reader is not None else Reader(),
        out=out if out is not None else io.StringIO(),
    )


def _too_short(secret: str) -> str | None:
    return "The token is too short." if len(secret) < 10 else None


def test_env_var_wins_without_prompting() -> None:
    reader = Reader()
    assert _ask(reader, env={"TEST_TOKEN": f"  {SECRET}\n"}) == SECRET
    assert reader.prompts == []


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
    out = io.StringIO()
    assert _ask(reader, out=out) == SECRET
    assert reader.prompts == ["Test token: "]
    assert out.getvalue() == ""


def test_default_reader_is_getpass_with_its_own_prompt_stream(monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[tuple[str, object]] = []

    def fake_getpass(prompt: str = "Password: ", stream: object = None) -> str:
        calls.append((prompt, stream))
        return SECRET

    monkeypatch.setattr(getpass, "getpass", fake_getpass)
    assert ask_secret("Test token", "TEST_TOKEN", need="", env={}, is_tty=True) == SECRET
    assert calls == [("Test token: ", None)]  # getpass writes the prompt to /dev/tty.


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
    assert _ask(reader, optional=True) is None
    assert reader.prompts == ["Test token (press Enter to skip): "]


def test_blank_on_required_asks_again() -> None:
    reader = Reader("", SECRET)
    out = io.StringIO()
    assert _ask(reader, out=out) == SECRET
    assert len(reader.prompts) == 2
    assert out.getvalue() == "A value is required. Press Ctrl-C to cancel.\n"


def test_blank_every_attempt_raises_without_the_ctrl_c_hint() -> None:
    with pytest.raises(ConfigError) as info:
        _ask(Reader("", "", ""))
    assert str(info.value) == "No usable Test token was given. Stopped after 3 attempts."


def test_spaces_or_control_characters_are_refused() -> None:
    out = io.StringIO()
    assert _ask(Reader("ghp_a b", "ghp_\x1b[2J", SECRET), out=out) == SECRET
    assert out.getvalue() == "The value contains spaces or control characters.\n" * 2
    with pytest.raises(ConfigError) as info:
        _ask(Reader("a\tb", "a\x00b", "a\nb"))
    assert str(info.value) == "No usable Test token was given. Stopped after 3 attempts."
    with pytest.raises(ConfigError) as info:
        _ask(env={"TEST_TOKEN": " ghp_a\r\nX-Evil: 1 "})
    assert str(info.value) == "TEST_TOKEN is not usable: The value contains spaces or control characters."


def test_check_failure_then_success() -> None:
    reader = Reader("short", SECRET)
    out = io.StringIO()
    assert _ask(reader, out=out, check=_too_short) == SECRET
    assert out.getvalue() == "The token is too short.\n"


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
    with pytest.raises(ConfigError) as info:
        _ask(Reader(interrupt))
    assert str(info.value) == "Cancelled."
    assert info.value.__suppress_context__


def test_secret_never_shown(capsys: pytest.CaptureFixture[str]) -> None:
    out = io.StringIO()
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
    out = io.StringIO()
    assert git_token(CONFIG, env={}, is_tty=True, read=reader, out=out) is None
    assert git_token(CONFIG, env={"GITHUB_TOKEN": SECRET}, is_tty=True, read=reader, out=out) == SECRET
    assert reader.prompts == []
    assert out.getvalue() == ""


def test_github_public_warns_about_plain_http_only_with_a_token() -> None:
    config = replace(CONFIG, api_url="http://ghe.internal/api/v3")
    out = io.StringIO()
    assert git_token(config, env={}, out=out) is None
    assert out.getvalue() == ""
    assert git_token(config, env={"GITHUB_TOKEN": SECRET}, out=out) == SECRET
    assert out.getvalue().startswith("Warning: ghe.internal uses plain http")


def test_github_private_asks_for_the_github_token() -> None:
    config = replace(CONFIG, include_private=True)
    reader = Reader(SECRET)
    assert git_token(config, env={}, is_tty=True, read=reader, out=io.StringIO()) == SECRET
    assert reader.prompts == ["GitHub token (fine-grained, Contents read-only): "]
    assert git_token(config, env={"GITHUB_TOKEN": SECRET}, is_tty=False, out=io.StringIO()) == SECRET
    with pytest.raises(ConfigError, match="private repositories.*-e GITHUB_TOKEN"):
        git_token(config, env={}, is_tty=False, out=io.StringIO())


def test_azure_devops_always_asks() -> None:
    reader = Reader(SECRET)
    assert git_token(ADO, env={}, is_tty=True, read=reader, out=io.StringIO()) == SECRET
    assert reader.prompts == ["Azure DevOps personal access token (Code read, Project and team read): "]
    assert git_token(ADO, env={"AZURE_DEVOPS_TOKEN": SECRET}, is_tty=False, out=io.StringIO()) == SECRET


def test_plain_http_warns_before_the_prompt() -> None:
    out = io.StringIO()

    def read(prompt: str) -> str:
        out.write("<prompt>")
        return SECRET

    config = replace(ADO, api_url="http://ado.internal:8080/tfs")
    assert git_token(config, env={}, is_tty=True, read=read, out=out) == SECRET
    assert out.getvalue() == (
        "Warning: ado.internal:8080 uses plain http, so the Azure DevOps token will be sent to it unencrypted.\n"
        "<prompt>"
    )
    assert SECRET not in out.getvalue()


def test_https_does_not_warn() -> None:
    out = io.StringIO()
    git_token(replace(CONFIG, include_private=True), env={}, is_tty=True, read=Reader(SECRET), out=out)
    assert out.getvalue() == ""


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
        out=out if out is not None else io.StringIO(),
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
    out = io.StringIO()
    assert _value(reader, out=out) == "a"
    assert reader.prompts == ["Test value: "] * 3
    assert out.getvalue() == "Choose a or b.\nA value is required. Press Ctrl-C to cancel.\n"
    with pytest.raises(ConfigError) as info:
        _value(Reader("c", "x y", "d"))
    assert str(info.value) == "Choose a or b. Stopped after 3 attempts."


def test_value_default_is_shown_and_taken_on_enter_or_without_a_terminal() -> None:
    reader = Reader("")
    assert _value(reader, default="b") == "b"
    assert reader.prompts == ["Test value [b]: "]
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
    assert capsys.readouterr().err == "Test value: "
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
        env={} if env is None else env,
        is_tty=tty,
        read=read,
        read_secret=read_secret,
        out=io.StringIO() if out is None else out,
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
        "AI review needs the LLM address, such as https://api.anthropic.com. "
        "Run it with docker run -it, or set AMP_LLM_URL and pass it with -e AMP_LLM_URL."
    )


def test_llm_settings_questions_with_defaults() -> None:
    settings, read, read_secret = _llm(
        replace(AI, judge=True), answers=("openai", "", "gpt-x", ""), secret=(f" {LLM_KEY} ",)
    )
    assert settings == LlmSettings("openai", "https://api.openai.com/v1", "gpt-x", "gpt-x", LLM_KEY)
    assert read.prompts == [
        "LLM provider (openai or anthropic): ",
        "LLM address [https://api.openai.com/v1]: ",
        "LLM model: ",
        "Judge model [gpt-x]: ",
    ]
    assert read_secret.prompts == ["LLM API key: "]


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
    out = io.StringIO()
    settings, _, _ = _llm(answers=("x", "openai", "ftp://h", "https://h", "m"), secret=(LLM_KEY,), out=out)
    assert settings.url == "https://h"
    assert out.getvalue() == (
        "The LLM provider must be openai (for any OpenAI-compatible API) or anthropic.\n"
        "The LLM address must start with http:// or https://.\n"
        "AI review: openai at h, model m.\n"
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
    assert str(info.value).startswith("AI review needs ")
    assert str(info.value).endswith(f"set {variable} and pass it with -e {variable}.")


def test_llm_settings_warn_before_sending_the_key_over_plain_http() -> None:
    out = io.StringIO()

    def read_secret(prompt: str) -> str:
        out.write("<prompt>")
        return LLM_KEY

    settings = llm_settings(
        AI,
        env={"AMP_LLM_PROVIDER": "openai", "AMP_LLM_URL": "http://ollama.internal:11434/v1", "AMP_LLM_MODEL": "m"},
        is_tty=True,
        read=Reader(),
        read_secret=read_secret,
        out=out,
    )
    assert settings.url == "http://ollama.internal:11434/v1"
    assert out.getvalue() == (
        "AI review: openai at ollama.internal:11434, model m.\n"
        "Warning: ollama.internal:11434 uses plain http, so the LLM API key will be sent to it unencrypted.\n<prompt>"
    )


def test_llm_key_is_never_shown(capsys: pytest.CaptureFixture[str]) -> None:
    out = io.StringIO()
    settings, _, _ = _llm(env=LLM_ENV, tty=False, out=out)
    texts = [repr(settings), str(settings), out.getvalue()]
    with pytest.raises(ConfigError) as info:
        _llm(env={**LLM_ENV, "AMP_LLM_API_KEY": f"{LLM_KEY} x"}, tty=False, out=out)
    texts += [str(info.value), repr(info.value), out.getvalue(), *capsys.readouterr()]
    assert all(LLM_KEY not in text for text in texts)


def test_llm_settings_line_is_printed_before_the_key_and_never_holds_it() -> None:
    out = io.StringIO()

    def read_secret(prompt: str) -> str:
        out.write("<prompt>")
        return LLM_KEY

    env = {"AMP_LLM_PROVIDER": "openai", "AMP_LLM_URL": "https://gateway.example.com/llm/v1", "AMP_LLM_MODEL": "gpt-x"}
    llm_settings(
        replace(AI, judge=True),
        env={**env, "AMP_LLM_JUDGE_MODEL": "gpt-y"},
        is_tty=True,
        read=Reader(),
        read_secret=read_secret,
        out=out,
    )
    assert out.getvalue() == "AI review: openai at gateway.example.com, model gpt-x (judge gpt-y).\n<prompt>"
    out = io.StringIO()
    _llm(env=LLM_ENV, tty=False, out=out)
    assert out.getvalue() == "AI review: anthropic at api.anthropic.com, model claude-x.\n"
    assert LLM_KEY not in out.getvalue()


def test_a_key_that_is_not_ascii_is_refused() -> None:
    message = "The API key must contain only ASCII letters, digits and symbols."
    with pytest.raises(ConfigError) as info:
        _llm(env={**LLM_ENV, "AMP_LLM_API_KEY": "sk-\u00e9t\u00e9"}, tty=False)
    assert str(info.value) == f"AMP_LLM_API_KEY is not usable: {message}"
    out = io.StringIO()
    env = {name: value for name, value in LLM_ENV.items() if name != "AMP_LLM_API_KEY"}
    settings, _, _ = _llm(env=env, secret=("sk-\u2013key", LLM_KEY), out=out)
    assert settings.api_key == LLM_KEY
    assert out.getvalue().endswith(f"{message}\n")
