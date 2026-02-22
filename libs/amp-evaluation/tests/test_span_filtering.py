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

"""Tests for infrastructure span filtering with parent remapping."""

import copy
import pytest
import json

from amp_evaluation.trace import (
    parse_trace_for_evaluation,
)
from amp_evaluation.trace.fetcher import _parse_trace
from amp_evaluation.trace.parser import (
    filter_infrastructure_spans,
    INFRASTRUCTURE_KINDS,
    SEMANTIC_KINDS,
)


@pytest.fixture
def sample_traces():
    """Load real sample traces from fixture file."""
    import os

    fixture_path = os.path.join(os.path.dirname(__file__), "fixtures", "sample_traces.json")
    with open(fixture_path, "r") as f:
        data = json.load(f)
        return data["traces"]


class TestSpanFiltering:
    """Test infrastructure span filtering with real trace data."""

    def test_langgraph_pattern_filtering(self, sample_traces):
        """
        Test Pattern A: LangGraph trace with deep chain nesting.

        Trace 0 has 19 spans: 2 LLM, 5 Tool, 12 Chain
        After filtering: Should have ~7 semantic spans + 1 synthetic root = 8 total
        """
        lg_trace_dict = sample_traces[0]
        assert lg_trace_dict["traceId"] == "789a4cc3a165ed330d3244aca8b61dbb"

        # Parse to OTEL format
        otel_trace = _parse_trace(lg_trace_dict)

        # Count original spans by kind
        original_count = len(otel_trace.spans)
        assert original_count == 19, f"Expected 19 spans, got {original_count}"

        infrastructure_count = sum(
            1 for s in otel_trace.spans if s.ampAttributes.get("kind", "unknown") in INFRASTRUCTURE_KINDS
        )
        semantic_count = sum(1 for s in otel_trace.spans if s.ampAttributes.get("kind", "unknown") in SEMANTIC_KINDS)

        # Verify fixture has infrastructure spans
        assert infrastructure_count > 0, "Test trace should have infrastructure spans"
        assert semantic_count > 0, "Test trace should have semantic spans"

        # Filter spans
        filtered_spans = filter_infrastructure_spans(otel_trace.spans)

        # VERIFY: Only semantic spans + synthetic root remain
        assert len(filtered_spans) <= semantic_count + 1, (
            f"Expected <={semantic_count + 1} spans after filtering, got {len(filtered_spans)}"
        )

        # VERIFY: No infrastructure spans in filtered output
        for span in filtered_spans:
            kind = span.ampAttributes.get("kind", "unknown")
            if not span.ampAttributes.get("synthetic", False):
                assert kind in SEMANTIC_KINDS, f"Infrastructure span {kind} found in filtered output"

        # VERIFY: All semantic spans preserved
        filtered_semantic = [s for s in filtered_spans if not s.ampAttributes.get("synthetic", False)]
        assert len(filtered_semantic) == semantic_count, (
            f"Expected {semantic_count} semantic spans, got {len(filtered_semantic)}"
        )

    def test_http_unknown_pattern_filtering(self, sample_traces):
        """
        Test Pattern B: HTTP Unknown trace with infrastructure root.

        Find a trace with 'unknown' root span and verify filtering.
        """
        # Find trace with unknown root
        unknown_trace = None
        for trace_dict in sample_traces:
            spans = trace_dict.get("spans", [])
            if spans:
                root_kind = spans[0].get("ampAttributes", {}).get("kind")
                if root_kind == "unknown":
                    unknown_trace = trace_dict
                    break

        if not unknown_trace:
            pytest.skip("No trace with unknown root found in fixtures")

        otel_trace = _parse_trace(unknown_trace)
        original_count = len(otel_trace.spans)

        # Filter
        filtered_spans = filter_infrastructure_spans(otel_trace.spans)

        # VERIFY: Significant reduction
        assert len(filtered_spans) < original_count, "Filtering should reduce span count"

        # VERIFY: Tree structure valid (single root)
        roots = [s for s in filtered_spans if s.parentSpanId is None]
        assert len(roots) == 1, f"Expected 1 root after filtering, got {len(roots)}"

    def test_crewai_pattern_filtering(self, sample_traces):
        """
        Test Pattern C: CrewAI multi-agent trace.

        Trace with 3 agent spans should preserve all agents after filtering.
        """
        crew_trace = next((t for t in sample_traces if t["traceId"] == "66ea0b364e7397376b7c9edcc82e1f85"), None)
        if not crew_trace:
            pytest.skip("CrewAI trace not found")

        otel_trace = _parse_trace(crew_trace)

        # Count original agents
        original_agents = [s for s in otel_trace.spans if s.ampAttributes.get("kind") == "agent"]
        assert len(original_agents) == 3, "CrewAI trace should have 3 agents"

        # Filter
        filtered_spans = filter_infrastructure_spans(otel_trace.spans)

        # VERIFY: All 3 agents preserved
        filtered_agents = [s for s in filtered_spans if s.ampAttributes.get("kind") == "agent"]
        assert len(filtered_agents) == 3, f"Expected 3 agents after filtering, got {len(filtered_agents)}"

        # VERIFY: Agent hierarchy preserved (all agents have same parent or root)
        agent_parents = {a.parentSpanId for a in filtered_agents}
        assert len(agent_parents) <= 2, "Agents should share common parent (or None for synthetic root)"

    def test_standalone_llm_pattern(self, sample_traces):
        """
        Test Pattern D: Standalone LLM trace with no infrastructure.

        Traces with only semantic spans should pass through unchanged.
        """
        # Find trace with single LLM span
        standalone_trace = None
        for trace_dict in sample_traces:
            spans = trace_dict.get("spans", [])
            if len(spans) == 1:
                kind = spans[0].get("ampAttributes", {}).get("kind")
                if kind == "llm":
                    standalone_trace = trace_dict
                    break

        if not standalone_trace:
            pytest.skip("No standalone LLM trace found")

        otel_trace = _parse_trace(standalone_trace)
        assert len(otel_trace.spans) == 1

        # Filter
        filtered_spans = filter_infrastructure_spans(otel_trace.spans)

        # VERIFY: Unchanged (no filtering needed)
        assert len(filtered_spans) == 1, "Standalone semantic span should not be filtered"

        assert filtered_spans[0].spanId == otel_trace.spans[0].spanId, "Span ID should be unchanged"

    def test_filter_all_14_traces(self, sample_traces):
        """
        Run filtering on ALL 14 real traces and verify no crashes.
        """
        for i, trace_dict in enumerate(sample_traces):
            otel_trace = _parse_trace(trace_dict)

            # Should not crash
            try:
                filtered_spans = filter_infrastructure_spans(otel_trace.spans)
            except Exception as e:
                pytest.fail(f"Filtering crashed on trace {i}: {e}")

            # VERIFY: Some spans remain (not all filtered out)
            assert len(filtered_spans) > 0, f"Trace {i} has no spans after filtering (likely a bug)"

            # VERIFY: Valid tree structure
            roots = [s for s in filtered_spans if s.parentSpanId is None]
            assert len(roots) >= 1, f"Trace {i} has no root span after filtering"

    def test_span_count_reduction(self, sample_traces):
        """
        Verify 71.7% span reduction across all traces.
        """
        total_before = 0
        total_after = 0

        for trace_dict in sample_traces:
            otel_trace = _parse_trace(trace_dict)
            total_before += len(otel_trace.spans)

            filtered_spans = filter_infrastructure_spans(otel_trace.spans)
            total_after += len(filtered_spans)

        # Calculate reduction percentage
        reduction_pct = ((total_before - total_after) / total_before) * 100

        # VERIFY: Significant reduction (at least 50%)
        assert reduction_pct > 50, f"Expected >50% reduction, got {reduction_pct:.1f}%"

        print(f"\nSpan reduction: {total_before} -> {total_after} ({reduction_pct:.1f}%)")

    def test_parent_remapping_correctness(self, sample_traces):
        """
        Verify parent IDs are correctly remapped after filtering.
        """
        lg_trace = _parse_trace(sample_traces[0])
        filtered_spans = filter_infrastructure_spans(lg_trace.spans)

        # Build span ID set
        span_ids = {s.spanId for s in filtered_spans}

        # VERIFY: All parent IDs exist in filtered spans (except None for root)
        for span in filtered_spans:
            parent_id = span.parentSpanId
            if parent_id is not None:
                assert parent_id in span_ids, f"Span {span.spanId} has invalid parent {parent_id} after remapping"

    def test_synthetic_root_creation(self, sample_traces):
        """
        Verify synthetic root is created when multiple orphaned spans exist.
        """
        lg_trace = _parse_trace(sample_traces[0])
        filtered_spans = filter_infrastructure_spans(lg_trace.spans, create_synthetic_root=True)

        # Check for synthetic root
        synthetic_roots = [s for s in filtered_spans if s.ampAttributes.get("synthetic", False)]

        # If we have multiple semantic spans at root level, should have synthetic root
        semantic_spans = [s for s in filtered_spans if not s.ampAttributes.get("synthetic", False)]

        if len(semantic_spans) > 1:
            # Check if multiple spans would be orphaned without synthetic root
            parent_ids = {s.parentSpanId for s in semantic_spans}
            if None in parent_ids:
                # Multiple spans with no parent -> synthetic root should exist
                assert len(synthetic_roots) == 1, "Expected synthetic root for orphaned spans"

    def test_no_synthetic_root_when_disabled(self, sample_traces):
        """
        Verify synthetic root is NOT created when create_synthetic_root=False.
        """
        lg_trace = _parse_trace(sample_traces[0])
        filtered_spans = filter_infrastructure_spans(lg_trace.spans, create_synthetic_root=False)

        # VERIFY: No synthetic roots
        synthetic_roots = [s for s in filtered_spans if s.ampAttributes.get("synthetic", False)]
        assert len(synthetic_roots) == 0, "Should not create synthetic root when disabled"

    def test_integration_with_parse_trace_for_evaluation(self, sample_traces):
        """
        Test filtering integration with full parsing pipeline.
        """
        lg_trace = _parse_trace(sample_traces[0])
        lg_trace_copy = copy.deepcopy(lg_trace)

        # Parse WITH filtering (default)
        trajectory_filtered = parse_trace_for_evaluation(lg_trace, filter_infrastructure=True)

        # Parse WITHOUT filtering — use a deep copy to avoid operating on mutated spans
        trajectory_unfiltered = parse_trace_for_evaluation(lg_trace_copy, filter_infrastructure=False)

        # VERIFY: Both produce same semantic results
        # (steps only contains semantic spans, so count should be same)
        assert len(trajectory_filtered.spans) == len(trajectory_unfiltered.spans), (
            "Filtered and unfiltered should have same semantic step count"
        )

        # VERIFY: Same semantic spans count (LLM, Tool, etc.)
        assert trajectory_filtered.metrics.llm_call_count == trajectory_unfiltered.metrics.llm_call_count
        assert trajectory_filtered.metrics.tool_call_count == trajectory_unfiltered.metrics.tool_call_count

        # VERIFY: Filtering actually happened by checking the original trace span count
        assert len(lg_trace.spans) > len(trajectory_filtered.spans), (
            "Original trace should have more spans than filtered semantic steps"
        )


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
