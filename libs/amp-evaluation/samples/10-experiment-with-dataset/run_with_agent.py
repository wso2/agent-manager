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
Run experiment with a real agent (requires running agent + trace service).

This script demonstrates the full experiment flow where the framework:
    1. Loads a dataset of tasks with ground truth
    2. Invokes a running agent for each task via HTTP
    3. Waits for traces to be collected by the trace service
    4. Fetches traces from the trace service
    5. Matches traces to tasks using OTEL baggage (task_id, trial_id)
    6. Evaluates each trace against its corresponding task

Prerequisites:
    1. Start your agent: e.g., python -m your_agent --port 8000
    2. Start the trace service (or configure AMP_TRACE_API_URL)
    3. Run this script
"""

import sys
from pathlib import Path

# Add sample dir to path for evaluator imports
sys.path.insert(0, str(Path(__file__).parent))

from amp_evaluation import (
    Experiment,
    HttpAgentInvoker,
    discover_evaluators,
    builtin,
    load_dataset_from_json,
)

import evaluators  # noqa: E402 -- local evaluators module

DATA_DIR = Path(__file__).parent.parent / "data"


def main():
    # 1. Load dataset
    dataset = load_dataset_from_json(str(DATA_DIR / "sample_dataset.json"))
    print(f"Loaded dataset: {dataset.name} ({len(dataset.tasks)} tasks)")

    # 2. Discover evaluators + add a built-in
    evals = discover_evaluators(evaluators)
    evals.append(builtin("latency", max_latency_ms=10000))
    print(f"Evaluators: {[e.name for e in evals]}")

    # 3. Configure agent invoker pointing to a real running agent
    invoker = HttpAgentInvoker(
        base_url="http://localhost:8000",
        endpoint="/chat",
    )

    # 4. Create and run experiment
    #    The experiment will:
    #    - Invoke the agent for each task in the dataset
    #    - Propagate task_id and trial_id via OTEL baggage
    #    - Wait for traces to be exported (trace_fetch_wait_seconds)
    #    - Fetch traces from the trace service
    #    - Match traces to tasks using baggage parameters
    #    - Evaluate each trace against its task
    experiment = Experiment(
        evaluators=evals,
        invoker=invoker,
        dataset=dataset,
        trials_per_task=1,
        trace_fetch_wait_seconds=30.0,
    )

    print("\nRunning experiment (this will invoke the agent for each task)...")
    result = experiment.run()

    # 5. Print detailed results (includes individual scores)
    print()
    result.print_summary(verbosity="detailed")


if __name__ == "__main__":
    main()
