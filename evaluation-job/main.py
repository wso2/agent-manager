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
Monitor job for running evaluations in Argo Workflows.

This script is invoked by the ClusterWorkflowTemplate to run monitor evaluations
against agent traces within a specified time window.

Uses the amp-evaluation SDK to register evaluators and run the monitor.

Usage:
    python main.py \
        --monitor-name=my-monitor \
        --agent-id=agent-uid-123 \
        --environment-id=env-uid-456 \
        --evaluators='[{"name":"latency_performance","config":{"max_latency_ms":3000}}]' \
        --sampling-rate=1.0 \
        --trace-start=2026-01-01T00:00:00Z \
        --trace-end=2026-01-02T00:00:00Z \
        --traces-api-endpoint=http://traces-observer:8080
"""

import argparse
import json
import logging
import os
import signal
import sys
import time
from datetime import datetime
from typing import Dict, List, Any

import requests
from amp_evaluation import Monitor, builtin
from amp_evaluation.models import EvaluatorSummary
from amp_evaluation.trace import TraceFetcher

logger = logging.getLogger(__name__)

# Maximum number of retries for publishing scores
PUBLISH_MAX_RETRIES = 3
PUBLISH_INITIAL_BACKOFF = 2  # seconds


class OAuth2TokenManager:
    """Manages OAuth2 client_credentials tokens with caching."""

    def __init__(self, token_url: str, client_id: str, client_secret: str):
        self.token_url = token_url
        self.client_id = client_id
        self.client_secret = client_secret
        self._token: str | None = None
        self._expires_at: float = 0

    def get_token(self) -> str:
        """Get a valid access token, refreshing if needed."""
        if self._token and time.time() < self._expires_at - 30:
            return self._token

        response = requests.post(
            self.token_url,
            data={
                "grant_type": "client_credentials",
                "client_id": self.client_id,
                "client_secret": self.client_secret,
            },
            timeout=10,
        )
        response.raise_for_status()
        data = response.json()
        self._token = data["access_token"]
        self._expires_at = time.time() + data.get("expires_in", 3600)
        return self._token


def handle_sigterm(signum: int, frame: Any) -> None:
    """Handle SIGTERM for graceful shutdown in Kubernetes."""
    logger.info("Received SIGTERM, shutting down gracefully")
    sys.exit(0)


signal.signal(signal.SIGTERM, handle_sigterm)


class JsonFormatter(logging.Formatter):
    """Format log records as single-line JSON matching Go slog output."""

    # Python uses "WARNING"; normalise to the short form used by Go slog.
    _LEVEL_MAP = {"WARNING": "WARN"}

    def format(self, record):
        log_entry = {
            "time": self.formatTime(record, self.datefmt),
            "level": self._LEVEL_MAP.get(record.levelname, record.levelname),
            "msg": record.getMessage(),
            "logger": record.name,
        }
        if record.exc_info and record.exc_info[0] is not None:
            log_entry["trace"] = self.formatException(record.exc_info)
        if record.stack_info:
            log_entry["stack"] = self.formatStack(record.stack_info)
        return json.dumps(log_entry)


def configure_logging() -> None:
    """Configure JSON logging from LOG_LEVEL env var (default: INFO).

    The root logger stays at INFO so library internals (e.g. trace parser
    debug messages) don't leak into run logs.  Only the evaluation-job's
    own logger is set to the requested level.
    """
    level_name = os.environ.get("LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)
    handler = logging.StreamHandler()
    handler.setFormatter(JsonFormatter(datefmt="%Y-%m-%dT%H:%M:%S"))
    logging.basicConfig(level=logging.INFO, handlers=[handler])
    logging.getLogger(__name__).setLevel(level)
    logging.getLogger("LiteLLM").setLevel(logging.WARNING)


def parse_args() -> argparse.Namespace:
    """Parse command-line arguments for monitor execution."""
    parser = argparse.ArgumentParser(
        description="Run monitor evaluation for AI agent traces",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    parser.add_argument(
        "--monitor-name",
        required=True,
        help="Unique name of the monitor",
    )

    parser.add_argument(
        "--agent-id",
        required=True,
        help="Unique identifier (UID) of the agent",
    )

    parser.add_argument(
        "--environment-id",
        required=True,
        help="Unique identifier (UID) of the environment",
    )

    parser.add_argument(
        "--evaluators",
        required=True,
        help='JSON array of evaluator configurations (e.g., \'[{"name":"latency_performance","config":{"max_latency_ms":3000}}]\')',
    )

    parser.add_argument(
        "--sampling-rate",
        type=float,
        default=1.0,
        help="Sampling rate for traces (0.0-1.0), default: 1.0",
    )

    parser.add_argument(
        "--trace-start",
        required=True,
        help="Start time for trace evaluation (ISO 8601 format)",
    )

    parser.add_argument(
        "--trace-end",
        required=True,
        help="End time for trace evaluation (ISO 8601 format)",
    )

    parser.add_argument(
        "--traces-api-endpoint",
        required=True,
        help="Traces API endpoint (e.g., http://traces-observer-service:8080)",
    )

    parser.add_argument(
        "--monitor-id",
        required=True,
        help="Monitor UUID for publishing scores",
    )

    parser.add_argument(
        "--run-id",
        required=True,
        help="Run UUID for this evaluation execution",
    )

    parser.add_argument(
        "--publisher-endpoint",
        required=True,
        help="Publisher API endpoint for score publishing (e.g., http://agent-manager-internal:8081)",
    )

    return parser.parse_args()


def validate_time_format(time_str: str) -> bool:
    """Validate ISO 8601 time format."""
    try:
        datetime.fromisoformat(time_str.replace("Z", "+00:00"))
        return True
    except ValueError:
        return False


def publish_scores(
    monitor_id: str,
    run_id: str,
    scores: Dict[str, EvaluatorSummary],
    display_name_to_identifier: Dict[str, str],
    api_endpoint: str,
    token_manager: OAuth2TokenManager,
) -> bool:
    """
    Publish evaluation scores to the agent-manager internal API.

    Args:
        monitor_id: Monitor UUID
        run_id: Run UUID
        scores: Dict of evaluator display_name -> EvaluatorSummary from RunResult
        display_name_to_identifier: Mapping of display_name -> evaluator identifier
        api_endpoint: Agent Manager internal API base URL
        token_manager: OAuth2 token manager for authentication

    Returns:
        True if publishing succeeded, False otherwise
    """
    if not scores:
        logger.warning("No scores to publish")
        return True

    # Build the publish request payload
    individual_scores: List[Dict[str, Any]] = []
    aggregated_scores: List[Dict[str, Any]] = []

    for display_name, summary in scores.items():
        identifier = display_name_to_identifier.get(display_name, display_name)
        # Add aggregated scores (evaluator metadata + aggregations)
        aggregated_scores.append(
            {
                "identifier": identifier,
                "evaluatorName": display_name,
                "level": summary.level,
                "aggregations": summary.aggregated_scores,
                "count": summary.count,
                "skippedCount": summary.skipped_count,
            }
        )

        # Add individual scores (per-trace/span scores)
        for score in summary.individual_scores:
            item: Dict[str, Any] = {
                "evaluatorName": display_name,
                "level": summary.level,
                "traceId": score.trace_id,
            }

            # Optional fields
            if score.is_successful and score.score is not None:
                item["score"] = score.score
            elif score.skip_reason:
                item["skipReason"] = score.skip_reason
            else:
                item["skipReason"] = "Evaluation did not produce a score"
            if score.explanation:
                item["explanation"] = score.explanation
            if score.trace_start_time:
                item["traceStartTime"] = score.trace_start_time.isoformat()
            # Span context (enriched by the framework)
            if score.span_context:
                span_ctx: Dict[str, str] = {}
                if score.span_context.span_id:
                    span_ctx["spanId"] = score.span_context.span_id
                if score.span_context.agent_name:
                    span_ctx["agentName"] = score.span_context.agent_name
                if score.span_context.model:
                    span_ctx["model"] = score.span_context.model
                if score.span_context.vendor:
                    span_ctx["vendor"] = score.span_context.vendor
                if span_ctx:
                    item["spanContext"] = span_ctx

            individual_scores.append(item)

    payload = {
        "individualScores": individual_scores,
        "aggregatedScores": aggregated_scores,
    }

    # Publish scores to agent-manager API
    url = f"{api_endpoint}/api/v1/publisher/monitors/{monitor_id}/runs/{run_id}/scores"

    logger.info(
        "Publishing scores monitor_id=%s run_id=%s evaluators=%d individual_scores=%d",
        monitor_id,
        run_id,
        len(scores),
        len(individual_scores),
    )

    for attempt in range(PUBLISH_MAX_RETRIES):
        try:
            headers = {
                "Authorization": f"Bearer {token_manager.get_token()}",
                "Content-Type": "application/json",
            }
            response = requests.post(url, json=payload, headers=headers, timeout=30)
            response.raise_for_status()

            logger.info("Successfully published scores to agent-manager")
            return True

        except requests.exceptions.RequestException as e:
            logger.error(
                "Failed to publish scores (attempt %d/%d): %s",
                attempt + 1,
                PUBLISH_MAX_RETRIES,
                e,
            )
            if hasattr(e, "response") and e.response is not None:
                logger.error("Response status: %d", e.response.status_code)
                logger.error("Response body: %s", e.response.text[:500])
                # Don't retry on 4xx client errors
                if 400 <= e.response.status_code < 500:
                    return False
            if attempt < PUBLISH_MAX_RETRIES - 1:
                backoff = PUBLISH_INITIAL_BACKOFF * (2**attempt)
                logger.info("Retrying in %d seconds...", backoff)
                time.sleep(backoff)

    return False


def _load_custom_code_evaluator(identifier: str, source: str, config: dict):
    """Dynamically load a custom code evaluator from Python source.

    Supports both plain function style (preferred) and class-based style:

    - Plain function (preferred): the platform wraps it with FunctionEvaluator so
      that Param() defaults in the function signature are honoured as config fields.
    - Class-based (backward-compatible): must be a BaseEvaluator subclass.
    """
    from amp_evaluation import BaseEvaluator
    from amp_evaluation.evaluators.base import FunctionEvaluator

    if not source or not source.strip():
        raise ValueError(f"Custom evaluator '{identifier}' has empty source")

    namespace = {"__name__": f"custom_evaluator_{identifier}"}
    exec(source, namespace)  # noqa: S102

    found_func: Any = None
    found_cls: Any = None
    for obj in namespace.values():
        if isinstance(obj, FunctionEvaluator):
            # Already decorated with @evaluator — use as-is
            found_func = obj
            break
        if callable(obj) and not isinstance(obj, type) and not isinstance(obj, BaseEvaluator):
            found_func = obj  # plain function — will be wrapped below
        elif isinstance(obj, type) and issubclass(obj, BaseEvaluator) and obj is not BaseEvaluator:
            found_cls = obj

    if found_func is not None:
        if isinstance(found_func, FunctionEvaluator):
            instance = found_func
        else:
            instance = FunctionEvaluator(found_func, name=identifier)

        logger.info(
            "Loaded custom code evaluator: %s (function=%s)", identifier, getattr(found_func, "__name__", identifier)
        )
        instance = instance.with_config(**config) if config else instance
        return instance

    if found_cls is not None:
        logger.info("Loaded custom code evaluator: %s (class=%s)", identifier, found_cls.__name__)
        return found_cls(**config)

    raise ValueError(
        f"Custom evaluator '{identifier}' source must define a plain function "
        f"(with optional Param() defaults) or a BaseEvaluator subclass"
    )


def _eval_template(template: str, variables: dict) -> str:
    """Render a prompt template by evaluating it as a Python f-string.

    Uses the same trust model as custom code evaluators (which use ``exec()``).
    """
    try:
        # Escape any triple-quotes in the template to avoid breaking the f-string wrapper.
        safe = template.replace('"""', '\\"\\"\\"')
        result = eval(f'f"""{safe}"""', {"__builtins__": __builtins__}, variables)  # noqa: S307
        return result if result is not None else ""
    except Exception as e:
        raise ValueError(f"Failed to evaluate template: {e}")


