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

"""Tests for sitecustomize.py automatic initialization."""

import sys
import subprocess
from pathlib import Path


def test_sitecustomize_initialization_failure_exits_with_error():
    """
    Test that sitecustomize.py exits with error code 1 when initialization fails.
    """
    bootstrap_dir = (
        Path(__file__).parent.parent / "src" / "amp_instrumentation" / "_bootstrap"
    )

    # Test script that imports sitecustomize (which will fail due to missing env vars)
    script = "import sitecustomize"

    # Run WITHOUT required environment variables to trigger initialization failure
    result = subprocess.run(
        [sys.executable, "-c", script],
        env={"PYTHONPATH": str(bootstrap_dir)},
        capture_output=True,
        text=True,
    )

    # Should exit with error code 1
    assert result.returncode == 1, "Expected exit code 1 when initialization fails"

    # Should print error message to stderr
    assert "ERROR: WSO2 AMP instrumentation failed" in result.stderr
    assert "Check your environment variables and configuration" in result.stderr


def test_sitecustomize_successful_initialization():
    """
    Test that sitecustomize.py initializes successfully when all env vars are set.

    This verifies that sitecustomize actually initializes instrumentation when
    imported with proper environment variable configuration.
    """
    bootstrap_dir = (
        Path(__file__).parent.parent / "src" / "amp_instrumentation" / "_bootstrap"
    )

    # Test script that imports sitecustomize and verifies initialization
    script = """
import sitecustomize
from amp_instrumentation._bootstrap import initialization
# Check that initialization was successful
assert initialization._initialized is True, "Instrumentation should be initialized"
print("INIT_SUCCESS")
"""

    # Run with required environment variables
    env = {
        "PYTHONPATH": str(bootstrap_dir),
        "AMP_AGENT_NAME": "test-app",
        "AMP_OTEL_ENDPOINT": "https://otel.example.com",
        "AMP_AGENT_API_KEY": "test-key",
    }

    result = subprocess.run(
        [sys.executable, "-c", script], env=env, capture_output=True, text=True
    )

    # Should exit successfully
    assert result.returncode == 0, f"Expected success but got: {result.stderr}"
    assert "INIT_SUCCESS" in result.stdout
