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
from amp_evaluation.evaluators.config import Param
from amp_evaluation.models import EvalResult
from amp_evaluation.trace.models import Trace
from amp_evaluation.dataset.models import Task


logger = logging.getLogger(__name__)


# =============================================================================
# Output Quality Evaluators
# =============================================================================


class AnswerLengthEvaluator(BaseEvaluator):
    """Evaluates if the answer length is within acceptable bounds."""

    name = "answer_length"
    description = "Validates that output character length falls within configured bounds"
    tags = ["standard", "rule-based", "final-response", "compliance"]

    min_length: int = Param(default=1, min=0, description="Minimum acceptable length")
    max_length: int = Param(default=10000, min=1, description="Maximum acceptable length")

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        output_length = len(trace.output) if trace.output else 0

        if output_length < self.min_length:
            return EvalResult(
                score=0.0,
                passed=False,
                explanation=f"Output too short: {output_length} < {self.min_length}",
                details={"output_length": output_length},
            )

        if output_length > self.max_length:
            return EvalResult(
                score=0.0,
                passed=False,
                explanation=f"Output too long: {output_length} > {self.max_length}",
                details={"output_length": output_length},
            )

        return EvalResult(
            score=1.0,
            passed=True,
            explanation=f"Output length acceptable: {output_length}",
            details={"output_length": output_length},
        )


class AnswerRelevancyEvaluator(BaseEvaluator):
    """Evaluates if the answer is relevant to the input query."""

    name = "answer_relevancy"
    description = "Measures relevancy between input and output using word overlap analysis"
    tags = ["standard", "rule-based", "final-response", "relevancy"]

    min_overlap_ratio: float = Param(default=0.1, min=0.0, max=1.0, description="Minimum word overlap ratio")

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        input_text = trace.input.lower() if trace.input else ""
        output_text = trace.output.lower() if trace.output else ""

        input_words = set(input_text.split())
        output_words = set(output_text.split())

        if not input_words:
            return EvalResult.skip("No input text to compare")

        overlap = input_words.intersection(output_words)
        overlap_ratio = len(overlap) / len(input_words) if input_words else 0

        passed = overlap_ratio >= self.min_overlap_ratio
        return EvalResult(
            score=overlap_ratio,
            passed=passed,
            explanation=f"Word overlap ratio: {overlap_ratio:.2f}",
            details={"overlap_ratio": overlap_ratio, "overlapping_words": list(overlap)[:10]},
        )


class RequiredContentEvaluator(BaseEvaluator):
    """Evaluates if the output contains all required content."""

    name = "required_content"
    description = "Ensures output contains all required strings and regex pattern matches"
    tags = ["standard", "rule-based", "final-response", "completeness"]

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
        for required in self.required_strings:
            compare_required = required if self.case_sensitive else required.lower()
            if compare_required not in compare_output:
                missing_strings.append(required)

        missing_patterns = []
        for pattern in self.required_patterns:
            flags = 0 if self.case_sensitive else re.IGNORECASE
            if not re.search(pattern, output, flags):
                missing_patterns.append(pattern)

        total_required = len(self.required_strings) + len(self.required_patterns)
        total_missing = len(missing_strings) + len(missing_patterns)

        if total_required == 0:
            return EvalResult(score=1.0, passed=True, explanation="No required content specified", details={})

        score = (total_required - total_missing) / total_required
        passed = total_missing == 0

        return EvalResult(
            score=score,
            passed=passed,
            explanation=f"Found {total_required - total_missing}/{total_required} required items",
            details={"missing_strings": missing_strings, "missing_patterns": missing_patterns},
        )


