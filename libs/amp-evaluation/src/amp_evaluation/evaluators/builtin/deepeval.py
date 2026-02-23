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
DeepEval Agent Evaluation Metrics.

This module provides wrapper evaluators for DeepEval's agent-specific metrics.
These metrics are designed to evaluate AI agents across three layers:

1. Reasoning Layer:
   - PlanQualityMetric: Evaluates if the agent's plan is logical and complete
   - PlanAdherenceMetric: Evaluates if the agent follows its own plan

2. Action Layer:
   - ToolCorrectnessMetric: Evaluates if the agent selects correct tools
   - ArgumentCorrectnessMetric: Evaluates if tool arguments are correct

3. Execution Layer:
   - TaskCompletionMetric: Evaluates if the agent completes the task
   - StepEfficiencyMetric: Evaluates if the agent completes tasks efficiently

All evaluators have tags: ["deepeval", "llm-judge"] plus layer and aspect-specific tags.

Reference: https://deepeval.com/guides/guides-ai-agent-evaluation-metrics
"""

from __future__ import annotations

import logging
from typing import Optional, List, Dict, Any

from amp_evaluation.evaluators.base import BaseEvaluator
from amp_evaluation.evaluators.params import Param
from amp_evaluation.models import EvalResult
from amp_evaluation.trace.models import Trace
from amp_evaluation.dataset import Task

logger = logging.getLogger(__name__)


def _check_deepeval_installed() -> bool:
    """Check if deepeval is installed."""
    import importlib.util

    return importlib.util.find_spec("deepeval") is not None


def _get_deepeval_metric_class(metric_name: str):
    """Dynamically import a DeepEval metric class."""
    try:
        from deepeval import metrics

        return getattr(metrics, metric_name)
    except ImportError as e:
        raise ImportError(f"DeepEval is not installed. Install with: pip install deepeval\nError: {e}")
    except AttributeError as e:
        raise ImportError(
            f"DeepEval metric '{metric_name}' not found in deepeval.metrics. "
            f"Check the metric name or update deepeval: pip install --upgrade deepeval\nError: {e}"
        )


class DeepEvalBaseEvaluator(BaseEvaluator):
    """
    Base class for DeepEval metric wrappers.

    Provides common functionality for converting between amp_evaluation
    and DeepEval data structures.
    """

    # Param descriptors — type inferred from annotation
    threshold: float = Param(default=0.7, min=0.0, max=1.0, description="Minimum score for passing")
    model: str = Param(default="gpt-4o", description="LLM model to use for evaluation")
    include_reason: bool = Param(default=True, description="Whether to include reasoning in the result")
    strict_mode: bool = Param(default=False, description="If True, use binary scoring (0 or 1)")

    def __init__(self, **kwargs):
        """
        Initialize DeepEval evaluator wrapper.

        Args:
            threshold: Minimum score for passing (0.0-1.0)
            model: LLM model to use for evaluation (e.g., "gpt-4o", "gpt-4o-mini")
            include_reason: Whether to include reasoning in the result
            strict_mode: If True, use binary scoring (0 or 1)
        """
        super().__init__(**kwargs)

        # Verify deepeval is available
        if not _check_deepeval_installed():
            logger.warning(
                "DeepEval is not installed. Evaluator will fail at runtime. Install with: pip install deepeval"
            )

    def _build_deepeval_test_case(self, trace: Trace, task: Task) -> Any:
        """
        Build a DeepEval LLMTestCase from trace and task.

        Subclasses may override to customize test case construction.
        """
        try:
            from deepeval.test_case import LLMTestCase
        except ImportError:
            raise ImportError("DeepEval is required. Install with: pip install deepeval")

        # Extract input and output
        input_text = trace.input or ""
        actual_output = trace.output or ""

        # Build test case kwargs
        kwargs: Dict[str, Any] = {
            "input": input_text,
            "actual_output": actual_output,
        }

        # Add expected output if available
        if task and task.expected_output:
            kwargs["expected_output"] = task.expected_output

        # Add retrieval context if available from tool spans
        retrieval_context = self._extract_retrieval_context(trace)
        if retrieval_context:
            kwargs["retrieval_context"] = retrieval_context

        # Add tool call information
        tool_calls = self._extract_tools_called(trace)
        if tool_calls:
            kwargs["tools_called"] = tool_calls

        return LLMTestCase(**kwargs)

    def _extract_retrieval_context(self, trace: Trace) -> Optional[List[str]]:
        """Extract retrieval context from retriever spans if available."""
        retriever_spans = trace.get_retrievals()
        if not retriever_spans:
            return None

        context = []
        for span in retriever_spans:
            if span.documents:
                if isinstance(span.documents, list):
                    context.extend([str(item) for item in span.documents])
                else:
                    context.append(str(span.documents))

        return context if context else None

    def _extract_tools_called(self, trace: Trace) -> List[Any]:
        """Extract tool call information from trace as DeepEval ToolCall objects."""
        try:
            from deepeval.test_case import ToolCall
        except ImportError:
            raise ImportError("DeepEval is required. Install with: pip install deepeval")

        tools = []
        for span in trace.get_tool_calls():
            # Convert to DeepEval ToolCall format
            tool_call = ToolCall(
                name=span.name,
                input_parameters=span.arguments if hasattr(span, "arguments") else None,
                output=span.result if hasattr(span, "result") else None,
            )
            tools.append(tool_call)
        return tools

    def _convert_deepeval_result(self, metric: Any) -> EvalResult:
        """Convert DeepEval metric result to EvalResult."""
        # DeepEval metrics store results after measure()
        score = getattr(metric, "score", 0.0)
        reason = getattr(metric, "reason", "")

        # Handle None score
        if score is None:
            score = 0.0

        passed = score >= self.threshold

        return EvalResult(
            score=score,
            passed=passed,
            explanation=reason if self.include_reason else "",
            details={
                "threshold": self.threshold,
                "model": self.model,
                "strict_mode": self.strict_mode,
            },
        )

    def evaluate(self, trace: Trace, task: Task) -> EvalResult:
        """
        Template method for DeepEval evaluations with error handling.

        Subclasses should implement _evaluate_with_deepeval() instead of this method.
        This method wraps the evaluation with consistent error handling.
        """
        try:
            return self._evaluate_with_deepeval(trace, task)
        except ImportError as e:
            return EvalResult.skip(
                f"DeepEval not installed: {e}",
                details={"error": str(e)},
            )
        except Exception as e:
            logger.error(f"{self.name} evaluation failed: {e}")
            return EvalResult.skip(
                f"Evaluation failed: {e}",
                details={"error": str(e)},
            )

    def _evaluate_with_deepeval(self, trace: Trace, task: Task) -> EvalResult:
        """
        Perform the actual DeepEval metric evaluation.

        Subclasses must implement this method to define their specific evaluation logic.
        This method should:
        1. Get the DeepEval metric class
        2. Create and configure the metric
        3. Build the test case
        4. Call metric.measure()
        5. Convert and return the result

        Error handling is done by the parent evaluate() method.
        """
        raise NotImplementedError("Subclasses must implement _evaluate_with_deepeval()")


# =============================================================================
# Reasoning Layer Evaluators
# =============================================================================


class DeepEvalPlanQualityEvaluator(DeepEvalBaseEvaluator):
    """
    Evaluates whether the agent's plan is logical, complete, and efficient.

    The PlanQualityMetric extracts the task and plan from the agent's trace
    and uses an LLM judge to assess plan quality.

    Score calculation:
        Plan Quality Score = AlignmentScore(Task, Plan)

    The metric extracts the task (user's goal) and plan (agent's strategy) from
    the trace, then uses an LLM to score how well the plan addresses the task
    requirements.

    Note: If no plan is detectable in the trace, the metric passes with score 1.0.
    """

    # Class-level metadata
    name = "deepeval/plan-quality"
    description = "Assesses whether agent's plan is logical, complete and efficient for task"
    tags = ["deepeval", "llm-judge", "reasoning", "planning"]
    evaluator_type = "agent"
    version = "1.0"

    def _evaluate_with_deepeval(self, trace: Trace, task: Task) -> EvalResult:
        """Evaluate the quality of the agent's plan."""
        PlanQualityMetric = _get_deepeval_metric_class("PlanQualityMetric")

        # Create metric
        metric = PlanQualityMetric(
            threshold=self.threshold,
            model=self.model,
            include_reason=self.include_reason,
            strict_mode=self.strict_mode,
        )

        # Build test case
        test_case = self._build_deepeval_test_case(trace, task)

        # Measure
        metric.measure(test_case)

        return self._convert_deepeval_result(metric)


