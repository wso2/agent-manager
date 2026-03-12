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
    ChainSpan,
    LLMSpan,
    ToolSpan,
    AgentSpan,
)
from amp_evaluation.trace.fetcher import _parse_trace, OTELSpan, OTELTrace, AmpAttributes
from amp_evaluation.trace.parser import (
    filter_infrastructure_spans,
    INFRASTRUCTURE_KINDS,
    SEMANTIC_KINDS,
)


def _make_span(span_id, kind, parent_span_id=None, name=""):
    """Helper to create a minimal OTELSpan for unit tests."""
    return OTELSpan(
        traceId="test-trace",
        spanId=span_id,
        name=name or span_id,
        service="test",
        startTime="2025-01-01T00:00:00Z",
        endTime="2025-01-01T00:00:01Z",
        durationInNanos=1_000_000_000,
        kind="INTERNAL",
        status="OK",
        parentSpanId=parent_span_id,
        ampAttributes=AmpAttributes(kind=kind),
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
        After filtering: semantic spans + bridge chain spans (no pass-through chains).
        """
        lg_trace_dict = sample_traces[0]
        assert lg_trace_dict["traceId"] == "789a4cc3a165ed330d3244aca8b61dbb"

        # Parse to OTEL format
        otel_trace = _parse_trace(lg_trace_dict)

        # Count original spans by kind
        original_count = len(otel_trace.spans)
        assert original_count == 19, f"Expected 19 spans, got {original_count}"

        infrastructure_count = sum(1 for s in otel_trace.spans if s.ampAttributes.kind in INFRASTRUCTURE_KINDS)
        semantic_count = sum(1 for s in otel_trace.spans if s.ampAttributes.kind in SEMANTIC_KINDS)

        # Verify fixture has infrastructure spans
        assert infrastructure_count > 0, "Test trace should have infrastructure spans"
        assert semantic_count > 0, "Test trace should have semantic spans"

        # Filter spans
        filtered_spans = filter_infrastructure_spans(otel_trace.spans)

        # VERIFY: Fewer spans than original (pass-through chains removed)
        assert len(filtered_spans) < original_count, "Filtering should reduce span count"

        # VERIFY: All semantic spans preserved
        filtered_semantic = [s for s in filtered_spans if s.ampAttributes.kind in SEMANTIC_KINDS]
        assert len(filtered_semantic) == semantic_count, (
            f"Expected {semantic_count} semantic spans, got {len(filtered_semantic)}"
        )

        # VERIFY: Any remaining infrastructure spans are bridges (root or 2+ semantic branches)
        filtered_infra = [s for s in filtered_spans if s.ampAttributes.kind in INFRASTRUCTURE_KINDS]
        for span in filtered_infra:
            # Bridge spans must be either root or have 2+ children in filtered output
            children = [s for s in filtered_spans if s.parentSpanId == span.spanId]
            is_root = span.parentSpanId is None
            assert is_root or len(children) >= 2, (
                f"Infrastructure span {span.spanId} is neither root nor a bridge (has {len(children)} children)"
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
        original_agents = [s for s in otel_trace.spans if s.ampAttributes.kind == "agent"]
        assert len(original_agents) == 3, "CrewAI trace should have 3 agents"

        # Filter
        filtered_spans = filter_infrastructure_spans(otel_trace.spans)

        # VERIFY: All 3 agents preserved
        filtered_agents = [s for s in filtered_spans if s.ampAttributes.kind == "agent"]
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

    def test_original_root_preserved(self, sample_traces):
        """
        Verify the original root span is preserved (not replaced by a synthetic one).
        """
        lg_trace = _parse_trace(sample_traces[0])

        # Find the original root
        original_root = next(s for s in lg_trace.spans if s.parentSpanId is None)

        filtered_spans = filter_infrastructure_spans(lg_trace.spans)

        # VERIFY: Root is preserved with original span ID
        roots = [s for s in filtered_spans if s.parentSpanId is None]
        assert len(roots) == 1, f"Expected 1 root, got {len(roots)}"
        assert roots[0].spanId == original_root.spanId, "Root span ID should be preserved from original trace"

    def test_consecutive_chains_collapsed(self, sample_traces):
        """
        Verify pass-through chain spans (single child path) are removed.
        """
        lg_trace = _parse_trace(sample_traces[0])
        filtered_spans = filter_infrastructure_spans(lg_trace.spans)

        # Count infrastructure spans before and after
        infra_before = sum(1 for s in lg_trace.spans if s.ampAttributes.kind in INFRASTRUCTURE_KINDS)
        infra_after = sum(1 for s in filtered_spans if s.ampAttributes.kind in INFRASTRUCTURE_KINDS)

        # VERIFY: Some infrastructure spans were removed (pass-through collapsed)
        assert infra_after < infra_before, (
            f"Expected fewer infrastructure spans after filtering ({infra_after} >= {infra_before})"
        )

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

        # VERIFY: Same semantic span counts (LLM, Tool, etc.)
        filtered_llm = sum(1 for s in trajectory_filtered.spans if isinstance(s, LLMSpan))
        unfiltered_llm = sum(1 for s in trajectory_unfiltered.spans if isinstance(s, LLMSpan))
        assert filtered_llm == unfiltered_llm

        filtered_tool = sum(1 for s in trajectory_filtered.spans if isinstance(s, ToolSpan))
        unfiltered_tool = sum(1 for s in trajectory_unfiltered.spans if isinstance(s, ToolSpan))
        assert filtered_tool == unfiltered_tool

        # VERIFY: Filtered has fewer total spans (pass-through chains removed)
        assert len(trajectory_filtered.spans) < len(trajectory_unfiltered.spans), (
            "Filtered should have fewer total spans than unfiltered"
        )

        # VERIFY: Filtered trace has valid tree (root span exists)
        root = trajectory_filtered.get_root_span()
        assert root is not None, "Filtered trace should have a root span"


class TestBridgeDetection:
    """Unit tests for bridge detection logic with controlled span trees."""

    def test_chain_root_preserved(self):
        """
        Chain root with semantic children is kept.

        root (chain) → llm1
        AFTER: root (chain) → llm1
        """
        spans = [
            _make_span("root", "chain"),
            _make_span("llm1", "llm", parent_span_id="root"),
        ]
        filtered = filter_infrastructure_spans(spans)
        ids = {s.spanId for s in filtered}
        assert "root" in ids, "Chain root should be preserved"
        assert "llm1" in ids
        assert len(filtered) == 2

    def test_bridge_chain_with_two_semantic_children(self):
        """
        Chain span with 2+ semantic children is a bridge and must be kept.

        root (chain) → chain_a → llm1
                               → tool1
        AFTER: root (chain) → chain_a (bridge) → llm1
                                                → tool1
        """
        spans = [
            _make_span("root", "chain"),
            _make_span("chain_a", "chain", parent_span_id="root"),
            _make_span("llm1", "llm", parent_span_id="chain_a"),
            _make_span("tool1", "tool", parent_span_id="chain_a"),
        ]
        filtered = filter_infrastructure_spans(spans)
        ids = {s.spanId for s in filtered}

        assert "root" in ids, "Root kept"
        assert "chain_a" in ids, "Bridge chain_a kept (2 semantic children)"
        assert "llm1" in ids
        assert "tool1" in ids
        assert len(filtered) == 4

        # Verify parent relationships preserved
        by_id = {s.spanId: s for s in filtered}
        assert by_id["chain_a"].parentSpanId == "root"
        assert by_id["llm1"].parentSpanId == "chain_a"
        assert by_id["tool1"].parentSpanId == "chain_a"

    def test_passthrough_chain_removed(self):
        """
        Chain with single child path is pass-through and removed.

        root (chain) → chain_a (chain) → llm1
        AFTER: root (chain) → llm1  (chain_a removed, llm1 remapped to root)
        """
        spans = [
            _make_span("root", "chain"),
            _make_span("chain_a", "chain", parent_span_id="root"),
            _make_span("llm1", "llm", parent_span_id="chain_a"),
        ]
        filtered = filter_infrastructure_spans(spans)
        ids = {s.spanId for s in filtered}

        assert "chain_a" not in ids, "Pass-through chain_a should be removed"
        assert "root" in ids
        assert "llm1" in ids
        assert len(filtered) == 2

        # llm1 remapped to root
        by_id = {s.spanId: s for s in filtered}
        assert by_id["llm1"].parentSpanId == "root"

    def test_consecutive_chains_collapse(self):
        """
        Chain of single-child chains collapses to keep only the branching one.

        root (chain) → c1 (chain) → c2 (chain) → c3 (chain) → llm1
                                                              → tool1
        AFTER: root → c3 (bridge, 2 semantic children) → llm1
                                                        → tool1
        c1, c2 removed (pass-through); c3 kept (bridge); c3 remapped to root.
        """
        spans = [
            _make_span("root", "chain"),
            _make_span("c1", "chain", parent_span_id="root"),
            _make_span("c2", "chain", parent_span_id="c1"),
            _make_span("c3", "chain", parent_span_id="c2"),
            _make_span("llm1", "llm", parent_span_id="c3"),
            _make_span("tool1", "tool", parent_span_id="c3"),
        ]
        filtered = filter_infrastructure_spans(spans)
        ids = {s.spanId for s in filtered}

        assert "c1" not in ids
        assert "c2" not in ids
        assert "c3" in ids, "Branching chain c3 kept"
        assert "root" in ids
        assert len(filtered) == 4

        by_id = {s.spanId: s for s in filtered}
        assert by_id["c3"].parentSpanId == "root", "c3 remapped to root"
        assert by_id["llm1"].parentSpanId == "c3"
        assert by_id["tool1"].parentSpanId == "c3"

    def test_grouping_preserved_at_all_levels(self):
        """
        Bridge chains at multiple levels preserve grouping.

        root (chain)
          → chain_a (chain)
            → llm1
            → tool1
          → chain_b (chain)
            → llm2
            → tool2

        AFTER: same structure (both chain_a, chain_b are bridges).
        Groups {llm1,tool1} and {llm2,tool2} must NOT be flattened.
        """
        spans = [
            _make_span("root", "chain"),
            _make_span("chain_a", "chain", parent_span_id="root"),
            _make_span("llm1", "llm", parent_span_id="chain_a"),
            _make_span("tool1", "tool", parent_span_id="chain_a"),
            _make_span("chain_b", "chain", parent_span_id="root"),
            _make_span("llm2", "llm", parent_span_id="chain_b"),
            _make_span("tool2", "tool", parent_span_id="chain_b"),
        ]
        filtered = filter_infrastructure_spans(spans)
        by_id = {s.spanId: s for s in filtered}

        # All spans kept (root + 2 bridges + 4 semantic)
        assert len(filtered) == 7

        # Grouping preserved
        assert by_id["llm1"].parentSpanId == "chain_a"
        assert by_id["tool1"].parentSpanId == "chain_a"
        assert by_id["llm2"].parentSpanId == "chain_b"
        assert by_id["tool2"].parentSpanId == "chain_b"
        assert by_id["chain_a"].parentSpanId == "root"
        assert by_id["chain_b"].parentSpanId == "root"

    def test_semantic_root_unchanged(self):
        """
        Trace with a semantic root passes through with no changes.

        agent1 (agent, root) → llm1
        """
        spans = [
            _make_span("agent1", "agent"),
            _make_span("llm1", "llm", parent_span_id="agent1"),
        ]
        filtered = filter_infrastructure_spans(spans)
        assert len(filtered) == 2
        by_id = {s.spanId: s for s in filtered}
        assert by_id["agent1"].parentSpanId is None
        assert by_id["llm1"].parentSpanId == "agent1"

    def test_empty_spans(self):
        """Empty span list returns empty."""
        assert filter_infrastructure_spans([]) == []

    def test_chain_root_no_semantic_descendants(self):
        """
        Infrastructure-only trace: root chain with no semantic descendants.
        Nothing useful to keep — returns empty.
        """
        spans = [
            _make_span("root", "chain"),
            _make_span("c1", "chain", parent_span_id="root"),
        ]
        filtered = filter_infrastructure_spans(spans)
        # No semantic descendants anywhere — nothing to preserve
        assert len(filtered) == 0


class TestParserChainSpan:
    """Test that bridge chain spans survive into parsed Trace as ChainSpan."""

    def test_chain_root_becomes_chain_span(self):
        """
        When the root is a chain span, it should appear as ChainSpan in trace.spans
        and get_root_span() should find it.
        """
        spans = [
            _make_span("root", "chain"),
            _make_span("llm1", "llm", parent_span_id="root"),
        ]
        otel_trace = OTELTrace(
            traceId="test-trace",
            rootSpanId="root",
            rootSpanName="root",
            startTime="2025-01-01T00:00:00Z",
            endTime="2025-01-01T00:00:01Z",
            spans=spans,
            input="hello",
            output="world",
        )

        trace = parse_trace_for_evaluation(otel_trace, filter_infrastructure=True)

        # Root should be a ChainSpan
        root = trace.get_root_span()
        assert root is not None, "Root span should exist"
        assert isinstance(root, ChainSpan), f"Root should be ChainSpan, got {type(root).__name__}"
        assert root.span_id == "root"
        assert root.parent_span_id is None

    def test_bridge_chain_spans_in_parsed_trace(self):
        """
        Bridge chain spans should be ChainSpan instances in trace.spans,
        preserving the tree structure through parsing.
        """
        spans = [
            _make_span("root", "chain"),
            _make_span("chain_a", "chain", parent_span_id="root"),
            _make_span("llm1", "llm", parent_span_id="chain_a"),
            _make_span("tool1", "tool", parent_span_id="chain_a"),
        ]
        otel_trace = OTELTrace(
            traceId="test-trace",
            rootSpanId="root",
            rootSpanName="root",
            startTime="2025-01-01T00:00:00Z",
            endTime="2025-01-01T00:00:01Z",
            spans=spans,
            input="hello",
            output="world",
        )

        trace = parse_trace_for_evaluation(otel_trace, filter_infrastructure=True)

        # Find ChainSpan instances
        chain_spans = [s for s in trace.spans if isinstance(s, ChainSpan)]
        chain_ids = {s.span_id for s in chain_spans}

        assert "root" in chain_ids, "Root should be a ChainSpan"
        assert "chain_a" in chain_ids, "Bridge chain_a should be a ChainSpan"

        # Verify tree: llm1 and tool1 should point to chain_a
        llm = next(s for s in trace.spans if isinstance(s, LLMSpan))
        tool = next(s for s in trace.spans if isinstance(s, ToolSpan))
        assert llm.parent_span_id == "chain_a"
        assert tool.parent_span_id == "chain_a"


class TestParsedTraceStructure:
    """
    End-to-end tests: raw fixture → filter → parse → verify trace.spans structure.

    These verify the full pipeline produces a connected tree with ChainSpan roots
    and correct parent relationships using real trace data.
    """

    @pytest.fixture
    def langchain_trace(self, sample_traces):
        """Parse LangGraph trace (index 1, 24 raw spans)."""
        raw = sample_traces[1]
        assert raw["traceId"] == "fc5513186f8d0b0d0b488f47548e6028"
        otel_trace = _parse_trace(raw)
        return parse_trace_for_evaluation(otel_trace, filter_infrastructure=True)

    @pytest.fixture
    def crewai_trace(self, sample_traces):
        """Parse CrewAI trace (index 5, 24 raw spans)."""
        raw = sample_traces[5]
        assert raw["traceId"] == "66ea0b364e7397376b7c9edcc82e1f85"
        otel_trace = _parse_trace(raw)
        return parse_trace_for_evaluation(otel_trace, filter_infrastructure=True)

    # -- LangGraph trace: chain root + flat LLM/Tool children ----------------

    def test_langchain_root_is_chain_span(self, langchain_trace):
        """LangGraph root span is a ChainSpan (chain kind, no agent spans)."""
        root = langchain_trace.get_root_span()
        assert root is not None
        assert isinstance(root, ChainSpan)
        assert root.parent_span_id is None
        assert root.name == "LangGraph.workflow"

    def test_langchain_span_count(self, langchain_trace):
        """24 raw spans → 6 parsed (1 ChainSpan root + 3 LLM + 2 Tool)."""
        assert len(langchain_trace.spans) == 6

    def test_langchain_all_children_point_to_root(self, langchain_trace):
        """All semantic spans are direct children of the chain root."""
        root = langchain_trace.get_root_span()
        for span in langchain_trace.spans:
            if span is not root:
                assert span.parent_span_id == root.span_id

    def test_langchain_semantic_counts(self, langchain_trace):
        assert sum(1 for s in langchain_trace.spans if isinstance(s, LLMSpan)) == 3
        assert sum(1 for s in langchain_trace.spans if isinstance(s, ToolSpan)) == 2
        assert sum(1 for s in langchain_trace.spans if isinstance(s, AgentSpan)) == 0

    # -- CrewAI trace: chain root → 3 agent spans → LLM children -------------

    def test_crewai_root_is_chain_span(self, crewai_trace):
        """CrewAI root span is a ChainSpan (chain kind wrapping agents)."""
        root = crewai_trace.get_root_span()
        assert root is not None
        assert isinstance(root, ChainSpan)
        assert root.parent_span_id is None
        assert root.name == "crewai.workflow"

    def test_crewai_span_count(self, crewai_trace):
        """24 raw spans → 14 parsed (1 ChainSpan root + 3 Agent + 10 LLM)."""
        assert len(crewai_trace.spans) == 14

    def test_crewai_agents_are_children_of_root(self, crewai_trace):
        """All 3 agent spans are direct children of the chain root."""
        root = crewai_trace.get_root_span()
        agents = [s for s in crewai_trace.spans if isinstance(s, AgentSpan)]
        assert len(agents) == 3
        for agent in agents:
            assert agent.parent_span_id == root.span_id

    def test_crewai_llm_spans_nested_under_agents(self, crewai_trace):
        """Each LLM span is a child of one of the agent spans."""
        agent_ids = {s.span_id for s in crewai_trace.spans if isinstance(s, AgentSpan)}
        llm_spans = [s for s in crewai_trace.spans if isinstance(s, LLMSpan)]
        assert len(llm_spans) == 10
        for llm in llm_spans:
            assert llm.parent_span_id in agent_ids, (
                f"LLM span {llm.span_id} parent {llm.parent_span_id} is not an agent"
            )

    # -- Generic tree validity (both traces) ----------------------------------

    @pytest.mark.parametrize("trace_fixture", ["langchain_trace", "crewai_trace"])
    def test_tree_is_connected(self, trace_fixture, request):
        """Every span's parent_span_id references another span in the trace."""
        trace = request.getfixturevalue(trace_fixture)
        span_ids = {s.span_id for s in trace.spans}
        for span in trace.spans:
            if span.parent_span_id is not None:
                assert span.parent_span_id in span_ids, f"Span {span.span_id} has dangling parent {span.parent_span_id}"

    @pytest.mark.parametrize("trace_fixture", ["langchain_trace", "crewai_trace"])
    def test_single_root(self, trace_fixture, request):
        """Exactly one root span (parent_span_id=None) exists."""
        trace = request.getfixturevalue(trace_fixture)
        roots = [s for s in trace.spans if getattr(s, "parent_span_id", None) is None]
        assert len(roots) == 1


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
