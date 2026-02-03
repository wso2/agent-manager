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
from typing import List, Dict, Any, Optional, Literal, TYPE_CHECKING, Union
from uuid import uuid4

# Import Constraints from dataset_schema (single source of truth)
from .dataset_schema import Constraints

if TYPE_CHECKING:
    from .trace.models import Trajectory

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
# TASK MODELS
# ============================================================================


@dataclass
class Task:
    """
    A single test case - matches JSON DatasetTask schema.
    The fundamental building block of evaluation.
    """

    # Required
    task_id: str
    name: str
    description: str
    input: Union[str, Dict[str, Any]]  # User query/prompt (string or structured JSON)

    # Reference data for evaluation (ground truth)
    expected_output: Optional[str] = None
    expected_trajectory: Optional[List[Dict[str, Any]]] = None  # Expected sequence of steps
    expected_outcome: Optional[Dict[str, Any]] = None  # Expected side effects/state
    success_criteria_text: Optional[str] = None  # Human-readable success criteria for LLM judges

    # Constraints
    prohibited_content: Optional[List[str]] = None  # Content that should NOT appear
    constraints: Optional[Constraints] = None  # Performance limits

    # Classification
    task_type: str = "general"  # "qa", "code_gen", "rag", "tool_use", "math"
    difficulty: Literal["easy", "medium", "hard", "expert"] = "medium"
    tags: List[str] = field(default_factory=list)
    domain: Optional[str] = None  # "medical", "legal", "technical", etc.

    # Extensibility
    custom: Dict[str, Any] = field(default_factory=dict)  # Passed to evaluators
    metadata: Dict[str, Any] = field(default_factory=dict)  # Not passed to evaluators

    # Dataset membership
    dataset_id: Optional[str] = None

    # Timestamps
    created_at: datetime = field(default_factory=datetime.now)
    created_by: Optional[str] = None


# ============================================================================
# EVALUATION RESULT MODELS
# ============================================================================


@dataclass
class EvalResult:
    """
    Result returned by evaluators. Contains only the evaluation outcome.

    This is what developers return from their evaluator functions.

    Examples:
        return EvalResult(score=0.8, explanation="Good response")
        return EvalResult(score=1.0, passed=True, details={"tokens": 100})
    """

    score: float  # 0.0 to 1.0
    explanation: str = ""
    details: Optional[Dict[str, Any]] = None
    passed: Optional[bool] = None

    def __post_init__(self):
        """Auto-calculate passed if not provided."""
        if self.passed is None:
            self.passed = self.score >= 0.5


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
        return self.aggregated_scores.get("pass_rate") or self.aggregated_scores.get("pass_rate_0.5")