class DeepEvalPlanAdherenceEvaluator(DeepEvalBaseEvaluator):
    """
    Evaluates whether the agent follows its own plan during execution.

    Creating a good plan is only half the battle—an agent that deviates from
    its strategy mid-execution undermines its own reasoning.

    Score calculation:
        Plan Adherence Score = AlignmentScore((Task, Plan), Execution Steps)

    The metric extracts the task, plan, and actual execution steps from the trace,
    then uses an LLM to evaluate how faithfully the agent adhered to its stated plan.

    Tip: Combine with PlanQualityMetric—a high-quality plan that's ignored is as
    problematic as a poor plan that's followed perfectly.
    """

    # Class-level metadata
    name = "deepeval/plan-adherence"
    description = "Measures how faithfully the agent follows its stated plan during execution"
    tags = ["deepeval", "llm-judge", "reasoning", "planning"]
    evaluator_type = "agent"
    version = "1.0"

    def _evaluate_with_deepeval(self, trace: Trace, task: Task) -> EvalResult:
        """Evaluate how well the agent adheres to its plan."""
        PlanAdherenceMetric = _get_deepeval_metric_class("PlanAdherenceMetric")

        # Create metric
        metric = PlanAdherenceMetric(
            threshold=self.threshold,
            model=self.model,
            include_reason=self.include_reason,
            strict_mode=self.strict_mode,
        )

        # Build test case
        test_case = self._build_deepeval_test_case(trace, task)

        # Measure
        metric.measure(test_case)

        return self._convert_deepeval_result(metric)


