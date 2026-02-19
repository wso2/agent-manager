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
Monitor runner for a Swiss Airlines customer support agent.

Evaluates recent traces against a comprehensive set of evaluators at multiple levels:

  Trace-level (whole trace):
    - tool-call-relevance: Are the right tools being called?
    - response-grounding: Are responses based on actual tool results?
    - response-completeness: Are responses complete and not broken?
    - llm-hallucination-judge: LLM-verified hallucination detection
    - answer_relevancy (built-in): Is the response relevant to the input?

  Agent-level (per agent):
    - agent-tool-efficiency: Is the agent using tools efficiently?
    - latency (built-in): Is agent latency within bounds?
    - hallucination (built-in): Keyword-based hallucination detection per agent

  Span-level (per span):
    - tool-success-rate: Did each tool execute without errors?
    - llm-response-quality: Is each LLM call producing valid output?

Usage:
    # Run against last 7 days of traces
    python run_monitor.py

    # Custom time range
    python run_monitor.py --start 2025-07-01T00:00:00Z --end 2025-07-08T00:00:00Z

    # Limit traces for testing
    python run_monitor.py --limit 10

    # Different environment
    python run_monitor.py --environment Production

Environment variables:
    AMP_API_KEY         - API key for the AMP platform
    AMP_API_URL         - AMP platform URL (default: https://api.amp.wso2.com)
    AMP_MONITOR_ID      - (Optional) Monitor ID to publish results to
    AMP_INTERNAL_API_KEY - (Optional) API key for internal endpoint (required if publishing)
    OPENAI_API_KEY      - Required for llm-hallucination-judge evaluator
    JUDGE_MODEL         - LLM model for judging (default: gpt-4o-mini)
"""

import argparse
import sys
import os
from datetime import datetime, timedelta, timezone

from dotenv import load_dotenv

load_dotenv()

# Import evaluators — the @evaluator decorators and register_evaluator() calls
# register them to the global registry automatically on import
import evaluators  # noqa: F401, E402

from amp_evaluation import (  # noqa: E402
    Monitor,
    list_evaluators,
    get_evaluator_metadata,
    register_builtin,
)


# =============================================================================
# Register built-in evaluators alongside the custom ones
# =============================================================================

# Latency: registered at agent level — checks per-agent duration
register_builtin("latency", max_latency_ms=5000, level="agent")

# Hallucination: registered at span level — checks each span for hallucination keywords
register_builtin("hallucination", level="span")

# Answer relevancy: trace-level (only supports trace) — checks word overlap between input and output
register_builtin("answer_relevancy")


# =============================================================================
# CLI
# =============================================================================


def parse_args():
    parser = argparse.ArgumentParser(
        description="Run evaluation monitor for the customer support agent",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Quick test with 5 traces
  python run_monitor.py --limit 5

  # Last 24 hours in production
  python run_monitor.py --start 2025-07-09T00:00:00Z --end 2025-07-10T00:00:00Z --environment Production

  # Full week scan
  python run_monitor.py --start 2025-07-01T00:00:00Z --end 2025-07-08T00:00:00Z --limit 500
        """,
    )

    now = datetime.now(timezone.utc)
    default_end = now.strftime("%Y-%m-%dT%H:%M:%SZ")
    default_start = (now - timedelta(days=7)).strftime("%Y-%m-%dT%H:%M:%SZ")

    parser.add_argument(
        "--start",
        type=str,
        default=default_start,
        help=f"Start time in RFC3339 format (default: 7 days ago, {default_start})",
    )
    parser.add_argument(
        "--end",
        type=str,
        default=default_end,
        help=f"End time in RFC3339 format (default: now, {default_end})",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=100,
        help="Maximum number of traces to evaluate (default: 100)",
    )
    parser.add_argument(
        "--environment",
        type=str,
        default="Development",
        help="Environment to pull traces from (default: Development)",
    )

    return parser.parse_args()


def print_header(args):
    print("=" * 70)
    print("  Customer Support Agent — Evaluation Monitor")
    print("=" * 70)
    print(f"  Environment:  {args.environment}")
    print(f"  Time range:   {args.start}")
    print(f"                {args.end}")
    print(f"  Trace limit:  {args.limit}")
    print("=" * 70)
    print()


