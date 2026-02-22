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
Evaluation Levels: Demonstrates that different evaluator levels produce
different call counts — trace-level is called once per trace, agent-level
once per agent span, and LLM-level once per LLM call.
"""

import sys
from pathlib import Path

# Add sample dir to path for evaluator imports
sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import Monitor, discover_evaluators
from amp_evaluation.trace import TraceLoader, parse_traces_for_evaluation

import evaluators  # noqa: E402 — local evaluators module

DATA_DIR = Path(__file__).parent.parent / "data"


def load_sample_traces():
    loader = TraceLoader(
        file_path=str(DATA_DIR / "sample_traces.json"),
        agent_uid="sample-agent",
        environment_uid="dev",
    )
    otel_traces = loader.load_batch(limit=10)
    return parse_traces_for_evaluation(otel_traces)


def main():
    # 1. Load traces
    traces = load_sample_traces()
    print(f"Loaded {len(traces)} traces\n")

    # 2. Discover evaluators (trace-level, agent-level, llm-level)
    evals = discover_evaluators(evaluators)
    for ev in evals:
        print(f"  {ev.name:25s} level={ev.level.value}")
    print()

    # 3. Run Monitor
    monitor = Monitor(evaluators=evals)
    result = monitor.run(traces=traces)

    # 4. Print per-evaluator results — note the DIFFERENT call counts
    print("Results by evaluator:")
    print("-" * 60)
    for name, summary in result.scores.items():
        print(f"  {name}:")
        print(f"    level: {summary.level}")
        print(f"    count: {summary.count}")
        if summary.mean is not None:
            print(f"    mean:  {summary.mean:.2f}")
        print()

    print("Key observation:")
    print("  trace-level-check  -> called once per trace")
    print("  agent-level-check  -> called N times (once per agent span)")
    print("  llm-level-check    -> called M times (once per LLM call)")


if __name__ == "__main__":
    main()
