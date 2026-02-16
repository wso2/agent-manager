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
Unit tests for Param descriptor.

Tests the Param descriptor functionality for evaluator configuration.
"""

import pytest
import sys
from pathlib import Path
from datetime import datetime

sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from amp_evaluation.evaluators.config import Param
from amp_evaluation.evaluators.base import BaseEvaluator
from amp_evaluation.models import Observation, EvalResult
from amp_evaluation.trace import Trajectory, TraceMetrics, TokenUsage


# ============================================================================
# TEST CONFIG DESCRIPTOR BASICS
# ============================================================================


class TestConfigDescriptor:
    """Test basic Config descriptor functionality."""

    def test_config_default_value(self):
        """Test Config descriptor with default value."""

        class TestEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, description="Test threshold")

            def evaluate(self, observation, task=None):
                return EvalResult(score=self.threshold)

        evaluator = TestEvaluator()
        assert evaluator.threshold == 0.7

    def test_config_set_value_via_init(self):
        """Test setting Config value via __init__ kwargs."""

        class TestEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, description="Test threshold")

            def evaluate(self, observation, task=None):
                return EvalResult(score=self.threshold)

        evaluator = TestEvaluator(threshold=0.9)
        assert evaluator.threshold == 0.9

    def test_config_set_value_directly(self):
        """Test setting Config value directly on instance."""

        class TestEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, description="Test threshold")

            def evaluate(self, observation, task=None):
                return EvalResult(score=self.threshold)

        evaluator = TestEvaluator()
        evaluator.threshold = 0.8
        assert evaluator.threshold == 0.8

    def test_config_class_level_access(self):
        """Test accessing Config descriptor at class level."""

        class TestEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, description="Test threshold")

            def evaluate(self, observation, task=None):
                return EvalResult(score=self.threshold)

        # Class-level access returns the descriptor itself
        assert isinstance(TestEvaluator.threshold, Param)
        assert TestEvaluator.threshold.description == "Test threshold"


# ============================================================================
# TEST CONFIG VALIDATION
# ============================================================================


class TestConfigValidation:
    """Test Config validation constraints."""

    def test_type_validation_success(self):
        """Test type validation passes for correct type."""

        class TestEvaluator(BaseEvaluator):
            count = Param(int, default=5, description="Count")

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        evaluator = TestEvaluator(count=10)
        assert evaluator.count == 10

    def test_type_validation_failure(self):
        """Test type validation fails for wrong type."""

        class TestEvaluator(BaseEvaluator):
            count = Param(int, default=5, description="Count")

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        with pytest.raises(TypeError, match="expects int"):
            TestEvaluator(count="not an int")

    def test_type_validation_int_to_float(self):
        """Test int is accepted for float type."""

        class TestEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, description="Threshold")

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        evaluator = TestEvaluator(threshold=1)  # int
        assert evaluator.threshold == 1  # Accepted

    def test_min_constraint(self):
        """Test min constraint validation."""

        class TestEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, min=0.0, max=1.0, description="Threshold")

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        # Valid value
        evaluator = TestEvaluator(threshold=0.5)
        assert evaluator.threshold == 0.5

        # Invalid value - too low
        with pytest.raises(ValueError, match="must be >= 0.0"):
            TestEvaluator(threshold=-0.1)

    def test_max_constraint(self):
        """Test max constraint validation."""

        class TestEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, min=0.0, max=1.0, description="Threshold")

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        # Valid value
        evaluator = TestEvaluator(threshold=0.9)
        assert evaluator.threshold == 0.9

        # Invalid value - too high
        with pytest.raises(ValueError, match="must be <= 1.0"):
            TestEvaluator(threshold=1.5)

    def test_enum_constraint(self):
        """Test enum constraint validation."""

        class TestEvaluator(BaseEvaluator):
            model = Param(
                str, default="gpt-4o-mini", enum=["gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"], description="Model name"
            )

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        # Valid value
        evaluator = TestEvaluator(model="gpt-4o")
        assert evaluator.model == "gpt-4o"

        # Invalid value - not in enum
        with pytest.raises(ValueError, match="must be one of"):
            TestEvaluator(model="invalid-model")


# ============================================================================
# TEST CONFIG SCHEMA GENERATION
# ============================================================================


class TestConfigSchema:
    """Test Config schema generation."""

    def test_schema_generation_basic(self):
        """Test basic schema generation from Config descriptors."""

        class TestEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, description="Test threshold")
            model = Param(str, default="gpt-4o-mini", description="Model name")

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        evaluator = TestEvaluator()
        metadata = evaluator.get_metadata()

        assert "config_schema" in metadata
        config_schema = metadata["config_schema"]

        # Should have 2 config fields
        assert len(config_schema) == 2

        # Check threshold config
        threshold_config = next(c for c in config_schema if c["key"] == "threshold")
        assert threshold_config["type"] == "float"
        assert threshold_config["default"] == 0.7
        assert threshold_config["description"] == "Test threshold"
        assert not threshold_config["required"]

        # Check model config
        model_config = next(c for c in config_schema if c["key"] == "model")
        assert model_config["type"] == "string"
        assert model_config["default"] == "gpt-4o-mini"
        assert model_config["description"] == "Model name"

    def test_schema_generation_with_constraints(self):
        """Test schema generation includes constraints."""

        class TestEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, min=0.0, max=1.0, description="Threshold with constraints")
            model = Param(str, default="gpt-4o-mini", enum=["gpt-4o", "gpt-4o-mini"], description="Model with enum")

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        evaluator = TestEvaluator()
        metadata = evaluator.get_metadata()
        config_schema = metadata["config_schema"]

        # Check threshold constraints
        threshold_config = next(c for c in config_schema if c["key"] == "threshold")
        assert threshold_config["min"] == 0.0
        assert threshold_config["max"] == 1.0

        # Check model enum
        model_config = next(c for c in config_schema if c["key"] == "model")
        assert model_config["enum_values"] == ["gpt-4o", "gpt-4o-mini"]

    def test_schema_required_field(self):
        """Test schema generation for required fields."""

        class TestEvaluator(BaseEvaluator):
            api_key = Param(str, required=True, description="Required API key")

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        evaluator = TestEvaluator()
        metadata = evaluator.get_metadata()
        config_schema = metadata["config_schema"]

        api_key_config = next(c for c in config_schema if c["key"] == "api_key")
        assert api_key_config["required"]


# ============================================================================
# TEST BACKWARD COMPATIBILITY
# ============================================================================


class TestBackwardCompatibility:
    """Test backward compatibility with existing evaluators."""

    def test_answer_length_evaluator_default(self):
        """Test AnswerLengthEvaluator works with defaults."""
        from amp_evaluation.evaluators.builtin.standard import AnswerLengthEvaluator

        evaluator = AnswerLengthEvaluator()
        assert evaluator.min_length == 1
        assert evaluator.max_length == 10000

    def test_answer_length_evaluator_custom_values(self):
        """Test AnswerLengthEvaluator accepts custom values via init."""
        from amp_evaluation.evaluators.builtin.standard import AnswerLengthEvaluator

        evaluator = AnswerLengthEvaluator(min_length=10, max_length=500)
        assert evaluator.min_length == 10
        assert evaluator.max_length == 500

    def test_answer_length_evaluator_functionality(self):
        """Test AnswerLengthEvaluator works correctly with Config descriptors."""
        from amp_evaluation.evaluators.builtin.standard import AnswerLengthEvaluator

        evaluator = AnswerLengthEvaluator(min_length=10, max_length=50)

        # Create test observation
        trajectory = Trajectory(
            trace_id="test-1",
            input="Test",
            output="This is a test output",  # 21 chars
            timestamp=datetime.now(),
            metrics=TraceMetrics(
                total_duration_ms=100.0,
                token_usage=TokenUsage(input_tokens=10, output_tokens=5, total_tokens=15),
            ),
            steps=[],
        )
        observation = Observation(trajectory=trajectory)

        result = evaluator.evaluate(observation)
        assert result.passed
        assert result.score == 1.0

    def test_answer_length_evaluator_metadata(self):
        """Test AnswerLengthEvaluator provides correct metadata."""
        from amp_evaluation.evaluators.builtin.standard import AnswerLengthEvaluator

        evaluator = AnswerLengthEvaluator()
        metadata = evaluator.get_metadata()

        assert metadata["name"] == "answer_length"
        assert "config_schema" in metadata

        config_schema = metadata["config_schema"]
        assert len(config_schema) == 2

        # Check min_length config
        min_config = next(c for c in config_schema if c["key"] == "min_length")
        assert min_config["type"] == "integer"
        assert min_config["default"] == 1
        assert min_config["min"] == 0

        # Check max_length config
        max_config = next(c for c in config_schema if c["key"] == "max_length")
        assert max_config["type"] == "integer"
        assert max_config["default"] == 10000
        assert max_config["min"] == 1


# ============================================================================
# TEST MULTIPLE CONFIGS
# ============================================================================


class TestMultipleConfigs:
    """Test evaluators with multiple Config descriptors."""

    def test_multiple_configs(self):
        """Test evaluator with multiple Config descriptors."""

        class ComplexEvaluator(BaseEvaluator):
            threshold = Param(float, default=0.7, min=0.0, max=1.0, description="Score threshold")
            model = Param(str, default="gpt-4o-mini", description="LLM model")
            max_retries = Param(int, default=3, min=1, max=10, description="Max retries")
            strict_mode = Param(bool, default=False, description="Enable strict mode")

            def evaluate(self, observation, task=None):
                return EvalResult(score=self.threshold)

        # Test defaults
        evaluator = ComplexEvaluator()
        assert evaluator.threshold == 0.7
        assert evaluator.model == "gpt-4o-mini"
        assert evaluator.max_retries == 3
        assert not evaluator.strict_mode

        # Test custom values
        evaluator = ComplexEvaluator(threshold=0.9, model="gpt-4o", max_retries=5, strict_mode=True)
        assert evaluator.threshold == 0.9
        assert evaluator.model == "gpt-4o"
        assert evaluator.max_retries == 5
        assert evaluator.strict_mode

        # Test partial custom values
        evaluator = ComplexEvaluator(threshold=0.8)
        assert evaluator.threshold == 0.8
        assert evaluator.model == "gpt-4o-mini"  # default
        assert evaluator.max_retries == 3  # default


# ============================================================================
# TEST CONFIG ENFORCEMENT FOR BUILT-IN EVALUATORS
# ============================================================================


class TestConfigEnforcement:
    """Test that built-in evaluators are enforced to use Config descriptors."""

    def test_builtin_evaluator_without_config_fails(self):
        """Test that built-in evaluators with __init__ params but no Config descriptors fail."""

        # Create a mock built-in evaluator (simulate it being in the builtin package)
        class BadBuiltinEvaluator(BaseEvaluator):
            """Simulates a built-in evaluator that doesn't use Config."""

            def __init__(self, threshold: float = 0.7):
                # This should fail validation because threshold is not a Config descriptor
                super().__init__()

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        # Manually set the module to simulate a built-in evaluator
        BadBuiltinEvaluator.__module__ = "amp_evaluation.evaluators.builtin.test"

        with pytest.raises(ValueError, match="has __init__ parameters.*that are not defined as Param descriptors"):
            BadBuiltinEvaluator(threshold=0.8)

    def test_builtin_evaluator_with_config_passes(self):
        """Test that built-in evaluators with Config descriptors pass validation."""

        class GoodBuiltinEvaluator(BaseEvaluator):
            """Simulates a properly configured built-in evaluator."""

            threshold = Param(float, default=0.7, description="Threshold")

            def __init__(self, **kwargs):
                super().__init__(**kwargs)

            def evaluate(self, observation, task=None):
                return EvalResult(score=1.0)

        # Manually set the module to simulate a built-in evaluator
        GoodBuiltinEvaluator.__module__ = "amp_evaluation.evaluators.builtin.test"

        # Should not raise
        evaluator = GoodBuiltinEvaluator(threshold=0.8)
        assert evaluator.threshold == 0.8

    def test_user_evaluator_without_config_allowed(self):
        """Test that user-defined evaluators can use __init__ params without Config."""

        class UserEvaluator(BaseEvaluator):
            """User-defined evaluator outside the builtin package."""

            def __init__(self, threshold: float = 0.7, **kwargs):
                super().__init__(**kwargs)
                self.threshold = threshold

            def evaluate(self, observation, task=None):
                return EvalResult(score=self.threshold)

        # User evaluators (not in builtin package) should not be validated
        # Module name won't start with amp_evaluation.evaluators.builtin
        evaluator = UserEvaluator(threshold=0.8)
        assert evaluator.threshold == 0.8

    def test_answer_length_evaluator_properly_configured(self):
        """Test that the migrated AnswerLengthEvaluator uses Config properly."""
        from amp_evaluation.evaluators.builtin.standard import AnswerLengthEvaluator

        # Should not raise validation error
        evaluator = AnswerLengthEvaluator(min_length=10, max_length=500)
        assert evaluator.min_length == 10
        assert evaluator.max_length == 500

        # Should have Config descriptors
        assert isinstance(AnswerLengthEvaluator.min_length, Param)
        assert isinstance(AnswerLengthEvaluator.max_length, Param)


