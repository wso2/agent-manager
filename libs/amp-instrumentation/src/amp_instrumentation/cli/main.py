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

"""
WSO2 Agent Management Platform - CLI wrapper for automatic instrumentation.

This CLI tool wraps Python commands to automatically inject tracing instrumentation
using sitecustomize.py and PYTHONPATH manipulation.
"""

import subprocess
import sys
import os
from pathlib import Path
from typing import List, Dict, NoReturn


def check_sitecustomize_conflicts() -> None:
    """
    Check if there's an existing sitecustomize.py that might conflict.

    Warns the user if a sitecustomize.py file exists in the current directory,
    as it may interfere with WSO2 AMP instrumentation.
    """
    cwd = os.getcwd()
    sitecustomize_path = Path(cwd) / "sitecustomize.py"

    if sitecustomize_path.exists():
        print(
            "Warning: Found sitecustomize.py in current directory, which may conflict with instrumentation.\n",
            file=sys.stderr,
        )


def run_with_sitecustomize(args: List[str]) -> NoReturn:
    """
    This function modifies the PYTHONPATH environment variable to prepend the
    _bootstrap directory, which contains sitecustomize.py for automatic
    instrumentation initialization.

    Args:
        args: Command line arguments to pass to the subprocess

    Raises:
        SystemExit: Always exits with the return code of the subprocess

    Example:
        >>> run_with_sitecustomize(["python", "my_script.py"])
    """
    # Validate that we have arguments to run
    if not args:
        print(
            "Error: No command specified. Usage: amp-instrument <command> [args...]",
            file=sys.stderr,
        )
        sys.exit(1)

    # Check for potential conflicts
    check_sitecustomize_conflicts()

    # Find the _bootstrap directory in the installed package
    # __file__ is cli/main.py, so go up two levels to amp_instrumentation/
    package_dir = Path(__file__).parent.parent
    bootstrap_dir = package_dir / "_bootstrap"

    if not bootstrap_dir.exists():
        print(
            f"Error: Package installation is incomplete. Try: pip install --force-reinstall amp-instrumentation",
            file=sys.stderr,
        )
        sys.exit(1)

    # Prepare environment with modified PYTHONPATH
    env: Dict[str, str] = os.environ.copy()
    current_pythonpath = env.get("PYTHONPATH", "")

    # Prepend bootstrap directory to PYTHONPATH
    if current_pythonpath:
        env["PYTHONPATH"] = f"{bootstrap_dir}{os.pathsep}{current_pythonpath}"
    else:
        env["PYTHONPATH"] = str(bootstrap_dir)

    # Run the command with modified environment
    try:
        result = subprocess.run(args, env=env)
        sys.exit(result.returncode)
    except Exception as e:
        print(f"Error running command: {e}", file=sys.stderr)
        sys.exit(1)


def cli() -> None:
    """
    Main CLI entry point for amp-instrument command.

    This function is registered as a console script entry point in pyproject.toml.
    It wraps any Python command with automatic WSO2 AMP instrumentation.

    Usage:
        amp-instrument python my_script.py
        amp-instrument uvicorn app:main --reload
        amp-instrument poetry run python script.py
    """
    args = sys.argv[1:]
    run_with_sitecustomize(args)


if __name__ == "__main__":
    cli()
