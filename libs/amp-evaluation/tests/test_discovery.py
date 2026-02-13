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
Comprehensive tests for the automatic evaluator discovery system.
Tests the builtin/__init__.py discovery logic and integration with registry.
"""

import pytest
from amp_evaluation.evaluators.builtin import discover_evaluator, list_builtin_evaluators, _get_evaluator_modules
from amp_evaluation.evaluators.base import BaseEvaluator


class TestModuleDiscovery:
    """Test automatic module file discovery."""

    def test_get_evaluator_modules_returns_list(self):
        """Should return a list of module names."""
        modules = _get_evaluator_modules()
        assert isinstance(modules, list)
        assert len(modules) > 0

    def test_get_evaluator_modules_finds_standard(self):
        """Should find standard.py module."""
        modules = _get_evaluator_modules()
        assert "standard" in modules

    def test_get_evaluator_modules_finds_deepeval(self):
        """Should find deepeval.py module."""
        modules = _get_evaluator_modules()
        assert "deepeval" in modules

    def test_get_evaluator_modules_excludes_base(self):
        """Should not include base.py."""
        modules = _get_evaluator_modules()
        assert "base" not in modules

    def test_get_evaluator_modules_excludes_init(self):
        """Should not include __init__.py."""
        modules = _get_evaluator_modules()
        assert "__init__" not in modules


class TestDiscoverEvaluator:
    """Test discover_evaluator function."""

    # Standard evaluators
    def test_discover_latency_evaluator(self):
        """Should discover LatencyEvaluator by name 'latency'."""
        cls = discover_evaluator("latency")
        assert cls is not None
        assert cls.__name__ == "LatencyEvaluator"
        assert issubclass(cls, BaseEvaluator)

    def test_discover_exact_match_evaluator(self):
        """Should discover ExactMatchEvaluator by name 'exact_match'."""
        cls = discover_evaluator("exact_match")
        assert cls is not None
        assert cls.__name__ == "ExactMatchEvaluator"

    def test_discover_token_efficiency_evaluator(self):
        """Should discover TokenEfficiencyEvaluator by name 'token_efficiency'."""
        cls = discover_evaluator("token_efficiency")
        assert cls is not None
        assert cls.__name__ == "TokenEfficiencyEvaluator"

    # DeepEval evaluators
    def test_discover_deepeval_plan_quality(self):
        """Should discover DeepEvalPlanQualityEvaluator by name 'deepeval/plan-quality'."""
        cls = discover_evaluator("deepeval/plan-quality")
        assert cls is not None
        assert cls.__name__ == "DeepEvalPlanQualityEvaluator"

    def test_discover_deepeval_task_completion(self):
        """Should discover DeepEvalTaskCompletionEvaluator by name 'deepeval/task-completion'."""
        cls = discover_evaluator("deepeval/task-completion")
        assert cls is not None
        assert cls.__name__ == "DeepEvalTaskCompletionEvaluator"

    def test_discover_deepeval_tool_correctness(self):
        """Should discover DeepEvalToolCorrectnessEvaluator by name 'deepeval/tool-correctness'."""
        cls = discover_evaluator("deepeval/tool-correctness")
        assert cls is not None
        assert cls.__name__ == "DeepEvalToolCorrectnessEvaluator"

    # Negative cases
    def test_discover_nonexistent_evaluator_returns_none(self):
        """Should return None for non-existent evaluator."""
        cls = discover_evaluator("nonexistent-evaluator")
        assert cls is None

    def test_discover_invalid_name_returns_none(self):
        """Should return None for invalid evaluator name."""
        cls = discover_evaluator("invalid/name/format")
        assert cls is None

    def test_discover_empty_name_returns_none(self):
        """Should return None for empty name."""
        cls = discover_evaluator("")
        assert cls is None

    def test_discover_with_wrong_prefix_returns_none(self):
        """Should return None if name has wrong module prefix."""
        # 'latency' is in standard, not deepeval
        cls = discover_evaluator("deepeval/latency")
        assert cls is None

    # Verify instance creation works
    def test_discovered_class_can_instantiate(self):
        """Should be able to instantiate discovered class."""
        cls = discover_evaluator("latency")
        instance = cls()
        assert isinstance(instance, BaseEvaluator)
        assert instance.name == "latency"

    def test_discovered_deepeval_class_can_instantiate(self):
        """Should be able to instantiate discovered DeepEval class."""
        cls = discover_evaluator("deepeval/plan-quality")
        instance = cls()
        assert isinstance(instance, BaseEvaluator)
        assert instance.name == "deepeval/plan-quality"


class TestListBuiltinEvaluators:
    """Test list_builtin_evaluators function."""

    def test_list_returns_list_of_dicts(self):
        """Should return a list of dictionaries."""
        evaluators = list_builtin_evaluators()
        assert isinstance(evaluators, list)
        assert len(evaluators) > 0
        assert all(isinstance(ev, dict) for ev in evaluators)

    def test_list_contains_required_keys(self):
        """Each evaluator dict should have required keys."""
        evaluators = list_builtin_evaluators()
        required_keys = {"name", "description", "tags", "version", "config_schema", "metadata"}

        for ev in evaluators:
            assert all(key in ev for key in required_keys), f"Missing keys in {ev}"

    def test_list_includes_standard_evaluators(self):
        """Should include standard evaluators."""
        evaluators = list_builtin_evaluators()
        names = [ev["name"] for ev in evaluators]

        assert "latency" in names
        assert "exact_match" in names
        assert "token_efficiency" in names

    def test_list_includes_deepeval_evaluators(self):
        """Should include DeepEval evaluators."""
        evaluators = list_builtin_evaluators()
        names = [ev["name"] for ev in evaluators]

        assert "deepeval/plan-quality" in names
        assert "deepeval/task-completion" in names

    def test_list_metadata_is_dict(self):
        """Metadata should be a dictionary."""
        evaluators = list_builtin_evaluators()
        for ev in evaluators:
            assert isinstance(ev["metadata"], dict)

    def test_list_metadata_contains_implementation_details(self):
        """Metadata should contain implementation details."""
        evaluators = list_builtin_evaluators()
        for ev in evaluators:
            assert "class_name" in ev["metadata"]
            assert "module" in ev["metadata"]

    def test_list_module_identifies_correctly(self):
        """Module field should correctly identify source file."""
        evaluators = list_builtin_evaluators()

        # Find a standard evaluator
        latency = next(ev for ev in evaluators if ev["name"] == "latency")
        assert latency["metadata"]["module"] == "standard"

        # Find a deepeval evaluator
        plan_quality = next(ev for ev in evaluators if ev["name"] == "deepeval/plan-quality")
        assert plan_quality["metadata"]["module"] == "deepeval"

    def test_list_no_duplicates(self):
        """Should not have duplicate evaluator names."""
        evaluators = list_builtin_evaluators()
        names = [ev["name"] for ev in evaluators]
        assert len(names) == len(set(names)), "Duplicate evaluator names found"

    def test_list_excludes_base_evaluator(self):
        """Should not include BaseEvaluator."""
        evaluators = list_builtin_evaluators()
        class_names = [ev["metadata"]["class_name"] for ev in evaluators]
        assert "BaseEvaluator" not in class_names


class TestGetEvaluatorWithKwargs:
    """Test get_builtin_evaluator with custom configuration via kwargs."""

    def test_get_evaluator_default_config(self):
        """Should get builtin evaluator with default configuration."""
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator

        evaluator = get_builtin_evaluator("latency")
        assert evaluator.name == "latency"
        assert isinstance(evaluator, BaseEvaluator)

    def test_get_evaluator_with_single_kwarg(self):
        """Should pass single kwarg to constructor."""
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator

        evaluator = get_builtin_evaluator("latency", max_latency_ms=500)
        assert evaluator.name == "latency"
        assert evaluator.max_latency_ms == 500

    def test_get_evaluator_with_multiple_kwargs(self):
        """Should pass multiple kwargs to constructor."""
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator

        evaluator = get_builtin_evaluator("exact_match", case_sensitive=False, strip_whitespace=True)
        assert evaluator.name == "exact_match"
        assert evaluator.case_sensitive is False
        assert evaluator.strip_whitespace is True

    def test_get_evaluator_deepeval_with_kwargs(self):
        """Should pass kwargs to DeepEval evaluator."""
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator

        evaluator = get_builtin_evaluator("deepeval/task-completion", threshold=0.8, model="gpt-4o-mini")
        assert evaluator.name == "deepeval/task-completion"
        assert evaluator.threshold == 0.8
        assert evaluator.model == "gpt-4o-mini"

    def test_get_evaluator_tool_correctness_with_special_kwargs(self):
        """Should handle evaluators with special configuration params."""
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator

        evaluator = get_builtin_evaluator(
            "deepeval/tool-correctness", evaluate_input=True, evaluate_output=True, threshold=0.9
        )
        assert evaluator.name == "deepeval/tool-correctness"
        assert evaluator.evaluate_input is True
        assert evaluator.evaluate_output is True
        assert evaluator.threshold == 0.9

    def test_get_evaluator_invalid_kwarg_raises_error(self):
        """Should raise TypeError for invalid kwargs."""
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator

        with pytest.raises(TypeError):
            get_builtin_evaluator("latency", invalid_param=True)

    def test_get_evaluator_nonexistent_raises_error(self):
        """Should raise ValueError for non-existent evaluator."""
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator

        with pytest.raises(ValueError, match="not a built-in evaluator"):
            get_builtin_evaluator("nonexistent-evaluator")

    def test_get_evaluator_answer_length_with_bounds(self):
        """Should configure AnswerLengthEvaluator bounds."""
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator

        evaluator = get_builtin_evaluator("answer_length", min_length=10, max_length=100)
        assert evaluator.name == "answer_length"
        assert evaluator.min_length == 10
        assert evaluator.max_length == 100


class TestDirectImportPattern:
    """Test Option C - direct import for type safety."""

    def test_direct_import_latency_evaluator(self):
        """Should be able to import and instantiate directly."""
        from amp_evaluation.evaluators.builtin.standard import LatencyEvaluator

        evaluator = LatencyEvaluator(max_latency_ms=1000, use_task_constraint=False)
        assert evaluator.name == "latency"
        assert evaluator.max_latency_ms == 1000
        assert evaluator.use_task_constraint is False

    def test_direct_import_exact_match_evaluator(self):
        """Should be able to import ExactMatchEvaluator directly."""
        from amp_evaluation.evaluators.builtin.standard import ExactMatchEvaluator

        evaluator = ExactMatchEvaluator(case_sensitive=False)
        assert evaluator.name == "exact_match"
        assert evaluator.case_sensitive is False

    def test_direct_import_deepeval_evaluator(self):
        """Should be able to import DeepEval evaluator directly."""
        from amp_evaluation.evaluators.builtin.deepeval import DeepEvalPlanQualityEvaluator

        evaluator = DeepEvalPlanQualityEvaluator(threshold=0.8, model="gpt-4o")
        assert evaluator.name == "deepeval/plan-quality"
        assert evaluator.threshold == 0.8
        assert evaluator.model == "gpt-4o"

    def test_direct_import_has_correct_type(self):
        """Direct import should give specific type, not BaseEvaluator."""
        from amp_evaluation.evaluators.builtin.standard import LatencyEvaluator

        evaluator = LatencyEvaluator()
        assert type(evaluator).__name__ == "LatencyEvaluator"
        assert isinstance(evaluator, BaseEvaluator)


class TestEdgeCases:
    """Test edge cases and error handling."""

    def test_discover_handles_missing_module_gracefully(self):
        """Should handle import errors gracefully."""
        # This should not crash even if a module can't be imported
        cls = discover_evaluator("some-evaluator-in-broken-module")
        assert cls is None

    def test_list_handles_broken_evaluators(self):
        """Should skip evaluators that can't be instantiated."""
        # Should not crash if an evaluator has issues
        evaluators = list_builtin_evaluators()
        assert isinstance(evaluators, list)
        # Should still find working evaluators
        assert len(evaluators) > 0

    def test_get_evaluator_preserves_class_name(self):
        """Should preserve the name attribute from class definition."""
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator

        evaluator = get_builtin_evaluator("deepeval/plan-quality")
        # Name comes from class attribute, not overridden
        assert evaluator.name == "deepeval/plan-quality"

    def test_discover_handles_instantiation_errors(self):
        """Should skip evaluators that fail to instantiate."""
        # Use a mock to simulate an evaluator that can't be instantiated
        from unittest import mock

        # Create a mock evaluator that raises on instantiation
        class BrokenEvaluator(BaseEvaluator):
            name = "broken"

            def __init__(self):
                raise ValueError("Broken evaluator")

            def evaluate(self, observation, task=None):
                pass

        # Create a mock module
        mock_module = mock.MagicMock()
        mock_module.__name__ = "amp_evaluation.evaluators.builtin.broken"
        mock_module.BrokenEvaluator = BrokenEvaluator

        # Mock inspect.getmembers to return our broken evaluator
        with mock.patch("inspect.getmembers", return_value=[("BrokenEvaluator", BrokenEvaluator)]):
            with mock.patch("importlib.import_module", return_value=mock_module):
                with mock.patch("amp_evaluation.evaluators.builtin._get_evaluator_modules", return_value=["broken"]):
                    # Should not crash, should return None
                    result = discover_evaluator("broken")
                    assert result is None  # Can't find it because it can't instantiate

    def test_list_handles_metadata_errors(self):
        """Should skip evaluators with broken metadata."""
        # This tests the exception handling in list_builtin_evaluators
        evaluators = list_builtin_evaluators()
        # Should complete without crashing
        assert len(evaluators) > 0
        # All returned evaluators should have valid metadata
        for ev in evaluators:
            assert isinstance(ev["metadata"], dict)
            assert "class_name" in ev["metadata"]
            assert "module" in ev["metadata"]


