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

"""
Manual-instrumentation helper.

``init_otel()`` configures the global OpenTelemetry tracer provider to export
spans over OTLP/HTTP to the AMP gateway, using the ``AMP_OTEL_ENDPOINT`` and
``AMP_AGENT_API_KEY`` environment variables (the same ones AMP injects into
platform-hosted agents). It does *no* instrumentation itself — it only wires up
the exporter so a customer emitting their own OpenTelemetry GenAI spans does not
have to hand-write the exporter boilerplate. See the manual-instrumentation
guide for the span-attribute contract those spans should follow.
"""

import logging
import os
import threading

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

from ._bootstrap import constants as env_vars

logger = logging.getLogger(__name__)

# OTLP/HTTP traces signal path appended to AMP_OTEL_ENDPOINT.
_TRACES_PATH = "/v1/traces"

_init_lock = threading.Lock()
_initialized = False


def _traces_endpoint(base: str) -> str:
    """Return the OTLP/HTTP traces endpoint for the given base AMP endpoint."""
    base = base.rstrip("/")
    if base.endswith(_TRACES_PATH):
        return base
    return base + _TRACES_PATH


def _require_env(name: str) -> str:
    value = (os.getenv(name) or "").strip()
    if not value:
        raise ValueError(f"Environment variable '{name}' is required but not set.")
    return value


def init_otel() -> None:
    """
    Configure the global OpenTelemetry tracer provider to export spans to AMP.

    Reads ``AMP_OTEL_ENDPOINT`` and ``AMP_AGENT_API_KEY`` from the environment,
    sets up a :class:`~opentelemetry.sdk.trace.TracerProvider` with a
    :class:`~opentelemetry.sdk.trace.export.BatchSpanProcessor` feeding an
    OTLP/HTTP exporter (``<AMP_OTEL_ENDPOINT>/v1/traces`` with the
    ``x-amp-api-key`` header), and installs it via
    :func:`opentelemetry.trace.set_tracer_provider`.

    Idempotent: a second call is a no-op. Instruments no library — the caller is
    responsible for emitting spans (see the manual-instrumentation guide).

    Raises:
        ValueError: if ``AMP_OTEL_ENDPOINT`` or ``AMP_AGENT_API_KEY`` is unset.
    """
    global _initialized

    with _init_lock:
        if _initialized:
            logger.debug("init_otel: already initialized, skipping")
            return

        endpoint = _require_env(env_vars.AMP_OTEL_ENDPOINT)
        api_key = _require_env(env_vars.AMP_AGENT_API_KEY)

        provider = TracerProvider()
        provider.add_span_processor(
            BatchSpanProcessor(
                OTLPSpanExporter(
                    endpoint=_traces_endpoint(endpoint),
                    headers={"x-amp-api-key": api_key},
                )
            )
        )
        trace.set_tracer_provider(provider)

        _initialized = True
        logger.info("init_otel: OpenTelemetry exporter configured for AMP")
