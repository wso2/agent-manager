"""Validation utilities for OpenTelemetry tracing configuration."""


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
        "openchoreo.dev/project-uid",
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
