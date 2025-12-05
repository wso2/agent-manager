# Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

"""Tests for CLI functionality."""

import os
import sys
import pytest
from pathlib import Path
from unittest.mock import patch, MagicMock, call
from io import StringIO
from amp_instrumentation.cli import main


class TestRunWithSitecustomize:
    """Test the run_with_sitecustomize function."""

    def test_no_args_exits_with_error(self):
        """Test that calling without args exits with error."""
        with patch("sys.stderr", StringIO()):
            with pytest.raises(SystemExit) as exc_info:
                # Call the function with empty arguments
                main.run_with_sitecustomize([])

        assert exc_info.value.code == 1

    @patch("subprocess.run")
    def test_successful_execution(self, mock_run, tmp_path):
        """Test successful command execution with proper environment setup."""
        # Create mock bootstrap directory
        bootstrap_dir = tmp_path / "_bootstrap"
        bootstrap_dir.mkdir()

        # Mock the package directory structure
        with patch.object(Path, "parent", tmp_path):
            # Mock subprocess.run to return success
            mock_result = MagicMock()
            mock_result.returncode = 0
            mock_run.return_value = mock_result

            # Patch check_sitecustomize_conflicts to avoid stderr output
            with patch("amp_instrumentation.cli.main.check_sitecustomize_conflicts"):
                with pytest.raises(SystemExit) as exc_info:
                    # Simulate the __file__ being in cli/main.py
                    with patch.object(
                        main, "__file__", str(tmp_path / "cli" / "main.py")
                    ):
                        main.run_with_sitecustomize(["python", "test.py"])

        # Should exit with return code from subprocess
        assert exc_info.value.code == 0

        # Verify subprocess.run was called with modified PYTHONPATH
        mock_run.assert_called_once()
        call_args = mock_run.call_args

        # Check that env was passed to subprocess.run
        assert "env" in call_args.kwargs
        env = call_args.kwargs["env"]

        # Verify PYTHONPATH was modified to include bootstrap directory
        assert "PYTHONPATH" in env
        pythonpath = env["PYTHONPATH"]
        assert str(bootstrap_dir) in pythonpath

        # Verify the command was correct
        expected_command = ["python", "test.py"]
        assert call_args.args[0] == expected_command

    def test_bootstrap_directory_not_found(self, tmp_path):
        """
        Test that CLI exits with error when bootstrap directory doesn't exist.
        """
        # Create package structure WITHOUT _bootstrap directory
        package_dir = tmp_path / "amp_instrumentation"
        package_dir.mkdir(parents=True)
        cli_dir = package_dir / "cli"
        cli_dir.mkdir()

        with patch("sys.stderr", StringIO()) as mock_stderr:
            with pytest.raises(SystemExit) as exc_info:
                # Simulate the __file__ being in cli/main.py
                with patch.object(main, "__file__", str(cli_dir / "main.py")):
                    main.run_with_sitecustomize(["python", "test.py"])

        # Should exit with error code 1
        assert exc_info.value.code == 1

        # Check that error message was printed to stderr
        stderr_output = mock_stderr.getvalue()
        assert "Error: Bootstrap directory not found" in stderr_output
        assert "Package may not be properly installed" in stderr_output
        assert "pip install --force-reinstall" in stderr_output


class TestCheckSitecustomizeConflicts:
    """Test the check_sitecustomize_conflicts function."""

    def test_warns_when_sitecustomize_exists(self, tmp_path):
        """
        Test that check_sitecustomize_conflicts warns when sitecustomize.py exists.

        When a sitecustomize.py file exists in the current directory, the function
        should print a warning to stderr about potential conflicts.
        """
        # Create a sitecustomize.py file in the temporary directory
        sitecustomize_file = tmp_path / "sitecustomize.py"
        sitecustomize_file.write_text("# Existing sitecustomize.py")

        # Change to the temporary directory
        original_cwd = os.getcwd()
        try:
            os.chdir(tmp_path)

            # Capture stderr output
            with patch("sys.stderr", StringIO()) as mock_stderr:
                main.check_sitecustomize_conflicts()

                stderr_output = mock_stderr.getvalue()
                # Verify warning was printed
                assert "Warning: Found existing sitecustomize.py" in stderr_output
                assert (
                    "This may conflict with WSO2 AMP instrumentation" in stderr_output
                )
        finally:
            os.chdir(original_cwd)

    def test_no_warning_when_sitecustomize_does_not_exist(self, tmp_path):
        """
        Test that check_sitecustomize_conflicts doesn't warn when sitecustomize.py doesn't exist.

        When no sitecustomize.py file exists in the current directory, the function
        should not print any warnings.
        """
        # Change to the temporary directory (no sitecustomize.py exists)
        original_cwd = os.getcwd()
        try:
            os.chdir(tmp_path)

            # Capture stderr output
            with patch("sys.stderr", StringIO()) as mock_stderr:
                main.check_sitecustomize_conflicts()

                stderr_output = mock_stderr.getvalue()
                # Verify no warning was printed
                assert "Warning" not in stderr_output
                assert stderr_output == ""
        finally:
            os.chdir(original_cwd)


class TestCLI:
    """Test the main CLI entry point."""

    @patch("amp_instrumentation.cli.main.run_with_sitecustomize")
    def test_cli_passes_args(self, mock_run):
        """Test that CLI passes arguments to run_with_sitecustomize."""
        with patch.object(
            sys, "argv", ["wso2-agent-trace", "python", "script.py", "--arg"]
        ):
            main.cli()

        mock_run.assert_called_once_with(["python", "script.py", "--arg"])
