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
DeepEval evaluator integration sample.

Demonstrates all 6 DeepEval evaluators available through the builtin() factory.
DeepEval evaluators are experiment-only (they require task data / ground truth).

Prerequisites:
    pip install amp-evaluation[deepeval]
    export OPENAI_API_KEY=your-key-here
"""

import sys
from pathlib import Path

# Add sample dir to path for evaluator imports
sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import builtin, Experiment, HttpAgentInvoker, load_dataset_from_json
from amp_evaluation.trace import TraceLoader, parse_traces_for_evaluation

DATA_DIR = Path(__file__).parent.parent / "data"


def load_sample_traces():
    loader = TraceLoader(file_path=str(DATA_DIR / "sample_traces.json"))
    otel_traces = loader.load_traces()
    return parse_traces_for_evaluation(otel_traces)


def main():
    # 1. Try loading all 6 DeepEval evaluators
    print("Loading DeepEval evaluators...")
    deepeval_evaluators = []
    deepeval_names = [
        "deepeval/plan-quality",
        "deepeval/plan-adherence",
        "deepeval/tool-correctness",
        "deepeval/argument-correctness",
        "deepeval/task-completion",
        "deepeval/step-efficiency",
    ]

    for name in deepeval_names:
        try:
            ev = builtin(name, threshold=0.5)
            deepeval_evaluators.append(ev)
            print(f"  Loaded: {name}")
        except Exception as e:
            print(f"  Skipped {name}: {e}")

    if not deepeval_evaluators:
        print("\nNo DeepEval evaluators available.")
        print("Install with: pip install amp-evaluation[deepeval]")
        print("Set OPENAI_API_KEY environment variable.")
        sys.exit(0)

    print(f"\nLoaded {len(deepeval_evaluators)} DeepEval evaluators")

    # 2. Load dataset (DeepEval evaluators require ground truth / task data)
    dataset = load_dataset_from_json(str(DATA_DIR / "sample_dataset.json"))
    print(f"Loaded dataset: {dataset.name} ({len(dataset.tasks)} tasks)")

    # 3. Load traces
    traces = load_sample_traces()
    print(f"Loaded {len(traces)} traces")
    print()

    # 4. Assign task_ids for matching traces to dataset tasks
    #    In production, this matching happens automatically via OTEL baggage
    for i, trace in enumerate(traces):
        if i < len(dataset.tasks):
            trace.trace_id = dataset.tasks[i].task_id

    # 5. Run experiment with pre-loaded traces
    #    HttpAgentInvoker is required by Experiment.__init__ but not called
    #    when using pre-loaded traces
    invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat")
    experiment = Experiment(
        evaluators=deepeval_evaluators,
        invoker=invoker,
        dataset=dataset,
    )
    result = experiment.run(traces=traces, dataset=dataset)

    # 6. Print summary
    result.print_summary()


if __name__ == "__main__":
    main()
