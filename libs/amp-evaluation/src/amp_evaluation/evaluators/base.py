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

One abstract method: evaluate(). Type hints drive everything:
- First parameter type hint determines evaluation level (Trace, AgentTrace, LLMSpan)
- Task parameter presence determines mode compatibility:
  - def evaluate(self, trace: Trace) -> EvalResult:              # both modes
  - def evaluate(self, trace: Trace, task: Task) -> EvalResult:  # experiment only
  - def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:  # both modes

LLM-as-judge evaluators use build_prompt() instead of evaluate():
  - def build_prompt(self, trace: Trace, task: Task = None) -> str
  - Level and mode auto-detected from build_prompt() type hints (same mechanism)
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import List, Optional, Callable, TYPE_CHECKING, Any, Dict, Tuple
import logging
import inspect
import typing

from pydantic import BaseModel, Field, ValidationError

from ..models import EvalResult, EvaluatorInfo
from .params import Param, _ParamDescriptor, EvaluationLevel, EvalMode, _NO_DEFAULT

if TYPE_CHECKING:
    from ..dataset import Task
    from ..trace.models import Trace


logger = logging.getLogger(__name__)


# Type hint name to evaluation level mapping
TYPE_TO_LEVEL = {
    "Trace": EvaluationLevel.TRACE,
    "AgentTrace": EvaluationLevel.AGENT,
    "LLMSpan": EvaluationLevel.LLM,
}


# ============================================================================
# REUSABLE DETECTION HELPERS
# ============================================================================


def _detect_level_from_callable(target, evaluator_name: str = "evaluator") -> EvaluationLevel:
    """
    Detect evaluation level from a callable's first parameter type hint.

    Reused by BaseEvaluator, FunctionEvaluator, LLMAsJudgeEvaluator,
    and FunctionLLMJudge — each passes its target method/function.

    Args:
        target: The callable to inspect (method or function)
        evaluator_name: Name for error messages

    Returns:
        EvaluationLevel (TRACE, AGENT, or LLM)
    """
    try:
        hints = typing.get_type_hints(target)
    except Exception:
        hints = {}

    sig = inspect.signature(target)
    params = [p for p in sig.parameters.keys() if p != "self"]

    if not params:
        raise TypeError(
            f"Evaluator '{evaluator_name}': method must have at least one parameter "
            f"with a type hint (Trace, AgentTrace, or LLMSpan)."
        )

    first_param = params[0]
    first_hint = hints.get(first_param)

    if first_hint is None:
        raise TypeError(
            f"Evaluator '{evaluator_name}': first parameter '{first_param}' must have a type hint "
            f"(Trace, AgentTrace, or LLMSpan) to determine the evaluation level."
        )

    type_name = getattr(first_hint, "__name__", str(first_hint))
    level = TYPE_TO_LEVEL.get(type_name)

    if level is None:
        raise TypeError(
            f"Evaluator '{evaluator_name}': unsupported type '{type_name}' on first parameter. "
            f"Must be one of: Trace, AgentTrace, LLMSpan"
        )

    return level


def _detect_modes_from_callable(target, skip_param_defaults: bool = False) -> List[EvalMode]:
    """
    Detect supported eval modes from a callable's signature.

    Reused by BaseEvaluator, FunctionEvaluator, LLMAsJudgeEvaluator,
    and FunctionLLMJudge — each passes its target method/function.

    Args:
        target: The callable to inspect
        skip_param_defaults: If True, skip params with Param defaults (for FunctionEvaluator)

    Returns:
        List of supported EvalMode values
    """
    sig = inspect.signature(target)
    params = [p for p in sig.parameters.values() if p.name != "self"]

    if skip_param_defaults:
        params = [p for p in params if not isinstance(p.default, _ParamDescriptor)]

    required = [p for p in params if p.default is inspect.Parameter.empty]

    if len(params) <= 1:
        return [EvalMode.EXPERIMENT, EvalMode.MONITOR]
    elif len(required) >= 2:
        return [EvalMode.EXPERIMENT]
    else:
        return [EvalMode.EXPERIMENT, EvalMode.MONITOR]


