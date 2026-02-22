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
Evaluators that use ground truth from a dataset.

Demonstrates three patterns:
  1. output_match:   experiment-only (requires Task with expected_output)
  2. criteria_check: experiment-only (requires Task with success_criteria)
  3. adaptive_check: both modes (uses ground truth when available, basic check otherwise)
"""

from typing import Optional

from amp_evaluation import evaluator, EvalResult
from amp_evaluation.trace import Trace
from amp_evaluation.dataset import Task


@evaluator("output-match", description="Checks if output contains expected content")
def output_match(trace: Trace, task: Task) -> EvalResult:
    """Experiment-only: requires ground truth expected_output."""
    expected = (task.expected_output or "").strip().lower()
    actual = (trace.output or "").strip().lower()

    if not expected:
        return EvalResult.skip("No expected output defined")

    if expected in actual:
        return EvalResult(
            score=1.0,
            explanation="Output contains expected content",
        )

    return EvalResult(
        score=0.0,
        explanation="Expected content not found in output",
        details={"expected_preview": expected[:100], "actual_preview": actual[:100]},
    )


@evaluator("criteria-check", description="Checks success criteria against output")
def criteria_check(trace: Trace, task: Task) -> EvalResult:
    """Experiment-only: checks task.success_criteria against output."""
    criteria = task.success_criteria or ""
    if not criteria:
        return EvalResult.skip("No success criteria defined")

    output = (trace.output or "").lower()

    # Simple keyword-based criteria check
    criteria_words = [w.strip().lower() for w in criteria.split(",") if w.strip()]
    if not criteria_words:
        return EvalResult.skip("No parseable criteria found")

    met = sum(1 for c in criteria_words if c in output)
    score = met / len(criteria_words)

    return EvalResult(
        score=score,
        explanation=f"{met}/{len(criteria_words)} criteria keywords found in output",
        details={"criteria_words": criteria_words, "met_count": met},
    )


@evaluator("adaptive-check", description="Works in both monitor and experiment modes")
def adaptive_check(trace: Trace, task: Optional[Task] = None) -> EvalResult:
    """Both modes: uses ground truth when available, basic output check otherwise."""
    if task and task.expected_output:
        expected = task.expected_output.lower()
        actual = (trace.output or "").lower()
        match = expected in actual
        return EvalResult(
            score=1.0 if match else 0.0,
            explanation="Checked against ground truth" if match else "Output does not match ground truth",
        )

    # Monitor mode: basic output presence check
    has_output = bool(trace.output and len(trace.output.strip()) > 0)
    return EvalResult(
        score=1.0 if has_output else 0.0,
        explanation="Output present" if has_output else "No output produced",
    )
