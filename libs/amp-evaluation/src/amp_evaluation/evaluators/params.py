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


class _ParamDescriptor:
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
            self.required = True  # No default → always required

    def __set_name__(self, owner: type, name: str):
        """Called when the descriptor is assigned to a class attribute."""
        self._attr_name = name
        # Infer type from class annotation
        try:
            annotations = typing.get_type_hints(owner) if hasattr(owner, "__annotations__") else {}
        except Exception:
            annotations = getattr(owner, "__annotations__", {})
        if name in annotations:
            self.type = annotations[name]

    def __get__(self, obj: Any, objtype: Any = None) -> Any:
        """Get the param value from the instance, or the descriptor from the class."""
        if obj is None:
            return self
        if self._attr_name in obj.__dict__:
            return obj.__dict__[self._attr_name]
        if self.default is _NO_DEFAULT:
            raise AttributeError(f"Required parameter '{self._attr_name}' has not been set on {type(obj).__name__}")
        # Copy mutable defaults so instances don't share state
        if isinstance(self.default, (list, dict, set)):
            copy = type(self.default)(self.default)  # type: ignore[arg-type]
            obj.__dict__[self._attr_name] = copy
            return copy
        return self.default

    def __set__(self, obj: Any, value: Any) -> None:
        """Set and validate the param value. All values go through _validate."""
        if self.type is not None:
            value = self._validate(value)
        obj.__dict__[self._attr_name] = value

    def _is_optional_type(self) -> bool:
        """Check if the declared type allows None (e.g., Optional[T] or T | None)."""
        origin = typing.get_origin(self.type)
        if origin is typing.Union:
            return type(None) in typing.get_args(self.type)
        # Python 3.10+: X | None produces types.UnionType
        import types as _types

        if isinstance(self.type, _types.UnionType):
            return type(None) in self.type.__args__
        return False

    def _resolve_base_type(self):
        """
        Resolve the concrete base type, unwrapping Optional and generic aliases.

        Returns the original type unchanged for multi-arg unions (e.g., int | str | None)
        since those can't be reduced to a single base type.

        Examples:
            Optional[List[str]] → list
            Optional[Set[str]]  → set
            List[str]           → list
            Optional[str]       → str
            int                 → int
            int | str | None    → int | str | None  (unchanged)
        """
        tp = self.type
        if tp is None:
            return None

        # Unwrap Optional[X] (Union[X, None]) → X (only when exactly one inner type)
        origin = typing.get_origin(tp)
        if origin is typing.Union:
            args = [a for a in typing.get_args(tp) if a is not type(None)]
            if len(args) == 1:
                tp = args[0]
            else:
                return tp  # Multi-arg union, can't resolve to single base type

        # Python 3.10+: X | None produces types.UnionType
        import types as _types

        if isinstance(tp, _types.UnionType):
            args = [a for a in tp.__args__ if a is not type(None)]
            if len(args) == 1:
                tp = args[0]
            else:
                return tp  # Multi-arg union, can't resolve to single base type

        # Resolve generic alias origin: List[str] → list, Set[str] → set
        base = typing.get_origin(tp)
        if base is not None:
            return base

        return tp

    def _validate(self, value):
        """Validate a param value against constraints. Returns the coerced value."""
        if self.type is None:
            return value

        # None handling: allowed only for Optional types
        if value is None:
            if self._is_optional_type():
                return value
            type_name = getattr(self.type, "__name__", str(self.type))
            raise TypeError(f"Param '{self._attr_name}' expects {type_name}, got None")

        # Resolve the base type (unwrap Optional, resolve generics)
        base_type = self._resolve_base_type()

        # Coerce str → Enum when type is an Enum subclass
        if isinstance(base_type, type) and issubclass(base_type, _enum.Enum):
            if not isinstance(value, base_type):
                try:
                    value = base_type(value)
                except ValueError:
                    valid = [e.value for e in base_type]
                    raise ValueError(f"Param '{self._attr_name}' must be one of {valid}, got '{value}'")
            return value

        # Type coercion for common cases
        if base_type is set and isinstance(value, (list, tuple)):
            value = set(value)
        elif base_type is list and isinstance(value, (set, tuple)):
            value = list(value)

        # Type check
        if isinstance(base_type, type) and not isinstance(value, base_type):
            # Allow int for float
            if base_type is float and isinstance(value, int):
                value = float(value)
            else:
                type_name = getattr(base_type, "__name__", str(base_type))
                raise TypeError(f"Param '{self._attr_name}' expects {type_name}, got {type(value).__name__}")

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
        base_type = self._resolve_base_type()
        if base_type is not None and isinstance(base_type, type) and issubclass(base_type, _enum.Enum):
            schema["type"] = "string"
            schema["enum_values"] = [e.value for e in base_type]
        elif base_type is not None:
            schema["type"] = type_map.get(base_type, "string")
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


def Param(
    default: Any = _NO_DEFAULT,
    description: str = "",
    required: bool = False,
    min: Optional[float] = None,
    max: Optional[float] = None,
    enum: Optional[List[str]] = None,
) -> Any:
    """
    Create a parameter descriptor for evaluator configuration.

    Returns a descriptor that provides type validation, defaults, and metadata.

    Usage (class-based)::

        class MyEvaluator(BaseEvaluator):
            threshold: float = Param(default=0.7, description="Min score to pass")

    Usage (function-based)::

        @evaluator("my-eval")
        def my_eval(
            trace: Trace,
            threshold: float = Param(default=0.7, description="Pass threshold"),
        ) -> EvalResult:
            ...
    """
    return _ParamDescriptor(
        default=default,
        description=description,
        required=required,
        min=min,
        max=max,
        enum=enum,
    )
