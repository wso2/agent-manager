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

from typing import Dict, Callable, Optional, List
from functools import wraps
import inspect
import logging

from .evaluators.base import BaseEvaluator, FunctionEvaluator
from .models import EvalResult
from .trace.models import Trace
from .dataset.schema import Task


logger = logging.getLogger(__name__)


class EvaluatorRegistry:
    """
    Registry for managing evaluators.

    Supports:
    - Class-based and function-based evaluators
    - Metadata tracking (description, tags, type)
    - Validation of evaluator signatures
    - Global and instance registries
    - Lazy loading of built-in evaluators

    Built-in Evaluators:
        Built-in evaluators (like "deepeval/plan-quality") are NOT automatically
        registered. They are loaded on-demand when requested via get() or
        explicitly registered via register_builtin().
    """

    def __init__(self):
        self._evaluators: Dict[str, BaseEvaluator] = {}

    def register_evaluator(self, evaluator: BaseEvaluator):
        """
        Register an evaluator instance.

        Args:
            evaluator: Evaluator instance (name is taken from evaluator.name)
        """
        name = evaluator.name
        if name in self._evaluators:
            logger.warning(f"Overwriting existing evaluator '{name}'")

        self._evaluators[name] = evaluator

    def register_builtin(self, name: str, display_name: Optional[str] = None, **kwargs) -> None:
        """
        Register a built-in evaluator by name with optional configuration.

        This makes the evaluator available in list_evaluators() and enables
        tag-based filtering. The evaluator is loaded using convention-based discovery.

        Args:
            name: Built-in evaluator identifier (e.g., "deepeval/plan-quality")
            display_name: User-facing name to register under; defaults to the evaluator's own name
            **kwargs: Configuration parameters to store with the evaluator.
                     These will be used as defaults when get() is called.

        Raises:
            ValueError: If the evaluator is not found
            ImportError: If the evaluator's dependencies are not installed
        """
        from .evaluators.builtin import get_builtin_evaluator

        # Get and store the configured evaluator instance
        instance = get_builtin_evaluator(name, **kwargs)
        if display_name:
            instance.name = display_name
        self.register_evaluator(instance)

    def get(self, name: str) -> BaseEvaluator:
        """
        Get an evaluator by name from the registry.

        Args:
            name: Evaluator name

        Returns:
            Evaluator instance

        Raises:
            ValueError: If evaluator not found

        Examples:
            >>> # Get registered evaluator
            >>> evaluator = registry.get("latency")
        """
        # Check if registered
        if name in self._evaluators:
            return self._evaluators[name]

        # Not found
        available = list(self._evaluators.keys())
        raise ValueError(f"Evaluator '{name}' not found.\nRegistered evaluators: {available}\n")

    def list_evaluators(self) -> List[str]:
        """List all registered evaluator names (does not include unregistered built-ins)."""
        return list(self._evaluators.keys())

    def get_metadata(self, name: str) -> dict:
        """
        Get metadata for an evaluator.

        For registered evaluators, gets metadata from the instance.
        Raises ValueError if evaluator not found.
        """
        # Check if registered
        if name in self._evaluators:
            return self._evaluators[name].get_metadata()

        # Not found
        available = list(self._evaluators.keys())
        raise ValueError(f"Evaluator '{name}' not found.\nRegistered evaluators: {available}\n")

    def list_by_tag(self, tag: str) -> List[str]:
        """List registered evaluators by tag."""
        result = []
        for name in self._evaluators.keys():
            metadata = self.get_metadata(name)
            if tag in metadata.get("tags", []):
                result.append(name)
        return result

    def register(
        self,
        name: str,
        description: Optional[str] = None,
        tags: Optional[List[str]] = None,
        version: Optional[str] = None,
        aggregations: Optional[List] = None,
        level: Optional[str] = None,
    ):
        """
        Decorator to register an evaluator instance to the registry.

        Args:
            name: Unique evaluator name
            description: Human-readable description
            tags: Tags for categorization (e.g., ["quality", "rag"])
            version: Evaluator version
            aggregations: List of aggregations to compute for this evaluator.
                          Accepts AggregationType, Aggregation objects, or custom callables.

        Examples:
            # Function-based evaluator with aggregations
            from amp_evaluation import evaluator, AggregationType, Aggregation

            @evaluator(
                "answer-length",
                description="Check minimum answer length",
                aggregations=[
                    AggregationType.MEAN,
                    AggregationType.MEDIAN,
                    Aggregation(AggregationType.PASS_RATE, threshold=0.7)
                ]
            )
            def check_length(trace, task=None) -> EvalResult:
                return EvalResult(score=1.0 if len(trace.output) > 50 else 0.5)

            # Class-based evaluator
            @evaluator(
                "hallucination-check",
                tags=["quality", "rag"],
                aggregations=[AggregationType.MEAN, AggregationType.P95]
            )
            class HallucinationDetector(BaseEvaluator):
                def evaluate(self, trace, task=None) -> EvalResult:
                    # ... logic ...
                    return EvalResult(...)
        """

        def decorator(evaluator_or_func):
            # If it's a class
            if isinstance(evaluator_or_func, type):
                if not issubclass(evaluator_or_func, BaseEvaluator):
                    raise TypeError(
                        f"Class '{evaluator_or_func.__name__}' must inherit from BaseEvaluator.\n"
                        f"Example:\n"
                        f"  @register('{name}')\n"
                        f"  class {evaluator_or_func.__name__}(BaseEvaluator):\n"
                        f"      name = '{name}'\n"
                        f"      description = '...'\n"
                        f"      tags = ['...']\n"
                        f"      \n"
                        f"      def evaluate(self, trace, task=None) -> EvalResult:\n"
                        f"          ..."
                    )

                # Create instance and override metadata if provided via decorator
                instance = evaluator_or_func()
                if name:
                    instance.name = name
                if description:
                    instance.description = description
                if tags:
                    instance.tags = tags
                if version:
                    instance.version = version
                if aggregations:
                    instance.aggregations = aggregations

                self.register_evaluator(instance)
                return evaluator_or_func

            # If it's a function, validate and wrap
            _validate_evaluator_function(evaluator_or_func, name)

            sig = inspect.signature(evaluator_or_func)
            nparams = len(list(sig.parameters.values()))

            @wraps(evaluator_or_func)
            def wrapper(trace: "Trace", task: Optional["Task"] = None) -> EvalResult:
                if nparams == 1:
                    result = evaluator_or_func(trace)
                else:
                    result = evaluator_or_func(trace, task)
                return _normalize_result(result)

            # Build level kwargs for FunctionEvaluator
            func_kwargs = {}
            if level is not None:
                func_kwargs["level"] = level

            # Wrap in FunctionEvaluator and set metadata as instance attributes
            func_eval = FunctionEvaluator(wrapper, name=name, **func_kwargs)
            func_eval.description = description or ""
            func_eval.tags = tags or []
            func_eval.version = version or "1.0"
            if aggregations:
                func_eval.aggregations = aggregations

            self.register_evaluator(func_eval)
            return wrapper

        return decorator


