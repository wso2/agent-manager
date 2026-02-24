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
Unit tests for core built-in evaluators.

Tests all evaluators in evaluators/builtin/core.py
"""

import pytest
import sys
from pathlib import Path
from datetime import datetime

sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from amp_evaluation.evaluators.builtin.standard import (
    AnswerLengthEvaluator,
    RequiredContentEvaluator,
    ProhibitedContentEvaluator,
    ExactMatchEvaluator,
    ContainsMatchEvaluator,
    ToolSequenceEvaluator,
    RequiredToolsEvaluator,
    StepSuccessRateEvaluator,
    LatencyEvaluator,
    TokenEfficiencyEvaluator,
    IterationCountEvaluator,
)
from amp_evaluation.dataset import Task
from amp_evaluation.trace import (
    Trace,
    TraceMetrics,
    TokenUsage,
    ToolSpan,
)
from amp_evaluation.trace.models import ToolMetrics


# ============================================================================
# TEST FIXTURES
# ============================================================================


@pytest.fixture
def basic_trajectory():
    """Create a basic observation for testing."""
    trajectory = Trace(
        trace_id="test-trace-1",
        input="What is the capital of France?",
        output="The capital of France is Paris.",
        timestamp=datetime.now(),
        metrics=TraceMetrics(
            total_duration_ms=1500.0,
            token_usage=TokenUsage(input_tokens=100, output_tokens=50, total_tokens=150),
        ),
        spans=[],
    )
    return trajectory


@pytest.fixture
def trajectory_with_tools():
    """Create an observation with tool calls."""
    tool_span_1 = ToolSpan(
        span_id="tool-1",
        name="search_flights",
        arguments={"origin": "NYC", "destination": "Paris"},
        result={"flights": [{"id": "FL123", "price": 450}]},
    )
    tool_span_2 = ToolSpan(
        span_id="tool-2",
        name="book_flight",
        arguments={"flight_id": "FL123"},
        result={"confirmation": "CONF-789"},
    )

    trajectory = Trace(
        trace_id="test-trace-2",
        input="Book a flight from NYC to Paris",
        output="Flight booked successfully. Confirmation: CONF-789",
        timestamp=datetime.now(),
        metrics=TraceMetrics(
            total_duration_ms=2000.0,
            token_usage=TokenUsage(input_tokens=120, output_tokens=80, total_tokens=200),
        ),
        spans=[tool_span_1, tool_span_2],
    )
    return trajectory


@pytest.fixture
def basic_task():
    """Create a basic task for testing."""
    return Task(
        task_id="task-1",
        name="Test Task",
        description="A simple test task",
        input="What is the capital of France?",
        expected_output="Paris",
        prohibited_content=["London", "Berlin"],
    )


# ============================================================================
# OUTPUT QUALITY EVALUATORS
# ============================================================================


class TestAnswerLengthEvaluator:
    """Test AnswerLengthEvaluator."""

    def test_answer_within_bounds(self, basic_trajectory):
        """Test when answer length is within acceptable bounds."""
        evaluator = AnswerLengthEvaluator(min_length=10, max_length=100)
        result = evaluator.evaluate(basic_trajectory)

        assert result.score == 1.0
        assert result.passed is True
        assert "acceptable" in result.explanation.lower()

    def test_answer_too_short(self, basic_trajectory):
        """Test when answer is too short."""
        evaluator = AnswerLengthEvaluator(min_length=100, max_length=1000)
        result = evaluator.evaluate(basic_trajectory)

        assert result.score == 0.0
        assert result.passed is False
        assert "too short" in result.explanation.lower()

    def test_answer_too_long(self, basic_trajectory):
        """Test when answer is too long."""
        evaluator = AnswerLengthEvaluator(min_length=1, max_length=10)
        result = evaluator.evaluate(basic_trajectory)

        assert result.score == 0.0
        assert result.passed is False
        assert "too long" in result.explanation.lower()

    def test_empty_output(self):
        """Test with empty output."""
        trajectory = Trace(
            trace_id="test",
            input="test",
            output="",
            timestamp=datetime.now(),
            metrics=TraceMetrics(),
            spans=[],
        )

        evaluator = AnswerLengthEvaluator(min_length=1, max_length=100)
        result = evaluator.evaluate(trajectory)

        assert result.score == 0.0
        assert result.passed is False


class TestRequiredContentEvaluator:
    """Test RequiredContentEvaluator."""

    def test_all_required_strings_present(self, basic_trajectory):
        """Test when all required strings are present."""
        evaluator = RequiredContentEvaluator(required_strings=["Paris", "France"], case_sensitive=False)
        result = evaluator.evaluate(basic_trajectory)

        assert result.score == 1.0
        assert result.passed is True

    def test_missing_required_strings(self, basic_trajectory):
        """Test when some required strings are missing."""
        evaluator = RequiredContentEvaluator(required_strings=["Paris", "London", "Berlin"], case_sensitive=False)
        result = evaluator.evaluate(basic_trajectory)

        assert result.score < 1.0
        assert result.passed is False

    def test_required_patterns(self, basic_trajectory):
        """Test with regex patterns."""
        evaluator = RequiredContentEvaluator(required_patterns=[r"\bParis\b", r"\bcapital\b"], case_sensitive=False)
        result = evaluator.evaluate(basic_trajectory)

        assert result.score == 1.0
        assert result.passed is True


class TestProhibitedContentEvaluator:
    """Test ProhibitedContentEvaluator."""

    def test_no_prohibited_content(self, basic_trajectory):
        """Test when no prohibited content is found."""
        evaluator = ProhibitedContentEvaluator(prohibited_strings=["London", "Berlin"], case_sensitive=False)
        result = evaluator.evaluate(basic_trajectory)

        assert result.score == 1.0
        assert result.passed is True

    def test_prohibited_content_found(self, basic_trajectory):
        """Test when prohibited content is found."""
        evaluator = ProhibitedContentEvaluator(prohibited_strings=["Paris"], case_sensitive=False)
        result = evaluator.evaluate(basic_trajectory)

        assert result.score == 0.0
        assert result.passed is False

    def test_prohibited_from_task_context(self, basic_trajectory, basic_task):
        """Test using prohibited content from task."""
        evaluator = ProhibitedContentEvaluator(use_context_prohibited=True)
        result = evaluator.evaluate(basic_trajectory, basic_task)

        # Paris is in output, but London and Berlin (from task) are not
        assert result.score == 1.0
        assert result.passed is True


class TestExactMatchEvaluator:
    """Test ExactMatchEvaluator."""

    def test_exact_match(self):
        """Test when output exactly matches expected."""
        trajectory = Trace(
            trace_id="test",
            input="What is 2+2?",
            output="4",
            timestamp=datetime.now(),
            metrics=TraceMetrics(),
            spans=[],
        )
        task = Task(
            task_id="task-1",
            name="Test",
            description="Test",
            input="What is 2+2?",
            expected_output="4",
        )

        evaluator = ExactMatchEvaluator()
        result = evaluator.evaluate(trajectory, task)

        assert result.score == 1.0
        assert result.passed is True

    def test_no_match(self, basic_trajectory, basic_task):
        """Test when output doesn't match expected."""
        evaluator = ExactMatchEvaluator()
        result = evaluator.evaluate(basic_trajectory, basic_task)

        assert result.score == 0.0
        assert result.passed is False

    def test_case_insensitive_match(self):
        """Test case-insensitive matching."""
        trajectory = Trace(
            trace_id="test",
            input="test",
            output="PARIS",
            timestamp=datetime.now(),
            metrics=TraceMetrics(),
            spans=[],
        )
        task = Task(
            task_id="task-1",
            name="Test",
            description="Test",
            input="test",
            expected_output="paris",
        )

        evaluator = ExactMatchEvaluator(case_sensitive=False)
        result = evaluator.evaluate(trajectory, task)

        assert result.score == 1.0
        assert result.passed is True


