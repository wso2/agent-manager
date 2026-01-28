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

1. BenchmarkRunner - Evaluation with a dataset of predefined tasks
   - You have a set of test cases (questions/prompts with expected answers)
   - Runner INVOKES the agent with each task
   - Gets traces and evaluates against ground truth
   - Use case: "Test my agent against these 100 questions"

2. LiveRunner - Evaluation on live production traces
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
    runner = LiveRunner(config=config)
    result = runner.run(limit=1000)

    # Programmatic (explicit parameters)
    runner = LiveRunner(
        trace_service_url="http://traces-api:8001",
        exclude_tags=["llm-judge"]
    )
    result = runner.run(start_time="2024-01-26T00:00:00")

    # Hybrid (config + overrides)
    runner = LiveRunner(
        config=config,
        trace_service_url="http://custom-url:9000",  # Overrides config
        include_tags=["quality"]
    )

    # Benchmark evaluation
    runner = BenchmarkRunner(
        agent=my_agent_function,
        dataset=Dataset.from_csv("test_cases.csv")
    )
    result = runner.run()
"""

from typing import List, Dict, Optional, Any, Callable, Union
from dataclasses import dataclass, field
from datetime import datetime
from abc import ABC, abstractmethod
from enum import Enum
import logging

from .trace import EvalTrace, parse_trace_for_evaluation, TraceFetcher
from .registry import get_registry, get_evaluator
from .evaluators.base import BaseEvaluator
from .models import EvalResult, Task, Dataset, EvalContext, Constraints
from .aggregators.aggregation import ResultAggregator, AggregatedResults
from .aggregators.base import AggregationType
from .config import Config


logger = logging.getLogger(__name__)


# ============================================================================
# RUN TYPE
# ============================================================================


class RunType(str, Enum):
    """Type of evaluation run."""

    BENCHMARK = "benchmark"
    LIVE = "live"


# ============================================================================
# RUN RESULT
# ============================================================================


@dataclass
class RunResult:
    """Result of an evaluation run."""

    run_id: str
    run_type: RunType  # BENCHMARK or LIVE
    started_at: datetime
    completed_at: Optional[datetime] = None

    # Counts
    traces_evaluated: int = 0
    evaluators_run: int = 0

    # Per-evaluator aggregated results
    scores: Dict[str, AggregatedResults] = field(default_factory=dict)

    # Individual results per trace (optional, for detailed analysis)
    trace_results: List[Dict[str, EvalResult]] = field(default_factory=list)

    # Errors encountered
    errors: List[str] = field(default_factory=list)

    # Metadata
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
            f"  Traces evaluated: {self.traces_evaluated}",
            f"  Evaluators run: {self.evaluators_run}",
            f"  Duration: {self.duration_seconds:.2f}s",
            f"  Errors: {len(self.errors)}",
            "",
            "Scores:",
        ]

        for name, agg in self.scores.items():
            lines.append(f"  {name}:")
            # Show all available aggregations
            for agg_name, value in agg.aggregations.items():
                lines.append(f"    {agg_name}: {value:.4f}")
            # Show count
            lines.append(f"    count: {agg.count}")

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
        include: Optional[List[str]] = None,
        exclude: Optional[List[str]] = None,
        include_tags: Optional[List[str]] = None,
        exclude_tags: Optional[List[str]] = None,
    ):
        """
        Initialize runner with evaluator filtering options.

        Args:
            config: Config object (loads from env if None). Individual params override this.
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
        self.config = config if config is not None else Config.from_env()

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

        # Results storage
        self._results_by_evaluator: Dict[str, List[EvalResult]] = {}
        self.reset()

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

    def reset(self):
        """Reset accumulated results."""
        self._results_by_evaluator = {e.name: [] for e in self._evaluators}

    def evaluate_trace(self, trace: EvalTrace, task: Optional[Task] = None) -> Dict[str, EvalResult]:
        """
        Run all evaluators on a single trace.

        Args:
            trace: The EvalTrace to evaluate
            task: Optional task for reference/ground truth data

        Returns:
            Dict mapping evaluator name to EvalResult
        """
        # Build constraints from task if available
        constraints = None
        if task and (task.max_latency_ms or task.max_tokens or task.max_iterations):
            constraints = Constraints(
                max_latency_ms=task.max_latency_ms, max_tokens=task.max_tokens, max_iterations=task.max_iterations
            )

        # Build evaluation context with all available data from task
        context = EvalContext(
            trace=trace,
            is_benchmark=self.run_type == RunType.BENCHMARK,
            # Expected data (ground truth)
            _expected_output=task.expected_output if task else None,
            _expected_trajectory=task.expected_trajectory if task else None,
            _expected_outcome=task.expected_outcome if task else None,
            # Guidelines
            _success_criteria=task.success_criteria_text if task else None,
            _prohibited_content=task.prohibited_content if task else None,
            # Constraints
            _constraints=constraints,
            # Custom attributes
            _custom=task.custom if task else {},
            # Task reference
            _task=task,
        )

        results = {}

        for evaluator in self._evaluators:
            try:
                result = evaluator.evaluate(context)
                results[evaluator.name] = result
                self._results_by_evaluator[evaluator.name].append(result)
            except Exception as e:
                error_result = EvalResult(
                    evaluator_name=evaluator.name,
                    target_id=trace.trace_id,
                    target_type="trace",
                    score=0.0,
                    passed=False,
                    explanation=f"Evaluator error: {str(e)}",
                )
                results[evaluator.name] = error_result
                self._results_by_evaluator[evaluator.name].append(error_result)

        return results

    def _evaluate_traces(
        self, traces: List[EvalTrace], tasks: Optional[Dict[str, Task]] = None, store_individual_results: bool = False
    ) -> RunResult:
        """
        Internal method to evaluate a list of traces.

        Args:
            traces: List of EvalTrace objects
            tasks: Optional dict mapping trace_id to Task
            store_individual_results: Whether to store per-trace results

        Returns:
            RunResult with aggregated scores
        """
        from .models import generate_id

        run_id = generate_id("run")
        started_at = datetime.now()

        result = RunResult(
            run_id=run_id, run_type=self.run_type, started_at=started_at, evaluators_run=len(self._evaluators)
        )

        # Reset any previous results
        self.reset()

        # Evaluate each trace
        for trace in traces:
            task = tasks.get(trace.trace_id) if tasks else None

            try:
                trace_results = self.evaluate_trace(trace, task)
                result.traces_evaluated += 1

                if store_individual_results:
                    result.trace_results.append(trace_results)

            except Exception as e:
                error_msg = f"Error evaluating trace {trace.trace_id}: {e}"
                result.errors.append(error_msg)
                logger.error(error_msg)

        # Aggregate results
        result.scores = self._get_aggregated_scores()
        result.completed_at = datetime.now()

        return result

    def _get_aggregated_scores(self) -> Dict[str, AggregatedResults]:
        """Get aggregated scores from all evaluators."""
        aggregated = {}

        for evaluator in self._evaluators:
            results = self._results_by_evaluator.get(evaluator.name, [])

            # Use evaluator's aggregations or default to MEAN
            aggregations = getattr(evaluator, "aggregations", None)
            if aggregations is None:
                aggregations = [AggregationType.MEAN]

            agg = ResultAggregator.aggregate(results=results, aggregations=aggregations, evaluator_name=evaluator.name)
            aggregated[evaluator.name] = agg

        return aggregated

    @property
    @abstractmethod
    def run_type(self) -> RunType:
        """Return the type of run: RunType.BENCHMARK or RunType.LIVE."""
        pass

    @abstractmethod
    def run(self, **kwargs) -> RunResult:
        """Run the evaluation. Subclasses implement this."""
        pass


