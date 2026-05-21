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

"""Tests for the init_otel manual-instrumentation helper."""

import os

import pytest
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider

from amp_instrumentation import otel
from amp_instrumentation._bootstrap import constants as env_vars


class TestTracesEndpoint:
    """Test the _traces_endpoint helper."""

    @pytest.mark.parametrize(
        ("base", "expected"),
        [
            ("https://otel.example.com", "https://otel.example.com/v1/traces"),
            ("https://otel.example.com/", "https://otel.example.com/v1/traces"),
            (
                "https://otel.example.com/v1/traces",
                "https://otel.example.com/v1/traces",
            ),
            (
                "https://otel.example.com/v1/traces/",
                "https://otel.example.com/v1/traces",
            ),
        ],
    )
    def test_appends_traces_path_once(self, base, expected):
        assert otel._traces_endpoint(base) == expected


class TestInitOtel:
    """Test the init_otel function."""

    def test_missing_endpoint_raises(self, clean_environment):
        otel._initialized = False
        os.environ[env_vars.AMP_AGENT_API_KEY] = "test-key"
        with pytest.raises(ValueError) as exc_info:
            otel.init_otel()
        assert env_vars.AMP_OTEL_ENDPOINT in str(exc_info.value)

    def test_missing_api_key_raises(self, clean_environment):
        otel._initialized = False
        os.environ[env_vars.AMP_OTEL_ENDPOINT] = "https://otel.example.com"
        with pytest.raises(ValueError) as exc_info:
            otel.init_otel()
        assert env_vars.AMP_AGENT_API_KEY in str(exc_info.value)

    def test_configures_provider_and_is_idempotent(self, configure_environment):
        otel._initialized = False
        otel.init_otel()
        assert otel._initialized is True
        assert isinstance(trace.get_tracer_provider(), TracerProvider)

        # A second call must not raise and must not change state.
        otel.init_otel()
        assert otel._initialized is True
