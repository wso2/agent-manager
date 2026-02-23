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
Monitor mode: experiment-only evaluators are automatically skipped.
"""

import sys
from pathlib import Path

# Add sample dir to path for evaluator imports
sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import Monitor, discover_evaluators
from amp_evaluation.trace import TraceLoader

import evaluators  # noqa: E402 — local evaluators module

DATA_DIR = Path(__file__).parent.parent / "data"


def main():
    # 1. Discover all evaluators
    evals = discover_evaluators(evaluators)
    print(f"Discovered evaluators: {[e.name for e in evals]}")
    for ev in evals:
        modes = [m.value for m in ev._supported_eval_modes]
        print(f"  {ev.name:25s} modes={modes}")
    print()

    # 2. Run Monitor — experiment-only evaluators are skipped automatically
    print("Running Monitor mode...")
    loader = TraceLoader(
        file_path=str(DATA_DIR / "sample_traces.json"),
        agent_uid="sample-agent",
        environment_uid="dev",
    )
    monitor = Monitor(evaluators=evals, trace_fetcher=loader)
    result = monitor.run(limit=10)

    # 4. Print results — experiment-only evaluator should NOT appear
    print(f"\nEvaluators that ran: {list(result.scores.keys())}")
    print()
    for name, summary in result.scores.items():
        mean_str = f"{summary.mean:.2f}" if summary.mean is not None else "N/A"
        print(f"  {name}: count={summary.count}, mean={mean_str}")

    print("\nNote: 'experiment-only' was skipped because Monitor mode has no tasks.")


if __name__ == "__main__":
    main()