@dataclass
class EvalContext:
    """
    Context provided to evaluators during evaluation.

    Contains observed execution trace, expected values, and constraints.
    Accessing unavailable fields raises DataNotAvailableError.
    Use has_*() methods to check availability before accessing.

    Structure:
        trace: The observed execution (required, Trajectory)
        expected_*: Ground truth values (optional, from dataset)
        guidelines: Qualitative evaluation guidance (optional)
        constraints: Quantitative performance limits (optional)
        custom: User-defined attributes (optional)

    Always available:
        - trace: The observed execution trace (Trajectory)
        - trace_id: ID of the trace (convenience - same as trace.trace_id)
        - timestamp: Start time of the trace (convenience - same as trace.timestamp)
        - metrics: Aggregated metrics (convenience - same as trace.metrics)
        - input: Trace input (convenience - same as trace.input)
        - output: Trace output (convenience - same as trace.output)
        - is_experiment: True if running Experiment (with dataset), False if Monitor (live)
        - custom: Dict of custom attributes (empty dict if none)

    Expected data (may be unavailable, raises DataNotAvailableError):
        - expected_output: Expected final output/answer
        - expected_trajectory: Expected sequence of tool calls
        - expected_outcome: Expected side effects or state changes

    Guidelines (may be unavailable, raises DataNotAvailableError):
        - success_criteria: Human-readable success criteria for LLM judges
        - prohibited_content: Content that should NOT appear in output

    Constraints (optional, access via constraints property):
        - constraints.max_latency_ms: Maximum allowed latency
        - constraints.max_tokens: Maximum token budget
        - constraints.max_iterations: Maximum agent iterations

    Task reference:
        - task: The original task from dataset (optional)

    Example:
        # Strict evaluator - requires expected_output
        def evaluate(self, ctx: EvalContext) -> EvalResult:
            # Will raise DataNotAvailableError if expected_output not in dataset
            return EvalResult(
                score=1.0 if ctx.output == ctx.expected_output else 0.0
            )

        # Flexible evaluator - handles missing data gracefully
        def evaluate(self, ctx: EvalContext) -> EvalResult:
            if ctx.has_expected_output():
                return self._compare_with_expected(ctx)
            else:
                return self._evaluate_standalone(ctx)

        # Using constraints
        def evaluate(self, ctx: EvalContext) -> EvalResult:
            if ctx.constraints and ctx.constraints.has_latency_constraint():
                max_latency = ctx.constraints.max_latency_ms
                actual = ctx.metrics.total_duration_ms
                return EvalResult(score=1.0 if actual <= max_latency else 0.0)

        # Time-series analysis using timestamp
        def evaluate(self, ctx: EvalContext) -> EvalResult:
            # Useful for Monitor - track performance over time
            hour_of_day = ctx.timestamp.hour
            # Adjust thresholds based on load patterns...
    """

    # ==========================================================================
    # OBSERVED - Always required
    # ==========================================================================
    trace: Trajectory  # The observed execution trajectory
    is_experiment: bool = False  # True = Experiment with dataset, False = Monitor (live)

    # ==========================================================================
    # EXPECTED - Optional ground truth (from dataset)
    # ==========================================================================
    _expected_output: Optional[str] = field(default=None, repr=False)
    _expected_trajectory: Optional[List[Dict[str, Any]]] = field(default=None, repr=False)
    _expected_outcome: Optional[Dict[str, Any]] = field(default=None, repr=False)

    # ==========================================================================
    # GUIDELINES - Optional qualitative guidance
    # ==========================================================================
    _success_criteria: Optional[str] = field(default=None, repr=False)
    _prohibited_content: Optional[List[str]] = field(default=None, repr=False)

    # ==========================================================================
    # CONSTRAINTS - Optional quantitative limits
    # ==========================================================================
    _constraints: Optional[Constraints] = field(default=None, repr=False)

    # ==========================================================================
    # CUSTOM - User-defined attributes
    # ==========================================================================
    _custom: Dict[str, Any] = field(default_factory=dict, repr=False)

    # ==========================================================================
    # METADATA
    # ==========================================================================
    _task: Optional["Task"] = field(default=None, repr=False)

    # ==========================================================================
    # Convenience Properties (always available from trace)
    # ==========================================================================

    @property
    def trace_id(self) -> str:
        """Convenience property for trace.trace_id. Useful for creating EvalResults."""
        return self.trace.trace_id

    @property
    def timestamp(self):
        """Convenience property for trace.timestamp. Useful for time-series analysis."""
        return self.trace.timestamp

    @property
    def metrics(self):
        """Convenience property for trace.metrics. Access all aggregated metrics."""
        return self.trace.metrics

    @property
    def input(self) -> str:
        """Convenience property for trace.input."""
        return self.trace.input

    @property
    def output(self) -> str:
        """Convenience property for trace.output."""
        return self.trace.output

    # ==========================================================================
    # Expected Data Properties
    # ==========================================================================

    @property
    def expected_output(self) -> str:
        """Expected final output. Raises DataNotAvailableError if not provided."""
        if self._expected_output is None:
            raise DataNotAvailableError("expected_output")
        return self._expected_output

    @property
    def expected_trajectory(self) -> List[Dict[str, Any]]:
        """Expected trajectory. Raises DataNotAvailableError if not provided."""
        if self._expected_trajectory is None:
            raise DataNotAvailableError("expected_trajectory")
        return self._expected_trajectory

    @property
    def expected_outcome(self) -> Dict[str, Any]:
        """Expected outcome/state. Raises DataNotAvailableError if not provided."""
        if self._expected_outcome is None:
            raise DataNotAvailableError("expected_outcome")
        return self._expected_outcome

    # ==========================================================================
    # Guidelines Properties
    # ==========================================================================

    @property
    def success_criteria(self) -> str:
        """Success criteria text. Raises DataNotAvailableError if not provided."""
        if self._success_criteria is None:
            raise DataNotAvailableError("success_criteria")
        return self._success_criteria

    @property
    def prohibited_content(self) -> List[str]:
        """Prohibited content list. Raises DataNotAvailableError if not provided."""
        if self._prohibited_content is None:
            raise DataNotAvailableError("prohibited_content")
        return self._prohibited_content

    # ==========================================================================
    # Constraints Property
    # ==========================================================================

    @property
    def constraints(self) -> Optional[Constraints]:
        """
        Get constraints if set, otherwise None.

        Check if constraints exist before accessing specific limits:
            if ctx.constraints and ctx.constraints.has_latency_constraint():
                max_latency = ctx.constraints.max_latency_ms
        """
        return self._constraints

    # ==========================================================================
    # Task Property
    # ==========================================================================

    @property
    def task(self) -> "Task":
        """Original task. Raises DataNotAvailableError if not provided."""
        if self._task is None:
            raise DataNotAvailableError("task")
        return self._task

    # ==========================================================================
    # Custom Attributes (always available, defaults to empty dict)
    # ==========================================================================    @property
    def custom(self) -> Dict[str, Any]:
        """Custom attributes from dataset. Always available (empty dict if none)."""
        return self._custom

    def get_custom(self, key: str, default: Any = None) -> Any:
        """Get a custom attribute with optional default value."""
        return self._custom.get(key, default)

    # ==========================================================================
    # Availability Checks
    # ==========================================================================

    def has_expected_output(self) -> bool:
        """Check if expected_output is available."""
        return self._expected_output is not None

    def has_expected_trajectory(self) -> bool:
        """Check if expected_trajectory is available."""
        return self._expected_trajectory is not None

    def has_expected_outcome(self) -> bool:
        """Check if expected_outcome is available."""
        return self._expected_outcome is not None

    def has_success_criteria(self) -> bool:
        """Check if success_criteria is available."""
        return self._success_criteria is not None

    def has_prohibited_content(self) -> bool:
        """Check if prohibited_content is available."""
        return self._prohibited_content is not None

    def has_constraints(self) -> bool:
        """Check if any constraints are set."""
        return self._constraints is not None

    def has_task(self) -> bool:
        """Check if task is available."""
        return self._task is not None


