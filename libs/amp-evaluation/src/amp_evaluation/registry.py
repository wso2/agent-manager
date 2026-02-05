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
Registry system for evaluators with validation and metadata tracking.
"""

from typing import Dict, Union, Callable, Optional, Type, List
from functools import wraps
import inspect
import logging

from .evaluators.base import BaseEvaluator, FunctionEvaluator
from .models import EvalResult, Observation, Task


logger = logging.getLogger(__name__)


class EvaluatorRegistry:
    """
    Registry for managing evaluators.

    Supports:
    - Class-based and function-based evaluators
    - Metadata tracking (description, tags, type)
    - Validation of evaluator signatures
    - Global and instance registries
    """

    def __init__(self):
        self._evaluators: Dict[str, Union[Type[BaseEvaluator], BaseEvaluator]] = {}
        self._metadata: Dict[str, dict] = {}

    def register_evaluator(
        self, name: str, evaluator: Union[Type[BaseEvaluator], BaseEvaluator, Callable], metadata: Optional[dict] = None
    ):
        """
        Register an evaluator.

        Args:
            name: Unique evaluator name
            evaluator: Evaluator class, instance, or function
            metadata: Optional metadata (description, tags, etc.)
        """
        if name in self._evaluators:
            logger.warning(f"Overwriting existing evaluator '{name}'")

        self._evaluators[name] = evaluator
        self._metadata[name] = metadata or {}

    def get(self, name: str) -> BaseEvaluator:
        """
        Get an evaluator by name.

        Args:
            name: Evaluator name

        Returns:
            Evaluator instance

        Raises:
            ValueError: If evaluator not found
        """
        if name not in self._evaluators:
            available = list(self._evaluators.keys())
            raise ValueError(f"Evaluator '{name}' not found.\nAvailable evaluators: {available}")

        evaluator = self._evaluators[name]
        metadata = self._metadata.get(name, {})

        # If it's a class, instantiate it
        if isinstance(evaluator, type):
            instance = evaluator()
            instance.name = name
            # Set aggregations if defined in registration
            if hasattr(evaluator, "_registered_aggregations") and evaluator._registered_aggregations:
                instance.aggregations = evaluator._registered_aggregations
            elif metadata.get("aggregations"):
                instance.aggregations = metadata["aggregations"]
            return instance

        # For instances, ensure aggregations are set from metadata if not already
        if evaluator.aggregations is None and metadata.get("aggregations"):
            evaluator.aggregations = metadata["aggregations"]

        return evaluator

    def list_evaluators(self) -> List[str]:
        """List all registered evaluator names."""
        return list(self._evaluators.keys())

    def get_metadata(self, name: str) -> dict:
        """Get metadata for an evaluator."""
        return self._metadata.get(name, {})

    def list_by_tag(self, tag: str) -> List[str]:
        """List evaluators by tag."""
        return [name for name, meta in self._metadata.items() if tag in meta.get("tags", [])]

    def list_by_type(self, evaluator_type: str) -> List[str]:
        """List evaluators by type (trace, trajectory, outcome, etc.)."""
        return [name for name, meta in self._metadata.items() if meta.get("evaluator_type") == evaluator_type]

    def register(
        self,
        name: str,
        evaluator_type: str = "trace",
        description: Optional[str] = None,
        tags: Optional[List[str]] = None,
        version: Optional[str] = None,
        aggregations: Optional[List] = None,
    ):
        """
        Decorator to register an evaluator.

        Args:
            name: Unique evaluator name
            evaluator_type: Type of evaluator ("trace", "trajectory", "outcome", "trial", "task")
            description: Human-readable description
            tags: Tags for categorization (e.g., ["quality", "rag"])
            version: Evaluator version
            aggregations: List of aggregations to compute for this evaluator.
                          Accepts AggregationType, Aggregation objects, or custom callables.

        Examples:
            # Function-based evaluator with aggregations
            from amp_eval import AggregationType, Aggregation

            @register(
                "answer-length",
                description="Check minimum answer length",
                aggregations=[
                    AggregationType.MEAN,
                    AggregationType.MEDIAN,
                    Aggregation(AggregationType.PASS_RATE, threshold=0.7)
                ]
            )
            def check_length(trace: Trace) -> float:
                return 1.0 if len(trace.output) > 50 else 0.5

            # Class-based evaluator
            @register(
                "hallucination-check",
                tags=["quality", "rag"],
                aggregations=[AggregationType.MEAN, AggregationType.P95]
            )
            class HallucinationDetector(TraceEvaluator):
                def evaluate_trace(self, trace):
                    # ... logic ...
                    return EvalResult(...)
        """

        def decorator(evaluator_or_func):
            metadata = {
                "evaluator_type": evaluator_type,
                "description": description or "",
                "tags": tags or [],
                "version": version or "1.0",
                "aggregations": aggregations,
            }

            # If it's a class
            if isinstance(evaluator_or_func, type):
                if not issubclass(evaluator_or_func, BaseEvaluator):
                    raise TypeError(
                        f"Class '{evaluator_or_func.__name__}' must inherit from BaseEvaluator "
                        f"or one of its subclasses.\n"
                        f"Valid base classes: TraceEvaluator, TrajectoryEvaluator, "
                        f"OutcomeEvaluator, TrialEvaluator, TaskEvaluator\n"
                        f"Example:\n"
                        f"  @register('{name}')\n"
                        f"  class {evaluator_or_func.__name__}(TraceEvaluator):\n"
                        f"      def evaluate_trace(self, trace: Trace) -> EvalResult:\n"
                        f"          ..."
                    )

                # Store aggregations on the class for later access
                evaluator_or_func._registered_aggregations = aggregations

                self.register_evaluator(name, evaluator_or_func, metadata)
                return evaluator_or_func

            # If it's a function, validate and wrap
            _validate_evaluator_function(evaluator_or_func, name)

            @wraps(evaluator_or_func)
            def wrapper(observation: "Observation", task: Optional["Task"] = None) -> EvalResult:
                result = evaluator_or_func(observation, task)
                return _normalize_result(result)

            # Wrap in FunctionEvaluator
            func_eval = FunctionEvaluator(wrapper, name=name)
            if version:
                func_eval.version = version
            if aggregations:
                func_eval.aggregations = aggregations

            self.register_evaluator(name, func_eval, metadata)
            return wrapper

        return decorator


# Global registry instance
_global_registry = EvaluatorRegistry()


def _validate_evaluator_function(func: Callable, name: str) -> None:
    """
    Validate that a function has the correct signature.

    Expected: (target) -> EvalResult | dict | float
    Where target can be Trace, Trajectory, Outcome, Trial, or Task
    """
    sig = inspect.signature(func)
    params = list(sig.parameters.values())

    # Check parameter count
    if len(params) == 0:
        raise TypeError(
            f"Evaluator '{name}' must accept at least one parameter.\n"
            f"Example: def {func.__name__}(trace: Trace) -> EvalResult:"
        )

    if len(params) > 2:  # Allow optional second param for context
        raise TypeError(
            f"Evaluator '{name}' accepts {len(params)} parameters, but should accept 1-2.\n"
            f"Found parameters: {[p.name for p in params]}\n"
            f"Expected: def {func.__name__}(trace: Trace) -> EvalResult:"
        )


def _normalize_result(result) -> EvalResult:
    """
    Normalize different result types to EvalResult.

    Supports:
    - EvalResult (passthrough)
    - dict with 'score' field (create EvalResult)
    - float/int (create EvalResult)
    """
    if isinstance(result, EvalResult):
        return result

    elif isinstance(result, dict):
        if "score" not in result:
            raise ValueError(
                f"Evaluator returned dict without 'score' field.\n"
                f"Expected: {{'score': 0.95, 'explanation': '...'}}\n"
                f"Got: {result}"
            )

        return EvalResult(
            score=result.get("score", 0.0),
            passed=result.get("passed"),
            explanation=result.get("explanation", ""),
            details=result.get("details"),
        )

    elif isinstance(result, (int, float)):
        score = float(result)
        return EvalResult(score=score, passed=score >= 0.7, explanation="", details=None)

    else:
        raise TypeError(
            f"Evaluator returned invalid type {type(result).__name__}.\n"
            f"Expected: EvalResult | dict | float\n"
            f"Got: {result}\n"
            f"\nValid return examples:\n"
            f"  return EvalResult(...)\n"
            f"  return {{'score': 0.95, 'explanation': '...'}}\n"
            f"  return 0.95"
        )


# ============================================================================
# GLOBAL REGISTRY FUNCTIONS
# ============================================================================


def evaluator(
    name: str,
    evaluator_type: str = "trace",
    description: Optional[str] = None,
    tags: Optional[List[str]] = None,
    version: Optional[str] = None,
    aggregations: Optional[List] = None,
):
    """
    Decorator to register an evaluator to the global registry.

    See EvaluatorRegistry.register for details and examples.

    Args:
        name: Unique evaluator name
        evaluator_type: Type of evaluator
        description: Human-readable description
        tags: Tags for categorization
        version: Evaluator version
        aggregations: List of aggregations (AggregationType, Aggregation, or callable)
    """
    return _global_registry.register(name, evaluator_type, description, tags, version, aggregations)


def get_evaluator(name: str) -> BaseEvaluator:
    """Get an evaluator by name from the global registry."""
    return _global_registry.get(name)


def list_evaluators() -> List[str]:
    """List all registered evaluators."""
    return _global_registry.list_evaluators()


def get_evaluator_metadata(name: str) -> dict:
    """Get metadata for an evaluator."""
    return _global_registry.get_metadata(name)


def list_by_tag(tag: str) -> List[str]:
    """List evaluators by tag."""
    return _global_registry.list_by_tag(tag)


def list_by_type(evaluator_type: str) -> List[str]:
    """List evaluators by type."""
    return _global_registry.list_by_type(evaluator_type)


# Export the global registry for advanced use cases
def get_registry():
    return _global_registry
