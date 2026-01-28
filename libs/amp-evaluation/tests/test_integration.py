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
Integration tests for the complete evaluation pipeline.

Tests end-to-end flow:
1. Register evaluators with different aggregation configs
2. Run evaluators on traces
3. Verify aggregated results include proper scores and trace_ids
4. Test default MEAN aggregation when none specified
"""
import pytest
import sys
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent / 'src'))

from amp_evaluation.registry import EvaluatorRegistry
from amp_evaluation.models import Trace, EvalResult, EvalContext
from amp_evaluation.aggregators.aggregation import AggregationType, Aggregation
from amp_evaluation.aggregators.aggregation import ResultAggregator
from amp_evaluation.evaluators.base import BaseEvaluator


class TestEvaluatorIntegration:
    """Test complete evaluation workflow."""
    
    def setup_method(self):
        """Set up test fixtures."""
        self.registry = EvaluatorRegistry()
        
        # Register evaluators with different aggregation configs
        @self.registry.register(
            "has-output",
            description="Check if trace has output",
            aggregations=[
                AggregationType.MEAN,
                Aggregation(AggregationType.PASS_RATE, threshold=1.0)
            ]
        )
        def has_output(context: EvalContext) -> float:
            return 1.0 if context.trace.output else 0.0
        
        @self.registry.register(
            "output-length",
            description="Score based on output length",
            aggregations=[
                AggregationType.MEAN,
                AggregationType.MEDIAN,
                AggregationType.P95
            ]
        )
        def output_length(context: EvalContext) -> float:
            if not context.trace.output:
                return 0.0
            length = len(context.trace.output)
            # Normalize to 0-1 (cap at 1000 chars)
            return min(length / 1000.0, 1.0)
        
        @self.registry.register(
            "default-agg-test",
            description="Test default aggregation (should use MEAN)"
            # No aggregations specified - should default to MEAN
        )
        def default_agg(context: EvalContext) -> float:
            return 0.5
    
    def create_test_trace(self, trace_id: str, output: str = "") -> Trace:
        """Helper to create test traces."""
        return Trace(
            trace_id=trace_id,
            agent_id="test-agent",
            input="test input",
            output=output,
            spans=[]
        )
    
    def test_single_evaluator_on_multiple_traces(self):
        """Test running one evaluator on multiple traces."""
        # Create test traces
        traces = [
            self.create_test_trace("trace_1", "hello"),
            self.create_test_trace("trace_2", "world"),
            self.create_test_trace("trace_3", ""),  # No output
            self.create_test_trace("trace_4", "test output"),
        ]
        
        # Get evaluator
        evaluator = self.registry.get("has-output")
        
        # Run on all traces
        results = []
        for trace in traces:
            result = evaluator.evaluate(EvalContext(trace=trace, is_benchmark=False))
            results.append(result)
        
        # Aggregate
        agg_result = ResultAggregator.aggregate(
            results,
            aggregations=evaluator.aggregations,
            evaluator_name="has-output"
        )
        
        # Verify count
        assert agg_result.count == 4
        
        # Verify aggregations
        assert "mean" in agg_result.aggregations
        assert "pass_rate_threshold_1.0" in agg_result.aggregations
        
        # 3 out of 4 have output
        assert agg_result.aggregations["mean"] == 0.75
        assert agg_result.aggregations["pass_rate_threshold_1.0"] == 0.75
        
        # Verify individual scores include trace_ids
        assert len(agg_result.individual_scores) == 4
        trace_ids = [tid for tid, _ in agg_result.individual_scores]
        assert "trace_1" in trace_ids
        assert "trace_2" in trace_ids
        assert "trace_3" in trace_ids
        assert "trace_4" in trace_ids
        
        # Verify scores match
        scores_dict = {tid: score for tid, score in agg_result.individual_scores}
        assert scores_dict["trace_1"] == 1.0
        assert scores_dict["trace_2"] == 1.0
        assert scores_dict["trace_3"] == 0.0
        assert scores_dict["trace_4"] == 1.0
    
    def test_multiple_evaluators_different_aggregations(self):
        """Test running multiple evaluators with different aggregations."""
        # Create test traces
        traces = [
            self.create_test_trace("t1", "a" * 500),   # 0.5 score
            self.create_test_trace("t2", "b" * 800),   # 0.8 score
            self.create_test_trace("t3", "c" * 1200),  # 1.0 score (capped)
        ]
        
        # Get evaluators
        has_output_eval = self.registry.get("has-output")
        length_eval = self.registry.get("output-length")
        
        # Run both evaluators
        has_output_results = []
        length_results = []
        
        for trace in traces:
            has_output_results.append(has_output_eval.evaluate(EvalContext(trace=trace, is_benchmark=False)))
            length_results.append(length_eval.evaluate(EvalContext(trace=trace, is_benchmark=False)))
        
        # Aggregate
        agg_has_output = ResultAggregator.aggregate(
            has_output_results,
            aggregations=has_output_eval.aggregations
        )
        
        agg_length = ResultAggregator.aggregate(
            length_results,
            aggregations=length_eval.aggregations
        )
        
        # Verify has-output: all traces have output
        assert agg_has_output.aggregations["mean"] == 1.0
        assert agg_has_output.aggregations["pass_rate_threshold_1.0"] == 1.0
        
        # Verify output-length
        assert "mean" in agg_length.aggregations
        assert "median" in agg_length.aggregations
        assert "p95" in agg_length.aggregations
        
        # Mean should be (0.5 + 0.8 + 1.0) / 3 ≈ 0.767
        assert abs(agg_length.aggregations["mean"] - 0.767) < 0.01
        
        # Median should be 0.8
        assert agg_length.aggregations["median"] == 0.8
    
    def test_default_aggregation_when_none_specified(self):
        """Test that evaluators with no aggregations default to MEAN."""
        # Create test traces
        traces = [
            self.create_test_trace("t1", "output"),
            self.create_test_trace("t2", "output"),
        ]
        
        # Get evaluator (has no aggregations specified)
        evaluator = self.registry.get("default-agg-test")
        
        # Should have None aggregations
        assert evaluator.aggregations is None
        
        # Run evaluator
        results = [evaluator.evaluate(EvalContext(trace=trace, is_benchmark=False)) for trace in traces]
        
        # Aggregate with default (MEAN)
        # This simulates what the runner does
        aggregations = evaluator.aggregations if evaluator.aggregations else [AggregationType.MEAN]
        
        agg_result = ResultAggregator.aggregate(
            results,
            aggregations=aggregations
        )
        
        # Should only have MEAN
        assert len(agg_result.aggregations) == 1
        assert "mean" in agg_result.aggregations
        assert agg_result.aggregations["mean"] == 0.5
    
    def test_class_based_evaluator_integration(self):
        """Test that class-based evaluators work in the full pipeline."""
        
        @self.registry.register(
            "class-eval-integration",
            aggregations=[
                AggregationType.MEAN,
                Aggregation(AggregationType.PASS_RATE, threshold=0.7)
            ]
        )
        class MyEvaluator(BaseEvaluator):
            def evaluate(self, context) -> EvalResult:
                # Score based on whether output contains "pass"
                score = 1.0 if "pass" in (context.trace.output or "").lower() else 0.0
                return self._create_result(
                    target_id=context.trace.trace_id,
                    target_type="trace",
                    score=score
                )
        
        # Create traces
        traces = [
            self.create_test_trace("t1", "This will pass"),
            self.create_test_trace("t2", "This will fail"),
            self.create_test_trace("t3", "PASS all tests"),
            self.create_test_trace("t4", "Another failure"),
        ]
        
        # Get evaluator and run
        from amp_evaluation.models import EvalContext
        evaluator = self.registry.get("class-eval-integration")
        results = [evaluator.evaluate(EvalContext(trace=trace, is_benchmark=False)) for trace in traces]
        
        # Aggregate
        agg_result = ResultAggregator.aggregate(
            results,
            aggregations=evaluator.aggregations
        )
        
        # 2 out of 4 pass (t1 and t3)
        assert agg_result.aggregations["mean"] == 0.5
        assert agg_result.aggregations["pass_rate_threshold_0.7"] == 0.5
        
        # Verify individual scores
        scores_dict = {tid: score for tid, score in agg_result.individual_scores}
        assert scores_dict["t1"] == 1.0
        assert scores_dict["t2"] == 0.0
        assert scores_dict["t3"] == 1.0
        assert scores_dict["t4"] == 0.0
    
    def test_individual_scores_preserve_order(self):
        """Test that individual_scores preserve the order of results."""
        traces = [
            self.create_test_trace(f"trace_{i}", "output")
            for i in range(10)
        ]
        
        evaluator = self.registry.get("has-output")
        results = [evaluator.evaluate(EvalContext(trace=trace, is_benchmark=False)) for trace in traces]
        
        agg_result = ResultAggregator.aggregate(
            results,
            aggregations=[AggregationType.MEAN]
        )
        
        # Verify order is preserved
        for i, (trace_id, score) in enumerate(agg_result.individual_scores):
            assert trace_id == f"trace_{i}"
    
    def test_empty_results_handling(self):
        """Test that empty results are handled gracefully."""
        evaluator = self.registry.get("has-output")
        
        # Aggregate empty results
        agg_result = ResultAggregator.aggregate(
            [],
            aggregations=evaluator.aggregations
        )
        
        assert agg_result.count == 0
        assert len(agg_result.aggregations) == 0
        assert len(agg_result.individual_scores) == 0


class TestMultipleAggregationsWithSameType:
    """Test handling multiple aggregations of the same type with different params."""
    
    def setup_method(self):
        """Set up test fixtures."""
        self.registry = EvaluatorRegistry()
        
        @self.registry.register(
            "multi-threshold",
            aggregations=[
                AggregationType.MEAN,
                Aggregation(AggregationType.PASS_RATE, threshold=0.5),
                Aggregation(AggregationType.PASS_RATE, threshold=0.7),
                Aggregation(AggregationType.PASS_RATE, threshold=0.9),
            ]
        )
        def multi_threshold_eval(trace: Trace) -> float:
            # Return a score based on trace_id for testing
            num = int(trace.trace_id.split("_")[1])
            return num / 10.0
    
    def test_multiple_pass_rates_different_thresholds(self):
        """Test multiple PASS_RATE aggregations with different thresholds."""
        # Create traces with scores: 0.0, 0.1, 0.2, ..., 1.0
        traces = [
            Trace(
                trace_id=f"trace_{i}",
                agent_id="test-agent",
                input="input",
                output="output",
                spans=[]
            )
            for i in range(11)  # 0 to 10
        ]
        
        evaluator = self.registry.get("multi-threshold")
        results = [evaluator.evaluate(EvalContext(trace=trace, is_benchmark=False)) for trace in traces]
        
        agg_result = ResultAggregator.aggregate(
            results,
            aggregations=evaluator.aggregations
        )
        
        # Verify all aggregations are present
        assert "mean" in agg_result.aggregations
        assert "pass_rate_threshold_0.5" in agg_result.aggregations
        assert "pass_rate_threshold_0.7" in agg_result.aggregations
        assert "pass_rate_threshold_0.9" in agg_result.aggregations
        
        # Mean should be 0.5 (average of 0.0 to 1.0)
        assert abs(agg_result.aggregations["mean"] - 0.5) < 0.01
        
        # Pass rate >= 0.5: scores 0.5, 0.6, 0.7, 0.8, 0.9, 1.0 = 6/11 ≈ 0.545
        assert abs(agg_result.aggregations["pass_rate_threshold_0.5"] - 0.545) < 0.01
        
        # Pass rate >= 0.7: scores 0.7, 0.8, 0.9, 1.0 = 4/11 ≈ 0.364
        assert abs(agg_result.aggregations["pass_rate_threshold_0.7"] - 0.364) < 0.01
        
        # Pass rate >= 0.9: scores 0.9, 1.0 = 2/11 ≈ 0.182
        assert abs(agg_result.aggregations["pass_rate_threshold_0.9"] - 0.182) < 0.01


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
