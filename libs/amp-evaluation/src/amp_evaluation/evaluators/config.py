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
Parameter descriptor for evaluators.

Provides declarative parameter definition with type validation, defaults, and constraints.
"""

from typing import Any, Optional, List


# Sentinel value to distinguish "no default" from "default is None"
_NO_DEFAULT = object()


class Param:
    """
    Descriptor for evaluator parameters.

    Provides:
      - Type validation
      - Default values
      - Rich metadata (description, constraints)
      - Runtime introspection for schema generation

    Usage:
        class MyEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, description="Min score to pass")
            model = Param(str, default="gpt-4o-mini", description="LLM model")

            def evaluate(self, observation):
                print(self.threshold)  # 0.7 or whatever was passed
    """

    def __init__(
        self,
        type: type,
        default: Any = _NO_DEFAULT,
        description: str = "",
        required: bool = False,
        min: Optional[float] = None,
        max: Optional[float] = None,
        enum: Optional[List[str]] = None,
    ):
        self.type = type
        self.default = default
        self.description = description
        self.min = min
        self.max = max
        self.enum = enum

        # Descriptor internals
        self._attr_name = None

        # Determine if required based on whether a default was provided
        if default is not _NO_DEFAULT:
            # Has an explicit default (even if None) - not required unless explicitly set
            self.required = required
        else:
            # No default provided - required unless explicitly set to False
            self.required = True if not required else required

    def __set_name__(self, owner, name):
        """Called when the descriptor is assigned to a class attribute."""
        self._attr_name = name

    def __get__(self, obj, objtype=None):
        """Get the param value from the instance, or the descriptor from the class."""
        if obj is None:
            # Class-level access — return the descriptor itself
            # This allows introspection: MyEvaluator.threshold.description
            return self
        # Return the value if set, otherwise the default (even if None)
        if self._attr_name in obj.__dict__:
            return obj.__dict__[self._attr_name]
        return None if self.default is _NO_DEFAULT else self.default

    def __set__(self, obj, value):
        """Set and validate the param value."""
        if value is not None:
            value = self._validate(value)
        obj.__dict__[self._attr_name] = value

    def _validate(self, value):
        """Validate a param value against constraints. Returns the coerced value."""
        # Type coercion for common cases
        if self.type is set and isinstance(value, (list, tuple)):
            value = set(value)
        elif self.type is list and isinstance(value, (set, tuple)):
            value = list(value)

        # Type check
        if not isinstance(value, self.type):
            # Allow int for float
            if self.type is float and isinstance(value, int):
                value = float(value)
            else:
                raise TypeError(f"Param '{self._attr_name}' expects {self.type.__name__}, got {type(value).__name__}")

        # Range check
        if self.min is not None and value < self.min:
            raise ValueError(f"Param '{self._attr_name}' must be >= {self.min}, got {value}")
        if self.max is not None and value > self.max:
            raise ValueError(f"Param '{self._attr_name}' must be <= {self.max}, got {value}")

        # Enum check
        if self.enum is not None and value not in self.enum:
            raise ValueError(f"Param '{self._attr_name}' must be one of {self.enum}, got {value}")

        return value

    def to_schema(self) -> dict:
        """Convert to schema dictionary for API responses."""
        type_map = {
            str: "string",
            int: "integer",
            float: "float",
            bool: "boolean",
            list: "array",
            dict: "object",
            set: "array",
        }

        schema = {
            "key": self._attr_name,
            "type": type_map.get(self.type, "string"),
            "required": self.required,
            "description": self.description,
        }

        # Only include default if one was explicitly provided
        if self.default is not _NO_DEFAULT:
            schema["default"] = self.default
        if self.min is not None:
            schema["min"] = self.min
        if self.max is not None:
            schema["max"] = self.max
        if self.enum is not None:
            schema["enum_values"] = self.enum

        return schema


# Backward compatibility alias
Config = Param