# ============================================================================
# BENCHMARK RUNNER
# ============================================================================

# Type for agent callable: takes a task/prompt, returns trace or trace_id
AgentCallable = Callable[[Task], Union[EvalTrace, str, Dict[str, Any]]]


class BenchmarkRunner(BaseRunner):
    """
    Evaluation runner for benchmark/dataset-based testing.

    This runner:
    1. Takes a dataset of tasks (prompts with expected answers)
    2. INVOKES the agent with each task
    3. Collects traces from the agent execution
    4. Evaluates traces against ground truth

    Use this when you have a predefined set of test cases.

    Example:
        def my_agent(task: Task) -> EvalTrace:
            response = agent.invoke(task.query)
            return get_trace()  # Get trace from instrumentation

        runner = BenchmarkRunner(
            agent=my_agent,
            dataset=Dataset.from_csv("test_cases.csv")
        )
        result = runner.run()
        print(f"Accuracy: {result.scores['accuracy'].mean}")
    """

    def __init__(
        self,
        agent: AgentCallable,
        dataset: Optional[Dataset] = None,
        trials_per_task: int = 1,
        config: Optional[Config] = None,
        include: Optional[List[str]] = None,
        exclude: Optional[List[str]] = None,
        include_tags: Optional[List[str]] = None,
        exclude_tags: Optional[List[str]] = None,
    ):
        """
        Initialize benchmark runner.

        Args:
            agent: Callable that takes a Task and returns trace/trace_id
            dataset: Dataset with test tasks
            trials_per_task: Number of times to run each task (default: 1)
            config: Config object (loads from env if None)
            include: Only run these evaluators (by name)
            exclude: Don't run these evaluators (by name)
            include_tags: Only run evaluators with these tags
            exclude_tags: Don't run evaluators with these tags
        """
        super().__init__(config, include, exclude, include_tags, exclude_tags)

        self.agent = agent
        self.dataset = dataset
        self.trials_per_task = trials_per_task

    @property
    def run_type(self) -> RunType:
        return RunType.BENCHMARK

    def run(self, dataset: Optional[Dataset] = None, store_individual_results: bool = False) -> RunResult:
        """
        Run benchmark evaluation.

        Args:
            dataset: Optional dataset (overrides constructor dataset)
            store_individual_results: Whether to store per-trace results

        Returns:
            RunResult with aggregated scores
        """
        dataset = dataset or self.dataset
        if not dataset:
            raise ValueError("No dataset provided. Pass dataset to constructor or run().")

        traces: List[EvalTrace] = []
        tasks_by_trace_id: Dict[str, Task] = {}
        errors: List[str] = []

        # Run agent on each task
        for task in dataset.tasks:
            for trial in range(self.trials_per_task):
                try:
                    # Invoke agent
                    result = self.agent(task)

                    # Handle different return types
                    if isinstance(result, EvalTrace):
                        trace = result
                    elif isinstance(result, dict):
                        trace = parse_trace_for_evaluation(result)
                    elif isinstance(result, str):
                        # trace_id returned, would need to fetch
                        errors.append(f"Task {task.task_id}: trace_id return not supported yet")
                        continue
                    else:
                        errors.append(f"Task {task.task_id}: unexpected return type {type(result)}")
                        continue

                    traces.append(trace)
                    tasks_by_trace_id[trace.trace_id] = task

                except Exception as e:
                    errors.append(f"Task {task.task_id} trial {trial}: {e}")

        # Evaluate all traces
        run_result = self._evaluate_traces(
            traces=traces, tasks=tasks_by_trace_id, store_individual_results=store_individual_results
        )

        # Add benchmark-specific metadata
        run_result.metadata["dataset_size"] = len(dataset.tasks)
        run_result.metadata["trials_per_task"] = self.trials_per_task
        run_result.metadata["total_invocations"] = len(dataset.tasks) * self.trials_per_task

        # Add any agent invocation errors
        run_result.errors.extend(errors)

        return run_result


