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

"""amp_shadow_runner.prompts: secrets from the environment or a hidden terminal prompt."""

import getpass
import io
import warnings
from collections.abc import Callable
from dataclasses import replace
from datetime import UTC, datetime

import pytest

from amp_shadow_runner.config import ConfigError, ScanConfig
from amp_shadow_runner.prompts import Check, ask_secret, git_token

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
