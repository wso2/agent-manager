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
# OBSERVATION MODEL
# ============================================================================


@dataclass
class Observation:
    """
    What we observed from the agent's execution.

    This is ALWAYS available - for both Experiments and Monitors.
    Contains the complete execution trace and all derived metrics.

    Source: Trace data from OpenTelemetry

    Example:
        observation = Observation(trajectory=trajectory)

        # Access observed data
        output = observation.output
        latency = observation.metrics.total_duration_ms
        steps = observation.trajectory.steps
    """

    # Core trace data
    trajectory: "Trajectory"

    # ==========================================================================
    # Convenience Properties (delegated from trajectory)
    # ==========================================================================

    @property
    def trace_id(self) -> str:
        """Unique identifier for this execution."""
        return self.trajectory.trace_id

    @property
    def input(self) -> str:
        """What was sent to the agent (observed input)."""
        return self.trajectory.input

    @property
    def output(self) -> str:
        """What the agent produced (observed output)."""
        return self.trajectory.output

    @property
    def metrics(self):
        """Performance metrics (latency, tokens, cost, etc.)."""
        return self.trajectory.metrics

    @property
    def timestamp(self) -> Optional[datetime]:
        """When the execution started."""
        return self.trajectory.timestamp

    @property
    def steps(self) -> List:
        """Sequential execution steps (LLM calls, tool calls, etc.)."""
        return self.trajectory.steps

    @property
    def tool_spans(self) -> List:
        """Tool call spans from the trajectory."""
        return self.trajectory.tool_spans


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
