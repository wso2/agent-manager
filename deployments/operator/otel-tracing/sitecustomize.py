import os
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

try:
    # Use traceloop-sdk for OpenLLMetry instrumentation
    from traceloop.sdk import Traceloop

    os.environ["TRACELOOP_TRACE_CONTENT"] = os.environ.get("AMP_TRACELOOP_TRACE_CONTENT", "true")
    os.environ["OTEL_EXPORTER_OTLP_INSECURE"] = os.environ.get("AMP_OTEL_EXPORTER_OTLP_INSECURE", "true")
    os.environ["TRACELOOP_METRICS_ENABLED"] = os.environ.get("AMP_TRACELOOP_METRICS_ENABLED", "false")

    # Initialize Traceloop with environment variables
    Traceloop.init(
        telemetry_enabled=os.getenv("AMP_TRACELOOP_TELEMETRY_ENABLED", "false"),
        app_name=os.getenv("AMP_APP_NAME", "sample-app"),
        resource_attributes={"env": os.getenv("AMP_ENV", "prod"), "version": os.getenv("AMP_APP_VERSION", "1.0.0")},
        api_endpoint=os.getenv("AMP_OTEL_EXPORTER_OTLP_ENDPOINT", "data-prepper.openchoreo-observability-plane.svc.cluster.local:21893"),
        headers={"x-api-key": os.getenv("AMP_API_KEY", "")}
    )
    Traceloop.set_association_properties({ "component_id": os.getenv("AMP_COMPONENT_ID", "default-component") })
    logger.info("Automatic Tracing initialized successfully.")
except Exception as e:
    logger.error(f"Failed to initialize Automatic Tracing: {e}")
