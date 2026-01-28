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
Built-in aggregation functions.

Provides standard statistical aggregations for evaluator scores.
"""
from typing import List, Dict, Callable
import statistics

from .base import AggregationType


# Built-in aggregation functions
def _mean(scores: List[float], **kwargs) -> float:
    """Calculate mean (average) of scores."""
    return statistics.mean(scores) if scores else 0.0


def _median(scores: List[float], **kwargs) -> float:
    """Calculate median of scores."""
    return statistics.median(scores) if scores else 0.0


def _min(scores: List[float], **kwargs) -> float:
    """Calculate minimum score."""
    return min(scores) if scores else 0.0


def _max(scores: List[float], **kwargs) -> float:
    """Calculate maximum score."""
    return max(scores) if scores else 0.0


def _sum(scores: List[float], **kwargs) -> float:
    """Calculate sum of scores."""
    return sum(scores)


def _count(scores: List[float], **kwargs) -> float:
    """Count number of scores."""
    return float(len(scores))


def _stdev(scores: List[float], **kwargs) -> float:
    """Calculate standard deviation of scores."""
    return statistics.stdev(scores) if len(scores) > 1 else 0.0


def _variance(scores: List[float], **kwargs) -> float:
    """Calculate variance of scores."""
    return statistics.variance(scores) if len(scores) > 1 else 0.0


def _percentile(scores: List[float], p: float = 50, **kwargs) -> float:
    """
    Calculate percentile of scores.
    
    Args:
        scores: List of scores
        p: Percentile (0-100)
    """
    if not scores:
        return 0.0
    sorted_scores = sorted(scores)
    k = (len(sorted_scores) - 1) * (p / 100)
    f = int(k)
    c = f + 1 if f + 1 < len(sorted_scores) else f
    return sorted_scores[f] + (k - f) * (sorted_scores[c] - sorted_scores[f])


def _p50(scores: List[float], **kwargs) -> float:
    """Calculate 50th percentile (median)."""
    return _percentile(scores, 50)


def _p75(scores: List[float], **kwargs) -> float:
    """Calculate 75th percentile."""
    return _percentile(scores, 75)


def _p90(scores: List[float], **kwargs) -> float:
    """Calculate 90th percentile."""
    return _percentile(scores, 90)


def _p95(scores: List[float], **kwargs) -> float:
    """Calculate 95th percentile."""
    return _percentile(scores, 95)


def _p99(scores: List[float], **kwargs) -> float:
    """Calculate 99th percentile."""
    return _percentile(scores, 99)


def _pass_rate(scores: List[float], threshold: float = 0.7, **kwargs) -> float:
    """
    Calculate pass rate based on threshold.
    
    Args:
        scores: List of scores
        threshold: Score threshold for passing (default: 0.7)
    
    Returns:
        Fraction of scores >= threshold
    """
    if not scores:
        return 0.0
    passed = sum(1 for s in scores if s >= threshold)
    return passed / len(scores)


# Mapping from AggregationType to functions
BUILTIN_AGGREGATORS: Dict[AggregationType, Callable] = {
    AggregationType.MEAN: _mean,
    AggregationType.MEDIAN: _median,
    AggregationType.MIN: _min,
    AggregationType.MAX: _max,
    AggregationType.SUM: _sum,
    AggregationType.COUNT: _count,
    AggregationType.STDEV: _stdev,
    AggregationType.VARIANCE: _variance,
    AggregationType.P50: _p50,
    AggregationType.P75: _p75,
    AggregationType.P90: _p90,
    AggregationType.P95: _p95,
    AggregationType.P99: _p99,
    AggregationType.PASS_RATE: _pass_rate,
}
