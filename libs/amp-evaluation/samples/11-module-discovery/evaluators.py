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
A module with various evaluator types for demonstrating discover_evaluators().

discover_evaluators() scans a module and finds all BaseEvaluator INSTANCES.
This means:
  - Class-based evaluators must be instantiated at module level
  - Decorator-based evaluators are instances by default (@evaluator returns one)
  - Built-in evaluators created via builtin() are also instances
  - Bare classes (not instantiated) are NOT discovered
  - Non-evaluator objects are NOT discovered
"""

from amp_evaluation import BaseEvaluator, evaluator, EvalResult, builtin
from amp_evaluation.trace import Trace


# ============================================================================
# 1. Class-based evaluator (instantiated at module level)
# ============================================================================


class OutputLengthCheck(BaseEvaluator):
    """Checks that the agent produced output of reasonable length."""

    name = "output-length"
    description = "Checks output length"
    tags = ["custom", "rule-based"]

    def evaluate(self, trace: Trace) -> EvalResult:
        output = trace.output or ""
        length = len(output)
        if length == 0:
            return EvalResult(score=0.0, explanation="No output produced")
        if length < 10:
            return EvalResult(score=0.5, explanation=f"Very short output ({length} chars)")
        return EvalResult(score=1.0, explanation=f"Output OK ({length} chars)")


# DISCOVERED: instantiated at module level
output_length = OutputLengthCheck()


# ============================================================================
# 2. Decorator-based evaluator (instance by default)
# ============================================================================


@evaluator("has-tools", description="Checks whether the agent used any tools")
def has_tools(trace: Trace) -> EvalResult:
    """Check if the trace includes tool calls."""
    tools = trace.get_tool_calls()
    if tools:
        tool_names = [t.name for t in tools]
        return EvalResult(
            score=1.0,
            explanation=f"Used {len(tools)} tools: {', '.join(tool_names)}",
        )
    return EvalResult(score=0.5, explanation="No tools used")


# DISCOVERED: @evaluator returns a FunctionEvaluator instance


# ============================================================================
# 3. Built-in evaluator (instance via factory)
# ============================================================================

# DISCOVERED: builtin() returns a BaseEvaluator instance
latency = builtin("latency", max_latency_ms=10000)


# ============================================================================
# 4. NOT DISCOVERED: bare class (not instantiated)
# ============================================================================


class NotDiscovered(BaseEvaluator):
    """This class is NOT discovered because it is not instantiated."""

    name = "not-discovered"
    description = "Will not be found by discover_evaluators"

    def evaluate(self, trace: Trace) -> EvalResult:
        return EvalResult(score=1.0, explanation="This should never run via discovery")


# NOT DISCOVERED: class is defined but never instantiated at module level


# ============================================================================
# 5. NOT DISCOVERED: non-evaluator objects
# ============================================================================

helper_string = "this is not an evaluator"
helper_number = 42
helper_list = ["also", "not", "an", "evaluator"]
