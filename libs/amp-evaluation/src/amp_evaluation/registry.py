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
Evaluator decorators and discovery utilities.

@evaluator wraps a function into a FunctionEvaluator instance. No global state.
@llm_judge wraps a prompt-building function into a FunctionLLMJudge instance.
discover_evaluators() scans a module for all BaseEvaluator instances.
"""

from typing import Optional, List
import types
import inspect
import logging

from .evaluators.base import (
    BaseEvaluator,
    FunctionEvaluator,
    FunctionLLMJudge,
    LLMAsJudgeEvaluator,
    validate_unique_evaluator_names,
)


logger = logging.getLogger(__name__)


def evaluator(
    name: str,
    description: Optional[str] = None,
    tags: Optional[List[str]] = None,
    version: Optional[str] = None,
    aggregations: Optional[List] = None,
):
    """
    Decorator to wrap a function as a FunctionEvaluator instance.

    No global registry. Returns the evaluator instance directly.

    The function's first parameter type hint determines the evaluation level:
      - trace: Trace           -> trace-level
      - agent_trace: AgentTrace -> agent-level
      - llm_span: LLMSpan      -> llm-level

    Config params use Param() as default values:
      @evaluator("latency-check")
      def latency_check(
          trace: Trace,
          max_latency_ms: float = Param(default=5000, description="Max latency"),
      ) -> EvalResult:
          ...

    Args:
        name: Unique evaluator name
        description: Human-readable description
        tags: Tags for categorization
        version: Evaluator version
        aggregations: List of aggregations to compute

    Returns:
        Decorator that wraps the function into a FunctionEvaluator

    Example:
        @evaluator("answer-length", description="Check answer length")
        def check_length(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0 if len(trace.output) > 50 else 0.5)

        # check_length is now a FunctionEvaluator instance
        result = check_length.run(trace)

        # Create configured copy
        strict = check_length.with_config(min_length=100)
    """

    def decorator(func):
        func_eval = FunctionEvaluator(func, name=name)
        if description is not None:
            func_eval.description = description
        if tags is not None:
            func_eval.tags = tags
        if version is not None:
            func_eval.version = version
        if aggregations is not None:
            func_eval.aggregations = aggregations
        return func_eval

    return decorator


def llm_judge(func=None, *, name=None, **kwargs):
    """
    Decorator to create an LLM-as-judge evaluator from a prompt-building function.

    The function receives typed objects (Trace/AgentTrace/LLMSpan + optional Task)
    and returns a prompt string. The framework handles LLM calling, output
    validation, and retry.

    Usage:
        @llm_judge
        def quality_judge(trace: Trace, task: Task = None) -> str:
            return f"Evaluate: {trace.input} -> {trace.output}"

        @llm_judge(model="gpt-4o", criteria="accuracy")
        def grounding_judge(trace: Trace) -> str:
            tools = trace.get_tool_calls()
            return f"Is this grounded? {trace.output}\\nTools: {tools}"

    Args:
        func: The prompt-building function (when used without parens)
        name: Evaluator name (defaults to function name)
        **kwargs: LLM config (model, criteria, temperature, max_tokens, threshold, max_retries)

    Returns:
        FunctionLLMJudge instance
    """

    def decorator(fn):
        judge = FunctionLLMJudge(fn, name=name or fn.__name__, **kwargs)
        return judge

    if func is not None:
        return decorator(func)
    return decorator


def discover_evaluators(module: types.ModuleType) -> List[BaseEvaluator]:
    """
    Scan a module for all BaseEvaluator instances.

    Finds:
    - FunctionEvaluator instances (created by @evaluator decorator)
    - FunctionLLMJudge instances (created by @llm_judge decorator)
    - BaseEvaluator subclass instances (module-level instances)
    - BaseEvaluator subclasses (auto-instantiated with no args)

    Args:
        module: Python module to scan

    Returns:
        List of BaseEvaluator instances found in the module

    Example:
        import my_evaluators
        evaluators = discover_evaluators(my_evaluators)
        monitor = Monitor(evaluators=evaluators)
    """
    # Framework base classes that should never be auto-instantiated
    _framework_classes = (BaseEvaluator, FunctionEvaluator, FunctionLLMJudge, LLMAsJudgeEvaluator)

    found = []
    found_instance_types = set()
    candidate_classes = []

    # Pass 1: collect existing instances and candidate classes
    for attr_name in dir(module):
        obj = getattr(module, attr_name, None)
        if isinstance(obj, BaseEvaluator):
            found.append(obj)
            found_instance_types.add(type(obj))
        elif (
            isinstance(obj, type)
            and issubclass(obj, BaseEvaluator)
            and obj not in _framework_classes
            and not inspect.isabstract(obj)
        ):
            candidate_classes.append(obj)

    # Pass 2: auto-instantiate classes that don't already have an instance
    for cls in candidate_classes:
        if cls not in found_instance_types:
            try:
                logger.debug("Auto-instantiating evaluator class: %s", cls.__name__)
                found.append(cls())
            except Exception:
                logger.warning("Failed to auto-instantiate evaluator class: %s", cls.__name__, exc_info=True)

    validate_unique_evaluator_names(found)

    return found
