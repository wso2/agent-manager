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
Class-based evaluators: Discover evaluators, run Monitor, print results per evaluator.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import Monitor, discover_evaluators
from amp_evaluation.trace import TraceLoader

import evaluators  # noqa: E402 — local evaluators module

DATA_DIR = Path(__file__).parent.parent / "data"


def main():
    # 1. Discover all class-based evaluator instances
    evals = discover_evaluators(evaluators)
    print(f"Discovered evaluators: {[e.name for e in evals]}")
    for ev in evals:
        info = ev.info
        print(f"  {info.name} (level={info.level}, modes={info.modes})")
    print()

    # 2. Run Monitor — traces are fetched and parsed internally
    loader = TraceLoader(
        file_path=str(DATA_DIR / "sample_traces.json"),
        agent_uid="sample-agent",
        environment_uid="dev",
    )
    monitor = Monitor(evaluators=evals, trace_fetcher=loader)
    result = monitor.run(limit=10)

    # 4. Print results per evaluator
    print(result.summary())


if __name__ == "__main__":
    main()
