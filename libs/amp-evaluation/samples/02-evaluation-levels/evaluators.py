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
Three evaluators demonstrating evaluation level auto-detection.

The first parameter's type hint controls the evaluation level:
  - Trace      -> trace-level  (called once per trace)
  - AgentTrace -> agent-level  (called once per agent span)
  - LLMSpan    -> llm-level    (called once per LLM call)
"""

from amp_evaluation.evaluators import BaseEvaluator
from amp_evaluation.models import EvalResult
from amp_evaluation.trace import Trace, AgentTrace, LLMSpan


class TraceLevelCheck(BaseEvaluator):
    name = "trace-level-check"
    description = "Trace-level: called once per trace"

    def evaluate(self, trace: Trace) -> EvalResult:
        return EvalResult(
            score=1.0,
            explanation=f"Trace has {len(trace.spans)} spans",
        )


class AgentLevelCheck(BaseEvaluator):
    name = "agent-level-check"
    description = "Agent-level: called per agent span"

    def evaluate(self, agent: AgentTrace) -> EvalResult:
        return EvalResult(
            score=1.0,
            explanation=f"Agent '{agent.agent_name}' has {len(agent.steps)} steps",
        )


class LLMLevelCheck(BaseEvaluator):
    name = "llm-level-check"
    description = "LLM-level: called per LLM call"

    def evaluate(self, llm: LLMSpan) -> EvalResult:
        return EvalResult(
            score=1.0,
            explanation=f"LLM model: {llm.model}, response length: {len(llm.response or '')}",
        )


# Instantiate (required for discover_evaluators)
trace_level = TraceLevelCheck()
agent_level = AgentLevelCheck()
llm_level = LLMLevelCheck()