def print_evaluator_list(evaluator_names):
    """Print loaded evaluators grouped by level."""
    print(f"  Loaded {len(evaluator_names)} evaluators:")
    print()

    # Group by level
    by_level = {"trace": [], "agent": [], "span": []}
    for name in evaluator_names:
        meta = get_evaluator_metadata(name)
        levels = meta.get("supported_levels", ["trace"])
        # Show at the highest level it supports
        if "span" in levels:
            by_level["span"].append((name, levels))
        elif "agent" in levels:
            by_level["agent"].append((name, levels))
        else:
            by_level["trace"].append((name, levels))

    level_labels = {
        "trace": "Trace-level (whole trace)",
        "agent": "Agent-level (per agent)",
        "span": "Span-level (per span)",
    }

    for level_key in ["trace", "agent", "span"]:
        evals = by_level[level_key]
        if evals:
            print(f"  {level_labels[level_key]}:")
            for name, levels in evals:
                levels_str = ", ".join(levels)
                print(f"    - {name}  [{levels_str}]")
            print()


def print_results(result):
    """Print a formatted summary of evaluation results."""

    print()
    print("=" * 70)
    print("  EVALUATION RESULTS")
    print("=" * 70)
    print()

    # --- Run metadata ---
    print(f"  Run ID:              {result.run_id}")
    print(f"  Status:              {'SUCCESS' if result.success else 'FAILED'}")
    print(f"  Duration:            {result.duration_seconds:.1f}s")
    print(f"  Traces evaluated:    {result.traces_evaluated}")
    print(f"  Evaluators run:      {result.evaluators_run}")
    print()

    if not result.scores:
        print("  No evaluation scores recorded.")
        print()
        if result.errors:
            print_errors(result)
        return

    # --- Scores table ---
    print("  +-----------------------------+----------+----------+-------+")
    print("  | Evaluator                   | Avg      | Pass Rate| Count |")
    print("  +-----------------------------+----------+----------+-------+")

    for name, summary in result.scores.items():
        avg = summary.aggregated_scores.get("mean", 0.0)
        pass_rate = summary.aggregated_scores.get("pass_rate")
        count = summary.count

        # Color indicator
        if avg >= 0.8:
            indicator = "🟢"
        elif avg >= 0.5:
            indicator = "🟡"
        else:
            indicator = "🔴"

        # Format name (truncate if too long)
        display_name = name[:25] if len(name) > 25 else name

        # Format pass rate
        pass_str = f"{pass_rate:.0%}" if pass_rate is not None else "  —"

        print(f"  | {indicator} {display_name:<25}| {avg:>7.3f}  | {pass_str:>7}  | {count:>5} |")

    print("  +-----------------------------+----------+----------+-------+")
    print()

    # --- Detailed breakdown per evaluator ---
    print("  DETAILED BREAKDOWN")
    print("  " + "-" * 66)

    for name, summary in result.scores.items():
        print()
        print(f"  >> {name}")

        for agg_name, value in summary.aggregated_scores.items():
            label = agg_name.replace("_", " ").title()
            if agg_name == "pass_rate":
                print(f"     {label:<20} {value:.1%}")
            else:
                print(f"     {label:<20} {value:.4f}")

        print(f"     {'Count':<20} {summary.count}")

    print()

    # --- Health assessment ---
    print_health_assessment(result)

    # --- Errors ---
    if result.errors:
        print_errors(result)


def print_health_assessment(result):
    """Print an overall health assessment based on scores."""
    print("  HEALTH ASSESSMENT")
    print("  " + "-" * 66)

    alerts = []
    healthy = []

    for name, summary in result.scores.items():
        avg = summary.aggregated_scores.get("mean", 0.0)
        pass_rate = summary.aggregated_scores.get("pass_rate")

        if avg < 0.5:
            alerts.append(f"  [CRITICAL] {name}: avg score {avg:.3f} is critically low")
        elif avg < 0.7:
            alerts.append(f"  [WARNING]  {name}: avg score {avg:.3f} needs attention")
        else:
            healthy.append(name)

        if pass_rate is not None and pass_rate < 0.5:
            alerts.append(f"  [CRITICAL] {name}: pass rate {pass_rate:.0%} — majority of traces failing")
        elif pass_rate is not None and pass_rate < 0.8:
            alerts.append(f"  [WARNING]  {name}: pass rate {pass_rate:.0%} — significant failure rate")

    if not alerts:
        print("  [OK] All evaluators healthy")
    else:
        for alert in alerts:
            print(alert)

    if healthy:
        print(f"  [OK] Healthy: {', '.join(healthy)}")

    print()


