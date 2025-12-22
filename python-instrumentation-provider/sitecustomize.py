import os
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


def validate_resource_attributes(resource_attributes):
    """
    Validate that required resource attributes are present.

    Args:
        resource_attributes: Comma-separated key=value pairs

    Raises:
        ValueError: If resource_attributes is empty or missing required attributes
    """
    if not resource_attributes:
        raise ValueError("AMP_TRACE_ATTRIBUTES is required but not set")

    # Define required attributes
    required_attrs = [
        "openchoreo.dev/environment-uid",
        "openchoreo.dev/component-uid",
    ]

    # Parse resource attributes into a dictionary
    attrs_dict = {}
    for attr in resource_attributes.split(","):
        if "=" in attr:
            key, value = attr.split("=", 1)
            key = key.strip()
            value = value.strip()
            if not value:
                raise ValueError(
                    f"Empty value for attribute '{key}' in AMP_TRACE_ATTRIBUTES"
                )
            attrs_dict[key] = value

    # Check for missing attributes
    missing_attrs = [attr for attr in required_attrs if attr not in attrs_dict]
    if missing_attrs:
        raise ValueError(
            f"Missing required resource attributes in AMP_TRACE_ATTRIBUTES: {', '.join(missing_attrs)}. "
        )


try:
    # Use traceloop-sdk for OpenLLMetry instrumentation
    from traceloop.sdk import Traceloop

    # Validate and read required configuration
    otel_endpoint = os.getenv("AMP_OTEL_ENDPOINT")
    api_key = os.getenv("AMP_AGENT_API_KEY")
    resource_attributes = os.getenv("AMP_TRACE_ATTRIBUTES")
    # Get trace content setting (default: true)
    trace_content = os.getenv("AMP_TRACE_CONTENT", "true")

    if not otel_endpoint or not api_key or not resource_attributes:
        raise ValueError(
            "Missing required environment variables for Automatic Tracing: AMP_OTEL_ENDPOINT, AMP_AGENT_API_KEY, AMP_TRACE_ATTRIBUTES"
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
        api_endpoint=otel_endpoint,
        headers={"x-api-key": api_key},
    )
    logger.info("Automatic Tracing initialized successfully.")
except Exception as e:
    logger.exception(f"Failed to initialize Automatic Tracing: {e}")
