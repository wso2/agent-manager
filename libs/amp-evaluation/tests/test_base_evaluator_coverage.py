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
from typing import Optional
from amp_evaluation.evaluators.base import BaseEvaluator, LLMAsJudgeEvaluator, FunctionEvaluator
from amp_evaluation.evaluators.config import EvalMode
from amp_evaluation.models import EvalResult
from amp_evaluation.dataset import Task
from amp_evaluation.trace import Trace, TraceMetrics, TokenUsage


def create_test_trajectory():
    """Helper to create a test trajectory."""
    trajectory = Trace(
        trace_id="test-123",
        input="What is AI?",
        output="AI is artificial intelligence.",
        metrics=TraceMetrics(
            total_duration_ms=100.0,
            token_usage=TokenUsage(input_tokens=10, output_tokens=20, total_tokens=30),
        ),
        spans=[],
    )
    return trajectory


class TestBaseEvaluatorCall:
    """Test the __call__ method of BaseEvaluator."""

    def test_call_method_delegates_to_evaluate(self):
        """Test that __call__ properly delegates to run() which calls evaluate()."""

        class SimpleEvaluator(BaseEvaluator):
            name = "simple-eval"

            def evaluate(self, trace: Trace, task=None):
                return EvalResult(score=0.8, explanation="Good")

        evaluator = SimpleEvaluator()
        observation = create_test_trajectory()

        # Call using __call__ (delegates to run())
        results = evaluator(observation)

        assert isinstance(results, list)
        assert isinstance(results[0], EvalResult)
        assert results[0].score == 0.8
        assert results[0].explanation == "Good"


class TestLLMAsJudgeEvaluator:
    """Tests for the redesigned LLMAsJudgeEvaluator with build_prompt()."""

    def test_default_initialization(self):
        """Test LLM evaluator with default Param values."""
        evaluator = LLMAsJudgeEvaluator()

        assert evaluator.model == "gpt-4o-mini"
        assert evaluator.criteria == "quality, accuracy, and helpfulness"
        assert evaluator.temperature == 0.0
        assert evaluator.max_tokens == 1024
        assert evaluator.threshold == 0.5
        assert evaluator.max_retries == 2

    def test_custom_initialization(self):
        """Test LLM evaluator with custom Param overrides."""
        evaluator = LLMAsJudgeEvaluator(
            model="gpt-4o", criteria="accuracy", temperature=0.3, threshold=0.7
        )

        assert evaluator.model == "gpt-4o"
        assert evaluator.criteria == "accuracy"
        assert evaluator.temperature == 0.3
        assert evaluator.threshold == 0.7

    def test_default_build_prompt_without_task(self):
        """Test default build_prompt includes trace input/output."""
        evaluator = LLMAsJudgeEvaluator()
        trace = create_test_trajectory()

        prompt = evaluator.build_prompt(trace)
        assert "What is AI?" in prompt
        assert "AI is artificial intelligence" in prompt
        assert "Expected Output" not in prompt

    def test_default_build_prompt_with_task_expected_output(self):
        """Test default build_prompt includes expected output from task."""
        evaluator = LLMAsJudgeEvaluator()
        trace = create_test_trajectory()

        task = Task(
            task_id="task-1",
            name="AI Question",
            description="Test task",
            input="What is AI?",
            expected_output="correct answer",
        )

        prompt = evaluator.build_prompt(trace, task)
        assert "Expected Output: correct answer" in prompt

    def test_default_build_prompt_with_task_success_criteria(self):
        """Test default build_prompt includes success criteria from task."""
        evaluator = LLMAsJudgeEvaluator()
        trace = create_test_trajectory()

        task = Task(
            task_id="task-2",
            name="AI Question",
            description="Test task",
            input="What is AI?",
            success_criteria="Must be accurate",
        )

        prompt = evaluator.build_prompt(trace, task)
        assert "Success Criteria: Must be accurate" in prompt

    def test_custom_build_prompt_subclass(self):
        """Test subclass overriding build_prompt()."""

        class CustomJudge(LLMAsJudgeEvaluator):
            name = "custom-judge"

            def build_prompt(self, trace: Trace) -> str:
                return f"Rate: {trace.input.upper()} -> {trace.output.upper()}"

        evaluator = CustomJudge()
        trace = create_test_trajectory()

        prompt = evaluator.build_prompt(trace)
        assert "WHAT IS AI?" in prompt
        assert "AI IS ARTIFICIAL INTELLIGENCE." in prompt

    def test_parse_and_validate_valid(self):
        """Test Pydantic validation of valid JSON."""
        evaluator = LLMAsJudgeEvaluator()

        result, error = evaluator._parse_and_validate('{"score": 0.8, "explanation": "Good"}')
        assert result is not None
        assert error is None
        assert result.score == 0.8
        assert result.explanation == "Good"

    def test_parse_and_validate_invalid_score(self):
        """Test Pydantic validation rejects out-of-range score."""
        evaluator = LLMAsJudgeEvaluator()

        result, error = evaluator._parse_and_validate('{"score": 5.0, "explanation": "Bad"}')
        assert result is None
        assert error is not None

    def test_parse_and_validate_missing_score(self):
        """Test Pydantic validation rejects missing score."""
        evaluator = LLMAsJudgeEvaluator()

        result, error = evaluator._parse_and_validate('{"explanation": "No score"}')
        assert result is None
        assert error is not None

    def test_parse_and_validate_invalid_json(self):
        """Test Pydantic validation rejects invalid JSON."""
        evaluator = LLMAsJudgeEvaluator()

        result, error = evaluator._parse_and_validate('not json at all')
        assert result is None
        assert error is not None

    def test_threshold_pass_fail(self):
        """Test that threshold controls pass/fail."""
        evaluator = LLMAsJudgeEvaluator(threshold=0.8)

        result_pass, _ = evaluator._parse_and_validate('{"score": 0.9, "explanation": "Great"}')
        assert result_pass.passed is True

        result_fail, _ = evaluator._parse_and_validate('{"score": 0.5, "explanation": "OK"}')
        assert result_fail.passed is False


