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
Core data models for the evaluation framework.

This module defines the evaluation result and summary models:
- EvalResult: Result returned by evaluators (score + pass/fail + explanation)
- SpanContext: Identity for sub-trace (agent/LLM) evaluations
- TaskContext: Experiment context (task/trial IDs)
- EvaluatorScore: Individual score for a single trace/evaluator pair
- EvaluatorSummary: Aggregated results for one evaluator across all traces
- EvaluatorInfo: Metadata describing an evaluator (name, tags, config schema)
- DataNotAvailableError: Exception for missing evaluation data
- Agent: Minimal agent info from config
"""

from __future__ import annotations


from dataclasses import dataclass, field
from datetime import datetime
from typing import List, Dict, Optional, Any

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
    Result returned by evaluators — the pure DX model.

    This is the ONLY type evaluator authors interact with. It carries:
      - A score (0.0–1.0)
      - A pass/fail flag (defaults to score >= 0.5)
      - A human-readable explanation of why the score was given

    The explanation field is the sole channel for diagnostic context. Put
    everything the user needs to understand the score into the explanation
    string (e.g. "Found 3/5 required items. Missing: ['term1', 'term2']").

    Score convention:
      - Range:    0.0 to 1.0 (enforced — raises ValueError if violated)
      - Polarity: 0.0 = worst outcome, 1.0 = best outcome (higher is always better)

    Two types of results:
      1. Success: Evaluation completed with a score
         EvalResult(score=0.8, explanation="Good response")
         EvalResult(score=0.0, passed=False, explanation="Failed quality check")

      2. Skip: Evaluation could not be performed
         EvalResult.skip("Missing required data")
         EvalResult.skip("API key not configured")

    Always check is_skipped before accessing score/passed on unknown results.

    Design rationale:
      - score=0.0 means "evaluated and failed" (bad outcome)
      - score=1.0 means "evaluated and passed" (best outcome)
      - skip() means "could not evaluate at all" (not a score)
    """

    _score: Optional[float] = field(default=None, init=False, repr=False)
    _passed: Optional[bool] = field(default=None, init=False, repr=False)
    explanation: Optional[str] = None
    skip_reason: Optional[str] = field(default=None, init=False, repr=False)

    def __init__(
        self,
        score: float,  # REQUIRED: must be 0.0-1.0
        explanation: Optional[str] = None,
        passed: Optional[bool] = None,
    ):
        """
        Create a successful evaluation result.

        Args:
            score: Evaluation score between 0.0 and 1.0
            explanation: Human-readable explanation of the result
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
        self.skip_reason = None

    @classmethod
    def skip(cls, reason: str) -> "EvalResult":
        """
        Create a skipped result when evaluation cannot be performed.

        Use this when:
        - Required data is missing
        - Dependencies are not available
        - Evaluation logic encounters an error

        Args:
            reason: Why the evaluation was skipped

        Returns:
            EvalResult with is_skipped=True
        """
        obj = object.__new__(cls)
        obj._score = None
        obj._passed = None
        obj.explanation = None
        obj.skip_reason = reason
        return obj

    @property
    def score(self) -> float:
        """Get evaluation score. Raises AttributeError if this is a skipped result."""
        if self._score is None:
            raise AttributeError(
                f"Cannot access score on a skipped result. Check is_skipped before accessing score. Reason: {self.skip_reason}"
            )
        return self._score

    @property
    def passed(self) -> bool:
        """Get pass/fail status. Raises AttributeError if this is a skipped result."""
        if self._passed is None:
            raise AttributeError(
                f"Cannot access passed on a skipped result. Check is_skipped before accessing passed. Reason: {self.skip_reason}"
            )
        return self._passed

    @property
    def is_skipped(self) -> bool:
        """Check if this result was skipped (could not be evaluated)."""
        return self.skip_reason is not None

    def __repr__(self) -> str:
        if self.is_skipped:
            return f"EvalResult(skip_reason={self.skip_reason!r})"
        return f"EvalResult(score={self._score}, passed={self._passed}, explanation={self.explanation!r})"


@dataclass
class SpanContext:
    """Identity for sub-trace (agent/LLM) evaluations.

    Present when evaluation targets a specific span within a trace.
    None on EvaluatorScore means it's a trace-level evaluation.

    For agent-level: span_id is agent_id, agent_name and model from AgentTrace.
    For LLM-level: span_id is the LLM span ID, model and vendor from LLMSpan.
    """

    span_id: str  # Required anchor — if context exists, span_id is always set
    agent_name: Optional[str] = None  # Agent-level: from AgentTrace.agent_name
    model: Optional[str] = None  # Agent or LLM level: model name
    vendor: Optional[str] = None  # LLM-level only: from LLMSpan.vendor


@dataclass
class TaskContext:
    """Experiment context — only set when running evaluations against a dataset.

    Present when evaluations are run via Experiment runner with tasks.
    None on EvaluatorScore means it's a monitoring run (no dataset).
    """

    task_id: str  # Required anchor — if context exists, task_id is always set
    trial_id: Optional[str] = None  # Set when multiple trials per task


@dataclass
class EvaluatorScore:
    """
    Individual evaluation score for a single trace — the framework's enriched model.

    Created from EvalResult (DX model returned by evaluator authors) via the
    from_eval_result() factory. Framework enrichments (span identity, trace context)
    are added here, NOT on EvalResult.

    This is the detailed record of how one trace was evaluated by one evaluator.
    Used in EvaluatorSummary.individual_scores for detailed analysis.

    Two levels of failure in the evaluation pipeline:
      1. Run/trace level — trace can't be fetched/parsed, runner failure
         → stored in RunResult.errors (logged, not persisted to DB)
      2. Evaluator level — evaluator can't produce a score for this trace
         → stored here as a "skip" with skip_reason explaining why

    A skip means "the evaluator could not evaluate this trace." The reason may
    be benign ("missing expected output") or an error ("LLM call timed out").
    Either way, no score was produced.

    States:
      - Successful: score and passed are set, skip_reason is None
      - Skipped:    score and passed are None, skip_reason explains why

    Contexts (None when not applicable):
      - span_context is None  → trace-level evaluation
      - task_context is None  → monitoring run (not an experiment)

    Score convention (when successful):
      - Range:    0.0 to 1.0 (validated at EvalResult creation time)
      - Polarity: 0.0 = worst outcome, 1.0 = best outcome (higher is always better)
    """

    # Trace identity
    trace_id: str
    trace_start_time: Optional[datetime] = None  # When the trace started (root span start time)
    # Evaluation results
    score: Optional[float] = None  # None when skipped
    passed: Optional[bool] = None  # None when skipped
    explanation: Optional[str] = None  # Why this score was assigned
    # Skip tracking (if evaluator could not produce a score)
    skip_reason: Optional[str] = None  # Why evaluation was skipped
    # Contexts — None when not applicable
    span_context: Optional[SpanContext] = None  # None = trace-level evaluation
    task_context: Optional[TaskContext] = None  # None = monitoring run (not experiment)

    @classmethod
    def from_eval_result(
        cls,
        eval_result: "EvalResult",
        trace_id: str,
        trace_start_time: Optional[datetime] = None,
        span_context: Optional["SpanContext"] = None,
    ) -> "EvaluatorScore":
        """Create an EvaluatorScore from an EvalResult with optional span context.

        This is the bridge between the DX model (EvalResult, authored by evaluator
        writers) and the framework model (EvaluatorScore, enriched with trace/span
        identity). Called by BaseEvaluator.run() after evaluate() returns.
        """
        if eval_result.is_skipped:
            return cls(
                trace_id=trace_id,
                trace_start_time=trace_start_time,
                skip_reason=eval_result.skip_reason,
                span_context=span_context,
            )
        return cls(
            trace_id=trace_id,
            trace_start_time=trace_start_time,
            score=eval_result.score,
            passed=eval_result.passed,
            explanation=eval_result.explanation,
            span_context=span_context,
        )

    @property
    def is_skipped(self) -> bool:
        """Check if this evaluation was skipped (could not produce a score)."""
        return self.skip_reason is not None

    @property
    def is_successful(self) -> bool:
        """Check if this evaluation completed successfully with a score."""
        return not self.is_skipped


@dataclass
class EvaluatorSummary:
    """
    Aggregated results for a single evaluator across all evaluated traces.

    This combines both aggregated scores and individual scores in one place.
    Used as the value type in RunResult.scores dict.

    Example:
        summary = run_result.scores["groundedness"]
        print(summary.aggregated_scores["mean"])  # 0.85
        print(summary.aggregated_scores["pass_rate_0.5"])  # 0.92
        for score in summary.individual_scores:
            print(f"{score.trace_id}: {score.score}")
    """

    evaluator_name: str
    count: int
    level: str  # Evaluation level: "trace", "agent", or "llm"
    skipped_count: int = 0  # Evaluations that could not produce a score (intentional or exception)
    aggregated_scores: Dict[str, float] = field(default_factory=dict)  # e.g., {"mean": 0.85, "pass_rate_0.5": 0.9}
    individual_scores: List[EvaluatorScore] = field(default_factory=list)
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

    @property
    def error_count(self) -> int:
        """Alias for skipped_count (matches Go-side naming convention)."""
        return self.skipped_count

    @property
    def successful_scores(self) -> List[EvaluatorScore]:
        """Get only successful evaluation scores (excludes skipped)."""
        return [s for s in self.individual_scores if s.is_successful]

    @property
    def skipped_scores(self) -> List[EvaluatorScore]:
        """Get only skipped evaluation scores."""
        return [s for s in self.individual_scores if s.is_skipped]

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

    def get_agent_scores(self, agent_name: str) -> List[EvaluatorScore]:
        """
        Get all scores for a specific agent (for agent-level evaluators).

        Args:
            agent_name: Name of the agent to filter by

        Returns:
            List of EvaluatorScore objects for this agent
        """
        return [
            score
            for score in self.individual_scores
            if score.span_context is not None and score.span_context.agent_name == agent_name
        ]

    def get_model_scores(self, model: str) -> List[EvaluatorScore]:
        """
        Get all scores for a specific model (for agent or LLM-level evaluators).

        Args:
            model: Model name to filter by

        Returns:
            List of EvaluatorScore objects for this model
        """
        return [
            score
            for score in self.individual_scores
            if score.span_context is not None and score.span_context.model == model
        ]

    def summary(self, verbosity: str = "default") -> str:
        """Format this evaluator's results as a human-readable string.

        Args:
            verbosity: "compact", "default", or "detailed"
        """
        name = self.evaluator_name
        mean_str = f"{self.mean:.4f}" if self.mean is not None else "N/A"
        pass_rate = self.pass_rate
        pass_rate_str = f"{pass_rate:.1%}" if pass_rate is not None else "N/A"
        skipped_str = f", skipped={self.skipped_count}" if self.skipped_count > 0 else ""

        if verbosity == "compact":
            return f"  {name}: count={self.count}{skipped_str}, mean={mean_str}, pass_rate={pass_rate_str}"

        # Default and detailed share the header block
        lines = [f"  {name}:"]
        lines.append(f"    level: {self.level}")
        lines.append(f"    count: {self.count}")
        if self.skipped_count > 0:
            lines.append(f"    skipped: {self.skipped_count}")
        for agg_name, value in self.aggregated_scores.items():
            if isinstance(value, (int, float)):
                lines.append(f"    {agg_name}: {value:.4f}")
            else:
                lines.append(f"    {agg_name}: {value}")

        if verbosity == "detailed" and self.individual_scores:
            lines.append(f"    individual scores ({len(self.individual_scores)}):")
            for score in self.individual_scores:
                trace_short = score.trace_id[:12] if len(score.trace_id) > 12 else score.trace_id
                if score.is_skipped:
                    lines.append(f"      [ SKIP] trace={trace_short}...")
                    if score.skip_reason:
                        lines.append(f"              {score.skip_reason}")
                else:
                    status = "PASS" if score.passed else "FAIL"
                    lines.append(f"      [{status}] trace={trace_short}... score={score.score:.2f}")
                    if score.explanation:
                        for line in score.explanation.strip().splitlines():
                            lines.append(f"              {line}")

        return "\n".join(lines)


# ============================================================================
# LLM PROVIDER CATALOG MODELS
# ============================================================================


@dataclass
class LLMConfigField:
    """A configuration field required by an LLM provider.

    field_type drives how the platform renders and handles the value:
      "password"  -> secret (mask in UI, do not log, treat as credential)
      "text"      -> plain text (e.g. api_base URL, api_version string)

    Values come from litellm's provider_create_fields.json — no custom
    secret detection logic needed.
    """

    key: str  # "api_key", "api_base"
    label: str  # "API Key", "Base URL"
    field_type: str  # "password" (secret) | "text" (plain)
    required: bool
    env_var: str  # e.g. "OPENAI_API_KEY" — env var platform sets on the job process
    description: str = ""


@dataclass
class LLMProviderInfo:
    """Metadata about a supported LLM provider."""

    name: str  # "openai" — platform provider identifier
    display_name: str  # "OpenAI"
    config_fields: List["LLMConfigField"]
    models: List[str]  # curated model names in provider/model format


# ============================================================================
# AGENT MODEL (Minimal - loaded from config)
# ============================================================================


@dataclass
class EvaluatorInfo:
    """
    Metadata describing an evaluator.

    Returned by .info property and builtin_evaluator_catalog().
    """

    name: str
    description: str
    tags: List[str]
    version: str
    modes: List[str]
    level: str = "trace"  # Single level: "trace", "agent", or "llm"
    config_schema: List[Dict[str, Any]] = field(default_factory=list)
    class_name: Optional[str] = None
    module: Optional[str] = None


@dataclass
class Agent:
    """
    Minimal agent information for evaluation.
    All fields are loaded from environment variables/config.
    """

    agent_uid: str
    environment_uid: str
