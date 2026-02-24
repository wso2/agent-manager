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
LLM-as-judge evaluators: Discover evaluators, run Monitor, handle missing API keys.

This sample requires:
  pip install litellm
  export OPENAI_API_KEY=...  (or another LLM provider key)

If the LLM API key is not set, the sample will still run but LLM-based
evaluators will report errors gracefully.
"""

import sys
from pathlib import Path

from amp_evaluation import Monitor, discover_evaluators
from amp_evaluation.trace import TraceLoader

import evaluators  # noqa: E402 — local evaluators module

sys.path.insert(0, str(Path(__file__).parent))

DATA_DIR = Path(__file__).parent.parent / "data"


def main():
    # 1. Discover all evaluators (class-based and decorator-based judges)
    evals = discover_evaluators(evaluators)
    print(f"Discovered {len(evals)} evaluators:")
    for ev in evals:
        info = ev.info
        print(f"  {info.name} (level={info.level}, modes={info.modes})")
    print()

    # 2. Run Monitor with error handling for missing API keys
    loader = TraceLoader(file_path=str(DATA_DIR / "sample_traces.json"))
    monitor = Monitor(evaluators=evals, trace_fetcher=loader)
    try:
        result = monitor.run(limit=10)

        # 4. Show detailed results with individual scores and explanations
        result.print_summary(verbosity="detailed")

        # 5. Show runner-level errors if any
        if result.errors:
            print(f"\nNote: {len(result.errors)} runner errors occurred.")
            print("This is expected if LLM API keys are not configured.")
            print("Set OPENAI_API_KEY (or another provider key) to run LLM judges.")
    except Exception as e:
        print(f"Error running monitor: {e}")
        print("\nEnsure litellm is installed and an API key is set:")
        print("  pip install litellm")
        print("  export OPENAI_API_KEY=sk-...")


if __name__ == "__main__":
    main()