# =============================================================================
# Action Layer Evaluators
# =============================================================================


class DeepEvalToolCorrectnessEvaluator(DeepEvalBaseEvaluator):
    """
    Evaluates whether the agent selects the correct tools for the task.

    Compares the tools the agent actually called against a list of expected tools.

    Score calculation:
        Tool Correctness = Number of Correctly Used Tools / Total Number of Tools Called

    The metric supports configurable strictness:
    - Tool name matching (default): considers a call correct if the tool name matches
    - Input parameter matching: also requires input arguments to match
    - Output matching: additionally requires outputs to match
    - Ordering consideration: optionally enforces call sequence
    - Exact matching: requires tools_called and expected_tools to be identical

    When available_tools is provided, the metric also uses an LLM to evaluate
    whether your tool selection was optimal.
    """

    # Class-level metadata
    name = "deepeval/tool-correctness"
    description = "Validates agent selects appropriate tools based on task requirements"
    tags = ["deepeval", "llm-judge", "action", "correctness"]
    evaluator_type = "agent"
    version = "1.0"

    # Additional Param descriptors — type inferred from annotation
    evaluate_input: bool = Param(default=False, description="If True, also check input arguments match")
    evaluate_output: bool = Param(default=False, description="If True, also check outputs match")
    evaluate_order: bool = Param(default=False, description="If True, enforce call sequence")
    exact_match: bool = Param(default=False, description="If True, require exact match of tools called vs expected")
    available_tools: Optional[List[str]] = Param(
        default=None, description="List of available tool names for LLM-based evaluation"
    )

    def __init__(self, **kwargs):
        """
        Initialize ToolCorrectness evaluator.

        Args:
            threshold: Minimum score for passing (0.0-1.0)
            model: LLM model to use for evaluation
            include_reason: Whether to include reasoning in the result
            strict_mode: If True, use binary scoring (0 or 1)
            evaluate_input: If True, also check input arguments match
            evaluate_output: If True, also check outputs match
            evaluate_order: If True, enforce call sequence
            exact_match: If True, require exact match of tools called vs expected
            available_tools: List of available tool names for LLM-based evaluation
        """
        super().__init__(**kwargs)

    def _evaluate_with_deepeval(self, trace: Trace, task: Task) -> EvalResult:
        """Evaluate if the agent selected the correct tools."""
        ToolCorrectnessMetric = _get_deepeval_metric_class("ToolCorrectnessMetric")

        # Create metric with configuration
        metric_kwargs: Dict[str, Any] = {
            "threshold": self.threshold,
            "evaluate_order": self.evaluate_order,
            "exact_match": self.exact_match,
        }

        # Only add model if LLM-based evaluation is needed
        if self.available_tools:
            metric_kwargs["model"] = self.model
            metric_kwargs["include_reason"] = self.include_reason

        metric = ToolCorrectnessMetric(**metric_kwargs)

        # Build test case with tools information
        test_case = self._build_tool_test_case(trace, task)

        # Measure
        metric.measure(test_case)

        return self._convert_deepeval_result(metric)

    def _build_tool_test_case(self, trace: Trace, task: Task) -> Any:
        """Build test case with tool call information."""
        try:
            from deepeval.test_case import LLMTestCase, ToolCall
        except ImportError:
            raise ImportError("DeepEval is required. Install with: pip install deepeval")

        # Extract tools called from trace
        tools_called = []
        for span in trace.get_tool_calls():
            tc_kwargs: Dict[str, Any] = {"name": span.name}
            if self.evaluate_input:
                tc_kwargs["input"] = span.arguments
            if self.evaluate_output:
                tc_kwargs["output"] = span.result
            tools_called.append(ToolCall(**tc_kwargs))

        # Build expected tools from task if available
        expected_tools = None
        if task and task.expected_trajectory:
            expected_tools = []
            for step in task.expected_trajectory:
                # TrajectoryStep has: tool, args, expected_output
                et_kwargs: Dict[str, Any] = {"name": step.tool}
                if self.evaluate_input and step.args:
                    et_kwargs["input"] = step.args
                if self.evaluate_output and step.expected_output:
                    et_kwargs["output"] = step.expected_output
                expected_tools.append(ToolCall(**et_kwargs))

        # Build kwargs for test case
        kwargs: Dict[str, Any] = {
            "input": trace.input or "",
            "actual_output": trace.output or "",
            "tools_called": tools_called,
        }

        # Add expected_tools if we have ground truth
        if expected_tools:
            kwargs["expected_tools"] = expected_tools

        # Add available_tools for LLM-based evaluation
        if self.available_tools:
            kwargs["available_tools"] = self.available_tools

        return LLMTestCase(**kwargs)


