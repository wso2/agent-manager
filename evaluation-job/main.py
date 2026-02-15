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
import sys
from datetime import datetime

from amp_evaluation import Monitor, register_builtin
from amp_evaluation.trace import TraceFetcher

logger = logging.getLogger(__name__)


class JsonFormatter(logging.Formatter):
    """Format log records as single-line JSON matching Go slog output."""

    def format(self, record):
        return json.dumps(
            {
                "time": self.formatTime(record, self.datefmt),
                "level": record.levelname,
                "msg": record.getMessage(),
                "logger": record.name,
            }
        )


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

    return parser.parse_args()


def validate_time_format(time_str: str) -> bool:
    """Validate ISO 8601 time format."""
    try:
        datetime.fromisoformat(time_str.replace("Z", "+00:00"))
        return True
    except ValueError:
        return False


def main():
    """Main entry point for monitor job."""
    args = parse_args()
    configure_logging()

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

    evaluator_names_summary = [e.get("name", "unknown") for e in evaluators_config]
    logger.info("Evaluators to run: %s", ", ".join(evaluator_names_summary))
    for evaluator in evaluators_config:
        config = evaluator.get("config", {})
        if config:
            logger.debug("Evaluator '%s' config: %s", evaluator.get("name"), config)

    # Register built-in evaluators with configurations
    evaluator_names = []
    for evaluator in evaluators_config:
        name = evaluator.get("name")
        if not name:
            logger.error("Evaluator missing 'name' field")
            sys.exit(1)

        config = evaluator.get("config", {})

        try:
            register_builtin(name, **config)  # Pass config as kwargs
            evaluator_names.append(name)
        except (ValueError, ImportError) as e:
            logger.error("Failed to register evaluator '%s': %s", name, e)
            sys.exit(1)
        except TypeError as e:
            logger.error("Invalid config for evaluator '%s': %s", name, e)
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

        # Exit with appropriate code
        sys.exit(0 if result.success else 1)

    except Exception as e:
        logger.error("Monitor execution failed: %s", e)
        logger.debug("Monitor execution failed", exc_info=True)
        sys.exit(1)


if __name__ == "__main__":
    main()
