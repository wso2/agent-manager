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
from amp_evaluation.trace import TraceLoader, parse_traces_for_evaluation

import evaluators  # noqa: E402 — local evaluators module

sys.path.insert(0, str(Path(__file__).parent))

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

    # 2. Discover all evaluators (class-based and decorator-based judges)
    evals = discover_evaluators(evaluators)
    print(f"Discovered {len(evals)} evaluators:")
    for ev in evals:
        info = ev.info
        print(f"  {info.name} (level={info.level}, modes={info.modes})")
    print()

    # 3. Run Monitor with error handling for missing API keys
    monitor = Monitor(evaluators=evals)
    try:
        result = monitor.run(traces=traces)
        print(result.summary())

        # 4. Show individual scores and explanations per evaluator
        print("\n" + "=" * 60)
        print("Individual Scores & Explanations")
        print("=" * 60)
        for evaluator_name, summary in result.scores.items():
            total = len(summary.individual_scores)
            errors = sum(1 for s in summary.individual_scores if s.is_error)
            passed = sum(1 for s in summary.individual_scores if not s.is_error and s.passed)
            failed = total - errors - passed
            print(f"\n--- {evaluator_name} (total={total}, passed={passed}, failed={failed}, errors={errors}) ---")
            for score in summary.individual_scores:
                if score.is_error:
                    print(f"  [ERR ] trace={score.trace_id[:12]}...")
                    if score.error:
                        print(f"         {score.error}")
                else:
                    status = "PASS" if score.passed else "FAIL"
                    print(f"  [{status}] trace={score.trace_id[:12]}... score={score.score:.2f}")
                    if score.explanation:
                        for line in score.explanation.strip().splitlines():
                            print(f"         {line}")

        # 5. Show which evaluators had errors vs succeeded
        if result.errors:
            print(f"\nNote: {len(result.errors)} errors occurred.")
            print("This is expected if LLM API keys are not configured.")
            print("Set OPENAI_API_KEY (or another provider key) to run LLM judges.")
    except Exception as e:
        print(f"Error running monitor: {e}")
        print("\nEnsure litellm is installed and an API key is set:")
        print("  pip install litellm")
        print("  export OPENAI_API_KEY=sk-...")


if __name__ == "__main__":
    main()
