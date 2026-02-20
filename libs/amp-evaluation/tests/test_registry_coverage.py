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
Tests for registry coverage - aiming for 90%+.
"""

import pytest
from amp_evaluation.registry import get_registry
from amp_evaluation.evaluators.base import BaseEvaluator
from amp_evaluation.models import EvalResult


@pytest.fixture
def clean_registry():
    """Clean registry for each test."""
    registry = get_registry()
    original = registry._evaluators.copy()
    registry._evaluators.clear()
    yield registry
    registry._evaluators.clear()
    registry._evaluators.update(original)


class TestRegistryOverwrite:
    """Test registry overwrite warning."""

    def test_overwrite_warning(self, clean_registry, caplog):
        """Test that overwriting an evaluator logs a warning."""
        registry = clean_registry

        class TestEval(BaseEvaluator):
            name = "my-eval"

            def _trace_evaluation(self, trace, task=None):
                return EvalResult(score=1.0)

        # Register first time
        registry.register_evaluator(TestEval())

        # Register again - should warn
        import logging

        with caplog.at_level(logging.WARNING):
            registry.register_evaluator(TestEval())

        assert "Overwriting existing evaluator 'my-eval'" in caplog.text


class TestRegistryBuiltin:
    """Test built-in evaluator registration."""

    def test_register_builtin_success(self, clean_registry):
        """Test registering a valid built-in evaluator."""
        registry = clean_registry

        # Register a DeepEval evaluator
        registry.register_builtin("deepeval/plan-quality")

        # Should be in list
        assert "deepeval/plan-quality" in registry.list_evaluators()

    def test_register_builtin_invalid_name(self, clean_registry):
        """Test registering invalid built-in raises ValueError."""
        registry = clean_registry

        with pytest.raises(ValueError, match="is not a built-in evaluator"):
            registry.register_builtin("nonexistent-builtin")

    def test_get_unregistered_builtin_raises_error(self, clean_registry):
        """Test that unregistered builtins raise an error."""
        registry = clean_registry

        # Don't register, just try to get - should fail
        with pytest.raises(ValueError, match="not found"):
            registry.get("deepeval/plan-quality")

        # Should NOT be in list_evaluators (not registered)
        assert "deepeval/plan-quality" not in registry.list_evaluators()


class TestRegistryMetadata:
    """Test metadata retrieval for classes vs instances."""

    def test_get_metadata_for_class_registration(self, clean_registry):
        """Test getting metadata when registered as class (converted to instance)."""
        registry = clean_registry

        class MyEval(BaseEvaluator):
            name = "class-eval"
            description = "A class evaluator"
            tags = ["test"]

            def _trace_evaluation(self, trace, task=None):
                return EvalResult(score=1.0)

        # Register as instance (registry only accepts instances)
        registry.register_evaluator(MyEval())

        metadata = registry.get_metadata("class-eval")

        assert metadata["name"] == "class-eval"
        assert metadata["description"] == "A class evaluator"
        assert "test" in metadata["tags"]

    def test_get_metadata_for_instance_registration(self, clean_registry):
        """Test getting metadata when registered as instance."""
        registry = clean_registry

        class MyEval(BaseEvaluator):
            name = "instance-eval"
            description = "An instance evaluator"

            def _trace_evaluation(self, trace, task=None):
                return EvalResult(score=1.0)

        # Register as INSTANCE
        instance = MyEval()
        registry.register_evaluator(instance)

        metadata = registry.get_metadata("instance-eval")

        assert metadata["name"] == "instance-eval"
        assert metadata["description"] == "An instance evaluator"

    def test_get_metadata_nonexistent_raises_error(self, clean_registry):
        """Test get_metadata for nonexistent evaluator raises error."""
        registry = clean_registry

        with pytest.raises(ValueError, match="Evaluator 'does-not-exist' not found"):
            registry.get_metadata("does-not-exist")


class TestRegistryFiltering:
    """Test list_by_tag and list_by_type."""

    def test_list_by_tag(self, clean_registry):
        """Test filtering evaluators by tag."""
        registry = clean_registry

        class Eval1(BaseEvaluator):
            name = "eval1"
            tags = ["quality", "test"]

            def _trace_evaluation(self, trace, task=None):
                return EvalResult(score=1.0)

        class Eval2(BaseEvaluator):
            name = "eval2"
            tags = ["performance"]

            def _trace_evaluation(self, trace, task=None):
                return EvalResult(score=1.0)

        registry.register_evaluator(Eval1())
        registry.register_evaluator(Eval2())

        quality_evals = registry.list_by_tag("quality")

        assert "eval1" in quality_evals
        assert "eval2" not in quality_evals

    def test_list_by_tag_no_matches(self, clean_registry):
        """Test list_by_tag with no matches returns empty."""
        registry = clean_registry

        class Eval1(BaseEvaluator):
            name = "eval1"
            tags = ["other"]

            def _trace_evaluation(self, trace, task=None):
                return EvalResult(score=1.0)

        registry.register_evaluator(Eval1())

        result = registry.list_by_tag("nonexistent")
        assert result == []


class TestRegistryDecorator:
    """Test the register() decorator."""

    def test_register_decorator_on_class(self, clean_registry):
        """Test using @register decorator on a class."""
        registry = clean_registry

        @registry.register(name="decorated", tags=["test"])
        class DecoratedEval(BaseEvaluator):
            def _trace_evaluation(self, trace, task=None):
                return EvalResult(score=1.0)

        # Should be retrievable
        retrieved = registry.get("decorated")
        # The instance name comes from class name by default
        assert retrieved.name in ["decorated", "DecoratedEval"]

        # Check metadata
        metadata = registry.get_metadata("decorated")
        # Name in metadata should match registration
        assert "name" in metadata
        assert "test" in metadata.get("tags", [])

    def test_register_decorator_on_function_with_metadata(self, clean_registry):
        """Test using @register decorator on a function with metadata."""
        registry = clean_registry

        @registry.register(
            name="func-eval",
            description="A function evaluator",
            tags=["custom", "test"],
            version="2.0",
        )
        def my_evaluator(trajectory, task=None):
            return EvalResult(score=0.9)

        # Should be registered
        assert "func-eval" in registry.list_evaluators()

        # Check metadata
        metadata = registry.get_metadata("func-eval")
        assert metadata["description"] == "A function evaluator"
        assert "custom" in metadata["tags"]
        assert metadata["version"] == "2.0"

    def test_register_decorator_on_invalid_class(self, clean_registry):
        """Test that decorator raises error for non-BaseEvaluator class."""
        registry = clean_registry

        with pytest.raises(TypeError) as exc_info:

            @registry.register(name="invalid")
            class NotAnEvaluator:
                pass

        assert "must inherit from BaseEvaluator" in str(exc_info.value)

    def test_register_decorator_with_metadata_overrides(self, clean_registry):
        """Test that decorator can override instance metadata."""
        registry = clean_registry

        @registry.register(
            name="override-test",
            description="Overridden description",
            tags=["override"],
            version="3.0",
            aggregations=["mean", "max"],
        )
        class OriginalEval(BaseEvaluator):
            description = "Original"
            tags = ["original"]
            version = "1.0"

            def _trace_evaluation(self, trace, task=None):
                return EvalResult(score=1.0)

        # Get the registered instance and verify metadata was overridden
        instance = registry.get("override-test")
        assert instance.description == "Overridden description"
        assert instance.tags == ["override"]
        assert instance.version == "3.0"
        assert instance.aggregations == ["mean", "max"]

    def test_register_decorator_on_function_with_aggregations(self, clean_registry):
        """Test decorator on function with aggregations."""
        registry = clean_registry

        @registry.register(name="func-agg", aggregations=["mean", "p95"])
        def custom_eval(trajectory, task=None):
            return {"score": 0.8}

        retrieved = registry.get("func-agg")
        assert retrieved.aggregations == ["mean", "p95"]


class TestRegistryErrorCases:
    """Test error handling in registry."""

    def test_get_nonexistent_evaluator(self, clean_registry):
        """Test error when getting non-existent evaluator."""
        registry = clean_registry

        with pytest.raises(ValueError) as exc_info:
            registry.get("does-not-exist")

        assert "not found" in str(exc_info.value)
        assert "Registered evaluators" in str(exc_info.value)

    def test_register_builtin_import_error(self, clean_registry):
        """Test ValueError when builtin evaluator is not found."""
        registry = clean_registry

        # Mock to simulate evaluator not being discovered
        from unittest.mock import patch

        with pytest.raises(ValueError) as exc_info:
            with patch("amp_evaluation.evaluators.builtin.discover_evaluator", return_value=None):
                registry.register_builtin("deepeval/plan-quality")

        assert "is not a built-in evaluator" in str(exc_info.value)


class TestValidationErrors:
    """Test validation error cases."""

    def test_validate_function_no_parameters(self, clean_registry):
        """Test error when function has no parameters."""
        registry = clean_registry

        with pytest.raises(TypeError) as exc_info:

            @registry.register(name="no-params")
            def bad_func():
                return EvalResult(score=1.0)

        assert "must accept at least one parameter" in str(exc_info.value)

    def test_validate_function_too_many_parameters(self, clean_registry):
        """Test error when function has too many parameters."""
        registry = clean_registry

        with pytest.raises(TypeError) as exc_info:

            @registry.register(name="too-many")
            def bad_func(observation, task, extra, another):
                return EvalResult(score=1.0)

        assert "accepts 4 parameters" in str(exc_info.value)
        assert "should accept 1-2" in str(exc_info.value)

    def test_normalize_result_dict_without_score(self, clean_registry):
        """Test error when function returns dict without score."""
        registry = clean_registry

        @registry.register(name="bad-dict")
        def bad_func(observation, task=None):
            return {"explanation": "missing score"}

        evaluator = registry.get("bad-dict")

        # Create trace
        from amp_evaluation.trace import Trace, TraceMetrics, TokenUsage

        trace = Trace(
            trace_id="test",
            input="test",
            output="test",
            metrics=TraceMetrics(
                total_duration_ms=100.0,
                token_usage=TokenUsage(input_tokens=10, output_tokens=10, total_tokens=20),
            ),
            steps=[],
        )

        with pytest.raises(ValueError) as exc_info:
            evaluator.evaluate(trace)

        assert "without 'score' field" in str(exc_info.value)

    def test_normalize_result_invalid_type(self, clean_registry):
        """Test error when function returns invalid type."""
        registry = clean_registry

        @registry.register(name="bad-type")
        def bad_func(observation, task=None):
            return "not a valid result"

        evaluator = registry.get("bad-type")

        # Create trace
        from amp_evaluation.trace import Trace, TraceMetrics, TokenUsage

        trace = Trace(
            trace_id="test",
            input="test",
            output="test",
            metrics=TraceMetrics(
                total_duration_ms=100.0,
                token_usage=TokenUsage(input_tokens=10, output_tokens=10, total_tokens=20),
            ),
            steps=[],
        )

        with pytest.raises(TypeError) as exc_info:
            evaluator.evaluate(trace)

        assert "returned invalid type" in str(exc_info.value)
        assert "str" in str(exc_info.value)


class TestGlobalAPIFunctions:
    """Test global-level API functions."""

    def test_global_evaluator_decorator(self):
        """Test global evaluator() decorator."""
        from amp_evaluation import evaluator

        @evaluator(name="global-test", tags=["global"])
        def custom_eval(trajectory, task=None):
            return EvalResult(score=0.95)

        # Should be in global registry
        from amp_evaluation import get_evaluator

        retrieved = get_evaluator("global-test")
        assert retrieved.name == "global-test"

    def test_global_list_evaluators(self):
        """Test global list_evaluators() function."""
        from amp_evaluation import list_evaluators, evaluator

        initial_count = len(list_evaluators())

        @evaluator(name="list-test")
        def custom_eval(trajectory, task=None):
            return EvalResult(score=1.0)

        # Should appear in list
        evaluators = list_evaluators()
        assert len(evaluators) > initial_count
        assert "list-test" in evaluators

    def test_global_get_evaluator_metadata(self):
        """Test global get_evaluator_metadata() function."""
        from amp_evaluation import get_evaluator_metadata, evaluator

        @evaluator(name="meta-test", description="Test metadata", version="5.0")
        def custom_eval(trajectory, task=None):
            return EvalResult(score=1.0)

        metadata = get_evaluator_metadata("meta-test")
        assert metadata["description"] == "Test metadata"
        assert metadata["version"] == "5.0"

    def test_global_list_by_tag(self):
        """Test global list_by_tag() function."""
        from amp_evaluation import list_by_tag, evaluator

        @evaluator(name="tag-test-1", tags=["global-tag"])
        def eval1(observation, task=None):
            return EvalResult(score=1.0)

        @evaluator(name="tag-test-2", tags=["global-tag"])
        def eval2(observation, task=None):
            return EvalResult(score=1.0)

        tagged = list_by_tag("global-tag")
        assert "tag-test-1" in tagged
        assert "tag-test-2" in tagged

    def test_global_register_builtin(self):
        """Test global register_builtin() function."""
        from amp_evaluation import register_builtin, list_evaluators

        initial = list_evaluators()

        # Register a builtin that hasn't been loaded yet
        # Use a standard evaluator that might not be in the registry yet
        register_builtin("answer_length")

        # Should now appear in list (or already be there from auto-discovery)
        current = list_evaluators()
        assert "answer_length" in current
        # The list should not shrink
        assert len(current) >= len(initial)

    def test_global_list_builtin_evaluators(self):
        """Test global list_builtin_evaluators() function."""
        from amp_evaluation import list_builtin_evaluators

        builtins = list_builtin_evaluators()
        assert isinstance(builtins, list)
        assert len(builtins) > 0
        # Should have deepeval evaluators
        assert any("deepeval" in name for name in builtins)
