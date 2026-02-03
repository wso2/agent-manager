#!/usr/bin/env python3
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
Monitor a LangGraph customer support agent.

This script:
1. Loads config from .env file
2. Imports custom evaluators (auto-registers via @evaluator)
3. Runs Monitor to fetch and evaluate production traces
4. Prints results

Setup:
    1. Copy .env.example to .env
    2. Fill in your credentials:
       - OPENAI_API_KEY (for LLM-as-judge)
       - AGENT_UID and ENVIRONMENT_UID (to fetch traces)
       - AMP_API_URL (your AMP platform endpoint)

Usage:
    python run_monitoring.py

    # Or specify time range (defaults to last 24 hours)
    python run_monitoring.py --hours 12
    python run_monitoring.py --limit 50 --hours 24
"""

import os
import argparse
from datetime import datetime, timezone, timedelta

# Load environment variables from .env file
try:
    from dotenv import load_dotenv

    load_dotenv()
except ImportError:
    print("Warning: python-dotenv not installed. Install with: pip install python-dotenv")
    print("Loading environment variables from shell...")

# Import evaluators - this registers them automatically!
import evaluators  # noqa: F401

from amp_evaluation import Monitor, list_evaluators


# def check_config():
#     """Check if required config is present."""
#     required = {
#         "OPENAI_API_KEY": "For LLM-as-judge hallucination detection",
#         "AGENT_UID": "To identify which agent's traces to fetch",
#         "ENVIRONMENT_UID": "To identify the environment",
#     }

#     missing = []
#     for key, description in required.items():
#         if not os.getenv(key):
#             missing.append(f"  - {key}: {description}")

#     if missing:
#         print("⚠️  Missing required environment variables:")
#         print("\n".join(missing))
#         print("\nPlease set these in your .env file (see .env.example)")
#         print()
#         return False

#     return True


def main():
    # Parse arguments
    parser = argparse.ArgumentParser(description="Monitor LangGraph customer support agent")
    parser.add_argument("--hours", type=int, default=2400, help="Number of hours to look back (default: 24)")
    parser.add_argument("--limit", type=int, default=100, help="Maximum number of traces to evaluate (default: 100)")
    args = parser.parse_args()

    print("=" * 70)
    print("LangGraph Customer Support Agent Monitor")
    print("=" * 70)
    print()

    # Check configuration
    # if not check_config():
    #     print("Tip: Copy .env.example to .env and fill in your credentials")
    #     sys.exit(1)

    print("✓ Configuration loaded")
    print(f"  Agent UID: {os.getenv('AMP_AGENT_UID')}")
    print(f"  Environment: {os.getenv('AMP_ENVIRONMENT_UID')}")
    print(f"  API URL: {os.getenv('AMP_API_URL', 'http://localhost:8001')}")
    print()

    # Calculate time range (default: last 24 hours)
    end_time = datetime.now(timezone.utc)
    start_time = end_time - timedelta(hours=args.hours)

    # Convert to ISO format strings (required by API)
    start_time_str = start_time.isoformat()
    end_time_str = end_time.isoformat()

    print(f"⏰ Time range: Last {args.hours} hours")
    print(f"  From: {start_time.strftime('%Y-%m-%d %H:%M:%S UTC')}")
    print(f"  To:   {end_time.strftime('%Y-%m-%d %H:%M:%S UTC')}")
    print()

    # Show registered evaluators
    print("Registered evaluators:")
    for name in list_evaluators():
        print(f"  - {name}")
    print()

    # Create monitor - config loads from env vars automatically
    monitor = Monitor(
        # include_tags=["quality", "hallucination"]  # Run our custom evaluators
    )

    print(f"Running {monitor.evaluator_count} evaluators: {monitor.evaluator_names}")
    print()

    # Run evaluation with time range (MANDATORY for trace fetching)
    # API expects ISO 8601 format strings
    print("🔄 Fetching traces from AMP platform...")
    result = monitor.run(start_time=start_time_str, end_time=end_time_str, limit=args.limit)

    # Print trace count info
    print()
    print("=" * 70)
    print(f"✓ Fetched and evaluated {result.traces_evaluated} traces")
    print("=" * 70)
    print()

    # Print detailed results
    print(result.summary())


if __name__ == "__main__":
    main()
