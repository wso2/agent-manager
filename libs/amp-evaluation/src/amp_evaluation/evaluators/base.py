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
Base evaluator classes and interfaces.

Simplified architecture with single evaluate(context: EvalContext) interface.
All evaluators receive the same EvalContext and access what they need.
"""
from abc import ABC, abstractmethod
from typing import List, Optional, Literal, Callable, TYPE_CHECKING
import time
import logging

from ..models import EvalResult, CompositeScore, EvalContext

if TYPE_CHECKING:
    pass


logger = logging.getLogger(__name__)


class BaseEvaluator(ABC):
    """
    Abstract base class for all evaluators.

    Evaluators score specific aspects of agent performance.
    All evaluators implement a single evaluate(context) method and access
    whatever data they need from the EvalContext.
    
    Evaluator Types:
        - code: Deterministic logic-based evaluation (default)
        - model: LLM-as-judge evaluation
        - human: Human evaluation (async)
    
    Example:
        class ExactMatchEvaluator(BaseEvaluator):
            def evaluate(self, ctx: EvalContext) -> EvalResult:
                matches = ctx.trace.output == ctx.expected_output
                return self._create_result(
                    target_id=ctx.trace.trace_id,
                    target_type="trace",
                    score=1.0 if matches else 0.0,
                    explanation=f"Exact match: {matches}"
                )
    """

    def __init__(self):
        self.evaluator_type: Literal["code", "model", "human"] = "code"
        self.version: Optional[str] = "1.0"
        self._name: Optional[str] = None
        self._aggregations: Optional[List] = None

    @property
    def name(self) -> str:
        """Evaluator name. Defaults to class name."""
        return self._name or self.__class__.__name__

    @name.setter
    def name(self, value: str):
        self._name = value

    @property
    def aggregations(self) -> Optional[List]:
        """Get configured aggregations for this evaluator."""
        return self._aggregations
    
    @aggregations.setter
    def aggregations(self, value: List):
        """Set aggregations for this evaluator."""
        self._aggregations = value

    @abstractmethod
    def evaluate(self, context: EvalContext) -> EvalResult:
        """
        Evaluate an agent's performance given the context.

        Args:
            context: EvalContext containing trace and optional ground truth

        Returns:
            EvalResult with score and explanation
        """
        pass

    def _create_result(
        self,
        target_id: str,
        target_type: str,
        score: float,
        passed: bool = None,
        explanation: str = "",
        reasoning_steps: List[str] = None,
        evidence: dict = None,
        details: dict = None
    ) -> EvalResult:
        """Helper to create EvalResult with consistent metadata."""
        if passed is None:
            passed = score >= 0.7

        return EvalResult(
            evaluator_name=self.name,
            target_id=target_id,
            target_type=target_type,
            score=score,
            passed=passed,
            explanation=explanation,
            reasoning_steps=reasoning_steps or [],
            evidence=evidence or {},
            details=details or {},
            evaluator_type=self.evaluator_type,
            evaluator_version=self.version
        )


class LLMAsJudgeEvaluator(BaseEvaluator):
    """
    Base class for LLM-as-judge evaluators.
    Uses an LLM to evaluate outputs for subjective criteria.
    """

    def __init__(
        self,
        model: str = "gpt-4",
        prompt_template: Optional[str] = None,
        criteria: Optional[str] = None
    ):
        super().__init__()
        self.evaluator_type = "model"
        self.model = model
        self.prompt_template = prompt_template or self._default_prompt_template()
        self.criteria = criteria or "quality, accuracy, and helpfulness"

    def _default_prompt_template(self) -> str:
        """Default evaluation prompt template."""
        return """You are an expert evaluator assessing AI agent outputs.

Task Input: {input}
Agent Output: {output}
{reference_section}
{criteria_section}

Please evaluate the agent's output on a scale of 0.0 to 1.0.
Provide your score and explanation in this format:
Score: <number between 0.0 and 1.0>
Explanation: <your reasoning>
"""

    @abstractmethod
    def call_llm(self, prompt: str) -> dict:
        """Call the LLM API. Must be implemented by subclasses."""
        pass

    def evaluate(self, context: EvalContext) -> EvalResult:
        """Evaluate using LLM-as-judge."""
        start_time = time.time()
        
        reference_section = ""
        if context.has_expected_output():
            reference_section = f"\nExpected Output: {context.expected_output}"
        
        criteria_section = f"\nEvaluation Criteria: {self.criteria}"
        if context.has_success_criteria():
            criteria_section = f"\nSuccess Criteria: {context.success_criteria}"

        prompt = self.prompt_template.format(
            input=context.trace.input,
            output=context.trace.output,
            reference_section=reference_section,
            criteria_section=criteria_section
        )

        llm_response = self.call_llm(prompt)
        
        result = self._create_result(
            target_id=context.trace.trace_id,
            target_type="trace",
            score=llm_response.get("score", 0.0),
            explanation=llm_response.get("explanation", ""),
            details={"model": self.model, "criteria": self.criteria}
        )
        result.evaluation_duration_ms = (time.time() - start_time) * 1000
        return result


class CompositeEvaluator:
    """Runs multiple evaluators and combines their scores."""

    def __init__(
        self,
        evaluators: List[BaseEvaluator],
        aggregation_method: Literal["weighted_average", "minimum", "all_pass", "majority"] = "weighted_average",
        weights: Optional[dict] = None,
        threshold: float = 0.7
    ):
        self.evaluators = evaluators
        self.aggregation_method = aggregation_method
        self.weights = weights or {}
        self.threshold = threshold

    def evaluate(self, context: EvalContext) -> CompositeScore:
        """Run all evaluators and combine scores."""
        component_scores = {}
        
        for evaluator in self.evaluators:
            try:
                result = evaluator.evaluate(context)
                component_scores[result.evaluator_name] = result
            except Exception as e:
                logger.warning(f"Evaluator {evaluator.name} failed: {e}")

        composite = CompositeScore(
            composite_id=f"composite_{context.trace.trace_id}",
            trial_id=context.trace.trace_id,
            component_scores=component_scores,
            aggregation_method=self.aggregation_method,
            weights=self.weights,
            threshold=self.threshold
        )

        composite.calculate()
        return composite


class FunctionEvaluator(BaseEvaluator):
    """Wraps a simple function as an evaluator."""

    def __init__(self, func: Callable[[EvalContext], any], name: Optional[str] = None):
        super().__init__()
        self.func = func
        self._name = name or func.__name__

    def evaluate(self, context: EvalContext) -> EvalResult:
        """Call the wrapped function."""
        start_time = time.time()
        
        result = self.func(context)

        if isinstance(result, EvalResult):
            eval_result = result
        elif isinstance(result, dict):
            eval_result = self._create_result(
                target_id=context.trace.trace_id,
                target_type="trace",
                score=result.get("score", 0.0),
                passed=result.get("passed"),
                explanation=result.get("explanation", ""),
                evidence=result.get("evidence"),
                details=result.get("details")
            )
        elif isinstance(result, (int, float)):
            eval_result = self._create_result(
                target_id=context.trace.trace_id,
                target_type="trace",
                score=float(result)
            )
        else:
            raise TypeError(f"Evaluator function must return EvalResult, dict, or float, got {type(result)}")
        
        eval_result.evaluation_duration_ms = (time.time() - start_time) * 1000
        return eval_result
