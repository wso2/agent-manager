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
Unit tests for built-in LLM-as-judge evaluators.

All tests mock _call_llm_with_retry -- no actual LLM calls are made.
Tests verify: instantiation, level/mode detection, prompt content,
span-dependent behavior (skip vs zero), global config, and builtin() discovery.
"""

import sys
import pytest
from pathlib import Path
from unittest.mock import patch, MagicMock

# Ensure litellm is mocked before imports
_mock_litellm = MagicMock()
sys.modules.setdefault("litellm", _mock_litellm)

sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from amp_evaluation.evaluators.params import EvaluationLevel, EvalMode
from amp_evaluation.evaluators.base import LLMAsJudgeEvaluator
from amp_evaluation.evaluators.builtin import builtin, list_builtin_evaluators
from amp_evaluation.models import EvalResult
from amp_evaluation.trace.models import (
    Trace,
    AgentTrace,
    LLMSpan,
    ToolSpan,
    RetrieverSpan,
    RetrievedDoc,
    AgentSpan,
    TraceMetrics,
    TokenUsage,
    SpanMetrics,
    LLMMetrics,
    RetrieverMetrics,
    AgentMetrics,
    SystemMessage,
    UserMessage,
    AssistantMessage,
)
from amp_evaluation.dataset.models import Task
from amp_evaluation.evaluators.builtin.llm_judge import (
    HelpfulnessEvaluator,
    ClarityEvaluator,
    AccuracyEvaluator,
    CoherenceEvaluator,
    CompletenessEvaluator,
    ConcisenessEvaluator,
    FaithfulnessEvaluator,
    ContextRelevanceEvaluator,
    SafetyEvaluator,
    ToneEvaluator,
    InstructionFollowingEvaluator,
    RelevanceEvaluator,
    SemanticSimilarityEvaluator,
    HallucinationEvaluator,
    GoalClarityEvaluator,
    ReasoningQualityEvaluator,
    PathEfficiencyEvaluator,
    ErrorRecoveryEvaluator,
)


# ============================================================================
# HELPERS
# ============================================================================

_GOOD_RESULT = EvalResult(score=0.9, passed=True, explanation="Good")
_BAD_RESULT = EvalResult(score=0.2, passed=False, explanation="Poor")


def _make_trace(
    trace_id: str = "trace-1",
    input: str = "What is the capital of France?",
    output: str = "The capital of France is Paris.",
    spans=None,
) -> Trace:
    return Trace(
        trace_id=trace_id,
        input=input,
        output=output,
        spans=spans or [],
        metrics=TraceMetrics(token_usage=TokenUsage(input_tokens=10, output_tokens=20, total_tokens=30)),
    )


def _make_trace_with_tool(tool_result="{'population': 2.1}") -> Trace:
    tool_span = ToolSpan(
        span_id="tool-1",
        parent_span_id=None,
        name="search_web",
        result=tool_result,
        metrics=SpanMetrics(),
    )
    return _make_trace(spans=[tool_span])


def _make_trace_with_retrieval(doc_content="Paris is the capital of France.") -> Trace:
    retrieval_span = RetrieverSpan(
        span_id="ret-1",
        parent_span_id=None,
        query="capital of France",
        documents=[RetrievedDoc(content=doc_content)],
        metrics=RetrieverMetrics(),
    )
    return _make_trace(spans=[retrieval_span])


def _make_trace_with_system_prompt(system_prompt: str = "You are a helpful assistant.") -> Trace:
    agent_span = AgentSpan(
        span_id="agent-1",
        name="assistant",
        system_prompt=system_prompt,
        metrics=AgentMetrics(),
    )
    return _make_trace(spans=[agent_span])


def _make_trace_with_llm_system_message(system_content: str = "You are a helpful assistant.") -> Trace:
    llm_span = LLMSpan(
        span_id="llm-1",
        messages=[
            SystemMessage(content=system_content),
            UserMessage(content="What is Paris?"),
            AssistantMessage(content="Paris is the capital of France."),
        ],
        response="Paris is the capital of France.",
        metrics=LLMMetrics(),
    )
    return _make_trace(spans=[llm_span])


def _make_llm_span(
    user_content: str = "What is the capital of France?",
    response: str = "The capital of France is Paris.",
    system_content: str = "",
) -> LLMSpan:
    """Create an LLMSpan for LLM-level evaluator tests."""
    messages = []
    if system_content:
        messages.append(SystemMessage(content=system_content))
    messages.append(UserMessage(content=user_content))
    messages.append(AssistantMessage(content=response))
    return LLMSpan(
        span_id="llm-1",
        messages=messages,
        response=response,
        metrics=LLMMetrics(),
    )


def _make_agent_trace(
    agent_id: str = "agent-1",
    input: str = "Find the population of Paris.",
    output: str = "The population of Paris is approximately 2.1 million.",
    system_prompt: str = "",
    steps=None,
) -> AgentTrace:
    return AgentTrace(
        agent_id=agent_id,
        agent_name="research_agent",
        system_prompt=system_prompt,
        input=input,
        output=output,
        steps=steps or [],
        metrics=TraceMetrics(),
    )


def _mock_llm(evaluator_instance, return_result=_GOOD_RESULT):
    """Patch _call_llm_with_retry on a specific evaluator instance."""
    evaluator_instance._call_llm_with_retry = lambda prompt: return_result
    return evaluator_instance


# ============================================================================
# SECTION 1: DISCOVERY & CATALOG
# ============================================================================


class TestDiscovery:
    """Test that all 18 evaluators are discoverable via builtin()."""

    ALL_LLM_JUDGE_NAMES = [
        "helpfulness",
        "clarity",
        "accuracy",
        "coherence",
        "completeness",
        "conciseness",
        "faithfulness",
        "context_relevance",
        "safety",
        "tone",
        "instruction_following",
        "relevance",
        "semantic_similarity",
        "hallucination",
        "goal_clarity",
        "reasoning_quality",
        "path_efficiency",
        "error_recovery",
    ]

    def test_all_evaluators_discoverable(self):
        available = list_builtin_evaluators()
        for name in self.ALL_LLM_JUDGE_NAMES:
            assert name in available, f"Expected '{name}' in list_builtin_evaluators()"

    def test_builtin_factory_creates_instances(self):
        for name in self.ALL_LLM_JUDGE_NAMES:
            ev = builtin(name)
            assert ev.name == name

    def test_monitor_mode_evaluators_discoverable(self):
        monitor_names = list_builtin_evaluators(mode="monitor")
        monitor_llm_judge = [
            "helpfulness",
            "clarity",
            "accuracy",
            "coherence",
            "completeness",
            "conciseness",
            "faithfulness",
            "context_relevance",
            "safety",
            "tone",
            "instruction_following",
            "relevance",
            "hallucination",
            "goal_clarity",
            "reasoning_quality",
            "path_efficiency",
            "error_recovery",
        ]
        for name in monitor_llm_judge:
            assert name in monitor_names, f"Expected '{name}' in monitor evaluators"

    def test_semantic_similarity_is_experiment_only(self):
        monitor_names = list_builtin_evaluators(mode="monitor")
        assert "semantic_similarity" not in monitor_names
        experiment_names = list_builtin_evaluators(mode="experiment")
        assert "semantic_similarity" in experiment_names

    def test_removed_evaluators_not_found(self):
        """Verify removed/renamed evaluators raise ValueError."""
        with pytest.raises(ValueError):
            builtin("response_quality")
        with pytest.raises(ValueError):
            builtin("agent_goal_clarity")
        with pytest.raises(ValueError):
            builtin("agent_reasoning_quality")
        with pytest.raises(ValueError):
            builtin("answer_relevancy")
        with pytest.raises(ValueError):
            builtin("hallucination_llm")
        with pytest.raises(ValueError):
            builtin("llm_relevancy")


# ============================================================================
# SECTION 2: TRACE-LEVEL EVALUATORS
# ============================================================================


class TestTraceLevel:
    """Tests for trace-level evaluators: helpfulness, clarity, accuracy, completeness, relevance, etc."""

    @pytest.mark.parametrize(
        "cls,name",
        [
            (HelpfulnessEvaluator, "helpfulness"),
            (ClarityEvaluator, "clarity"),
            (AccuracyEvaluator, "accuracy"),
            (CompletenessEvaluator, "completeness"),
            (RelevanceEvaluator, "relevance"),
            (HallucinationEvaluator, "hallucination"),
        ],
    )
    def test_name_and_level(self, cls, name):
        ev = cls()
        assert ev.name == name
        assert ev.level == EvaluationLevel.TRACE

    @pytest.mark.parametrize(
        "cls",
        [
            HelpfulnessEvaluator,
            ClarityEvaluator,
            AccuracyEvaluator,
            CompletenessEvaluator,
            RelevanceEvaluator,
            HallucinationEvaluator,
        ],
    )
    def test_supports_both_modes(self, cls):
        ev = cls()
        assert EvalMode.MONITOR in ev._supported_eval_modes
        assert EvalMode.EXPERIMENT in ev._supported_eval_modes

    @pytest.mark.parametrize(
        "cls",
        [
            HelpfulnessEvaluator,
            ClarityEvaluator,
            AccuracyEvaluator,
            CompletenessEvaluator,
            RelevanceEvaluator,
            HallucinationEvaluator,
        ],
    )
    def test_evaluate_returns_eval_result(self, cls):
        ev = _mock_llm(cls())
        trace = _make_trace()
        result = ev.evaluate(trace)
        assert isinstance(result, EvalResult)
        assert result.score == 0.9

    def test_helpfulness_prompt_contains_input_output(self):
        ev = HelpfulnessEvaluator()
        trace = _make_trace(input="Hello?", output="Hi there!")
        prompt = ev.build_prompt(trace)
        assert "Hello?" in prompt
        assert "Hi there!" in prompt

    def test_helpfulness_prompt_includes_success_criteria(self):
        ev = HelpfulnessEvaluator()
        trace = _make_trace()
        task = Task(
            task_id="t1",
            name="test",
            description="test",
            input="test",
            success_criteria="Must be actionable",
        )
        prompt = ev.build_prompt(trace, task)
        assert "Must be actionable" in prompt

    def test_completeness_prompt_includes_success_criteria(self):
        ev = CompletenessEvaluator()
        trace = _make_trace()
        task = Task(
            task_id="t1",
            name="test",
            description="test",
            input="test",
            success_criteria="Must cover all 3 points",
        )
        prompt = ev.build_prompt(trace, task)
        assert "Must cover all 3 points" in prompt

    def test_builtin_factory_with_model_override(self):
        ev = builtin("helpfulness", model="gpt-4o")
        assert ev.model == "gpt-4o"


# ============================================================================
# SECTION 3: LLM-SPAN-LEVEL EVALUATORS
# ============================================================================


class TestLLMSpanLevel:
    """Tests for LLM-span-level evaluators: coherence, conciseness, safety, tone."""

    @pytest.mark.parametrize(
        "cls,name",
        [
            (CoherenceEvaluator, "coherence"),
            (ConcisenessEvaluator, "conciseness"),
            (SafetyEvaluator, "safety"),
            (ToneEvaluator, "tone"),
        ],
    )
    def test_name_and_level(self, cls, name):
        ev = cls()
        assert ev.name == name
        assert ev.level == EvaluationLevel.LLM

    @pytest.mark.parametrize(
        "cls",
        [
            CoherenceEvaluator,
            ConcisenessEvaluator,
            SafetyEvaluator,
            ToneEvaluator,
        ],
    )
    def test_supports_both_modes(self, cls):
        ev = cls()
        assert EvalMode.MONITOR in ev._supported_eval_modes
        assert EvalMode.EXPERIMENT in ev._supported_eval_modes

    @pytest.mark.parametrize(
        "cls",
        [
            CoherenceEvaluator,
            ConcisenessEvaluator,
            SafetyEvaluator,
            ToneEvaluator,
        ],
    )
    def test_evaluate_returns_result(self, cls):
        ev = _mock_llm(cls())
        llm_span = _make_llm_span()
        result = ev.evaluate(llm_span)
        assert isinstance(result, EvalResult)
        assert result.score == 0.9

    def test_coherence_prompt_contains_response(self):
        ev = CoherenceEvaluator()
        llm_span = _make_llm_span(response="Paris is the capital.")
        prompt = ev.build_prompt(llm_span)
        assert "Paris is the capital." in prompt

    def test_conciseness_prompt_contains_response(self):
        ev = ConcisenessEvaluator()
        llm_span = _make_llm_span(response="Well, you see, actually...")
        prompt = ev.build_prompt(llm_span)
        assert "Well, you see, actually..." in prompt

    def test_safety_context_param_accepted(self):
        ev = SafetyEvaluator(context="customer support")
        assert ev.context == "customer support"

    def test_safety_prompt_includes_context(self):
        ev = SafetyEvaluator(context="children's education")
        llm_span = _make_llm_span()
        prompt = ev.build_prompt(llm_span)
        assert "children's education" in prompt

    def test_safety_prompt_includes_categories(self):
        ev = SafetyEvaluator()
        llm_span = _make_llm_span()
        prompt = ev.build_prompt(llm_span)
        assert "Hate speech" in prompt or "discrimination" in prompt
        assert "Self-harm" in prompt or "self-harm" in prompt.lower()

    def test_tone_context_param_accepted(self):
        ev = ToneEvaluator(context="technical documentation")
        assert ev.context == "technical documentation"

    def test_tone_prompt_includes_context(self):
        ev = ToneEvaluator(context="medical advice")
        llm_span = _make_llm_span()
        prompt = ev.build_prompt(llm_span)
        assert "medical advice" in prompt


# ============================================================================
# SECTION 4: GROUNDEDNESS & FAITHFULNESS
# ============================================================================


class TestFaithfulness:
    """Tests for faithfulness evaluator."""

    def test_name_and_level(self):
        ev = FaithfulnessEvaluator()
        assert ev.name == "faithfulness"
        assert ev.level == EvaluationLevel.TRACE

    def test_supports_both_modes(self):
        ev = FaithfulnessEvaluator()
        assert EvalMode.MONITOR in ev._supported_eval_modes
        assert EvalMode.EXPERIMENT in ev._supported_eval_modes

    def test_skips_when_no_tool_or_retrieval_spans(self):
        ev = FaithfulnessEvaluator()  # default: on_missing_context="skip"
        trace = _make_trace()  # no spans
        result = ev.evaluate(trace)
        assert result.is_skipped is True
        assert "No tool or retrieval spans" in result.skip_reason

    def test_zero_when_no_spans_and_on_missing_context_zero(self):
        ev = FaithfulnessEvaluator(on_missing_context="zero")
        trace = _make_trace()
        result = ev.evaluate(trace)
        assert result.is_skipped is False
        assert result.score == 0.0
        assert result.passed is False

    def test_evaluates_when_tool_spans_present(self):
        ev = _mock_llm(FaithfulnessEvaluator())
        trace = _make_trace_with_tool()
        result = ev.evaluate(trace)
        assert result.score == 0.9

    def test_evaluates_when_retrieval_spans_present(self):
        ev = _mock_llm(FaithfulnessEvaluator())
        trace = _make_trace_with_retrieval()
        result = ev.evaluate(trace)
        assert result.score == 0.9

    def test_prompt_includes_tool_results(self):
        ev = FaithfulnessEvaluator()
        trace = _make_trace_with_tool(tool_result="{'population': 2.1}")
        prompt = ev.build_prompt(trace)
        assert "search_web" in prompt
        assert "population" in prompt.lower() or "2.1" in prompt

    def test_prompt_includes_retrieved_docs(self):
        ev = FaithfulnessEvaluator()
        trace = _make_trace_with_retrieval(doc_content="Paris is the capital of France.")
        prompt = ev.build_prompt(trace)
        assert "Paris" in prompt

    def test_prompt_includes_claim_level_analysis(self):
        """Faithfulness prompt should use claim-level grounding approach."""
        ev = FaithfulnessEvaluator()
        trace = _make_trace_with_tool()
        prompt = ev.build_prompt(trace)
        assert "claim" in prompt.lower()

    def test_on_missing_context_param_validation(self):
        with pytest.raises(Exception):
            FaithfulnessEvaluator(on_missing_context="invalid_value")


class TestContextRelevance:
    """Tests for context_relevance evaluator."""

    def test_name_and_level(self):
        ev = ContextRelevanceEvaluator()
        assert ev.name == "context_relevance"
        assert ev.level == EvaluationLevel.TRACE

    def test_skips_when_no_retrieval_spans(self):
        ev = ContextRelevanceEvaluator()
        trace = _make_trace()
        result = ev.evaluate(trace)
        assert result.is_skipped is True

    def test_zero_when_no_retrievals_and_on_missing_context_zero(self):
        ev = ContextRelevanceEvaluator(on_missing_context="zero")
        trace = _make_trace()
        result = ev.evaluate(trace)
        assert result.score == 0.0

    def test_evaluates_when_retrieval_spans_present(self):
        ev = _mock_llm(ContextRelevanceEvaluator())
        trace = _make_trace_with_retrieval()
        result = ev.evaluate(trace)
        assert result.score == 0.9

    def test_tool_spans_alone_do_not_satisfy_context_relevance(self):
        # context_relevance requires retriever spans, not just tool spans
        ev = ContextRelevanceEvaluator()
        trace = _make_trace_with_tool()
        result = ev.evaluate(trace)
        assert result.is_skipped is True


# ============================================================================
# SECTION 5: INSTRUCTION FOLLOWING
# ============================================================================


class TestInstructionFollowing:
    """Tests for instruction_following evaluator."""

    def test_name_and_level(self):
        ev = InstructionFollowingEvaluator()
        assert ev.name == "instruction_following"
        assert ev.level == EvaluationLevel.TRACE

    def test_supports_both_modes(self):
        ev = InstructionFollowingEvaluator()
        assert EvalMode.MONITOR in ev._supported_eval_modes

    def test_skips_when_no_system_prompt_no_task(self):
        ev = InstructionFollowingEvaluator()
        trace = _make_trace()  # no spans, no task
        result = ev.evaluate(trace)
        assert result.is_skipped is True
        assert "No system prompt" in result.skip_reason

    def test_zero_when_no_instructions_and_on_missing_context_zero(self):
        ev = InstructionFollowingEvaluator(on_missing_context="zero")
        trace = _make_trace()
        result = ev.evaluate(trace)
        assert result.score == 0.0

    def test_evaluates_when_agent_system_prompt_present(self):
        ev = _mock_llm(InstructionFollowingEvaluator())
        trace = _make_trace_with_system_prompt("Always respond in English.")
        result = ev.evaluate(trace)
        assert result.score == 0.9

    def test_evaluates_when_llm_system_message_present(self):
        ev = _mock_llm(InstructionFollowingEvaluator())
        trace = _make_trace_with_llm_system_message("Be concise.")
        result = ev.evaluate(trace)
        assert result.score == 0.9

    def test_evaluates_when_task_description_present(self):
        ev = _mock_llm(InstructionFollowingEvaluator())
        trace = _make_trace()  # no spans
        task = Task(task_id="t1", name="test", description="Respond in bullet points", input="test")
        result = ev.evaluate(trace, task)
        assert result.score == 0.9

    def test_prompt_contains_system_prompt_content(self):
        ev = InstructionFollowingEvaluator()
        trace = _make_trace_with_system_prompt("You are a professional lawyer.")
        prompt = ev.build_prompt(trace)
        assert "professional lawyer" in prompt

    def test_task_success_criteria_used_as_instructions(self):
        ev = _mock_llm(InstructionFollowingEvaluator())
        trace = _make_trace()
        task = Task(
            task_id="t1",
            name="test",
            description="test",
            input="test",
            success_criteria=["Must be under 100 words", "Must include examples"],
        )
        result = ev.evaluate(trace, task)
        assert result.score == 0.9


# ============================================================================
# SECTION 6: SEMANTIC EVALUATORS
# ============================================================================


class TestRelevance:
    """Tests for relevance evaluator."""

    def test_name_and_level(self):
        ev = RelevanceEvaluator()
        assert ev.name == "relevance"
        assert ev.level == EvaluationLevel.TRACE

    def test_supports_both_modes(self):
        ev = RelevanceEvaluator()
        assert EvalMode.MONITOR in ev._supported_eval_modes

    def test_evaluate_returns_result(self):
        ev = _mock_llm(RelevanceEvaluator())
        trace = _make_trace()
        result = ev.evaluate(trace)
        assert isinstance(result, EvalResult)


class TestSemanticSimilarity:
    """Tests for semantic_similarity evaluator (experiment-only)."""

    def test_name_and_level(self):
        ev = SemanticSimilarityEvaluator()
        assert ev.name == "semantic_similarity"
        assert ev.level == EvaluationLevel.TRACE

    def test_experiment_only(self):
        ev = SemanticSimilarityEvaluator()
        assert ev._supported_eval_modes == [EvalMode.EXPERIMENT]
        assert EvalMode.MONITOR not in ev._supported_eval_modes

    def test_skips_when_task_has_no_expected_output(self):
        ev = SemanticSimilarityEvaluator()
        trace = _make_trace()
        task = Task(task_id="t1", name="test", description="test", input="test")
        # task has no expected_output
        result = ev.evaluate(trace, task)
        assert result.is_skipped is True

    def test_zero_when_no_expected_output_and_on_missing_context_zero(self):
        ev = SemanticSimilarityEvaluator(on_missing_context="zero")
        trace = _make_trace()
        task = Task(task_id="t1", name="test", description="test", input="test")
        result = ev.evaluate(trace, task)
        assert result.score == 0.0

    def test_evaluates_when_expected_output_present(self):
        ev = _mock_llm(SemanticSimilarityEvaluator())
        trace = _make_trace()
        task = Task(
            task_id="t1",
            name="test",
            description="test",
            input="test",
            expected_output="Paris is the capital of France.",
        )
        result = ev.evaluate(trace, task)
        assert result.score == 0.9

    def test_prompt_contains_expected_output(self):
        ev = SemanticSimilarityEvaluator()
        trace = _make_trace()
        task = Task(
            task_id="t1",
            name="test",
            description="test",
            input="test",
            expected_output="The answer is Paris.",
        )
        prompt = ev.build_prompt(trace, task)
        assert "The answer is Paris." in prompt


class TestHallucination:
    """Tests for hallucination evaluator."""

    def test_name_and_level(self):
        ev = HallucinationEvaluator()
        assert ev.name == "hallucination"
        assert ev.level == EvaluationLevel.TRACE

    def test_supports_both_modes(self):
        ev = HallucinationEvaluator()
        assert EvalMode.MONITOR in ev._supported_eval_modes

    def test_evaluate_without_spans(self):
        ev = _mock_llm(HallucinationEvaluator())
        trace = _make_trace()  # no spans -- still evaluates (just without tool context)
        result = ev.evaluate(trace)
        assert result.score == 0.9

    def test_evaluate_with_tool_spans(self):
        ev = _mock_llm(HallucinationEvaluator())
        trace = _make_trace_with_tool()
        result = ev.evaluate(trace)
        assert result.score == 0.9

    def test_prompt_includes_tool_context_when_present(self):
        ev = HallucinationEvaluator()
        trace = _make_trace_with_tool(tool_result="{'city': 'Paris', 'population': 2100000}")
        prompt = ev.build_prompt(trace)
        assert "Paris" in prompt or "population" in prompt.lower()

    def test_prompt_includes_evidence_verification(self):
        """Hallucination prompt should ask for evidence verification."""
        ev = HallucinationEvaluator()
        trace = _make_trace_with_tool()
        prompt = ev.build_prompt(trace)
        assert "evidence" in prompt.lower()

    def test_tags_include_correctness_and_safety(self):
        ev = HallucinationEvaluator()
        assert "correctness" in ev.tags
        assert "safety" in ev.tags


# ============================================================================
# SECTION 7: AGENT-LEVEL EVALUATORS
# ============================================================================


class TestGoalClarity:
    """Tests for goal_clarity evaluator."""

    def test_name_and_level(self):
        ev = GoalClarityEvaluator()
        assert ev.name == "goal_clarity"
        assert ev.level == EvaluationLevel.AGENT

    def test_supports_both_modes(self):
        ev = GoalClarityEvaluator()
        assert EvalMode.MONITOR in ev._supported_eval_modes
        assert EvalMode.EXPERIMENT in ev._supported_eval_modes

    def test_evaluate_returns_result(self):
        ev = _mock_llm(GoalClarityEvaluator())
        agent_trace = _make_agent_trace()
        result = ev.evaluate(agent_trace)
        assert isinstance(result, EvalResult)
        assert result.score == 0.9

    def test_prompt_contains_agent_input(self):
        ev = GoalClarityEvaluator()
        agent_trace = _make_agent_trace(input="Find the best restaurant in Paris.")
        prompt = ev.build_prompt(agent_trace)
        assert "Find the best restaurant in Paris" in prompt

    def test_prompt_contains_system_prompt(self):
        ev = GoalClarityEvaluator()
        agent_trace = _make_agent_trace(system_prompt="You are a travel assistant.")
        prompt = ev.build_prompt(agent_trace)
        assert "travel assistant" in prompt

    def test_builtin_factory(self):
        ev = builtin("goal_clarity")
        assert ev.name == "goal_clarity"
        assert ev.level == EvaluationLevel.AGENT


class TestReasoningQuality:
    """Tests for reasoning_quality evaluator."""

    def test_name_and_level(self):
        ev = ReasoningQualityEvaluator()
        assert ev.name == "reasoning_quality"
        assert ev.level == EvaluationLevel.AGENT

    def test_supports_both_modes(self):
        ev = ReasoningQualityEvaluator()
        assert EvalMode.MONITOR in ev._supported_eval_modes

    def test_evaluate_returns_result(self):
        ev = _mock_llm(ReasoningQualityEvaluator())
        agent_trace = _make_agent_trace()
        result = ev.evaluate(agent_trace)
        assert result.score == 0.9

    def test_prompt_contains_execution_steps(self):
        from amp_evaluation.trace.models import LLMStep, ToolExecutionStep, ToolCallInfo

        ev = ReasoningQualityEvaluator()
        steps = [
            LLMStep(content="I'll search for information.", tool_calls=[ToolCallInfo(id="1", name="search")]),
            ToolExecutionStep(tool_name="search", tool_output={"result": "Paris info"}),
            LLMStep(content="Based on the search, Paris is the capital."),
        ]
        agent_trace = _make_agent_trace(steps=steps)
        prompt = ev.build_prompt(agent_trace)
        assert "search" in prompt.lower()

    def test_builtin_factory(self):
        ev = builtin("reasoning_quality")
        assert ev.name == "reasoning_quality"
        assert ev.level == EvaluationLevel.AGENT


class TestPathEfficiency:
    """Tests for path_efficiency evaluator."""

    def test_name_and_level(self):
        ev = PathEfficiencyEvaluator()
        assert ev.name == "path_efficiency"
        assert ev.level == EvaluationLevel.AGENT

    def test_supports_both_modes(self):
        ev = PathEfficiencyEvaluator()
        assert EvalMode.MONITOR in ev._supported_eval_modes

    def test_evaluate_returns_result(self):
        ev = _mock_llm(PathEfficiencyEvaluator())
        agent_trace = _make_agent_trace()
        result = ev.evaluate(agent_trace)
        assert result.score == 0.9

    def test_prompt_includes_total_steps(self):
        from amp_evaluation.trace.models import LLMStep, ToolExecutionStep, ToolCallInfo

        ev = PathEfficiencyEvaluator()
        steps = [
            LLMStep(content="Searching.", tool_calls=[ToolCallInfo(id="1", name="search")]),
            ToolExecutionStep(tool_name="search", tool_output={"result": "info"}),
        ]
        agent_trace = _make_agent_trace(steps=steps)
        prompt = ev.build_prompt(agent_trace)
        assert "Total Steps: 2" in prompt

    def test_builtin_factory(self):
        ev = builtin("path_efficiency")
        assert ev.name == "path_efficiency"
        assert ev.level == EvaluationLevel.AGENT

    def test_tags_include_efficiency(self):
        ev = PathEfficiencyEvaluator()
        assert "efficiency" in ev.tags


class TestErrorRecovery:
    """Tests for error_recovery evaluator."""

    def test_name_and_level(self):
        ev = ErrorRecoveryEvaluator()
        assert ev.name == "error_recovery"
        assert ev.level == EvaluationLevel.AGENT

    def test_supports_both_modes(self):
        ev = ErrorRecoveryEvaluator()
        assert EvalMode.MONITOR in ev._supported_eval_modes

    def test_skips_when_no_errors_in_trace(self):
        ev = ErrorRecoveryEvaluator()  # default: on_missing_context="skip"
        agent_trace = _make_agent_trace()  # no steps = no errors
        result = ev.evaluate(agent_trace)
        assert result.is_skipped is True
        assert "No errors found" in result.skip_reason

    def test_zero_when_no_errors_and_on_missing_context_zero(self):
        ev = ErrorRecoveryEvaluator(on_missing_context="zero")
        agent_trace = _make_agent_trace()
        result = ev.evaluate(agent_trace)
        assert result.score == 0.0
        assert result.passed is False

    def test_evaluates_when_errors_present(self):
        from amp_evaluation.trace.models import ToolExecutionStep

        ev = _mock_llm(ErrorRecoveryEvaluator())
        steps = [
            ToolExecutionStep(tool_name="api_call", error="Connection timeout"),
        ]
        agent_trace = _make_agent_trace(steps=steps)
        result = ev.evaluate(agent_trace)
        assert result.score == 0.9

    def test_prompt_shows_errors(self):
        from amp_evaluation.trace.models import ToolExecutionStep

        ev = ErrorRecoveryEvaluator()
        steps = [
            ToolExecutionStep(tool_name="api_call", error="Connection timeout"),
        ]
        agent_trace = _make_agent_trace(steps=steps)
        prompt = ev.build_prompt(agent_trace)
        assert "Connection timeout" in prompt
        assert "api_call" in prompt

    def test_builtin_factory(self):
        ev = builtin("error_recovery")
        assert ev.name == "error_recovery"
        assert ev.level == EvaluationLevel.AGENT

    def test_on_missing_context_param_validation(self):
        with pytest.raises(Exception):
            ErrorRecoveryEvaluator(on_missing_context="invalid_value")


# ============================================================================
# SECTION 8: GLOBAL MODEL CONFIG
# ============================================================================


class TestGlobalModelConfig:
    """Test that evaluators respect AMP_LLM_JUDGE_DEFAULT_MODEL config."""

    def test_default_model_is_gpt4o_mini(self):
        ev = HelpfulnessEvaluator()
        # Default should be gpt-4o-mini (or from global config if set)
        assert isinstance(ev.model, str)
        assert len(ev.model) > 0

    def test_per_evaluator_model_override_wins(self):
        """Explicit model kwarg always wins over global config."""
        ev = HelpfulnessEvaluator(model="gpt-4o")
        assert ev.model == "gpt-4o"

    def test_global_config_used_when_no_explicit_model(self):
        """When model is not passed, global config default_model is used."""
        from amp_evaluation import config as cfg

        # Save original
        original_config = cfg._config

        try:
            # Set a custom global config
            from amp_evaluation.config import Config, LLMJudgeConfig

            custom_llm_judge = LLMJudgeConfig(default_model="claude-opus-4-6")
            # Create a Config with the custom llm_judge config
            cfg._config = Config(llm_judge=custom_llm_judge)

            ev = HelpfulnessEvaluator()  # no model kwarg
            assert ev.model == "claude-opus-4-6"
        finally:
            # Restore original config
            cfg._config = original_config

    def test_explicit_model_overrides_global_config(self):
        """Explicit model always wins even when global config is set."""
        from amp_evaluation import config as cfg

        original_config = cfg._config
        try:
            from amp_evaluation.config import Config, LLMJudgeConfig

            cfg._config = Config(llm_judge=LLMJudgeConfig(default_model="gpt-4o"))

            ev = HelpfulnessEvaluator(model="gpt-4o-mini")
            assert ev.model == "gpt-4o-mini"  # explicit wins
        finally:
            cfg._config = original_config

    def test_builtin_factory_model_override(self):
        ev = builtin("coherence", model="gpt-4o")
        assert ev.model == "gpt-4o"


# ============================================================================
# SECTION 9: PARAM VALIDATION
# ============================================================================


class TestParamValidation:
    """Test that Param constraints are enforced on invalid inputs."""

    def test_on_missing_context_invalid_value_raises(self):
        with pytest.raises(Exception):
            FaithfulnessEvaluator(on_missing_context="ignore")

    def test_unknown_param_raises(self):
        with pytest.raises(TypeError):
            HelpfulnessEvaluator(nonexistent_param=True)


# ============================================================================
# SECTION 10: EVALUATOR INFO METADATA
# ============================================================================


class TestEvaluatorInfo:
    """Test EvaluatorInfo metadata is correct."""

    def test_helpfulness_info(self):
        ev = HelpfulnessEvaluator()
        info = ev.info
        assert info.name == "helpfulness"
        assert info.level == "trace"
        assert "monitor" in info.modes

    def test_clarity_info(self):
        ev = ClarityEvaluator()
        info = ev.info
        assert info.name == "clarity"
        assert info.level == "trace"

    def test_accuracy_info(self):
        ev = AccuracyEvaluator()
        info = ev.info
        assert info.name == "accuracy"
        assert info.level == "trace"
        assert "correctness" in ev.tags

    def test_semantic_similarity_info(self):
        ev = SemanticSimilarityEvaluator()
        info = ev.info
        assert info.name == "semantic_similarity"
        assert "experiment" in info.modes
        assert "monitor" not in info.modes

    def test_goal_clarity_info(self):
        ev = GoalClarityEvaluator()
        info = ev.info
        assert info.name == "goal_clarity"
        assert info.level == "agent"

    def test_reasoning_quality_info(self):
        ev = ReasoningQualityEvaluator()
        info = ev.info
        assert info.name == "reasoning_quality"
        assert info.level == "agent"

    def test_path_efficiency_info(self):
        ev = PathEfficiencyEvaluator()
        info = ev.info
        assert info.name == "path_efficiency"
        assert info.level == "agent"

    def test_error_recovery_info(self):
        ev = ErrorRecoveryEvaluator()
        info = ev.info
        assert info.name == "error_recovery"
        assert info.level == "agent"

    def test_coherence_info_is_llm_level(self):
        ev = CoherenceEvaluator()
        info = ev.info
        assert info.name == "coherence"
        assert info.level == "llm"

    def test_safety_info_is_llm_level(self):
        ev = SafetyEvaluator()
        info = ev.info
        assert info.name == "safety"
        assert info.level == "llm"

    def test_faithfulness_config_schema_includes_on_missing_context(self):
        ev = FaithfulnessEvaluator()
        schema = ev._extract_config_schema()
        param_names = [s["key"] for s in schema]
        assert "on_missing_context" in param_names

    def test_safety_config_schema_includes_context(self):
        ev = SafetyEvaluator()
        schema = ev._extract_config_schema()
        param_names = [s["key"] for s in schema]
        assert "context" in param_names


# ============================================================================
# SECTION 11: TAG TAXONOMY
# ============================================================================


class TestTagTaxonomy:
    """Test that all evaluators follow the tagging taxonomy."""

    ALL_LLM_JUDGE_CLASSES = [
        HelpfulnessEvaluator,
        ClarityEvaluator,
        AccuracyEvaluator,
        CoherenceEvaluator,
        CompletenessEvaluator,
        ConcisenessEvaluator,
        FaithfulnessEvaluator,
        ContextRelevanceEvaluator,
        SafetyEvaluator,
        ToneEvaluator,
        InstructionFollowingEvaluator,
        RelevanceEvaluator,
        SemanticSimilarityEvaluator,
        HallucinationEvaluator,
        GoalClarityEvaluator,
        ReasoningQualityEvaluator,
        PathEfficiencyEvaluator,
        ErrorRecoveryEvaluator,
    ]

    def test_all_evaluators_have_builtin_tag(self):
        """Every LLM-judge evaluator should have 'builtin' as first tag."""
        for cls in self.ALL_LLM_JUDGE_CLASSES:
            ev = cls()
            assert ev.tags[0] == "builtin", f"{ev.name}: first tag should be 'builtin', got '{ev.tags[0]}'"

    def test_all_evaluators_have_llm_judge_tag(self):
        """Every LLM-judge evaluator should have 'llm-judge' as second tag."""
        for cls in self.ALL_LLM_JUDGE_CLASSES:
            ev = cls()
            assert ev.tags[1] == "llm-judge", f"{ev.name}: second tag should be 'llm-judge', got '{ev.tags[1]}'"

    def test_conciseness_has_dual_aspect_tags(self):
        ev = ConcisenessEvaluator()
        assert "quality" in ev.tags
        assert "efficiency" in ev.tags

    def test_hallucination_has_dual_aspect_tags(self):
        ev = HallucinationEvaluator()
        assert "correctness" in ev.tags
        assert "safety" in ev.tags

    def test_all_prompts_include_evaluation_steps(self):
        """All prompts should include structured Evaluation Steps."""
        trace = _make_trace()
        llm_span = _make_llm_span()
        agent_trace = _make_agent_trace()
        task = Task(task_id="t1", name="test", description="test", input="test", expected_output="expected")

        for cls, arg in [
            (HelpfulnessEvaluator, trace),
            (ClarityEvaluator, trace),
            (AccuracyEvaluator, trace),
            (CompletenessEvaluator, trace),
            (RelevanceEvaluator, trace),
            (CoherenceEvaluator, llm_span),
            (ConcisenessEvaluator, llm_span),
            (SafetyEvaluator, llm_span),
            (ToneEvaluator, llm_span),
            (GoalClarityEvaluator, agent_trace),
            (ReasoningQualityEvaluator, agent_trace),
            (PathEfficiencyEvaluator, agent_trace),
        ]:
            ev = cls()
            prompt = ev.build_prompt(arg)
            assert "Evaluation Steps:" in prompt, f"{ev.name}: prompt missing 'Evaluation Steps:'"

    def test_all_prompts_include_5_point_rubric(self):
        """All prompts should include 5-point scoring rubric."""
        trace = _make_trace()
        llm_span = _make_llm_span()
        agent_trace = _make_agent_trace()

        for cls, arg in [
            (HelpfulnessEvaluator, trace),
            (ClarityEvaluator, trace),
            (AccuracyEvaluator, trace),
            (CompletenessEvaluator, trace),
            (RelevanceEvaluator, trace),
            (CoherenceEvaluator, llm_span),
            (ConcisenessEvaluator, llm_span),
            (SafetyEvaluator, llm_span),
            (ToneEvaluator, llm_span),
            (GoalClarityEvaluator, agent_trace),
            (ReasoningQualityEvaluator, agent_trace),
            (PathEfficiencyEvaluator, agent_trace),
        ]:
            ev = cls()
            prompt = ev.build_prompt(arg)
            for anchor in ["0.0", "0.25", "0.5", "0.75", "1.0"]:
                assert anchor in prompt, f"{ev.name}: prompt missing rubric anchor {anchor}"

    def test_total_evaluator_count(self):
        """There should be exactly 18 LLM-judge evaluators."""
        assert len(self.ALL_LLM_JUDGE_CLASSES) == 18


# ============================================================================
# SECTION 12: NAME UNIQUENESS
# ============================================================================


class TestNameUniqueness:
    """Test that duplicate evaluator names are detected."""

    def test_validate_unique_evaluator_names_passes_on_unique(self):
        from amp_evaluation.evaluators.base import validate_unique_evaluator_names

        evaluators = [HelpfulnessEvaluator(), SafetyEvaluator(), CoherenceEvaluator()]
        # Should not raise
        validate_unique_evaluator_names(evaluators)

    def test_validate_unique_evaluator_names_raises_on_duplicates(self):
        from amp_evaluation.evaluators.base import validate_unique_evaluator_names

        evaluators = [HelpfulnessEvaluator(), HelpfulnessEvaluator()]
        with pytest.raises(ValueError, match="Duplicate evaluator name"):
            validate_unique_evaluator_names(evaluators)


# ============================================================================
# SECTION 13: SCORE RANGE AND POLARITY ENFORCEMENT
# ============================================================================


class TestScoreRangeAndPolarity:
    """Test that score range and polarity conventions are enforced and documented."""

    def test_output_instructions_state_worst_best_polarity(self):
        """_OUTPUT_INSTRUCTIONS must remind the LLM that 0.0 is worst and 1.0 is best."""
        from amp_evaluation.evaluators.base import LLMAsJudgeEvaluator

        instructions = LLMAsJudgeEvaluator._OUTPUT_INSTRUCTIONS
        assert "0.0" in instructions
        assert "1.0" in instructions
        assert "worst" in instructions.lower() or "best" in instructions.lower()

    def test_output_instructions_appended_to_every_prompt(self):
        """Every call to evaluate() must include the output instructions in the prompt sent to LLM."""
        from unittest.mock import MagicMock
        import json

        mock_response = MagicMock()
        mock_response.choices = [MagicMock()]
        mock_response.choices[0].message.content = json.dumps({"score": 0.8, "explanation": "ok"})

        with patch("litellm.completion", return_value=mock_response) as mock_completion:
            ev = HelpfulnessEvaluator()
            ev.evaluate(_make_trace())

        prompt_sent = mock_completion.call_args[1]["messages"][0]["content"]
        assert LLMAsJudgeEvaluator._OUTPUT_INSTRUCTIONS.strip() in prompt_sent

    def test_all_prompt_rubrics_have_0_as_worst(self):
        """Every evaluator prompt rubric must have 0.0 anchored to the worst outcome."""
        trace = _make_trace()
        llm_span = _make_llm_span()
        agent_trace = _make_agent_trace()

        bad_words = [
            "worst",
            "bad",
            "fail",
            "poor",
            "unsafe",
            "hallucin",
            "no ",
            "none",
            "incoher",
            "incomprehensible",
            "verbose",
            "padd",
            "irrelevant",
            "ignor",
            "mismatch",
            "fabricat",
            "not helpful",
            "not help",
            "wrong",
            "entirely",
            "significant",
            "random",
            "illogical",
            "stuck",
            "disorganized",
        ]

        for cls, arg in [
            (HelpfulnessEvaluator, trace),
            (ClarityEvaluator, trace),
            (AccuracyEvaluator, trace),
            (CompletenessEvaluator, trace),
            (RelevanceEvaluator, trace),
            (HallucinationEvaluator, trace),
            (CoherenceEvaluator, llm_span),
            (ConcisenessEvaluator, llm_span),
            (SafetyEvaluator, llm_span),
            (ToneEvaluator, llm_span),
            (GoalClarityEvaluator, agent_trace),
            (ReasoningQualityEvaluator, agent_trace),
            (PathEfficiencyEvaluator, agent_trace),
        ]:
            ev = cls()
            prompt = ev.build_prompt(arg).lower()
            # Find the line containing "0.0" in the rubric
            rubric_lines = [l for l in prompt.split("\n") if "0.0" in l and "=" in l]
            assert rubric_lines, f"{ev.name}: no '0.0 =' line found in prompt rubric"
            rubric_0 = " ".join(rubric_lines).lower()
            assert any(w in rubric_0 for w in bad_words), (
                f"{ev.name}: 0.0 rubric line doesn't describe a bad outcome: {rubric_lines}"
            )

    def test_all_prompt_rubrics_have_1_as_best(self):
        """Every evaluator prompt rubric must have 1.0 anchored to the best outcome."""
        trace = _make_trace()
        llm_span = _make_llm_span()
        agent_trace = _make_agent_trace()

        good_words = [
            "best",
            "excellent",
            "perfect",
            "fully",
            "complete",
            "safe",
            "no hallucin",
            "no detectable",
            "coherent",
            "concise",
            "clear",
            "optimal",
            "accurate",
            "helpful",
            "every",
            "all ",
            "grounded",
            "well-suited",
            "appropriately",
        ]

        for cls, arg in [
            (HelpfulnessEvaluator, trace),
            (ClarityEvaluator, trace),
            (AccuracyEvaluator, trace),
            (CompletenessEvaluator, trace),
            (RelevanceEvaluator, trace),
            (HallucinationEvaluator, trace),
            (CoherenceEvaluator, llm_span),
            (ConcisenessEvaluator, llm_span),
            (SafetyEvaluator, llm_span),
            (ToneEvaluator, llm_span),
            (GoalClarityEvaluator, agent_trace),
            (ReasoningQualityEvaluator, agent_trace),
            (PathEfficiencyEvaluator, agent_trace),
        ]:
            ev = cls()
            prompt = ev.build_prompt(arg).lower()
            rubric_lines = [l for l in prompt.split("\n") if "1.0" in l and "=" in l]
            assert rubric_lines, f"{ev.name}: no '1.0 =' line found in prompt rubric"
            rubric_1 = " ".join(rubric_lines).lower()
            assert any(w in rubric_1 for w in good_words), (
                f"{ev.name}: 1.0 rubric line doesn't describe a good outcome: {rubric_lines}"
            )


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
