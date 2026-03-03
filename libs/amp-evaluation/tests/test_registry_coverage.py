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
Tests for the evaluator decorator, FunctionEvaluator config, discovery,
built-in evaluator factory/catalog, and mode auto-detection.
"""

import pytest
import types
import sys
from pathlib import Path
from typing import Optional

sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from amp_evaluation.registry import evaluator, discover_evaluators
from amp_evaluation.evaluators.base import BaseEvaluator
from amp_evaluation.evaluators.params import EvaluationLevel, Param, EvalMode
from amp_evaluation.models import EvalResult, EvaluatorInfo
from amp_evaluation.trace import Trace
from amp_evaluation.trace.models import AgentTrace, LLMSpan
from amp_evaluation.dataset import Task
from amp_evaluation.evaluators.builtin import builtin, list_builtin_evaluators, builtin_evaluator_catalog


# =============================================================================
# 1. TestEvaluatorDecoratorCoverage
# =============================================================================


class TestEvaluatorDecoratorCoverage:
    """Tests for the @evaluator decorator behaviour."""

    def test_decorator_returns_base_evaluator_instance(self):
        """The @evaluator decorator must return a BaseEvaluator instance."""

        @evaluator("dec-instance-check")
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        assert isinstance(my_eval, BaseEvaluator)

    def test_decorator_auto_detects_trace_level(self):
        """Level should be TRACE when the first param is typed as Trace."""

        @evaluator("trace-level-check")
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        assert my_eval.level == EvaluationLevel.TRACE

    def test_decorator_auto_detects_agent_level(self):
        """Level should be AGENT when the first param is typed as AgentTrace."""

        @evaluator("agent-level-check")
        def my_eval(agent_trace: AgentTrace) -> EvalResult:
            return EvalResult(score=1.0)

        assert my_eval.level == EvaluationLevel.AGENT

    def test_decorator_auto_detects_llm_level(self):
        """Level should be LLM when the first param is typed as LLMSpan."""

        @evaluator("llm-level-check")
        def my_eval(llm_span: LLMSpan) -> EvalResult:
            return EvalResult(score=1.0)

        assert my_eval.level == EvaluationLevel.LLM

    def test_decorator_missing_type_hint_raises(self):
        """A TypeError must be raised when the first param has no type hint."""

        @evaluator("no-hint")
        def my_eval(trace) -> EvalResult:
            return EvalResult(score=1.0)

        with pytest.raises(TypeError, match="must have a type hint"):
            _ = my_eval.level

    def test_decorator_unsupported_type_hint_raises(self):
        """A TypeError must be raised for an unsupported first-param type."""

        @evaluator("bad-hint")
        def my_eval(data: str) -> EvalResult:
            return EvalResult(score=1.0)

        with pytest.raises(TypeError, match="unsupported type"):
            _ = my_eval.level

    def test_decorator_with_metadata(self):
        """All metadata fields (description, tags, version) should be set."""

        @evaluator(
            "meta-eval",
            description="A meta evaluator",
            tags=["quality", "test"],
            version="2.0",
        )
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        assert my_eval.name == "meta-eval"
        assert my_eval.description == "A meta evaluator"
        assert my_eval.tags == ["quality", "test"]
        assert my_eval.version == "2.0"


# =============================================================================
# 2. TestFunctionEvaluatorConfig
# =============================================================================


class TestFunctionEvaluatorConfig:
    """Tests for FunctionEvaluator config (with_config, Param defaults, schema)."""

    def test_with_config_creates_copy(self):
        """with_config() should return a new instance without mutating the original."""

        @evaluator("cfg-copy")
        def my_eval(
            trace: Trace,
            threshold: float = Param(default=0.7, description="Threshold", min=0, max=1),
        ) -> EvalResult:
            return EvalResult(score=1.0)

        copy = my_eval.with_config(threshold=0.9)

        # Must be a new instance
        assert copy is not my_eval

        # Original unchanged
        assert my_eval._config["threshold"] == 0.7

        # Copy has new value
        assert copy._config["threshold"] == 0.9

    def test_with_config_invalid_key_raises(self):
        """with_config() should raise TypeError for an unknown config key."""

        @evaluator("cfg-bad-key")
        def my_eval(
            trace: Trace,
            threshold: float = Param(default=0.7, description="Threshold"),
        ) -> EvalResult:
            return EvalResult(score=1.0)

        with pytest.raises(TypeError, match="Unknown config parameter"):
            my_eval.with_config(nonexistent_key=42)

    def test_function_param_default_values(self):
        """Param defaults should be correctly stored in _config."""

        @evaluator("cfg-defaults")
        def my_eval(
            trace: Trace,
            threshold: float = Param(default=0.5, description="Score threshold"),
            max_length: int = Param(default=1000, description="Max length"),
        ) -> EvalResult:
            return EvalResult(score=1.0)

        assert my_eval._config["threshold"] == 0.5
        assert my_eval._config["max_length"] == 1000

    def test_function_param_schema_extraction(self):
        """info.config_schema should contain entries for each Param."""

        @evaluator("cfg-schema")
        def my_eval(
            trace: Trace,
            threshold: float = Param(default=0.7, description="Threshold", min=0, max=1),
            model: str = Param(default="gpt-4", description="Model name"),
        ) -> EvalResult:
            return EvalResult(score=1.0)

        schema = my_eval.info.config_schema
        keys = [entry["key"] for entry in schema]

        assert "threshold" in keys
        assert "model" in keys

        # Verify threshold entry details
        threshold_entry = next(e for e in schema if e["key"] == "threshold")
        assert threshold_entry["description"] == "Threshold"
        assert threshold_entry["min"] == 0
        assert threshold_entry["max"] == 1
        assert threshold_entry["default"] == 0.7


# =============================================================================
# 3. TestDiscoverEvaluatorsCoverage
# =============================================================================


class TestDiscoverEvaluatorsCoverage:
    """Tests for discover_evaluators() scanning a module."""

    def test_discover_finds_function_evaluators(self):
        """discover_evaluators should find @evaluator-decorated functions."""
        mod = types.ModuleType("fake_mod_func")

        @evaluator("disc-func")
        def disc_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        mod.disc_eval = disc_eval
        sys.modules["fake_mod_func"] = mod

        try:
            found = discover_evaluators(mod)
            names = [e.name for e in found]
            assert "disc-func" in names
        finally:
            del sys.modules["fake_mod_func"]

    def test_discover_finds_class_evaluators(self):
        """discover_evaluators should find BaseEvaluator *instances*."""
        mod = types.ModuleType("fake_mod_cls")

        class MyClassEval(BaseEvaluator):
            name = "disc-class"

            def evaluate(self, trace: Trace) -> EvalResult:
                return EvalResult(score=1.0)

        instance = MyClassEval()
        mod.my_instance = instance
        sys.modules["fake_mod_cls"] = mod

        try:
            found = discover_evaluators(mod)
            names = [e.name for e in found]
            assert "disc-class" in names
        finally:
            del sys.modules["fake_mod_cls"]

    def test_discover_auto_instantiates_classes(self):
        """discover_evaluators should auto-instantiate BaseEvaluator subclasses."""
        mod = types.ModuleType("fake_mod_no_inst")

        class UninstantiatedEval(BaseEvaluator):
            name = "auto-instantiated"

            def evaluate(self, trace: Trace) -> EvalResult:
                return EvalResult(score=1.0)

        # Attach the class itself, not an instance
        mod.UninstantiatedEval = UninstantiatedEval
        sys.modules["fake_mod_no_inst"] = mod

        try:
            found = discover_evaluators(mod)
            names = [e.name for e in found]
            assert "auto-instantiated" in names
        finally:
            del sys.modules["fake_mod_no_inst"]


# =============================================================================
# 4. TestBuiltinEvaluators
# =============================================================================


class TestBuiltinEvaluators:
    """Tests for the built-in evaluator factory, listing, and catalog."""

    def test_builtin_factory_creates_evaluator(self):
        """builtin('latency') should return a BaseEvaluator instance."""
        ev = builtin("latency")
        assert isinstance(ev, BaseEvaluator)
        assert ev.name == "latency"

    def test_builtin_with_config(self):
        """builtin('latency', max_latency_ms=5000) should apply config."""
        ev = builtin("latency", max_latency_ms=5000)
        assert isinstance(ev, BaseEvaluator)
        assert ev.max_latency_ms == 5000

    def test_builtin_unknown_raises_value_error(self):
        """builtin() should raise ValueError for an unknown evaluator name."""
        with pytest.raises(ValueError, match="Unknown built-in evaluator"):
            builtin("this-evaluator-does-not-exist-xyz")

    def test_list_builtin_returns_strings(self):
        """list_builtin_evaluators() should return a list of strings."""
        names = list_builtin_evaluators()
        assert isinstance(names, list)
        assert len(names) > 0
        assert all(isinstance(n, str) for n in names)

    def test_builtin_catalog_returns_evaluator_info(self):
        """builtin_evaluator_catalog() should return a list of EvaluatorInfo."""
        catalog = builtin_evaluator_catalog()
        assert isinstance(catalog, list)
        assert len(catalog) > 0
        assert all(isinstance(info, EvaluatorInfo) for info in catalog)

    def test_builtin_catalog_filter_by_mode(self):
        """Passing mode= should filter the catalog to matching evaluators."""
        all_catalog = builtin_evaluator_catalog()
        monitor_catalog = builtin_evaluator_catalog(mode="monitor")

        # Monitor-filtered should be a (non-strict) subset
        assert len(monitor_catalog) <= len(all_catalog)

        # Every returned entry must support "monitor"
        for info in monitor_catalog:
            assert "monitor" in info.modes


# =============================================================================
# 5. TestModeAutoDetection
# =============================================================================


class TestModeAutoDetection:
    """Tests for automatic eval-mode detection from function signatures."""

    def test_no_task_param_both_modes(self):
        """An evaluator without a task param should support both modes."""

        @evaluator("both-no-task")
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        modes = my_eval.info.modes
        assert EvalMode.EXPERIMENT.value in modes
        assert EvalMode.MONITOR.value in modes

    def test_required_task_experiment_only(self):
        """An evaluator with a required task param should be experiment-only."""

        @evaluator("exp-only")
        def my_eval(trace: Trace, task: Task) -> EvalResult:
            return EvalResult(score=1.0)

        modes = my_eval.info.modes
        assert EvalMode.EXPERIMENT.value in modes
        assert EvalMode.MONITOR.value not in modes

    def test_optional_task_both_modes(self):
        """An evaluator with an optional task param should support both modes."""

        @evaluator("both-opt-task")
        def my_eval(trace: Trace, task: Optional[Task] = None) -> EvalResult:
            return EvalResult(score=1.0)

        modes = my_eval.info.modes
        assert EvalMode.EXPERIMENT.value in modes
        assert EvalMode.MONITOR.value in modes