def print_errors(result):
    """Print errors encountered during the run."""
    print("  ERRORS")
    print("  " + "-" * 66)

    for i, error in enumerate(result.errors[:10], 1):
        print(f"  {i}. {error}")

    if len(result.errors) > 10:
        print(f"  ... and {len(result.errors) - 10} more errors")

    print()


def publish_scores_to_platform(result, monitor_id: str):
    """
    Publish evaluation scores to the AMP platform internal API.

    Args:
        result: RunResult from monitor.run()
        monitor_id: Monitor ID from environment variable

    Environment variables required:
        AMP_API_URL: Base URL of the AMP platform
        AMP_INTERNAL_API_KEY: API key for internal endpoint authentication
    """
    import requests

    api_url = os.getenv("AMP_API_URL")
    api_key = os.getenv("AMP_INTERNAL_API_KEY")

    if not api_url:
        print("  [WARNING] AMP_API_URL not set - skipping results publishing")
        return False

    if not api_key:
        print("  [WARNING] AMP_INTERNAL_API_KEY not set - skipping results publishing")
        return False

    # Build the publish request payload
    individual_scores = []
    aggregated_scores = []

    for evaluator_name, summary in result.scores.items():
        # Add individual scores
        for score in summary.individual_scores:
            score_record = {
                "evaluatorName": evaluator_name,
                "level": summary.level,
                "traceId": score.trace_id,
                "spanId": score.span_id,
                "score": score.score if not score.is_error else None,
                "explanation": score.explanation,
                "traceTimestamp": score.timestamp.isoformat() if score.timestamp else None,
                "metadata": score.metadata,
                "error": score.error,
            }
            individual_scores.append(score_record)

        # Add aggregate scores with flexible aggregations
        # Count errors (scores with error field populated)
        error_count = sum(1 for s in summary.individual_scores if s.error is not None)

        # Build aggregations map from all computed aggregations
        aggregations = {}
        for agg_name, agg_value in summary.aggregated_scores.items():
            # Convert python naming (e.g., pass_rate_0.5) to more standard format
            if agg_value is not None:
                aggregations[agg_name] = agg_value

        aggregate_record = {
            "evaluatorName": evaluator_name,
            "level": summary.level,
            "count": summary.count,
            "aggregations": aggregations,
            "errorCount": error_count,
        }

        aggregated_scores.append(aggregate_record)

    payload = {
        "individualScores": individual_scores,
        "aggregatedScores": aggregated_scores,
    }

    # POST to internal endpoint
    endpoint = f"{api_url}/api/publisher/v1/monitors/{monitor_id}/runs/{result.run_id}/scores"
    headers = {
        "x-api-key": api_key,
        "Content-Type": "application/json",
    }

    try:
        print(f"  Publishing scores to {endpoint}...")
        response = requests.post(endpoint, json=payload, headers=headers, timeout=30)
        response.raise_for_status()
        print(f"  [OK] Scores published successfully (HTTP {response.status_code})")
        return True
    except requests.exceptions.RequestException as e:
        print(f"  [ERROR] Failed to publish scores: {e}")
        if hasattr(e, "response") and e.response is not None:
            print(f"  Response: {e.response.text}")
        return False


def main():
    args = parse_args()
    print_header(args)

    # Create the monitor — it auto-discovers evaluators registered
    # via @evaluator decorator and register_evaluator() / register_builtin()
    monitor = Monitor()

    # Show which evaluators were loaded (grouped by level)
    evaluator_names = list_evaluators()
    print_evaluator_list(evaluator_names)

    print("  Running evaluations...")
    print()

    # Run the monitor
    result = monitor.run(
        start_time=args.start,
        end_time=args.end,
        limit=args.limit,
    )

    # Print results
    print_results(result)

    # Publish scores to platform if monitor ID is provided
    monitor_id = os.getenv("AMP_MONITOR_ID")
    if monitor_id:
        print()
        print("=" * 70)
        print("  PUBLISHING RESULTS TO PLATFORM")
        print("=" * 70)
        publish_scores_to_platform(result, monitor_id)
    else:
        print()
        print("  [INFO] AMP_MONITOR_ID not set - skipping results publishing")
        print("         Set AMP_MONITOR_ID environment variable to publish scores")

    # Exit with non-zero if there were errors
    if not result.success:
        sys.exit(1)


if __name__ == "__main__":
    main()
