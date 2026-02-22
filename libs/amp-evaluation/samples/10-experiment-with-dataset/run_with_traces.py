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
Run experiment using pre-loaded traces (no running agent needed).

Uses Experiment.run(traces=..., dataset=...) to evaluate pre-collected
traces against a ground-truth dataset. This is useful for offline
evaluation of previously recorded agent interactions.
"""

import sys
from pathlib import Path

# Add sample dir to path for evaluator imports
sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import (
    Experiment,
    HttpAgentInvoker,
    discover_evaluators,
    load_dataset_from_json,
    load_dataset_from_csv,
)
from amp_evaluation.trace import TraceLoader, parse_traces_for_evaluation

import evaluators  # noqa: E402 -- local evaluators module

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
    # 1. Load dataset (JSON format)
    dataset = load_dataset_from_json(str(DATA_DIR / "sample_dataset.json"))
    print(f"Loaded dataset: {dataset.name} ({len(dataset.tasks)} tasks)")
    for task in dataset.tasks[:3]:
        inp = str(task.input)
        print(f"  Task {task.task_id}: {inp[:60]}...")

    # 2. Load traces
    traces = load_sample_traces()
    print(f"\nLoaded {len(traces)} traces")

    # 3. Assign dataset task_ids to traces for task-trace matching
    #    In production, this matching happens automatically via OTEL baggage
    for i, trace in enumerate(traces):
        if i < len(dataset.tasks):
            trace.trace_id = dataset.tasks[i].task_id

    # 4. Discover custom evaluators from the evaluators module
    evals = discover_evaluators(evaluators)
    print(f"\nDiscovered evaluators: {[e.name for e in evals]}")
    for ev in evals:
        modes = [m.value for m in ev._supported_eval_modes]
        print(f"  {ev.name:20s} modes={modes}")

    # 5. Run experiment with pre-loaded traces
    #    HttpAgentInvoker is required by Experiment.__init__ but not called
    #    when using pre-loaded traces
    invoker = HttpAgentInvoker(base_url="http://localhost:8000", endpoint="/chat")
    experiment = Experiment(
        evaluators=evals,
        invoker=invoker,
        dataset=dataset,
    )
    result = experiment.run(traces=traces, dataset=dataset)

    # 6. Print summary
    print("\n" + result.summary())

    # 7. Also demonstrate CSV dataset loading
    print("\n" + "=" * 60)
    print("CSV Dataset Loading")
    print("=" * 60)
    csv_dataset = load_dataset_from_csv(
        str(DATA_DIR / "simple_dataset.csv"),
        name="CSV Dataset",
    )
    print(f"Loaded CSV dataset: {csv_dataset.name} ({len(csv_dataset.tasks)} tasks)")
    for task in csv_dataset.tasks[:3]:
        inp = str(task.input)
        print(f"  Task {task.task_id}: {inp[:60]}...")


if __name__ == "__main__":
    main()
