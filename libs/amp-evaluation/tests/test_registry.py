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
Unit tests for the evaluator registry system.

Tests:
- Registration with no aggregations (should default to MEAN)
- Registration with single aggregation
- Registration with multiple aggregations
- Registration with aggregations containing parameters
- Function-based evaluators
- Class-based evaluators
"""
import pytest
import sys
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent / 'src'))

from amp_evaluation.registry import EvaluatorRegistry
from amp_evaluation.models import Trace, EvalResult
from amp_evaluation.evaluators.base import BaseEvaluator
from amp_evaluation.aggregators.aggregation import AggregationType, Aggregation


class TestRegistryAggregations:
    """Test registration with different aggregation configurations."""
    
    def setup_method(self):
        """Create a fresh registry for each test."""
        self.registry = EvaluatorRegistry()
    
    def test_registration_no_aggregations_defaults_to_mean(self):
        """When no aggregations specified, evaluator should use None (runner defaults to MEAN)."""
        
        @self.registry.register("test-no-agg")
        def evaluator(trace: Trace) -> EvalResult:
            return EvalResult(
                evaluator_name="test-no-agg",
                target_id=trace.trace_id,
                target_type="trace",
                score=1.0
            )
        
        # Get evaluator instance
        eval_instance = self.registry.get("test-no-agg")
        
        # Should have None aggregations (runner will default to MEAN)
        assert eval_instance.aggregations is None
    
    def test_registration_single_aggregation(self):
        """Test registration with a single aggregation type."""
        
        @self.registry.register(
            "test-single-agg",
            aggregations=[AggregationType.MEDIAN]
        )
        def evaluator(trace: Trace) -> EvalResult:
            return EvalResult(
                evaluator_name="test-single-agg",
                target_id=trace.trace_id,
                target_type="trace",
                score=0.5
            )
        
        eval_instance = self.registry.get("test-single-agg")
        
        assert eval_instance.aggregations is not None
        assert len(eval_instance.aggregations) == 1
        assert eval_instance.aggregations[0] == AggregationType.MEDIAN
    
    def test_registration_multiple_aggregations(self):
        """Test registration with multiple aggregation types."""
        
        @self.registry.register(
            "test-multi-agg",
            aggregations=[
                AggregationType.MEAN,
                AggregationType.MEDIAN,
                AggregationType.P95
            ]
        )
        def evaluator(trace: Trace) -> EvalResult:
            return EvalResult(
                evaluator_name="test-multi-agg",
                target_id=trace.trace_id,
                target_type="trace",
                score=0.8
            )
        
        eval_instance = self.registry.get("test-multi-agg")
        
        assert eval_instance.aggregations is not None
        assert len(eval_instance.aggregations) == 3
        assert AggregationType.MEAN in eval_instance.aggregations
        assert AggregationType.MEDIAN in eval_instance.aggregations
        assert AggregationType.P95 in eval_instance.aggregations
    
    def test_registration_aggregations_with_params(self):
        """Test registration with parameterized aggregations (e.g., PASS_RATE with threshold)."""
        
        @self.registry.register(
            "test-param-agg",
            aggregations=[
                AggregationType.MEAN,
                Aggregation(AggregationType.PASS_RATE, threshold=0.7),
                Aggregation(AggregationType.PASS_RATE, threshold=0.9)
            ]
        )
        def evaluator(trace: Trace) -> EvalResult:
            return EvalResult(
                evaluator_name="test-param-agg",
                target_id=trace.trace_id,
                target_type="trace",
                score=0.85
            )
        
        eval_instance = self.registry.get("test-param-agg")
        
        assert eval_instance.aggregations is not None
        assert len(eval_instance.aggregations) == 3
        
        # Check that we have MEAN and two PASS_RATE with different thresholds
        agg_types = [agg.type if isinstance(agg, Aggregation) else agg 
                     for agg in eval_instance.aggregations]
        assert AggregationType.MEAN in agg_types
        
        # Count PASS_RATE aggregations
        pass_rate_aggs = [agg for agg in eval_instance.aggregations 
                          if isinstance(agg, Aggregation) and agg.type == AggregationType.PASS_RATE]
        assert len(pass_rate_aggs) == 2
        
        # Check thresholds
        thresholds = [agg.params.get('threshold') for agg in pass_rate_aggs]
        assert 0.7 in thresholds
        assert 0.9 in thresholds
    
    def test_registration_function_based(self):
        """Test that function-based evaluators work with aggregations."""
        
        @self.registry.register(
            "func-eval",
            aggregations=[AggregationType.MEAN, AggregationType.MAX]
        )
        def simple_eval(trace: Trace) -> float:
            return 1.0 if len(trace.output or "") > 0 else 0.0
        
        eval_instance = self.registry.get("func-eval")
        
        assert eval_instance.aggregations is not None
        assert len(eval_instance.aggregations) == 2
    
    def test_registration_class_based(self):
        """Test that class-based evaluators work with aggregations."""
        
        @self.registry.register(
            "class-eval",
            aggregations=[
                AggregationType.MEDIAN,
                Aggregation(AggregationType.PASS_RATE, threshold=0.5)
            ]
        )
        class CustomEvaluator(BaseEvaluator):
            def evaluate(self, context) -> EvalResult:
                return self._create_result(
                    target_id=context.trace.trace_id,
                    target_type="trace",
                    score=0.75
                )
        
        eval_instance = self.registry.get("class-eval")
        
        assert eval_instance.aggregations is not None
        assert len(eval_instance.aggregations) == 2
        
        # Verify it's actually our class
        assert isinstance(eval_instance, CustomEvaluator)
    
    def test_multiple_evaluators_different_aggregations(self):
        """Test that different evaluators can have different aggregations."""
        
        @self.registry.register("eval1", aggregations=[AggregationType.MEAN])
        def eval1(trace: Trace) -> float:
            return 0.5
        
        @self.registry.register("eval2", aggregations=[AggregationType.MEDIAN, AggregationType.P99])
        def eval2(trace: Trace) -> float:
            return 0.8
        
        @self.registry.register("eval3")  # No aggregations - should default
        def eval3(trace: Trace) -> float:
            return 0.9
        
        e1 = self.registry.get("eval1")
        e2 = self.registry.get("eval2")
        e3 = self.registry.get("eval3")
        
        assert len(e1.aggregations) == 1
        assert len(e2.aggregations) == 2
        assert e3.aggregations is None  # Will default to MEAN at runtime


class TestRegistryBasics:
    """Test basic registry functionality."""
    
    def setup_method(self):
        """Create a fresh registry for each test."""
        self.registry = EvaluatorRegistry()
    
    def test_register_and_get_evaluator(self):
        """Test basic registration and retrieval."""
        
        @self.registry.register("basic-test")
        def evaluator(trace: Trace) -> float:
            return 1.0
        
        eval_instance = self.registry.get("basic-test")
        assert eval_instance is not None
        assert eval_instance.name == "basic-test"
    
    def test_get_nonexistent_evaluator_raises_error(self):
        """Test that getting a non-existent evaluator raises ValueError."""
        
        with pytest.raises(ValueError, match="Evaluator 'does-not-exist' not found"):
            self.registry.get("does-not-exist")
    
    def test_list_evaluators(self):
        """Test listing all registered evaluators."""
        
        @self.registry.register("eval-a")
        def eval_a(trace: Trace) -> float:
            return 0.5
        
        @self.registry.register("eval-b")
        def eval_b(trace: Trace) -> float:
            return 0.7
        
        evaluators = self.registry.list_evaluators()
        assert "eval-a" in evaluators
        assert "eval-b" in evaluators
        assert len(evaluators) == 2
    
    def test_metadata_storage(self):
        """Test that metadata is properly stored and retrieved."""
        
        @self.registry.register(
            "meta-test",
            description="A test evaluator",
            tags=["test", "quality"],
            version="1.2.3"
        )
        def evaluator(trace: Trace) -> float:
            return 0.5
        
        metadata = self.registry.get_metadata("meta-test")
        assert metadata["description"] == "A test evaluator"
        assert "test" in metadata["tags"]
        assert "quality" in metadata["tags"]
        assert metadata["version"] == "1.2.3"


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