class TestIntegrationScenarios:
    """Test real-world integration scenarios."""

    def test_experiment_config_scenario(self):
        """Simulate loading evaluators from experiment config using register_builtin."""
        from amp_evaluation import register_builtin, get_evaluator

        # Typical experiment config structure
        config = [
            {"name": "latency", "config": {"max_latency_ms": 500}},
            {"name": "exact_match", "config": {"case_sensitive": False}},
            {"name": "deepeval/plan-quality", "config": {"threshold": 0.8}},
        ]

        evaluators = []
        for ev_config in config:
            # Register with configuration
            register_builtin(ev_config["name"], **ev_config["config"])
            # Then get from registry
            evaluator = get_evaluator(ev_config["name"])
            evaluators.append(evaluator)

        assert len(evaluators) == 3
        assert evaluators[0].max_latency_ms == 500
        assert evaluators[1].case_sensitive is False
        assert evaluators[2].threshold == 0.8

    def test_mixed_usage_pattern(self):
        """Test using both registry and direct import in same code."""
        from amp_evaluation import register_builtin, get_evaluator
        from amp_evaluation.evaluators.builtin import get_builtin_evaluator
        from amp_evaluation.evaluators.builtin.standard import ExactMatchEvaluator

        # Use register + get for simple cases
        register_builtin("latency")
        eval1 = get_evaluator("latency")

        # Use direct import for complex configuration
        eval2 = ExactMatchEvaluator(case_sensitive=False, strip_whitespace=True)

        # Use get_builtin_evaluator with kwargs for middle ground
        eval3 = get_builtin_evaluator("token_efficiency", max_tokens=1000)

        assert all(isinstance(e, BaseEvaluator) for e in [eval1, eval2, eval3])

    def test_all_standard_evaluators_discoverable(self):
        """All standard evaluators should be discoverable."""
        expected_names = [
            "answer_length",
            "answer_relevancy",
            "contains_match",
            "exact_match",
            "iteration_count",
            "latency",
            "prohibited_content",
            "required_content",
            "required_tools",
            "step_success_rate",
            "token_efficiency",
            "tool_sequence",
        ]

        for name in expected_names:
            cls = discover_evaluator(name)
            assert cls is not None, f"Failed to discover {name}"
            instance = cls()
            assert instance.name == name

    def test_all_deepeval_evaluators_discoverable(self):
        """All DeepEval evaluators should be discoverable."""
        expected_names = [
            "deepeval/plan-quality",
            "deepeval/plan-adherence",
            "deepeval/tool-correctness",
            "deepeval/argument-correctness",
            "deepeval/task-completion",
            "deepeval/step-efficiency",
        ]

        for name in expected_names:
            cls = discover_evaluator(name)
            assert cls is not None, f"Failed to discover {name}"
            instance = cls()
            assert instance.name == name
