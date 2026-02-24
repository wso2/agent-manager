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
Tests for EvalResult model including the new skip() pattern.
"""

import pytest
from amp_evaluation.models import EvalResult, EvaluatorScore


class TestEvalResultSuccess:
    """Test EvalResult for successful evaluations."""

    def test_create_with_score(self):
        """Test creating result with just a score."""
        result = EvalResult(score=0.8)
        assert result.score == 0.8
        assert result.passed is True  # >= 0.5
        assert result.is_skipped is False
        assert result.explanation is None

    def test_create_with_score_and_explanation(self):
        """Test creating result with score and explanation."""
        result = EvalResult(score=0.3, explanation="Too short")
        assert result.score == 0.3
        assert result.passed is False  # < 0.5
        assert result.explanation == "Too short"
        assert result.is_skipped is False

    def test_create_with_explicit_passed(self):
        """Test creating result with explicit passed override."""
        result = EvalResult(score=0.6, passed=False)
        assert result.score == 0.6
        assert result.passed is False  # Explicit override
        assert result.is_skipped is False

    def test_create_with_details(self):
        """Test creating result with details."""
        result = EvalResult(score=0.9, explanation="Great", details={"metric1": 100, "metric2": 200})
        assert result.score == 0.9
        assert result.details == {"metric1": 100, "metric2": 200}
        assert result.is_skipped is False

    def test_score_validation_min(self):
        """Test that score < 0.0 raises ValueError."""
        with pytest.raises(ValueError, match="must be between 0.0 and 1.0"):
            EvalResult(score=-0.1)

    def test_score_validation_max(self):
        """Test that score > 1.0 raises ValueError."""
        with pytest.raises(ValueError, match="must be between 0.0 and 1.0"):
            EvalResult(score=1.1)

    def test_score_validation_type(self):
        """Test that non-numeric score raises TypeError."""
        with pytest.raises(TypeError, match="must be a number"):
            EvalResult(score="0.8")  # type: ignore

    def test_score_zero_is_valid(self):
        """Test that score=0.0 is a valid evaluation result."""
        result = EvalResult(score=0.0, explanation="Complete failure")
        assert result.score == 0.0
        assert result.passed is False
        assert result.is_skipped is False
        assert result.explanation == "Complete failure"

    def test_score_one_is_valid(self):
        """Test that score=1.0 is a valid evaluation result."""
        result = EvalResult(score=1.0, explanation="Perfect")
        assert result.score == 1.0
        assert result.passed is True
        assert result.is_skipped is False


class TestEvalResultError:
    """Test EvalResult.skip() for error cases."""

    def test_skip_with_reason(self):
        """Test creating skip result with reason."""
        result = EvalResult.skip("Missing API key")
        assert result.is_skipped is True
        assert result.skip_reason == "Missing API key"
        assert result.explanation is None  # explanation is for scores, not skips

    def test_skip_with_details(self):
        """Test creating skip result with details."""
        result = EvalResult.skip(
            "DeepEval not installed", details={"package": "deepeval", "pip_install": "pip install deepeval"}
        )
        assert result.is_skipped is True
        assert result.skip_reason == "DeepEval not installed"
        assert result.details == {"package": "deepeval", "pip_install": "pip install deepeval"}

    def test_skip_score_inaccessible(self):
        """Test that accessing score on skipped result raises AttributeError."""
        result = EvalResult.skip("Cannot evaluate")
        with pytest.raises(AttributeError, match="Cannot access score on a skipped result"):
            _ = result.score

    def test_skip_passed_inaccessible(self):
        """Test that accessing passed on skipped result raises AttributeError."""
        result = EvalResult.skip("Cannot evaluate")
        with pytest.raises(AttributeError, match="Cannot access passed on a skipped result"):
            _ = result.passed

    def test_skip_explanation_is_empty(self):
        """Test that explanation is empty on skipped results (skip_reason holds the reason)."""
        result = EvalResult.skip("Test error")
        assert result.explanation is None
        assert result.skip_reason == "Test error"


class TestEvalResultRepr:
    """Test string representation of EvalResult."""

    def test_repr_success(self):
        """Test repr for successful result."""
        result = EvalResult(score=0.85, explanation="Good quality")
        repr_str = repr(result)
        assert "score=0.85" in repr_str
        assert "passed=True" in repr_str
        assert "Good quality" in repr_str

    def test_repr_error(self):
        """Test repr for skipped result."""
        result = EvalResult.skip("Missing data")
        repr_str = repr(result)
        assert "skip_reason='Missing data'" in repr_str
        assert "score" not in repr_str  # Should not show score


class TestEvalResultPatterns:
    """Test common usage patterns."""

    def test_pattern_success_high_score(self):
        """Test pattern: evaluation succeeded with high score."""
        result = EvalResult(score=0.95, explanation="Excellent response")
        assert not result.is_skipped
        assert result.score == 0.95
        assert result.passed is True

    def test_pattern_success_low_score(self):
        """Test pattern: evaluation succeeded but low score."""
        result = EvalResult(score=0.2, explanation="Response too short")
        assert not result.is_skipped
        assert result.score == 0.2
        assert result.passed is False

    def test_pattern_error_missing_dependency(self):
        """Test pattern: evaluation skipped due to missing dependency."""
        result = EvalResult.skip("openai package not installed")
        assert result.is_skipped
        assert "openai" in result.skip_reason

    def test_pattern_error_missing_data(self):
        """Test pattern: evaluation skipped due to missing data."""
        result = EvalResult.skip("No expected output in task")
        assert result.is_skipped
        assert "expected output" in result.skip_reason.lower()

    def test_pattern_conditional_access(self):
        """Test pattern: conditionally access score based on is_skipped."""
        result1 = EvalResult(score=0.7)
        result2 = EvalResult.skip("Error")

        # Safe access pattern
        if not result1.is_skipped:
            assert result1.score == 0.7

        if not result2.is_skipped:
            pytest.fail("Should not reach here")
        else:
            assert result2.skip_reason == "Error"


# ============================================================================
# EvaluatorScore score validation
# ============================================================================


class TestEvaluatorScoreValidation:
    """Test EvaluatorScore creation and skip_reason tracking.

    Score range validation (0.0-1.0) is enforced at EvalResult creation time.
    EvaluatorScore trusts that scores coming from the runner are already valid.
    """

    def test_valid_score_accepted(self):
        score = EvaluatorScore(trace_id="t1", score=0.8, passed=True)
        assert score.score == 0.8

    def test_boundary_zero_accepted(self):
        score = EvaluatorScore(trace_id="t1", score=0.0, passed=False)
        assert score.score == 0.0

    def test_boundary_one_accepted(self):
        score = EvaluatorScore(trace_id="t1", score=1.0, passed=True)
        assert score.score == 1.0

    def test_skipped_record(self):
        """Skipped evaluations have score=None and skip_reason set."""
        score = EvaluatorScore(trace_id="t1", skip_reason="LLM call failed")
        assert score.is_skipped
        assert not score.is_successful
        assert score.skip_reason == "LLM call failed"
        assert score.score is None
        assert score.passed is None

    def test_non_skipped_record(self):
        score = EvaluatorScore(trace_id="t1", score=0.8, passed=True)
        assert not score.is_skipped
        assert score.skip_reason is None
