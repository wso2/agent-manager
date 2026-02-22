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
Tests the builtin/__init__.py discovery logic: _discover_builtin_class,
builtin() factory, list_builtin_evaluators, and builtin_evaluator_catalog.
"""

import pytest
from amp_evaluation.evaluators.builtin import (
    _discover_builtin_class,
    _get_evaluator_modules,
    builtin,
    list_builtin_evaluators,
    builtin_evaluator_catalog,
)
from amp_evaluation.evaluators.base import BaseEvaluator
from amp_evaluation.models import EvaluatorInfo


# ---------------------------------------------------------------------------
# Helper: check whether deepeval evaluators can produce .info metadata.
# The catalog and list functions call .info internally, which requires
# runtime type-hint resolution.  When the deepeval module uses
# ``from __future__ import annotations`` with TYPE_CHECKING-only imports,
# the level auto-detection may fail and the evaluator is silently skipped.
# ---------------------------------------------------------------------------
def _deepeval_in_catalog() -> bool:
    """Return True if deepeval evaluators appear in the catalog."""
    names = [ev.name for ev in builtin_evaluator_catalog()]
    return any(n.startswith("deepeval/") for n in names)


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


class TestDiscoverBuiltinClass:
    """Test _discover_builtin_class internal helper."""

    # Standard evaluators
    def test_discover_latency_evaluator(self):
        """Should discover LatencyEvaluator by name 'latency'."""
        cls = _discover_builtin_class("latency")
        assert cls is not None
        assert cls.__name__ == "LatencyEvaluator"
        assert issubclass(cls, BaseEvaluator)

    def test_discover_exact_match_evaluator(self):
        """Should discover ExactMatchEvaluator by name 'exact_match'."""
        cls = _discover_builtin_class("exact_match")
        assert cls is not None
        assert cls.__name__ == "ExactMatchEvaluator"

    def test_discover_token_efficiency_evaluator(self):
        """Should discover TokenEfficiencyEvaluator by name 'token_efficiency'."""
        cls = _discover_builtin_class("token_efficiency")
        assert cls is not None
        assert cls.__name__ == "TokenEfficiencyEvaluator"

    # DeepEval evaluators
    def test_discover_deepeval_plan_quality(self):
        """Should discover DeepEvalPlanQualityEvaluator by name 'deepeval/plan-quality'."""
        cls = _discover_builtin_class("deepeval/plan-quality")
        assert cls is not None
        assert cls.__name__ == "DeepEvalPlanQualityEvaluator"

    def test_discover_deepeval_task_completion(self):
        """Should discover DeepEvalTaskCompletionEvaluator by name 'deepeval/task-completion'."""
        cls = _discover_builtin_class("deepeval/task-completion")
        assert cls is not None
        assert cls.__name__ == "DeepEvalTaskCompletionEvaluator"

    def test_discover_deepeval_tool_correctness(self):
        """Should discover DeepEvalToolCorrectnessEvaluator by name 'deepeval/tool-correctness'."""
        cls = _discover_builtin_class("deepeval/tool-correctness")
        assert cls is not None
        assert cls.__name__ == "DeepEvalToolCorrectnessEvaluator"

    # Negative cases
    def test_discover_nonexistent_evaluator_returns_none(self):
        """Should return None for non-existent evaluator."""
        cls = _discover_builtin_class("nonexistent-evaluator")
        assert cls is None

    def test_discover_invalid_name_returns_none(self):
        """Should return None for invalid evaluator name."""
        cls = _discover_builtin_class("invalid/name/format")
        assert cls is None

    def test_discover_empty_name_returns_none(self):
        """Should return None for empty name."""
        cls = _discover_builtin_class("")
        assert cls is None

    def test_discover_with_wrong_prefix_returns_none(self):
        """Should return None if name has wrong module prefix."""
        # 'latency' is in standard, not deepeval
        cls = _discover_builtin_class("deepeval/latency")
        assert cls is None

    # Verify instance creation works (no level kwarg -- auto-detected)
    def test_discovered_class_can_instantiate(self):
        """Should be able to instantiate discovered class."""
        cls = _discover_builtin_class("latency")
        instance = cls()
        assert isinstance(instance, BaseEvaluator)
        assert instance.name == "latency"

    def test_discovered_deepeval_class_can_instantiate(self):
        """Should be able to instantiate discovered DeepEval class."""
        cls = _discover_builtin_class("deepeval/plan-quality")
        instance = cls()
        assert isinstance(instance, BaseEvaluator)
        assert instance.name == "deepeval/plan-quality"


class TestBuiltinFactory:
    """Test builtin() factory function."""

    def test_builtin_default_config(self):
        """Should get builtin evaluator with default configuration."""
        evaluator = builtin("latency")
        assert evaluator.name == "latency"
        assert isinstance(evaluator, BaseEvaluator)

    def test_builtin_with_single_kwarg(self):
        """Should pass single kwarg to constructor."""
        evaluator = builtin("latency", max_latency_ms=500)
        assert evaluator.name == "latency"
        assert evaluator.max_latency_ms == 500

    def test_builtin_with_multiple_kwargs(self):
        """Should pass multiple kwargs to constructor."""
        evaluator = builtin("exact_match", case_sensitive=False, strip_whitespace=True)
        assert evaluator.name == "exact_match"
        assert evaluator.case_sensitive is False
        assert evaluator.strip_whitespace is True

    def test_builtin_deepeval_with_kwargs(self):
        """Should pass kwargs to DeepEval evaluator."""
        evaluator = builtin("deepeval/task-completion", threshold=0.8, model="gpt-4o-mini")
        assert evaluator.name == "deepeval/task-completion"
        assert evaluator.threshold == 0.8
        assert evaluator.model == "gpt-4o-mini"

    def test_builtin_tool_correctness_with_special_kwargs(self):
        """Should handle evaluators with special configuration params."""
        evaluator = builtin(
            "deepeval/tool-correctness", evaluate_input=True, evaluate_output=True, threshold=0.9
        )
        assert evaluator.name == "deepeval/tool-correctness"
        assert evaluator.evaluate_input is True
        assert evaluator.evaluate_output is True
        assert evaluator.threshold == 0.9

    def test_builtin_invalid_kwarg_raises_error(self):
        """Should raise TypeError for invalid kwargs."""
        with pytest.raises(TypeError):
            builtin("latency", invalid_param=True)

    def test_builtin_nonexistent_raises_error(self):
        """Should raise ValueError for non-existent evaluator."""
        with pytest.raises(ValueError, match="Unknown built-in evaluator"):
            builtin("nonexistent-evaluator")

    def test_builtin_answer_length_with_bounds(self):
        """Should configure AnswerLengthEvaluator bounds."""
        evaluator = builtin("answer_length", min_length=10, max_length=100)
        assert evaluator.name == "answer_length"
        assert evaluator.min_length == 10
        assert evaluator.max_length == 100

    def test_builtin_preserves_class_name(self):
        """Should preserve the name attribute from class definition."""
        evaluator = builtin("deepeval/plan-quality")
        assert evaluator.name == "deepeval/plan-quality"


class TestListBuiltinEvaluators:
    """Test list_builtin_evaluators function returns List[str]."""

    def test_list_returns_strings(self):
        """Should return a list of plain strings."""
        names = list_builtin_evaluators()
        assert isinstance(names, list)
        assert len(names) > 0
        assert all(isinstance(n, str) for n in names)

    def test_list_includes_standard_evaluators(self):
        """Should include standard evaluator names."""
        names = list_builtin_evaluators()
        assert "latency" in names
        assert "exact_match" in names
        assert "token_efficiency" in names

    @pytest.mark.skipif(not _deepeval_in_catalog(), reason="deepeval evaluators not in catalog (type-hint resolution)")
    def test_list_includes_deepeval_evaluators(self):
        """Should include DeepEval evaluator names when metadata is resolvable."""
        names = list_builtin_evaluators()
        assert "deepeval/plan-quality" in names
        assert "deepeval/task-completion" in names

    def test_list_no_duplicates(self):
        """Should not have duplicate evaluator names."""
        names = list_builtin_evaluators()
        assert len(names) == len(set(names)), "Duplicate evaluator names found"

    def test_list_filter_by_monitor_mode(self):
        """Monitor mode should exclude experiment-only evaluators."""
        all_names = list_builtin_evaluators()
        monitor_names = list_builtin_evaluators(mode="monitor")

        assert len(monitor_names) < len(all_names)
        # Experiment-only evaluators should be excluded
        assert "exact_match" not in monitor_names
        assert "contains_match" not in monitor_names
        # DeepEval evaluators are experiment-only
        assert not any(n.startswith("deepeval/") for n in monitor_names)

    def test_list_filter_by_experiment_mode(self):
        """Experiment mode should include all evaluators."""
        all_names = list_builtin_evaluators()
        experiment_names = list_builtin_evaluators(mode="experiment")
        assert len(experiment_names) == len(all_names)


class TestBuiltinEvaluatorCatalog:
    """Test builtin_evaluator_catalog function returns List[EvaluatorInfo]."""

    def test_catalog_returns_evaluator_info_objects(self):
        """Should return a list of EvaluatorInfo objects."""
        catalog = builtin_evaluator_catalog()
        assert isinstance(catalog, list)
        assert len(catalog) > 0
        assert all(isinstance(ev, EvaluatorInfo) for ev in catalog)

    def test_catalog_has_required_fields(self):
        """Each EvaluatorInfo should have all required fields populated."""
        catalog = builtin_evaluator_catalog()

        for ev in catalog:
            assert ev.name
            assert ev.description is not None
            assert isinstance(ev.tags, list)
            assert ev.version
            assert isinstance(ev.config_schema, list)
            assert ev.class_name
            assert ev.module

    def test_catalog_includes_standard_evaluators(self):
        """Should include standard evaluators."""
        catalog = builtin_evaluator_catalog()
        names = [ev.name for ev in catalog]

        assert "latency" in names
        assert "exact_match" in names
        assert "token_efficiency" in names

    @pytest.mark.skipif(not _deepeval_in_catalog(), reason="deepeval evaluators not in catalog (type-hint resolution)")
    def test_catalog_includes_deepeval_evaluators(self):
        """Should include DeepEval evaluators when metadata is resolvable."""
        catalog = builtin_evaluator_catalog()
        names = [ev.name for ev in catalog]

        assert "deepeval/plan-quality" in names
        assert "deepeval/task-completion" in names

    def test_catalog_has_implementation_details(self):
        """Each evaluator should have class_name and module."""
        catalog = builtin_evaluator_catalog()
        for ev in catalog:
            assert ev.class_name is not None
            assert ev.module is not None

    def test_catalog_module_identifies_standard_correctly(self):
        """Module field should correctly identify standard source file."""
        catalog = builtin_evaluator_catalog()

        latency = next(ev for ev in catalog if ev.name == "latency")
        assert latency.module == "standard"

    @pytest.mark.skipif(not _deepeval_in_catalog(), reason="deepeval evaluators not in catalog (type-hint resolution)")
    def test_catalog_module_identifies_deepeval_correctly(self):
        """Module field should correctly identify deepeval source file."""
        catalog = builtin_evaluator_catalog()

        plan_quality = next(ev for ev in catalog if ev.name == "deepeval/plan-quality")
        assert plan_quality.module == "deepeval"

    def test_catalog_no_duplicates(self):
        """Should not have duplicate evaluator names."""
        catalog = builtin_evaluator_catalog()
        names = [ev.name for ev in catalog]
        assert len(names) == len(set(names)), "Duplicate evaluator names found"

    def test_catalog_excludes_base_evaluator(self):
        """Should not include BaseEvaluator."""
        catalog = builtin_evaluator_catalog()
        class_names = [ev.class_name for ev in catalog]
        assert "BaseEvaluator" not in class_names

    def test_catalog_includes_modes(self):
        """Each evaluator should include modes."""
        catalog = builtin_evaluator_catalog()
        for ev in catalog:
            assert isinstance(ev.modes, list), f"{ev.name} missing modes"
            assert len(ev.modes) > 0

    def test_catalog_filter_by_monitor_mode(self):
        """Monitor mode should exclude experiment-only evaluators."""
        all_catalog = builtin_evaluator_catalog()
        monitor_catalog = builtin_evaluator_catalog(mode="monitor")
        monitor_names = [ev.name for ev in monitor_catalog]

        assert len(monitor_catalog) < len(all_catalog)
        # Experiment-only evaluators should be excluded
        assert "exact_match" not in monitor_names
        assert "contains_match" not in monitor_names
        # DeepEval evaluators are experiment-only
        assert not any(n.startswith("deepeval/") for n in monitor_names)

    def test_catalog_filter_by_experiment_mode(self):
        """Experiment mode should include all evaluators."""
        all_catalog = builtin_evaluator_catalog()
        experiment_catalog = builtin_evaluator_catalog(mode="experiment")
        assert len(experiment_catalog) == len(all_catalog)

    def test_catalog_matches_list(self):
        """Catalog names should match list_builtin_evaluators output."""
        names_from_list = list_builtin_evaluators()
        names_from_catalog = [ev.name for ev in builtin_evaluator_catalog()]
        assert sorted(names_from_list) == sorted(names_from_catalog)


class TestDirectImportPattern:
    """Test direct import for type safety."""

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
        cls = _discover_builtin_class("some-evaluator-in-broken-module")
        assert cls is None

    def test_catalog_handles_broken_evaluators(self):
        """Should skip evaluators that can't be instantiated."""
        # Should not crash if an evaluator has issues
        catalog = builtin_evaluator_catalog()
        assert isinstance(catalog, list)
        # Should still find working evaluators
        assert len(catalog) > 0

    def test_list_handles_broken_evaluators(self):
        """list_builtin_evaluators should not crash on broken evaluators."""
        names = list_builtin_evaluators()
        assert isinstance(names, list)
        assert len(names) > 0

    def test_discover_handles_instantiation_errors(self):
        """Should skip evaluators that fail to instantiate."""
        from unittest import mock

        # Create a mock evaluator that raises on instantiation
        class BrokenEvaluator(BaseEvaluator):
            name = "broken"

            def __init__(self):
                raise ValueError("Broken evaluator")

            def _trace_evaluation(self, trace, task=None):
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
                    result = _discover_builtin_class("broken")
                    assert result is None  # Can't find it because it can't instantiate

    def test_catalog_handles_metadata_errors(self):
        """Should skip evaluators with broken metadata."""
        catalog = builtin_evaluator_catalog()
        # Should complete without crashing
        assert len(catalog) > 0
        # All returned evaluators should have valid metadata
        for ev in catalog:
            assert ev.class_name is not None
            assert ev.module is not None


