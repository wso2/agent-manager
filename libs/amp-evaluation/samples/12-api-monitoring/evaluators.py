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
Simple evaluators for production monitoring.

These evaluators work without ground truth (monitor mode), making them
suitable for continuous evaluation of live production traces.
"""

from amp_evaluation import evaluator, EvalResult
from amp_evaluation.trace import Trace


@evaluator("response-quality", description="Basic response quality check")
def response_quality(trace: Trace) -> EvalResult:
    """Check that the agent produced a meaningful response."""
    output = trace.output or ""
    if not output:
        return EvalResult(score=0.0, explanation="No response produced")
    if len(output) < 20:
        return EvalResult(score=0.5, explanation=f"Short response ({len(output)} chars)")
    return EvalResult(score=1.0, explanation=f"Response OK ({len(output)} chars)")


@evaluator("error-free", description="Checks that the trace has no errors")
def error_free(trace: Trace) -> EvalResult:
    """Check that no errors occurred during execution."""
    error_count = trace.metrics.error_count
    if error_count > 0:
        return EvalResult(
            score=0.0,
            explanation=f"{error_count} error(s) detected in trace",
            details={"error_count": error_count},
        )
    return EvalResult(score=1.0, explanation="No errors detected")
