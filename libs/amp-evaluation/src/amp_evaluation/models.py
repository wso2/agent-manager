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

from __future__ import annotations


from dataclasses import dataclass, field
from datetime import datetime
from typing import List, Dict, Any, Optional


"""
Core data models for the evaluation framework.

This module defines all the fundamental concepts:
- Task: A test case with inputs and success criteria
- Dataset: Collection of tasks for evaluation
- Trajectory: Step-by-step execution record from traces
- EvalResult: Result from an evaluator
- EvalContext: Context provided to evaluators during evaluation
- Constraints: Performance constraints for evaluation
"""

# ============================================================================
# EXCEPTIONS
# ============================================================================


class DataNotAvailableError(Exception):
    """Raised when an evaluator tries to access unavailable data."""

    def __init__(self, field_name: str):
        message = (
            f"'{field_name}' is not available in this evaluation context.\n"
            f"This evaluator requires a dataset with the '{field_name}' field.\n"
            f"Hint: For live evaluation, use evaluators that don't require ground truth."
        )
        super().__init__(message)
        self.field_name = field_name


# ============================================================================
# EVAL RESULT MODELS
# ============================================================================


@dataclass
class EvalResult:
    """
    Result returned by evaluators.

    Two types of results:
      1. Success: Evaluation completed with a score
         EvalResult(score=0.8, explanation="Good response")
         EvalResult(score=0.0, passed=False, explanation="Failed quality check")

      2. Error: Evaluation could not be performed
         EvalResult.skip("Missing required data")
         EvalResult.skip("API key not configured")

    Always check is_error before accessing score/passed on unknown results.

    Design rationale:
      - score=0.0 means "evaluated and failed"
      - skip() means "could not evaluate at all"
    """

    _score: Optional[float] = field(default=None, init=False, repr=False)
    _passed: Optional[bool] = field(default=None, init=False, repr=False)
    explanation: str = ""
    details: Optional[Dict[str, Any]] = None
    error: Optional[str] = field(default=None, init=False, repr=False)

    def __init__(
        self,
        score: float,  # REQUIRED: must be 0.0-1.0
        explanation: str = "",
        details: Optional[Dict[str, Any]] = None,
        passed: Optional[bool] = None,
    ):
        """
        Create a successful evaluation result.

        Args:
            score: Evaluation score between 0.0 and 1.0
            explanation: Human-readable explanation of the result
            details: Additional structured data
            passed: Override pass/fail (defaults to score >= 0.5)

        For error cases, use EvalResult.skip() instead.
        """
        if not isinstance(score, (int, float)):
            raise TypeError(f"score must be a number, got {type(score).__name__}")
        if not 0.0 <= score <= 1.0:
            raise ValueError(f"score must be between 0.0 and 1.0, got {score}")

        self._score = float(score)
        self._passed = passed if passed is not None else score >= 0.5
        self.explanation = explanation
        self.details = details
        self.error = None

    @classmethod
    def skip(cls, reason: str, details: Optional[Dict[str, Any]] = None) -> "EvalResult":
        """
        Create an error result when evaluation cannot be performed.

        Use this when:
        - Required data is missing
        - Dependencies are not available
        - Evaluation logic encounters an error

        Args:
            reason: Why the evaluation was skipped
            details: Additional context about the error

        Returns:
            EvalResult with is_error=True
        """
        obj = object.__new__(cls)
        obj._score = None
        obj._passed = None
        obj.explanation = reason
        obj.details = details
        obj.error = reason
        return obj

    @property
    def score(self) -> float:
        """Get evaluation score. Raises AttributeError if this is an error result."""
        if self._score is None:
            raise AttributeError(
                f"Cannot access score on an error result. Check is_error before accessing score. Error: {self.error}"
            )
        return self._score

    @property
    def passed(self) -> bool:
        """Get pass/fail status. Raises AttributeError if this is an error result."""
        if self._passed is None:
            raise AttributeError(
                f"Cannot access passed on an error result. Check is_error before accessing passed. Error: {self.error}"
            )
        return self._passed

    @property
    def is_error(self) -> bool:
        """Check if this result represents an evaluation error."""
        return self.error is not None

    def __repr__(self) -> str:
        if self.is_error:
            return f"EvalResult(error={self.error!r})"
        return f"EvalResult(score={self._score}, passed={self._passed}, explanation={self.explanation!r})"


