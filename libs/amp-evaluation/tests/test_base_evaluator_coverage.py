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
Tests for base evaluator coverage - aiming for 90%+.
"""

import pytest
from amp_evaluation.evaluators.base import BaseEvaluator, LLMAsJudgeEvaluator, FunctionEvaluator
from amp_evaluation.models import EvalResult, Observation, Task
from amp_evaluation.trace import Trajectory, TraceMetrics, TokenUsage


def create_test_observation():
    """Helper to create a test observation with a trajectory."""
    trajectory = Trajectory(
        trace_id="test-123",
        input="What is AI?",
        output="AI is artificial intelligence.",
        metrics=TraceMetrics(
            total_duration_ms=100.0,
            token_usage=TokenUsage(input_tokens=10, output_tokens=20, total_tokens=30),
        ),
        steps=[],
    )
    return Observation(trajectory=trajectory)


class TestBaseEvaluatorCall:
    """Test the __call__ method of BaseEvaluator."""

    def test_call_method_delegates_to_evaluate(self):
        """Test that __call__ properly delegates to evaluate()."""

        class SimpleEvaluator(BaseEvaluator):
            name = "simple-eval"

            def evaluate(self, observation, task=None):
                return EvalResult(score=0.8, explanation="Good")

        evaluator = SimpleEvaluator()
        observation = create_test_observation()

        # Call using __call__
        result = evaluator(observation)

        assert isinstance(result, EvalResult)
        assert result.score == 0.8
        assert result.explanation == "Good"


class TestLLMAsJudgeEvaluator:
    """Tests for LLMAsJudgeEvaluator to increase coverage."""

    def test_default_initialization(self):
        """Test LLM evaluator with default parameters."""

        class MockLLMEval(LLMAsJudgeEvaluator):
            def call_llm(self, prompt):
                return {"score": 0.9, "explanation": "Excellent"}

        evaluator = MockLLMEval()

        assert evaluator.model == "gpt-4"
        assert evaluator.criteria == "quality, accuracy, and helpfulness"
        assert "You are an expert evaluator" in evaluator.prompt_template

    def test_custom_initialization(self):
        """Test LLM evaluator with custom parameters."""

        class MockLLMEval(LLMAsJudgeEvaluator):
            def call_llm(self, prompt):
                return {"score": 0.7, "explanation": "Good"}

        custom_template = "Rate this: {input} -> {output}"
        evaluator = MockLLMEval(model="gpt-3.5-turbo", prompt_template=custom_template, criteria="clarity and brevity")

        assert evaluator.model == "gpt-3.5-turbo"
        assert evaluator.criteria == "clarity and brevity"
        assert evaluator.prompt_template == custom_template

    def test_default_prompt_builder_without_task(self):
        """Test default prompt builder without task."""

        class MockLLMEval(LLMAsJudgeEvaluator):
            def call_llm(self, prompt):
                assert "What is AI?" in prompt
                assert "AI is artificial intelligence" in prompt
                assert "Expected Output" not in prompt
                return {"score": 0.85, "explanation": "Clear"}

        evaluator = MockLLMEval()
        observation = create_test_observation()

        result = evaluator.evaluate(observation)
        assert result.score == 0.85

    def test_default_prompt_builder_with_task_expected_output(self):
        """Test default prompt builder with task expected output."""

        class MockLLMEval(LLMAsJudgeEvaluator):
            def call_llm(self, prompt):
                assert "Expected Output: correct answer" in prompt
                return {"score": 0.95, "explanation": "Perfect match"}

        evaluator = MockLLMEval()
        observation = create_test_observation()

        task = Task(
            task_id="task-1",
            name="AI Question",
            description="Test task",
            input="What is AI?",
            expected_output="correct answer",
        )

        result = evaluator.evaluate(observation, task)
        assert result.score == 0.95

    def test_default_prompt_builder_with_task_success_criteria(self):
        """Test default prompt builder with task success criteria."""

        class MockLLMEval(LLMAsJudgeEvaluator):
            def call_llm(self, prompt):
                assert "Success Criteria: Must be accurate" in prompt
                return {"score": 0.75, "explanation": "Meets criteria"}

        evaluator = MockLLMEval()
        observation = create_test_observation()

        task = Task(
            task_id="task-2",
            name="AI Question",
            description="Test task",
            input="What is AI?",
            success_criteria="Must be accurate",
        )

        result = evaluator.evaluate(observation, task)
        assert result.score == 0.75

    def test_custom_prompt_builder(self):
        """Test with custom prompt builder."""

        def custom_builder(observation, task):
            return {
                "input": observation.input.upper(),
                "output": observation.output.upper(),
                "reference_section": "",
                "criteria_section": "Custom criteria",
            }

        class MockLLMEval(LLMAsJudgeEvaluator):
            def call_llm(self, prompt):
                assert "WHAT IS AI?" in prompt
                assert "AI IS ARTIFICIAL INTELLIGENCE" in prompt
                return {"score": 0.6, "explanation": "Custom eval"}

        evaluator = MockLLMEval(prompt_builder=custom_builder)
        observation = create_test_observation()

        result = evaluator.evaluate(observation)
        assert result.score == 0.6

    def test_llm_response_with_details(self):
        """Test that LLM response details are preserved."""

        class MockLLMEval(LLMAsJudgeEvaluator):
            def call_llm(self, prompt):
                return {
                    "score": 0.88,
                    "explanation": "Well done",
                    "details": {"reasoning": "Clear and concise", "confidence": 0.9},
                }

        evaluator = MockLLMEval(model="gpt-4-turbo", criteria="excellence")
        observation = create_test_observation()

        result = evaluator.evaluate(observation)

        assert result.score == 0.88
        assert result.explanation == "Well done"
        assert result.details["model"] == "gpt-4-turbo"
        assert result.details["criteria"] == "excellence"
        assert result.details["reasoning"] == "Clear and concise"
        assert result.details["confidence"] == 0.9


class TestFunctionEvaluator:
    """Tests for FunctionEvaluator to increase coverage."""

    def test_function_returns_eval_result(self):
        """Test function that returns EvalResult."""

        def my_eval(observation, task=None):
            return EvalResult(score=0.7, explanation="Custom")

        evaluator = FunctionEvaluator(my_eval, name="test-eval")
        observation = create_test_observation()

        result = evaluator.evaluate(observation)

        assert isinstance(result, EvalResult)
        assert result.score == 0.7
        assert result.explanation == "Custom"

    def test_function_returns_dict_full(self):
        """Test function that returns dict with all fields."""

        def my_eval(observation, task=None):
            return {"score": 0.85, "passed": True, "explanation": "All good", "details": {"key": "value"}}

        evaluator = FunctionEvaluator(my_eval)
        observation = create_test_observation()

        result = evaluator.evaluate(observation)

        assert result.score == 0.85
        assert result.passed is True
        assert result.explanation == "All good"
        assert result.details == {"key": "value"}

    def test_function_returns_dict_minimal(self):
        """Test function that returns dict with only score."""

        def my_eval(observation, task=None):
            return {"score": 0.5}

        evaluator = FunctionEvaluator(my_eval)
        observation = create_test_observation()

        result = evaluator.evaluate(observation)

        assert result.score == 0.5
        assert result.passed is True  # Auto-calculated from score >= 0.5
        assert result.explanation == ""
        assert result.details is None

    def test_function_returns_float(self):
        """Test function that returns float."""

        def my_eval(observation, task=None):
            return 0.92

        evaluator = FunctionEvaluator(my_eval)
        observation = create_test_observation()

        result = evaluator.evaluate(observation)

        assert result.score == 0.92

    def test_function_returns_int(self):
        """Test function that returns int."""

        def my_eval(observation, task=None):
            return 1

        evaluator = FunctionEvaluator(my_eval)
        observation = create_test_observation()

        result = evaluator.evaluate(observation)

        assert result.score == 1.0

    def test_function_returns_invalid_type(self):
        """Test function that returns invalid type raises error."""

        def bad_eval(observation, task=None):
            return "invalid return type"

        evaluator = FunctionEvaluator(bad_eval)
        observation = create_test_observation()

        with pytest.raises(TypeError, match="must return EvalResult, dict, or float"):
            evaluator.evaluate(observation)
