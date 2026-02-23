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
Base evaluator classes and interfaces.

Two-parameter architecture: evaluate(trace, task)
- trace: The agent's execution trace (always available)
- task: What we expected (only for experiments)
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import List, Optional, Callable, TYPE_CHECKING, Any, Dict
import logging
import inspect

from ..models import EvalResult
from .config import Param, EvaluationLevel

if TYPE_CHECKING:
    from ..dataset import Task
    from ..trace.models import Trace


logger = logging.getLogger(__name__)


class BaseEvaluator(ABC):
    """
    Abstract base class for all evaluators.

    Evaluators score specific aspects of agent performance using a two-parameter interface:
    - trace: The agent's execution trace (always available)
    - task: What it should have done (only for experiments with datasets)

    The runner automatically enriches EvalResult into EvaluatorScore
    with metadata (trace ID, timestamp, task ID, trial ID).

    Class Attributes for Metadata:
        name: Unique evaluator name (defaults to class name)
        description: Human-readable description of what the evaluator does
        tags: List of tags for categorization (e.g., ["quality", "rag", "deepeval"])
        version: Evaluator version string
        aggregations: Default aggregations to compute for this evaluator

    Example (Success with score):
        class LatencyEvaluator(BaseEvaluator):
            name = "latency"
            description = "Checks if response latency is within acceptable limits"
            tags = ["performance", "sla"]
            version = "1.0"

            def __init__(self, max_latency_ms: float = 5000):
                super().__init__()
                self.max_latency = max_latency_ms

            def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
                latency = trace.metrics.total_duration_ms
                passed = latency <= self.max_latency
                return EvalResult(
                    score=1.0 if passed else 0.0,
                    explanation=f"Latency: {latency}ms"
                )

    Example (Error when cannot evaluate):
        class ExactMatchEvaluator(BaseEvaluator):
            name = "exact-match"
            description = "Checks if output exactly matches expected output"
            tags = ["accuracy"]

            def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
                if not task or not task.expected_output:
                    return EvalResult.skip("Requires task with expected_output")

                matches = trace.output == task.expected_output
                return EvalResult(
                    score=1.0 if matches else 0.0,
                    explanation=f"Exact match: {matches}"
                )
    """

    # Class-level metadata attributes (can be overridden by subclasses or instances)
    name: str = ""  # Defaults to class name if not set
    description: str = ""
    tags: List[str] = []  # Subclasses should override; __init__ creates a per-instance copy
    version: str = "1.0"

    # Configuration parameters (using Param descriptors)
    level = Param(EvaluationLevel, default=EvaluationLevel.TRACE, description="Evaluation level: trace, agent, or span")

    def __init__(self, **kwargs):
        # Set default name to class name if not already set
        if not self.name:
            self.name = self.__class__.__name__

        # Ensure tags is a mutable list per instance
        if not isinstance(self.tags, list):
            self.tags = list(self.tags)

        self._aggregations: Optional[List] = None

        # Check if class has default aggregations set via decorator
        if hasattr(self.__class__, "_default_aggregations") and self.__class__._default_aggregations:
            self._aggregations = self.__class__._default_aggregations

        # Initialize Param descriptors from kwargs
        self._init_params_from_kwargs(kwargs)

        # Validate that built-in evaluators use Param descriptors properly
        self._validate_param_usage()

        # Auto-detect supported levels from method overrides (always — no class-level declaration allowed)
        self._supported_levels = self._auto_detect_supported_levels()

        # Validate level is supported (runtime validation)
        if self.level not in self._supported_levels:
            raise ValueError(
                f"{self.name} does not support level='{self.level.value}'. "
                f"Supported levels: {', '.join(lvl.value for lvl in self._supported_levels)}"
            )

    def _init_params_from_kwargs(self, kwargs: Dict[str, Any]):
        """
        Initialize Param descriptors from kwargs.

        This allows evaluators to be instantiated with:
            evaluator = MyEvaluator(threshold=0.8, model="gpt-4")

        Even when the evaluator uses Param descriptors instead of __init__ parameters.

        Raises:
            TypeError: If unknown kwargs are passed
        """
        # Find all Param descriptors on the class
        valid_config_names = set()
        for attr_name in dir(type(self)):
            attr = getattr(type(self), attr_name, None)
            if isinstance(attr, Param):
                valid_config_names.add(attr_name)
                # If a value was passed in kwargs, set it
                if attr_name in kwargs:
                    setattr(self, attr_name, kwargs[attr_name])

        # Check for unknown kwargs
        unknown_kwargs = set(kwargs.keys()) - valid_config_names
        if unknown_kwargs:
            raise TypeError(
                f"{self.__class__.__name__}.__init__() got unexpected keyword argument(s): "
                f"{', '.join(sorted(unknown_kwargs))}"
            )

    def _validate_param_usage(self):
        """
        Validate that built-in evaluators use Param descriptors instead of __init__ params.

        This ensures all built-in evaluators follow the declarative Param pattern.
        Only validates evaluators in the amp_evaluation.evaluators.builtin package.

        Raises:
            ValueError: If a built-in evaluator has __init__ parameters that aren't Param descriptors
        """
        # Only validate built-in evaluators
        module_name = self.__class__.__module__
        if not module_name.startswith("amp_evaluation.evaluators.builtin"):
            return  # Skip validation for user-defined evaluators

        # Get the __init__ signature of the concrete class (not BaseEvaluator)
        init_method = self.__class__.__init__
        sig = inspect.signature(init_method)

        # Get all Param descriptors on the class
        param_attrs = set()
        for attr_name in dir(type(self)):
            attr = getattr(type(self), attr_name, None)
            if isinstance(attr, Param):
                param_attrs.add(attr_name)

        # Check for __init__ parameters that aren't Param descriptors
        invalid_params = []
        for param_name, param in sig.parameters.items():
            # Skip 'self' and 'kwargs'
            if param_name in ("self", "kwargs"):
                continue

            # If it's not a Param descriptor, it's invalid
            if param_name not in param_attrs:
                invalid_params.append(param_name)

        if invalid_params:
            raise ValueError(
                f"Built-in evaluator '{self.__class__.__name__}' has __init__ parameters "
                f"{invalid_params} that are not defined as Param descriptors. "
                f"Built-in evaluators must use Param descriptors for all configuration. "
                f"Example:\n"
                f"  class {self.__class__.__name__}(BaseEvaluator):\n"
                f"      {invalid_params[0]} = Param(type, default=..., description='...')\n"
                f"      def __init__(self, **kwargs):\n"
                f"          super().__init__(**kwargs)"
            )

    def _auto_detect_supported_levels(self) -> List[EvaluationLevel]:
        """
        Auto-detect supported evaluation levels from overridden methods.

        Checks which protected methods are implemented by the subclass to
        automatically determine the list of supported levels. Levels cannot be
        declared manually — they are always derived from the evaluator's implementation.

        Detection rules:
        - "trace": Always included (_trace_evaluation is abstract, must be implemented)
        - "agent": Included if _agent_evaluation is overridden in the subclass
        - "span": Included if _span_evaluation is overridden in the subclass

        Returns:
            List of EvaluationLevel values the evaluator supports

        Example:
            class MyEvaluator(BaseEvaluator):
                def _trace_evaluation(self, trace, task): ...  # trace supported
                def _agent_evaluation(self, agent, task): ...  # agent auto-detected
                # No _span_evaluation → span NOT supported
        """
        levels = [EvaluationLevel.TRACE]  # Always supported - _trace_evaluation is abstract

        # Check if _agent_evaluation is overridden in any class in the MRO
        # (excluding BaseEvaluator itself - we check if subclass provides an impl)
        if type(self)._agent_evaluation is not BaseEvaluator._agent_evaluation:
            levels.append(EvaluationLevel.AGENT)

        # Check if _span_evaluation is overridden in any class in the MRO
        if type(self)._span_evaluation is not BaseEvaluator._span_evaluation:
            levels.append(EvaluationLevel.SPAN)

        return levels

    def _extract_config_schema(self) -> List[Dict[str, Any]]:
        """
        Extract configuration schema from Param descriptors.

        Scans the evaluator class for Param descriptors and builds
        a schema describing what parameters this evaluator accepts.
        """
        schema = []

        # Find all Param descriptors on the class
        for attr_name in dir(type(self)):
            attr = getattr(type(self), attr_name, None)
            if isinstance(attr, Param):
                schema.append(attr.to_schema())

        return schema

    @property
    def aggregations(self) -> Optional[List]:
        """Get configured aggregations for this evaluator."""
        return self._aggregations

    @aggregations.setter
    def aggregations(self, value: List):
        """Set aggregations for this evaluator."""
        self._aggregations = value

    def get_metadata(self) -> dict:
        """
        Get evaluator metadata including configuration schema.

        For class-based evaluators, the config schema is derived from
        Param descriptors defined as class attributes.

        Excludes internal fields (name, description, tags, version)
        and fields starting with underscore.

        The level param's enum_values are filtered to only include
        levels actually supported by this evaluator (auto-detected
        from method overrides).
        """
        schema = self._extract_config_schema()

        supported_level_values = [lvl.value for lvl in self._supported_levels]

        # Filter level enum to only supported levels
        for param in schema:
            if param["key"] == "level":
                param["enum_values"] = supported_level_values
                break
        else:  # If no level param defined, add it with supported levels
            schema.append(
                {
                    "key": "level",
                    "type": "enum",
                    "description": "Evaluation level",
                    "enum_values": supported_level_values,
                    "default": self.level.value if self.level else None,
                }
            )

        metadata = {
            "name": self.name,
            "description": getattr(self, "description", ""),
            "tags": list(getattr(self, "tags", [])),
            "version": getattr(self, "version", "1.0"),
            "config_schema": schema,
        }
        return metadata

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> List[EvalResult]:
        """
        Evaluate an agent's performance at the configured level.

        This method handles multi-level dispatching internally based on self.level:
        - "trace": Calls _trace_evaluation() once → 1 result
        - "agent": Calls _agent_evaluation() for each agent → N results
        - "span": Calls _span_evaluation() for each LLM span → M results

        Args:
            trace: The agent's execution trace (always available)
            task: What we expected (ground truth, constraints) - only for experiments

        Returns:
            List of EvalResult objects (one per evaluated item)
        """

        results = []

        if self.level == "trace":
            # Trace-level: evaluate entire trace once
            result = self._trace_evaluation(trace, task)
            results.append(result)

        elif self.level == "agent":
            # Agent-level: evaluate each agent separately as an AgentTrace
            from ..trace.models import AgentTrace as _AgentTrace

            agent_spans = trace.get_agents()

            if not agent_spans:
                # No explicit agents — wrap the full trace as a single AgentTrace
                fallback = _AgentTrace(
                    agent_id=trace.trace_id,
                    input=trace.input,
                    output=trace.output,
                    steps=trace.get_agent_steps(deduplicate_messages=True),
                    metrics=trace.metrics,
                )
                result = self._agent_evaluation(fallback, task)
                # Enrich span_id in details
                if result.details is None:
                    result.details = {}
                result.details["span_id"] = fallback.agent_id
                results.append(result)
            else:
                for agent_span in agent_spans:
                    agent_trace = trace.create_agent_trace(agent_span.span_id)
                    result = self._agent_evaluation(agent_trace, task)
                    # Enrich span_id in details
                    if result.details is None:
                        result.details = {}
                    result.details["span_id"] = agent_trace.agent_id
                    results.append(result)

        elif self.level == "span":
            # Span-level: evaluate each LLM span
            filtered_spans = trace.get_llm_calls(deduplicate_messages=True)

            for span in filtered_spans:
                result = self._span_evaluation(span, task)
                # Enrich span_id in details
                if result.details is None:
                    result.details = {}
                result.details["span_id"] = getattr(span, "span_id", None)
                results.append(result)
        else:
            # Unknown level - should not happen due to validation
            raise ValueError(f"Unknown evaluation level: {self.level}")

        return results

    @abstractmethod
    def _trace_evaluation(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        """
        Implement trace-level evaluation logic.

        This method is called when level="trace" and should evaluate the entire trace.

        Args:
            trace: The complete trace to evaluate
            task: Optional task for ground truth

        Returns:
            EvalResult with score and explanation
        """
        pass

    def _agent_evaluation(self, agent_trace: Any, task: Optional[Task] = None) -> EvalResult:
        """
        Implement agent-level evaluation logic.

        This method is called once per agent when level="agent".
        Only implement if evaluator supports agent-level evaluation.

        Args:
            agent_trace: AgentTrace scoped to this agent (reconstructed steps,
                         metadata, and metrics for a single agent in the trace)
            task: Optional task for ground truth

        Returns:
            EvalResult with score and explanation

        Raises:
            NotImplementedError: If evaluator doesn't support agent-level
        """
        raise NotImplementedError(
            f"{self.name} does not support level='agent'. "
            f"Supported levels: {', '.join(lvl.value for lvl in self._supported_levels)}"
        )

    def _span_evaluation(self, span: Any, task: Optional[Task] = None) -> EvalResult:
        """
        Implement span-level evaluation logic.

        This method is called once per LLM span when level="span".
        Only implement if evaluator supports span-level evaluation.

        Args:
            span: The LLM span to evaluate
            task: Optional task for ground truth

        Returns:
            EvalResult with score and explanation

        Raises:
            NotImplementedError: If evaluator doesn't support span-level
        """
        raise NotImplementedError(
            f"{self.name} does not support level='span'. "
            f"Supported levels: {', '.join(lvl.value for lvl in self._supported_levels)}"
        )

    def __call__(self, trace: Trace, task: Optional[Task] = None) -> List[EvalResult]:
        """
        Execute the evaluator.

        Simply calls evaluate() - the runner will handle enriching with metadata.
        """
        return self.evaluate(trace, task)


class LLMAsJudgeEvaluator(BaseEvaluator):
    """
    Base class for LLM-as-judge evaluators.
    Uses an LLM to evaluate outputs for subjective criteria.

    Supports flexible prompt templates with flat variable access (Python str.format()).
    Use a custom prompt_builder to extract and flatten data from trace/task.

    Example with custom prompt:
        class CustomJudge(LLMAsJudgeEvaluator):
            def __init__(self):
                super().__init__(
                    model="gpt-4o-mini",
                    prompt_template='''
                        Evaluate if the agent used tools appropriately.

                        Query: {query}
                        Tools Used: {tools_used}
                        Response: {response}

                        Score (0.0-1.0):
                        Explanation:
                    ''',
                    prompt_builder=self.build_prompt
                )

            def build_prompt(self, trace, task):
                # Extract and flatten data for template variables
                tools_used = [s.name for s in trace.get_tool_calls()]
                return {
                    "query": trace.input,
                    "response": trace.output,
                    "tools_used": ", ".join(tools_used) if tools_used else "None"
                }

            def call_llm(self, prompt):
                # Your LLM API call
                pass
    """

    def __init__(
        self,
        model: str = "gpt-4",
        prompt_template: Optional[str] = None,
        criteria: Optional[str] = None,
        prompt_builder: Optional[Callable] = None,
        **kwargs,
    ):
        """
        Initialize LLM-as-judge evaluator.

        Args:
            model: LLM model to use
            prompt_template: Template string with {variable} placeholders
            criteria: Default evaluation criteria (used if no prompt_template)
            prompt_builder: Optional function(observation, task) -> dict of template variables
                           Allows custom logic to prepare prompt context
        """
        super().__init__(**kwargs)
        self.model = model
        self.prompt_template = prompt_template or self._default_prompt_template()
        self.criteria = criteria or "quality, accuracy, and helpfulness"
        self.prompt_builder = prompt_builder or self._default_prompt_builder

    def _default_prompt_template(self) -> str:
        """Default evaluation prompt template."""
        return """You are an expert evaluator assessing AI agent outputs.

Task Input: {input}
Agent Output: {output}
{reference_section}
{criteria_section}

Please evaluate the agent's output on a scale of 0.0 to 1.0.
Provide your score and explanation in this format:
Score: <number between 0.0 and 1.0>
Explanation: <your reasoning>
"""

    def _default_prompt_builder(self, trace: Trace, task: Optional[Task] = None) -> dict:
        """
        Build template variables for the default prompt.

        Returns a dict of flat variables for str.format(). Python's str.format() doesn't
        support nested attribute access (like {trace.input}), so extract and flatten
        all needed values.

        Override this or provide custom prompt_builder to customize variables.
        """
        reference_section = ""
        if task and task.expected_output:
            reference_section = f"\nExpected Output: {task.expected_output}"

        criteria_section = f"\nEvaluation Criteria: {self.criteria}"
        if task and task.success_criteria:
            criteria_section = f"\nSuccess Criteria: {task.success_criteria}"

        return {
            "input": trace.input,
            "output": trace.output,
            "reference_section": reference_section,
            "criteria_section": criteria_section,
        }

    @abstractmethod
    def call_llm(self, prompt: str) -> dict:
        """
        Call the LLM API. Must be implemented by subclasses.

        Args:
            prompt: The formatted prompt string

        Returns:
            dict with keys:
                - score: float between 0.0 and 1.0
                - explanation: str with reasoning
                - (optional) other details
        """
        pass

    def _trace_evaluation(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        """Evaluate using LLM-as-judge."""
        # Build template variables using custom or default builder
        template_vars = self.prompt_builder(trace, task)

        # Format the prompt with variables
        prompt = self.prompt_template.format(**template_vars)

        # Call LLM
        llm_response = self.call_llm(prompt)

        return EvalResult(
            score=llm_response.get("score", 0.0),
            explanation=llm_response.get("explanation", ""),
            details={"model": self.model, "criteria": self.criteria, **llm_response.get("details", {})},
        )


class FunctionEvaluator(BaseEvaluator):
    """
    Wraps a plain function as an evaluator (single-level).

    The function receives the appropriate observation type for its level:
    - trace:  fn(trace: Trace, task=None)
    - agent:  fn(agent_trace: AgentTrace, task=None)
    - span:   fn(span: LLMSpan|ToolSpan|..., task=None)

    Supports exactly one level (the level it was registered with).
    _auto_detect_supported_levels() returns [self.level] instead of
    inspecting method overrides.
    """

    def __init__(self, func: Callable, name: Optional[str] = None, **kwargs):
        # super().__init__ runs _init_params_from_kwargs (sets self.level)
        # then _auto_detect_supported_levels (uses self.level) — ordering is correct.
        super().__init__(**kwargs)
        self.func = func
        self.name = name or func.__name__

    def _auto_detect_supported_levels(self) -> List:
        """Return only the level this function was registered with."""
        return [self.level]

    def _call_func(self, observation: Any, task: Optional[Task] = None) -> EvalResult:
        """Call the wrapped function and normalize its return value."""
        result = self.func(observation, task)
        if isinstance(result, EvalResult):
            return result
        elif isinstance(result, dict):
            return EvalResult(
                score=result.get("score", 0.0),
                passed=result.get("passed"),
                explanation=result.get("explanation", ""),
                details=result.get("details"),
            )
        elif isinstance(result, (int, float)):
            return EvalResult(score=float(result))
        else:
            raise TypeError(f"Evaluator function must return EvalResult, dict, or float, got {type(result)}")

    def _trace_evaluation(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        return self._call_func(trace, task)

    def _agent_evaluation(self, agent_trace: Any, task: Optional[Task] = None) -> EvalResult:
        return self._call_func(agent_trace, task)

    def _span_evaluation(self, span: Any, task: Optional[Task] = None) -> EvalResult:
        return self._call_func(span, task)