class ProhibitedContentEvaluator(BaseEvaluator):
    """Evaluates if the output avoids prohibited content."""

    name = "prohibited_content"
    description = "Detects presence of prohibited strings and regex patterns in output"
    tags = ["standard", "rule-based", "final-response", "safety"]

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

        all_prohibited = list(self.prohibited_strings)
        if self.use_context_prohibited and task and task.prohibited_content:
            all_prohibited.extend(task.prohibited_content)

        found_strings = []
        for prohibited in all_prohibited:
            compare_prohibited = prohibited if self.case_sensitive else prohibited.lower()
            if compare_prohibited in compare_output:
                found_strings.append(prohibited)

        found_patterns = []
        for pattern in self.prohibited_patterns:
            flags = 0 if self.case_sensitive else re.IGNORECASE
            if re.search(pattern, output, flags):
                found_patterns.append(pattern)

        total_found = len(found_strings) + len(found_patterns)
        passed = total_found == 0

        return EvalResult(
            score=1.0 if passed else 0.0,
            passed=passed,
            explanation="No prohibited content found" if passed else f"Found {total_found} prohibited items",
            details={"found_strings": found_strings, "found_patterns": found_patterns},
        )


class ExactMatchEvaluator(BaseEvaluator):
    """Evaluates if the output exactly matches the reference output."""

    name = "exact_match"
    description = "Validates output exactly matches the expected output (ground truth)"
    tags = ["standard", "rule-based", "final-response", "accuracy"]

    case_sensitive: bool = Param(default=True, description="Whether to use case-sensitive matching")
    strip_whitespace: bool = Param(default=True, description="Whether to strip whitespace before comparing")

    def evaluate(self, trace: Trace, task: Task) -> EvalResult:
        if task.expected_output is None:
            return EvalResult.skip(
                "Expected output not available for exact match evaluation",
                details={"expected_available": False, "output_available": trace.output is not None},
            )
        expected = task.expected_output

        output = trace.output if trace.output else None
        if not output:
            return EvalResult.skip(
                "Actual output not available for exact match evaluation",
                details={"expected_available": expected is not None, "output_available": False},
            )

        if self.strip_whitespace:
            output = output.strip()
            expected = expected.strip()

        if not self.case_sensitive:
            output = output.lower()
            expected = expected.lower()

        passed = output == expected

        return EvalResult(
            score=1.0 if passed else 0.0,
            passed=passed,
            explanation="Exact match" if passed else "Output does not match expected",
            details={
                "output_preview": output[:100] if output else "",
                "expected_preview": expected[:100] if expected else "",
            },
        )


class ContainsMatchEvaluator(BaseEvaluator):
    """Evaluates if the output contains the reference output."""

    name = "contains_match"
    description = "Validates that expected output substring is present in actual output"
    tags = ["standard", "rule-based", "final-response", "accuracy"]

    case_sensitive: bool = Param(default=False, description="Whether to use case-sensitive matching")

    def evaluate(self, trace: Trace, task: Task) -> EvalResult:
        if task.expected_output is None:
            return EvalResult.skip(
                "Expected output not available for contains match evaluation",
                details={"expected_available": False, "output_available": trace.output is not None},
            )
        expected = task.expected_output

        output = trace.output if trace.output else ""

        compare_output = output if self.case_sensitive else output.lower()
        compare_expected = expected if self.case_sensitive else expected.lower()

        passed = compare_expected in compare_output

        return EvalResult(
            score=1.0 if passed else 0.0,
            passed=passed,
            explanation="Expected found in output" if passed else "Expected not found in output",
            details={"output_length": len(output), "expected_length": len(expected)},
        )


# =============================================================================
# Trajectory Evaluators
# =============================================================================


