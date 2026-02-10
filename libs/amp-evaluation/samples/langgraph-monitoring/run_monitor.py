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

Evaluates recent traces against a set of quality evaluators:
  - tool-call-relevance: Are the right tools being called?
  - response-grounding: Are responses based on actual tool results?
  - tool-success-rate: Are tools executing without errors?
  - response-completeness: Are responses complete and not broken?
  - llm-hallucination-judge: LLM-verified hallucination detection

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
    OPENAI_API_KEY      - Required for llm-hallucination-judge evaluator
    JUDGE_MODEL         - LLM model for judging (default: gpt-4o-mini)
"""

import argparse
import sys
from datetime import datetime, timedelta, timezone

# Import evaluators — the @evaluator decorators register them automatically
import evaluators  # noqa: F401

from amp_evaluation import Monitor, list_evaluators


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


def print_results(result):
    """Print a formatted summary of evaluation results."""

    print()
    print("=" * 70)
    print("  EVALUATION RESULTS")
    print("=" * 70)
    print()

    # --- Run metadata ---
    print(f"  Run ID:              {result.run_id}")
    print(f"  Status:              {'✅ SUCCESS' if result.success else '❌ FAILED'}")
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
    print("  ┌─────────────────────────────┬──────────┬──────────┬───────┐")
    print("  │ Evaluator                   │ Avg      │ Pass Rate│ Count │")
    print("  ├─────────────────────────────┼──────────┼──────────┼───────┤")

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

        print(f"  │ {indicator} {display_name:<25} │ {avg:>7.3f}  │ {pass_str:>7}  │ {count:>5} │")

    print("  └─────────────────────────────┴──────────┴──────────┴───────┘")
    print()

    # --- Detailed breakdown per evaluator ---
    print("  DETAILED BREAKDOWN")
    print("  " + "-" * 66)

    for name, summary in result.scores.items():
        print()
        print(f"  📊 {name}")

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
            alerts.append(f"🔴 {name}: avg score {avg:.3f} is critically low")
        elif avg < 0.7:
            alerts.append(f"🟡 {name}: avg score {avg:.3f} needs attention")
        else:
            healthy.append(name)

        if pass_rate is not None and pass_rate < 0.5:
            alerts.append(f"🔴 {name}: pass rate {pass_rate:.0%} — majority of traces failing")
        elif pass_rate is not None and pass_rate < 0.8:
            alerts.append(f"🟡 {name}: pass rate {pass_rate:.0%} — significant failure rate")

    if not alerts:
        print("  🟢 All evaluators healthy")
    else:
        for alert in alerts:
            print(f"  {alert}")

    if healthy:
        print(f"  ✅ Healthy: {', '.join(healthy)}")

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


def main():
    args = parse_args()
    print_header(args)

    # Create the monitor — it auto-discovers evaluators registered
    # via @evaluator decorator in the imported evaluators module
    monitor = Monitor()

    # Show which evaluators were loaded
    evaluator_names = list_evaluators()
    print(f"  Loaded {len(evaluator_names)} evaluators:")
    for name in evaluator_names:
        print(f"    • {name}")
    print()
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

    # Print the SDK's built-in summary as well
    print("  " + "=" * 66)
    print("  RAW SUMMARY")
    print("  " + "=" * 66)
    print()
    print(result.summary())

    # Exit with non-zero if there were errors
    if not result.success:
        sys.exit(1)


if __name__ == "__main__":
    main()
