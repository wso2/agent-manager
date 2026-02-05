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
Tests for the evaluation runners (Experiment and Monitor).

Tests:
1. Default behavior - runs all registered evaluators
2. Tag-based filtering
3. Name-based filtering
4. Aggregation of results
"""

import pytest
import sys
import os
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from amp_evaluation import evaluator, get_registry
from amp_evaluation.runner import Monitor, RunResult
from amp_evaluation.trace import Trajectory, TraceMetrics, TokenUsage


# ============================================================================
# TEST FIXTURES
# ============================================================================


@pytest.fixture(autouse=True)
def test_env_vars():
    """Set minimal required environment variables for tests."""
    # Store original values
    original_env = {}
    required_vars = {
        "AGENT_UID": "test-agent",
        "ENVIRONMENT_UID": "test-env",
        "PUBLISH_RESULTS": "false",  # Disable platform publishing
        "TRACE_LOADER_MODE": "file",  # Use file mode for tests
        "TRACE_FILE_PATH": "/tmp/test_traces.json",
    }

    for key, value in required_vars.items():
        if key in os.environ:
            original_env[key] = os.environ[key]
        os.environ[key] = value

    yield

    # Restore original values
    for key in required_vars:
        if key in original_env:
            os.environ[key] = original_env[key]
        else:
            del os.environ[key]


@pytest.fixture(autouse=True)
def clean_registry():
    """Clean registry before each test."""
    registry = get_registry()
    # Store original evaluators
    original = registry._evaluators.copy()
    original_meta = registry._metadata.copy()

    # Clear for test
    registry._evaluators.clear()
    registry._metadata.clear()

    yield registry

    # Restore after test
    registry._evaluators.clear()
    registry._metadata.clear()
    registry._evaluators.update(original)
    registry._metadata.update(original_meta)


@pytest.fixture
def sample_traces():
    """Create sample Trajectory objects for testing."""
    return [
        Trajectory(
            trace_id="trace_1",
            input="What is 2+2?",
            output="4",
            metrics=TraceMetrics(
                total_duration_ms=100.0, total_token_usage=TokenUsage(total_tokens=50), llm_call_count=1
            ),
        ),
        Trajectory(
            trace_id="trace_2",
            input="Hello",
            output="Hi there!",
            metrics=TraceMetrics(
                total_duration_ms=200.0, total_token_usage=TokenUsage(total_tokens=30), llm_call_count=1
            ),
        ),
        Trajectory(
            trace_id="trace_3",
            input="Bad input",
            output="",  # Empty output
            metrics=TraceMetrics(
                total_duration_ms=500.0, total_token_usage=TokenUsage(total_tokens=100), llm_call_count=2, error_count=1
            ),
        ),
    ]


# ============================================================================
# TESTS: BASIC RUNNER FUNCTIONALITY
# ============================================================================


class TestMonitorBasic:
    """Test basic EvalRunner functionality."""

    def test_empty_runner_no_evaluators(self, clean_registry):
        """Runner with no registered evaluators has empty list."""
        runner = Monitor()
        assert runner.evaluator_count == 0
        assert runner.evaluator_names == []

    def test_runs_all_registered_evaluators(self, clean_registry):
        """Runner runs all registered evaluators by default."""

        # Register some evaluators
        @evaluator(name="eval_a")
        def eval_a(context):
            return 1.0

        @evaluator(name="eval_b")
        def eval_b(context):
            return 0.5

        runner = Monitor()
        assert runner.evaluator_count == 2
        assert set(runner.evaluator_names) == {"eval_a", "eval_b"}

    def test_run_returns_result(self, clean_registry, sample_traces):
        """Run returns a RunResult with proper structure."""

        @evaluator(name="simple_eval")
        def simple_eval(observation):
            return 1.0 if observation.output else 0.0

        runner = Monitor()
        result = runner.run(traces=sample_traces)

        assert isinstance(result, RunResult)
        assert result.traces_evaluated == 3
        assert result.evaluators_run == 1
        assert result.success
        assert "simple_eval" in result.scores


# ============================================================================
# TESTS: TAG-BASED FILTERING
# ============================================================================


class TestMonitorTagFiltering:
    """Test tag-based filtering of evaluators."""

    def test_include_tags_filters_evaluators(self, clean_registry):
        """include_tags only runs evaluators with those tags."""

        @evaluator(name="quality_eval", tags=["quality"])
        def quality_eval(context):
            return 1.0

        @evaluator(name="perf_eval", tags=["performance"])
        def perf_eval(context):
            return 0.8

        @evaluator(name="both_eval", tags=["quality", "performance"])
        def both_eval(context):
            return 0.9

        # Only quality tagged
        runner = Monitor(include_tags=["quality"])
        assert set(runner.evaluator_names) == {"quality_eval", "both_eval"}

        # Only performance tagged
        runner = Monitor(include_tags=["performance"])
        assert set(runner.evaluator_names) == {"perf_eval", "both_eval"}

    def test_exclude_tags_filters_evaluators(self, clean_registry):
        """exclude_tags removes evaluators with those tags."""

        @evaluator(name="fast_eval", tags=["fast"])
        def fast_eval(context):
            return 1.0

        @evaluator(name="llm_judge", tags=["quality", "llm-judge"])
        def llm_judge(context):
            return 0.5

        @evaluator(name="simple_eval", tags=["quality"])
        def simple_eval(context):
            return 0.7

        # Exclude LLM judges
        runner = Monitor(exclude_tags=["llm-judge"])
        assert set(runner.evaluator_names) == {"fast_eval", "simple_eval"}

    def test_include_and_exclude_tags_together(self, clean_registry):
        """Can use both include_tags and exclude_tags."""

        @evaluator(name="fast_quality", tags=["quality", "fast"])
        def fast_quality(context):
            return 1.0

        @evaluator(name="llm_quality", tags=["quality", "llm-judge"])
        def llm_quality(context):
            return 0.5

        @evaluator(name="perf_eval", tags=["performance"])
        def perf_eval(context):
            return 0.8

        # Quality but not LLM judge
        runner = Monitor(include_tags=["quality"], exclude_tags=["llm-judge"])
        assert runner.evaluator_names == ["fast_quality"]


# ============================================================================
# TESTS: NAME-BASED FILTERING
# ============================================================================


class TestMonitorNameFiltering:
    """Test name-based filtering of evaluators."""

    def test_include_by_name(self, clean_registry):
        """include parameter specifies exact evaluators to run."""

        @evaluator(name="eval_a")
        def eval_a(context):
            return 1.0

        @evaluator(name="eval_b")
        def eval_b(context):
            return 0.5

        @evaluator(name="eval_c")
        def eval_c(context):
            return 0.7

        runner = Monitor(include=["eval_a", "eval_c"])
        assert set(runner.evaluator_names) == {"eval_a", "eval_c"}

    def test_exclude_by_name(self, clean_registry):
        """exclude parameter removes specific evaluators."""

        @evaluator(name="eval_a")
        def eval_a(context):
            return 1.0

        @evaluator(name="eval_b")
        def eval_b(context):
            return 0.5

        @evaluator(name="eval_c")
        def eval_c(context):
            return 0.7

        runner = Monitor(exclude=["eval_b"])
        assert set(runner.evaluator_names) == {"eval_a", "eval_c"}


# ============================================================================
# TESTS: AGGREGATION
# ============================================================================


class TestMonitorAggregation:
    """Test result aggregation."""

    def test_default_mean_aggregation(self, clean_registry, sample_traces):
        """Default aggregation is MEAN when none specified."""

        @evaluator(name="output_check")
        def output_check(observation, task=None):
            return 1.0 if observation.output else 0.0

        runner = Monitor()
        result = runner.run(traces=sample_traces)

        # trace_1 and trace_2 have output (1.0), trace_3 doesn't (0.0)
        # Mean = (1.0 + 1.0 + 0.0) / 3 = 0.667
        assert "output_check" in result.scores
        agg = result.scores["output_check"]
        # AggregatedResults has .mean property and .aggregations dict
        assert agg.mean is not None
        assert abs(agg.mean - 0.667) < 0.01

    def test_individual_scores_included(self, clean_registry, sample_traces):
        """Individual scores with trace_ids are included in results."""

        @evaluator(name="test_eval")
        def test_eval(observation, task=None):
            return 1.0 if observation.output else 0.0

        runner = Monitor()
        result = runner.run(traces=sample_traces)

        summary = result.scores["test_eval"]
        assert len(summary.individual_scores) == 3

        # Check trace_ids are captured
        trace_ids = [s.trace_id for s in summary.individual_scores]
        assert "trace_1" in trace_ids
        assert "trace_2" in trace_ids
        assert "trace_3" in trace_ids


# ============================================================================
# TESTS: VALIDATION
# ============================================================================


class TestMonitorValidation:
    """Test input validation."""

    def test_conflicting_evaluator_names_raises_error(self, clean_registry):
        """Cannot have same evaluator name in both include and exclude."""
        with pytest.raises(ValueError, match="Evaluator names cannot be in both include and exclude"):
            Monitor(
                include=["eval_a", "eval_b"],
                exclude=["eval_b", "eval_c"],  # eval_b is in both
            )

    def test_conflicting_tags_raises_error(self, clean_registry):
        """Cannot have same tag in both include_tags and exclude_tags."""
        with pytest.raises(ValueError, match="Tags cannot be in both include_tags and exclude_tags"):
            Monitor(
                include_tags=["quality", "basic"],
                exclude_tags=["basic", "llm-judge"],  # basic is in both
            )

    def test_no_conflict_when_disjoint_names(self, clean_registry):
        """No error when include and exclude lists are disjoint."""
        # Should not raise
        runner = Monitor(include=["eval_a", "eval_b"], exclude=["eval_c", "eval_d"])
        assert runner is not None

    def test_no_conflict_when_disjoint_tags(self, clean_registry):
        """No error when include_tags and exclude_tags are disjoint."""
        # Should not raise
        runner = Monitor(include_tags=["quality"], exclude_tags=["llm-judge"])
        assert runner is not None


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
