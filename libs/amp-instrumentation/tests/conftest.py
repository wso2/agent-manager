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

"""Pytest configuration and shared fixtures."""

import os
import pytest
from typing import Generator, Dict


@pytest.fixture
def clean_environment() -> Generator[None, None, None]:
    """
    Fixture to clean up environment variables before and after tests.

    Yields control to the test and restores the original environment afterward.
    """
    # Save original environment
    original_env = os.environ.copy()

    # Remove AMP-related environment variables
    amp_vars = [
        "AMP_AGENT_NAME",
        "AMP_OTEL_ENDPOINT",
        "AMP_AGENT_API_KEY",
    ]

    for var in amp_vars:
        os.environ.pop(var, None)

    yield

    # Restore original environment
    os.environ.clear()
    os.environ.update(original_env)


@pytest.fixture
def set_env_vars() -> Dict[str, str]:
    """
    Fixture providing a valid set of environment variables.

    Returns:
        Dictionary with valid AMP configuration
    """
    return {
        "AMP_AGENT_NAME": "test-app",
        "AMP_OTEL_ENDPOINT": "https://otel.example.com",
        "AMP_AGENT_API_KEY": "test-api-key",
    }


@pytest.fixture
def configure_environment(clean_environment, set_env_vars) -> Dict[str, str]:
    """
    Fixture that sets valid environment variables for testing.

    Args:
        clean_environment: Fixture that ensures clean environment
        set_env_vars: Fixture with valid configuration

    Returns:
        Dictionary with the set environment variables
    """
    for key, value in set_env_vars.items():
        os.environ[key] = value
    return set_env_vars


@pytest.fixture
def mock_traceloop(monkeypatch):
    """
    Fixture to mock the Traceloop SDK to avoid actual initialization during tests.

    Args:
        monkeypatch: Pytest monkeypatch fixture
    """

    class MockTraceloop:
        initialized = False
        init_kwargs = {}

        @classmethod
        def init(cls, **kwargs):
            cls.initialized = True
            cls.init_kwargs = kwargs

        @classmethod
        def reset(cls):
            """Reset class state for clean test isolation."""
            cls.initialized = False
            cls.init_kwargs = {}

    # Reset state before each test
    MockTraceloop.reset()

    # Mock the import
    import sys
    from unittest.mock import MagicMock

    mock_module = MagicMock()
    mock_module.Traceloop = MockTraceloop
    sys.modules["traceloop.sdk"] = mock_module

    yield MockTraceloop

    # Clean up
    if "traceloop.sdk" in sys.modules:
        del sys.modules["traceloop.sdk"]
    if "traceloop" in sys.modules:
        del sys.modules["traceloop"]