class TestFunctionEvaluator:
    """Tests for FunctionEvaluator to increase coverage."""

    def test_function_returns_eval_result(self):
        """Test function that returns EvalResult."""

        def custom_eval(trajectory: Trace, task=None):
            return EvalResult(score=0.7, explanation="Custom")

        evaluator = FunctionEvaluator(custom_eval, name="test-eval")
        observation = create_test_trajectory()

        result = evaluator.evaluate(observation)

        assert isinstance(result, EvalResult)
        assert result.score == 0.7
        assert result.explanation == "Custom"

    def test_function_returns_dict_full(self):
        """Test function that returns dict with all fields."""

        def custom_eval(trajectory: Trace, task=None):
            return {"score": 0.85, "passed": True, "explanation": "All good", "details": {"key": "value"}}

        evaluator = FunctionEvaluator(custom_eval)
        observation = create_test_trajectory()

        result = evaluator.evaluate(observation)

        assert result.score == 0.85
        assert result.passed is True
        assert result.explanation == "All good"
        assert result.details == {"key": "value"}

    def test_function_returns_dict_minimal(self):
        """Test function that returns dict with only score."""

        def custom_eval(trajectory: Trace, task=None):
            return {"score": 0.5}

        evaluator = FunctionEvaluator(custom_eval)
        observation = create_test_trajectory()

        result = evaluator.evaluate(observation)

        assert result.score == 0.5
        assert result.passed is True  # Auto-calculated from score >= 0.5
        assert result.explanation == ""
        assert result.details is None

    def test_function_returns_float(self):
        """Test function that returns float."""

        def custom_eval(trajectory: Trace, task=None):
            return 0.92

        evaluator = FunctionEvaluator(custom_eval)
        observation = create_test_trajectory()

        result = evaluator.evaluate(observation)

        assert result.score == 0.92

    def test_function_returns_int(self):
        """Test function that returns int."""

        def custom_eval(trajectory: Trace, task=None):
            return 1

        evaluator = FunctionEvaluator(custom_eval)
        observation = create_test_trajectory()

        result = evaluator.evaluate(observation)

        assert result.score == 1.0

    def test_function_returns_invalid_type(self):
        """Test function that returns invalid type raises error."""

        def custom_eval(trajectory: Trace, task=None):
            return "invalid return type"

        evaluator = FunctionEvaluator(custom_eval)
        observation = create_test_trajectory()

        with pytest.raises(TypeError, match="Evaluator returned invalid type"):
            evaluator.evaluate(observation)


