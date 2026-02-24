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
Decorator-based evaluators using the @evaluator decorator.

Demonstrates:
- Creating evaluators with @evaluator(name, description, tags)
- Trace-level evaluation (type hint on first parameter)
- Returning EvalResult with score, explanation, and details
- Automatic discovery via discover_evaluators()

The @evaluator decorator wraps a plain function into a FunctionEvaluator
instance. The decorated variable IS the evaluator instance, not a function.
"""

from amp_evaluation import evaluator, EvalResult
from amp_evaluation.trace import Trace


@evaluator("has-output", description="Checks if agent produced output", tags=["basic"])
def has_output(trace: Trace) -> EvalResult:
    """Simplest possible evaluator -- just check if there's output."""
    if trace.output:
        return EvalResult(score=1.0, explanation="Output present")
    return EvalResult(score=0.0, explanation="No output")


@evaluator("tool-usage", description="Checks if tools were called", tags=["tool-use"])
def tool_usage(trace: Trace) -> EvalResult:
    """Check tool usage in the trace."""
    tools = trace.get_tool_calls()
    if not tools:
        return EvalResult(score=0.5, explanation="No tools called")
    tool_names = [t.name for t in tools]
    return EvalResult(
        score=1.0,
        explanation=f"Called {len(tools)} tools: {', '.join(tool_names)}",
    )


@evaluator("error-free", description="Checks trace has no errors", tags=["reliability"])
def error_free(trace: Trace) -> EvalResult:
    """Check for errors in the trace."""
    error_summary = trace.get_error_summary()
    error_count = error_summary.get("total", 0)
    if error_count > 0:
        return EvalResult(score=0.0, explanation=f"{error_count} errors found")
    return EvalResult(score=1.0, explanation="No errors")
