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

"""amp_shadow_runner.config: the scan flags the console emits, and the environment."""

import base64
import dataclasses
import json
import shlex
import time
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, cast

import pytest

from amp_shadow_runner.cli import build_parser, main
from amp_shadow_runner.config import MAX_REPOSITORIES, ConfigError, ScanConfig, build_scan_config
from amp_shadow_runner.exitcodes import ExitCode
from amp_shadow_runner.scantoken import ScanTokenError

NOW = 1_800_000_000
SCAN_ID = "3f2b6c1e-8d4a-4f5b-9c7e-1a2b3c4d5e6f"
AMP_URL = "https://agent-manager.example.com"


def _b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")


def _token(exp: float) -> str:
    header = _b64url(json.dumps({"alg": "RS256", "typ": "JWT"}).encode())
    body = _b64url(json.dumps({"exp": exp, "scope": "amp:shadow-agent:publish"}).encode())
    return f"{header}.{body}.{_b64url(b'dummy-signature')}"


TOKEN = _token(NOW + 3600)
ENV = {"AMP_URL": AMP_URL, "AMP_SCAN_TOKEN": TOKEN}


def _console_argv(command: str) -> list[str]:
    """The ``scan`` arguments of a command exactly as the console builds it (one option per line)."""
    words = shlex.split(command.replace("\\\n", " "))
    return words[words.index("scan") :]


def _command(*lines: str) -> str:
    return " \\\n  ".join(
        [
            "docker run --rm -it",
            '--user "$(id -u):$(id -g)"',
            '-v "$PWD:/out"',
            f"-e AMP_URL={AMP_URL}",
            f"-e AMP_SCAN_TOKEN={TOKEN}",
            "ghcr.io/wso2/amp-shadow-runner:1.0.0 scan",
            f"--scan-id {SCAN_ID}",
            *lines,
        ]
    )


def _config(argv: list[str], env: dict[str, str] | None = None) -> ScanConfig:
    return build_scan_config(build_parser().parse_args(argv), ENV if env is None else env)


def _refuse(argv: list[str], env: dict[str, str] | None = None) -> str:
    with pytest.raises((ConfigError, ScanTokenError)) as info:
        _config(argv, env)
    exc = info.value
    assert isinstance(exc, ConfigError | ScanTokenError)
    assert exc.exit_code is ExitCode.CONFIG_ERROR
    assert TOKEN not in str(exc)
    assert exc.__cause__ is None
    return str(exc)


def _scan(*flags: str) -> list[str]:
    return ["scan", "--scan-id", SCAN_ID, *flags]


