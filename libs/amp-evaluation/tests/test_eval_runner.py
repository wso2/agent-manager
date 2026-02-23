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
1. Basic runner functionality with explicit evaluator lists
2. Aggregation of results
3. Validation (empty evaluators list raises ValueError)
"""

import pytest
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from amp_evaluation import evaluator, EvalResult
from amp_evaluation.runner import Monitor, RunResult
from amp_evaluation.evaluators.params import EvalMode
from amp_evaluation.trace import Trace, TraceLoader, parse_traces_for_evaluation

FIXTURES_DIR = Path(__file__).parent / "fixtures"


# ============================================================================
# TEST FIXTURES
# ============================================================================


@pytest.fixture
def sample_traces():
    """Load real traces from fixture file."""
    loader = TraceLoader(
        file_path=str(FIXTURES_DIR / "sample_traces.json"),
        agent_uid="sample-agent",
        environment_uid="test",
    )
    otel_traces = loader.load_batch(limit=5)
    return parse_traces_for_evaluation(otel_traces)


# ============================================================================
# TESTS: BASIC RUNNER FUNCTIONALITY
# ============================================================================


class TestMonitorBasic:
    """Test basic Monitor functionality with explicit evaluator lists."""

    def test_empty_runner_no_evaluators(self):
        """Monitor with empty evaluators list raises ValueError."""
        with pytest.raises(ValueError, match="At least one evaluator is required"):
            Monitor(evaluators=[])

    def test_runs_all_provided_evaluators(self):
        """Monitor runs all evaluators passed explicitly."""

        @evaluator(name="eval_a")
        def eval_a(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        @evaluator(name="eval_b")
        def eval_b(trace: Trace) -> EvalResult:
            return EvalResult(score=0.5)

        runner = Monitor(evaluators=[eval_a, eval_b])
        assert runner.evaluator_count == 2
        assert set(runner.evaluator_names) == {"eval_a", "eval_b"}

    def test_run_returns_result(self, sample_traces):
        """Run returns a RunResult with proper structure."""

        @evaluator(name="simple_eval")
        def simple_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0 if trace.output else 0.0)

        runner = Monitor(evaluators=[simple_eval])
        result = runner.run(traces=sample_traces)

        assert isinstance(result, RunResult)
        assert result.traces_evaluated == len(sample_traces)
        assert result.evaluators_run == 1
        assert result.success
        assert "simple_eval" in result.scores

    def test_multiple_evaluators_run(self, sample_traces):
        """Multiple evaluators all produce scores."""

        @evaluator(name="length_eval")
        def length_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=min(len(trace.output) / 10.0, 1.0))

        @evaluator(name="has_output_eval")
        def has_output_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0 if trace.output else 0.0)

        runner = Monitor(evaluators=[length_eval, has_output_eval])
        result = runner.run(traces=sample_traces)

        assert result.evaluators_run == 2
        assert "length_eval" in result.scores
        assert "has_output_eval" in result.scores


# ============================================================================
# TESTS: AGGREGATION
# ============================================================================


class TestMonitorAggregation:
    """Test result aggregation."""

    def test_default_mean_aggregation(self, sample_traces):
        """Default aggregation is MEAN when none specified."""

        @evaluator(name="constant_eval")
        def constant_eval(trace: Trace) -> EvalResult:
            # Always return 0.8 — deterministic, tests aggregation math
            return EvalResult(score=0.8)

        runner = Monitor(evaluators=[constant_eval])
        result = runner.run(traces=sample_traces)

        assert "constant_eval" in result.scores
        agg = result.scores["constant_eval"]
        assert agg.mean is not None
        assert abs(agg.mean - 0.8) < 0.001  # Mean of constant 0.8 must be 0.8

    def test_individual_scores_included(self, sample_traces):
        """Individual scores with trace_ids are included in results."""

        @evaluator(name="test_eval")
        def custom_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        runner = Monitor(evaluators=[custom_eval])
        result = runner.run(traces=sample_traces)

        summary = result.scores["test_eval"]
        # One score per loaded trace
        assert len(summary.individual_scores) == len(sample_traces)
        # All trace IDs are non-empty strings from the real data
        trace_ids = [s.trace_id for s in summary.individual_scores]
        assert all(isinstance(tid, str) and len(tid) > 0 for tid in trace_ids)


# ============================================================================
# TESTS: VALIDATION
# ============================================================================


class TestMonitorValidation:
    """Test input validation."""

    def test_missing_evaluators_raises_error(self):
        """Monitor with empty evaluators list raises ValueError."""
        with pytest.raises(ValueError, match="At least one evaluator is required"):
            Monitor(evaluators=[])

    def test_single_evaluator_accepted(self):
        """Monitor accepts a single evaluator."""

        @evaluator(name="solo_eval")
        def solo_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        runner = Monitor(evaluators=[solo_eval])
        assert runner.evaluator_count == 1
        assert runner.evaluator_names == ["solo_eval"]


# ============================================================================
# REGRESSION TESTS: Bugs found during code review
# ============================================================================


class TestRunResultSuccess:
    """Regression: RunResult.success must be False when no traces were evaluated."""

    def test_success_false_with_zero_traces(self):
        """An empty run (0 traces evaluated) should NOT be reported as successful."""
        from datetime import datetime

        result = RunResult(
            run_id="test_empty",
            eval_mode=EvalMode.MONITOR,
            started_at=datetime.now(),
            traces_evaluated=0,
            evaluators_run=0,
        )
        assert result.success is False

    def test_success_true_with_traces_and_no_errors(self):
        """A run with evaluated traces and no errors should be successful."""
        from datetime import datetime

        result = RunResult(
            run_id="test_ok",
            eval_mode=EvalMode.MONITOR,
            started_at=datetime.now(),
            traces_evaluated=5,
            evaluators_run=1,
        )
        assert result.success is True

    def test_success_false_with_traces_but_errors(self):
        """A run with errors should not be successful even if traces were evaluated."""
        from datetime import datetime

        result = RunResult(
            run_id="test_errors",
            eval_mode=EvalMode.MONITOR,
            started_at=datetime.now(),
            traces_evaluated=5,
            evaluators_run=1,
            errors=["Something went wrong"],
        )
        assert result.success is False


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