class TestContainsMatchEvaluator:
    """Test ContainsMatchEvaluator."""

    def test_contains_match(self, basic_trajectory, basic_task):
        """Test when output contains expected."""
        evaluator = ContainsMatchEvaluator()
        result = evaluator.evaluate(basic_trajectory, basic_task)

        assert result.score == 1.0
        assert result.passed is True

    def test_no_contains_match(self):
        """Test when output doesn't contain expected."""
        trajectory = Trace(
            trace_id="test",
            input="test",
            output="The answer is London",
            timestamp=datetime.now(),
            metrics=TraceMetrics(),
            spans=[],
        )
        task = Task(
            task_id="task-1",
            name="Test",
            description="Test",
            input="test",
            expected_output="Paris",
        )

        evaluator = ContainsMatchEvaluator()
        result = evaluator.evaluate(trajectory, task)

        assert result.score == 0.0
        assert result.passed is False


# ============================================================================
# TRAJECTORY EVALUATORS
# ============================================================================


class TestToolSequenceEvaluator:
    """Test ToolSequenceEvaluator."""

    def test_correct_sequence(self, trajectory_with_tools):
        """Test when tools are called in correct sequence."""
        evaluator = ToolSequenceEvaluator(expected_sequence=["search_flights", "book_flight"], strict=True)
        result = evaluator.evaluate(trajectory_with_tools)

        assert result.score == 1.0
        assert result.passed is True

    def test_wrong_sequence(self, trajectory_with_tools):
        """Test when tools are called in wrong sequence."""
        evaluator = ToolSequenceEvaluator(expected_sequence=["book_flight", "search_flights"], strict=True)
        result = evaluator.evaluate(trajectory_with_tools)

        assert result.score < 1.0
        assert result.passed is False

    def test_partial_sequence_non_strict(self, trajectory_with_tools):
        """Test partial sequence in non-strict mode."""
        evaluator = ToolSequenceEvaluator(expected_sequence=["search_flights"], strict=False)
        result = evaluator.evaluate(trajectory_with_tools)

        assert result.score > 0.0
        assert result.passed is True


