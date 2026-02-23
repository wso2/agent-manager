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
Monitor production traces using the TraceFetcher API.

This sample demonstrates fetching traces from a real trace service API
and evaluating them with the Monitor runner. This is the recommended
approach for continuous monitoring of production agent behavior.

Prerequisites:
    1. Set environment variables (see .env.example)
    2. Ensure the trace service is accessible

Usage:
    python run.py
    python run.py --start 2025-01-01T00:00:00Z --end 2025-01-02T00:00:00Z
    python run.py --limit 50
"""

import os
import sys
import argparse
from pathlib import Path
from datetime import datetime, timedelta, timezone

# Add sample dir to path for evaluator imports
sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import Monitor, discover_evaluators, builtin
from amp_evaluation.trace import TraceFetcher

import evaluators  # noqa: E402 -- local evaluators module


def main():
    # Parse CLI arguments
    parser = argparse.ArgumentParser(description="Monitor production traces")
    parser.add_argument("--start", help="Start time (ISO 8601)")
    parser.add_argument("--end", help="End time (ISO 8601)")
    parser.add_argument("--limit", type=int, default=100, help="Max traces to fetch")
    args = parser.parse_args()

    # Configure from environment variables
    TRACE_SERVICE_URL = os.environ.get("TRACE_SERVICE_URL", "http://localhost:8001")
    AGENT_UID = os.environ.get("AGENT_UID", "my-agent")
    ENVIRONMENT_UID = os.environ.get("ENVIRONMENT_UID", "production")

    print(f"Trace service: {TRACE_SERVICE_URL}")
    print(f"Agent UID:     {AGENT_UID}")
    print(f"Environment:   {ENVIRONMENT_UID}")

    # Default time range: last 24 hours
    now = datetime.now(timezone.utc)
    start_time = args.start or (now - timedelta(hours=24)).isoformat()
    end_time = args.end or now.isoformat()

    # Create trace fetcher
    fetcher = TraceFetcher(
        base_url=TRACE_SERVICE_URL,
        agent_uid=AGENT_UID,
        environment_uid=ENVIRONMENT_UID,
    )

    # Health check
    if not fetcher.health_check():
        print(f"\nTrace service at {TRACE_SERVICE_URL} is not accessible.")
        print("Set TRACE_SERVICE_URL environment variable to your trace service URL.")
        print("See .env.example for configuration options.")
        sys.exit(1)

    print(f"\nFetching traces from {start_time} to {end_time} (limit: {args.limit})...")

    # Discover custom evaluators + add built-in evaluators
    evals = discover_evaluators(evaluators)
    evals.append(builtin("latency", max_latency_ms=5000))
    evals.append(builtin("hallucination"))

    print(f"Evaluators: {[e.name for e in evals]}")

    # Run monitor evaluation — traces are fetched and parsed internally
    monitor = Monitor(evaluators=evals, trace_fetcher=fetcher)
    result = monitor.run(
        start_time=start_time,
        end_time=end_time,
        limit=args.limit,
    )

    # Print summary
    print("\n" + result.summary())

    # Print per-evaluator pass rates
    print("\nPass rates:")
    for name, summary in result.scores.items():
        pass_rate = summary.pass_rate
        mean = summary.mean
        if pass_rate is not None:
            mean_str = f"{mean:.3f}" if mean is not None else "N/A"
            print(f"  {name:25s} pass_rate={pass_rate:.1%}  mean={mean_str}")


if __name__ == "__main__":
    main()
