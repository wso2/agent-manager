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
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import (
    InMemorySpanExporter,
)

from amp_instrumentation import otel
from amp_instrumentation._bootstrap import constants as env_vars
from amp_instrumentation.conversation import ConversationIdSpanProcessor


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
            (
                "http://localhost:9000/m/observability/otel",
                "http://localhost:9000/m/observability/otel/v1/traces",
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

    def test_exports_to_the_module_endpoint_with_the_key(
        self, clean_environment, monkeypatch
    ):
        captured = {}

        class FakeExporter:
            def __init__(self, **kwargs):
                captured.update(kwargs)

            def export(self, spans):
                return None

            def shutdown(self):
                return None

            def force_flush(self, timeout_millis=None):
                return True

        monkeypatch.setattr(otel, "OTLPSpanExporter", FakeExporter)
        monkeypatch.setenv(
            env_vars.AMP_OTEL_ENDPOINT, "http://localhost:9000/m/observability/otel"
        )
        monkeypatch.setenv(env_vars.AMP_AGENT_API_KEY, "agent-key")
        otel._initialized = False
        otel.init_otel()
        assert captured == {
            "endpoint": "http://localhost:9000/m/observability/otel/v1/traces",
            "headers": {"x-amp-api-key": "agent-key"},
        }

    def test_configures_provider_and_is_idempotent(self, configure_environment):
        otel._initialized = False
        otel.init_otel()
        assert otel._initialized is True
        assert isinstance(trace.get_tracer_provider(), TracerProvider)

        # A second call must not raise and must not change state.
        otel.init_otel()
        assert otel._initialized is True

    def test_registers_conversation_processor_once_beside_the_exporter(
        self, configure_environment, monkeypatch, record_processors
    ):
        created = []

        def make_provider():
            provider = TracerProvider()
            created.append((provider, record_processors(provider)))
            return provider

        exporter_processor = SimpleSpanProcessor(InMemorySpanExporter())
        monkeypatch.setattr(otel, "TracerProvider", make_provider)
        monkeypatch.setattr(
            otel, "BatchSpanProcessor", lambda exporter: exporter_processor
        )
        monkeypatch.setattr(trace, "set_tracer_provider", lambda provider: None)
        monkeypatch.setattr(otel, "_initialized", False)

        otel.init_otel()
        otel.init_otel()

        assert len(created) == 1
        _, added = created[0]
        assert added[0] is exporter_processor
        assert [type(p) for p in added[1:]] == [ConversationIdSpanProcessor]