class ToolSequenceEvaluator(BaseEvaluator):
    """Evaluates if tools were called in the expected sequence."""

    name = "tool_sequence"
    description = "Validates that tools were invoked in the expected sequential order"
    tags = ["standard", "rule-based", "trajectory", "correctness"]

    expected_sequence: Optional[List[str]] = Param(default=None, description="List of tool names in expected order")
    strict: bool = Param(default=False, description="If True, requires exact sequence. If False, allows extra tools")
    use_context_trajectory: bool = Param(default=True, description="If True, uses task.expected_trajectory")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        if self.expected_sequence is None:
            self.expected_sequence = []

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        expected = list(self.expected_sequence)
        if self.use_context_trajectory and task and task.expected_trajectory:
            expected_trajectory = task.expected_trajectory
            expected = [step.tool for step in expected_trajectory if step.tool]

        if not expected:
            return EvalResult.skip(
                "No expected tool sequence specified",
                details={"actual_sequence": [step.name for step in trace.get_tool_calls() if step.name]},
            )

        actual_sequence = [step.name for step in trace.get_tool_calls() if step.name]

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
            explanation=f"Matched {score * 100:.0f}% of expected sequence",
            details={"expected_sequence": expected, "actual_sequence": actual_sequence},
        )


class RequiredToolsEvaluator(BaseEvaluator):
    """Evaluates if all required tools were used."""

    name = "required_tools"
    description = "Ensures all required tools were invoked at least once during execution"
    tags = ["standard", "rule-based", "trajectory", "completeness"]

    required_tools: Optional[Set[str]] = Param(default=None, description="Set of required tool names")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        if self.required_tools is None:
            self.required_tools = set()
        elif not isinstance(self.required_tools, set):
            self.required_tools = set(self.required_tools)

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        required = set(self.required_tools)

        if not required and task and task.expected_trajectory:
            expected_trajectory = task.expected_trajectory
            for step in expected_trajectory:
                if step.tool:
                    required.add(step.tool)

        if not required:
            return EvalResult.skip(
                "No required tools specified",
                details={"used_tools": [step.name for step in trace.get_tool_calls() if step.name]},
            )

        used_tools = {step.name for step in trace.get_tool_calls() if step.name}

        missing_tools = required - used_tools
        found_tools = required.intersection(used_tools)

        score = len(found_tools) / len(required) if required else 1.0
        passed = len(missing_tools) == 0

        return EvalResult(
            score=score,
            passed=passed,
            explanation=f"Used {len(found_tools)}/{len(required)} required tools",
            details={
                "required_tools": list(required),
                "used_tools": list(used_tools),
                "missing_tools": list(missing_tools),
            },
        )


class StepSuccessRateEvaluator(BaseEvaluator):
    """Evaluates the success rate of trace spans."""

    name = "step_success_rate"
    description = "Measures the percentage of execution spans completed without errors"
    tags = ["standard", "rule-based", "trajectory", "reliability"]

    min_success_rate: float = Param(default=0.8, min=0.0, max=1.0, description="Minimum required success rate")

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        if not trace.spans:
            return EvalResult.skip("No spans to evaluate", details={"span_count": 0})

        successful = sum(1 for span in trace.spans if not getattr(getattr(span, "metrics", None), "error", False))
        total = len(trace.spans)
        success_rate = successful / total

        passed = success_rate >= self.min_success_rate

        return EvalResult(
            score=success_rate,
            passed=passed,
            explanation=f"Span success rate: {success_rate:.1%} ({successful}/{total})",
            details={"successful_spans": successful, "total_spans": total, "success_rate": success_rate},
        )


# =============================================================================
# Performance Evaluators
# =============================================================================


class LatencyEvaluator(BaseEvaluator):
    """Evaluates if execution completed within latency constraints (trace-level)."""

    name = "latency"
    description = "Validates execution time meets configured latency constraints"
    tags = ["standard", "rule-based", "performance", "efficiency"]

    max_latency_ms: float = Param(default=30000.0, min=0.0, description="Maximum allowed latency in milliseconds")
    use_task_constraint: bool = Param(default=True, description="Whether to use task.constraints.max_latency_ms")

    def _get_max_latency(self, task: Optional[Task]) -> float:
        max_latency = self.max_latency_ms
        if self.use_task_constraint and task and task.constraints:
            constraints = task.constraints
            if constraints and constraints.max_latency_ms is not None:
                max_latency = constraints.max_latency_ms
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
            details={"actual_latency_ms": actual_latency, "max_latency_ms": max_latency},
        )


