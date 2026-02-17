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
Evaluation runners for the framework.

Two evaluation scenarios:

1. Experiment - Evaluation with a dataset of predefined tasks
   - You have a set of test cases (questions/prompts with expected answers)
   - Runner INVOKES the agent with each task
   - Gets traces and evaluates against ground truth
   - Use case: "Test my agent against these 100 questions"

2. Monitor - Evaluation on live production traces
   - No predefined tasks - uses live traffic
   - Runner FETCHES traces from API for a time period
   - Evaluates without ground truth (quality metrics, latency, errors)
   - Use case: "Evaluate last 24 hours of production traffic"

Both runners share the same evaluation logic but differ in:
- How they get traces (invoke agent vs fetch from API)
- Whether they have ground truth (tasks with reference answers)

Configuration styles:
- Config-driven: Pass config parameter (loads from env variables)
- Programmatic: Pass individual parameters (overrides config)

Examples:
    # Config-driven (loads from environment variables)
    config = Config()  # Loads AMP_API_URL etc from env
    runner = Monitor(config=config)
    result = runner.run(limit=1000)

    # Programmatic (explicit parameters)
    runner = Monitor(
        trace_service_url="http://traces-api:8001",
        exclude_tags=["llm-judge"]
    )
    result = runner.run(start_time="2024-01-26T00:00:00")

    # Hybrid (config + overrides)
    runner = Monitor(
        config=config,
        trace_service_url="http://custom-url:9000",  # Overrides config
        include_tags=["quality"]
    )

    # Benchmark evaluation
    runner = Experiment(
        invoker=my_agent_invoker,
        dataset=Dataset.from_csv("test_cases.csv")
    )
    result = runner.run()
