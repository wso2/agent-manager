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
Tests for the LLM-as-judge evaluator redesign.

All tests mock litellm.completion — no actual LLM calls.
"""

import json
import sys
import pytest
from pydantic import ValidationError
from unittest.mock import patch, MagicMock
from typing import Optional

# Create a mock litellm module so we can patch it
_mock_litellm = MagicMock()
sys.modules.setdefault("litellm", _mock_litellm)

from amp_evaluation.evaluators.base import (  # noqa: E402
    LLMAsJudgeEvaluator,
    FunctionLLMJudge,
    JudgeOutput,
)
from amp_evaluation.evaluators.params import EvalMode, EvaluationLevel, Param  # noqa: E402
from amp_evaluation.models import EvalResult  # noqa: E402
from amp_evaluation.dataset import Task  # noqa: E402
from amp_evaluation.trace import Trace, TraceMetrics, TokenUsage  # noqa: E402
from amp_evaluation.trace.models import AgentTrace, LLMSpan  # noqa: E402
from amp_evaluation.registry import llm_judge  # noqa: E402


def _make_trace(**overrides):
    """Create a test trace."""
    defaults = dict(
        trace_id="test-123",
        input="What is AI?",
        output="AI is artificial intelligence.",
        metrics=TraceMetrics(
            total_duration_ms=100.0,
            token_usage=TokenUsage(input_tokens=10, output_tokens=20, total_tokens=30),
        ),
        spans=[],
    )
    defaults.update(overrides)
    return Trace(**defaults)


def _mock_litellm_response(score: float, explanation: str = "Good"):
    """Create a mock litellm completion response."""
    mock_response = MagicMock()
    mock_response.choices = [MagicMock()]
    mock_response.choices[0].message.content = json.dumps({"score": score, "explanation": explanation})
    return mock_response


def _mock_litellm_raw_response(content: str):
    """Create a mock litellm response with raw content string."""
    mock_response = MagicMock()
    mock_response.choices = [MagicMock()]
    mock_response.choices[0].message.content = content
    return mock_response


class _SimpleJudge(LLMAsJudgeEvaluator):
    """Minimal concrete subclass for testing (build_prompt is abstract)."""

    def build_prompt(self, trace: Trace, task: Optional[Task] = None) -> str:
        prompt = f"Evaluate:\nInput: {trace.input}\nOutput: {trace.output}"
        if task and task.expected_output:
            prompt += f"\n\nExpected Output: {task.expected_output}"
        if task and task.success_criteria:
            prompt += f"\nSuccess Criteria: {task.success_criteria}"
        return prompt


# =============================================================================
# LLMAsJudgeEvaluator init and params
# =============================================================================


class TestLLMAsJudgeInit:
    """Test LLMAsJudgeEvaluator initialization and Param defaults."""

    def test_default_params(self):
        evaluator = _SimpleJudge()
        assert evaluator.model == "gpt-4o-mini"
        assert evaluator.temperature == 0.0
        assert evaluator.max_tokens == 1024
        assert evaluator.max_retries == 2

    def test_custom_params(self):
        evaluator = _SimpleJudge(
            model="gpt-4o",
            temperature=0.5,
            max_tokens=2048,
            max_retries=3,
        )
        assert evaluator.model == "gpt-4o"
        assert evaluator.temperature == 0.5
        assert evaluator.max_tokens == 2048
        assert evaluator.max_retries == 3

    def test_default_build_prompt_content(self):
        evaluator = _SimpleJudge()
        trace = _make_trace()
        prompt = evaluator.build_prompt(trace)
        assert "What is AI?" in prompt
        assert "AI is artificial intelligence" in prompt


# =============================================================================
# Level detection from build_prompt()
# =============================================================================


class TestLevelDetection:
    """Test level auto-detection from build_prompt() type hints."""

    def test_trace_level_from_build_prompt(self):
        class TraceJudge(LLMAsJudgeEvaluator):
            name = "trace-judge"

            def build_prompt(self, trace: Trace) -> str:
                return f"Evaluate: {trace.input}"

        evaluator = TraceJudge()
        assert evaluator.level == EvaluationLevel.TRACE

    def test_agent_level_from_build_prompt(self):
        class AgentJudge(LLMAsJudgeEvaluator):
            name = "agent-judge"

            def build_prompt(self, agent: AgentTrace) -> str:
                return f"Evaluate agent: {agent.input}"

        evaluator = AgentJudge()
        assert evaluator.level == EvaluationLevel.AGENT

    def test_llm_level_from_build_prompt(self):
        class LLMJudge(LLMAsJudgeEvaluator):
            name = "llm-judge"

            def build_prompt(self, llm: LLMSpan) -> str:
                return f"Evaluate LLM: {llm.output}"

        evaluator = LLMJudge()
        assert evaluator.level == EvaluationLevel.LLM


# =============================================================================
# Mode detection from build_prompt()
# =============================================================================


class TestModeDetection:
    """Test mode auto-detection from build_prompt() task parameter."""

    def test_optional_task_both_modes(self):
        class BothModes(LLMAsJudgeEvaluator):
            name = "both-modes"

            def build_prompt(self, trace: Trace, task: Optional[Task] = None) -> str:
                return "prompt"

        evaluator = BothModes()
        assert EvalMode.EXPERIMENT in evaluator._supported_eval_modes
        assert EvalMode.MONITOR in evaluator._supported_eval_modes

    def test_required_task_experiment_only(self):
        class ExperimentOnly(LLMAsJudgeEvaluator):
            name = "experiment-only"

            def build_prompt(self, trace: Trace, task: Task) -> str:
                return "prompt"

        evaluator = ExperimentOnly()
        assert evaluator._supported_eval_modes == [EvalMode.EXPERIMENT]

    def test_no_task_both_modes(self):
        class MonitorFriendly(LLMAsJudgeEvaluator):
            name = "monitor-friendly"

            def build_prompt(self, trace: Trace) -> str:
                return "prompt"

        evaluator = MonitorFriendly()
        assert EvalMode.EXPERIMENT in evaluator._supported_eval_modes
        assert EvalMode.MONITOR in evaluator._supported_eval_modes


# =============================================================================
# Pydantic output validation
# =============================================================================


class TestPydanticOutputValidation:
    """Test JudgeOutput Pydantic validation."""

    def test_valid_json_parsed(self):
        evaluator = _SimpleJudge()
        result, error = evaluator._parse_and_validate('{"score": 0.8, "explanation": "Good"}')
        assert result is not None
        assert error is None
        assert result.score == 0.8
        assert "Good" in result.explanation
        evaluator = _SimpleJudge()
        result, error = evaluator._parse_and_validate('{"explanation": "no score"}')
        assert result is None
        assert error is not None

    def test_score_out_of_range_returns_error(self):
        evaluator = _SimpleJudge()
        result, error = evaluator._parse_and_validate('{"score": 5.0, "explanation": "too high"}')
        assert result is None
        assert error is not None

    def test_score_negative_returns_error(self):
        evaluator = _SimpleJudge()
        result, error = evaluator._parse_and_validate('{"score": -0.5, "explanation": "negative"}')
        assert result is None
        assert error is not None

    def test_invalid_json_returns_error(self):
        evaluator = _SimpleJudge()
        result, error = evaluator._parse_and_validate("not json")
        assert result is None
        assert error is not None

    def test_score_boundary_zero(self):
        evaluator = _SimpleJudge()
        result, _ = evaluator._parse_and_validate('{"score": 0.0}')
        assert result is not None
        assert result.score == 0.0

    def test_score_boundary_one(self):
        evaluator = _SimpleJudge()
        result, _ = evaluator._parse_and_validate('{"score": 1.0}')
        assert result is not None
        assert result.score == 1.0


# =============================================================================
# End-to-end pipeline (mocked LiteLLM)
# =============================================================================


class TestEndToEnd:
    """Test full LLM-as-judge pipeline with mocked LiteLLM."""

    @patch("litellm.completion")
    def test_full_pipeline(self, mock_completion):
        mock_completion.return_value = _mock_litellm_response(0.85, "Well done")

        evaluator = _SimpleJudge()
        trace = _make_trace()

        result = evaluator.evaluate(trace)

        assert result.score == 0.85
        assert "Well done" in result.explanation
        assert "model=gpt-4o-mini" in result.explanation
        mock_completion.assert_called_once()

    @patch("litellm.completion")
    def test_output_format_auto_appended(self, mock_completion):
        """Verify prompt sent to LLM contains JSON format instructions."""
        mock_completion.return_value = _mock_litellm_response(0.9, "Great")

        evaluator = _SimpleJudge()
        trace = _make_trace()
        evaluator.evaluate(trace)

        call_args = mock_completion.call_args
        prompt_sent = call_args[1]["messages"][0]["content"]
        assert '"score"' in prompt_sent
        assert '"explanation"' in prompt_sent
        assert "JSON" in prompt_sent

    @patch("litellm.completion")
    def test_build_prompt_receives_correct_types(self, mock_completion):
        """Verify build_prompt receives correct types."""
        received_args = []

        class TraceJudge(LLMAsJudgeEvaluator):
            name = "trace-judge"

            def build_prompt(self, trace: Trace, task=None) -> str:
                received_args.append({"trace": trace, "task": task})
                return "Evaluate this"

        mock_completion.return_value = _mock_litellm_response(0.9, "OK")

        evaluator = TraceJudge()
        trace = _make_trace()
        task = Task(task_id="t1", name="test", description="test", input="test")

        evaluator.evaluate(trace, task)

        assert len(received_args) == 1
        assert isinstance(received_args[0]["trace"], Trace)
        assert isinstance(received_args[0]["task"], Task)

    @patch("litellm.completion")
    def test_retry_on_invalid_output(self, mock_completion):
        """Test retry sends Pydantic error on invalid first response."""
        # First call: invalid, second call: valid
        mock_completion.side_effect = [
            _mock_litellm_raw_response('{"score": 99}'),  # Invalid
            _mock_litellm_response(0.8, "Retry worked"),  # Valid
        ]

        evaluator = _SimpleJudge()
        trace = _make_trace()

        result = evaluator.evaluate(trace)

        assert result.score == 0.8
        assert "Retry worked" in result.explanation
        assert mock_completion.call_count == 2

        # Second call should have retry context
        second_prompt = mock_completion.call_args_list[1][1]["messages"][0]["content"]
        assert "invalid" in second_prompt.lower() or "Previous response" in second_prompt

    @patch("litellm.completion")
    def test_all_retries_exhausted(self, mock_completion):
        """Test skipped result when all retries fail."""
        mock_completion.return_value = _mock_litellm_raw_response('{"bad": "json"}')

        evaluator = _SimpleJudge(max_retries=1)
        trace = _make_trace()

        result = evaluator.evaluate(trace)

        assert result.is_skipped
        assert "failed after 2 attempts" in result.skip_reason.lower()
        assert mock_completion.call_count == 2  # 1 initial + 1 retry

    @patch("litellm.completion")
    def test_custom_model_passed_to_litellm(self, mock_completion):
        """Test that custom model identifier is passed through."""
        mock_completion.return_value = _mock_litellm_response(0.7, "OK")

        evaluator = _SimpleJudge(model="anthropic/claude-sonnet-4-20250514")
        trace = _make_trace()
        evaluator.evaluate(trace)

        call_args = mock_completion.call_args
        assert call_args[1]["model"] == "anthropic/claude-sonnet-4-20250514"

    @patch("litellm.completion")
    def test_temperature_and_max_tokens_passed(self, mock_completion):
        """Test that temperature and max_tokens are forwarded."""
        mock_completion.return_value = _mock_litellm_response(0.7, "OK")

        evaluator = _SimpleJudge(temperature=0.5, max_tokens=2048)
        trace = _make_trace()
        evaluator.evaluate(trace)

        call_args = mock_completion.call_args
        assert call_args[1]["temperature"] == 0.5
        assert call_args[1]["max_tokens"] == 2048


# =============================================================================
# @llm_judge decorator
# =============================================================================


class TestLLMJudgeDecorator:
    """Test the @llm_judge decorator."""

    def test_basic_decorator(self):
        @llm_judge
        def quality_judge(trace: Trace) -> str:
            return f"Rate: {trace.input}"

        assert isinstance(quality_judge, FunctionLLMJudge)
        assert quality_judge.name == "quality_judge"

    def test_decorator_with_config(self):
        @llm_judge(model="gpt-4o")
        def grounding_judge(trace: Trace) -> str:
            return f"Grounding: {trace.output}"

        assert isinstance(grounding_judge, FunctionLLMJudge)
        assert grounding_judge.name == "grounding_judge"
        assert grounding_judge.model == "gpt-4o"

    def test_decorator_with_name(self):
        @llm_judge(name="custom-name")
        def my_judge(trace: Trace) -> str:
            return "prompt"

        assert my_judge.name == "custom-name"

    def test_decorator_level_from_function(self):
        @llm_judge
        def agent_judge(agent: AgentTrace) -> str:
            return f"Agent: {agent.input}"

        assert agent_judge.level == EvaluationLevel.AGENT

    def test_decorator_modes_from_function(self):
        @llm_judge
        def experiment_judge(trace: Trace, task: Task) -> str:
            return f"Evaluate: {trace.input} against {task.expected_output}"

        assert experiment_judge._supported_eval_modes == [EvalMode.EXPERIMENT]

    @patch("litellm.completion")
    def test_decorator_end_to_end(self, mock_completion):
        mock_completion.return_value = _mock_litellm_response(0.9, "Excellent")

        @llm_judge
        def quality_judge(trace: Trace) -> str:
            return f"Rate quality of: {trace.output}"

        trace = _make_trace()
        result = quality_judge.evaluate(trace)

        assert result.score == 0.9
        assert "Excellent" in result.explanation
        mock_completion.assert_called_once()


# =============================================================================
# Subclassing
# =============================================================================


class TestSubclassing:
    """Test subclassing LLMAsJudgeEvaluator."""

    @patch("litellm.completion")
    def test_custom_build_prompt_called(self, mock_completion):
        mock_completion.return_value = _mock_litellm_response(0.75, "Custom")

        class CustomJudge(LLMAsJudgeEvaluator):
            name = "custom"

            def build_prompt(self, trace: Trace) -> str:
                return f"CUSTOM: {trace.input} -> {trace.output}"

        evaluator = CustomJudge()
        trace = _make_trace()
        evaluator.evaluate(trace)

        prompt_sent = mock_completion.call_args[1]["messages"][0]["content"]
        assert "CUSTOM:" in prompt_sent

    def test_custom_call_llm_override(self):
        """Test that _call_llm_with_retry can be overridden (custom LLM client)."""

        class CustomLLMJudge(LLMAsJudgeEvaluator):
            name = "custom-llm"

            def build_prompt(self, trace: Trace) -> str:
                return f"Evaluate: {trace.output}"

            def _call_llm_with_retry(self, prompt: str) -> EvalResult:
                # Custom LLM logic — no litellm needed
                return EvalResult(
                    score=0.95,
                    passed=True,
                    explanation="Custom LLM says great",
                )

        evaluator = CustomLLMJudge()
        trace = _make_trace()
        result = evaluator.evaluate(trace)

        assert result.score == 0.95
        assert result.explanation == "Custom LLM says great"


# =============================================================================
# JudgeOutput Pydantic model
# =============================================================================


class TestJudgeOutput:
    """Test JudgeOutput Pydantic model directly."""

    def test_valid_creation(self):
        output = JudgeOutput(score=0.8, explanation="Good")
        assert output.score == 0.8
        assert output.explanation == "Good"

    def test_default_explanation(self):
        output = JudgeOutput(score=0.5)
        assert output.explanation == ""

    def test_score_validation_too_high(self):
        with pytest.raises(ValidationError):
            JudgeOutput(score=1.5, explanation="Too high")

    def test_score_validation_too_low(self):
        with pytest.raises(ValidationError):
            JudgeOutput(score=-0.1, explanation="Too low")

    def test_json_round_trip(self):
        output = JudgeOutput(score=0.7, explanation="OK")
        json_str = output.model_dump_json()
        parsed = JudgeOutput.model_validate_json(json_str)
        assert parsed.score == 0.7
        assert parsed.explanation == "OK"


# =============================================================================
# FunctionLLMJudge — Param support and with_config
# =============================================================================


class TestFunctionLLMJudgeParams:
    """Test that @llm_judge supports Param descriptors and with_config()."""

    def test_param_defaults_extracted(self):
        """Param defaults should be stored in _func_config."""

        @llm_judge
        def my_judge(
            trace: Trace,
            strictness: float = Param(default=0.8, description="How strict"),
        ) -> str:
            return f"Evaluate with strictness {strictness}: {trace.output}"

        assert my_judge._func_param_descriptors["strictness"] is not None
        assert my_judge._func_config["strictness"] == 0.8

    def test_param_injected_into_build_prompt(self):
        """Config values should be injected when building the prompt."""

        @llm_judge
        def my_judge(
            trace: Trace,
            strictness: float = Param(default=0.8, description="How strict"),
        ) -> str:
            return f"strictness={strictness} output={trace.output}"

        trace = _make_trace()
        prompt = my_judge._dispatch_build_prompt(trace, None)
        assert "strictness=0.8" in prompt

    def test_param_override_via_constructor(self):
        """Param values can be overridden via decorator kwargs."""

        @llm_judge(name="custom-judge", strictness=0.5)
        def my_judge(
            trace: Trace,
            strictness: float = Param(default=0.8, description="How strict"),
        ) -> str:
            return f"strictness={strictness}"

        assert my_judge._func_config["strictness"] == 0.5

    def test_with_config_func_param(self):
        """with_config() should create a copy with updated function Param values."""

        @llm_judge
        def my_judge(
            trace: Trace,
            threshold: float = Param(default=0.7, description="Threshold", min=0, max=1),
        ) -> str:
            return f"threshold={threshold}"

        copy = my_judge.with_config(threshold=0.9)

        assert copy is not my_judge
        assert my_judge._func_config["threshold"] == 0.7
        assert copy._func_config["threshold"] == 0.9

    def test_with_config_llm_param(self):
        """with_config() should also accept inherited LLM params like model."""

        @llm_judge
        def my_judge(trace: Trace) -> str:
            return f"Evaluate: {trace.output}"

        copy = my_judge.with_config(model="openai/gpt-4o", temperature=0.5)

        assert copy.model == "openai/gpt-4o"
        assert copy.temperature == 0.5
        # Original unchanged
        assert my_judge.temperature == 0.0

    def test_with_config_mixed_params(self):
        """with_config() should handle both function Params and LLM params together."""

        @llm_judge
        def my_judge(
            trace: Trace,
            strictness: float = Param(default=0.8, description="Strictness"),
        ) -> str:
            return f"strictness={strictness}"

        copy = my_judge.with_config(strictness=0.5, model="openai/gpt-4o")

        assert copy._func_config["strictness"] == 0.5
        assert copy.model == "openai/gpt-4o"

    def test_with_config_unknown_key_raises(self):
        """with_config() should raise TypeError for unknown keys."""

        @llm_judge
        def my_judge(trace: Trace) -> str:
            return "prompt"

        with pytest.raises(TypeError, match="Unknown config parameter"):
            my_judge.with_config(nonexistent=42)

    def test_required_param_missing_raises_at_init(self):
        """Required Params (no default) not supplied at init must raise TypeError immediately (Issue 7)."""

        with pytest.raises(TypeError, match="missing required parameter"):

            @llm_judge
            def my_judge(
                trace: Trace,
                criteria: str = Param(description="Required evaluation criteria"),
            ) -> str:
                return f"Evaluate against: {criteria}"

    def test_required_param_supplied_at_init_succeeds(self):
        """Required Params provided at decoration time should not raise."""

        @llm_judge(criteria="be concise")
        def my_judge(
            trace: Trace,
            criteria: str = Param(description="Required evaluation criteria"),
        ) -> str:
            return f"Evaluate against: {criteria}"

        assert my_judge._func_config["criteria"] == "be concise"

    def test_with_config_does_not_raise_for_required_param_already_set(self):
        """with_config() on an evaluator with a required Param must not raise TypeError.

        Regression: the missing_required check in _init_function_params fired
        during cloning because _func_config was reset before the old values
        were rehydrated.
        """

        @llm_judge(criteria="be concise")
        def my_judge(
            trace: Trace,
            criteria: str = Param(description="Required evaluation criteria"),
        ) -> str:
            return f"Evaluate against: {criteria}"

        # Should not raise — criteria is already satisfied in the original instance
        clone = my_judge.with_config(model="openai/gpt-4o")
        assert clone._func_config["criteria"] == "be concise"
        assert clone.model == "openai/gpt-4o"

    def test_with_config_validation(self):
        """with_config() should validate Param constraints."""

        @llm_judge
        def my_judge(
            trace: Trace,
            threshold: float = Param(default=0.5, description="Threshold", min=0, max=1),
        ) -> str:
            return "prompt"

        with pytest.raises(ValueError):
            my_judge.with_config(threshold=5.0)

    def test_config_schema_includes_func_params(self):
        """info.config_schema should include both LLM params and function params."""

        @llm_judge
        def my_judge(
            trace: Trace,
            strictness: float = Param(default=0.8, description="How strict"),
        ) -> str:
            return "prompt"

        schema = my_judge.info.config_schema
        keys = [s["key"] for s in schema]
        # LLM params from LLMAsJudgeEvaluator
        assert "model" in keys
        assert "temperature" in keys
        # Function param
        assert "strictness" in keys

    def test_mode_detection_skips_param_defaults(self):
        """Param params should not affect mode detection."""

        @llm_judge
        def my_judge(
            trace: Trace,
            strictness: float = Param(default=0.8, description="Strictness"),
        ) -> str:
            return "prompt"

        # No task param (ignoring Param defaults) → both modes
        modes = my_judge._supported_eval_modes
        assert EvalMode.EXPERIMENT in modes
        assert EvalMode.MONITOR in modes

    def test_docstring_used_as_description(self):
        """Function docstring should be used as evaluator description."""

        @llm_judge
        def my_judge(trace: Trace) -> str:
            """Check response quality."""
            return "prompt"

        assert my_judge.description == "Check response quality."

    @patch("litellm.completion")
    def test_end_to_end_with_params(self, mock_completion):
        """Full evaluate() call with Param injection into prompt."""
        mock_completion.return_value = _mock_litellm_response(0.9, "Great")

        @llm_judge
        def my_judge(
            trace: Trace,
            strictness: float = Param(default=0.8, description="Strictness"),
        ) -> str:
            return f"Evaluate with strictness={strictness}: {trace.output}"

        trace = _make_trace()
        result = my_judge.evaluate(trace)

        assert result.score == 0.9
        # Verify the prompt sent to LLM includes the config value
        call_args = mock_completion.call_args
        prompt_sent = call_args[1]["messages"][0]["content"]
        assert "strictness=0.8" in prompt_sent