# ============================================================================
# LIVE RUNNER
# ============================================================================


class LiveRunner(BaseRunner):
    """
    Evaluation runner for live/production trace analysis.

    This runner:
    1. FETCHES traces from a trace service API
    2. Evaluates without ground truth (no expected answers)
    3. Uses quality metrics, error detection, latency analysis

    Use this for continuous monitoring of production traffic.

    Example:
        # Config-driven
        config = Config()  # Loads from env
        runner = LiveRunner(config=config)
        result = runner.run(limit=1000)

        # Programmatic
        runner = LiveRunner(
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
        trace_service_url: Optional[str] = None,
        trace_fetcher: Optional[TraceFetcher] = None,
        config: Optional[Config] = None,
        include: Optional[List[str]] = None,
        exclude: Optional[List[str]] = None,
        include_tags: Optional[List[str]] = None,
        exclude_tags: Optional[List[str]] = None,
    ):
        """
        Initialize live runner.

        Args:
            trace_service_url: URL of the trace service API (overrides config)
            trace_fetcher: Custom TraceFetcher instance (overrides URL and config)
            config: Config object (loads from env if None). Platform API URL from config.platform.api_url
            include: Only run these evaluators (by name)
            exclude: Don't run these evaluators (by name)
            include_tags: Only run evaluators with these tags
            exclude_tags: Don't run evaluators with these tags

        Priority for trace service URL (highest to lowest):
            1. trace_fetcher parameter (if provided)
            2. trace_service_url parameter (if provided)
            3. config.platform.api_url (if config provided and mode=platform)
            4. Default config from env variables (AMP_API_URL)
            5. Fallback to localhost:8001
        """
        super().__init__(config, include, exclude, include_tags, exclude_tags)

        # Store parameters for lazy fetcher creation
        self._trace_service_url = trace_service_url
        self._trace_fetcher = trace_fetcher
        self._fetcher_instance: Optional[TraceFetcher] = None

    def _get_fetcher(self) -> TraceFetcher:
        """Get or create the trace fetcher instance (lazy initialization)."""
        if self._fetcher_instance is not None:
            return self._fetcher_instance

        # Determine trace fetcher with priority
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
        elif self.config.trace_loader.mode == "platform" and self.config.platform.api_url:
            # Use platform API URL when in platform mode
            self._fetcher_instance = TraceFetcher(
                base_url=self.config.platform.api_url,
                agent_uid=self.config.agent.agent_uid,
                environment_uid=self.config.agent.environment_uid,
            )
        else:
            # Fallback to localhost with config agent info
            self._fetcher_instance = TraceFetcher(
                base_url="http://localhost:8001",
                agent_uid=self.config.agent.agent_uid,
                environment_uid=self.config.agent.environment_uid,
            )

        return self._fetcher_instance

    @property
    def run_type(self) -> RunType:
        return RunType.LIVE

    def run(
        self,
        start_time: Optional[str] = None,
        end_time: Optional[str] = None,
        limit: Optional[int] = None,
        agent_id: Optional[str] = None,
        traces: Optional[List[EvalTrace]] = None,
        raw_traces: Optional[List[Dict[str, Any]]] = None,
        store_individual_results: bool = False,
    ) -> RunResult:
        """
        Run live evaluation.

        Provide traces directly OR specify time range to fetch.

        Args:
            start_time: Start time in ISO format (for fetching)
            end_time: End time in ISO format (for fetching)
            limit: Max traces to fetch
            agent_id: Filter by agent ID
            traces: Pre-fetched EvalTrace objects (skip fetching)
            raw_traces: Pre-fetched raw trace dicts (will be parsed)
            store_individual_results: Whether to store per-trace results

        Returns:
            RunResult with aggregated scores
        """
        eval_traces: List[EvalTrace] = []

        if traces:
            # Use provided traces directly
            eval_traces = traces
        elif raw_traces:
            # Parse provided raw traces
            eval_traces = [parse_trace_for_evaluation(t) for t in raw_traces]
        else:
            # Fetch from trace service
            try:
                fetcher = self._get_fetcher()
                fetched = fetcher.fetch_traces(start_time=start_time, end_time=end_time, limit=limit, agent_id=agent_id)
                # Parse fetched traces to EvalTrace
                for trace in fetched:
                    # Handle both dict and object formats
                    if isinstance(trace, dict):
                        trace_dict = trace
                    elif hasattr(trace, "__dict__"):
                        trace_dict = trace.__dict__
                    else:
                        logger.warning(f"Unknown trace type {type(trace)}, skipping")
                        continue

                    try:
                        eval_traces.append(parse_trace_for_evaluation(trace_dict))
                    except Exception as parse_error:
                        logger.error(f"Error parsing trace: {parse_error}")
                        continue

            except Exception as e:
                # Log error and return empty result
                error_msg = f"Failed to fetch traces: {e}"
                logger.error(error_msg, exc_info=True)

                from .models import generate_id

                return RunResult(
                    run_id=generate_id("run"),
                    run_type=RunType.LIVE,
                    started_at=datetime.now(),
                    completed_at=datetime.now(),
                    errors=[error_msg],
                )

        # Evaluate traces (no tasks/ground truth for live)
        run_result = self._evaluate_traces(
            traces=eval_traces,
            tasks=None,  # No ground truth in live mode
            store_individual_results=store_individual_results,
        )

        # Add live-specific metadata
        if start_time:
            run_result.metadata["start_time"] = start_time
        if end_time:
            run_result.metadata["end_time"] = end_time
        if agent_id:
            run_result.metadata["agent_id"] = agent_id

        return run_result


# ============================================================================
# CONVENIENCE FUNCTION
# ============================================================================


def evaluate(
    traces: List[EvalTrace],
    tasks: Optional[Dict[str, Task]] = None,
    include: Optional[List[str]] = None,
    exclude: Optional[List[str]] = None,
    include_tags: Optional[List[str]] = None,
    exclude_tags: Optional[List[str]] = None,
) -> RunResult:
    """
    Convenience function to evaluate traces without creating a runner.

    Args:
        traces: List of EvalTrace objects
        tasks: Optional dict mapping trace_id to Task (for ground truth)
        include/exclude: Evaluator filtering by name
        include_tags/exclude_tags: Evaluator filtering by tag

    Returns:
        RunResult with aggregated scores

    Example:
        result = evaluate(traces, include_tags=["quality"])
        print(result.scores["accuracy"].mean)
    """
    runner = LiveRunner(
        include=include,
        exclude=exclude,
        include_tags=include_tags,
        exclude_tags=exclude_tags,
    )
    return runner.run(traces=traces)
