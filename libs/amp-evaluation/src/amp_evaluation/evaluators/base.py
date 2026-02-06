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

Two-parameter architecture: evaluate(observation, task)
- observation: What we observed (always available)
- task: What we expected (only for experiments)
"""

from abc import ABC, abstractmethod
from typing import List, Optional, Callable, TYPE_CHECKING
import logging

from ..models import EvalResult, Observation, Task

if TYPE_CHECKING:
    pass


logger = logging.getLogger(__name__)


class BaseEvaluator(ABC):
    """
    Abstract base class for all evaluators.

    Evaluators score specific aspects of agent performance using a two-parameter interface:
    - observation: What the agent did (always available)
    - task: What it should have done (only for experiments with datasets)

    The runner automatically enriches EvalResult into EvaluatorScore
    with metadata (trace ID, timestamp, task ID, trial ID).

    Class Attributes for Metadata:
        name: Unique evaluator name (defaults to class name)
        description: Human-readable description of what the evaluator does
        tags: List of tags for categorization (e.g., ["quality", "rag", "deepeval"])
        version: Evaluator version string
        aggregations: Default aggregations to compute for this evaluator

    Example (Success with score):
        class LatencyEvaluator(BaseEvaluator):
            name = "latency"
            description = "Checks if response latency is within acceptable limits"
            tags = ["performance", "sla"]
            version = "1.0"

            def __init__(self, max_latency_ms: float = 5000):
                super().__init__()
                self.max_latency = max_latency_ms

            def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
                latency = observation.metrics.total_duration_ms
                passed = latency <= self.max_latency
                return EvalResult(
                    score=1.0 if passed else 0.0,
                    explanation=f"Latency: {latency}ms"
                )

    Example (Error when cannot evaluate):
        class ExactMatchEvaluator(BaseEvaluator):
            name = "exact-match"
            description = "Checks if output exactly matches expected output"
            tags = ["accuracy"]

            def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
                if not task or not task.expected_output:
                    return EvalResult.skip("Requires task with expected_output")

                matches = observation.output == task.expected_output
                return EvalResult(
                    score=1.0 if matches else 0.0,
                    explanation=f"Exact match: {matches}"
                )
    """

    # Class-level metadata attributes (can be overridden by subclasses or instances)
    name: str = ""  # Defaults to class name if not set
    description: str = ""
    tags: List[str] = ()  # Immutable default; subclasses should override with a list
    version: str = "1.0"
    evaluator_type: str = "trace"

    def __init__(self):
        # Set default name to class name if not already set
        if not self.name:
            self.name = self.__class__.__name__

        # Ensure tags is a mutable list per instance
        if not isinstance(self.tags, list):
            self.tags = list(self.tags)

        self._aggregations: Optional[List] = None

        # Check if class has default aggregations set via decorator
        if hasattr(self.__class__, "_default_aggregations") and self.__class__._default_aggregations:
            self._aggregations = self.__class__._default_aggregations

    @property
    def aggregations(self) -> Optional[List]:
        """Get configured aggregations for this evaluator."""
        return self._aggregations

    @aggregations.setter
    def aggregations(self, value: List):
        """Set aggregations for this evaluator."""
        self._aggregations = value

    def get_metadata(self) -> dict:
        """
        Get evaluator metadata.

        Returns metadata from instance attributes if set (e.g., for FunctionEvaluator),
        otherwise from class attributes.
        """
        return {
            "name": self.name,
            "description": getattr(self, "description", ""),
            "tags": list(getattr(self, "tags", [])),  # Copy to avoid mutation
            "version": getattr(self, "version", "1.0"),
            "evaluator_type": getattr(self, "evaluator_type", "trace"),
        }

    @abstractmethod
    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        """
        Evaluate an agent's performance.

        Args:
            observation: What we observed from the agent's execution (always available)
            task: What we expected (ground truth, constraints) - only for experiments

        Returns:
            EvalResult with score and explanation (metadata added automatically by runner)
        """
        pass

    def __call__(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        """
        Execute the evaluator.

        Simply calls evaluate() - the runner will handle enriching with metadata.
        """
        return self.evaluate(observation, task)


class LLMAsJudgeEvaluator(BaseEvaluator):
    """
    Base class for LLM-as-judge evaluators.
    Uses an LLM to evaluate outputs for subjective criteria.

    Supports flexible prompt templates with flat variable access (Python str.format()).
    Use a custom prompt_builder to extract and flatten data from observation/task.

    Example with custom prompt:
        class CustomJudge(LLMAsJudgeEvaluator):
            def __init__(self):
                super().__init__(
                    model="gpt-4o-mini",
                    prompt_template='''
                        Evaluate if the agent used tools appropriately.

                        Query: {query}
                        Tools Used: {tools_used}
                        Response: {response}

                        Score (0.0-1.0):
                        Explanation:
                    ''',
                    prompt_builder=self.build_prompt
                )

            def build_prompt(self, observation, task):
                # Extract and flatten data for template variables
                tools_used = [s.name for s in observation.trajectory.tool_spans]
                return {
                    "query": observation.input,
                    "response": observation.output,
                    "tools_used": ", ".join(tools_used) if tools_used else "None"
                }

            def call_llm(self, prompt):
                # Your LLM API call
                pass
    """

    def __init__(
        self,
        model: str = "gpt-4",
        prompt_template: Optional[str] = None,
        criteria: Optional[str] = None,
        prompt_builder: Optional[Callable] = None,
    ):
        """
        Initialize LLM-as-judge evaluator.

        Args:
            model: LLM model to use
            prompt_template: Template string with {variable} placeholders
            criteria: Default evaluation criteria (used if no prompt_template)
            prompt_builder: Optional function(observation, task) -> dict of template variables
                           Allows custom logic to prepare prompt context
        """
        super().__init__()
        self.model = model
        self.prompt_template = prompt_template or self._default_prompt_template()
        self.criteria = criteria or "quality, accuracy, and helpfulness"
        self.prompt_builder = prompt_builder or self._default_prompt_builder

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

    def _default_prompt_builder(self, observation: Observation, task: Optional[Task] = None) -> dict:
        """
        Build template variables for the default prompt.

        Returns a dict of flat variables for str.format(). Python's str.format() doesn't
        support nested attribute access (like {observation.input}), so extract and flatten
        all needed values.

        Override this or provide custom prompt_builder to customize variables.
        """
        reference_section = ""
        if task and task.expected_output:
            reference_section = f"\nExpected Output: {task.expected_output}"

        criteria_section = f"\nEvaluation Criteria: {self.criteria}"
        if task and task.success_criteria:
            criteria_section = f"\nSuccess Criteria: {task.success_criteria}"

        return {
            "input": observation.input,
            "output": observation.output,
            "reference_section": reference_section,
            "criteria_section": criteria_section,
        }

    @abstractmethod
    def call_llm(self, prompt: str) -> dict:
        """
        Call the LLM API. Must be implemented by subclasses.

        Args:
            prompt: The formatted prompt string

        Returns:
            dict with keys:
                - score: float between 0.0 and 1.0
                - explanation: str with reasoning
                - (optional) other details
        """
        pass

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        """Evaluate using LLM-as-judge."""
        # Build template variables using custom or default builder
        template_vars = self.prompt_builder(observation, task)

        # Format the prompt with variables
        prompt = self.prompt_template.format(**template_vars)

        # Call LLM
        llm_response = self.call_llm(prompt)

        return EvalResult(
            score=llm_response.get("score", 0.0),
            explanation=llm_response.get("explanation", ""),
            details={"model": self.model, "criteria": self.criteria, **llm_response.get("details", {})},
        )


class FunctionEvaluator(BaseEvaluator):
    """Wraps a simple function as an evaluator."""

    def __init__(self, func: Callable[[Observation, Optional[Task]], any], name: Optional[str] = None):
        super().__init__()
        self.func = func
        self.name = name or func.__name__

    def evaluate(self, observation: Observation, task: Optional[Task] = None) -> EvalResult:
        """Call the wrapped function."""
        result = self.func(observation, task)

        # Handle different return types from user functions
        if isinstance(result, EvalResult):
            return result
        elif isinstance(result, dict):
            return EvalResult(
                score=result.get("score", 0.0),
                passed=result.get("passed"),
                explanation=result.get("explanation", ""),
                details=result.get("details"),
            )
        elif isinstance(result, (int, float)):
            return EvalResult(score=float(result))
        else:
            raise TypeError(f"Evaluator function must return EvalResult, dict, or float, got {type(result)}")