def _get_llm_base_keys() -> frozenset:
    from amp_evaluation.evaluators.base import LLMAsJudgeEvaluator
    from amp_evaluation.evaluators.params import _ParamDescriptor

    return frozenset(name for name, val in vars(LLMAsJudgeEvaluator).items() if isinstance(val, _ParamDescriptor))


_LLM_BASE_KEYS = _get_llm_base_keys()


def _create_custom_llm_judge(identifier: str, prompt_template: str, level: str, config: dict):
    """Create an LLM-as-judge evaluator from a prompt template.

    The prompt template supports Python expressions inside ``{...}`` placeholders,
    evaluated as f-string expressions. The trace/agent/LLM span object is always
    available, along with any user-defined config params (i.e. params not in the
    base LLM config).

    Example template (agent level)::

        You are an expert {domain} evaluator.
        Agent: {agent_trace.agent_name or 'agent'}
        Input: {agent_trace.input}
        Output: {agent_trace.output}
        Steps: {len(agent_trace.steps)}

    Here ``domain`` would be a user-defined config param.
    """
    if not prompt_template or not prompt_template.strip():
        raise ValueError(f"Custom LLM-judge evaluator '{identifier}' has empty prompt template")

    from amp_evaluation.evaluators.base import FunctionLLMJudge
    from amp_evaluation.trace.models import Trace, AgentTrace, LLMSpan

    if level not in ("agent", "llm", "trace"):
        raise ValueError(f"Unsupported evaluator level: {level}")

    # User-defined params (non-LLM-base) are injected as extra template variables
    template_extra = {k: v for k, v in config.items() if k not in _LLM_BASE_KEYS}
    llm_config = {k: v for k, v in config.items() if k in _LLM_BASE_KEYS}

    if level == "agent":

        def _build_prompt(agent_trace: AgentTrace) -> str:
            return _eval_template(prompt_template, {"agent_trace": agent_trace, **template_extra})
    elif level == "llm":

        def _build_prompt(llm_span: LLMSpan) -> str:  # type: ignore[misc]
            return _eval_template(prompt_template, {"llm_span": llm_span, **template_extra})
    else:

        def _build_prompt(trace: Trace) -> str:  # type: ignore[misc]
            return _eval_template(prompt_template, {"trace": trace, **template_extra})

    logger.info("Created custom LLM-as-judge evaluator: %s (level=%s)", identifier, level)
    return FunctionLLMJudge(_build_prompt, name=identifier, **llm_config)


