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
Built-in evaluators for common evaluation patterns.

Each evaluator operates at exactly ONE level (determined by its type hint).
Type from annotation, not from Param's first arg.
"""

from __future__ import annotations

import logging
import re
from typing import List, Optional, Set

from amp_evaluation.evaluators.base import BaseEvaluator
from amp_evaluation.evaluators.params import Param
from amp_evaluation.models import EvalResult
from amp_evaluation.trace.models import AgentTrace, Trace
from amp_evaluation.dataset.models import Task


logger = logging.getLogger(__name__)


# =============================================================================
# Output Quality Evaluators
# =============================================================================


class AnswerLengthEvaluator(BaseEvaluator):
    """Evaluates if the answer length is within acceptable bounds."""

    name = "length_compliance"
    description = "Checks if output length is within configured min/max character bounds. 100% = within limits, 0% = outside limits."
    tags = ["rule-based", "quality"]

    min_length: int = Param(default=1, min=0, description="Minimum acceptable length")
    max_length: int = Param(default=10000, min=1, description="Maximum acceptable length")

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        output_length = len(trace.output) if trace.output else 0

        if output_length < self.min_length:
            return EvalResult(
                score=0.0,
                passed=False,
                explanation=f"Output too short: {output_length} < {self.min_length}",
            )

        if output_length > self.max_length:
            return EvalResult(
                score=0.0,
                passed=False,
                explanation=f"Output too long: {output_length} > {self.max_length}",
            )

        return EvalResult(
            score=1.0,
            passed=True,
            explanation=f"Output length acceptable: {output_length}",
        )


class RequiredContentEvaluator(BaseEvaluator):
    """Evaluates if the output contains all required content."""

    name = "content_coverage"
    description = (
        "Measures how many required strings and patterns were found in the output. "
        "Score represents the proportion found (e.g., 75% = 3 of 4 required items present)."
    )
    tags = ["rule-based", "compliance"]

    required_strings: Optional[List[str]] = Param(default=None, description="List of required strings")
    required_patterns: Optional[List[str]] = Param(default=None, description="List of required regex patterns")
    case_sensitive: bool = Param(default=False, description="Whether to use case-sensitive matching")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        if self.required_strings is None:
            self.required_strings = []
        if self.required_patterns is None:
            self.required_patterns = []

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        output = trace.output if trace.output else ""
        compare_output = output if self.case_sensitive else output.lower()

        missing_strings = []
        for required in self.required_strings or []:
            compare_required = required if self.case_sensitive else required.lower()
            if compare_required not in compare_output:
                missing_strings.append(required)

        missing_patterns = []
        for pattern in self.required_patterns or []:
            flags = 0 if self.case_sensitive else re.IGNORECASE
            if not re.search(pattern, output, flags):
                missing_patterns.append(pattern)

        total_required = len(self.required_strings or []) + len(self.required_patterns or [])
        total_missing = len(missing_strings) + len(missing_patterns)

        if total_required == 0:
            return EvalResult(score=1.0, passed=True, explanation="No required content specified")

        score = (total_required - total_missing) / total_required
        passed = total_missing == 0

        missing_info = ""
        if missing_strings:
            missing_info += f" Missing strings: {missing_strings}."
        if missing_patterns:
            missing_info += f" Missing patterns: {missing_patterns}."

        return EvalResult(
            score=score,
            passed=passed,
            explanation=f"Found {total_required - total_missing}/{total_required} required items.{missing_info}",
        )


class ProhibitedContentEvaluator(BaseEvaluator):
    """Evaluates if the output avoids prohibited content."""

    name = "content_safety"
    description = "Checks output for prohibited strings and patterns. 100% = clean (no violations found), 0% = prohibited content detected."
    tags = ["rule-based", "safety", "compliance"]

    prohibited_strings: Optional[List[str]] = Param(default=None, description="List of prohibited strings")
    prohibited_patterns: Optional[List[str]] = Param(default=None, description="List of prohibited regex patterns")
    case_sensitive: bool = Param(default=False, description="Whether to use case-sensitive matching")
    use_context_prohibited: bool = Param(default=True, description="Whether to use task.prohibited_content")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        if self.prohibited_strings is None:
            self.prohibited_strings = []
        if self.prohibited_patterns is None:
            self.prohibited_patterns = []

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        output = trace.output if trace.output else ""
        compare_output = output if self.case_sensitive else output.lower()

        all_prohibited = list(self.prohibited_strings or [])
        if self.use_context_prohibited and task and task.prohibited_content:
            all_prohibited.extend(task.prohibited_content)

        found_strings = []
        for prohibited in all_prohibited:
            compare_prohibited = prohibited if self.case_sensitive else prohibited.lower()
            if compare_prohibited in compare_output:
                found_strings.append(prohibited)

        found_patterns = []
        for pattern in self.prohibited_patterns or []:
            flags = 0 if self.case_sensitive else re.IGNORECASE
            if re.search(pattern, output, flags):
                found_patterns.append(pattern)

        total_found = len(found_strings) + len(found_patterns)
        passed = total_found == 0

        if passed:
            explanation = "No prohibited content found"
        else:
            found_info = []
            if found_strings:
                found_info.append(f"strings: {found_strings}")
            if found_patterns:
                found_info.append(f"patterns: {found_patterns}")
            explanation = f"Found {total_found} prohibited items ({', '.join(found_info)})"

        return EvalResult(
            score=1.0 if passed else 0.0,
            passed=passed,
            explanation=explanation,
        )


class ExactMatchEvaluator(BaseEvaluator):
    """Evaluates if the output exactly matches the reference output."""

    name = "exact_match"
    description = "Compares output against expected output for exact string match. Experiment-only. Scores 1.0 or 0.0."
    tags = ["rule-based", "correctness"]

    case_sensitive: bool = Param(default=True, description="Whether to use case-sensitive matching")
    strip_whitespace: bool = Param(default=True, description="Whether to strip whitespace before comparing")

    def evaluate(self, trace: Trace, task: Task) -> EvalResult:
        if task.expected_output is None:
            return EvalResult.skip(
                "Expected output not available for exact match evaluation",
            )
        expected = task.expected_output

        output = trace.output if trace.output else None
        if not output:
            return EvalResult.skip(
                "Actual output not available for exact match evaluation",
            )

        if self.strip_whitespace:
            output = output.strip()
            expected = expected.strip()

        if not self.case_sensitive:
            output = output.lower()
            expected = expected.lower()

        passed = output == expected

        if passed:
            explanation = "Exact match"
        else:
            explanation = (
                f"Output does not match expected. Output length: {len(output)}, expected length: {len(expected)}"
            )

        return EvalResult(
            score=1.0 if passed else 0.0,
            passed=passed,
            explanation=explanation,
        )


class ContainsMatchEvaluator(BaseEvaluator):
    """Evaluates if the output contains the reference output."""

    name = "contains_match"
    description = (
        "Checks whether expected output appears as a substring in actual output. Experiment-only. Scores 1.0 or 0.0."
    )
    tags = ["rule-based", "correctness"]

    case_sensitive: bool = Param(default=False, description="Whether to use case-sensitive matching")

    def evaluate(self, trace: Trace, task: Task) -> EvalResult:
        if task.expected_output is None:
            return EvalResult.skip(
                "Expected output not available for contains match evaluation",
            )
        expected = task.expected_output

        output = trace.output if trace.output else ""

        compare_output = output if self.case_sensitive else output.lower()
        compare_expected = expected if self.case_sensitive else expected.lower()

        passed = compare_expected in compare_output

        return EvalResult(
            score=1.0 if passed else 0.0,
            passed=passed,
            explanation=(
                f"Expected found in output (output_length={len(output)}, expected_length={len(expected)})"
                if passed
                else f"Expected not found in output (output_length={len(output)}, expected_length={len(expected)})"
            ),
        )


# =============================================================================
# Trajectory Evaluators
# =============================================================================


class ToolSequenceEvaluator(BaseEvaluator):
    """Evaluates if tools were called in the expected sequence."""

    name = "sequence_adherence"
    description = "Measures how closely the actual tool call sequence matches the expected order. Score represents the proportion of the expected sequence matched in order."
    tags = ["rule-based", "tool-use"]

    expected_sequence: Optional[List[str]] = Param(default=None, description="List of tool names in expected order")
    strict: bool = Param(default=False, description="If True, requires exact sequence. If False, allows extra tools")
    use_context_trajectory: bool = Param(default=True, description="If True, uses task.expected_trajectory")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        if self.expected_sequence is None:
            self.expected_sequence = []

    def evaluate(self, agent_trace: AgentTrace, task: Optional[Task] = None) -> EvalResult:
        expected = list(self.expected_sequence or [])
        if self.use_context_trajectory and task and task.expected_trajectory:
            expected_trajectory = task.expected_trajectory
            expected = [step.tool for step in expected_trajectory if step.tool]

        actual_sequence = [step.tool_name for step in agent_trace.get_tool_steps() if step.tool_name]
        if not expected:
            return EvalResult.skip(
                f"No expected tool sequence specified. Actual tools called: {actual_sequence}",
            )

        if self.strict:
            passed = actual_sequence == expected
            score = 1.0 if passed else 0.0
        else:
            expected_idx = 0
            for tool in actual_sequence:
                if expected_idx < len(expected) and tool == expected[expected_idx]:
                    expected_idx += 1

            score = expected_idx / len(expected) if expected else 1.0
            passed = expected_idx == len(expected)

        return EvalResult(
            score=score,
            passed=passed,
            explanation=f"Matched {score * 100:.0f}% of expected sequence. Expected: {expected}, Actual: {actual_sequence}",
        )


class RequiredToolsEvaluator(BaseEvaluator):
    """Evaluates if all required tools were used."""

    name = "tool_coverage"
    description = "Measures how many required tools were invoked at least once. Score represents the proportion of required tools found (e.g., 50% = half of required tools used)."
    tags = ["rule-based", "tool-use"]

    required_tools: Optional[Set[str]] = Param(default=None, description="Set of required tool names")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        if self.required_tools is None:
            self.required_tools = set()
        elif not isinstance(self.required_tools, set):
            self.required_tools = set(self.required_tools)

    def evaluate(self, agent_trace: AgentTrace, task: Optional[Task] = None) -> EvalResult:
        required = set(self.required_tools or set())

        if not required and task and task.expected_trajectory:
            expected_trajectory = task.expected_trajectory
            for step in expected_trajectory:
                if step.tool:
                    required.add(step.tool)

        if not required:
            used_tools_list = [step.tool_name for step in agent_trace.get_tool_steps() if step.tool_name]
            return EvalResult.skip(
                f"No required tools specified. Tools used: {used_tools_list}",
            )

        used_tools = {step.tool_name for step in agent_trace.get_tool_steps() if step.tool_name}

        missing_tools = required - used_tools
        found_tools = required.intersection(used_tools)

        score = len(found_tools) / len(required) if required else 1.0
        passed = len(missing_tools) == 0

        missing_info = f" Missing: {sorted(missing_tools)}" if missing_tools else ""

        return EvalResult(
            score=score,
            passed=passed,
            explanation=f"Used {len(found_tools)}/{len(required)} required tools.{missing_info}",
        )


class StepSuccessRateEvaluator(BaseEvaluator):
    """Evaluates the success rate of agent execution steps (tool calls)."""

    name = "step_success_rate"
    description = (
        "Measures the ratio of tool execution steps completed without errors. Score = successful steps / total steps."
    )
    tags = ["rule-based", "tool-use"]

    min_success_rate: float = Param(default=0.8, min=0.0, max=1.0, description="Minimum required success rate")

    def evaluate(self, agent_trace: AgentTrace, task: Optional[Task] = None) -> EvalResult:
        tool_steps = agent_trace.tool_steps
        if not tool_steps:
            return EvalResult.skip("No tool execution steps to evaluate")

        failed = sum(1 for step in tool_steps if step.error)
        total = len(tool_steps)
        success_rate = (total - failed) / total

        passed = success_rate >= self.min_success_rate

        return EvalResult(
            score=success_rate,
            passed=passed,
            explanation=f"Step success rate: {success_rate:.1%} ({total - failed}/{total})",
        )


# =============================================================================
# Performance Evaluators
# =============================================================================


class LatencyEvaluator(BaseEvaluator):
    """Evaluates if execution completed within latency constraints (trace-level)."""

    name = "latency_performance"
    description = "Scores execution speed against a configurable time limit. 100% = within limit, degrades linearly as latency exceeds the limit."
    tags = ["rule-based", "efficiency"]

    max_latency_ms: float = Param(default=30000.0, min=0.0, description="Maximum allowed latency in milliseconds")
    use_task_constraint: bool = Param(default=True, description="Whether to use task.constraints.max_latency_ms")

    def _get_max_latency(self, task: Optional[Task]) -> float:
        max_latency = self.max_latency_ms
        if self.use_task_constraint and task and task.constraints:
            if task.constraints.max_latency_ms is not None:
                max_latency = task.constraints.max_latency_ms
        return max_latency

    def _calculate_score(self, actual_latency: float, max_latency: float) -> tuple:
        if max_latency <= 0:
            passed = actual_latency <= 0
            return (1.0 if passed else 0.0), passed
        passed = actual_latency <= max_latency
        if actual_latency <= max_latency:
            score = 1.0
        else:
            score = max(0.0, 1.0 - (actual_latency - max_latency) / max_latency)
        return score, passed

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        max_latency = self._get_max_latency(task)
        actual_latency = trace.metrics.total_duration_ms or 0
        score, passed = self._calculate_score(actual_latency, max_latency)

        return EvalResult(
            score=score,
            passed=passed,
            explanation=f"Latency: {actual_latency:.0f}ms (max: {max_latency:.0f}ms)",
        )


class TokenEfficiencyEvaluator(BaseEvaluator):
    """Evaluates if token usage is within constraints."""

    name = "token_efficiency"
    description = (
        "Checks total token usage against a configurable limit. Scores 1.0 within limit, degrades linearly above it."
    )
    tags = ["rule-based", "efficiency"]

    max_tokens: int = Param(default=10000, min=1, description="Maximum allowed tokens")
    use_context_constraint: bool = Param(default=True, description="Whether to use task.constraints.max_tokens")

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        max_tokens = self.max_tokens
        if self.use_context_constraint and task and task.constraints:
            if task.constraints.max_tokens is not None:
                max_tokens = task.constraints.max_tokens

        actual_tokens = trace.metrics.token_usage.total_tokens if trace.metrics.token_usage else 0
        passed = actual_tokens <= max_tokens

        if actual_tokens <= max_tokens:
            score = 1.0
        else:
            score = max(0.0, 1.0 - (actual_tokens - max_tokens) / max_tokens)

        return EvalResult(
            score=score,
            passed=passed,
            explanation=f"Tokens: {actual_tokens} (max: {max_tokens})",
        )


class IterationCountEvaluator(BaseEvaluator):
    """Evaluates if the agent completed within iteration constraints."""

    name = "iteration_efficiency"
    description = "Scores whether the agent completed within iteration limits (measured by LLM call count). 100% = within limit, degrades linearly as iterations exceed the limit."
    tags = ["rule-based", "efficiency"]

    max_iterations: int = Param(default=10, min=1, description="Maximum allowed iterations")
    use_context_constraint: bool = Param(default=True, description="Whether to use task.constraints.max_iterations")

    def evaluate(self, agent_trace: AgentTrace, task: Optional[Task] = None) -> EvalResult:
        max_iterations = self.max_iterations
        if self.use_context_constraint and task and task.constraints:
            if task.constraints.max_iterations is not None:
                max_iterations = task.constraints.max_iterations

        actual_iterations = len(agent_trace.llm_steps)

        passed = actual_iterations <= max_iterations

        if actual_iterations <= max_iterations:
            score = 1.0
        else:
            score = max(0.0, 1.0 - (actual_iterations - max_iterations) / max_iterations)

        return EvalResult(
            score=score,
            passed=passed,
            explanation=f"Iterations: {actual_iterations} (max: {max_iterations})",
        )
