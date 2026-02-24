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
Class-based evaluators at all three evaluation levels.

Demonstrates:
- Trace-level: ResponseCompleteness (called once per trace)
- Agent-level: AgentErrorCheck (called once per agent span)
- LLM-level: LLMOutputCheck (called once per LLM call)

Each evaluator subclasses BaseEvaluator and implements evaluate()
with a typed first parameter that determines its evaluation level.
"""

from amp_evaluation import BaseEvaluator, EvalResult
from amp_evaluation.trace import Trace, AgentTrace, LLMSpan


class ResponseCompleteness(BaseEvaluator):
    """Trace-level: checks that the response is complete."""

    name = "response-completeness"
    description = "Validates response completeness"
    tags = ["quality", "trace-level"]

    def evaluate(self, trace: Trace) -> EvalResult:
        output = trace.output or ""
        if not output:
            return EvalResult(score=0.0, passed=False, explanation="No output")
        if output.endswith((".", "!", "?", "```")):
            return EvalResult(score=1.0, passed=True, explanation="Response appears complete")
        return EvalResult(score=0.7, passed=True, explanation="Response may be truncated")


class AgentErrorCheck(BaseEvaluator):
    """Agent-level: checks for errors in agent execution."""

    name = "agent-error-check"
    description = "Checks if agent had errors"
    tags = ["reliability", "agent-level"]

    def evaluate(self, agent: AgentTrace) -> EvalResult:
        if agent.has_errors:
            return EvalResult(
                score=0.0,
                passed=False,
                explanation="Agent had errors",
                details={"error_count": len(agent.error_steps)},
            )
        return EvalResult(score=1.0, passed=True, explanation="No errors")


class LLMOutputCheck(BaseEvaluator):
    """LLM-level: checks individual LLM call outputs."""

    name = "llm-output-check"
    description = "Validates LLM outputs are non-empty"
    tags = ["quality", "llm-level"]

    def evaluate(self, llm: LLMSpan) -> EvalResult:
        has_response = bool(llm.response)
        has_tool_calls = bool(llm.tool_calls)
        if not has_response and not has_tool_calls:
            return EvalResult.skip("LLM produced no output and no tool calls")
        return EvalResult(score=1.0, explanation="LLM produced output")


# Instantiate for discovery
response_completeness = ResponseCompleteness()
agent_error_check = AgentErrorCheck()
llm_output_check = LLMOutputCheck()
