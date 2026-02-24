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
Experiment mode: ALL evaluators run, including experiment-only ones.

Uses pre-loaded traces with a dataset so no live agent invocation is needed.
"""

import sys
from pathlib import Path

# Add sample dir to path for evaluator imports
sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import Experiment, HttpAgentInvoker, discover_evaluators, load_dataset_from_json
from amp_evaluation.trace import TraceLoader, parse_traces_for_evaluation

import evaluators  # noqa: E402 — local evaluators module

DATA_DIR = Path(__file__).parent.parent / "data"


def load_sample_traces():
    loader = TraceLoader(file_path=str(DATA_DIR / "sample_traces.json"))
    otel_traces = loader.load_traces()
    return parse_traces_for_evaluation(otel_traces)


def main():
    # 1. Load traces and dataset
    traces = load_sample_traces()
    dataset = load_dataset_from_json(str(DATA_DIR / "sample_dataset.json"))
    print(f"Loaded {len(traces)} traces and {len(dataset.tasks)} tasks\n")

    # 2. Discover all evaluators
    evals = discover_evaluators(evaluators)
    print(f"Discovered evaluators: {[e.name for e in evals]}")
    for ev in evals:
        modes = [m.value for m in ev.info.modes]
        print(f"  {ev.name:25s} modes={modes}")
    print()

    # 3. HttpAgentInvoker is required by Experiment.__init__ but not called
    #    when traces are pre-loaded
    invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat")
    experiment = Experiment(evaluators=evals, invoker=invoker, dataset=dataset)

    # 4. Assign dataset task_ids to traces for matching
    for i, trace in enumerate(traces):
        if i < len(dataset.tasks):
            trace.trace_id = dataset.tasks[i].task_id

    # 5. Run experiment with pre-loaded traces
    print("Running Experiment mode...")
    result = experiment.run(traces=traces, dataset=dataset)

    # 6. Print results — ALL evaluators should appear
    result.print_summary(verbosity="compact")

    print("\nNote: ALL evaluators ran, including 'experiment-only'.")


if __name__ == "__main__":
    main()
