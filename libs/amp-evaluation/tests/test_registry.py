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
Unit tests for the evaluator decorator and discovery utilities.

Tests:
- @evaluator decorator: returns FunctionEvaluator, sets metadata fields
- FunctionEvaluator aggregations: default None, single, multiple, Aggregation objects
- discover_evaluators: finds evaluator instances in modules, ignores non-evaluators
- Evaluator .info property: returns EvaluatorInfo with correct fields
"""

import types
import pytest
import sys
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from amp_evaluation.registry import evaluator, discover_evaluators
from amp_evaluation.evaluators.base import BaseEvaluator, FunctionEvaluator
from amp_evaluation.models import EvalResult, EvaluatorInfo
from amp_evaluation.trace import Trace
from amp_evaluation.evaluators.config import EvaluationLevel
from amp_evaluation.aggregators.base import AggregationType, Aggregation


class TestEvaluatorDecorator:
    """Tests for the @evaluator decorator."""

    def test_decorator_returns_function_evaluator(self):
        """@evaluator('test-name') should return a FunctionEvaluator instance."""

        @evaluator("test-name")
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        assert isinstance(my_eval, FunctionEvaluator)

    def test_decorator_sets_name(self):
        """The returned FunctionEvaluator should have the correct name."""

        @evaluator("test-name")
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        assert my_eval.name == "test-name"

    def test_decorator_sets_description(self):
        """The description passed to @evaluator should be set on the instance."""

        @evaluator("desc-eval", description="A test evaluator for descriptions")
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        assert my_eval.description == "A test evaluator for descriptions"

    def test_decorator_sets_tags(self):
        """The tags passed to @evaluator should be set on the instance."""

        @evaluator("tags-eval", tags=["quality", "test", "example"])
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        assert my_eval.tags == ["quality", "test", "example"]

    def test_decorator_sets_version(self):
        """The version passed to @evaluator should be set on the instance."""

        @evaluator("version-eval", version="2.3.1")
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        assert my_eval.version == "2.3.1"

    def test_decorator_sets_aggregations(self):
        """The aggregations passed to @evaluator should be set on the instance."""

        @evaluator(
            "agg-eval",
            aggregations=[AggregationType.MEAN, AggregationType.MEDIAN],
        )
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        assert my_eval.aggregations is not None
        assert len(my_eval.aggregations) == 2
        assert AggregationType.MEAN in my_eval.aggregations
        assert AggregationType.MEDIAN in my_eval.aggregations

    def test_decorator_preserves_function_behavior(self):
        """Calling evaluate on the decorated evaluator should invoke the original function logic."""

        @evaluator("behavior-eval")
        def my_eval(trace: Trace) -> EvalResult:
            score = 1.0 if trace.output == "hello" else 0.0
            return EvalResult(score=score, explanation="checked output")

        trace = Trace(trace_id="t1", input="say hello", output="hello")
        results = my_eval.run(trace)
        assert len(results) == 1
        assert results[0].score == 1.0
        assert results[0].explanation == "checked output"


class TestFunctionEvaluatorAggregations:
    """Tests for aggregation handling on FunctionEvaluator instances."""

    def test_no_aggregations_default_none(self):
        """When no aggregations are set, .aggregations should be None."""

        @evaluator("no-agg-eval")
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=0.5)

        assert my_eval.aggregations is None

    def test_single_aggregation(self):
        """Setting a single aggregation should work correctly."""

        @evaluator("single-agg-eval", aggregations=[AggregationType.MEDIAN])
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=0.5)

        assert my_eval.aggregations is not None
        assert len(my_eval.aggregations) == 1
        assert my_eval.aggregations[0] == AggregationType.MEDIAN

    def test_multiple_aggregations(self):
        """Setting multiple aggregations should work correctly."""

        @evaluator(
            "multi-agg-eval",
            aggregations=[AggregationType.MEAN, AggregationType.MEDIAN, AggregationType.P95],
        )
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=0.75)

        assert my_eval.aggregations is not None
        assert len(my_eval.aggregations) == 3
        assert AggregationType.MEAN in my_eval.aggregations
        assert AggregationType.MEDIAN in my_eval.aggregations
        assert AggregationType.P95 in my_eval.aggregations

    def test_aggregations_with_objects(self):
        """Can pass Aggregation objects (with parameters) as aggregations."""

        @evaluator(
            "obj-agg-eval",
            aggregations=[
                AggregationType.MEAN,
                Aggregation(AggregationType.PASS_RATE, threshold=0.7),
                Aggregation(AggregationType.PASS_RATE, threshold=0.9),
            ],
        )
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=0.8)

        assert my_eval.aggregations is not None
        assert len(my_eval.aggregations) == 3

        # Verify Aggregation objects are preserved correctly
        pass_rate_aggs = [
            agg
            for agg in my_eval.aggregations
            if isinstance(agg, Aggregation) and agg.type == AggregationType.PASS_RATE
        ]
        assert len(pass_rate_aggs) == 2

        thresholds = sorted([agg.params.get("threshold") for agg in pass_rate_aggs])
        assert thresholds == [0.7, 0.9]


class TestDiscoverEvaluators:
    """Tests for the discover_evaluators function."""

    def test_discovers_evaluator_instances(self):
        """discover_evaluators should find all BaseEvaluator instances in a module."""

        mock_module = types.ModuleType("mock_module")

        @evaluator("found-1")
        def eval1(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        @evaluator("found-2")
        def eval2(trace: Trace) -> EvalResult:
            return EvalResult(score=0.5)

        mock_module.eval1 = eval1
        mock_module.eval2 = eval2

        found = discover_evaluators(mock_module)
        assert len(found) == 2

        found_names = {e.name for e in found}
        assert "found-1" in found_names
        assert "found-2" in found_names

    def test_ignores_non_evaluators(self):
        """discover_evaluators should skip objects that are not BaseEvaluator instances."""

        mock_module = types.ModuleType("mock_module")

        @evaluator("the-only-eval")
        def real_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        mock_module.real_eval = real_eval
        mock_module.non_evaluator = "just a string"
        mock_module.some_number = 42
        mock_module.some_list = [1, 2, 3]
        mock_module.some_dict = {"key": "value"}

        found = discover_evaluators(mock_module)
        assert len(found) == 1
        assert found[0].name == "the-only-eval"

    def test_empty_module_returns_empty(self):
        """discover_evaluators should return an empty list for a module with no evaluators."""

        mock_module = types.ModuleType("empty_module")
        mock_module.a_string = "hello"
        mock_module.a_number = 99

        found = discover_evaluators(mock_module)
        assert found == []


class TestEvaluatorMetadata:
    """Tests for the .info property on evaluators."""

    def test_info_returns_evaluator_info(self):
        """.info should return an EvaluatorInfo instance."""

        @evaluator("info-eval", description="An info test")
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        info = my_eval.info
        assert isinstance(info, EvaluatorInfo)

    def test_info_has_correct_fields(self):
        """.info should contain the correct name, description, tags, version, level, and modes."""

        @evaluator(
            "meta-eval",
            description="Metadata evaluator",
            tags=["meta", "test"],
            version="3.0.0",
        )
        def my_eval(trace: Trace) -> EvalResult:
            return EvalResult(score=1.0)

        info = my_eval.info
        assert info.name == "meta-eval"
        assert info.description == "Metadata evaluator"
        assert info.tags == ["meta", "test"]
        assert info.version == "3.0.0"
        assert info.level == EvaluationLevel.TRACE.value
        assert "experiment" in info.modes
        assert "monitor" in info.modes


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