"""

from typing import List, Dict, Optional, Any, TYPE_CHECKING
from dataclasses import dataclass, field
from datetime import datetime
from abc import ABC, abstractmethod
from enum import Enum
import logging

from .trace import Trace, parse_trace_for_evaluation, TraceFetcher, TraceLoader
from .registry import get_registry, get_evaluator
from .evaluators.base import BaseEvaluator
from .models import EvaluatorSummary, EvaluatorScore
from .dataset.schema import Task, Dataset
from .aggregators.base import normalize_aggregations
from .config import Config

if TYPE_CHECKING:
    from .invokers import AgentInvoker, InvokeResult


logger = logging.getLogger(__name__)

# Initialize RequestsInstrumentor once at module level to avoid duplicate instrumentation
_requests_instrumented = False


def _ensure_requests_instrumented():
    """Ensure requests library is instrumented exactly once."""
    global _requests_instrumented
    if not _requests_instrumented:
        try:
            from opentelemetry.instrumentation.requests import RequestsInstrumentor

            RequestsInstrumentor().instrument()
            _requests_instrumented = True
            logger.debug("RequestsInstrumentor initialized")
        except ImportError:
            logger.debug("OpenTelemetry requests instrumentation not available")
        except Exception as e:
            logger.warning(f"Failed to instrument requests library: {e}")


# ============================================================================
# RUN TYPE
# ============================================================================


class RunType(str, Enum):
    """Type of evaluation run."""

    EXPERIMENT = "experiment"
    MONITOR = "monitor"


# ============================================================================
# RUN RESULT
# ============================================================================


@dataclass
class RunResult:
    """Result of an evaluation run."""

    run_id: str
    run_type: RunType  # EXPERIMENT or MONITOR
    started_at: datetime
    completed_at: Optional[datetime] = None

    # Context information
    agent_uid: Optional[str] = None
    environment_uid: Optional[str] = None
    dataset_id: Optional[str] = None  # For experiments

    # Counts
    traces_evaluated: int = 0
    evaluators_run: int = 0

    # Per-evaluator aggregated results (new structure)
    scores: Dict[str, EvaluatorSummary] = field(default_factory=dict)

    # Errors encountered
    errors: List[str] = field(default_factory=list)

    # Metadata (additional context)
    metadata: Dict[str, Any] = field(default_factory=dict)

    @property
    def duration_seconds(self) -> float:
        if self.completed_at and self.started_at:
            return (self.completed_at - self.started_at).total_seconds()
        return 0.0

    @property
    def success(self) -> bool:
        return len(self.errors) == 0

    def summary(self) -> str:
        """Get a human-readable summary."""
        lines = [
            f"Evaluation Run: {self.run_id} ({self.run_type})",
            f"  Started: {self.started_at.isoformat()}",
            f"  Duration: {self.duration_seconds:.2f}s",
        ]

        # Add context information
        if self.agent_uid:
            lines.append(f"  Agent: {self.agent_uid}")
        if self.environment_uid:
            lines.append(f"  Environment: {self.environment_uid}")
        if self.dataset_id:
            lines.append(f"  Dataset: {self.dataset_id}")

        lines.extend(
            [
                "",
                f"Traces evaluated: {self.traces_evaluated}",
                f"Evaluators run: {self.evaluators_run}",
                f"Errors: {len(self.errors)}",
                "",
                "Scores:",
            ]
        )

        for name, summary in self.scores.items():
            lines.append(f"  {name}:")
            # Show all available aggregations
            for agg_name, value in summary.aggregated_scores.items():
                if isinstance(value, (int, float)):
                    lines.append(f"    {agg_name}: {value:.4f}")
                else:
                    lines.append(f"    {agg_name}: {value}")
            # Show count
            lines.append(f"    count: {summary.count}")

        if self.errors:
            lines.append("")
            lines.append(f"Errors ({len(self.errors)}):")
            for error in self.errors[:5]:  # Show first 5 errors
                lines.append(f"  - {error}")
            if len(self.errors) > 5:
                lines.append(f"  ... and {len(self.errors) - 5} more")

        return "\n".join(lines)


# ============================================================================
# BASE RUNNER
# ============================================================================


class BaseRunner(ABC):
    """
    Base class for evaluation runners.

    Handles evaluator loading, filtering, and result aggregation.
    Subclasses implement how to get traces.

    Supports two configuration styles:
    1. Config-driven: Pass config parameter (loads from env variables)
    2. Programmatic: Pass individual parameters (overrides config)
    """

    def __init__(
        self,
        config: Optional[Config] = None,
        trace_fetcher: Optional[TraceFetcher] = None,
        trace_service_url: Optional[str] = None,
        include: Optional[List[str]] = None,
        exclude: Optional[List[str]] = None,
        include_tags: Optional[List[str]] = None,
        exclude_tags: Optional[List[str]] = None,
    ):
        """
        Initialize runner with evaluator filtering options.

        Args:
            config: Config object (loads from env if None). Individual params override this.
            trace_fetcher: Optional TraceFetcher instance (overrides config-based creation)
            trace_service_url: Optional trace service URL (overrides config, used if no trace_fetcher)
            include: Only run these evaluators (by name)
            exclude: Don't run these evaluators (by name)
            include_tags: Only run evaluators with ANY of these tags
            exclude_tags: Don't run evaluators with ANY of these tags

        Config priority (highest to lowest):
            1. Explicit parameters passed to __init__
            2. Config object passed to __init__
            3. Default Config (from environment variables)
        """
        # Store or create config (from environment)
        self.config = config if config is not None else Config()

        # Trace fetcher (lazy initialization)
        self._trace_fetcher = trace_fetcher
        self._trace_service_url = trace_service_url
        self._fetcher_instance: Optional[Any] = None  # Can be TraceFetcher or TraceLoader

        # Individual parameters override config
        self.include = set(include) if include else None
        self.exclude = set(exclude) if exclude else set()
        self.include_tags = set(include_tags) if include_tags else None
        self.exclude_tags = set(exclude_tags) if exclude_tags else set()

        # Validate no conflicts between include and exclude
        if self.include and self.exclude:
            conflicts = self.include & self.exclude
            if conflicts:
                raise ValueError(f"Evaluator names cannot be in both include and exclude lists. Conflicts: {conflicts}")

        if self.include_tags and self.exclude_tags:
            tag_conflicts = self.include_tags & self.exclude_tags
            if tag_conflicts:
                raise ValueError(
                    f"Tags cannot be in both include_tags and exclude_tags lists. Conflicts: {tag_conflicts}"
                )

        # Resolve which evaluators to use
        self._evaluators: List[BaseEvaluator] = []
        self._load_evaluators()

    def _get_fetcher(self) -> Any:
        """
        Get or create the trace fetcher instance (lazy initialization).

        Supports both TraceLoader (for local files) and TraceFetcher (for platform API).

        Priority (highest to lowest):
            1. Explicit trace_fetcher passed to __init__
            2. Explicit trace_service_url passed to __init__
            3. Create from config:
               - File mode: TraceLoader (if trace_file_path is set)
               - Platform mode: TraceFetcher (if api_url is set)

        Returns:
            TraceFetcher or TraceLoader instance

        Raises:
            ValueError: If no fetcher can be created from provided config
        """
        if self._fetcher_instance is not None:
            return self._fetcher_instance

        if self._trace_fetcher:
            # Explicit fetcher takes highest priority
            self._fetcher_instance = self._trace_fetcher
        elif self._trace_service_url:
            # Explicit URL overrides config, use agent info from config
            self._fetcher_instance = TraceFetcher(
                base_url=self._trace_service_url,
                agent_uid=self.config.agent.agent_uid,
                environment_uid=self.config.agent.environment_uid,
            )
        else:
            # Use config-based fetcher selection
            trace_config = self.config.trace_loader

            # File mode: try to use TraceLoader
            if trace_config.mode == "file" and trace_config.trace_file_path:
                logger.info(f"Using TraceLoader with file: {trace_config.trace_file_path}")
                self._fetcher_instance = TraceLoader(
                    file_path=trace_config.trace_file_path,
                    agent_uid=self.config.agent.agent_uid,
                    environment_uid=self.config.agent.environment_uid,
                )
            # Platform mode: use TraceFetcher
            elif self.config.platform.api_url:
                logger.info(f"Using TraceFetcher with platform API: {self.config.platform.api_url}")
                self._fetcher_instance = TraceFetcher(
                    base_url=self.config.platform.api_url,
                    agent_uid=self.config.agent.agent_uid,
                    environment_uid=self.config.agent.environment_uid,
                )
            else:
                raise ValueError(
                    "Cannot create trace fetcher. Either:\n"
                    "  1. Pass trace_fetcher or trace_service_url to runner __init__,\n"
                    "  2. Set AMP_API_URL environment variable (for platform mode), or\n"
                    "  3. Set AMP_TRACE_LOADER_MODE=file and AMP_TRACE_LOADER_TRACE_FILE_PATH=/path/to/traces.json"
                )

        return self._fetcher_instance

    def _fetch_traces(self, start_time: str, end_time: str, limit: int = 100, offset: int = 0) -> List[Trace]:
        """
        Unified interface to fetch traces from either TraceFetcher or TraceLoader.

        Args:
            start_time: Start time in ISO 8601 format
            end_time: End time in ISO 8601 format
            limit: Maximum number of traces to fetch
            offset: Number of traces to skip (used by TraceFetcher)

        Returns:
            List of Trace objects
        """
        fetcher = self._get_fetcher()

        # Handle TraceLoader (uses load_batch interface)
        if isinstance(fetcher, TraceLoader):
            return fetcher.load_batch(limit=limit, start_time=start_time, end_time=end_time)
        # Handle TraceFetcher (uses fetch_traces interface)
        else:
            return fetcher.fetch_traces(start_time=start_time, end_time=end_time, limit=limit, offset=offset)

    def _load_evaluators(self):
        """Load evaluators from registry based on filters."""
        registry = get_registry()
        all_names = registry.list_evaluators()

        # Start with all or just included
        if self.include:
            names = [n for n in all_names if n in self.include]
        else:
            names = all_names.copy()

        # Filter by include_tags (keep if has ANY of the tags)
        if self.include_tags:
            filtered = []
            for name in names:
                meta = registry.get_metadata(name)
                tags = set(meta.get("tags", []))
                if tags & self.include_tags:
                    filtered.append(name)
            names = filtered

        # Filter by exclude_tags (remove if has ANY of the tags)
        if self.exclude_tags:
            filtered = []
            for name in names:
                meta = registry.get_metadata(name)
                tags = set(meta.get("tags", []))
                if not (tags & self.exclude_tags):
                    filtered.append(name)
            names = filtered

        # Apply exclude list
        names = [n for n in names if n not in self.exclude]

        # Load evaluator instances
        for name in names:
            try:
                evaluator = get_evaluator(name)
                self._evaluators.append(evaluator)
            except Exception as e:
                logger.warning(f"Could not load evaluator '{name}': {e}")

    @property
    def evaluator_names(self) -> List[str]:
        """Get list of evaluator names that will run."""
        return [e.name for e in self._evaluators]

    @property
    def evaluator_count(self) -> int:
        """Get number of evaluators that will run."""
        return len(self._evaluators)

    def evaluate_trace(
        self, trace: Trace, task: Optional[Task] = None, trial_id: Optional[str] = None
    ) -> Dict[str, List[EvaluatorScore]]:
        """
        Run all evaluators on a single trace.

        Evaluators handle their own level dispatching internally, returning
        one or more results based on their configured level.

        Args:
            trace: The Trace to evaluate
            task: Optional task for reference/ground truth data
            trial_id: Optional trial identifier for experiments

        Returns:
            Dict mapping evaluator name to list of EvaluatorScore objects
        """
        scores = {}

        for evaluator in self._evaluators:
            try:
                # Call evaluator - it handles level dispatching internally
                eval_results = evaluator(trace, task)

                # Convert EvalResult list to EvaluatorScore list
                evaluator_scores = []
                for eval_result in eval_results:
                    if eval_result.is_error:
                        score = EvaluatorScore(
                            trace_id=trace.trace_id,
                            score=0.0,
                            passed=False,
                            timestamp=trace.timestamp,
                            explanation=eval_result.explanation,
                            task_id=task.task_id if task else None,
                            trial_id=trial_id,
                            metadata=eval_result.details or {},
                            error=eval_result.error,
                        )
                    else:
                        score = EvaluatorScore(
                            trace_id=trace.trace_id,
                            score=eval_result.score,
                            passed=eval_result.passed,
                            timestamp=trace.timestamp,
                            explanation=eval_result.explanation,
                            task_id=task.task_id if task else None,
                            trial_id=trial_id,
                            metadata=eval_result.details or {},
                            error=None,
                        )
                    evaluator_scores.append(score)

                scores[evaluator.name] = evaluator_scores

            except Exception as e:
                # Unexpected exception during evaluation
                error_score = EvaluatorScore(
                    trace_id=trace.trace_id,
                    score=0.0,
                    passed=False,
                    timestamp=trace.timestamp,
                    explanation=f"Evaluator raised exception: {str(e)}",
                    task_id=task.task_id if task else None,
                    trial_id=trial_id,
                    metadata={},
                    error=str(e),
                )
                scores[evaluator.name] = [error_score]

        return scores

    def _evaluate_traces(
        self,
        traces: List[Trace],
        tasks: Optional[Dict[str, Task]] = None,
        trial_info: Optional[Dict[str, str]] = None,
    ) -> RunResult:
        """
        Internal method to evaluate a list of traces.

        Args:
            traces: List of Trace objects
            tasks: Optional dict mapping trace_id to Task
            trial_info: Optional dict mapping trace_id to trial_id (for experiments)

        Returns:
            RunResult with aggregated scores
        """
        from .dataset import generate_id

        run_id = generate_id("run")
        started_at = datetime.now()

        result = RunResult(
            run_id=run_id,
            run_type=self.run_type,
            started_at=started_at,
            evaluators_run=len(self._evaluators),
            agent_uid=self.config.agent.agent_uid,
            environment_uid=self.config.agent.environment_uid,
        )

        # Collect scores by evaluator name
        scores_by_evaluator: Dict[str, List[EvaluatorScore]] = {e.name: [] for e in self._evaluators}

        # Evaluate each trace
        for trace in traces:
            task = tasks.get(trace.trace_id) if tasks else None
            trial_id = trial_info.get(trace.trace_id) if trial_info else None

            try:
                # evaluate_trace returns Dict[str, List[EvaluatorScore]]
                trace_scores = self.evaluate_trace(trace, task, trial_id=trial_id)
                result.traces_evaluated += 1

                # Collect scores by evaluator (extend for multi-level support)
                for evaluator_name, score_list in trace_scores.items():
                    scores_by_evaluator[evaluator_name].extend(score_list)

            except Exception as e:
                error_msg = f"Error evaluating trace {trace.trace_id}: {e}"
                result.errors.append(error_msg)
                logger.error(error_msg)

        # Aggregate results
        result.scores = self._get_aggregated_scores(scores_by_evaluator)
        result.completed_at = datetime.now()

        return result

    def _get_aggregated_scores(
        self, scores_by_evaluator: Dict[str, List[EvaluatorScore]]
    ) -> Dict[str, EvaluatorSummary]:
        """
        Compute aggregated scores for all evaluators.

        Args:
            scores_by_evaluator: Dict mapping evaluator name to list of EvaluatorScore objects

        Returns:
            Dict mapping evaluator name to EvaluatorSummary
        """
        # Build evaluator lookup for getting aggregation config
        evaluator_by_name = {e.name: e for e in self._evaluators}

        summaries = {}

        for evaluator_name, all_scores in scores_by_evaluator.items():
            # Get evaluator for aggregation config
            evaluator = evaluator_by_name.get(evaluator_name)
            aggregations = getattr(evaluator, "aggregations", None) if evaluator else None

            # Separate successful scores from errors
            successful_scores = [s for s in all_scores if not s.is_error]
            error_count = len(all_scores) - len(successful_scores)

            if error_count > 0:
                logger.warning(
                    f"Evaluator '{evaluator_name}' had {error_count} errors out of {len(all_scores)} evaluations"
                )

            # Normalize aggregations to List[Aggregation]
            agg_list = normalize_aggregations(aggregations)

            # Extract score values (only from successful evaluations)
            score_values = [s.score for s in successful_scores]

            # Compute each aggregation
            aggregated_scores = {}
            for agg in agg_list:
                try:
                    value = agg.compute(score_values)
                    aggregated_scores[agg.name] = value
                except Exception as e:
                    logger.warning(f"Failed to compute {agg.name} for {evaluator_name}: {e}")
                    # Skip failed aggregations - don't add 0.0 as it's misleading

            # Calculate items per trace for multi-level evaluators
            items_per_trace = {}
            for score in all_scores:
                trace_id = score.trace_id
                items_per_trace[trace_id] = items_per_trace.get(trace_id, 0) + 1

            # Get evaluator level
            level = evaluator.level if evaluator else "trace"

            # Create EvaluatorSummary
            summary = EvaluatorSummary(
                evaluator_name=evaluator_name,
                count=len(all_scores),
                aggregated_scores=aggregated_scores,
                individual_scores=all_scores,
                level=level,
                items_per_trace=items_per_trace if any(count > 1 for count in items_per_trace.values()) else None,
            )
            summaries[evaluator_name] = summary

        return summaries

    @property
    @abstractmethod
    def run_type(self) -> RunType:
        """Return the type of run: RunType.EXPERIMENT or RunType.MONITOR."""
        pass

    @abstractmethod
    def run(self, **kwargs) -> RunResult:
        """Run the evaluation. Subclasses implement this."""
        pass


# ============================================================================
# BENCHMARK RUNNER
# ============================================================================


class Experiment(BaseRunner):
    """
    Evaluation runner for benchmark/dataset-based testing.

    This runner:
    1. Takes a dataset of tasks (prompts with expected answers)
    2. INVOKES the agent with each task using an AgentInvoker
    3. Collects traces from the agent execution via time-based batch fetching
    4. Evaluates traces against ground truth

    Use this when you have a predefined set of test cases.

    Example:
        class MyInvoker(AgentInvoker):
            def invoke(self, task: Task) -> InvokeResult:
                response = requests.post(url, json=task.input)
                return InvokeResult(output=response.json())

        experiment = Experiment(
            invoker=MyInvoker(),
            dataset=dataset,
        )
        result = experiment.run()
    """

    def __init__(
        self,
        invoker: "AgentInvoker",
        dataset: Optional[Dataset] = None,
        trials_per_task: int = 1,
        trace_fetch_wait_seconds: float = 60.0,
        config: Optional[Config] = None,
        trace_fetcher: Optional[TraceFetcher] = None,
        trace_service_url: Optional[str] = None,
        include: Optional[List[str]] = None,
        exclude: Optional[List[str]] = None,
        include_tags: Optional[List[str]] = None,
        exclude_tags: Optional[List[str]] = None,
    ):
        """
        Initialize benchmark runner.

        Args:
            invoker: AgentInvoker instance for batch-friendly invocation
            dataset: Dataset with test tasks
            trials_per_task: Number of times to run each task (default: 1)
            trace_fetch_wait_seconds: Seconds to wait before batch fetching traces (default: 60.0)
            config: Config object (loads from env if None)
            trace_fetcher: Optional TraceFetcher (created from config if not provided)
            trace_service_url: Optional trace service URL (overrides config, used if no trace_fetcher)
            include: Only run these evaluators (by name)
            exclude: Don't run these evaluators (by name)
            include_tags: Only run evaluators with these tags
            exclude_tags: Don't run evaluators with these tags
        """
        super().__init__(
            config=config,
            trace_fetcher=trace_fetcher,
            trace_service_url=trace_service_url,
            include=include,
            exclude=exclude,
            include_tags=include_tags,
            exclude_tags=exclude_tags,
        )

        self.invoker = invoker
        self.dataset = dataset
        self.trials_per_task = trials_per_task
        self.trace_fetch_wait_seconds = trace_fetch_wait_seconds

    @dataclass
    class _TaskResult:
        """
        Internal structure for a single task trial during an experiment run.

        Tracks the full lifecycle: invocation result + matched trace.
        Eliminates tuple juggling between phases of _run_with_invoker.
        """

        task: Task
        trial_id: str
        invoke_result: "InvokeResult"

    @property
    def run_type(self) -> RunType:
        return RunType.EXPERIMENT

    def run(
        self,
        dataset: Optional[Dataset] = None,
        traces: Optional[List[Trace]] = None,
    ) -> RunResult:
        """
        Run benchmark evaluation.

        Args:
            dataset: Optional dataset (overrides constructor dataset)
            traces: Pre-fetched Trace objects (skip agent invocation and trace fetching)

        Returns:
            RunResult with aggregated scores
        """
        # If traces are provided directly, skip invocation and use them
        if traces:
            # Build task mapping if we have a dataset
            tasks_by_trace_id = None
            if dataset or self.dataset:
                ds = dataset or self.dataset
                # Try to match traces to tasks by trace_id == task_id (convention)
                tasks_by_trace_id = {task.task_id: task for task in ds.tasks}

            return self._evaluate_traces(
                traces=traces,
                tasks=tasks_by_trace_id,
            )

        # Normal invocation flow
        dataset = dataset or self.dataset
        if not dataset:
            raise ValueError("No dataset provided. Pass dataset to constructor or run().")

        return self._run_with_invoker(dataset)

    def _run_with_invoker(self, dataset: Dataset) -> RunResult:
        """
        Run experiment using AgentInvoker pattern.

        Orchestrates the experiment lifecycle:
        1. Invoke agent for all tasks, collect results
        2. Fetch traces and match to tasks using baggage (task_id, trial_id)
        3. Stamp authoritative input/output from invoker onto trajectories
        4. Evaluate all traces with registered evaluators
        5. Build and return RunResult
        """
        errors: List[str] = []

        # Phase 1: Invoke agent for all tasks
        task_results, invoke_errors, experiment_start, experiment_end = self._invoke_all(dataset)
        errors.extend(invoke_errors)

        # Phase 2: Fetch traces and match to task results
        match_errors = self._fetch_and_match_traces(task_results, experiment_start, experiment_end, dataset)
        errors.extend(match_errors)

        # Phase 3: Collect trajectories for evaluation
        traces: List[Trace] = []
        tasks_by_trace_id: Dict[str, Task] = {}
        trial_info_by_trace: Dict[str, str] = {}

        for tr in task_results:
            result = tr.invoke_result
            if result.has_trajectory:
                traces.append(result.trajectory)
                tasks_by_trace_id[result.trajectory.trace_id] = tr.task
                if tr.trial_id:
                    trial_info_by_trace[result.trajectory.trace_id] = tr.trial_id
            elif result.error:
                pass  # Already captured in errors
            else:
                errors.append(f"Task {tr.task.task_id}: No trajectory available")

        # Phase 4: Evaluate all traces
        run_result = self._evaluate_traces(
            traces=traces,
            tasks=tasks_by_trace_id,
            trial_info=trial_info_by_trace,
        )

        # Add experiment-specific metadata
        run_result.dataset_id = getattr(dataset, "dataset_id", None) or f"dataset-{len(dataset.tasks)}-tasks"
        run_result.metadata["dataset_size"] = len(dataset.tasks)
        run_result.metadata["trials_per_task"] = self.trials_per_task
        run_result.metadata["total_invocations"] = len(dataset.tasks) * self.trials_per_task

        run_result.errors.extend(errors)
        return run_result

    def _invoke_all(self, dataset: Dataset) -> tuple:
        """
        Phase 1: Invoke agent for all tasks, collect results.

        Sets OpenTelemetry baggage (task_id, trial_id) on each invocation
        for trace-to-task matching.

        Args:
            dataset: Dataset with tasks to invoke

        Returns:
            Tuple of (task_results, errors, start_time, end_time)
        """
        from .invokers import InvokeResult
        from datetime import datetime, timezone
        import uuid

        # Ensure requests library is instrumented (idempotent, only once per process)
        _ensure_requests_instrumented()

        # Import OpenTelemetry baggage for propagating task_id and trial_id
        try:
            from opentelemetry import baggage, context
            from opentelemetry.context import attach, detach

            otel_available = True
        except ImportError:
            logger.warning("OpenTelemetry not available - baggage propagation disabled")
            otel_available = False

        task_results: List[Experiment._TaskResult] = []
        errors: List[str] = []

        experiment_start_time = datetime.now(timezone.utc)
        logger.info(f"Experiment started at {experiment_start_time.isoformat()}")

        for task in dataset.tasks:
            for trial in range(self.trials_per_task):
                trial_id = f"trial-{uuid.uuid4()}"

                # Set baggage context for trace matching
                token = None
                if otel_available:
                    ctx = context.get_current()
                    ctx = baggage.set_baggage("task.id", task.task_id, context=ctx)
                    ctx = baggage.set_baggage("trial.id", trial_id, context=ctx)
                    token = attach(ctx)

                try:
                    result = self.invoker.invoke(task.input)
                    if result is None:
                        result = InvokeResult(input=task.input)

                    task_results.append(Experiment._TaskResult(task=task, trial_id=trial_id, invoke_result=result))

                    if result.error:
                        errors.append(f"Task {task.task_id} trial {trial}: {result.error}")

                except Exception as e:
                    errors.append(f"Task {task.task_id} trial {trial}: {e}")
                    task_results.append(
                        Experiment._TaskResult(
                            task=task,
                            trial_id=trial_id,
                            invoke_result=InvokeResult(input=task.input, error=str(e)),
                        )
                    )
                finally:
                    if token is not None:
                        detach(token)

        experiment_end_time = datetime.now(timezone.utc)
        return task_results, errors, experiment_start_time, experiment_end_time

    def _fetch_and_match_traces(
        self,
        task_results: List["Experiment._TaskResult"],
        experiment_start: "datetime",
        experiment_end: "datetime",
        dataset: Dataset,
    ) -> List[str]:
        """
        Phase 2: Fetch traces from trace service and match to task results.

        For each matched trace:
        - Parses the OTEL trace into a Trace
        - Stamps the invoker's authoritative input/output onto the trajectory
          (the invoker knows exactly what was sent/received; trace-parsed values
          may be truncated or missing)

        Args:
            task_results: List of _TaskResult from invocation phase (mutated in place)
            experiment_start: UTC start time of invocations
            experiment_end: UTC end time of invocations
            dataset: Dataset (for sizing the fetch limit)

        Returns:
            List of error messages encountered during fetching/matching
        """
        from .invokers import InvokeResult
        from .trace import parse_trace_for_evaluation
        from datetime import timedelta
        import time

        errors: List[str] = []

        # Buffer the time window to account for processing delays
        fetch_start = experiment_start - timedelta(seconds=5)
        fetch_end = experiment_end + timedelta(seconds=5)

        try:
            # Wait for traces to be exported
            if self.trace_fetch_wait_seconds > 0:
                logger.info(f"Waiting {self.trace_fetch_wait_seconds}s for traces to be exported...")
                time.sleep(self.trace_fetch_wait_seconds)

            expected_count = len(dataset.tasks) * self.trials_per_task
            fetch_limit = max(expected_count * 2, 100)

            fetched_traces = self._fetch_traces(
                start_time=fetch_start.isoformat(),
                end_time=fetch_end.isoformat(),
                limit=fetch_limit,
            )

            logger.info(
                f"Fetched {len(fetched_traces)} traces from trace service "
                f"(expected: {expected_count}, limit: {fetch_limit})"
            )

            # Build lookup: (task_id, trial_id) -> OTEL Trace
            trace_by_baggage: Dict[tuple[str, str], Any] = {}
            for trace in fetched_traces:
                task_id = trace.taskId
                trial_id = trace.trialId

                if task_id and trial_id:
                    trace_by_baggage[(task_id, trial_id)] = trace
                    logger.debug(f"Trace {trace.traceId}: taskId={task_id}, trialId={trial_id}")
                else:
                    logger.warning(f"Trace {trace.traceId} missing taskId={task_id} or trialId={trial_id}")

            logger.info(f"Matched {len(trace_by_baggage)} traces to tasks using baggage parameters")

            # Match traces to task results and stamp input/output
            for tr in task_results:
                baggage_key = (tr.task.task_id, tr.trial_id)

                if baggage_key in trace_by_baggage:
                    otel_trace = trace_by_baggage[baggage_key]
                    trajectory = parse_trace_for_evaluation(otel_trace)

                    # Stamp authoritative input/output from invoker onto trajectory.
                    # Evaluators access trajectory.input / trajectory.output which
                    # delegate to trajectory.input / trajectory.output. The invoker
                    # values are the ground truth of what was actually sent/received.
                    # Only override if invoker has non-None values; otherwise keep trace-parsed values.
                    if tr.invoke_result.input is not None:
                        trajectory.input = str(tr.invoke_result.input)
                    if tr.invoke_result.output is not None:
                        trajectory.output = str(tr.invoke_result.output)

                    # Update the invoke result with the matched trajectory
                    tr.invoke_result = InvokeResult(
                        input=tr.invoke_result.input,
                        output=tr.invoke_result.output,
                        trajectory=trajectory,
                        metadata=tr.invoke_result.metadata,
                        error=tr.invoke_result.error,
                    )
                else:
                    logger.warning(f"No trace found for task_id={tr.task.task_id}, trial_id={tr.trial_id}")
                    errors.append(
                        f"Task {tr.task.task_id} trial {tr.trial_id}: No trace found with matching task_id/trial_id"
                    )

        except ValueError as e:
            logger.warning(f"Cannot fetch traces: {e}")
            errors.append(f"Trace fetching failed: {e}")
        except Exception as e:
            logger.error(f"Error during trace fetching: {e}", exc_info=True)
            errors.append(f"Trace fetching error: {e}")

        return errors


# ============================================================================
# MONITOR RUNNER
# ============================================================================


class Monitor(BaseRunner):
    """
    Evaluation runner for monitor/production trace analysis.

    This runner:
    1. FETCHES traces from a trace service API
    2. Evaluates without ground truth (no expected answers)
    3. Uses quality metrics, error detection, latency analysis

    Use this for continuous monitoring of production traffic.

    Example:
        # Config-driven
        config = Config()  # Loads from env
        runner = Monitor(config=config)
        result = runner.run(limit=1000)

        # Programmatic
        runner = Monitor(
            trace_service_url="http://traces-api:8001",
            exclude_tags=["llm-judge"]
        )
        result = runner.run(
            start_time="2024-01-26T00:00:00",
            end_time="2024-01-27T00:00:00"
        )
        print(f"Error rate: {1 - result.scores['no_errors'].mean}")
    """

    def __init__(
        self,
        config: Optional[Config] = None,
        trace_fetcher: Optional[TraceFetcher] = None,
        trace_service_url: Optional[str] = None,
        include: Optional[List[str]] = None,
        exclude: Optional[List[str]] = None,
        include_tags: Optional[List[str]] = None,
        exclude_tags: Optional[List[str]] = None,
    ):
        """
        Initialize monitor runner.

        Args:
            config: Config object (loads from env if None). Platform API URL from config.platform.api_url
            trace_fetcher: Custom TraceFetcher instance (overrides URL and config)
            trace_service_url: URL of the trace service API (overrides config)
            include: Only run these evaluators (by name)
            exclude: Don't run these evaluators (by name)
            include_tags: Only run evaluators with these tags
            exclude_tags: Don't run evaluators with these tags

        Priority for trace service URL (highest to lowest):
            1. trace_fetcher parameter (if provided)
            2. trace_service_url parameter (if provided)
            3. config.platform.api_url (if config provided and mode=platform)
            4. Default config from env variables (AMP_API_URL)
        """
        super().__init__(
            config=config,
            trace_fetcher=trace_fetcher,
            trace_service_url=trace_service_url,
            include=include,
            exclude=exclude,
            include_tags=include_tags,
            exclude_tags=exclude_tags,
        )

    @property
    def run_type(self) -> RunType:
        return RunType.MONITOR

    def run(
        self,
        start_time: Optional[str] = None,
        end_time: Optional[str] = None,
        limit: Optional[int] = None,
        traces: Optional[List[Trace]] = None,
    ) -> RunResult:
        """
        Run monitor evaluation.

        Provide traces directly OR specify time range to fetch.

        Args:
            start_time: Start time in ISO format (for fetching)
            end_time: End time in ISO format (for fetching)
            limit: Max traces to fetch
            traces: Pre-fetched Trace objects (skip fetching)

        Returns:
            RunResult with aggregated scores
        """
        eval_traces: List[Trace] = []

        if traces:
            # Use provided traces directly
            eval_traces = traces
        else:
            # Fetch from trace service or file
            try:
                fetched = self._fetch_traces(start_time=start_time, end_time=end_time, limit=limit)
                # Parse fetched traces to Trace
                for trace in fetched:
                    try:
                        # _fetch_traces() returns Trace objects, pass directly to parser
                        eval_traces.append(parse_trace_for_evaluation(trace))
                    except Exception as parse_error:
                        logger.error(f"Error parsing trace: {parse_error}")
                        continue

            except Exception as e:
                # Log error and return empty result
                error_msg = f"Failed to fetch traces: {e}"
                logger.error(error_msg, exc_info=True)

                from .dataset import generate_id

                return RunResult(
                    run_id=generate_id("run"),
                    run_type=RunType.MONITOR,
                    started_at=datetime.now(),
                    completed_at=datetime.now(),
                    errors=[error_msg],
                )

        # Evaluate traces (no tasks/ground truth for monitor)
        run_result = self._evaluate_traces(
            traces=eval_traces,
            tasks=None,  # No ground truth in monitor mode
        )

        # Add monitor-specific metadata
        if start_time:
            run_result.metadata["start_time"] = start_time
        if end_time:
            run_result.metadata["end_time"] = end_time

        return run_result