def test_github_all_repositories() -> None:
    config = _config(_console_argv(_command("--github-org my-org", "--all-repos")))
    assert config == ScanConfig(
        amp_url=AMP_URL,
        token=TOKEN,
        token_expires_at=datetime.fromtimestamp(NOW + 3600, tz=UTC),
        scan_id=SCAN_ID,
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


def test_github_repository_list_with_options() -> None:
    command = _command(
        "--github-org Acme",
        "--repos api,web.app,Api,_tools",
        "--include-private",
        "--read-ci",
        "--github-api-url https://ghe.acme.test/api/v3",
        "--ai-review",
        "--judge",
    )
    config = _config(_console_argv(command))
    assert config.provider == "github"
    assert config.organization == "Acme"
    assert not config.all_repositories
    assert config.repositories == ("api", "web.app", "_tools")  # Case-insensitive de-duplication keeps the first.
    assert (config.include_private, config.read_ci, config.ai_review, config.judge) == (True, True, True, True)
    assert config.api_url == "https://ghe.acme.test/api/v3"


def test_azure_devops_is_an_unsupported_provider() -> None:
    config = _config(_scan("--github-org", "o", "--all-repos"))
    with pytest.raises(ConfigError) as info:
        dataclasses.replace(config, provider=cast(Any, "azure_devops"))  # A provider the type no longer allows.
    assert str(info.value) == "'azure_devops' is not a supported provider. This runner scans GitHub only."
    assert info.value.exit_code is ExitCode.CONFIG_ERROR
    assert TOKEN not in str(info.value)


def test_values_are_normalised() -> None:
    config = _config(
        _scan("--github-org", "o", "--repos", " a , ,b,", "--github-api-url", "https://ghe.test/api/v3//"),
        {"AMP_URL": "http://localhost:9000/ ", "AMP_SCAN_TOKEN": f"  {TOKEN}\n"},
    )
    assert config.repositories == ("a", "b")
    assert config.api_url == "https://ghe.test/api/v3"
    assert config.amp_url == "http://localhost:9000"
    assert config.token == TOKEN
    assert _config(["scan", "--scan-id", SCAN_ID.upper(), "--github-org", "o", "--all-repos"]).scan_id == SCAN_ID


def test_token_is_not_in_repr() -> None:
    config = _config(_scan("--github-org", "o", "--all-repos"))
    assert TOKEN not in repr(config)
    assert "token=" not in repr(config)


@pytest.mark.parametrize(
    ("flags", "message"),
    [
        (["--github-org", "o", "--all-repos", "--judge"], "--judge needs --ai-review."),
        (["--github-org", "o"], "Choose what to scan: --all-repos, or --repos"),
        (["--github-org", "bad-", "--all-repos"], "'bad-' is not a GitHub organization or user name"),
        (["--github-org", "a" * 40, "--all-repos"], "is not a GitHub organization or user name"),
        (["--github-org", "<organization>", "--all-repos"], "is not a GitHub organization or user name"),
        (["--github-org", "o", "--repos", "other/api"], "Repository 'other/api' is not in o."),
        (["--github-org", "o", "--repos", "o/a/b"], "'o/a/b' is not a GitHub repository name."),
        (["--github-org", "o", "--repos", "a" * 101], "is not a GitHub repository name."),
        (["--github-org", "o", "--repos", "a,.."], "'..' is not a GitHub repository name."),
        (["--github-org", "o", "--repos", " , ,"], "--repos lists no repository names."),
    ],
)
def test_bad_flags_are_refused(flags: list[str], message: str) -> None:
    assert message in _refuse(_scan(*flags))


def test_bad_scan_id_is_refused() -> None:
    for scan_id in ["<scan-id>", "", "abc", f"{SCAN_ID}/x", f"{{{SCAN_ID}}}"]:
        assert _refuse(["scan", "--scan-id", scan_id, "--github-org", "o", "--all-repos"]).startswith(
            "--scan-id must be the scan ID that Agent Manager issued."
        )


def test_too_many_repositories_are_refused() -> None:
    names = ",".join(f"r{i}" for i in range(MAX_REPOSITORIES))
    assert len(_config(_scan("--github-org", "o", "--repos", names)).repositories) == MAX_REPOSITORIES
    assert _refuse(_scan("--github-org", "o", "--repos", names + ",extra")).startswith(
        "--repos lists 5001 repository names, and a scan takes at most 5000."
    )


@pytest.mark.parametrize(
    ("url", "message"),
    [
        ("ftp://ghe.acme.test", "must start with http:// or https://."),
        ("ghe.acme.test/api/v3", "must start with http:// or https://."),
        ("https://", "is not a valid URL."),
        ("https://ghe.acme.test:99999", "is not a valid URL."),
        ("https://ghe.acme.test/a b", "is not a valid URL."),
        ("https://ghe.acme.test\\@evil.test/", "is not a valid URL."),
        ("https://user:pw@ghe.acme.test", "must not contain a user name or password."),
        ("https://ghe.acme.test/api?x=1", "must not contain a query or fragment."),
        ("https://ghe.acme.test/api#top", "must not contain a query or fragment."),
    ],
)
def test_bad_provider_urls_are_refused(url: str, message: str) -> None:
    error = _refuse(_scan("--github-org", "o", "--all-repos", "--github-api-url", url))
    assert error == f"--github-api-url {message}"
    assert url not in error


def test_plain_http_is_accepted() -> None:
    # Agent Manager accepts http for a self-hosted server; the runner warns before sending a token (R4).
    for url in ["http://ghe.acme.test/api/v3", "http://[::1]:8443/api/v3"]:
        assert _config(_scan("--github-org", "o", "--all-repos", "--github-api-url", url)).api_url == url


def test_url_scheme_is_lowercased() -> None:
    config = _config(
        _scan("--github-org", "o", "--all-repos", "--github-api-url", "HTTP://GHE.acme.test/API"),
        {"AMP_URL": "HTTPS://agent-manager.example.com", "AMP_SCAN_TOKEN": TOKEN},
    )
    assert (config.api_url, config.amp_url) == ("http://GHE.acme.test/API", "https://agent-manager.example.com")


def test_owner_slash_name_is_kept_as_the_name() -> None:
    config = _config(_scan("--github-org", "Acme", "--repos", "acme/api,api,ACME/Web,web"))
    assert config.repositories == ("api", "Web")


def test_equals_form_takes_names_starting_with_a_dash() -> None:
    assert _config(_scan("--github-org", "o", "--repos=-foo,bar")).repositories == ("-foo", "bar")


@pytest.mark.parametrize(
    ("env", "message"),
    [
        ({"AMP_SCAN_TOKEN": TOKEN}, "AMP_URL is not set. Copy the whole command from Agent Manager again."),
        ({"AMP_URL": " ", "AMP_SCAN_TOKEN": TOKEN}, "AMP_URL is not set."),
        ({"AMP_URL": "agent-manager.example.com", "AMP_SCAN_TOKEN": TOKEN}, "AMP_URL must start with http://"),
        ({"AMP_URL": f"{AMP_URL}?a=1", "AMP_SCAN_TOKEN": TOKEN}, "AMP_URL must not contain a query or fragment."),
        ({"AMP_URL": AMP_URL, "AMP_SCAN_TOKEN": "not-a-jwt"}, "AMP_SCAN_TOKEN is not a valid scan token."),
    ],
)
def test_bad_environment_is_refused(env: dict[str, str], message: str) -> None:
    assert _refuse(_scan("--github-org", "o", "--all-repos"), env).startswith(message)


@pytest.mark.parametrize(
    "env",
    [{}, {"AMP_URL": AMP_URL}, {"AMP_URL": AMP_URL, "AMP_SCAN_TOKEN": " \n"}, {"AMP_URL": "not a url"}],
)
def test_without_a_scan_token_the_scan_only_saves(env: dict[str, str]) -> None:
    config = _config(_scan("--github-org", "o", "--all-repos"), env)
    assert (config.amp_url, config.token, config.token_expires_at) == (None, None, None)


@pytest.mark.parametrize("exp", [NOW - 1, NOW + 60])
def test_an_expired_or_short_lived_token_is_kept_for_the_upload_step(exp: int) -> None:
    config = _config(_scan("--github-org", "o", "--all-repos"), {"AMP_URL": AMP_URL, "AMP_SCAN_TOKEN": _token(exp)})
    assert config.token_expires_at == datetime.fromtimestamp(exp, UTC)


def test_default_environment_is_os_environ(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("AMP_URL", AMP_URL)
    monkeypatch.setenv("AMP_SCAN_TOKEN", _token(time.time() + 3600))
    assert build_scan_config(build_parser().parse_args(_scan("--github-org", "o", "--all-repos"))).amp_url == AMP_URL


@pytest.mark.parametrize(
    "flags",
    [
        [],
        ["--github-org", "o", "--azure-devops-org", "o", "--all-repos"],  # Not an option any more.
        ["--azure-devops-org", "o", "--all-projects"],  # --github-org is required.
        ["--github-org", "o", "--all-repos", "--azure-devops-url", "https://dev.azure.com"],
        ["--github-org", "o", "--all-repos", "--repos", "a"],
        ["--github-org", "o", "--all-repos", "--ai"],  # No abbreviations: --ai is not --ai-review.
    ],
)
def test_usage_errors_exit_2(flags: list[str], capsys: pytest.CaptureFixture[str]) -> None:
    assert main(_scan(*flags)) == ExitCode.CONFIG_ERROR
    assert "error:" in capsys.readouterr().err


def test_cli_prints_only_the_message(monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]) -> None:
    token = _token(time.time() + 3600)
    monkeypatch.setenv("AMP_URL", AMP_URL)
    monkeypatch.setenv("AMP_SCAN_TOKEN", token)
    assert main(_scan("--github-org", "o", "--all-repos", "--judge")) == ExitCode.CONFIG_ERROR
    out, err = capsys.readouterr()
    assert (out, err) == ("", "amp-shadow-runner: --judge needs --ai-review.\n")

    monkeypatch.setenv("AMP_SCAN_TOKEN", "x.y")
    assert main(_scan("--github-org", "o", "--all-repos")) == ExitCode.CONFIG_ERROR
    assert capsys.readouterr().err == (
        "amp-shadow-runner: AMP_SCAN_TOKEN is not a valid scan token. Copy the whole command from Agent Manager again.\n"
    )

    monkeypatch.setenv("AMP_SCAN_TOKEN", token)  # A valid command reaches the scan, which needs /out first.
    monkeypatch.setattr("amp_shadow_runner.pipeline.DEFAULT_REPORT_PATH", Path("/nonexistent-amp-out/report.json"))
    assert main(_scan("--github-org", "o", "--all-repos")) == ExitCode.CONFIG_ERROR
    out, err = capsys.readouterr()
    assert err.startswith("amp-shadow-runner: Cannot write the scan report to /nonexistent-amp-out")
    assert token not in out + err
