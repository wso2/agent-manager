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
Dataset matcher for matching production traces to dataset tasks.
"""
from typing import Optional, List, Tuple
import difflib

from ..models import Trace, Task, Dataset


class DatasetMatcher:
    """
    Matches production traces to dataset tasks by input similarity.
    Used in Scenario 2 (Production Monitoring) when traces don't have task_id metadata.
    """

    def __init__(
        self,
        exact_match_threshold: float = 1.0,
        fuzzy_match_threshold: float = 0.8,
        use_embeddings: bool = False
    ):
        """
        Initialize dataset matcher.

        Args:
            exact_match_threshold: Threshold for exact matching (1.0 = perfect match)
            fuzzy_match_threshold: Minimum similarity for fuzzy matching (0.0-1.0)
            use_embeddings: Whether to use embedding-based similarity (not yet implemented)
        """
        self.exact_match_threshold = exact_match_threshold
        self.fuzzy_match_threshold = fuzzy_match_threshold
        self.use_embeddings = use_embeddings

    def match_trace_to_task(
        self,
        trace: Trace,
        dataset: Dataset
    ) -> Optional[Task]:
        """
        Find which task from the dataset this trace corresponds to.

        Args:
            trace: Production trace to match
            dataset: Dataset with reference tasks

        Returns:
            Matched task if found, None otherwise

        Example:
            >>> matcher = DatasetMatcher()
            >>> matched_task = matcher.match_trace_to_task(
            ...     production_trace,
            ...     golden_dataset
            ... )
            >>> if matched_task:
            ...     print(f"Matched to {matched_task.task_id}")
        """
        trace_input = str(trace.input)

        # Step 1: Try exact match first
        for task in dataset.tasks:
            if task.input.prompt == trace_input:
                return task

        # Step 2: Try fuzzy matching
        best_match = None
        best_similarity = 0.0

        for task in dataset.tasks:
            similarity = self._compute_text_similarity(trace_input, task.input.prompt)

            if similarity > best_similarity:
                best_similarity = similarity
                best_match = task

        # Return if above threshold
        if best_similarity >= self.fuzzy_match_threshold:
            return best_match

        return None

    def match_traces_to_tasks(
        self,
        traces: List[Trace],
        dataset: Dataset
    ) -> List[Tuple[Trace, Optional[Task]]]:
        """
        Match multiple traces to dataset tasks.

        Args:
            traces: List of production traces
            dataset: Dataset with reference tasks

        Returns:
            List of (trace, matched_task) pairs. matched_task is None if no match found.

        Example:
            >>> matcher = DatasetMatcher()
            >>> matched_pairs = matcher.match_traces_to_tasks(
            ...     production_traces,
            ...     golden_dataset
            ... )
            >>> matched_count = sum(1 for _, task in matched_pairs if task is not None)
            >>> print(f"Matched {matched_count}/{len(traces)} traces")
        """
        results = []

        for trace in traces:
            matched_task = self.match_trace_to_task(trace, dataset)
            results.append((trace, matched_task))

        return results

    def get_match_statistics(
        self,
        traces: List[Trace],
        dataset: Dataset
    ) -> dict:
        """
        Get statistics about how well traces match to dataset.

        Args:
            traces: List of traces to match
            dataset: Dataset to match against

        Returns:
            Dictionary with match statistics

        Example:
            >>> stats = matcher.get_match_statistics(traces, dataset)
            >>> print(f"Match rate: {stats['match_rate']:.1%}")
            >>> print(f"Avg similarity: {stats['avg_similarity']:.2f}")
        """
        matched_pairs = self.match_traces_to_tasks(traces, dataset)

        matched_count = sum(1 for _, task in matched_pairs if task is not None)
        total_count = len(traces)

        # Calculate average similarity
        similarities = []
        for trace, task in matched_pairs:
            if task:
                similarity = self._compute_text_similarity(
                    str(trace.input),
                    task.input.prompt
                )
                similarities.append(similarity)

        avg_similarity = sum(similarities) / len(similarities) if similarities else 0.0

        return {
            "total_traces": total_count,
            "matched_traces": matched_count,
            "unmatched_traces": total_count - matched_count,
            "match_rate": matched_count / total_count if total_count > 0 else 0.0,
            "avg_similarity": avg_similarity,
            "min_similarity": min(similarities) if similarities else 0.0,
            "max_similarity": max(similarities) if similarities else 0.0
        }

    def _compute_text_similarity(self, text1: str, text2: str) -> float:
        """
        Compute similarity between two text strings.

        Uses difflib's SequenceMatcher for now.
        Could be enhanced with:
        - Embedding-based similarity (sentence transformers)
        - Edit distance
        - Token-based similarity

        Args:
            text1: First text
            text2: Second text

        Returns:
            Similarity score between 0.0 (completely different) and 1.0 (identical)
        """
        if self.use_embeddings:
            # TODO: Implement embedding-based similarity
            # This would require sentence-transformers or similar
            raise NotImplementedError("Embedding-based similarity not yet implemented")

        # Normalize texts
        text1_normalized = text1.lower().strip()
        text2_normalized = text2.lower().strip()

        # Use difflib SequenceMatcher
        similarity = difflib.SequenceMatcher(
            None,
            text1_normalized,
            text2_normalized
        ).ratio()

        return similarity
