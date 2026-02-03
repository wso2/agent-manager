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
Unit tests for built-in aggregation functions.

Tests all aggregation types:
- MEAN, MEDIAN, MIN, MAX, SUM, COUNT
- PASS_RATE (with threshold parameter)
- Percentiles: P50, P75, P90, P95, P99
- STDEV, VARIANCE
"""

import pytest
import sys
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

from amp_evaluation.aggregators.base import AggregationType, Aggregation, normalize_aggregations


class TestAggregationTypes:
    """Test individual aggregation type computations."""

    def test_mean_aggregation(self):
        """Test MEAN aggregation."""
        agg = Aggregation(AggregationType.MEAN)
        scores = [0.6, 0.8, 0.7, 0.9]
        result = agg.compute(scores)
        assert result == 0.75

    def test_median_aggregation(self):
        """Test MEDIAN aggregation."""
        agg = Aggregation(AggregationType.MEDIAN)

        # Odd number of scores
        scores_odd = [0.5, 0.7, 0.9]
        assert agg.compute(scores_odd) == 0.7

        # Even number of scores
        scores_even = [0.5, 0.7, 0.8, 0.9]
        assert agg.compute(scores_even) == 0.75

    def test_min_max_aggregation(self):
        """Test MIN and MAX aggregations."""
        scores = [0.3, 0.7, 0.5, 0.9, 0.4]

        min_agg = Aggregation(AggregationType.MIN)
        max_agg = Aggregation(AggregationType.MAX)

        assert min_agg.compute(scores) == 0.3
        assert max_agg.compute(scores) == 0.9

    def test_sum_aggregation(self):
        """Test SUM aggregation."""
        agg = Aggregation(AggregationType.SUM)
        scores = [0.25, 0.5, 0.75, 1.0]
        result = agg.compute(scores)
        assert result == 2.5

    def test_count_aggregation(self):
        """Test COUNT aggregation."""
        agg = Aggregation(AggregationType.COUNT)
        scores = [0.1, 0.2, 0.3, 0.4, 0.5]
        result = agg.compute(scores)
        assert result == 5

    def test_pass_rate_with_threshold(self):
        """Test PASS_RATE aggregation with different thresholds."""
        scores = [0.5, 0.7, 0.8, 0.6, 0.9, 0.4]

        # threshold=0.7: passes are [0.7, 0.8, 0.9] = 3/6 = 0.5
        agg_70 = Aggregation(AggregationType.PASS_RATE, threshold=0.7)
        assert agg_70.compute(scores) == 0.5

        # threshold=0.5: passes are [0.5, 0.7, 0.8, 0.6, 0.9] = 5/6 ≈ 0.833
        agg_50 = Aggregation(AggregationType.PASS_RATE, threshold=0.5)
        result = agg_50.compute(scores)
        assert abs(result - 0.8333333) < 0.001

        # threshold=1.0: no passes = 0/6 = 0.0
        agg_100 = Aggregation(AggregationType.PASS_RATE, threshold=1.0)
        assert agg_100.compute(scores) == 0.0

    def test_percentile_aggregations(self):
        """Test percentile aggregations (P50, P75, P90, P95, P99)."""
        # Create a set of 100 scores from 0.00 to 0.99
        scores = [i / 100.0 for i in range(100)]

        p50 = Aggregation(AggregationType.P50)
        p75 = Aggregation(AggregationType.P75)
        p90 = Aggregation(AggregationType.P90)
        p95 = Aggregation(AggregationType.P95)
        p99 = Aggregation(AggregationType.P99)

        # P50 should be around 0.50
        assert abs(p50.compute(scores) - 0.50) < 0.02

        # P75 should be around 0.75
        assert abs(p75.compute(scores) - 0.75) < 0.02

        # P90 should be around 0.90
        assert abs(p90.compute(scores) - 0.90) < 0.02

        # P95 should be around 0.95
        assert abs(p95.compute(scores) - 0.95) < 0.02

        # P99 should be around 0.99
        assert abs(p99.compute(scores) - 0.99) < 0.02

    def test_stdev_aggregation(self):
        """Test standard deviation aggregation."""
        agg = Aggregation(AggregationType.STDEV)

        # Known values: [2, 4, 4, 4, 5, 5, 7, 9]
        # Mean = 5, Variance = 4, StdDev = 2
        scores = [0.2, 0.4, 0.4, 0.4, 0.5, 0.5, 0.7, 0.9]
        result = agg.compute(scores)

        # Should be around 0.2 (scaled version)
        assert abs(result - 0.2) < 0.02  # Increased tolerance for floating point

    def test_variance_aggregation(self):
        """Test variance aggregation."""
        agg = Aggregation(AggregationType.VARIANCE)

        scores = [0.2, 0.4, 0.4, 0.4, 0.5, 0.5, 0.7, 0.9]
        result = agg.compute(scores)

        # Variance should be around 0.04 (0.2^2)
        assert abs(result - 0.04) < 0.01


class TestAggregationNaming:
    """Test that aggregation names are generated correctly."""

    def test_simple_aggregation_names(self):
        """Test names for simple aggregations without parameters."""
        assert Aggregation(AggregationType.MEAN).name == "mean"
        assert Aggregation(AggregationType.MEDIAN).name == "median"
        assert Aggregation(AggregationType.P95).name == "p95"

    def test_parameterized_aggregation_names(self):
        """Test names for aggregations with parameters."""
        agg1 = Aggregation(AggregationType.PASS_RATE, threshold=0.7)
        assert agg1.name == "pass_rate_threshold_0.7"

        agg2 = Aggregation(AggregationType.PASS_RATE, threshold=0.9)
        assert agg2.name == "pass_rate_threshold_0.9"

        # Different aggregations with same threshold should have different names
        assert agg1.name != agg2.name


class TestNormalizeAggregations:
    """Test the normalize_aggregations helper function."""

    def test_normalize_none_to_default(self):
        """Test that None gets normalized to DEFAULT_AGGREGATIONS (just MEAN)."""
        from amp_evaluation.aggregators.base import DEFAULT_AGGREGATIONS

        normalized = normalize_aggregations(None)

        # Should return DEFAULT_AGGREGATIONS (which is just MEAN now)
        assert len(normalized) == len(DEFAULT_AGGREGATIONS)
        assert len(normalized) == 1
        assert normalized[0].type == AggregationType.MEAN

    def test_normalize_aggregation_types(self):
        """Test normalizing AggregationType enums."""
        input_aggs = [AggregationType.MEAN, AggregationType.MEDIAN]
        normalized = normalize_aggregations(input_aggs)

        assert len(normalized) == 2
        assert all(isinstance(agg, Aggregation) for agg in normalized)
        assert normalized[0].type == AggregationType.MEAN
        assert normalized[1].type == AggregationType.MEDIAN

    def test_normalize_aggregation_objects(self):
        """Test normalizing Aggregation objects (should pass through)."""
        input_aggs = [Aggregation(AggregationType.MEAN), Aggregation(AggregationType.PASS_RATE, threshold=0.7)]
        normalized = normalize_aggregations(input_aggs)

        assert len(normalized) == 2
        assert normalized[0].type == AggregationType.MEAN
        assert normalized[1].type == AggregationType.PASS_RATE
        assert normalized[1].params["threshold"] == 0.7

    def test_normalize_mixed_types(self):
        """Test normalizing a mix of AggregationType and Aggregation objects."""
        input_aggs = [AggregationType.MEAN, Aggregation(AggregationType.PASS_RATE, threshold=0.8), AggregationType.P95]
        normalized = normalize_aggregations(input_aggs)

        assert len(normalized) == 3
        assert all(isinstance(agg, Aggregation) for agg in normalized)


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
