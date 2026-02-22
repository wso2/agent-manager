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
Decorator-based evaluators: Discover evaluators, run Monitor, print results.

Shows that decorator-created evaluators are discoverable just like class-based
ones -- discover_evaluators() finds all BaseEvaluator instances in a module,
regardless of how they were created.
"""

import sys
from pathlib import Path

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

    # 2. Discover decorator-based evaluators
    evals = discover_evaluators(evaluators)
    print(f"Discovered evaluators: {[e.name for e in evals]}")
    for ev in evals:
        info = ev.info
        print(f"  {info.name}: {info.description} (level={info.level}, tags={info.tags})")
    print()

    # 3. Run Monitor
    monitor = Monitor(evaluators=evals)
    result = monitor.run(traces=traces)

    # 4. Print summary
    print(result.summary())


if __name__ == "__main__":
    main()
