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
LLM-as-judge evaluators using both class-based and decorator approaches.

Demonstrates five patterns:
1. Class-based LLMAsJudgeEvaluator at trace level
2. Class-based LLMAsJudgeEvaluator at agent level
3. @llm_judge decorator (simple, no config)
4. @llm_judge decorator with model and criteria config
5. Custom LLM client override via _call_llm_with_retry()

LLM-as-judge evaluators write a build_prompt() method (or a prompt-building
function for the decorator). The framework handles:
- Appending output format instructions
- Calling the LLM via LiteLLM
- Validating the response with Pydantic (JudgeOutput model)
- Retrying on invalid output with error context
"""

from typing import Optional

from amp_evaluation import LLMAsJudgeEvaluator, llm_judge, EvalResult
from amp_evaluation.trace import Trace, AgentTrace, LLMSpan
from amp_evaluation.dataset import Task


# ---------------------------------------------------------------------------
# 1. Class-based -- Trace-level judge
# ---------------------------------------------------------------------------

class ResponseQualityJudge(LLMAsJudgeEvaluator):
    """Uses an LLM to judge response quality."""

    name = "response-quality-judge"
    model = "gpt-4o-mini"  # LiteLLM model identifier
    criteria = "helpfulness, accuracy, and completeness"

    def build_prompt(self, trace: Trace, task: Optional[Task] = None) -> str:
        prompt = f"""Evaluate this AI agent response.

User Input: {trace.input}
Agent Response: {trace.output}

Criteria: {self.criteria}"""
        if task and task.expected_output:
            prompt += f"\n\nExpected Output: {task.expected_output}"
        return prompt


# ---------------------------------------------------------------------------
# 2. Class-based -- Agent-level judge
# ---------------------------------------------------------------------------

class AgentEfficiencyJudge(LLMAsJudgeEvaluator):
    """Uses an LLM to judge agent execution efficiency."""

    name = "agent-efficiency-judge"
    model = "gpt-4o-mini"

    def build_prompt(self, agent: AgentTrace) -> str:
        return f"""Evaluate this agent's efficiency.
Input: {agent.input}
Output: {agent.output}
Steps taken: {len(agent.steps)}
Tools available: {', '.join(agent.available_tools)}
Had errors: {agent.has_errors}"""


# ---------------------------------------------------------------------------
# 3. @llm_judge decorator -- simple
# ---------------------------------------------------------------------------

@llm_judge
def grounding_judge(trace: Trace) -> str:
    """Check if the response is grounded in tool results."""
    tools = trace.get_tool_calls()
    tool_info = "\n".join(f"- {t.name}: {t.result}" for t in tools) if tools else "No tools called."
    return f"""Is this response grounded in the tool results?

Response: {trace.output}
Tool Results:
{tool_info}"""


# ---------------------------------------------------------------------------
# 4. @llm_judge decorator -- with config
# ---------------------------------------------------------------------------

@llm_judge(model="gpt-4o", criteria="factual accuracy")
def accuracy_judge(trace: Trace, task: Optional[Task] = None) -> str:
    """Evaluate factual accuracy of the response."""
    prompt = f"Evaluate factual accuracy.\nResponse: {trace.output}"
    if task and task.expected_output:
        prompt += f"\nExpected: {task.expected_output}"
    return prompt


# ---------------------------------------------------------------------------
# 5. Custom LLM client override
# ---------------------------------------------------------------------------

class CustomClientJudge(LLMAsJudgeEvaluator):
    """Demonstrates overriding _call_llm_with_retry() to use a custom LLM client."""

    name = "custom-client-judge"

    def build_prompt(self, trace: Trace) -> str:
        return f"Evaluate: {trace.input} -> {trace.output}"

    def _call_llm_with_retry(self, prompt: str) -> EvalResult:
        """Override to use your own LLM client instead of LiteLLM.

        Example with OpenAI client:
            import openai
            client = openai.OpenAI()
            resp = client.chat.completions.create(
                model="gpt-4o",
                messages=[{"role": "user", "content": prompt}],
                response_format={"type": "json_object"},
            )
            result, error = self._parse_and_validate(resp.choices[0].message.content)
            return result or EvalResult(score=0.0, explanation=error)
        """
        return EvalResult(score=0.5, explanation="Custom client placeholder")


# Instantiate class-based evaluators for discovery
response_quality_judge = ResponseQualityJudge()
agent_efficiency_judge = AgentEfficiencyJudge()
custom_client_judge = CustomClientJudge()