class DeepEvalArgumentCorrectnessEvaluator(DeepEvalBaseEvaluator):
    """
    Evaluates whether the agent generates correct arguments for tool calls.

    Selecting the right tool with wrong arguments is as problematic as
    selecting the wrong tool entirely.

    Score calculation:
        Argument Correctness = Number of Correctly Generated Input Parameters / Total Number of Tool Calls

    Unlike ToolCorrectnessMetric, this metric is fully LLM-based and referenceless—
    it evaluates argument correctness based on the input context rather than
    comparing against expected values.
    """

    # Class-level metadata
    name = "deepeval/argument-correctness"
    description = "Validates correctness of arguments and parameters passed to each tool call"
    tags = ["deepeval", "llm-judge", "action", "correctness"]
    evaluator_type = "agent"
    version = "1.0"

    def _evaluate_with_deepeval(self, trace: Trace, task: Task) -> EvalResult:
        """Evaluate if the agent generated correct arguments for tool calls."""
        ArgumentCorrectnessMetric = _get_deepeval_metric_class("ArgumentCorrectnessMetric")

        # Create metric
        metric = ArgumentCorrectnessMetric(
            threshold=self.threshold,
            model=self.model,
            include_reason=self.include_reason,
            strict_mode=self.strict_mode,
        )

        # Build test case with tool call details
        test_case = self._build_argument_test_case(trace, task)

        # Measure
        metric.measure(test_case)

        return self._convert_deepeval_result(metric)

    def _build_argument_test_case(self, trace: Trace, task: Task) -> Any:
        """Build test case with tool argument information."""
        try:
            from deepeval.test_case import LLMTestCase, ToolCall
        except ImportError:
            raise ImportError("DeepEval is required. Install with: pip install deepeval")

        # Extract tools called with input arguments
        tools_called = []
        for span in trace.get_tool_calls():
            tc_kwargs: Dict[str, Any] = {"name": span.name}
            if hasattr(span, "input") and span.input:
                tc_kwargs["input"] = span.input
            tools_called.append(ToolCall(**tc_kwargs))

        return LLMTestCase(
            input=trace.input or "",
            actual_output=trace.output or "",
            tools_called=tools_called,
        )


# =============================================================================
# Execution Layer Evaluators
# =============================================================================


