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

"""Tests for instrumentation initialization logic."""

import os
import pytest
from amp_instrumentation._bootstrap import initialization
from amp_instrumentation._bootstrap import constants as env_vars


class TestGetRequiredEnvVar:
    """Test the _get_required_env_var helper function."""

    def test_missing_variable_raises_error(self, clean_environment):
        """Test that missing variable raises ConfigurationError."""
        with pytest.raises(initialization.ConfigurationError) as exc_info:
            initialization._get_required_env_var("MISSING_VAR")
        assert "MISSING_VAR" in str(exc_info.value)
        assert "not set or is empty" in str(exc_info.value)

    def test_empty_variable_raises_error(self, clean_environment):
        """Test that empty variable raises ConfigurationError."""
        os.environ["EMPTY_VAR"] = ""
        with pytest.raises(initialization.ConfigurationError) as exc_info:
            initialization._get_required_env_var("EMPTY_VAR")
        assert "EMPTY_VAR" in str(exc_info.value)

    def test_whitespace_only_variable_raises_error(self, clean_environment):
        """Test that whitespace-only variable raises ConfigurationError."""
        os.environ["WHITESPACE_VAR"] = "   "
        with pytest.raises(initialization.ConfigurationError) as exc_info:
            initialization._get_required_env_var("WHITESPACE_VAR")
        assert "WHITESPACE_VAR" in str(exc_info.value)


class TestInitializeInstrumentation:
    """Test the initialize_instrumentation function."""

    def test_successful_initialization(self, clean_environment, mock_traceloop):
        """Test successful initialization with all required env vars."""
        # Set required environment variables
        os.environ[env_vars.AMP_AGENT_NAME] = "test-app"
        os.environ[env_vars.AMP_OTEL_ENDPOINT] = (
            "https://otel.example.com"
        )
        os.environ[env_vars.AMP_AGENT_API_KEY] = "test-key"

        # Reset initialization state
        initialization._initialized = False

        # Call initialization
        initialization.initialize_instrumentation()

        # Verify Traceloop was initialized
        assert mock_traceloop.initialized is True
        assert mock_traceloop.init_kwargs["app_name"] == "test-app"
        assert mock_traceloop.init_kwargs["api_endpoint"] == "https://otel.example.com"
        assert mock_traceloop.init_kwargs["headers"]["x-api-key"] == "test-key"
