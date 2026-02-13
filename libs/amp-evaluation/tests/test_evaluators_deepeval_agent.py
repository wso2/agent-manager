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
Unit tests for DeepEval agent evaluators.

Tests all evaluators in evaluators/builtin/deepeval/agent.py
Uses real DeepEval library with mocked metric.measure() to verify:
1. Correct construction of DeepEval LLMTestCase from our Observation
2. Proper extraction of scores from DeepEval metrics
3. Integration with actual DeepEval data structures
"""

import pytest
import sys
from pathlib import Path
from datetime import datetime
from unittest.mock import Mock, patch, MagicMock

sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from amp_evaluation.evaluators.builtin.deepeval import (
    DeepEvalPlanQualityEvaluator,
    DeepEvalPlanAdherenceEvaluator,
    DeepEvalToolCorrectnessEvaluator,
    DeepEvalArgumentCorrectnessEvaluator,
    DeepEvalTaskCompletionEvaluator,
    DeepEvalStepEfficiencyEvaluator,
)
from amp_evaluation.models import Observation
from amp_evaluation.dataset import Task
from amp_evaluation.trace import (
    Trajectory,
    TraceMetrics,
    TokenUsage,
    ToolSpan,
)


# ============================================================================
# TEST FIXTURES
# ============================================================================


@pytest.fixture
def basic_observation():
    """Create a basic observation for testing."""
    trajectory = Trajectory(
        trace_id="test-trace-1",
        input="Book the cheapest flight from NYC to Paris",
        output="Flight booked successfully. Confirmation: CONF-789",
        timestamp=datetime.now(),
        metrics=TraceMetrics(
            total_duration_ms=2500.0,
            token_usage=TokenUsage(input_tokens=120, output_tokens=80, total_tokens=200),
        ),
        steps=[],
    )
    return Observation(trajectory=trajectory)


@pytest.fixture
def observation_with_tools():
    """Create an observation with tool calls."""
    tool_span_1 = ToolSpan(
        span_id="tool-1",
        name="search_flights",
        arguments={"origin": "NYC", "destination": "Paris", "date": "2025-03-15"},
        result={"flights": [{"id": "FL123", "price": 450}, {"id": "FL456", "price": 380}]},
    )
    tool_span_2 = ToolSpan(
        span_id="tool-2",
        name="book_flight",
        arguments={"flight_id": "FL456"},
        result={"confirmation": "CONF-789", "status": "confirmed"},
    )

    trajectory = Trajectory(
        trace_id="test-trace-2",
        input="Book the cheapest flight from NYC to Paris for March 15th",
        output="Booked flight FL456 for $380. Confirmation: CONF-789",
        timestamp=datetime.now(),
        metrics=TraceMetrics(
            total_duration_ms=2500.0,
            token_usage=TokenUsage(input_tokens=150, output_tokens=100, total_tokens=250),
        ),
        steps=[tool_span_1, tool_span_2],
    )
    trajectory._tool_spans = [tool_span_1, tool_span_2]
    return Observation(trajectory=trajectory)


@pytest.fixture
def basic_task():
    """Create a basic task for testing."""
    return Task(
        task_id="task-1",
        name="Flight Booking Task",
        description="Book the cheapest flight",
        input="Book the cheapest flight from NYC to Paris",
        expected_output="Flight booked successfully",
        expected_trajectory=[
            {"type": "tool", "name": "search_flights"},
            {"type": "tool", "name": "book_flight"},
        ],
    )


def mock_metric_measure(metric_instance, score, reason):
    """
    Helper to mock a DeepEval metric's measure() method.
    Sets the score and reason on the metric object directly.
    """

    def mock_measure(test_case):
        # Verify test_case is a proper DeepEval LLMTestCase
        from deepeval.test_case import LLMTestCase

        assert isinstance(test_case, LLMTestCase), f"Expected LLMTestCase, got {type(test_case)}"

        # Set score and reason on the metric (what DeepEval does)
        metric_instance.score = score
        metric_instance.reason = reason
        metric_instance.success = score >= getattr(metric_instance, "threshold", 0.5)

    metric_instance.measure = Mock(side_effect=mock_measure)
    return metric_instance


# ============================================================================
# REASONING LAYER EVALUATORS
# ============================================================================


class TestDeepEvalPlanQualityEvaluator:
    """Test DeepEvalPlanQualityEvaluator."""

    def test_evaluator_initialization(self):
        """Test evaluator can be initialized with custom parameters."""
        evaluator = DeepEvalPlanQualityEvaluator(threshold=0.8, model="gpt-4o-mini", strict_mode=True)

        assert evaluator.threshold == 0.8
        assert evaluator.model == "gpt-4o-mini"
        assert evaluator.strict_mode is True
        assert evaluator.name == "deepeval/plan-quality"

    def test_plan_quality_evaluation_success(self, basic_observation):
        """Test successful plan quality evaluation with real DeepEval integration."""
        from deepeval.metrics import PlanQualityMetric

        # Patch the metric class to return a mock we control
        with patch("deepeval.metrics.PlanQualityMetric") as MockMetric:
            # Create a real-ish metric instance
            metric_instance = MagicMock(spec=PlanQualityMetric)
            metric_instance.threshold = 0.7
            metric_instance.model = "gpt-4o"

            # Mock measure() to set score/reason (what real DeepEval does)
            mock_metric_measure(metric_instance, score=0.85, reason="Plan is logical and complete")

            MockMetric.return_value = metric_instance

            # Evaluate
            evaluator = DeepEvalPlanQualityEvaluator(threshold=0.7)
            result = evaluator.evaluate(basic_observation)

            # Verify results
            assert result.score == 0.85
            assert result.passed is True
            assert "logical and complete" in result.explanation
            assert result.details["model"] == "gpt-4o"
            assert result.details["threshold"] == 0.7

            # Verify measure() was called with a proper LLMTestCase
            assert metric_instance.measure.called
            test_case_arg = metric_instance.measure.call_args[0][0]

            # Validate the LLMTestCase was constructed correctly
            from deepeval.test_case import LLMTestCase

            assert isinstance(test_case_arg, LLMTestCase)
            assert test_case_arg.input == basic_observation.input
            assert test_case_arg.actual_output == basic_observation.output

    def test_plan_quality_evaluation_failure(self, basic_observation):
        """Test failed plan quality evaluation."""
        from deepeval.metrics import PlanQualityMetric

        with patch("deepeval.metrics.PlanQualityMetric") as MockMetric:
            metric_instance = MagicMock(spec=PlanQualityMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=0.3, reason="Plan is incomplete")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalPlanQualityEvaluator(threshold=0.7)
            result = evaluator.evaluate(basic_observation)

            assert result.score == 0.3
            assert result.passed is False
            assert "incomplete" in result.explanation

    def test_deepeval_not_installed(self, basic_observation):
        """Test behavior when DeepEval metric import fails."""
        with patch("amp_evaluation.evaluators.builtin.deepeval._get_deepeval_metric_class") as mock_get:
            mock_get.side_effect = ImportError("DeepEval not found")

            evaluator = DeepEvalPlanQualityEvaluator()
            result = evaluator.evaluate(basic_observation)

            assert result.is_error
            assert result.error is not None
            assert "not installed" in result.explanation.lower()


class TestDeepEvalPlanAdherenceEvaluator:
    """Test DeepEvalPlanAdherenceEvaluator."""

    def test_evaluator_initialization(self):
        """Test evaluator initialization."""
        evaluator = DeepEvalPlanAdherenceEvaluator(threshold=0.75)

        assert evaluator.threshold == 0.75
        assert evaluator.name == "deepeval/plan-adherence"

    def test_plan_adherence_high_score(self, basic_observation):
        """Test when agent follows its plan well."""
        from deepeval.metrics import PlanAdherenceMetric

        with patch("deepeval.metrics.PlanAdherenceMetric") as MockMetric:
            metric_instance = MagicMock(spec=PlanAdherenceMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=0.9, reason="Agent closely followed the stated plan")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalPlanAdherenceEvaluator(threshold=0.7)
            result = evaluator.evaluate(basic_observation)

            assert result.score == 0.9
            assert result.passed is True

    def test_plan_adherence_low_score(self, basic_observation):
        """Test when agent deviates from plan."""
        from deepeval.metrics import PlanAdherenceMetric

        with patch("deepeval.metrics.PlanAdherenceMetric") as MockMetric:
            metric_instance = MagicMock(spec=PlanAdherenceMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=0.4, reason="Agent deviated from plan")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalPlanAdherenceEvaluator(threshold=0.7)
            result = evaluator.evaluate(basic_observation)

            assert result.score == 0.4
            assert result.passed is False


# ============================================================================
# ACTION LAYER EVALUATORS
# ============================================================================


class TestDeepEvalToolCorrectnessEvaluator:
    """Test DeepEvalToolCorrectnessEvaluator."""

    def test_evaluator_initialization(self):
        """Test evaluator initialization with custom parameters."""
        evaluator = DeepEvalToolCorrectnessEvaluator(
            threshold=0.8,
            evaluate_input=True,
            evaluate_output=True,
            evaluate_order=True,
        )

        assert evaluator.threshold == 0.8
        assert evaluator.evaluate_input is True
        assert evaluator.evaluate_output is True
        assert evaluator.evaluate_order is True
        assert evaluator.name == "deepeval/tool-correctness"

    def test_correct_tools_selected(self, observation_with_tools):
        """Test when correct tools are selected."""
        from deepeval.metrics import ToolCorrectnessMetric

        with patch("deepeval.metrics.ToolCorrectnessMetric") as MockMetric:
            metric_instance = MagicMock(spec=ToolCorrectnessMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=1.0, reason="All tools correct")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalToolCorrectnessEvaluator(threshold=0.7)
            result = evaluator.evaluate(observation_with_tools)

            assert result.score == 1.0
            assert result.passed is True

    def test_incorrect_tools_selected(self, observation_with_tools):
        """Test when incorrect tools are selected."""
        from deepeval.metrics import ToolCorrectnessMetric

        with patch("deepeval.metrics.ToolCorrectnessMetric") as MockMetric:
            metric_instance = MagicMock(spec=ToolCorrectnessMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=0.5, reason="Some tools incorrect")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalToolCorrectnessEvaluator(threshold=0.7)
            result = evaluator.evaluate(observation_with_tools)

            assert result.score == 0.5
            assert result.passed is False

    def test_with_available_tools_list(self, observation_with_tools):
        """Test evaluation with available tools list."""
        from deepeval.metrics import ToolCorrectnessMetric

        with patch("deepeval.metrics.ToolCorrectnessMetric") as MockMetric:
            metric_instance = MagicMock(spec=ToolCorrectnessMetric)
            metric_instance.threshold = 0.5
            mock_metric_measure(metric_instance, score=0.9, reason="Optimal tool selection")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalToolCorrectnessEvaluator(
                available_tools=["search_flights", "book_flight", "cancel_flight"]
            )
            result = evaluator.evaluate(observation_with_tools)

            assert result.score == 0.9
            assert result.passed is True
        assert result.passed is True


class TestDeepEvalArgumentCorrectnessEvaluator:
    """Test DeepEvalArgumentCorrectnessEvaluator."""

    def test_evaluator_initialization(self):
        """Test evaluator initialization."""
        evaluator = DeepEvalArgumentCorrectnessEvaluator(threshold=0.75, model="gpt-4o-mini")

        assert evaluator.threshold == 0.75
        assert evaluator.model == "gpt-4o-mini"
        assert evaluator.name == "deepeval/argument-correctness"

    def test_correct_arguments(self, observation_with_tools):
        """Test when correct arguments are provided."""
        from deepeval.metrics import ArgumentCorrectnessMetric

        with patch("deepeval.metrics.ArgumentCorrectnessMetric") as MockMetric:
            metric_instance = MagicMock(spec=ArgumentCorrectnessMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=1.0, reason="All arguments correct")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalArgumentCorrectnessEvaluator(threshold=0.7)
            result = evaluator.evaluate(observation_with_tools)

            assert result.score == 1.0
            assert result.passed is True

    def test_incorrect_arguments(self, observation_with_tools):
        """Test when incorrect arguments are provided."""
        from deepeval.metrics import ArgumentCorrectnessMetric

        with patch("deepeval.metrics.ArgumentCorrectnessMetric") as MockMetric:
            metric_instance = MagicMock(spec=ArgumentCorrectnessMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=0.4, reason="Some arguments incorrect")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalArgumentCorrectnessEvaluator(threshold=0.7)
            result = evaluator.evaluate(observation_with_tools)

            assert result.score == 0.4
            assert result.passed is False


# ============================================================================
# EXECUTION LAYER EVALUATORS
# ============================================================================


class TestDeepEvalTaskCompletionEvaluator:
    """Test DeepEvalTaskCompletionEvaluator."""

    def test_evaluator_initialization(self):
        """Test evaluator initialization."""
        evaluator = DeepEvalTaskCompletionEvaluator(threshold=0.8, custom_task="Complete the booking")

        assert evaluator.threshold == 0.8
        assert evaluator.custom_task == "Complete the booking"
        assert evaluator.name == "deepeval/task-completion"

    def test_task_completed(self, basic_observation):
        """Test when task is completed successfully."""
        from deepeval.metrics import TaskCompletionMetric

        with patch("deepeval.metrics.TaskCompletionMetric") as MockMetric:
            metric_instance = MagicMock(spec=TaskCompletionMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=1.0, reason="Task fully completed")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalTaskCompletionEvaluator(threshold=0.7)
            result = evaluator.evaluate(basic_observation)

            assert result.score == 1.0
            assert result.passed is True

    def test_task_partially_completed(self, basic_observation):
        """Test when task is only partially completed."""
        from deepeval.metrics import TaskCompletionMetric

        with patch("deepeval.metrics.TaskCompletionMetric") as MockMetric:
            metric_instance = MagicMock(spec=TaskCompletionMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=0.6, reason="Task partially completed")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalTaskCompletionEvaluator(threshold=0.7)
            result = evaluator.evaluate(basic_observation)

            assert result.score == 0.6
            assert result.passed is False

    def test_task_failed(self, basic_observation):
        """Test when task completely failed."""
        from deepeval.metrics import TaskCompletionMetric

        with patch("deepeval.metrics.TaskCompletionMetric") as MockMetric:
            metric_instance = MagicMock(spec=TaskCompletionMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=0.0, reason="Task failed")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalTaskCompletionEvaluator(threshold=0.7)
            result = evaluator.evaluate(basic_observation)

            assert result.score == 0.0
            assert result.passed is False


class TestDeepEvalStepEfficiencyEvaluator:
    """Test DeepEvalStepEfficiencyEvaluator."""

    def test_evaluator_initialization(self):
        """Test evaluator initialization."""
        evaluator = DeepEvalStepEfficiencyEvaluator(threshold=0.75)

        assert evaluator.threshold == 0.75
        assert evaluator.name == "deepeval/step-efficiency"

    def test_efficient_execution(self, observation_with_tools):
        """Test when execution is efficient (no redundant steps)."""
        from deepeval.metrics import StepEfficiencyMetric

        with patch("deepeval.metrics.StepEfficiencyMetric") as MockMetric:
            metric_instance = MagicMock(spec=StepEfficiencyMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=0.95, reason="Very efficient execution")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalStepEfficiencyEvaluator(threshold=0.7)
            result = evaluator.evaluate(observation_with_tools)

            assert result.score == 0.95
            assert result.passed is True

    def test_inefficient_execution(self, observation_with_tools):
        """Test when execution has redundant steps."""
        from deepeval.metrics import StepEfficiencyMetric

        with patch("deepeval.metrics.StepEfficiencyMetric") as MockMetric:
            metric_instance = MagicMock(spec=StepEfficiencyMetric)
            metric_instance.threshold = 0.7
            mock_metric_measure(metric_instance, score=0.5, reason="Too many redundant steps")
            MockMetric.return_value = metric_instance

            evaluator = DeepEvalStepEfficiencyEvaluator(threshold=0.7)
            result = evaluator.evaluate(observation_with_tools)

            assert result.score == 0.5
            assert result.passed is False


# ============================================================================
# INTEGRATION TESTS
# ============================================================================


class TestDeepEvalEvaluatorsIntegration:
    """Integration tests for DeepEval evaluators."""

    def test_all_evaluators_have_correct_tags(self):
        """Test that all DeepEval evaluators have mandatory tags via class metadata."""
        evaluator_classes = [
            DeepEvalPlanQualityEvaluator,
            DeepEvalPlanAdherenceEvaluator,
            DeepEvalToolCorrectnessEvaluator,
            DeepEvalArgumentCorrectnessEvaluator,
            DeepEvalTaskCompletionEvaluator,
            DeepEvalStepEfficiencyEvaluator,
        ]

        for cls in evaluator_classes:
            instance = cls()
            metadata = instance.get_metadata()
            tags = metadata.get("tags", [])

            assert "deepeval" in tags, f"{cls.__name__} missing 'deepeval' tag"
            assert "llm-judge" in tags, f"{cls.__name__} missing 'llm-judge' tag"

    def test_all_evaluators_registered(self):
        """Test that all DeepEval evaluators are available in builtin registry."""
        from amp_evaluation import list_builtin_evaluators

        builtin_evaluators = list_builtin_evaluators()

        expected_evaluators = [
            "deepeval/plan-quality",
            "deepeval/plan-adherence",
            "deepeval/tool-correctness",
            "deepeval/argument-correctness",
            "deepeval/task-completion",
            "deepeval/step-efficiency",
        ]

        for name in expected_evaluators:
            assert name in builtin_evaluators, f"{name} not in builtin registry"

    def test_evaluators_can_be_loaded_on_demand(self):
        """Test that builtins can be retrieved on-demand without explicit registration."""
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator

        expected_evaluators = [
            "deepeval/plan-quality",
            "deepeval/plan-adherence",
            "deepeval/tool-correctness",
            "deepeval/argument-correctness",
            "deepeval/task-completion",
            "deepeval/step-efficiency",
        ]

        for name in expected_evaluators:
            evaluator = get_builtin_evaluator(name)
            assert evaluator is not None, f"Could not load {name}"
            assert evaluator.name == name

    def test_evaluators_can_be_explicitly_registered(self):
        """Test that evaluators can be explicitly registered for listing/filtering."""
        from amp_evaluation import register_builtin, list_evaluators, list_by_tag

        # Register one evaluator explicitly
        register_builtin("deepeval/plan-quality")

        # Now it should appear in list_evaluators()
        all_evaluators = list_evaluators()
        assert "deepeval/plan-quality" in all_evaluators

        # And in tag-based filtering
        deepeval_evaluators = list_by_tag("deepeval")
        assert "deepeval/plan-quality" in deepeval_evaluators

    @patch("amp_evaluation.evaluators.builtin.deepeval._get_deepeval_metric_class")
    def test_evaluator_handles_none_score(self, mock_get_class, basic_observation):
        """Test that evaluators handle malformed DeepEval responses gracefully."""
        # Create metric that returns None score
        metric_instance = MagicMock()
        metric_instance.score = None
        metric_instance.reason = "No score available"
        metric_instance.measure = MagicMock()

        mock_metric_class = Mock(return_value=metric_instance)
        mock_get_class.return_value = mock_metric_class

        evaluator = DeepEvalTaskCompletionEvaluator()
        result = evaluator.evaluate(basic_observation)

        # Should handle None score gracefully (convert to 0.0)
        assert result.score == 0.0
        assert result.passed is False

    @patch("amp_evaluation.evaluators.builtin.deepeval._get_deepeval_metric_class")
    def test_evaluator_handles_exceptions(self, mock_get_class, basic_observation):
        """Test that evaluators handle exceptions gracefully."""
        mock_get_class.side_effect = Exception("Unexpected error")

        evaluator = DeepEvalPlanQualityEvaluator()
        result = evaluator.evaluate(basic_observation)

        assert result.is_error
        assert result.error is not None
        assert "failed" in result.explanation.lower()
        assert "error" in result.details