def _count_callable_params(target, skip_param_defaults: bool = False) -> int:
    """
    Count non-self params of a callable, optionally skipping Param defaults.

    Args:
        target: The callable to inspect
        skip_param_defaults: If True, skip params with Param defaults

    Returns:
        Number of non-self params
    """
    sig = inspect.signature(target)
    params = [p for p in sig.parameters.values() if p.name != "self"]

    if skip_param_defaults:
        params = [p for p in params if not isinstance(p.default, _ParamDescriptor)]

    return len(params)


# ============================================================================
# BASE EVALUATOR
# ============================================================================


class BaseEvaluator(ABC):
    """
    Abstract base class for all evaluators.

    One abstract method: evaluate(). Type hint on the first parameter
    determines the evaluation level. Task parameter determines mode compatibility.

    Evaluation levels (auto-detected from type hint):
    - trace:  evaluate(self, trace: Trace) -> called once per trace
    - agent:  evaluate(self, agent_trace: AgentTrace) -> called once per agent
    - llm:    evaluate(self, llm_span: LLMSpan) -> called once per LLM call

    Class Attributes for Metadata:
        name: Unique evaluator name (defaults to class name)
        description: Human-readable description
        tags: List of tags for categorization
        version: Evaluator version string

    Example:
        class LatencyEvaluator(BaseEvaluator):
            name = "latency"
            description = "Checks response latency"
            tags = ["performance"]
            max_latency_ms: float = Param(default=5000, description="Max latency")

            def evaluate(self, trace: Trace) -> EvalResult:
                latency = trace.metrics.total_duration_ms
                return EvalResult(score=1.0 if latency <= self.max_latency_ms else 0.0)
    """

    # Class-level metadata attributes
    name: str = ""
    description: str = ""
    tags: List[str] = []
    version: str = "1.0"

    def __init__(self, **kwargs):
        # Set default name to class name if not already set
        if not self.name:
            self.name = self.__class__.__name__

        # Ensure tags is a fresh mutable list per instance (avoids shared-default mutation)
        self.tags = list(self.tags) if self.tags else []

        self._aggregations: Optional[List] = None

        # Check if class has default aggregations set via decorator
        if hasattr(self.__class__, "_default_aggregations") and self.__class__._default_aggregations:
            self._aggregations = self.__class__._default_aggregations

        # Initialize Param descriptors from kwargs
        self._init_params_from_kwargs(kwargs)

        # Auto-detect supported eval modes from method signature
        self._supported_eval_modes = self._auto_detect_supported_eval_modes()

        # Cache method param counts for smart dispatch in run()
        self._method_param_counts = self._cache_method_param_counts()

    def _init_params_from_kwargs(self, kwargs: Dict[str, Any]):
        """
        Initialize Param descriptors from kwargs and validate required params.

        Allows evaluators to be instantiated with:
            evaluator = MyEvaluator(threshold=0.8, model="gpt-4")

        Raises TypeError if any required Param (defined without a default) is
        not provided — catching configuration errors at init time rather than
        silently skipping at evaluation time.
        """
        valid_config_names = set()
        missing_required = []
        for attr_name in dir(type(self)):
            attr = getattr(type(self), attr_name, None)
            if isinstance(attr, _ParamDescriptor):
                valid_config_names.add(attr_name)
                if attr_name in kwargs:
                    setattr(self, attr_name, kwargs[attr_name])
                elif attr.required and attr.default is _NO_DEFAULT:
                    hint = f" ({attr.description})" if attr.description else ""
                    missing_required.append(f"'{attr_name}'{hint}")
                elif attr.default is not _NO_DEFAULT:
                    # Route defaults through __set__ so they get validated
                    setattr(self, attr_name, attr.default)

        if missing_required:
            raise TypeError(f"Evaluator '{self.name}' missing required parameter(s): {', '.join(missing_required)}")

        unknown_kwargs = set(kwargs.keys()) - valid_config_names
        if unknown_kwargs:
            raise TypeError(
                f"{self.__class__.__name__}.__init__() got unexpected keyword argument(s): "
                f"{', '.join(sorted(unknown_kwargs))}"
            )

    def _auto_detect_supported_eval_modes(self) -> List[EvalMode]:
        """Auto-detect supported eval modes from the evaluate() method signature."""
        return _detect_modes_from_callable(self.evaluate)

    def _cache_method_param_counts(self) -> Dict[str, int]:
        """Cache non-self param count for evaluate() method."""
        return {"evaluate": _count_callable_params(self.evaluate)}

    def _extract_config_schema(self) -> List[Dict[str, Any]]:
        """Extract configuration schema from Param descriptors."""
        schema = []
        for attr_name in dir(type(self)):
            attr = getattr(type(self), attr_name, None)
            if isinstance(attr, _ParamDescriptor):
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

    @property
    def level(self) -> EvaluationLevel:
        """
        Auto-detected evaluation level from evaluate()'s first parameter type hint.

        Returns:
            EvaluationLevel.TRACE, EvaluationLevel.AGENT, or EvaluationLevel.LLM

        Raises:
            TypeError: If type hint is missing or unsupported
        """
        return self._detect_level()

    def _detect_level(self) -> EvaluationLevel:
        """Detect evaluation level from evaluate() type hint."""
        return _detect_level_from_callable(self.evaluate, self.name)

    @property
    def info(self) -> EvaluatorInfo:
        """
        Evaluator metadata including name, level, modes, config schema.

        Returns:
            EvaluatorInfo with complete evaluator metadata
        """
        return EvaluatorInfo(
            name=self.name,
            description=getattr(self, "description", ""),
            tags=list(getattr(self, "tags", [])),
            version=getattr(self, "version", "1.0"),
            modes=[m.value for m in self._supported_eval_modes],
            level=self.level.value,
            config_schema=self._extract_config_schema(),
        )

    @abstractmethod
    def evaluate(self, *args: Any, **kwargs: Any) -> EvalResult:
        """
        Evaluate a single trace or span.

        Type hint the first parameter to set the evaluation level:
          - trace: Trace           -> trace-level (called once per trace)
          - agent_trace: AgentTrace -> agent-level (called once per agent)
          - llm_span: LLMSpan      -> llm-level (called once per LLM call)

        The task parameter determines mode compatibility:
          - No task param              -> works in both monitor and experiment
          - task: Task                 -> experiment only (requires ground truth)
          - task: Optional[Task] = None -> both modes (adapts behavior)

        Returns:
            EvalResult with score and explanation
        """
        ...

    def run(self, trace: Trace, task: Optional[Task] = None) -> List[EvalResult]:
        """
        Dispatch method called by the runner. Handles iteration.

        A single trace can have MULTIPLE agents and LLM calls.
        run() iterates and calls evaluate() once per item:

        - trace level: evaluate(trace) called once
        - agent level: evaluate(agent_trace) called N times (once per agent)
        - llm level:   evaluate(llm_span) called N times (once per LLM call)

        NOT overridden by evaluator authors.
        """
        from ..trace.models import AgentTrace as _AgentTrace

        results = []
        eval_level = self.level
        param_count = self._method_param_counts.get("evaluate", 2)

        def _call_evaluate(input_data, task_arg):
            if param_count <= 1:
                return self.evaluate(input_data)
            else:
                return self.evaluate(input_data, task_arg)

        if eval_level == EvaluationLevel.TRACE:
            result = _call_evaluate(trace, task)
            results.append(result)

        elif eval_level == EvaluationLevel.AGENT:
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
                result = _call_evaluate(fallback, task)
                if result.details is None:
                    result.details = {}
                result.details["span_id"] = fallback.agent_id
                results.append(result)
            else:
                for agent_span in agent_spans:
                    agent_trace = trace.create_agent_trace(agent_span.span_id)
                    result = _call_evaluate(agent_trace, task)
                    if result.details is None:
                        result.details = {}
                    result.details["span_id"] = agent_trace.agent_id
                    results.append(result)

        elif eval_level == EvaluationLevel.LLM:
            # No deduplication for LLM-level — evaluate each call as-is
            llm_spans = trace.get_llm_calls(deduplicate_messages=False)

            for span in llm_spans:
                result = _call_evaluate(span, task)
                if result.details is None:
                    result.details = {}
                result.details["span_id"] = getattr(span, "span_id", None)
                results.append(result)

        return results

    def __call__(self, trace: Trace, task: Optional[Task] = None) -> List[EvalResult]:
        """Execute the evaluator via run() dispatch."""
        return self.run(trace, task)