def main() -> None:
    """Main entry point for monitor job."""
    configure_logging()
    args = parse_args()

    # Read OAuth2 client credentials for publisher authentication
    idp_token_url = os.environ.get("IDP_TOKEN_URL")
    idp_client_id = os.environ.get("IDP_CLIENT_ID")
    idp_client_secret = os.environ.get("IDP_CLIENT_SECRET")
    if not all([idp_token_url, idp_client_id, idp_client_secret]):
        logger.error("IDP_TOKEN_URL, IDP_CLIENT_ID, and IDP_CLIENT_SECRET environment variables must all be set")
        sys.exit(1)
    assert idp_token_url and idp_client_id and idp_client_secret
    token_manager = OAuth2TokenManager(idp_token_url, idp_client_id, idp_client_secret)

    # Inject LLM provider credentials as env vars (LiteLLM reads these natively)
    llm_configs_raw = os.environ.get("LLM_PROVIDER_CONFIGS", "[]")
    try:
        llm_configs = json.loads(llm_configs_raw)
        for entry in llm_configs:
            env_var = entry.get("envVar", "")
            value = entry.get("value", "")
            if env_var and value:
                os.environ[env_var] = value
                logger.debug("Set LLM provider env var: %s", env_var)
        if llm_configs:
            logger.info("Injected %d LLM provider credential(s)", len(llm_configs))
    except (json.JSONDecodeError, TypeError) as e:
        logger.warning("Failed to parse LLM_PROVIDER_CONFIGS: %s", e)

    logger.info(
        "Starting monitor evaluation monitor=%s agent=%s env=%s time_range=%s..%s sampling=%.1f endpoint=%s",
        args.monitor_name,
        args.agent_id,
        args.environment_id,
        args.trace_start,
        args.trace_end,
        args.sampling_rate,
        args.traces_api_endpoint,
    )

    # Validate time formats
    if not validate_time_format(args.trace_start):
        logger.error(
            "Invalid time format for --trace-start: %s. Expected ISO 8601 format",
            args.trace_start,
        )
        sys.exit(1)

    if not validate_time_format(args.trace_end):
        logger.error(
            "Invalid time format for --trace-end: %s. Expected ISO 8601 format",
            args.trace_end,
        )
        sys.exit(1)

    # Parse evaluators JSON
    try:
        evaluators_config = json.loads(args.evaluators)
    except json.JSONDecodeError as e:
        logger.error("Invalid JSON in --evaluators: %s", e)
        sys.exit(1)

    if not evaluators_config or not isinstance(evaluators_config, list):
        logger.error("--evaluators must be a non-empty array")
        sys.exit(1)

    for i, evaluator in enumerate(evaluators_config):
        if not isinstance(evaluator, dict):
            logger.error(
                "Evaluator at index %d must be an object/dict, got %s",
                i,
                type(evaluator).__name__,
            )
            sys.exit(1)

    evaluator_names_summary = [e.get("displayName", e.get("identifier", "unknown")) for e in evaluators_config]
    logger.info("Evaluators to run: %s", ", ".join(evaluator_names_summary))
    for evaluator in evaluators_config:
        config = evaluator.get("config", {})
        if config:
            logger.debug(
                "Evaluator '%s' config: %s",
                evaluator.get("displayName", evaluator.get("identifier")),
                config,
            )

    # Create evaluator instances with configurations
    # Build identifier lookup for publish: display_name -> identifier
    display_name_to_identifier = {}
    evaluator_instances = []
    for evaluator in evaluators_config:
        identifier = evaluator.get("identifier")
        display_name = evaluator.get("displayName")
        if not identifier:
            logger.error("Evaluator missing 'identifier' field")
            sys.exit(1)
        if not display_name:
            logger.error("Evaluator missing 'displayName' field")
            sys.exit(1)

        config = evaluator.get("config", {})
        eval_type = evaluator.get("type")  # None for built-in, "code" or "llm_judge" for custom

        try:
            if eval_type == "code":
                source = evaluator.get("source", "")
                instance = _load_custom_code_evaluator(identifier, source, config)
            elif eval_type == "llm_judge":
                source = evaluator.get("source", "")
                level = evaluator.get("level", "trace")
                instance = _create_custom_llm_judge(identifier, source, level, config)
            else:
                instance = builtin(identifier, **config)
            instance.name = display_name
            evaluator_instances.append(instance)
            display_name_to_identifier[display_name] = identifier
        except Exception as e:
            logger.error("Failed to register evaluator '%s': %s", identifier, e)
            sys.exit(1)

    # Initialize and run monitor
    try:
        fetcher = TraceFetcher(
            base_url=args.traces_api_endpoint,
            agent_uid=args.agent_id,
            environment_uid=args.environment_id,
        )

        monitor = Monitor(
            evaluators=evaluator_instances,
            trace_fetcher=fetcher,
        )

        # Run evaluation
        result = monitor.run(start_time=args.trace_start, end_time=args.trace_end)

        # Fail if there were errors (e.g. trace fetching failed)
        if result.errors:
            for err in result.errors:
                logger.error("Evaluation error: %s", err)
            sys.exit(1)

        # Check if any traces were evaluated
        if result.traces_evaluated == 0:
            logger.warning(
                "No traces found in time range %s..%s",
                args.trace_start,
                args.trace_end,
            )
            sys.exit(0)

        # Log results
        logger.info(
            "Evaluation complete: %d evaluator(s), %d trace(s), duration=%.1fs, status=%s",
            len(evaluator_instances),
            result.traces_evaluated,
            result.duration_seconds,
            "SUCCESS" if result.success else "FAILED",
        )

        if result.scores:
            for name, summary in result.scores.items():
                passed = summary.count - (summary.skipped_count or 0)
                agg_info = ""
                agg_scores = summary.aggregated_scores
                if "mean" in agg_scores:
                    agg_info = " mean=%.3f" % agg_scores["mean"]
                elif agg_scores:
                    first_key = next(iter(agg_scores))
                    agg_info = " %s=%s" % (first_key, agg_scores[first_key])

                if summary.skipped_count and summary.skipped_count > 0:
                    logger.info(
                        "  %s: %d/%d passed (%d skipped)%s",
                        name,
                        passed,
                        summary.count,
                        summary.skipped_count,
                        agg_info,
                    )
                else:
                    logger.info("  %s: %d/%d passed%s", name, passed, summary.count, agg_info)

                # Log skip reasons with unique reason breakdown
                if summary.skipped_count and summary.skipped_count > 0:
                    skip_reason_counts: Dict[str, int] = {}
                    for score in summary.individual_scores:
                        if score.skip_reason:
                            reason = score.skip_reason
                        elif not score.is_successful and score.score is None:
                            reason = "Evaluation did not produce a score"
                        else:
                            continue
                        skip_reason_counts[reason] = skip_reason_counts.get(reason, 0) + 1

                    for reason, count in skip_reason_counts.items():
                        logger.warning("  %s: %d skip(s) — reason: %s", name, count, reason)

        # Publish scores to agent-manager
        publish_success = publish_scores(
            monitor_id=args.monitor_id,
            run_id=args.run_id,
            scores=result.scores,
            display_name_to_identifier=display_name_to_identifier,
            api_endpoint=args.publisher_endpoint,
            token_manager=token_manager,
        )

        if not publish_success:
            logger.error("Failed to publish scores - evaluation results not persisted")
            sys.exit(1)

        # Exit with appropriate code
        sys.exit(0 if result.success else 1)

    except Exception as e:
        logger.error("Monitor execution failed: %s", e)
        logger.debug("Monitor execution failed", exc_info=True)
        sys.exit(1)


if __name__ == "__main__":
    main()