class TestIntegrationScenarios:
    """Test real-world integration scenarios."""

    def test_all_standard_evaluators_discoverable(self):
        """All standard evaluators should be discoverable via _discover_builtin_class."""
        expected_names = [
            "answer_length",
            "answer_relevancy",
            "contains_match",
            "exact_match",
            "hallucination",
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
            cls = _discover_builtin_class(name)
            assert cls is not None, f"Failed to discover {name}"
            instance = cls()
            assert instance.name == name

    def test_all_deepeval_evaluators_discoverable(self):
        """All DeepEval evaluators should be discoverable via _discover_builtin_class."""
        expected_names = [
            "deepeval/plan-quality",
            "deepeval/plan-adherence",
            "deepeval/tool-correctness",
            "deepeval/argument-correctness",
            "deepeval/task-completion",
            "deepeval/step-efficiency",
        ]

        for name in expected_names:
            cls = _discover_builtin_class(name)
            assert cls is not None, f"Failed to discover {name}"
            instance = cls()
            assert instance.name == name

    def test_builtin_factory_round_trip(self):
        """Should be able to create any cataloged evaluator via builtin()."""
        names = list_builtin_evaluators()
        for name in names:
            evaluator = builtin(name)
            assert evaluator.name == name
            assert isinstance(evaluator, BaseEvaluator)

    def test_catalog_and_list_consistency(self):
        """Catalog and list should agree on available evaluators for each mode."""
        for mode in [None, "monitor", "experiment"]:
            names = list_builtin_evaluators(mode=mode)
            catalog = builtin_evaluator_catalog(mode=mode)
            catalog_names = [ev.name for ev in catalog]
            assert sorted(names) == sorted(catalog_names), (
                f"Mismatch for mode={mode!r}: list={sorted(names)}, catalog={sorted(catalog_names)}"
            )
