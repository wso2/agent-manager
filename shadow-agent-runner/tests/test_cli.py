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

import json
import subprocess
import sys

import pytest

from amp_shadow_runner import __version__
from amp_shadow_runner.cli import main
from amp_shadow_runner.engine import ENGINE_VERSION, RULE_PACK_ID
from amp_shadow_runner.exitcodes import ExitCode


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


@pytest.mark.parametrize("command", ["scan", "upload"])
def test_stub_commands_exit_1(command: str, capsys: pytest.CaptureFixture[str]) -> None:
    assert main([command]) == ExitCode.RUNTIME_ERROR
    assert capsys.readouterr().err.strip() == f"amp-shadow-runner {command}: not implemented yet"


def test_python_dash_m_entry_point() -> None:
    result = subprocess.run(
        [sys.executable, "-m", "amp_shadow_runner", "version", "--json"],
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stderr
    assert set(json.loads(result.stdout)) == {"runner_version", "engine_version", "rule_pack_id"}
