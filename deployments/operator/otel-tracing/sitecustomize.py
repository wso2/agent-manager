import os
import logging
from validation import validate_resource_attributes

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

try:
    # Use traceloop-sdk for OpenLLMetry instrumentation
    from traceloop.sdk import Traceloop

    # Validate and read required configuration
    app_name = os.getenv("AMP_AGENT_NAME")
    otel_endpoint = os.getenv("AMP_OTEL_ENDPOINT")
    api_key = os.getenv("AMP_AGENT_API_KEY")
    resource_attributes = os.getenv("AMP_TRACE_ATTRIBUTES")
    # Get trace content setting (default: true)
    trace_content = os.getenv("AMP_TRACE_CONTENT", "true")

    if not app_name or not otel_endpoint or not api_key or not resource_attributes:
        raise ValueError(
            "Missing required environment variables for Automatic Tracing: AMP_AGENT_NAME, AMP_OTEL_ENDPOINT, AMP_AGENT_API_KEY, AMP_TRACE_ATTRIBUTES"
        )
    # Validate resource attributes
    validate_resource_attributes(resource_attributes)

    # Set Traceloop environment variables
    os.environ["TRACELOOP_TRACE_CONTENT"] = trace_content
    os.environ["TRACELOOP_METRICS_ENABLED"] = "false"
    os.environ["OTEL_RESOURCE_ATTRIBUTES"] = resource_attributes
    # Intentional for development environment
    os.environ["OTEL_EXPORTER_OTLP_INSECURE"] = "true"

    # Initialize Traceloop with environment variables
    Traceloop.init(
        telemetry_enabled=False,
        app_name=app_name,
        api_endpoint=otel_endpoint,
        headers={"x-api-key": api_key},
    )
    logger.info("Automatic Tracing initialized successfully.")
except Exception as e:
    logger.exception(f"Failed to initialize Automatic Tracing: {e}")
