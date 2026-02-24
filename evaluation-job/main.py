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
        --evaluators='[{"name":"latency","config":{"max_latency_ms":3000}}]' \
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
from amp_evaluation import Monitor, register_builtin
from amp_evaluation.models import EvaluatorSummary
from amp_evaluation.trace import TraceFetcher

logger = logging.getLogger(__name__)

# Maximum number of retries for publishing scores
PUBLISH_MAX_RETRIES = 3
PUBLISH_INITIAL_BACKOFF = 2  # seconds


def handle_sigterm(signum, frame):
    """Handle SIGTERM for graceful shutdown in Kubernetes."""
    logger.info("Received SIGTERM, shutting down gracefully")
    sys.exit(0)


signal.signal(signal.SIGTERM, handle_sigterm)


class JsonFormatter(logging.Formatter):
    """Format log records as single-line JSON matching Go slog output."""

    def format(self, record):
        log_entry = {
            "time": self.formatTime(record, self.datefmt),
            "level": record.levelname,
            "msg": record.getMessage(),
            "logger": record.name,
        }
        if record.exc_info and record.exc_info[0] is not None:
            log_entry["trace"] = self.formatException(record.exc_info)
        if record.stack_info:
            log_entry["stack"] = self.formatStack(record.stack_info)
        return json.dumps(log_entry)


def configure_logging():
    """Configure JSON logging from LOG_LEVEL env var (default: INFO)."""
    level_name = os.environ.get("LOG_LEVEL", "INFO").upper()
    level = getattr(logging, level_name, logging.INFO)
    handler = logging.StreamHandler()
    handler.setFormatter(JsonFormatter(datefmt="%Y-%m-%dT%H:%M:%S"))
    logging.basicConfig(level=level, handlers=[handler])


def parse_args():
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
        help='JSON array of evaluator configurations (e.g., \'[{"name":"latency","config":{"max_latency_ms":3000}}]\')',
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
    api_key: str,
) -> bool:
    """
    Publish evaluation scores to the agent-manager internal API.

    Args:
        monitor_id: Monitor UUID
        run_id: Run UUID
        scores: Dict of evaluator display_name -> EvaluatorSummary from RunResult
        display_name_to_identifier: Mapping of display_name -> evaluator identifier
        api_endpoint: Agent Manager internal API base URL
        api_key: API key for authentication

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
                "displayName": display_name,
                "level": summary.level,
                "aggregations": summary.aggregated_scores,
                "count": summary.count,
                "errorCount": sum(1 for s in summary.individual_scores if s.is_error),
            }
        )

        # Add individual scores (per-trace/span scores)
        for score in summary.individual_scores:
            item: Dict[str, Any] = {
                "displayName": display_name,
                "level": summary.level,
                "traceId": score.trace_id,
            }

            # Optional fields
            if score.span_id:
                item["spanId"] = score.span_id
            if not score.is_error and score.score is not None:
                item["score"] = score.score
            if score.explanation:
                item["explanation"] = score.explanation
            if score.timestamp:
                item["traceTimestamp"] = score.timestamp.isoformat()
            if score.metadata:
                item["metadata"] = score.metadata
            if score.error:
                item["error"] = score.error

            individual_scores.append(item)

    payload = {
        "individualScores": individual_scores,
        "aggregatedScores": aggregated_scores,
    }

    # Publish scores to agent-manager API
    url = f"{api_endpoint}/api/v1/publisher/monitors/{monitor_id}/runs/{run_id}/scores"
    headers = {
        "x-api-key": api_key,
        "Content-Type": "application/json",
    }

    logger.info(
        "Publishing scores monitor_id=%s run_id=%s evaluators=%d individual_scores=%d",
        monitor_id,
        run_id,
        len(scores),
        len(individual_scores),
    )

    for attempt in range(PUBLISH_MAX_RETRIES):
        try:
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


def main():
    """Main entry point for monitor job."""
    configure_logging()
    args = parse_args()

    # Read API key from environment variable (injected via Kubernetes Secret)
    publisher_api_key = os.environ.get("PUBLISHER_API_KEY")
    if not publisher_api_key:
        logger.error("PUBLISHER_API_KEY environment variable is not set")
        sys.exit(1)

    logger.info(
        "Starting monitor evaluation monitor=%s agent=%s env=%s "
        "time_range=%s..%s sampling=%.1f endpoint=%s",
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

    evaluator_names_summary = [
        e.get("displayName", e.get("identifier", "unknown")) for e in evaluators_config
    ]
    logger.info("Evaluators to run: %s", ", ".join(evaluator_names_summary))
    for evaluator in evaluators_config:
        config = evaluator.get("config", {})
        if config:
            logger.debug(
                "Evaluator '%s' config: %s",
                evaluator.get("displayName", evaluator.get("identifier")),
                config,
            )

    # Register built-in evaluators with configurations
    # Build identifier lookup for publish: display_name -> identifier
    display_name_to_identifier = {}
    evaluator_names = []
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

        try:
            register_builtin(identifier, display_name=display_name, **config)
            evaluator_names.append(display_name)
            display_name_to_identifier[display_name] = identifier
        except (ValueError, ImportError) as e:
            logger.error("Failed to register evaluator '%s': %s", identifier, e)
            sys.exit(1)
        except TypeError as e:
            logger.error("Invalid config for evaluator '%s': %s", identifier, e)
            sys.exit(1)

    # Initialize and run monitor
    try:
        fetcher = TraceFetcher(
            base_url=args.traces_api_endpoint,
            agent_uid=args.agent_id,
            environment_uid=args.environment_id,
        )

        monitor = Monitor(
            trace_fetcher=fetcher,
            include=evaluator_names,  # Only run these registered evaluators
        )

        # Run evaluation
        result = monitor.run(start_time=args.trace_start, end_time=args.trace_end)

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
            "Evaluation complete traces_evaluated=%d duration=%.1fs status=%s",
            result.traces_evaluated,
            result.duration_seconds,
            "SUCCESS" if result.success else "FAILED",
        )

        if result.scores:
            for name, summary in result.scores.items():
                agg_scores = summary.aggregated_scores
                if "mean" in agg_scores:
                    logger.debug(
                        "Evaluator score %s mean=%.3f", name, agg_scores["mean"]
                    )
                elif agg_scores:
                    first_key = next(iter(agg_scores))
                    logger.debug(
                        "Evaluator score %s %s=%s",
                        name,
                        first_key,
                        agg_scores[first_key],
                    )

        # Log errors if any
        if result.errors:
            logger.warning("Evaluation produced %d error(s)", len(result.errors))
            for error in result.errors[:5]:
                logger.debug("Evaluation error: %s", error)
            if len(result.errors) > 5:
                logger.debug("... and %d more errors", len(result.errors) - 5)

        # Publish scores to agent-manager
        publish_success = publish_scores(
            monitor_id=args.monitor_id,
            run_id=args.run_id,
            scores=result.scores,
            display_name_to_identifier=display_name_to_identifier,
            api_endpoint=args.publisher_endpoint,
            api_key=publisher_api_key,
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
