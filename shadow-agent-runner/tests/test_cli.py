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

"""The command-line contract: subcommands, version output and exit codes."""

import io
import json
import subprocess
import sys
import time
from pathlib import Path

import pytest

from amp_shadow_runner import __version__
from amp_shadow_runner.cli import main
from amp_shadow_runner.engine import ENGINE_VERSION, RULE_PACK_ID
from amp_shadow_runner.exitcodes import ExitCode
from tests.test_config import _token


def test_exit_code_contract_is_stable() -> None:
    assert {code.name: code.value for code in ExitCode} == {
        "OK": 0,
        "RUNTIME_ERROR": 1,
        "CONFIG_ERROR": 2,
        "AUTH_ERROR": 3,
        "UPGRADE_REQUIRED": 4,
        "PARTIAL_FAILURE": 5,
    }


def test_version_json_shape(capsys: pytest.CaptureFixture[str]) -> None:
    assert main(["version", "--json"]) == ExitCode.OK
    assert json.loads(capsys.readouterr().out) == {
        "runner_version": __version__,
        "engine_version": ENGINE_VERSION,
        "rule_pack_id": RULE_PACK_ID,
    }


def test_version_text(capsys: pytest.CaptureFixture[str]) -> None:
    assert main(["version"]) == ExitCode.OK
    out = capsys.readouterr().out
    assert f"runner:    {__version__}" in out
    assert f"engine:    {ENGINE_VERSION}" in out
    assert f"rule pack: {RULE_PACK_ID}" in out


def test_top_level_version_flag(capsys: pytest.CaptureFixture[str]) -> None:
    assert main(["--version"]) == ExitCode.OK
    assert capsys.readouterr().out.strip() == f"amp-shadow-runner {__version__}"


def test_help_exits_0(capsys: pytest.CaptureFixture[str]) -> None:
    assert main(["--help"]) == ExitCode.OK
    out = capsys.readouterr().out
    for command in ("scan", "upload", "version"):
        assert command in out


def test_unknown_subcommand_exits_2(capsys: pytest.CaptureFixture[str]) -> None:
    assert main(["frobnicate"]) == ExitCode.CONFIG_ERROR
    assert "invalid choice" in capsys.readouterr().err


def test_unknown_option_exits_2() -> None:
    assert main(["version", "--no-such-flag"]) == ExitCode.CONFIG_ERROR


def test_missing_command_exits_2(capsys: pytest.CaptureFixture[str]) -> None:
    assert main([]) == ExitCode.CONFIG_ERROR
    assert "a command is required" in capsys.readouterr().err


def test_upload_without_a_scan_id_exits_2(capsys: pytest.CaptureFixture[str]) -> None:
    assert main(["upload"]) == ExitCode.CONFIG_ERROR
    assert "the following arguments are required: --scan-id" in capsys.readouterr().err


def test_scan_without_flags_exits_2(capsys: pytest.CaptureFixture[str]) -> None:
    assert main(["scan"]) == ExitCode.CONFIG_ERROR
    assert "the following arguments are required: --scan-id" in capsys.readouterr().err


def test_scan_help_lists_the_console_flags(capsys: pytest.CaptureFixture[str]) -> None:
    assert main(["scan", "--help"]) == ExitCode.OK
    out = capsys.readouterr().out
    for flag in (
        "--scan-id",
        "--github-org",
        "--all-repos",
        "--repos",
        "--include-private",
        "--github-api-url",
        "--azure-devops-org",
        "--all-projects",
        "--projects",
        "--azure-devops-url",
        "--read-ci",
        "--ai-review",
        "--judge",
        "--yes",
        "AMP_SCAN_TOKEN",
    ):
        assert flag in out


def test_python_dash_m_entry_point() -> None:
    result = subprocess.run(
        [sys.executable, "-m", "amp_shadow_runner", "version", "--json"],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stderr
    assert set(json.loads(result.stdout)) == {"runner_version", "engine_version", "rule_pack_id"}


def test_scan_of_private_repos_without_a_terminal_or_token_exits_2(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str], tmp_path: Path
) -> None:
    monkeypatch.setattr("amp_shadow_runner.pipeline.DEFAULT_REPORT_PATH", tmp_path / "report.json")
    monkeypatch.setenv("AMP_URL", "https://agent-manager.example.com")
    monkeypatch.setenv("AMP_SCAN_TOKEN", _token(time.time() + 3600))
    monkeypatch.delenv("GITHUB_TOKEN", raising=False)
    monkeypatch.setattr(sys, "stdin", io.StringIO())  # Not a terminal.
    argv = ["scan", "--scan-id", "3f2b6c1e-8d4a-4f5b-9c7e-1a2b3c4d5e6f", "--github-org", "o", "--all-repos"]
    assert main([*argv, "--include-private"]) == ExitCode.CONFIG_ERROR
    assert capsys.readouterr().err == (
        "amp-shadow-runner: A GitHub token is needed for private repositories. "
        "Run it with docker run -it, or set GITHUB_TOKEN and pass it with -e GITHUB_TOKEN.\n"
    )


def test_scan_of_azure_devops_is_not_available_yet(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    token = "ado-pat-value"
    monkeypatch.setenv("AMP_URL", "https://agent-manager.example.com")
    monkeypatch.setenv("AMP_SCAN_TOKEN", _token(time.time() + 3600))
    monkeypatch.setenv("AZURE_DEVOPS_TOKEN", token)
    monkeypatch.setattr(sys, "stdin", io.StringIO())
    argv = ["scan", "--scan-id", "3f2b6c1e-8d4a-4f5b-9c7e-1a2b3c4d5e6f", "--azure-devops-org", "o", "--all-projects"]
    assert main(argv) == ExitCode.RUNTIME_ERROR
    out, err = capsys.readouterr()
    assert err == "amp-shadow-runner: Azure DevOps scans are not available in this version of the runner yet.\n"
    assert token not in out + err