class DeepEvalTaskCompletionEvaluator(DeepEvalBaseEvaluator):
    """
    Evaluates whether the agent successfully accomplishes the intended task.

    This is the ultimate measure of agent success—did it do what the user asked?

    Score calculation:
        Task Completion Score = AlignmentScore(Task, Outcome)

    The metric extracts the task (either user-provided or inferred from the trace)
    and the outcome, then uses an LLM to evaluate alignment. A score of 1 means
    complete task fulfillment; lower scores indicate partial or failed completion.
    """

    # Class-level metadata
    name = "deepeval/task-completion"
    description = "Measures whether agent successfully completed the intended task goal"
    tags = ["deepeval", "llm-judge", "execution", "completeness"]
    evaluator_type = "agent"
    version = "1.0"

    # Additional Param descriptor — type inferred from annotation
    custom_task: Optional[str] = Param(
        default=None, description="Optional custom task description (overrides auto-inference)"
    )

    def __init__(self, **kwargs):
        """
        Initialize TaskCompletion evaluator.

        Args:
            threshold: Minimum score for passing (0.0-1.0)
            model: LLM model to use for evaluation
            include_reason: Whether to include reasoning in the result
            strict_mode: If True, use binary scoring (0 or 1)
            custom_task: Optional custom task description (overrides auto-inference)
        """
        super().__init__(**kwargs)

    def _evaluate_with_deepeval(self, trace: Trace, task: Task) -> EvalResult:
        """Evaluate if the agent completed the task."""
        TaskCompletionMetric = _get_deepeval_metric_class("TaskCompletionMetric")

        # Create metric
        metric_kwargs = {
            "threshold": self.threshold,
            "model": self.model,
            "include_reason": self.include_reason,
            "strict_mode": self.strict_mode,
        }

        metric = TaskCompletionMetric(**metric_kwargs)

        # Build test case, using custom_task to override the input if provided
        test_case = self._build_deepeval_test_case(trace, task)
        if self.custom_task:
            try:
                from deepeval.test_case import LLMTestCase
            except ImportError:
                raise ImportError("DeepEval is required. Install with: pip install deepeval")
            test_case = LLMTestCase(
                input=self.custom_task,
                actual_output=test_case.actual_output,
                expected_output=test_case.expected_output,
                retrieval_context=test_case.retrieval_context,
                tools_called=test_case.tools_called,
            )

        # Measure with better error handling
        try:
            metric.measure(test_case)
        except TypeError as te:
            # DeepEval sometimes has internal errors with missing data
            logger.warning(f"TaskCompletion metric measure failed: {te}")
            return EvalResult.skip(
                f"Cannot evaluate: {te}",
                details={"error": str(te)},
            )

        return self._convert_deepeval_result(metric)


class DeepEvalStepEfficiencyEvaluator(DeepEvalBaseEvaluator):
    """
    Evaluates whether the agent completes tasks without unnecessary steps.

    An agent might complete a task but waste tokens, time, and resources on
    redundant or circuitous actions.

    Score calculation:
        Step Efficiency Score = AlignmentScore(Task, Execution Steps)

    The metric extracts the task and all execution steps from the trace, then uses
    an LLM to evaluate efficiency. It penalizes redundant tool calls, unnecessary
    reasoning loops, and any actions not strictly required to complete the task.

    Tip: A high TaskCompletionMetric score with a low StepEfficiencyMetric score
    indicates your agent works but needs optimization.
    """

    # Class-level metadata
    name = "deepeval/step-efficiency"
    description = "Assesses execution efficiency by detecting redundant or unnecessary steps"
    tags = ["deepeval", "llm-judge", "execution", "efficiency"]
    evaluator_type = "agent"
    version = "1.0"

    def _evaluate_with_deepeval(self, trace: Trace, task: Task) -> EvalResult:
        """Evaluate if the agent completed the task efficiently."""
        StepEfficiencyMetric = _get_deepeval_metric_class("StepEfficiencyMetric")

        # Create metric
        metric = StepEfficiencyMetric(
            threshold=self.threshold,
            model=self.model,
            include_reason=self.include_reason,
            strict_mode=self.strict_mode,
        )

        # Build test case
        test_case = self._build_deepeval_test_case(trace, task)

        # Measure with better error handling
        try:
            metric.measure(test_case)
        except (UnboundLocalError, AttributeError, TypeError) as e:
            # DeepEval sometimes has internal errors with step analysis
            logger.warning(f"StepEfficiency metric measure failed: {e}")
            return EvalResult.skip(
                f"Cannot evaluate: {e}",
                details={"error": str(e)},
            )

        return self._convert_deepeval_result(metric)
