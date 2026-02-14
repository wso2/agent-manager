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
import sys
from datetime import datetime

from amp_evaluation import Monitor, register_builtin
from amp_evaluation.trace import TraceFetcher


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
        "--verbose",
        action="store_true",
        help="Enable verbose logging",
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

    # Print configuration
    print("=" * 70)
    print("  AMP Monitor Evaluation")
    print("=" * 70)
    print(f"  Monitor:      {args.monitor_name}")
    print(f"  Agent ID:     {args.agent_id}")
    print(f"  Environment ID:  {args.environment_id}")
    print(f"  Evaluators:   {args.evaluators}")
    print(f"  Time Range:   {args.trace_start} → {args.trace_end}")
    print(f"  Sampling:     {args.sampling_rate}")
    print(f"  API Endpoint: {args.traces_api_endpoint}")
    print("=" * 70)
    print()

    # Validate time formats
    if not validate_time_format(args.trace_start):
        print(f"Error: Invalid time format for --trace-start: {args.trace_start}")
        print("   Expected ISO 8601 format (e.g., 2026-01-01T00:00:00Z)")
        sys.exit(1)

    if not validate_time_format(args.trace_end):
        print(f"Error: Invalid time format for --trace-end: {args.trace_end}")
        print("   Expected ISO 8601 format (e.g., 2026-01-01T00:00:00Z)")
        sys.exit(1)

    # Parse evaluators JSON
    try:
        evaluators_config = json.loads(args.evaluators)
    except json.JSONDecodeError as e:
        print(f"Error: Invalid JSON in --evaluators: {e}")
        sys.exit(1)

    if not evaluators_config or not isinstance(evaluators_config, list):
        print("Error: --evaluators must be a non-empty array")
        sys.exit(1)

    print(f"Running {len(evaluators_config)} evaluators:")
    for evaluator in evaluators_config:
        name = evaluator.get("name")
        config = evaluator.get("config", {})
        print(f"   - {name}" + (f" (with config: {config})" if config else ""))
    print()

    # Register built-in evaluators with configurations
    evaluator_names = []
    for evaluator in evaluators_config:
        name = evaluator.get("name")
        if not name:
            print("Error: Evaluator missing 'name' field")
            sys.exit(1)

        config = evaluator.get("config", {})

        try:
            register_builtin(name, **config)  # Pass config as kwargs
            evaluator_names.append(name)
        except (ValueError, ImportError) as e:
            print(f"Error: Failed to register evaluator '{name}': {e}")
            sys.exit(1)
        except TypeError as e:
            print(f"Error: Invalid config for evaluator '{name}': {e}")
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
            print("=" * 70)
            print("  No traces found in the specified time range")
            print("=" * 70)
            sys.exit(0)

        # Print results
        print("=" * 70)
        print("  EVALUATION RESULTS")
        print("=" * 70)
        print(f"  Traces Evaluated:    {result.traces_evaluated}")
        print(f"  Duration:            {result.duration_seconds:.1f}s")
        print(f"  Status:              {'SUCCESS' if result.success else 'FAILED'}")
        print()

        if result.scores:
            print("  Evaluator Scores:")
            print("  " + "-" * 66)
            for name, summary in result.scores.items():
                # Get mean score if available
                agg_scores = summary.aggregated_scores
                if "mean" in agg_scores:
                    mean_val = agg_scores["mean"]
                    print(f"  {name:<30} Mean: {mean_val:.3f}")
                elif agg_scores:
                    # Print first available aggregation
                    first_key = next(iter(agg_scores))
                    first_val = agg_scores[first_key]
                    if isinstance(first_val, (int, float)):
                        print(f"  {name:<30} {first_key}: {first_val:.3f}")
                    else:
                        print(f"  {name:<30} {first_key}: {first_val}")
            print("  " + "-" * 66)
        print()

        # Print errors if any
        if result.errors:
            print("  Errors:")
            for error in result.errors[:10]:  # Limit to first 10 errors
                print(f"    - {error}")
            if len(result.errors) > 10:
                print(f"    ... and {len(result.errors) - 10} more errors")
            print()

        # Exit with appropriate code
        sys.exit(0 if result.success else 1)

    except Exception as e:
        print(f"Error during monitor execution: {e}")
        if args.verbose:
            import traceback

            traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()