# ============================================================================
# TEST SENTINEL VALUE FOR NONE DEFAULTS
# ============================================================================


class TestConfigNoneDefaults:
    """Test Config descriptor handling of None and falsy defaults."""

    def test_default_none_is_not_required(self):
        """Test that default=None makes field not required."""

        class TestEvaluator(BaseEvaluator):
            optional_field = Param(str, default=None, description="Optional with None default")

            def evaluate(self, observation, task=None):
                return EvalResult.skip("")

        evaluator = TestEvaluator()
        assert evaluator.optional_field is None
        assert not TestEvaluator.optional_field.required

    def test_no_default_is_required(self):
        """Test that no default makes field required."""

        class TestEvaluator(BaseEvaluator):
            required_field = Param(str, description="Required field")

            def evaluate(self, observation, task=None):
                return EvalResult.skip("")

        assert TestEvaluator.required_field.required

    def test_empty_string_default_is_not_required(self):
        """Test that default='' (empty string) makes field not required."""

        class TestEvaluator(BaseEvaluator):
            optional_str = Param(str, default="", description="Optional with empty string")

            def evaluate(self, observation, task=None):
                return EvalResult.skip("")

        evaluator = TestEvaluator()
        assert evaluator.optional_str == ""
        assert not TestEvaluator.optional_str.required

    def test_zero_default_is_not_required(self):
        """Test that default=0 makes field not required."""

        class TestEvaluator(BaseEvaluator):
            count = Param(int, default=0, description="Count with 0 default")

            def evaluate(self, observation, task=None):
                return EvalResult.skip("")

        evaluator = TestEvaluator()
        assert evaluator.count == 0
        assert not TestEvaluator.count.required

    def test_false_default_is_not_required(self):
        """Test that default=False makes field not required."""

        class TestEvaluator(BaseEvaluator):
            enabled = Param(bool, default=False, description="Enabled with False default")

            def evaluate(self, observation, task=None):
                return EvalResult.skip("")

        evaluator = TestEvaluator()
        assert evaluator.enabled is False
        assert not TestEvaluator.enabled.required

    def test_schema_includes_none_default(self):
        """Test that schema generation includes default=None."""

        class TestEvaluator(BaseEvaluator):
            optional_field = Param(str, default=None, description="Optional with None default")

            def evaluate(self, observation, task=None):
                return EvalResult.skip("")

        schema = TestEvaluator.optional_field.to_schema()
        assert "default" in schema
        assert schema["default"] is None
        assert not schema["required"]

    def test_schema_excludes_no_default(self):
        """Test that schema generation excludes default when not provided."""

        class TestEvaluator(BaseEvaluator):
            required_field = Param(str, description="Required field")

            def evaluate(self, observation, task=None):
                return EvalResult.skip("")

        schema = TestEvaluator.required_field.to_schema()
        assert "default" not in schema
        assert schema["required"]

    def test_explicit_required_overrides_default(self):
        """Test that explicit required=True works even with a default."""

        class TestEvaluator(BaseEvaluator):
            field = Param(str, default="value", required=True, description="Required despite default")

            def evaluate(self, observation, task=None):
                return EvalResult.skip("")

        assert TestEvaluator.field.required
        evaluator = TestEvaluator()
        assert evaluator.field == "value"