# ============================================================================
# LLM-AS-JUDGE EVALUATOR
# ============================================================================


class JudgeOutput(BaseModel):
    """Pydantic model for LLM judge output validation."""

    score: float = Field(ge=0.0, le=1.0, description="Score between 0.0 and 1.0")
    explanation: str = Field(default="", description="Explanation of the score")


class LLMAsJudgeEvaluator(BaseEvaluator):
    """
    LLM-as-judge evaluator — write a build_prompt() method, get back EvalResult.

    How it works:
    1. Override build_prompt() with typed parameters — same as evaluate()
    2. Level auto-detected from build_prompt() first param type hint
    3. Mode auto-detected from build_prompt() task param
    4. Framework appends output format instructions automatically
    5. LLM called via LiteLLM, response validated with Pydantic JudgeOutput
    6. Retries on invalid output with Pydantic error as context (like Instructor)

    Example:
        class GroundingJudge(LLMAsJudgeEvaluator):
            name = "grounding-judge"
            model = "gpt-4o"
            criteria = "Is the response grounded in tool results?"

            def build_prompt(self, trace: Trace, task: Task = None) -> str:
                tools = trace.get_tool_calls()
                tool_info = "\\n".join(f"- {t.name}: {t.result}" for t in tools)
                return f\"\"\"Evaluate whether this response is grounded.
                Input: {trace.input}
                Output: {trace.output}
                Tool Results: {tool_info}\"\"\"

        # Or use the @llm_judge decorator:
        @llm_judge(model="gpt-4o")
        def grounding_judge(trace: Trace) -> str:
            return f"Is this grounded? {trace.output}"
    """

    # Configurable via Param descriptors
    model: str = Param(default="gpt-4o-mini", description="LiteLLM model identifier")
    criteria: str = Param(default="quality, accuracy, and helpfulness", description="Evaluation criteria")
    temperature: float = Param(default=0.0, description="LLM temperature")
    max_tokens: int = Param(default=1024, description="Max tokens for LLM response")
    threshold: float = Param(default=0.5, description="Score threshold for pass/fail")
    max_retries: int = Param(default=2, description="Max retries on invalid LLM output")

    # Output format instructions — auto-appended to the user's prompt
    _OUTPUT_INSTRUCTIONS = """

Evaluate and respond with a JSON object:
{
  "score": <float between 0.0 and 1.0>,
  "explanation": "<brief explanation of your evaluation>"
}"""

    # ─── User must override this ────────────────────────────────────

    @abstractmethod
    def build_prompt(self, *args: Any, **kwargs: Any) -> str:
        """
        Override this method to write your evaluation prompt.

        Type hint on the first param controls level detection:
            trace: Trace        -> TRACE level (called once per trace)
            agent: AgentTrace   -> AGENT level (called per agent span)
            llm: LLMSpan        -> LLM level (called per LLM span)

        Task parameter controls mode detection:
            task: Task           -> experiment only (task required)
            task: Task = None    -> both experiment and monitor
            (no task param)      -> both modes (monitor-friendly)

        Returns the prompt string. Output format is auto-appended.
        Do NOT include scoring instructions.
        """
        ...

    # ─── Level/mode detection — reuses the SAME helper functions ─────

    def _detect_level(self) -> EvaluationLevel:
        """Point detection at build_prompt() instead of evaluate()."""
        return _detect_level_from_callable(self.build_prompt, self.name)

    def _auto_detect_supported_eval_modes(self) -> List[EvalMode]:
        """Point detection at build_prompt() instead of evaluate()."""
        return _detect_modes_from_callable(self.build_prompt)

    def _cache_method_param_counts(self) -> Dict[str, int]:
        """Cache param count for build_prompt() (used for dispatch)."""
        return {"evaluate": _count_callable_params(self.build_prompt)}

    # ─── Internal pipeline: evaluate → build_prompt → LLM → validate ─

    def evaluate(self, trace_or_span, task=None) -> EvalResult:
        """Internal: calls build_prompt() -> LLM -> validate -> EvalResult."""
        # 1. Dispatch to build_prompt() (respects signature param count)
        prompt = self._dispatch_build_prompt(trace_or_span, task)

        # 2. Append output format instructions
        full_prompt = prompt + self._OUTPUT_INSTRUCTIONS

        # 3. Call LLM with retry on invalid output
        return self._call_llm_with_retry(full_prompt)

    def _dispatch_build_prompt(self, input_data, task):
        """Dispatch to build_prompt() based on its param count."""
        param_count = self._method_param_counts.get("evaluate", 2)
        if param_count <= 1:
            return self.build_prompt(input_data)
        else:
            return self.build_prompt(input_data, task)

    def _call_llm_with_retry(self, prompt: str) -> EvalResult:
        """Call LLM via LiteLLM, validate with Pydantic, retry on failure."""
        try:
            from litellm import completion
        except ImportError:
            raise ImportError("LiteLLM is required for LLM-as-judge evaluators. Install with: pip install litellm")

        last_error = None
        for attempt in range(self.max_retries + 1):
            retry_ctx = ""
            if last_error and attempt > 0:
                retry_ctx = (
                    f"\n\n[Previous response was invalid: {last_error}. "
                    f"Please respond with valid JSON matching the format above.]"
                )

            response = completion(
                model=self.model,
                messages=[{"role": "user", "content": prompt + retry_ctx}],
                temperature=self.temperature,
                max_tokens=self.max_tokens,
                response_format={"type": "json_object"},
            )

            content = response.choices[0].message.content
            result, error = self._parse_and_validate(content)
            if result is not None:
                return result
            last_error = error

        # All retries exhausted
        return EvalResult(
            score=0.0,
            passed=False,
            explanation=f"LLM output validation failed after {self.max_retries + 1} attempts: {last_error}",
            details={"model": self.model, "error": str(last_error)},
        )

    def _parse_and_validate(self, content: str) -> Tuple[Optional[EvalResult], Optional[str]]:
        """
        Parse LLM response with Pydantic JudgeOutput model.
        Returns (EvalResult, None) on success, (None, error_msg) on failure.
        """
        try:
            output = JudgeOutput.model_validate_json(content)
        except ValidationError as e:
            return None, str(e)

        return EvalResult(
            score=output.score,
            passed=output.score >= self.threshold,
            explanation=output.explanation,
            details={"model": self.model, "criteria": self.criteria},
        ), None