@dataclass
class CompositeScore:
    """
    A weighted or binary combination of multiple evaluator scores.
    Determines if a trial reached a threshold of success.
    """

    composite_id: str
    trial_id: str

    # Component scores
    component_scores: Dict[str, EvalResult] = field(default_factory=dict)

    # Aggregation configuration
    aggregation_method: Literal["weighted_average", "minimum", "all_pass", "majority", "custom"] = "weighted_average"
    weights: Dict[str, float] = field(default_factory=dict)
    threshold: float = 0.7

    # Results
    final_score: float = 0.0
    passed: bool = False

    # Breakdown
    score_breakdown: Dict[str, float] = field(default_factory=dict)

    # Metadata
    calculated_at: datetime = field(default_factory=datetime.now)

    def calculate(self):
        """Calculate the composite score based on component scores."""
        if not self.component_scores:
            self.final_score = 0.0
            self.passed = False
            return

        scores = {name: result.score for name, result in self.component_scores.items()}
        self.score_breakdown = scores

        if self.aggregation_method == "weighted_average":
            total_weight = sum(self.weights.get(name, 1.0) for name in scores.keys())
            weighted_sum = sum(score * self.weights.get(name, 1.0) for name, score in scores.items())
            self.final_score = weighted_sum / total_weight if total_weight > 0 else 0.0

        elif self.aggregation_method == "minimum":
            self.final_score = min(scores.values())

        elif self.aggregation_method == "all_pass":
            self.final_score = 1.0 if all(r.passed for r in self.component_scores.values()) else 0.0

        elif self.aggregation_method == "majority":
            passed_count = sum(1 for r in self.component_scores.values() if r.passed)
            self.final_score = passed_count / len(self.component_scores)

        self.passed = self.final_score >= self.threshold


# ============================================================================
# DATASET & BENCHMARK MODELS
# ============================================================================


@dataclass
class Dataset:
    """
    A collection of tasks designed to measure specific capabilities or behaviors.
    Can be production traces, golden set, or synthetic data.
    """

    dataset_id: str
    name: str
    description: str

    # Tasks
    tasks: List[Task] = field(default_factory=list)

    # Classification
    dataset_type: Literal["golden_set", "production_traces", "synthetic", "human_annotated"] = "golden_set"
    domain: Optional[str] = None
    version: str = "1.0"

    # Source information
    source: Optional[str] = None  # File path, DB query, API endpoint
    source_filters: Dict[str, Any] = field(default_factory=dict)

    # Statistics
    task_count: int = 0
    difficulty_distribution: Dict[str, int] = field(default_factory=dict)

    # Metadata
    created_at: datetime = field(default_factory=datetime.now)
    updated_at: datetime = field(default_factory=datetime.now)
    created_by: Optional[str] = None
    tags: List[str] = field(default_factory=list)

    def add_task(self, task: Task):
        """Add a task to the dataset."""
        task.dataset_id = self.dataset_id
        self.tasks.append(task)
        self.task_count = len(self.tasks)

        # Update difficulty distribution
        difficulty = task.difficulty
        self.difficulty_distribution[difficulty] = self.difficulty_distribution.get(difficulty, 0) + 1


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


# ============================================================================
# UTILITY FUNCTIONS
# ============================================================================


def generate_id(prefix: str = "") -> str:
    """Generate a unique ID."""
    return f"{prefix}{uuid4().hex[:12]}" if prefix else uuid4().hex[:12]
