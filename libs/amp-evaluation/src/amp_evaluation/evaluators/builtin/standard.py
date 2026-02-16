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

All evaluators use the two-parameter interface:
    evaluate(observation: Observation, task: Optional[Task] = None) -> EvalResult

- observation: What the agent did (always available)
- task: What it should have done (only for experiments with datasets)
"""

from __future__ import annotations

import logging
import re
from typing import Optional, TYPE_CHECKING

from amp_evaluation.evaluators.base import BaseEvaluator
from amp_evaluation.evaluators.config import Param
from amp_evaluation.models import Observation, EvalResult

if TYPE_CHECKING:
    from amp_evaluation.dataset import Task


logger = logging.getLogger(__name__)


# =============================================================================
# Output Quality Evaluators
# =============================================================================


class AnswerLengthEvaluator(BaseEvaluator):
    """Evaluates if the answer length is within acceptable bounds."""

    name = "answer_length"
    description = "Validates that output character length falls within configured bounds"
    tags = ["standard", "rule-based", "final-response", "compliance"]

    # Declarative configuration using Param descriptors
    min_length = Param(int, default=1, min=0, description="Minimum acceptable length")
    max_length = Param(int, default=10000, min=1, description="Maximum acceptable length")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        output_length = len(observation.output) if observation.output else 0

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

    min_overlap_ratio = Param(float, default=0.1, min=0.0, max=1.0, description="Minimum word overlap ratio")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        input_text = observation.input.lower() if observation.input else ""
        output_text = observation.output.lower() if observation.output else ""

        # Simple word overlap relevancy check
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

    required_strings = Param(list, default=None, description="List of required strings")
    required_patterns = Param(list, default=None, description="List of required regex patterns")
    case_sensitive = Param(bool, default=False, description="Whether to use case-sensitive matching")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        # Ensure lists are initialized
        if self.required_strings is None:
            self.required_strings = []
        if self.required_patterns is None:
            self.required_patterns = []

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        output = observation.output if observation.output else ""
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

    prohibited_strings = Param(list, default=None, description="List of prohibited strings")
    prohibited_patterns = Param(list, default=None, description="List of prohibited regex patterns")
    case_sensitive = Param(bool, default=False, description="Whether to use case-sensitive matching")
    use_context_prohibited = Param(bool, default=True, description="Whether to use task.prohibited_content")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        # Ensure lists are initialized
        if self.prohibited_strings is None:
            self.prohibited_strings = []
        if self.prohibited_patterns is None:
            self.prohibited_patterns = []

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        output = observation.output if observation.output else ""
        compare_output = output if self.case_sensitive else output.lower()

        # Combine explicit and context prohibited content
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

    case_sensitive = Param(bool, default=True, description="Whether to use case-sensitive matching")
    strip_whitespace = Param(bool, default=True, description="Whether to strip whitespace before comparing")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        # Get expected output from context (raises DataNotAvailableError if not available)

        if task is None or task.expected_output is None:
            return EvalResult.skip(
                "Expected output not available for exact match evaluation",
                details={"expected_available": False, "output_available": observation.output is not None},
            )
        expected = task.expected_output

        output = observation.output if observation.output else None
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

    case_sensitive = Param(bool, default=False, description="Whether to use case-sensitive matching")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        expected = task.expected_output

        output = observation.output if observation.output else ""

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

    expected_sequence = Param(list, default=None, description="List of tool names in expected order")
    strict = Param(bool, default=False, description="If True, requires exact sequence. If False, allows extra tools")
    use_context_trajectory = Param(bool, default=True, description="If True, uses task.expected_trajectory")

    def __init__(self, **kwargs):
        """
        Args:
            expected_sequence: List of tool names in expected order
            strict: If True, requires exact sequence. If False, allows extra tools
            use_context_trajectory: If True, uses task.expected_trajectory
        """
        super().__init__(**kwargs)
        # Ensure list is initialized
        if self.expected_sequence is None:
            self.expected_sequence = []

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        trajectory = observation.trajectory

        # Get expected sequence
        expected = list(self.expected_sequence)
        if self.use_context_trajectory and task and task.expected_trajectory:
            # Expected trajectory is list of TrajectoryStep objects
            expected_trajectory = task.expected_trajectory
            expected = [step.tool for step in expected_trajectory if step.tool]

        if not expected:
            return EvalResult.skip(
                "No expected tool sequence specified",
                details={"actual_sequence": [step.name for step in trajectory.tool_spans if step.name]},
            )

        # Extract actual tool sequence
        actual_sequence = [step.name for step in trajectory.tool_spans if step.name]

        if self.strict:
            # Exact match
            passed = actual_sequence == expected
            score = 1.0 if passed else 0.0
        else:
            # Check if expected sequence is a subsequence
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

    required_tools = Param(set, default=None, description="Set of required tool names")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)
        # Ensure set is initialized
        if self.required_tools is None:
            self.required_tools = set()
        elif not isinstance(self.required_tools, set):
            self.required_tools = set(self.required_tools)

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        trajectory = observation.trajectory

        required = set(self.required_tools)

        # Also check context for expected trajectory tools
        if not required and task and task.expected_trajectory:
            expected_trajectory = task.expected_trajectory
            for step in expected_trajectory:
                if step.tool:
                    required.add(step.tool)

        if not required:
            return EvalResult.skip(
                "No required tools specified",
                details={"used_tools": [step.name for step in trajectory.tool_spans if step.name]},
            )

        # Get actually used tools
        used_tools = {step.name for step in trajectory.tool_spans if step.name}

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
    """Evaluates the success rate of trajectory steps."""

    name = "step_success_rate"
    description = "Measures the percentage of execution steps completed without errors"
    tags = ["standard", "rule-based", "trajectory", "reliability"]

    min_success_rate = Param(float, default=0.8, min=0.0, max=1.0, description="Minimum required success rate")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        trajectory = observation.trajectory

        if not trajectory.steps:
            return EvalResult.skip("No steps to evaluate", details={"step_count": 0})

        successful_steps = sum(1 for step in trajectory.steps if not step.error)
        total_steps = len(trajectory.steps)
        success_rate = successful_steps / total_steps

        passed = success_rate >= self.min_success_rate

        return EvalResult(
            score=success_rate,
            passed=passed,
            explanation=f"Step success rate: {success_rate:.1%} ({successful_steps}/{total_steps})",
            details={"successful_steps": successful_steps, "total_steps": total_steps, "success_rate": success_rate},
        )


# =============================================================================
# Performance Evaluators
# =============================================================================


class LatencyEvaluator(BaseEvaluator):
    """Evaluates if the trace completed within latency constraints."""

    name = "latency"
    description = "Validates total execution time meets configured latency constraints"
    tags = ["standard", "rule-based", "performance", "efficiency"]

    max_latency_ms = Param(float, default=None, min=0.0, description="Maximum allowed latency in milliseconds")
    use_task_constraint = Param(bool, default=True, description="Whether to use task.constraints.max_latency_ms")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        # Determine max latency
        max_latency = self.max_latency_ms
        if self.use_task_constraint and task and task.constraints:
            constraints = task.constraints
            if constraints and constraints.has_latency_constraint():
                max_latency = constraints.max_latency_ms

        if max_latency is None:
            return EvalResult.skip(
                "No latency constraint specified", details={"actual_latency_ms": observation.metrics.total_duration_ms}
            )

        actual_latency = observation.metrics.total_duration_ms or 0
        passed = actual_latency <= max_latency

        # Score: 1.0 if within limit, decreasing as we exceed
        if actual_latency <= max_latency:
            score = 1.0
        else:
            # Linear decrease, 0 at 2x the limit
            score = max(0.0, 1.0 - (actual_latency - max_latency) / max_latency)

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

    max_tokens = Param(int, default=None, min=1, description="Maximum allowed tokens")
    use_context_constraint = Param(bool, default=True, description="Whether to use task.constraints.max_tokens")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        # Determine max tokens
        max_tokens = self.max_tokens
        if self.use_context_constraint and task and task.constraints:
            constraints = task.constraints
            if constraints and constraints.has_token_constraint():
                max_tokens = constraints.max_tokens

        if max_tokens is None:
            return EvalResult.skip(
                "No token constraint specified",
                details={
                    "actual_tokens": observation.metrics.token_usage.total_tokens
                    if observation.metrics.token_usage
                    else 0
                },
            )

        actual_tokens = observation.metrics.token_usage.total_tokens if observation.metrics.token_usage else 0
        passed = actual_tokens <= max_tokens

        # Score: 1.0 if within limit, decreasing as we exceed
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

    max_iterations = Param(int, default=None, min=1, description="Maximum allowed iterations")
    use_context_constraint = Param(bool, default=True, description="Whether to use task.constraints.max_iterations")

    def __init__(self, **kwargs):
        super().__init__(**kwargs)

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        trajectory = observation.trajectory

        # Determine max iterations
        max_iterations = self.max_iterations
        if self.use_context_constraint and task and task.constraints:
            constraints = task.constraints
            if constraints and constraints.has_iteration_constraint():
                max_iterations = constraints.max_iterations

        actual_iterations = len(trajectory.steps)

        if max_iterations is None:
            return EvalResult.skip(
                "No iteration constraint specified", details={"actual_iterations": actual_iterations}
            )

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