# ============================================================================
# FUNCTION EVALUATOR
# ============================================================================


class FunctionEvaluator(BaseEvaluator):
    """
    Wraps a plain function as an evaluator.

    Level is auto-detected from the function's first parameter type hint.
    Config params are detected from Param defaults in the function signature.

    Supports with_config() to create configured copies.
    """

    def __init__(self, func: Callable, name: Optional[str] = None, **kwargs):
        self.func = func
        self._config: Dict[str, Any] = {}
        self._param_descriptors: Dict[str, _ParamDescriptor] = {}

        # Extract Param descriptors from function defaults BEFORE super().__init__
        self._extract_function_params(func)

        # Apply any config overrides from kwargs
        remaining_kwargs = {}
        for k, v in kwargs.items():
            if k in self._param_descriptors:
                self._config[k] = v
            else:
                remaining_kwargs[k] = v

        super().__init__(**remaining_kwargs)
        self.name = name or func.__name__

        # Copy metadata from function if present
        if hasattr(func, "__doc__") and func.__doc__ and not self.description:
            self.description = func.__doc__.strip().split("\n")[0]

    def _extract_function_params(self, func: Callable):
        """Extract Param descriptors from function signature defaults."""
        sig = inspect.signature(func)
        hints = {}
        try:
            hints = typing.get_type_hints(func)
        except Exception:
            pass

        for param_name, param in sig.parameters.items():
            if isinstance(param.default, _ParamDescriptor):
                p = param.default
                # Infer type from function type hint
                if param_name in hints:
                    p.type = hints[param_name]
                p._attr_name = param_name
                self._param_descriptors[param_name] = p
                if p.default is not _NO_DEFAULT:
                    self._config[param_name] = p.default

    def _detect_level(self) -> EvaluationLevel:
        """Detect level from the wrapped function's first parameter type hint."""
        return _detect_level_from_callable(self.func, self.name)

    def _auto_detect_supported_eval_modes(self) -> List[EvalMode]:
        """Detect modes from the wrapped function's signature."""
        return _detect_modes_from_callable(self.func, skip_param_defaults=True)

    def _cache_method_param_counts(self) -> Dict[str, int]:
        """Cache param count for the wrapped function."""
        return {"evaluate": _count_callable_params(self.func, skip_param_defaults=True)}

    def _extract_config_schema(self) -> List[Dict[str, Any]]:
        """Extract config schema from function Param descriptors."""
        schema = []
        for param in self._param_descriptors.values():
            schema.append(param.to_schema())
        return schema

    def evaluate(self, trace_or_span, task=None) -> EvalResult:
        """Call the wrapped function with config values injected."""
        sig = inspect.signature(self.func)
        non_config_params = [p for p in sig.parameters.values() if not isinstance(p.default, _ParamDescriptor)]

        # Build kwargs: trace/span + optional task + config params
        call_kwargs = {}
        param_names = list(sig.parameters.keys())

        # Set first param (trace or span)
        if param_names:
            call_kwargs[param_names[0]] = trace_or_span

        # Set task param if function accepts it
        if len(non_config_params) > 1:
            task_param_name = [p.name for p in non_config_params][1]
            call_kwargs[task_param_name] = task

        # Inject config values
        for config_name, config_value in self._config.items():
            if config_name in sig.parameters:
                call_kwargs[config_name] = config_value

        result = self.func(**call_kwargs)
        return _normalize_result(result)

    def with_config(self, **kwargs) -> "FunctionEvaluator":
        """
        Create a new evaluator with overridden config values.

        Args:
            **kwargs: Config values to override

        Returns:
            New FunctionEvaluator with updated config
        """
        # Validate config keys
        for key in kwargs:
            if key not in self._param_descriptors:
                raise TypeError(f"Unknown config parameter '{key}'. Available: {list(self._param_descriptors.keys())}")
            # Validate value
            self._param_descriptors[key]._validate(kwargs[key])

        new_eval = FunctionEvaluator(self.func, name=self.name)
        new_eval.description = self.description
        new_eval.tags = list(self.tags)
        new_eval.version = self.version
        if self._aggregations:
            new_eval._aggregations = list(self._aggregations)
        new_eval._config = {**self._config, **kwargs}
        return new_eval

    @property
    def info(self) -> EvaluatorInfo:
        """Evaluator metadata with config schema from function params."""
        return EvaluatorInfo(
            name=self.name,
            description=self.description,
            tags=list(self.tags),
            version=self.version,
            modes=[m.value for m in self._supported_eval_modes],
            level=self.level.value,
            config_schema=self._extract_config_schema(),
        )