class TestSignatureDrivenModeDetection:
    """Tests for signature-driven eval mode auto-detection."""

    def test_one_param_evaluator_supports_both_modes(self):
        """Evaluator with (self, trace: Trace) supports both experiment and monitor."""

        class MonitorOnlyEval(BaseEvaluator):
            name = "monitor-only"

            def evaluate(self, trace: Trace):
                return EvalResult(score=1.0)

        evaluator = MonitorOnlyEval()
        assert EvalMode.EXPERIMENT in evaluator._supported_eval_modes
        assert EvalMode.MONITOR in evaluator._supported_eval_modes

    def test_two_required_params_evaluator_experiment_only(self):
        """Evaluator with (self, trace: Trace, task) supports experiment only."""

        class ExperimentOnlyEval(BaseEvaluator):
            name = "experiment-only"

            def evaluate(self, trace: Trace, task: Task):
                return EvalResult(score=1.0 if trace.output == task.expected_output else 0.0)

        evaluator = ExperimentOnlyEval()
        assert evaluator._supported_eval_modes == [EvalMode.EXPERIMENT]

    def test_optional_task_param_supports_both_modes(self):
        """Evaluator with (self, trace: Trace, task=None) supports both modes."""

        class DualModeEval(BaseEvaluator):
            name = "dual-mode"

            def evaluate(self, trace: Trace, task=None):
                if task and task.expected_output:
                    return EvalResult(score=1.0 if trace.output == task.expected_output else 0.0)
                return EvalResult(score=1.0)

        evaluator = DualModeEval()
        assert EvalMode.EXPERIMENT in evaluator._supported_eval_modes
        assert EvalMode.MONITOR in evaluator._supported_eval_modes

    def test_monitor_dispatch_no_task_arg(self):
        """Evaluator with 1 param is called without task argument."""
        call_args = []

        class TraceOnlyEval(BaseEvaluator):
            name = "trace-only"

            def evaluate(self, trace: Trace):
                call_args.append({"trace": trace})
                return EvalResult(score=1.0)

        evaluator = TraceOnlyEval()
        trace = create_test_trajectory()
        task = Task(task_id="t1", name="test", description="test", input="test")

        # Even when task is passed to run(), the evaluate method receives no task
        evaluator.run(trace, task)
        assert len(call_args) == 1
        assert "trace" in call_args[0]

    def test_experiment_dispatch_with_task_arg(self):
        """Evaluator with 2 required params is called with task argument."""
        call_args = []

        class TaskRequiredEval(BaseEvaluator):
            name = "task-required"

            def evaluate(self, trace: Trace, task: Task):
                call_args.append({"trace": trace, "task": task})
                return EvalResult(score=1.0)

        evaluator = TaskRequiredEval()
        trace = create_test_trajectory()
        task = Task(task_id="t1", name="test", description="test", input="test")

        evaluator.run(trace, task)
        assert len(call_args) == 1
        assert call_args[0]["task"] is task

    def test_function_evaluator_auto_detect_both_modes(self):
        """FunctionEvaluator with 1-param function supports both modes."""

        def monitor_func(trace: Trace):
            return EvalResult(score=1.0)

        evaluator = FunctionEvaluator(monitor_func)
        assert EvalMode.EXPERIMENT in evaluator._supported_eval_modes
        assert EvalMode.MONITOR in evaluator._supported_eval_modes

    def test_function_evaluator_auto_detect_experiment_only(self):
        """FunctionEvaluator with 2-required-param function supports experiment only."""

        def experiment_func(trace: Trace, task: Task):
            return EvalResult(score=1.0)

        evaluator = FunctionEvaluator(experiment_func)
        assert evaluator._supported_eval_modes == [EvalMode.EXPERIMENT]

    def test_function_evaluator_dispatch_1_param(self):
        """FunctionEvaluator calls 1-param function without task."""
        call_args = []

        def monitor_func(trace: Trace):
            call_args.append(trace)
            return EvalResult(score=1.0)

        evaluator = FunctionEvaluator(monitor_func)
        trace = create_test_trajectory()
        evaluator.run(trace)
        assert len(call_args) == 1

    def test_function_evaluator_dispatch_2_params(self):
        """FunctionEvaluator calls 2-param function with task."""
        call_args = []

        def experiment_func(trace: Trace, task: Task):
            call_args.append({"trace": trace, "task": task})
            return EvalResult(score=1.0)

        evaluator = FunctionEvaluator(experiment_func)
        trace = create_test_trajectory()
        task = Task(task_id="t1", name="test", description="test", input="test")
        evaluator.run(trace, task)
        assert len(call_args) == 1
        assert call_args[0]["task"] is task
