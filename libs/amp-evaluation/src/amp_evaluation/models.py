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

This module defines all the fundamental concepts:
- Agent: The AI system being evaluated
- Task: A test case with inputs and success criteria
- Trial: A single execution attempt of a task
- Trajectory: Step-by-step execution record from traces
- Outcome: Final state verification
- Metrics: Quantitative performance data
- EvalResult: Result from an evaluator
- EvalContext: Context provided to evaluators during evaluation
- Constraints: Performance constraints for evaluation
"""
from dataclasses import dataclass, field
from datetime import datetime
from typing import List, Dict, Any, Optional, Union, Literal, Callable, TYPE_CHECKING
from enum import Enum
from uuid import uuid4

if TYPE_CHECKING:
    from .trace.models import EvalTrace

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
# TRACE & TRAJECTORY MODELS (Based on AMP Attributes)
# ============================================================================

@dataclass
class LLMTokenUsage:
    """Token usage for LLM operations."""
    input_tokens: int = 0
    output_tokens: int = 0
    cache_read_input_tokens: int = 0
    total_tokens: int = 0

    def __add__(self, other: 'LLMTokenUsage') -> 'LLMTokenUsage':
        """Add two token usage objects."""
        return LLMTokenUsage(
            input_tokens=self.input_tokens + other.input_tokens,
            output_tokens=self.output_tokens + other.output_tokens,
            cache_read_input_tokens=self.cache_read_input_tokens + other.cache_read_input_tokens,
            total_tokens=self.total_tokens + other.total_tokens
        )


@dataclass
class SpanStatus:
    """Execution status of a span."""
    error: bool = False
    error_type: Optional[str] = None
    error_message: Optional[str] = None


@dataclass
class ToolCall:
    """Represents a tool invocation."""
    id: str
    name: str
    arguments: Dict[str, Any]
    result: Any = None
    duration_ms: Optional[float] = None
    status: Optional[SpanStatus] = None


@dataclass
class PromptMessage:
    """A message in a conversation."""
    role: Literal["system", "user", "assistant", "tool"]
    content: str
    tool_calls: List[ToolCall] = field(default_factory=list)


@dataclass
class RetrievedDocument:
    """Document retrieved during RAG."""
    id: str
    content: str
    score: float
    rank: int
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class Span:
    """
    A single span in the trace (based on AMP Attributes).
    Represents one operation: LLM call, tool execution, retrieval, etc.
    """
    span_id: str
    kind: Literal["llm", "tool", "embedding", "retriever", "rerank", "agent", "task", "unknown"]

    # I/O
    input: Any = None
    output: Any = None

    # Status
    status: SpanStatus = field(default_factory=SpanStatus)

    # Timing
    start_time: Optional[datetime] = None
    end_time: Optional[datetime] = None
    duration_ms: float = 0.0

    # Kind-specific data
    data: Dict[str, Any] = field(default_factory=dict)

    # Hierarchy
    parent_span_id: Optional[str] = None
    trace_id: Optional[str] = None

    # Name
    name: str = ""

    # Metadata
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class TrajectoryStep:
    """
    A processed step in the agent's execution trajectory.
    Extracted from spans for easier evaluation.
    """
    step_number: int
    step_type: str  # "llm", "tool", "retriever", "agent_iteration"

    # Content
    input: Any = None
    output: Any = None

    # For tool steps
    tool_name: Optional[str] = None
    tool_args: Optional[Dict[str, Any]] = None
    tool_result: Any = None

    # For LLM steps
    llm_model: Optional[str] = None
    llm_messages: List[PromptMessage] = field(default_factory=list)
    llm_tokens: Optional[LLMTokenUsage] = None

    # For retrieval steps
    retrieved_docs: List[RetrievedDocument] = field(default_factory=list)

    # Timing & status
    duration_ms: float = 0.0
    success: bool = True
    error: Optional[str] = None

    # Original span reference
    span_id: Optional[str] = None
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class Trajectory:
    """
    The step-by-step record of a trial's execution.
    Includes model reasoning, tool calls, intermediate results, and outputs.
    """
    trajectory_id: str
    trial_id: Optional[str] = None

    # Steps
    steps: List[TrajectoryStep] = field(default_factory=list)

    # Summary statistics
    total_steps: int = 0
    successful_steps: int = 0
    failed_steps: int = 0

    # Tool usage summary
    tools_used: List[str] = field(default_factory=list)
    tool_sequence: List[str] = field(default_factory=list)

    # Token usage
    total_tokens: LLMTokenUsage = field(default_factory=LLMTokenUsage)

    # Timing
    total_duration_ms: float = 0.0

    def add_step(self, step: TrajectoryStep):
        """Add a step to the trajectory."""
        step.step_number = len(self.steps) + 1
        self.steps.append(step)
        self.total_steps += 1

        if step.success:
            self.successful_steps += 1
        else:
            self.failed_steps += 1

        if step.tool_name and step.tool_name not in self.tools_used:
            self.tools_used.append(step.tool_name)

        if step.tool_name:
            self.tool_sequence.append(step.tool_name)

        if step.llm_tokens:
            self.total_tokens += step.llm_tokens

        self.total_duration_ms += step.duration_ms


@dataclass
class Trace:
    """
    Complete execution trace of an agent.
    Contains raw spans and can generate trajectory.
    """
    trace_id: str
    agent_id: str

    # I/O
    input: str
    output: str

    environment_id: Optional[str] = None

    # Spans (raw observability data)
    spans: List[Span] = field(default_factory=list)

    # Extracted trajectory (lazy computed)
    _trajectory: Optional[Trajectory] = None

    # Metadata
    timestamp: datetime = field(default_factory=datetime.now)
    metadata: Dict[str, Any] = field(default_factory=dict)

    # Trial association
    trial_id: Optional[str] = None
    task_id: Optional[str] = None

    @property
    def trajectory(self) -> Trajectory:
        """Extract or return cached trajectory."""
        if self._trajectory is None:
            self._trajectory = self._extract_trajectory()
        return self._trajectory

    def _extract_trajectory(self) -> Trajectory:
        """Extract trajectory from spans."""
        traj = Trajectory(
            trajectory_id=self.trace_id,
            trial_id=self.trial_id
        )

        for span in self.spans:
            step = TrajectoryStep(
                step_number=len(traj.steps) + 1,
                step_type=span.kind,
                input=span.input,
                output=span.output,
                duration_ms=span.duration_ms,
                success=not span.status.error,
                error=span.status.error_message,
                span_id=span.span_id,
                metadata=span.metadata
            )

            # Extract type-specific details
            if span.kind == "tool":
                step.tool_name = span.data.get("name")
                step.tool_args = span.input if isinstance(span.input, dict) else {}
                step.tool_result = span.output

            elif span.kind == "llm":
                step.llm_model = span.data.get("model")
                step.llm_tokens = span.data.get("tokenUsage")
                if isinstance(span.input, list):
                    step.llm_messages = span.input

            elif span.kind == "retriever":
                step.retrieved_docs = span.data.get("retrievedDocuments", [])

            traj.add_step(step)

        return traj


# ============================================================================
# TASK & TRIAL MODELS
# ============================================================================

@dataclass
class TaskInput:
    """Input specification for a task."""
    prompt: str
    context: Dict[str, Any] = field(default_factory=dict)
    files: List[str] = field(default_factory=list)
    parameters: Dict[str, Any] = field(default_factory=dict)


@dataclass
class TaskSuccessCriteria:
    """Success criteria for evaluating a task."""
    # Expected outputs
    expected_output: Optional[str] = None
    acceptable_outputs: List[str] = field(default_factory=list)

    # Expected behavior
    expected_tool_sequence: List[str] = field(default_factory=list)
    required_tools: List[str] = field(default_factory=list)

    # Content requirements
    required_content: List[str] = field(default_factory=list)
    prohibited_content: List[str] = field(default_factory=list)

    # Performance constraints
    max_latency_ms: Optional[float] = None
    max_tokens: Optional[int] = None
    max_cost_usd: Optional[float] = None
    max_iterations: Optional[int] = None

    # Custom validator
    custom_validator: Optional[Callable] = None


@dataclass
class Task:
    """
    A single test case with defined inputs and success criteria.
    The fundamental building block of evaluation.
    """
    task_id: str
    name: str
    description: str

    # Input
    input: TaskInput

    # Success criteria
    success_criteria: TaskSuccessCriteria = field(default_factory=TaskSuccessCriteria)

    # Classification
    task_type: str = "general"  # "qa", "code_gen", "rag", "tool_use", "math"
    difficulty: Literal["easy", "medium", "hard", "expert"] = "medium"
    tags: List[str] = field(default_factory=list)
    domain: Optional[str] = None  # "medical", "legal", "technical", etc.

    # Expected data (ground truth)
    expected_output: Optional[str] = None
    expected_trajectory: Optional[Trajectory] = None  # Golden trajectory
    expected_documents: List[Dict[str, Any]] = field(default_factory=list)
    expected_outcome: Optional[Dict[str, Any]] = None  # Expected side effects/state
    success_criteria_text: Optional[str] = None  # Human-readable success criteria for LLM judges
    prohibited_content: Optional[List[str]] = None  # Content that should NOT appear
    
    # Constraints
    max_latency_ms: Optional[float] = None
    max_tokens: Optional[int] = None
    max_iterations: Optional[int] = None
    
    # Custom attributes (passed through to evaluators)
    custom: Dict[str, Any] = field(default_factory=dict)

    # Dataset membership
    dataset_id: Optional[str] = None

    # Metadata
    created_at: datetime = field(default_factory=datetime.now)
    created_by: Optional[str] = None
    metadata: Dict[str, Any] = field(default_factory=dict)


class TrialStatus(Enum):
    """Execution status of a trial."""
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    TIMEOUT = "timeout"
    CANCELLED = "cancelled"


@dataclass
class Outcome:
    """
    The final state of the environment or "external reality" after a trial.
    Used to verify that the agent actually accomplished the task.

    Example: Did the booking actually get placed? Was the file created?
    """
    outcome_id: str
    trial_id: str

    # Success determination
    success: bool
    success_reason: str = ""

    # State verification
    expected_state: Dict[str, Any] = field(default_factory=dict)
    actual_state: Dict[str, Any] = field(default_factory=dict)
    state_diff: Dict[str, Any] = field(default_factory=dict)

    # Evidence (screenshots, API responses, database state, etc.)
    evidence: Dict[str, Any] = field(default_factory=dict)

    # Validation metadata
    validated_by: Optional[str] = None  # "code", "human", "model"
    validated_at: datetime = field(default_factory=datetime.now)

    # Additional context
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class Trial:
    """
    A single execution attempt of a task.
    Multiple trials are run per task to account for model non-determinism.
    """
    trial_id: str
    task_id: str
    trial_number: int  # Which attempt (1, 2, 3, ...)

    # Agent configuration
    agent_id: str
    agent_config: Dict[str, Any] = field(default_factory=dict)
    environment_id: Optional[str] = None

    # Execution data
    trace_id: Optional[str] = None
    trace: Optional[Trace] = None

    # Outcome verification
    outcome: Optional[Outcome] = None

    # Status
    status: TrialStatus = TrialStatus.PENDING

    # Timing
    started_at: Optional[datetime] = None
    completed_at: Optional[datetime] = None
    duration_ms: Optional[float] = None

    # Evaluation results (populated by evaluators)
    evaluation_results: Dict[str, 'EvalResult'] = field(default_factory=dict)

    # Metadata
    metadata: Dict[str, Any] = field(default_factory=dict)

    @property
    def trajectory(self) -> Optional[Trajectory]:
        """Get trajectory from trace."""
        if self.trace:
            return self.trace.trajectory
        return None


# ============================================================================
# METRICS MODELS
# ============================================================================

@dataclass
class Metrics:
    """
    Quantitative metadata extracted directly from the system.
    Includes latency, token usage, cost, and iteration count.
    """
    # Performance
    latency_ms: float = 0.0
    latency_p50: Optional[float] = None
    latency_p95: Optional[float] = None
    latency_p99: Optional[float] = None

    # Token usage
    total_tokens: int = 0
    input_tokens: int = 0
    output_tokens: int = 0
    cache_tokens: int = 0

    # Cost estimation
    estimated_cost_usd: float = 0.0
    cost_per_token: float = 0.0

    # Agent behavior
    iteration_count: int = 0
    tool_call_count: int = 0
    llm_call_count: int = 0
    retrieval_count: int = 0
    unique_tools_used: int = 0

    # Success rates
    tool_success_rate: float = 1.0
    step_success_rate: float = 1.0

    # Error tracking
    error_count: int = 0
    retry_count: int = 0
    timeout_count: int = 0

    # Custom metrics
    custom: Dict[str, Union[int, float, str]] = field(default_factory=dict)

    @classmethod
    def from_trace(cls, trace: Trace) -> 'Metrics':
        """Extract metrics from a trace."""
        traj = trace.trajectory

        return cls(
            latency_ms=traj.total_duration_ms,
            total_tokens=traj.total_tokens.total_tokens,
            input_tokens=traj.total_tokens.input_tokens,
            output_tokens=traj.total_tokens.output_tokens,
            cache_tokens=traj.total_tokens.cache_read_input_tokens,
            tool_call_count=len(traj.tool_sequence),
            unique_tools_used=len(traj.tools_used),
            llm_call_count=sum(1 for s in traj.steps if s.step_type == "llm"),
            retrieval_count=sum(1 for s in traj.steps if s.step_type == "retriever"),
            error_count=traj.failed_steps,
            step_success_rate=traj.successful_steps / max(traj.total_steps, 1)
        )

    @classmethod
    def from_trial(cls, trial: Trial) -> 'Metrics':
        """Extract metrics from a trial."""
        if trial.trace:
            return cls.from_trace(trial.trace)
        return cls()


# ============================================================================
# EVALUATION RESULT MODELS
# ============================================================================

@dataclass
class EvalResult:
    """Result from a single evaluator."""
    evaluator_name: str
    target_id: str  # ID of what was evaluated
    target_type: Literal["trace", "trial", "task", "trajectory", "outcome"]

    # Score
    score: float  # 0.0 to 1.0
    passed: bool = True

    # Explanation
    explanation: str = ""
    reasoning_steps: List[str] = field(default_factory=list)

    # Evidence & details
    evidence: Dict[str, Any] = field(default_factory=dict)
    details: Dict[str, Any] = field(default_factory=dict)

    # Evaluator metadata
    evaluator_type: Literal["code", "model", "human"] = "code"
    evaluator_version: Optional[str] = None

    # Timing
    evaluated_at: datetime = field(default_factory=datetime.now)
    evaluation_duration_ms: float = 0.0

    # Additional context
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class Constraints:
    """
    Quantitative constraints for evaluation.
    
    Used to specify performance limits that the agent should meet.
    These are optional - evaluators can check if constraints are set.
    """
    max_latency_ms: Optional[float] = None
    max_tokens: Optional[int] = None
    max_iterations: Optional[int] = None
    
    def has_latency_constraint(self) -> bool:
        """Check if latency constraint is set."""
        return self.max_latency_ms is not None
    
    def has_token_constraint(self) -> bool:
        """Check if token constraint is set."""
        return self.max_tokens is not None
    
    def has_iteration_constraint(self) -> bool:
        """Check if iteration constraint is set."""
        return self.max_iterations is not None


@dataclass
class EvalContext:
    """
    Context provided to evaluators during evaluation.
    
    Contains observed execution trace, expected values, and constraints.
    Accessing unavailable fields raises DataNotAvailableError.
    Use has_*() methods to check availability before accessing.
    
    Structure:
        trace: The observed execution (required, EvalTrace)
        expected_*: Ground truth values (optional, from dataset)
        guidelines: Qualitative evaluation guidance (optional)
        constraints: Quantitative performance limits (optional)
        custom: User-defined attributes (optional)
    
    Always available:
        - trace: The observed execution trace (EvalTrace)
        - trace_id: ID of the trace (convenience - same as trace.trace_id)
        - timestamp: Start time of the trace (convenience - same as trace.timestamp)
        - metrics: Aggregated metrics (convenience - same as trace.metrics)
        - input: Trace input (convenience - same as trace.input)
        - output: Trace output (convenience - same as trace.output)
        - is_benchmark: True if running with dataset (BenchmarkRunner)
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
            # Useful for LiveRunner - track performance over time
            hour_of_day = ctx.timestamp.hour
            # Adjust thresholds based on load patterns...
    """
    # ==========================================================================
    # OBSERVED - Always required
    # ==========================================================================
    trace: 'EvalTrace'  # The observed execution trace
    is_benchmark: bool = False
    
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
    _task: Optional['Task'] = field(default=None, repr=False)
    
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
    def task(self) -> 'Task':
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
class DatasetSchema:
    """
    Defines what fields are present in a dataset.
    Makes datasets flexible - user chooses what to include.
    """
    # Required
    has_input: bool = True  # Always need input!

    # Optional expected data (ground truth)
    has_expected_output: bool = False
    has_expected_trajectory: bool = False
    has_expected_tools: bool = False
    has_expected_outcome: bool = False

    # Optional success criteria
    has_required_content: bool = False
    has_prohibited_content: bool = False
    has_max_latency: bool = False
    has_max_tokens: bool = False
    has_max_cost: bool = False
    has_max_iterations: bool = False

    # Custom fields
    custom_fields: List[str] = field(default_factory=list)


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


@dataclass
class BenchmarkEnvironment:
    """
    The specific environment where the agent runs.
    Ensures reproducible evaluation conditions.
    """
    environment_id: str
    name: str
    description: str

    # Environment specification
    os: Optional[str] = None
    runtime: Optional[str] = None
    python_version: Optional[str] = None
    dependencies: Dict[str, str] = field(default_factory=dict)

    # Setup
    setup_script: Optional[str] = None
    teardown_script: Optional[str] = None
    docker_image: Optional[str] = None

    # Configuration
    config: Dict[str, Any] = field(default_factory=dict)
    env_vars: Dict[str, str] = field(default_factory=dict)


@dataclass
class Benchmark:
    """
    A standardized evaluation suite with datasets, environment, and evaluation logic.
    Designed to objectively measure and compare AI system performance.
    """
    benchmark_id: str
    name: str
    description: str

    # The Dataset (The "What") - What tasks to run
    datasets: List[Dataset] = field(default_factory=list)

    # The Environment (The "Where") - Where agent runs
    environment: Optional[BenchmarkEnvironment] = None

    # The Evaluation Logic (The "How") - How to score
    evaluator_names: List[str] = field(default_factory=list)
    aggregation_config: Dict[str, Any] = field(default_factory=dict)

    # Scoring
    passing_threshold: float = 0.7
    scoring_criteria: Dict[str, Any] = field(default_factory=dict)

    # Leaderboard
    leaderboard_enabled: bool = False
    public: bool = False

    # Metadata
    version: str = "1.0"
    created_by: Optional[str] = None
    created_at: datetime = field(default_factory=datetime.now)
    license: Optional[str] = None
    citation: Optional[str] = None

    def get_all_tasks(self) -> List[Task]:
        """Get all tasks across all datasets."""
        return [task for dataset in self.datasets for task in dataset.tasks]

    @property
    def total_tasks(self) -> int:
        """Total number of tasks in benchmark."""
        return sum(d.task_count for d in self.datasets)



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
