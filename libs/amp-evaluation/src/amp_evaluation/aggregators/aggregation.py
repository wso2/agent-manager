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
Result aggregation for evaluation results.

Provides simple interface to compute multiple aggregations over EvalResult lists.
"""

from typing import List, Dict, Optional, Union
from dataclasses import dataclass, field

from ..models import EvalResult
from .base import Aggregation, AggregationType, normalize_aggregations


@dataclass
class AggregatedResults:
    """
    Results of aggregating multiple evaluation results.

    Contains:
    - evaluator_name: Name of the evaluator
    - count: Number of results aggregated
    - aggregations: Dict mapping aggregation name to computed value
    - scores: Raw score values (optional, for further analysis)
    - individual_scores: List of (trace_id, score) tuples for detailed analysis
    """

    evaluator_name: str
    count: int
    aggregations: Dict[str, float] = field(default_factory=dict)
    scores: List[float] = field(default_factory=list)
    individual_scores: List[tuple[str, float]] = field(default_factory=list)  # (trace_id, score) pairs

    def __getitem__(self, key: str) -> float:
        """Allow dict-like access to aggregations."""
        return self.aggregations[key]

    def get(self, key: str, default: float = 0.0) -> float:
        """Get aggregation value with default."""
        return self.aggregations.get(key, default)

    @property
    def mean(self) -> Optional[float]:
        """Convenience accessor for mean."""
        return self.aggregations.get("mean")

    @property
    def median(self) -> Optional[float]:
        """Convenience accessor for median."""
        return self.aggregations.get("median")


class ResultAggregator:
    """
    Aggregates evaluation results using configured aggregations.

    Usage:
        results = [result1, result2, result3, ...]

        # Compute multiple aggregations at once
        agg = ResultAggregator.aggregate(
            results,
            aggregations=[
                AggregationType.MEAN,
                AggregationType.MEDIAN,
                Aggregation(AggregationType.PASS_RATE, threshold=0.7),
                Aggregation(AggregationType.PASS_RATE, threshold=0.9),
            ]
        )

        print(agg.aggregations)
        # {
        #     "mean": 0.85,
        #     "median": 0.88,
        #     "pass_rate_threshold_0.7": 0.92,
        #     "pass_rate_threshold_0.9": 0.75
        # }
    """

    @classmethod
    def aggregate(
        cls,
        results: List[EvalResult],
        aggregations: Optional[List[Union[AggregationType, Aggregation]]] = None,
        evaluator_name: Optional[str] = None,
        include_scores: bool = False,
    ) -> AggregatedResults:
        """
        Aggregate a list of evaluation results.

        Args:
            results: List of EvalResult objects to aggregate
            aggregations: List of aggregations to compute.
                          Accepts AggregationType, Aggregation, str, or Callable.
                          Defaults to [mean, median, min, max]
            evaluator_name: Name for this aggregation (defaults to first result's evaluator)
            include_scores: Whether to include raw scores in result

        Returns:
            AggregatedResults with computed aggregations

        Examples:
            # Default aggregations
            agg = ResultAggregator.aggregate(results)

            # Custom aggregations
            agg = ResultAggregator.aggregate(results, [
                AggregationType.MEAN,
                AggregationType.P95,
                Aggregation(AggregationType.PASS_RATE, threshold=0.8)
            ])

            # With custom function
            def range_agg(scores, **kwargs):
                return max(scores) - min(scores)

            agg = ResultAggregator.aggregate(results, [
                AggregationType.MEAN,
                Aggregation(range_agg)
            ])
        """
        if not results:
            return AggregatedResults(
                evaluator_name=evaluator_name or "unknown", count=0, aggregations={}, scores=[], individual_scores=[]
            )

        # Normalize aggregations to List[Aggregation]
        agg_list = normalize_aggregations(aggregations)

        # Extract scores and trace_ids
        scores = [r.score for r in results]
        individual_scores = [(r.target_id, r.score) for r in results]

        # Get evaluator name from first result if not provided
        if evaluator_name is None:
            evaluator_name = results[0].evaluator_name

        # Compute each aggregation
        computed: Dict[str, float] = {}
        for agg in agg_list:
            try:
                value = agg.compute(scores)
                computed[agg.name] = value
            except Exception:
                # Store error but continue with other aggregations
                computed[agg.name] = float("nan")

        return AggregatedResults(
            evaluator_name=evaluator_name,
            count=len(results),
            aggregations=computed,
            scores=scores if include_scores else [],
            individual_scores=individual_scores,
        )

    @classmethod
    def aggregate_by_evaluator(
        cls,
        results: List[EvalResult],
        aggregations: Optional[List[Union[AggregationType, Aggregation]]] = None,
        include_scores: bool = False,
    ) -> Dict[str, AggregatedResults]:
        """
        Aggregate results grouped by evaluator name.

        Args:
            results: List of EvalResult objects (can be mixed evaluators)
            aggregations: Aggregations to compute for each group
            include_scores: Whether to include raw scores

        Returns:
            Dict mapping evaluator name to AggregatedResults

        Example:
            results = [result1, result2, result3, ...]  # Mixed evaluators
            aggs = ResultAggregator.aggregate_by_evaluator(results, [
                AggregationType.MEAN,
                Aggregation(AggregationType.PASS_RATE, threshold=0.7)
            ])

            print(aggs["safety-toxicity"]["mean"])
            print(aggs["llm-coherence"]["pass_rate_threshold_0.7"])
        """
        # Group by evaluator
        grouped: Dict[str, List[EvalResult]] = {}
        for result in results:
            name = result.evaluator_name
            if name not in grouped:
                grouped[name] = []
            grouped[name].append(result)

        # Aggregate each group
        return {
            name: cls.aggregate(group_results, aggregations, evaluator_name=name, include_scores=include_scores)
            for name, group_results in grouped.items()
        }

    @classmethod
    def aggregate_scores(
        cls, scores: List[float], aggregations: Optional[List[Union[AggregationType, Aggregation]]] = None
    ) -> Dict[str, float]:
        """
        Simple aggregation over raw scores (without EvalResult objects).

        Args:
            scores: List of float scores
            aggregations: Aggregations to compute

        Returns:
            Dict mapping aggregation name to value

        Example:
            scores = [0.8, 0.9, 0.7, 0.85]
            result = ResultAggregator.aggregate_scores(scores, [
                AggregationType.MEAN,
                AggregationType.MEDIAN
            ])
            # {"mean": 0.8125, "median": 0.825}
        """
        if not scores:
            return {}

        agg_list = normalize_aggregations(aggregations)

        return {agg.name: agg.compute(scores) for agg in agg_list}
