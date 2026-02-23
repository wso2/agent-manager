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
Three evaluators demonstrating evaluation mode auto-detection.

The task parameter determines when an evaluator can run:
  - No task parameter        -> runs in both monitor and experiment modes
  - task: Task (required)    -> experiment only (needs ground truth)
  - task: Optional[Task]     -> runs in both modes, adapts when task is available
"""

from typing import Optional

from amp_evaluation import evaluator, EvalResult
from amp_evaluation.trace import Trace
from amp_evaluation.dataset import Task


@evaluator("monitor-friendly", description="Works in both monitor and experiment modes")
def monitor_friendly(trace: Trace) -> EvalResult:
    """No task parameter - runs in both modes."""
    return EvalResult(
        score=1.0 if trace.output else 0.0,
        explanation="Has output" if trace.output else "No output",
    )


@evaluator("experiment-only", description="Only runs in experiment mode")
def experiment_only(trace: Trace, task: Task) -> EvalResult:
    """Required task parameter - experiment only."""
    if not task.expected_output:
        return EvalResult(score=0.0, explanation="No expected output to compare")
    match = task.expected_output.lower() in (trace.output or "").lower()
    return EvalResult(
        score=1.0 if match else 0.0,
        explanation="Match" if match else "No match",
    )


@evaluator("adaptive", description="Adapts based on task availability")
def adaptive(trace: Trace, task: Optional[Task] = None) -> EvalResult:
    """Optional task parameter - runs in both modes, uses task when available."""
    if task and task.expected_output:
        match = task.expected_output.lower() in (trace.output or "").lower()
        return EvalResult(
            score=1.0 if match else 0.0,
            explanation="With ground truth",
        )
    return EvalResult(
        score=1.0 if trace.output else 0.0,
        explanation="Without ground truth",
    )
