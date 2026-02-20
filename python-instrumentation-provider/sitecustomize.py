import os
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

try:
    # Use traceloop-sdk for OpenLLMetry instrumentation
    from traceloop.sdk import Traceloop

    # Validate and read required configuration
    otel_endpoint = os.getenv("AMP_OTEL_ENDPOINT")
    api_key = os.getenv("AMP_AGENT_API_KEY")
    # Get trace content setting (default: true)
    trace_content = os.getenv("AMP_TRACE_CONTENT", "true")

    if not otel_endpoint or not api_key:
        raise ValueError(
            "Missing required environment variables for Automatic Tracing: AMP_OTEL_ENDPOINT, AMP_AGENT_API_KEY"
        )

    # Set Traceloop environment variables
    os.environ["TRACELOOP_TRACE_CONTENT"] = trace_content
    os.environ["TRACELOOP_METRICS_ENABLED"] = "false"

    # Initialize Traceloop with environment variables
    Traceloop.init(
        telemetry_enabled=False,
        api_endpoint=otel_endpoint,
        headers={"x-amp-api-key": api_key},
    )
    logger.info("Automatic Tracing initialized successfully.")
except Exception as e:
    logger.exception(f"Failed to initialize Automatic Tracing: {e}")
