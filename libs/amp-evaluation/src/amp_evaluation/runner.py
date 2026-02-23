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

Both runners require evaluators to be passed directly:
    evaluators = [latency, hallucination, builtin("answer_relevancy")]
    monitor = Monitor(evaluators=evaluators)
    result = monitor.run(limit=1000)
"""

from typing import List, Dict, Optional, Any, TYPE_CHECKING
from dataclasses import dataclass, field
from datetime import datetime
from abc import ABC, abstractmethod
import logging

from .trace import Trace, parse_trace_for_evaluation, TraceFetcher, TraceLoader
from .trace.fetcher import OTELTrace
from .evaluators.base import BaseEvaluator
from .evaluators.params import EvalMode
from .models import EvaluatorSummary, EvaluatorScore
from .dataset.models import Task, Dataset
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
# RUN RESULT
# ============================================================================


@dataclass
class RunResult:
    """Result of an evaluation run."""

    run_id: str
    eval_mode: EvalMode  # EXPERIMENT or MONITOR
    started_at: datetime
    completed_at: Optional[datetime] = None

    # Context information
    agent_uid: Optional[str] = None
    environment_uid: Optional[str] = None
    dataset_id: Optional[str] = None  # For experiments

    # Counts
    traces_evaluated: int = 0
    evaluators_run: int = 0

    # Per-evaluator aggregated results
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
        """A run is successful only if traces were evaluated and no errors occurred."""
        return len(self.errors) == 0 and self.traces_evaluated > 0

    def summary(self) -> str:
        """Get a human-readable summary."""
        lines = [
            f"Evaluation Run: {self.run_id} ({self.eval_mode})",
            f"  Started: {self.started_at.isoformat()}",
            f"  Duration: {self.duration_seconds:.2f}s",
        ]

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
            for agg_name, value in summary.aggregated_scores.items():
                if isinstance(value, (int, float)):
                    lines.append(f"    {agg_name}: {value:.4f}")
                else:
                    lines.append(f"    {agg_name}: {value}")
            lines.append(f"    count: {summary.count}")

        if self.errors:
            lines.append("")
            lines.append(f"Errors ({len(self.errors)}):")
            for error in self.errors[:5]:
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

    Evaluators are passed directly — no global registry, no filters.
    """

    def __init__(
        self,
        evaluators: List[BaseEvaluator],
        config: Optional[Config] = None,
        trace_fetcher: Optional[TraceFetcher] = None,
        trace_service_url: Optional[str] = None,
    ):
        """
        Initialize runner with evaluators.

        Args:
            evaluators: List of evaluator instances to run (REQUIRED)
            config: Config object (loads from env if None)
            trace_fetcher: Optional TraceFetcher instance
            trace_service_url: Optional trace service URL (overrides config)
        """
        if not evaluators:
            raise ValueError("At least one evaluator is required. Pass evaluators=[...] to the runner.")

        self._evaluators: List[BaseEvaluator] = []

        # Filter evaluators by mode compatibility
        for ev in evaluators:
            if self.eval_mode in ev._supported_eval_modes:
                self._evaluators.append(ev)
            else:
                supported = [m.value for m in ev._supported_eval_modes]
                logger.warning(
                    f"Skipping evaluator '{ev.name}': does not support {self.eval_mode.value} mode. "
                    f"Supported modes: {supported}"
                )

        if not self._evaluators:
            raise ValueError(
                f"No evaluators support {self.eval_mode.value} mode. "
                f"All {len(evaluators)} provided evaluator(s) were filtered out."
            )

        # Store or create config (from environment)
        self.config = config if config is not None else Config()

        # Trace fetcher (lazy initialization)
        self._trace_fetcher = trace_fetcher
        self._trace_service_url = trace_service_url
        self._fetcher_instance: Optional[Any] = None

    def _get_fetcher(self) -> Any:
        """
        Get or create the trace fetcher instance (lazy initialization).

        Priority:
            1. Explicit trace_fetcher passed to __init__
            2. Explicit trace_service_url passed to __init__
            3. Create from config (file mode or platform mode)
        """
        if self._fetcher_instance is not None:
            return self._fetcher_instance

        if self._trace_fetcher:
            self._fetcher_instance = self._trace_fetcher
        elif self._trace_service_url:
            self._fetcher_instance = TraceFetcher(
                base_url=self._trace_service_url,
                agent_uid=self.config.agent.agent_uid,
                environment_uid=self.config.agent.environment_uid,
            )
        else:
            trace_config = self.config.trace_loader

            if trace_config.mode == "file" and trace_config.trace_file_path:
                logger.info(f"Using TraceLoader with file: {trace_config.trace_file_path}")
                self._fetcher_instance = TraceLoader(
                    file_path=trace_config.trace_file_path,
                    agent_uid=self.config.agent.agent_uid,
                    environment_uid=self.config.agent.environment_uid,
                )
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

    def _fetch_traces(self, start_time: str, end_time: str, limit: int = 100, offset: int = 0) -> List[OTELTrace]:
        """Unified interface to fetch traces."""
        fetcher = self._get_fetcher()

        if isinstance(fetcher, TraceLoader):
            return fetcher.load_batch(limit=limit, start_time=start_time, end_time=end_time)
        else:
            return fetcher.fetch_traces(start_time=start_time, end_time=end_time, limit=limit, offset=offset)

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

        Returns:
            Dict mapping evaluator name to list of EvaluatorScore objects
        """
        scores = {}

        for evaluator in self._evaluators:
            try:
                eval_results = evaluator(trace, task)

                evaluator_scores = []
                for eval_result in eval_results:
                    details = eval_result.details or {}
                    span_id = details.get("span_id")

                    if eval_result.is_error:
                        score = EvaluatorScore(
                            trace_id=trace.trace_id,
                            score=0.0,
                            passed=False,
                            span_id=span_id,
                            timestamp=trace.timestamp,
                            explanation=eval_result.explanation,
                            task_id=task.task_id if task else None,
                            trial_id=trial_id,
                            metadata=details,
                            error=eval_result.error,
                        )
                    else:
                        score = EvaluatorScore(
                            trace_id=trace.trace_id,
                            score=eval_result.score,
                            passed=eval_result.passed,
                            span_id=span_id,
                            timestamp=trace.timestamp,
                            explanation=eval_result.explanation,
                            task_id=task.task_id if task else None,
                            trial_id=trial_id,
                            metadata=details,
                            error=None,
                        )
                    evaluator_scores.append(score)

                scores[evaluator.name] = evaluator_scores

            except Exception as e:
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
        """Internal method to evaluate a list of traces."""
        from .dataset import generate_id

        run_id = generate_id("run")
        started_at = datetime.now()

        result = RunResult(
            run_id=run_id,
            eval_mode=self.eval_mode,
            started_at=started_at,
            evaluators_run=len(self._evaluators),
            agent_uid=self.config.agent.agent_uid,
            environment_uid=self.config.agent.environment_uid,
        )

        scores_by_evaluator: Dict[str, List[EvaluatorScore]] = {e.name: [] for e in self._evaluators}

        for trace in traces:
            task = tasks.get(trace.trace_id) if tasks else None
            trial_id = trial_info.get(trace.trace_id) if trial_info else None

            try:
                trace_scores = self.evaluate_trace(trace, task, trial_id=trial_id)
                result.traces_evaluated += 1

                for evaluator_name, score_list in trace_scores.items():
                    scores_by_evaluator[evaluator_name].extend(score_list)

            except Exception as e:
                error_msg = f"Error evaluating trace {trace.trace_id}: {e}"
                result.errors.append(error_msg)
                logger.error(error_msg)

        result.scores = self._get_aggregated_scores(scores_by_evaluator)
        result.completed_at = datetime.now()

        return result

    def _get_aggregated_scores(
        self, scores_by_evaluator: Dict[str, List[EvaluatorScore]]
    ) -> Dict[str, EvaluatorSummary]:
        """Compute aggregated scores for all evaluators."""
        evaluator_by_name = {e.name: e for e in self._evaluators}

        summaries = {}

        for evaluator_name, all_scores in scores_by_evaluator.items():
            evaluator = evaluator_by_name.get(evaluator_name)
            aggregations = getattr(evaluator, "aggregations", None) if evaluator else None

            successful_scores = [s for s in all_scores if not s.is_error]
            # Includes both explicit skips and evaluator errors — any evaluation
            # where we couldn't compute a valid score.
            skipped_count = len(all_scores) - len(successful_scores)

            if skipped_count > 0:
                logger.warning(
                    f"Evaluator '{evaluator_name}' skipped {skipped_count} out of {len(all_scores)} evaluations"
                )

            agg_list = normalize_aggregations(aggregations)
            score_values = [s.score for s in successful_scores]

            aggregated_scores = {}
            for agg in agg_list:
                try:
                    value = agg.compute(score_values)
                    aggregated_scores[agg.name] = value
                except Exception as e:
                    logger.warning(f"Failed to compute {agg.name} for {evaluator_name}: {e}")

            items_per_trace: Dict[str, int] = {}
            for score in all_scores:
                trace_id = score.trace_id
                items_per_trace[trace_id] = items_per_trace.get(trace_id, 0) + 1

            level = evaluator.level.value if evaluator else "trace"

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
    def eval_mode(self) -> EvalMode:
        """Return the eval mode: EvalMode.EXPERIMENT or EvalMode.MONITOR."""
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

    Example:
        experiment = Experiment(
            evaluators=[exact_match, latency],
            invoker=MyInvoker(),
            dataset=dataset,
        )
        result = experiment.run()
    """

    def __init__(
        self,
        evaluators: List[BaseEvaluator],
        invoker: "AgentInvoker",
        dataset: Optional[Dataset] = None,
        trials_per_task: int = 1,
        trace_fetch_wait_seconds: float = 60.0,
        config: Optional[Config] = None,
        trace_fetcher: Optional[TraceFetcher] = None,
        trace_service_url: Optional[str] = None,
    ):
        super().__init__(
            evaluators=evaluators,
            config=config,
            trace_fetcher=trace_fetcher,
            trace_service_url=trace_service_url,
        )

        self.invoker = invoker
        self.dataset = dataset
        self.trials_per_task = trials_per_task
        self.trace_fetch_wait_seconds = trace_fetch_wait_seconds

    @dataclass
    class _TaskResult:
        """Internal structure for a single task trial during an experiment run."""

        task: Task
        trial_id: str
        invoke_result: "InvokeResult"

    @property
    def eval_mode(self) -> EvalMode:
        return EvalMode.EXPERIMENT

    def run(
        self,
        dataset: Optional[Dataset] = None,
        traces: Optional[List[Trace]] = None,
        **kwargs: Any,
    ) -> RunResult:
        """
        Run benchmark evaluation.

        Args:
            dataset: Optional dataset (overrides constructor dataset)
            traces: Pre-fetched Trace objects (skip agent invocation and trace fetching)

        Returns:
            RunResult with aggregated scores
        """
        if traces:
            tasks_by_trace_id = None
            ds = dataset or self.dataset
            if ds is not None:
                # Build task lookup keyed by task_id.
                # When using pre-fetched traces, each trace.trace_id must match the
                # corresponding task.task_id so _evaluate_traces can pair them.
                tasks_by_trace_id = {task.task_id: task for task in ds.tasks}

            return self._evaluate_traces(
                traces=traces,
                tasks=tasks_by_trace_id,
            )

        dataset = dataset or self.dataset
        if not dataset:
            raise ValueError("No dataset provided. Pass dataset to constructor or run().")

        return self._run_with_invoker(dataset)

    def _run_with_invoker(self, dataset: Dataset) -> RunResult:
        """Run experiment using AgentInvoker pattern."""
        errors: List[str] = []

        task_results, invoke_errors, experiment_start, experiment_end = self._invoke_all(dataset)
        errors.extend(invoke_errors)

        match_errors = self._fetch_and_match_traces(task_results, experiment_start, experiment_end, dataset)
        errors.extend(match_errors)

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
                pass
            else:
                errors.append(f"Task {tr.task.task_id}: No trajectory available")

        run_result = self._evaluate_traces(
            traces=traces,
            tasks=tasks_by_trace_id,
            trial_info=trial_info_by_trace,
        )

        run_result.dataset_id = getattr(dataset, "dataset_id", None) or f"dataset-{len(dataset.tasks)}-tasks"
        run_result.metadata["dataset_size"] = len(dataset.tasks)
        run_result.metadata["trials_per_task"] = self.trials_per_task
        run_result.metadata["total_invocations"] = len(dataset.tasks) * self.trials_per_task

        run_result.errors.extend(errors)
        return run_result

    def _invoke_all(self, dataset: Dataset) -> tuple:
        """Phase 1: Invoke agent for all tasks."""
        from .invokers import InvokeResult
        from datetime import datetime, timezone
        import uuid

        _ensure_requests_instrumented()

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
        """Phase 2: Fetch traces and match to task results."""
        from .invokers import InvokeResult
        from .trace import parse_trace_for_evaluation
        from datetime import timedelta
        import time

        errors: List[str] = []

        fetch_start = experiment_start - timedelta(seconds=5)
        fetch_end = experiment_end + timedelta(seconds=5)

        try:
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

            trace_by_baggage: Dict[tuple, Any] = {}
            for trace in fetched_traces:
                task_id = trace.taskId
                trial_id = trace.trialId

                if task_id and trial_id:
                    trace_by_baggage[(task_id, trial_id)] = trace
                else:
                    logger.warning(f"Trace {trace.traceId} missing taskId={task_id} or trialId={trial_id}")

            logger.info(f"Matched {len(trace_by_baggage)} traces to tasks using baggage parameters")

            for tr in task_results:
                baggage_key = (tr.task.task_id, tr.trial_id)

                if baggage_key in trace_by_baggage:
                    otel_trace = trace_by_baggage[baggage_key]
                    trajectory = parse_trace_for_evaluation(otel_trace)

                    if tr.invoke_result.input is not None:
                        trajectory.input = str(tr.invoke_result.input)
                    if tr.invoke_result.output is not None:
                        trajectory.output = str(tr.invoke_result.output)

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

    Example:
        monitor = Monitor(
            evaluators=[latency, hallucination],
            trace_service_url="http://traces-api:8001",
        )
        result = monitor.run(limit=1000)
    """

    def __init__(
        self,
        evaluators: List[BaseEvaluator],
        config: Optional[Config] = None,
        trace_fetcher: Optional[TraceFetcher] = None,
        trace_service_url: Optional[str] = None,
    ):
        super().__init__(
            evaluators=evaluators,
            config=config,
            trace_fetcher=trace_fetcher,
            trace_service_url=trace_service_url,
        )

    @property
    def eval_mode(self) -> EvalMode:
        return EvalMode.MONITOR

    def run(
        self,
        start_time: Optional[str] = None,
        end_time: Optional[str] = None,
        limit: Optional[int] = None,
        traces: Optional[List[Trace]] = None,
        **kwargs: Any,
    ) -> RunResult:
        """
        Run monitor evaluation.

        Provide traces directly OR specify time range to fetch.
        """
        eval_traces: List[Trace] = []

        if traces:
            eval_traces = traces
        else:
            try:
                fetched = self._fetch_traces(
                    start_time=start_time or "",
                    end_time=end_time or "",
                    limit=limit or 100,
                )
                for trace in fetched:
                    try:
                        eval_traces.append(parse_trace_for_evaluation(trace))
                    except Exception as parse_error:
                        logger.error(f"Error parsing trace: {parse_error}")
                        continue

            except Exception as e:
                error_msg = f"Failed to fetch traces: {e}"
                logger.error(error_msg, exc_info=True)

                from .dataset import generate_id

                return RunResult(
                    run_id=generate_id("run"),
                    eval_mode=EvalMode.MONITOR,
                    started_at=datetime.now(),
                    completed_at=datetime.now(),
                    errors=[error_msg],
                )

        run_result = self._evaluate_traces(
            traces=eval_traces,
            tasks=None,
        )

        if start_time:
            run_result.metadata["start_time"] = start_time
        if end_time:
            run_result.metadata["end_time"] = end_time

        return run_result