class TokenEfficiencyEvaluator(BaseEvaluator):
    """Evaluates if token usage is within constraints."""

    name = "token_efficiency"
    description = "Validates total token consumption stays within configured usage limits"
    tags = ["standard", "rule-based", "performance", "efficiency"]

    max_tokens: int = Param(default=10000, min=1, description="Maximum allowed tokens")
    use_context_constraint: bool = Param(default=True, description="Whether to use task.constraints.max_tokens")

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        max_tokens = self.max_tokens
        if self.use_context_constraint and task and task.constraints:
            constraints = task.constraints
            if constraints and constraints.max_tokens is not None:
                max_tokens = constraints.max_tokens

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
            details={"actual_tokens": actual_tokens, "max_tokens": max_tokens},
        )


class IterationCountEvaluator(BaseEvaluator):
    """Evaluates if the agent completed within iteration constraints."""

    name = "iteration_count"
    description = "Validates agent completes task within maximum iteration count limit"
    tags = ["standard", "rule-based", "performance", "efficiency"]

    max_iterations: int = Param(default=10, min=1, description="Maximum allowed iterations")
    use_context_constraint: bool = Param(default=True, description="Whether to use task.constraints.max_iterations")

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        max_iterations = self.max_iterations
        if self.use_context_constraint and task and task.constraints:
            constraints = task.constraints
            if constraints and constraints.max_iterations is not None:
                max_iterations = constraints.max_iterations

        actual_iterations = len(trace.spans)

        passed = actual_iterations <= max_iterations

        if actual_iterations <= max_iterations:
            score = 1.0
        else:
            score = max(0.0, 1.0 - (actual_iterations - max_iterations) / max_iterations)

        return EvalResult(
            score=score,
            passed=passed,
            explanation=f"Iterations: {actual_iterations} (max: {max_iterations})",
            details={"actual_iterations": actual_iterations, "max_iterations": max_iterations},
        )


# =============================================================================
# Quality Evaluators
# =============================================================================


class HallucinationEvaluator(BaseEvaluator):
    """
    Detects potential hallucinations in AI outputs using keyword-based heuristics (trace-level).

    This is a simple heuristic evaluator. For production use, consider
    LLM-as-judge evaluators for more sophisticated hallucination detection.
    """

    name = "hallucination"
    description = "Detects potential hallucinations using keyword patterns"
    tags = ["standard", "rule-based", "quality", "safety"]

    hallucination_keywords: list = Param(
        default=["I don't have access", "I cannot", "I'm not sure", "I don't know", "unclear", "uncertain"],
        description="Keywords that may indicate uncertainty or hallucination",
    )
    case_sensitive: bool = Param(default=False, description="Whether keyword matching is case-sensitive")

    def _check_for_hallucination(self, text: str) -> tuple:
        if not text:
            return 1.0, True, "No output to check"

        search_text = text if self.case_sensitive else text.lower()
        keywords = (
            self.hallucination_keywords if self.case_sensitive else [k.lower() for k in self.hallucination_keywords]
        )

        found_keywords = []
        for keyword in keywords:
            if keyword in search_text:
                found_keywords.append(keyword)

        if found_keywords:
            score = max(0.0, 1.0 - (len(found_keywords) * 0.3))
            return score, False, f"Found hallucination indicators: {', '.join(found_keywords)}"
        else:
            return 1.0, True, "No hallucination indicators detected"

    def evaluate(self, trace: Trace, task: Optional[Task] = None) -> EvalResult:
        output = trace.output or ""
        score, passed, explanation = self._check_for_hallucination(output)

        return EvalResult(
            score=score,
            passed=passed,
            explanation=f"Trace: {explanation}",
            details={"output_length": len(output), "checked_keywords": self.hallucination_keywords},
        )
