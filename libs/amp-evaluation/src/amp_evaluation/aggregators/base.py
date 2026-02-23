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
Base aggregator classes and types.

Provides type-safe, extensible aggregation configuration for evaluators.
"""

from typing import Union, Callable, List, Dict, Any, Optional
from dataclasses import dataclass, field
from enum import Enum


class AggregationType(Enum):
    """
    Predefined aggregation types.

    Use these for common aggregation strategies. For aggregations that
    require parameters (like PASS_RATE), wrap in an Aggregation object.

    Example:
        # Simple aggregation (no params)
        aggregations=[AggregationType.MEAN, AggregationType.MEDIAN]

        # Aggregation with parameters
        aggregations=[Aggregation(AggregationType.PASS_RATE, threshold=0.7)]
    """

    # Basic statistics
    MEAN = "mean"
    MEDIAN = "median"
    MIN = "min"
    MAX = "max"
    SUM = "sum"
    COUNT = "count"
    STDEV = "stdev"
    VARIANCE = "variance"

    # Percentiles
    P50 = "p50"
    P75 = "p75"
    P90 = "p90"
    P95 = "p95"
    P99 = "p99"

    # Pass/fail based (requires threshold parameter)
    PASS_RATE = "pass_rate"


@dataclass
class Aggregation:
    """
    Configuration for an aggregation with optional parameters.

    Use this when you need to pass parameters to an aggregation function,
    like threshold for PASS_RATE.

    Examples:
        # With enum type
        Aggregation(AggregationType.PASS_RATE, threshold=0.7)
        Aggregation(AggregationType.PASS_RATE, threshold=0.9)

        # With custom registered function
        Aggregation("weighted_average", weights=[0.5, 0.3, 0.2])

        # With inline custom function
        Aggregation(lambda scores: max(scores) - min(scores))  # range
    """

    type: Union[AggregationType, str, Callable]
    params: Dict[str, Any] = field(default_factory=dict)

    def __init__(self, type: Union[AggregationType, str, Callable], **params):
        self.type = type
        self.params = params

    @property
    def name(self) -> str:
        """
        Get display name for this aggregation.

        Includes parameters in name for uniqueness.
        Examples:
            - "mean"
            - "pass_rate_threshold_0.7"
            - "weighted_average_weights_[0.5,0.3,0.2]"
        """
        # Get base name
        if isinstance(self.type, AggregationType):
            base = self.type.value
        elif isinstance(self.type, str):
            base = self.type
        elif callable(self.type):
            base = getattr(self.type, "__name__", "custom")
        else:
            base = str(self.type)

        # Add parameters to name if present
        if self.params:
            param_parts = []
            for k, v in sorted(self.params.items()):
                param_parts.append(f"{k}_{v}")
            return f"{base}_{'_'.join(param_parts)}"

        return base

    def get_function(self) -> Callable:
        """
        Get the aggregation function.

        Returns:
            Callable that takes (scores: List[float], **kwargs) -> float

        Raises:
            ValueError: If aggregation type not found
        """
        from .builtin import BUILTIN_AGGREGATORS

        if isinstance(self.type, AggregationType):
            return BUILTIN_AGGREGATORS[self.type]
        elif callable(self.type):
            return self.type
        elif isinstance(self.type, str):
            # Try builtin by string name
            for agg_type in AggregationType:
                if agg_type.value == self.type:
                    return BUILTIN_AGGREGATORS[agg_type]
            valid_types = [t.value for t in AggregationType]
            raise ValueError(f"Unknown aggregation type: '{self.type}'. Available: {valid_types}")
        else:
            raise TypeError(f"Invalid aggregation type: {type(self.type)}")

    def compute(self, scores: List[float]) -> float:
        """
        Compute the aggregation on a list of scores.

        Args:
            scores: List of float scores

        Returns:
            Aggregated score
        """
        func = self.get_function()
        return func(scores, **self.params)

    def __repr__(self) -> str:
        if self.params:
            params_str = ", ".join(f"{k}={v!r}" for k, v in self.params.items())
            return f"Aggregation({self.type!r}, {params_str})"
        return f"Aggregation({self.type!r})"


# Default aggregations for evaluators that don't specify any
# Using only MEAN as the default keeps it simple and predictable
DEFAULT_AGGREGATIONS: List[AggregationType] = [
    AggregationType.MEAN,
]


def normalize_aggregations(
    aggregations: Optional[List[Union[AggregationType, Aggregation, str, Callable]]],
) -> List[Aggregation]:
    """
    Normalize various aggregation specifications to List[Aggregation].

    Args:
        aggregations: List of aggregation specs (can be mixed types)

    Returns:
        List of Aggregation objects

    Examples:
        normalize_aggregations([AggregationType.MEAN, "median"])
        # -> [Aggregation(MEAN), Aggregation("median")]

        normalize_aggregations([
            AggregationType.MEAN,
            Aggregation(AggregationType.PASS_RATE, threshold=0.7)
        ])
        # -> [Aggregation(MEAN), Aggregation(PASS_RATE, threshold=0.7)]
    """
    if aggregations is None:
        return [Aggregation(a) for a in DEFAULT_AGGREGATIONS]

    result = []
    for agg in aggregations:
        if isinstance(agg, Aggregation):
            result.append(agg)
        elif isinstance(agg, AggregationType):
            result.append(Aggregation(agg))
        elif isinstance(agg, str):
            result.append(Aggregation(agg))
        elif callable(agg):
            result.append(Aggregation(agg))
        else:
            raise TypeError(
                f"Invalid aggregation type: {type(agg)}. Expected AggregationType, Aggregation, str, or Callable."
            )

    return result
