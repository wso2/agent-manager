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
Param Config: Load traces, use default + strict evaluator variants,
run Monitor, print results showing different scores, and inspect config schemas.
"""

import sys
from pathlib import Path

# Add sample dir to path for evaluator imports
sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import Monitor
from amp_evaluation.trace import TraceLoader

import evaluators  # noqa: E402 — local evaluators module

DATA_DIR = Path(__file__).parent.parent / "data"


def main():
    # 1. Rename strict variants so they appear separately in results
    evaluators.response_length_strict.name = "response-length-strict"
    evaluators.latency_strict.name = "latency-threshold-strict"

    # 3. Collect all evaluator variants (default + strict)
    evals = [
        evaluators.response_length_default,
        evaluators.response_length_strict,
        evaluators.latency_threshold,
        evaluators.latency_strict,
    ]

    print("Evaluators:")
    for ev in evals:
        print(f"  {ev.name}")
    print()

    # 3. Run Monitor — traces are fetched and parsed internally
    loader = TraceLoader(
        file_path=str(DATA_DIR / "sample_traces.json"),
        agent_uid="sample-agent",
        environment_uid="dev",
    )
    monitor = Monitor(evaluators=evals, trace_fetcher=loader)
    result = monitor.run(limit=10)

    # 5. Print results — compare default vs strict scores
    print("Results:")
    print("-" * 60)
    for name, summary in result.scores.items():
        mean = summary.mean if summary.mean is not None else 0.0
        print(f"  {name:35s} count={summary.count}  mean={mean:.2f}")
    print()

    # 6. Inspect .info property to show config schema
    print("Config schema (via .info property):")
    print("-" * 60)
    for ev in evals:
        info = ev.info
        print(f"\n  {info.name}:")
        print(f"    level: {info.level}")
        print(f"    modes: {info.modes}")
        if info.config_schema:
            print("    config_schema:")
            for param in info.config_schema:
                print(
                    f"      - {param['key']}: {param['type']} "
                    f"(default={param.get('default', 'N/A')}, "
                    f'description="{param.get("description", "")}")'
                )
        else:
            print("    config_schema: (none)")


if __name__ == "__main__":
    main()
