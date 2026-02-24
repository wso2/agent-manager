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
Module Discovery: Shows how discover_evaluators() finds evaluator instances.

discover_evaluators() scans a Python module for all BaseEvaluator instances.
Only instantiated evaluators are found — bare classes and non-evaluator objects
are ignored.
"""

import sys
from pathlib import Path

# Add sample dir to path for evaluator imports
sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import Monitor, discover_evaluators
from amp_evaluation.trace import TraceLoader

import evaluators  # noqa: E402 -- local evaluators module

DATA_DIR = Path(__file__).parent.parent / "data"


def main():
    # 1. Discover all evaluator instances in the module
    discovered = discover_evaluators(evaluators)

    print(f"Discovered {len(discovered)} evaluators:\n")
    for ev in discovered:
        modes = [m.value for m in ev._supported_eval_modes]
        print(f"  - {ev.name:20s} level={ev.level.value:6s} modes={modes}")

    # 2. Show what was NOT discovered
    print("\nNot discovered (by design):")
    print("  - NotDiscovered class:  not instantiated at module level")
    print("  - helper_string:        not a BaseEvaluator instance")
    print("  - helper_number:        not a BaseEvaluator instance")
    print("  - helper_list:          not a BaseEvaluator instance")
    print()

    # 3. Use discovered evaluators with Monitor — traces are fetched and parsed internally
    loader = TraceLoader(file_path=str(DATA_DIR / "sample_traces.json"))
    monitor = Monitor(evaluators=discovered, trace_fetcher=loader)
    result = monitor.run(limit=10)

    # 4. Print summary
    result.print_summary()


if __name__ == "__main__":
    main()
