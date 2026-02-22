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
Demonstrates the Param descriptor for configurable evaluators.

Shows two patterns:
  1. Class-based: Param as class attributes on a BaseEvaluator subclass
  2. Decorator-based: Param as function parameter defaults with @evaluator

Both support with_config() for creating configured variants and .info for
inspecting the config schema.
"""

from amp_evaluation import BaseEvaluator, evaluator, EvalResult, Param
from amp_evaluation.trace import Trace


# ============================================================================
# Pattern 1: Class-based evaluator with Param descriptors
# ============================================================================


class ResponseLengthChecker(BaseEvaluator):
    name = "response-length"
    description = "Checks response length against configurable bounds"

    min_length: int = Param(default=10, description="Minimum response length", min=0)
    max_length: int = Param(default=5000, description="Maximum response length", min=1)

    def evaluate(self, trace: Trace) -> EvalResult:
        length = len(trace.output or "")
        if length < self.min_length:
            return EvalResult(
                score=0.0,
                explanation=f"Too short: {length} < {self.min_length}",
            )
        if length > self.max_length:
            return EvalResult(
                score=0.5,
                explanation=f"Too long: {length} > {self.max_length}",
            )
        return EvalResult(
            score=1.0,
            explanation=f"Length OK: {length}",
        )


# Default config
response_length_default = ResponseLengthChecker()

# Strict config — different Param values
response_length_strict = ResponseLengthChecker(min_length=50, max_length=2000)


# ============================================================================
# Pattern 2: Decorator-based evaluator with Param defaults
# ============================================================================


@evaluator("latency-threshold", description="Checks latency against threshold")
def latency_threshold(
    trace: Trace,
    max_ms: float = Param(default=10000, description="Max latency in ms", min=0),
) -> EvalResult:
    latency = trace.metrics.total_duration_ms if trace.metrics else 0
    passed = latency <= max_ms
    return EvalResult(
        score=1.0 if passed else 0.0,
        explanation=f"Latency: {latency:.0f}ms (max: {max_ms:.0f}ms)",
    )


# Create configured variant using with_config
latency_strict = latency_threshold.with_config(max_ms=3000)
