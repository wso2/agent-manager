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
Parameter descriptor and evaluation level enums for evaluators.

Provides declarative parameter definition with type validation, defaults, and constraints,
plus typed enums for evaluation levels and eval modes.
"""

import enum as _enum
import typing
from typing import Any, Dict, Optional, List


# ============================================================================
# EVALUATION LEVEL ENUMS
# ============================================================================


class EvaluationLevel(str, _enum.Enum):
    """
    Supported evaluation levels for evaluators.

    Inherits from str so enum values are string-compatible:
        EvaluationLevel.TRACE == "trace"  # True

    The evaluation level is auto-detected from the evaluate() method's
    first parameter type hint:
        def evaluate(self, trace: Trace) -> EvalResult:        # trace level
        def evaluate(self, agent_trace: AgentTrace) -> EvalResult:  # agent level
        def evaluate(self, llm_span: LLMSpan) -> EvalResult:   # llm level
    """

    TRACE = "trace"
    AGENT = "agent"
    LLM = "llm"


class EvalMode(str, _enum.Enum):
    """
    Supported evaluation modes for evaluators.

    Determines in which type of evaluation run an evaluator can participate:
    - EXPERIMENT: Dataset-based benchmarking with ground truth (task is available)
    - MONITOR: Production traffic monitoring without ground truth (task is None)

    Inherits from str so enum values are string-compatible:
        EvalMode.EXPERIMENT == "experiment"  # True

    Usage:
        # Eval mode is auto-detected from the evaluate() method signature:
        def evaluate(self, trace: Trace) -> EvalResult:              # both modes (no task needed)
        def evaluate(self, trace: Trace, task: Task) -> EvalResult:  # experiment only (task required)
        def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:  # both modes
    """

    EXPERIMENT = "experiment"
    MONITOR = "monitor"


# ============================================================================
# PARAM DESCRIPTOR
# ============================================================================

# Sentinel value to distinguish "no default" from "default is None"
_NO_DEFAULT = object()


class Param:
    """
    Descriptor for evaluator parameters.

    Provides:
      - Type validation (type inferred from class annotation or function hint)
      - Default values
      - Rich metadata (description, constraints)
      - Runtime introspection for schema generation

    Usage (class-based):
        class MyEvaluator(BaseEvaluator):
            threshold: float = Param(default=0.7, description="Min score to pass")
            model: str = Param(default="gpt-4o-mini", description="LLM model")

            def evaluate(self, trace: Trace) -> EvalResult:
                print(self.threshold)  # 0.7 or whatever was passed

    Usage (function-based):
        @evaluator("my-eval")
        def my_eval(
            trace: Trace,
            threshold: float = Param(default=0.7, description="Pass threshold"),
        ) -> EvalResult:
            ...
    """

    def __init__(
        self,
        default: Any = _NO_DEFAULT,
        description: str = "",
        required: bool = False,
        min: Optional[float] = None,
        max: Optional[float] = None,
        enum: Optional[List[str]] = None,
    ):
        self.type: Optional[type] = None  # Inferred from annotation
        self.default = default
        self.description = description
        self.min = min
        self.max = max
        self.enum = enum

        # Descriptor internals
        self._attr_name: Optional[str] = None

        # Determine if required based on whether a default was provided
        if default is not _NO_DEFAULT:
            self.required = required
        else:
            self.required = True if not required else required

    def __set_name__(self, owner: type, name: str):
        """Called when the descriptor is assigned to a class attribute."""
        self._attr_name = name
        # Infer type from class annotation
        annotations = typing.get_type_hints(owner) if hasattr(owner, "__annotations__") else {}
        if name in annotations:
            self.type = annotations[name]

    def __get__(self, obj, objtype=None):
        """Get the param value from the instance, or the descriptor from the class."""
        if obj is None:
            return self
        if self._attr_name in obj.__dict__:
            return obj.__dict__[self._attr_name]
        return None if self.default is _NO_DEFAULT else self.default

    def __set__(self, obj, value):
        """Set and validate the param value. All values go through _validate."""
        if self.type is not None:
            value = self._validate(value)
        obj.__dict__[self._attr_name] = value

    def _is_optional_type(self) -> bool:
        """Check if the declared type allows None (e.g., Optional[T])."""
        origin = typing.get_origin(self.type)
        if origin is typing.Union:
            return type(None) in typing.get_args(self.type)
        return False

    def _validate(self, value):
        """Validate a param value against constraints. Returns the coerced value."""
        if self.type is None:
            return value

        # None handling: allowed only for Optional types
        if value is None:
            if self._is_optional_type():
                return value
            if isinstance(self.type, type):
                raise TypeError(f"Param '{self._attr_name}' expects {self.type.__name__}, got None")
            return value

        # Coerce str → Enum when type is an Enum subclass
        if isinstance(self.type, type) and issubclass(self.type, _enum.Enum):
            if not isinstance(value, self.type):
                try:
                    value = self.type(value)
                except ValueError:
                    valid = [e.value for e in self.type]
                    raise ValueError(f"Param '{self._attr_name}' must be one of {valid}, got '{value}'")
            return value

        # Type coercion for common cases
        if self.type is set and isinstance(value, (list, tuple)):
            value = set(value)
        elif self.type is list and isinstance(value, (set, tuple)):
            value = list(value)

        # Type check
        if isinstance(self.type, type) and not isinstance(value, self.type):
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

        # Enum check (for non-Enum-type params with allowed values list)
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

        schema: Dict[str, Any] = {
            "key": self._attr_name,
            "required": self.required,
            "description": self.description,
        }

        # Determine type string and enum_values
        if self.type is not None and isinstance(self.type, type) and issubclass(self.type, _enum.Enum):
            schema["type"] = "string"
            schema["enum_values"] = [e.value for e in self.type]
        elif self.type is not None:
            schema["type"] = type_map.get(self.type, "string")
            if self.enum is not None:
                schema["enum_values"] = self.enum
        else:
            schema["type"] = "string"
            if self.enum is not None:
                schema["enum_values"] = self.enum

        # Only include default if one was explicitly provided
        if self.default is not _NO_DEFAULT:
            default_val = self.default.value if isinstance(self.default, _enum.Enum) else self.default
            schema["default"] = default_val
        if self.min is not None:
            schema["min"] = self.min
        if self.max is not None:
            schema["max"] = self.max

        return schema