# ============================================================================
# FUNCTION LLM JUDGE (created by @llm_judge decorator)
# ============================================================================


class FunctionLLMJudge(LLMAsJudgeEvaluator):
    """LLM-as-judge wrapping a prompt-building function. Created by @llm_judge."""

    def __init__(self, func: Callable, name: Optional[str] = None, **kwargs):
        self.func = func
        super().__init__(**kwargs)
        self.name = name or func.__name__

    def build_prompt(self, *args, **kwargs) -> str:
        """Delegate to the wrapped function."""
        return self.func(*args, **kwargs)

    # Detection reuses the SAME helper functions, pointed at self.func
    def _detect_level(self) -> EvaluationLevel:
        return _detect_level_from_callable(self.func, self.name)

    def _auto_detect_supported_eval_modes(self) -> List[EvalMode]:
        return _detect_modes_from_callable(self.func)

    def _cache_method_param_counts(self) -> Dict[str, int]:
        return {"evaluate": _count_callable_params(self.func)}


# ============================================================================
# RESULT NORMALIZATION
# ============================================================================


def _normalize_result(result) -> EvalResult:
    """Normalize different result types to EvalResult."""
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
        return EvalResult(score=float(result))
    else:
        raise TypeError(
            f"Evaluator returned invalid type {type(result).__name__}.\nExpected: EvalResult | dict | float"
        )
