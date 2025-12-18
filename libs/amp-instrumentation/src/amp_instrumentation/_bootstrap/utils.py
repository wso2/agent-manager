"""
Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).

WSO2 LLC. licenses this file to you under the Apache License,
Version 2.0 (the "License"); you may not use this file except
in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
"""


def transform_resource_attributes(resource_attributes):
    """
    Validate that required resource attributes are present and prepend openchoreo.dev/ to each key.

    Args:
        resource_attributes: Comma-separated key=value pairs

    Returns:
        dict: Dictionary with openchoreo.dev/ prepended to each key

    Raises:
        ValueError: If resource_attributes is empty or missing required attributes
    """
    if not resource_attributes:
        raise ValueError("AMP_TRACE_ATTRIBUTES is required but not set")

    # Define required attributes
    required_attrs = ["project-uid", "environment-uid", "component-uid"]

    # Parse resource attributes into a dictionary
    attrs_dict = {}
    for attr in resource_attributes.split(","):
        if "=" in attr:
            key, value = attr.split("=", 1)
            attrs_dict[key.strip()] = value.strip()

    # Check for missing attributes
    missing_attrs = [attr for attr in required_attrs if attr not in attrs_dict]
    if missing_attrs:
        raise ValueError(
            f"Missing required resource attributes in AMP_TRACE_ATTRIBUTES: {', '.join(missing_attrs)}. "
            f"Expected format: organization=value,project=value,environment=value"
        )

    # Prepend openchoreo.dev/ to each key
    transformed_attrs = {
        f"openchoreo.dev/{key}": value for key, value in attrs_dict.items()
    }

    return transformed_attrs