# Global registry instance
_global_registry = EvaluatorRegistry()


def _validate_evaluator_function(func: Callable, name: str) -> None:
    """
    Validate that a function has the correct signature.

    Expected: (target) -> EvalResult | dict | float
    Where target can be Trace, Trace, Outcome, Trial, or Task
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
        return EvalResult(score=score, passed=None, explanation="", details=None)

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
    description: Optional[str] = None,
    tags: Optional[List[str]] = None,
    version: Optional[str] = None,
    aggregations: Optional[List] = None,
    level: Optional[str] = None,
):
    """
    Decorator to register an evaluator to the global registry.

    See EvaluatorRegistry.register for details and examples.

    Args:
        name: Unique evaluator name
        description: Human-readable description
        tags: Tags for categorization
        version: Evaluator version
        aggregations: List of aggregations (AggregationType, Aggregation, or callable)
        level: Evaluation level ("trace", "agent", "span"). Defaults to "trace".
    """
    return _global_registry.register(name, description, tags, version, aggregations, level)


def register_evaluator(evaluator: BaseEvaluator) -> None:
    """
    Register an evaluator instance to the global registry.

    Args:
        evaluator: Evaluator instance (name is taken from evaluator.name)
    """
    _global_registry.register_evaluator(evaluator)


def register_builtin(name: str, display_name: Optional[str] = None, **kwargs) -> None:
    """
    Register a built-in evaluator to the global registry with optional configuration.

    This makes the evaluator available in list_evaluators() and get_evaluator().

    This is useful when you want to:
    - Include built-in evaluators in tag-based filtering
    - Make them visible in list_evaluators()
    - Use them with Experiment.from_evaluators() by name
    - Pre-configure evaluators with custom parameters

    Args:
        name: Built-in evaluator identifier (e.g., "deepeval/plan-quality")
        display_name: User-facing name to register under; defaults to the evaluator's own name
        **kwargs: Configuration parameters for the evaluator.
                 The instance is created and stored with these parameters.

    Raises:
        ValueError: If the evaluator is not a known built-in
        ImportError: If the evaluator's dependencies are not installed

    Example:
        # Register with default configuration
        register_builtin("deepeval/plan-quality")

        # Register with custom configuration
        register_builtin("latency", display_name="API Latency", max_latency_ms=500)
        register_builtin("deepeval/tool-correctness", threshold=0.8, evaluate_input=True)

        # Now they appear in list_evaluators() and can be retrieved
        print(list_evaluators())  # [..., "deepeval/plan-quality", "API Latency", ...]

        evaluator = get_evaluator("API Latency")  # Returns instance with max_latency_ms=500
    """
    _global_registry.register_builtin(name, display_name=display_name, **kwargs)


def list_builtin_evaluators() -> List[str]:
    """
    List all available built-in evaluator names.

    These are evaluators that can be loaded on-demand or registered
    with register_builtin().

    Returns:
        List of built-in evaluator names
    """
    from .evaluators.builtin import list_builtin_evaluators as _list_builtin

    evaluators = _list_builtin()
    return [ev["name"] for ev in evaluators]


def get_evaluator(name: str) -> BaseEvaluator:
    """
    Get an evaluator by name from the global registry.

    For registered evaluators, returns the stored instance.
    To configure built-in evaluators, use register_builtin() first with kwargs.

    Args:
        name: Evaluator name (e.g., "latency", "deepeval/plan-quality")

    Returns:
        Evaluator instance

    Raises:
        ValueError: If evaluator not found

    Examples:
        >>> # Get registered evaluator
        >>> evaluator = get_evaluator("latency")

        >>> # For custom configuration, register first
        >>> register_builtin("latency", max_latency_ms=500)
        >>> evaluator = get_evaluator("latency")

        >>> # Or use direct import for type safety
        >>> from amp_evaluation.evaluators.builtin.standard import LatencyEvaluator
        >>> evaluator = LatencyEvaluator(max_latency_ms=500)
    """
    return _global_registry.get(name)


def list_evaluators() -> List[str]:
    """List all registered evaluators (does not include unregistered built-ins)."""
    return _global_registry.list_evaluators()


def get_evaluator_metadata(name: str) -> dict:
    """Get metadata for an evaluator."""
    return _global_registry.get_metadata(name)


def list_by_tag(tag: str) -> List[str]:
    """List evaluators by tag."""
    return _global_registry.list_by_tag(tag)


# Export the global registry for advanced use cases
def get_registry():
    return _global_registry