class TestRequiredToolsEvaluator:
    """Test RequiredToolsEvaluator."""

    def test_all_required_tools_called(self, trajectory_with_tools):
        """Test when all required tools are called."""
        evaluator = RequiredToolsEvaluator(required_tools=["search_flights", "book_flight"])
        result = evaluator.evaluate(trajectory_with_tools)

        assert result.score == 1.0
        assert result.passed is True

    def test_missing_required_tools(self, trajectory_with_tools):
        """Test when some required tools are missing."""
        evaluator = RequiredToolsEvaluator(required_tools=["search_flights", "book_flight", "cancel_flight"])
        result = evaluator.evaluate(trajectory_with_tools)

        assert result.score < 1.0
        assert result.passed is False


class TestStepSuccessRateEvaluator:
    """Test StepSuccessRateEvaluator."""

    def test_all_steps_successful(self, trajectory_with_tools):
        """Test when all steps are successful."""
        evaluator = StepSuccessRateEvaluator(min_success_rate=0.8)
        result = evaluator.evaluate(trajectory_with_tools)

        # All steps are successful (no error field)
        assert result.score == 1.0
        assert result.passed is True

    def test_some_steps_failed(self):
        """Test when some steps have errors."""
        tool_span_1 = ToolSpan(
            span_id="tool-1",
            name="search_flights",
            arguments={},
            result={"flights": []},
        )
        tool_span_2 = ToolSpan(
            span_id="tool-2",
            name="book_flight",
            arguments={},
            result=None,
        )
        # Set error on span 2 - create ToolMetrics with error set
        tool_span_2.metrics = ToolMetrics(error=True)

        trajectory = Trace(
            trace_id="test",
            input="test",
            output="test",
            timestamp=datetime.now(),
            metrics=TraceMetrics(),
            spans=[tool_span_1, tool_span_2],
        )

        evaluator = StepSuccessRateEvaluator(min_success_rate=0.8)
        result = evaluator.evaluate(trajectory)

        assert result.score == 0.5  # 1 out of 2 successful
        assert result.passed is False


# ============================================================================
# PERFORMANCE EVALUATORS
# ============================================================================


class TestLatencyEvaluator:
    """Test LatencyEvaluator."""

    def test_latency_within_limit(self, basic_trajectory):
        """Test when latency is within acceptable limit."""
        evaluator = LatencyEvaluator(max_latency_ms=2000.0)
        result = evaluator.evaluate(basic_trajectory)

        assert result.score == 1.0
        assert result.passed is True

    def test_latency_exceeds_limit(self, basic_trajectory):
        """Test when latency exceeds limit."""
        evaluator = LatencyEvaluator(max_latency_ms=1000.0)
        result = evaluator.evaluate(basic_trajectory)

        # basic_trajectory has 1500ms latency
        # Score decreases linearly: 1.0 - (1500-1000)/1000 = 0.5
        assert result.score == 0.5
        assert result.passed is False


class TestTokenEfficiencyEvaluator:
    """Test TokenEfficiencyEvaluator."""

    def test_efficient_token_usage(self, basic_trajectory):
        """Test when token usage is efficient."""
        # basic_trajectory uses 150 tokens
        evaluator = TokenEfficiencyEvaluator(max_tokens=200)
        result = evaluator.evaluate(basic_trajectory)

        assert result.passed is True
        assert result.score == 1.0

    def test_inefficient_token_usage(self, basic_trajectory):
        """Test when token usage is inefficient."""
        # basic_trajectory uses 150 tokens, set limit to 100
        evaluator = TokenEfficiencyEvaluator(max_tokens=100)
        result = evaluator.evaluate(basic_trajectory)

        assert result.passed is False
        # Score: 1.0 - (150-100)/100 = 0.5
        assert result.score == 0.5


class TestIterationCountEvaluator:
    """Test IterationCountEvaluator."""

    def test_within_max_iterations(self, trajectory_with_tools):
        """Test when iteration count is within max."""
        evaluator = IterationCountEvaluator(max_iterations=5)
        result = evaluator.evaluate(trajectory_with_tools)

        # 2 tool calls = 2 iterations
        assert result.score == 1.0
        assert result.passed is True

    def test_exceeds_max_iterations(self, trajectory_with_tools):
        """Test when iteration count exceeds max."""
        evaluator = IterationCountEvaluator(max_iterations=1)
        result = evaluator.evaluate(trajectory_with_tools)

        # 2 tool calls > 1 max
        assert result.score == 0.0
        assert result.passed is False
