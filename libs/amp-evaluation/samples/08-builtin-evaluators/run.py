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
Built-in evaluators: List, inspect, configure, and run built-in evaluators.

No custom evaluators.py needed -- this sample uses only the evaluators
that ship with amp-evaluation.

Demonstrates:
- list_builtin_evaluators() to discover available evaluators by mode
- builtin_evaluator_catalog() to get full metadata for each evaluator
- builtin() factory to create configured evaluator instances
- Running monitor-compatible built-ins with Monitor
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import builtin, list_builtin_evaluators, builtin_evaluator_catalog, Monitor
from amp_evaluation.trace import TraceLoader

DATA_DIR = Path(__file__).parent.parent / "data"


# =========================================================================
# Part 1: List all available built-in evaluators
# =========================================================================


def show_builtin_names():
    print("=== All Built-in Evaluators ===")
    all_names = list_builtin_evaluators()
    for name in all_names:
        print(f"  - {name}")

    print("\n=== Monitor-Compatible Built-ins ===")
    monitor_names = list_builtin_evaluators(mode="monitor")
    for name in monitor_names:
        print(f"  - {name}")

    print("\n=== Experiment-Only Built-ins ===")
    experiment_names = list_builtin_evaluators(mode="experiment")
    experiment_only = [n for n in experiment_names if n not in list_builtin_evaluators(mode="monitor")]
    if experiment_only:
        for name in experiment_only:
            print(f"  - {name}")
    else:
        print("  (all experiment evaluators also support monitor mode)")


# =========================================================================
# Part 2: Catalog with full metadata
# =========================================================================


def show_catalog():
    print("\n=== Evaluator Catalog ===")
    catalog = builtin_evaluator_catalog()
    for info in catalog:
        print(f"\n  {info.name}")
        print(f"    Description: {info.description}")
        print(f"    Level: {info.level}")
        print(f"    Modes: {info.modes}")
        if info.config_schema:
            print(f"    Config: {info.config_schema}")


# =========================================================================
# Part 3: Create and run monitor-compatible built-ins
# =========================================================================


def run_monitor_evaluators():
    print("\n=== Running Monitor with Built-in Evaluators ===\n")

    # Create configured built-in evaluator instances
    monitor_evals = [
        builtin("latency", max_latency_ms=10000),
        builtin("token_efficiency", max_tokens=10000),
        builtin("iteration_count", max_iterations=20),
        builtin("answer_length", min_length=1, max_length=10000),
    ]

    print(f"Created {len(monitor_evals)} built-in evaluators:")
    for ev in monitor_evals:
        print(f"  - {ev.name}")
    print()

    # Run monitor — traces are fetched and parsed internally
    loader = TraceLoader(file_path=str(DATA_DIR / "sample_traces.json"))
    monitor = Monitor(evaluators=monitor_evals, trace_fetcher=loader)
    result = monitor.run(limit=10)

    result.print_summary()


# =========================================================================
# Main
# =========================================================================


def main():
    show_builtin_names()
    show_catalog()
    run_monitor_evaluators()


if __name__ == "__main__":
    main()