@dataclass
class EvaluatorScore:
    """
    Individual evaluation score for a single trace.

    This is the detailed record of how one trace was evaluated by one evaluator.
    Used in EvaluatorSummary.individual_scores for detailed analysis.
    """

    trace_id: str
    score: float
    passed: bool
    span_id: Optional[str] = None  # Set for agent/span level evaluations
    timestamp: Optional[datetime] = None  # Trace timestamp (when trace occurred)
    explanation: Optional[str] = None
    # Experiment-specific (optional)
    task_id: Optional[str] = None
    trial_id: Optional[str] = None
    # Extra data from evaluator
    metadata: Dict[str, Any] = field(default_factory=dict)
    # Error tracking (if evaluator failed to run)
    error: Optional[str] = None  # Set if evaluation failed with exception

    @property
    def is_error(self) -> bool:
        """Check if this score represents an evaluation error."""
        return self.error is not None


@dataclass
class EvaluatorSummary:
    """
    Aggregated results for a single evaluator across all evaluated traces.

    This combines both aggregated scores and individual scores in one place.
    Used as the value type in RunResult.scores dict.

    Example:
        summary = run_result.scores["hallucination"]
        print(summary.aggregated_scores["mean"])  # 0.85
        print(summary.aggregated_scores["pass_rate_0.5"])  # 0.92
        for score in summary.individual_scores:
            print(f"{score.trace_id}: {score.score}")
    """

    evaluator_name: str
    count: int
    aggregated_scores: Dict[str, float] = field(default_factory=dict)  # e.g., {"mean": 0.85, "pass_rate_0.5": 0.9}
    individual_scores: List[EvaluatorScore] = field(default_factory=list)
    level: str = "trace"  # Evaluation level: "trace", "agent", or "span"
    items_per_trace: Optional[Dict[str, int]] = None  # For multi-item: {trace_id: num_items}

    def __getitem__(self, key: str) -> float:
        """Allow dict-like access to aggregated scores."""
        return self.aggregated_scores[key]

    def get(self, key: str, default: float = 0.0) -> float:
        """Get aggregation value with default."""
        return self.aggregated_scores.get(key, default)

    @property
    def mean(self) -> Optional[float]:
        """Convenience accessor for mean."""
        return self.aggregated_scores.get("mean")

    @property
    def pass_rate(self) -> Optional[float]:
        """Convenience accessor for default pass_rate (threshold 0.5)."""
        rate = self.aggregated_scores.get("pass_rate")
        if rate is not None:
            return rate
        return self.aggregated_scores.get("pass_rate_0.5")

    def get_by_trace(self, trace_id: str) -> List[EvaluatorScore]:
        """
        Get all evaluation scores for a specific trace.

        Useful for multi-item evaluators (agent-level, span-level) where
        one trace produces multiple scores.

        Args:
            trace_id: The trace ID to filter by

        Returns:
            List of EvaluatorScore objects for this trace
        """
        return [score for score in self.individual_scores if score.trace_id == trace_id]

    def get_by_metadata(self, key: str, value: Any) -> List[EvaluatorScore]:
        """
        Filter scores by metadata field.

        Args:
            key: Metadata key to filter by (e.g., "agent_name", "span_type")
            value: Value to match

        Returns:
            List of EvaluatorScore objects matching the filter
        """
        return [score for score in self.individual_scores if score.metadata.get(key) == value]

    def get_agent_scores(self, agent_name: str) -> List[EvaluatorScore]:
        """
        Get all scores for a specific agent (for agent-level evaluators).

        Args:
            agent_name: Name of the agent to filter by

        Returns:
            List of EvaluatorScore objects for this agent
        """
        return self.get_by_metadata("agent_name", agent_name)


# ============================================================================
# AGENT MODEL (Minimal - loaded from config)
# ============================================================================


@dataclass
class Agent:
    """
    Minimal agent information for evaluation.
    All fields are loaded from environment variables/config.
    """

    agent_uid: str
    environment_uid: str
