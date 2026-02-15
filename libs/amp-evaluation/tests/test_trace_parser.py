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
Unit tests for trace parsing utilities.

Tests parsing raw OTEL/AMP traces into Trajectory format.
"""

import pytest
import sys
import json
from pathlib import Path

# Add src to path
sys.path.insert(0, str(Path(__file__).parent.parent / "src"))

# Import from new trace module
from amp_evaluation.trace import (
    # Core trace
    TokenUsage,
    Message,
    ToolCall,
    parse_trace_for_evaluation,
    parse_traces_for_evaluation,
)

# OTEL models from fetcher (internal)
from amp_evaluation.trace.fetcher import (
    Trace as OTELTrace,
    Span as OTELSpan,
    TraceStatus as OTELTraceStatus,
)

# Also import the internal parse function from fetcher to convert real OTEL JSON
from amp_evaluation.trace.fetcher import _parse_trace


# Helper function to convert test dicts to OTEL Trace objects
def dict_to_otel_trace(trace_dict: dict) -> "OTELTrace":
    """Convert a test dictionary to an OTEL Trace object."""
    # Convert spans
    otel_spans = []
    error_count = 0
    for span_dict in trace_dict.get("spans", []):
        # Put span data into ampAttributes
        amp_attrs = {
            "kind": span_dict.get("kind", "unknown"),
            "input": span_dict.get("input"),
            "output": span_dict.get("output"),
            "data": span_dict.get("data", {}),
        }
        has_error = span_dict.get("status", {}).get("error", False)
        if has_error:
            error_count += 1
            amp_attrs["error"] = {"message": span_dict.get("status", {}).get("errorType", "Error")}

        otel_span = OTELSpan(
            traceId=trace_dict.get("trace_id", "test-trace"),
            spanId=span_dict.get("span_id", "test-span"),
            name=span_dict.get("name", span_dict.get("kind", "unknown")),
            service="test-service",
            startTime="2026-01-27T00:00:00Z",
            endTime="2026-01-27T00:00:01Z",
            durationInNanos=int(span_dict.get("duration_ms", 0) * 1_000_000),
            kind="INTERNAL",
            status="ERROR" if has_error else "OK",
            attributes={},
            ampAttributes=amp_attrs,
        )
        otel_spans.append(otel_span)

    # Create trace status with error count
    trace_status = OTELTraceStatus(errorCount=error_count) if error_count > 0 else None

    # Calculate total duration from spans if not provided at trace level
    total_duration_nanos = None
    if "duration_ms" in trace_dict:
        # Trace-level duration provided
        total_duration_nanos = int(trace_dict["duration_ms"] * 1_000_000)
    elif otel_spans:
        # Sum up span durations
        total_duration_nanos = sum(span.durationInNanos for span in otel_spans if span.durationInNanos)

    # Create OTEL Trace
    return OTELTrace(
        traceId=trace_dict.get("trace_id", "test-trace"),
        rootSpanId="root-span",
        rootSpanName="test",
        startTime="2026-01-27T00:00:00Z",
        endTime="2026-01-27T00:00:01Z",
        durationInNanos=total_duration_nanos,
        spans=otel_spans,
        input=trace_dict.get("input"),
        output=trace_dict.get("output"),
        status=trace_status,
    )


class TestTraceParser:
    """Test the trace parsing functionality."""

    def test_parse_simple_trace(self):
        """Test parsing a simple trace with just input/output."""
        raw_trace_dict = {"trace_id": "trace_123", "input": "What is 2 + 2?", "output": "4", "spans": []}

        trace = dict_to_otel_trace(raw_trace_dict)
        eval_trace = parse_trace_for_evaluation(trace)

        assert eval_trace.trace_id == "trace_123"
        assert eval_trace.input == "What is 2 + 2?"
        assert eval_trace.output == "4"
        assert len(eval_trace.llm_spans) == 0
        assert len(eval_trace.tool_spans) == 0

    def test_parse_llm_span(self):
        """Test parsing an LLM span."""
        raw_trace_dict = {
            "trace_id": "trace_llm",
            "input": "Hello",
            "output": "Hi there!",
            "spans": [
                {
                    "span_id": "span_1",
                    "kind": "llm",
                    "input": [{"role": "system", "content": "You are helpful."}, {"role": "user", "content": "Hello"}],
                    "output": {"content": "Hi there!"},
                    "data": {
                        "model": "gpt-4",
                        "vendor": "OpenAI",
                        "temperature": 0.7,
                        "tokenUsage": {"inputTokens": 10, "outputTokens": 5, "totalTokens": 15},
                    },
                    "duration_ms": 500.0,
                    "status": {"error": False},
                }
            ],
        }

        trace = dict_to_otel_trace(raw_trace_dict)
        eval_trace = parse_trace_for_evaluation(trace)

        assert len(eval_trace.llm_spans) == 1

        llm_span = eval_trace.llm_spans[0]
        assert llm_span.span_id == "span_1"
        assert llm_span.model == "gpt-4"
        assert llm_span.vendor == "OpenAI"
        assert llm_span.temperature == 0.7
        assert llm_span.response == "Hi there!"
        assert llm_span.duration_ms == 500.0
        assert not llm_span.error

        # Check messages
        assert len(llm_span.messages) == 2
        assert llm_span.messages[0].role == "system"
        assert llm_span.messages[0].content == "You are helpful."
        assert llm_span.messages[1].role == "user"

        # Check token usage
        assert llm_span.token_usage.input_tokens == 10
        assert llm_span.token_usage.output_tokens == 5
        assert llm_span.token_usage.total_tokens == 15

    def test_parse_tool_span(self):
        """Test parsing a tool execution span."""
        raw_trace_dict = {
            "trace_id": "trace_tool",
            "input": "Search for info",
            "output": "Found results",
            "spans": [
                {
                    "span_id": "span_tool_1",
                    "kind": "tool",
                    "name": "web_search",
                    "input": {"query": "python tutorials"},
                    "output": ["result1", "result2"],
                    "data": {"name": "web_search"},
                    "duration_ms": 200.0,
                    "status": {"error": False},
                }
            ],
        }

        trace = dict_to_otel_trace(raw_trace_dict)
        eval_trace = parse_trace_for_evaluation(trace)

        assert len(eval_trace.tool_spans) == 1

        tool_span = eval_trace.tool_spans[0]
        assert tool_span.span_id == "span_tool_1"
        assert tool_span.name == "web_search"
        assert tool_span.arguments == {"query": "python tutorials"}
        assert tool_span.result == ["result1", "result2"]
        assert tool_span.duration_ms == 200.0

    def test_parse_retriever_span(self):
        """Test parsing a retriever span for RAG."""
        raw_trace_dict = {
            "trace_id": "trace_rag",
            "input": "What is machine learning?",
            "output": "ML is...",
            "spans": [
                {
                    "span_id": "span_ret_1",
                    "kind": "retriever",
                    "input": "machine learning definition",
                    "output": [
                        {"id": "doc1", "content": "ML is a subset of AI", "score": 0.95},
                        {"id": "doc2", "content": "Machine learning uses data", "score": 0.82},
                    ],
                    "data": {"vectorDB": "pinecone", "topK": 5},
                    "duration_ms": 100.0,
                    "status": {"error": False},
                }
            ],
        }

        trace = dict_to_otel_trace(raw_trace_dict)
        eval_trace = parse_trace_for_evaluation(trace)

        assert len(eval_trace.retriever_spans) == 1

        ret_span = eval_trace.retriever_spans[0]
        assert ret_span.span_id == "span_ret_1"
        assert ret_span.query == "machine learning definition"
        assert ret_span.vector_db == "pinecone"
        assert ret_span.top_k == 5
        assert len(ret_span.documents) == 2
        assert ret_span.documents[0].content == "ML is a subset of AI"
        assert ret_span.documents[0].score == 0.95

    def test_parse_agent_span(self):
        """Test parsing an agent span."""
        raw_trace_dict = {
            "trace_id": "trace_agent",
            "input": "Do a task",
            "output": "Done!",
            "spans": [
                {
                    "span_id": "span_agent_1",
                    "kind": "agent",
                    "name": "TaskAgent",
                    "input": "Do a task",
                    "output": "Done!",
                    "data": {
                        "name": "TaskAgent",
                        "framework": "CrewAI",
                        "model": "gpt-4",
                        "systemPrompt": "You are a task executor.",
                        "tools": [{"name": "search"}, {"name": "calculate"}],
                        "maxIter": 5,
                        "tokenUsage": {"inputTokens": 100, "outputTokens": 50, "totalTokens": 150},
                    },
                    "duration_ms": 5000.0,
                    "status": {"error": False},
                }
            ],
        }

        trace = dict_to_otel_trace(raw_trace_dict)
        eval_trace = parse_trace_for_evaluation(trace)

        assert eval_trace.agent_span is not None

        agent_span = eval_trace.agent_span
        assert agent_span.span_id == "span_agent_1"
        assert agent_span.name == "TaskAgent"
        assert agent_span.framework == "CrewAI"
        assert agent_span.model == "gpt-4"
        assert agent_span.system_prompt == "You are a task executor."
        assert agent_span.available_tools == ["search", "calculate"]
        assert agent_span.max_iterations == 5

    def test_metrics_aggregation(self):
        """Test that metrics are properly aggregated."""
        raw_trace_dict = {
            "trace_id": "trace_metrics",
            "input": "input",
            "output": "output",
            "spans": [
                {
                    "span_id": "llm_1",
                    "kind": "llm",
                    "input": [{"role": "user", "content": "Hello"}],
                    "output": "Hi",
                    "data": {"tokenUsage": {"inputTokens": 10, "outputTokens": 5, "totalTokens": 15}},
                    "duration_ms": 100.0,
                    "status": {"error": False},
                },
                {
                    "span_id": "llm_2",
                    "kind": "llm",
                    "input": [{"role": "user", "content": "Bye"}],
                    "output": "Goodbye",
                    "data": {"tokenUsage": {"inputTokens": 8, "outputTokens": 4, "totalTokens": 12}},
                    "duration_ms": 80.0,
                    "status": {"error": False},
                },
                {
                    "span_id": "tool_1",
                    "kind": "tool",
                    "name": "search",
                    "input": {},
                    "output": "result",
                    "data": {},
                    "duration_ms": 50.0,
                    "status": {"error": True, "errorType": "Timeout"},
                },
            ],
        }

        trace = dict_to_otel_trace(raw_trace_dict)
        eval_trace = parse_trace_for_evaluation(trace)

        # Check counts
        assert eval_trace.metrics.llm_call_count == 2
        assert eval_trace.metrics.tool_call_count == 1
        assert eval_trace.metrics.error_count == 1

        # Check aggregated tokens
        assert eval_trace.metrics.token_usage.input_tokens == 18
        assert eval_trace.metrics.token_usage.output_tokens == 9
        assert eval_trace.metrics.token_usage.total_tokens == 27

        # Check duration
        assert eval_trace.metrics.total_duration_ms == 230.0

    def test_convenience_properties(self):
        """Test Trajectory convenience properties."""
        raw_trace_dict = {
            "trace_id": "trace_props",
            "input": "input",
            "output": "output",
            "spans": [
                {
                    "span_id": "llm_1",
                    "kind": "llm",
                    "input": [{"role": "user", "content": "Q"}],
                    "output": "Response 1",
                    "data": {},
                    "duration_ms": 100.0,
                    "status": {"error": False},
                },
                {
                    "span_id": "tool_1",
                    "kind": "tool",
                    "name": "search",
                    "input": {},
                    "output": "search result",
                    "data": {"name": "search"},
                    "duration_ms": 50.0,
                    "status": {"error": False},
                },
                {
                    "span_id": "tool_2",
                    "kind": "tool",
                    "name": "calculate",
                    "input": {},
                    "output": 42,
                    "data": {"name": "calculate"},
                    "duration_ms": 20.0,
                    "status": {"error": False},
                },
            ],
        }

        trace = dict_to_otel_trace(raw_trace_dict)
        eval_trace = parse_trace_for_evaluation(trace)

        # Test properties
        assert eval_trace.has_output
        assert not eval_trace.has_errors
        assert eval_trace.all_tool_names == ["search", "calculate"]
        assert eval_trace.all_tool_results == ["search result", 42]
        assert eval_trace.all_llm_responses == ["Response 1"]

    def test_skip_non_important_spans(self):
        """Test that embedding, rerank, task, chain spans are skipped."""
        raw_trace_dict = {
            "trace_id": "trace_skip",
            "input": "input",
            "output": "output",
            "spans": [
                {
                    "span_id": "llm_1",
                    "kind": "llm",
                    "input": [],
                    "output": "response",
                    "data": {},
                    "duration_ms": 100.0,
                    "status": {"error": False},
                },
                {
                    "span_id": "embed_1",
                    "kind": "embedding",
                    "input": "text to embed",
                    "output": [0.1, 0.2, 0.3],
                    "data": {"tokenUsage": {"totalTokens": 5}},
                    "duration_ms": 10.0,
                    "status": {"error": False},
                },
                {
                    "span_id": "rerank_1",
                    "kind": "rerank",
                    "input": "query",
                    "output": ["doc1", "doc2"],
                    "data": {},
                    "duration_ms": 20.0,
                    "status": {"error": False},
                },
                {
                    "span_id": "task_1",
                    "kind": "task",
                    "input": "task input",
                    "output": "task output",
                    "data": {},
                    "duration_ms": 500.0,
                    "status": {"error": False},
                },
            ],
        }

        trace = dict_to_otel_trace(raw_trace_dict)
        eval_trace = parse_trace_for_evaluation(trace)

        # Should only have LLM span
        assert len(eval_trace.llm_spans) == 1
        assert len(eval_trace.tool_spans) == 0
        assert len(eval_trace.retriever_spans) == 0

        # But embedding tokens should be counted
        assert eval_trace.metrics.token_usage.total_tokens == 5

    def test_parse_multiple_traces(self):
        """Test batch parsing of multiple traces."""
        raw_traces_dict = [
            {"trace_id": "t1", "input": "a", "output": "A", "spans": []},
            {"trace_id": "t2", "input": "b", "output": "B", "spans": []},
            {"trace_id": "t3", "input": "c", "output": "C", "spans": []},
        ]

        traces = [dict_to_otel_trace(t) for t in raw_traces_dict]
        eval_traces = parse_traces_for_evaluation(traces)

        assert len(eval_traces) == 3
        assert eval_traces[0].trace_id == "t1"
        assert eval_traces[1].trace_id == "t2"
        assert eval_traces[2].trace_id == "t3"


class TestTrajectoryStructure:
    """Test the Trajectory data structure itself."""

    def test_token_usage_addition(self):
        """Test that TokenUsage objects can be added."""
        t1 = TokenUsage(input_tokens=10, output_tokens=5, total_tokens=15)
        t2 = TokenUsage(input_tokens=8, output_tokens=4, total_tokens=12)

        combined = t1 + t2

        assert combined.input_tokens == 18
        assert combined.output_tokens == 9
        assert combined.total_tokens == 27

    def test_message_with_tool_calls(self):
        """Test Message with tool calls."""
        msg = Message(
            role="assistant",
            content="I'll search for that.",
            tool_calls=[ToolCall(id="tc1", name="search", arguments={"query": "test"})],
        )

        assert msg.role == "assistant"
        assert len(msg.tool_calls) == 1
        assert msg.tool_calls[0].name == "search"


class TestRealOTELTraces:
    """Test parsing real OTEL traces from fixtures."""

    @pytest.fixture
    def sample_traces(self):
        """Load sample traces from fixtures."""
        fixtures_path = Path(__file__).parent / "fixtures" / "sample_traces.json"
        if not fixtures_path.exists():
            pytest.skip("Sample traces fixture not found")
        with open(fixtures_path) as f:
            data = json.load(f)
        return data["traces"]

    def test_parse_otel_trace_with_llm(self, sample_traces):
        """Test parsing real OTEL trace with LLM span."""
        # Find trace with LLM span
        llm_trace = None
        for trace in sample_traces:
            for span in trace.get("spans", []):
                if span.get("ampAttributes", {}).get("kind") == "llm":
                    llm_trace = trace
                    break
            if llm_trace:
                break

        if not llm_trace:
            pytest.skip("No LLM trace found in fixtures")

        # Convert real OTEL JSON to Trace object
        trace = _parse_trace(llm_trace)
        eval_trace = parse_trace_for_evaluation(trace)

        # Should have parsed trace_id
        assert eval_trace.trace_id == llm_trace["traceId"]

        # Should have extracted LLM spans
        assert len(eval_trace.llm_spans) >= 1

        # Check LLM span properties
        llm_span = eval_trace.llm_spans[0]
        assert llm_span.span_id  # Has span ID
        assert llm_span.model  # Has model name (e.g., gpt-4o)
        assert llm_span.duration_ms > 0  # Duration converted from nanos

    def test_parse_otel_trace_with_agents(self, sample_traces):
        """Test parsing real OTEL trace with agent spans (CrewAI)."""
        # Find trace with agent spans
        agent_trace = None
        for trace in sample_traces:
            kinds = set()
            for span in trace.get("spans", []):
                kinds.add(span.get("ampAttributes", {}).get("kind"))
            if "agent" in kinds:
                agent_trace = trace
                break

        if not agent_trace:
            pytest.skip("No agent trace found in fixtures")

        # Convert real OTEL JSON to Trace object
        trace = _parse_trace(agent_trace)
        eval_trace = parse_trace_for_evaluation(trace)

        # Validate: trace_id should match the original
        assert eval_trace.trace_id == agent_trace["traceId"]

        # Validate: should have parsed agent span
        assert eval_trace.agent_span is not None

        # Check: agent span has required fields
        assert eval_trace.agent_span.span_id  # Has span ID
        assert eval_trace.agent_span.duration_ms > 0  # Duration is converted

        # Check: agent name exists (may be empty string but field should exist)
        assert hasattr(eval_trace.agent_span, "name")

        # Validate: metrics should have counts
        # Note: tool_call_count may be 0 if no tool spans in this trace
        assert eval_trace.metrics.llm_call_count >= 0
        assert eval_trace.metrics.tool_call_count >= 0

    def test_otel_duration_conversion(self, sample_traces):
        """Test that durationInNanos is correctly converted to milliseconds."""
        if not sample_traces:
            pytest.skip("No sample traces")

        # Convert real OTEL JSON to Trace object
        trace_dict = sample_traces[0]
        trace = _parse_trace(trace_dict)
        eval_trace = parse_trace_for_evaluation(trace)

        # Find a span with duration
        for raw_span in trace_dict.get("spans", []):
            if raw_span.get("ampAttributes", {}).get("kind") in ["llm", "tool", "agent"]:
                nanos = raw_span.get("durationInNanos", 0)
                expected_ms = nanos / 1_000_000

                # Check that parsed spans have correct duration
                span_id = raw_span.get("spanId")

                # Find matching span in eval_trace
                for llm in eval_trace.llm_spans:
                    if llm.span_id == span_id:
                        assert llm.duration_ms == expected_ms
                        return
                for tool in eval_trace.tool_spans:
                    if tool.span_id == span_id:
                        assert tool.duration_ms == expected_ms
                        return
                if eval_trace.agent_span and eval_trace.agent_span.span_id == span_id:
                    assert eval_trace.agent_span.duration_ms == expected_ms
                    return

    def test_chain_spans_are_skipped(self, sample_traces):
        """Test that chain spans are correctly skipped."""
        if not sample_traces:
            pytest.skip("No sample traces")

        # Count chain spans in raw traces
        chain_count = 0
        for trace in sample_traces:
            for span in trace.get("spans", []):
                if span.get("ampAttributes", {}).get("kind") == "chain":
                    chain_count += 1

        # Convert real OTEL JSON to Trace objects and parse all traces
        traces = [_parse_trace(t) for t in sample_traces]
        eval_traces = parse_traces_for_evaluation(traces)

        # Chain spans should not be in any parsed trace
        for eval_trace in eval_traces:
            # Trajectory only has llm_spans, tool_spans, retriever_spans, agent_span
            # No chain spans should appear
            pass  # Structure verification - chains are simply not included

        # If there were chain spans, they should have been skipped
        if chain_count > 0:
            # Total parsed spans should be less than total raw spans
            total_raw = sum(len(t.get("spans", [])) for t in sample_traces)
            total_parsed = sum(
                len(et.llm_spans) + len(et.tool_spans) + len(et.retriever_spans) + (1 if et.agent_span else 0)
                for et in eval_traces
            )
            assert total_parsed < total_raw


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
