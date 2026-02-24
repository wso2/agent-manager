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

Tests parsing raw OTEL/AMP traces into Trace format.
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
    ToolCall,
    parse_trace_for_evaluation,
    parse_traces_for_evaluation,
)

# OTEL models from fetcher (internal)
from amp_evaluation.trace.fetcher import (
    OTELTrace,
    OTELSpan,
    OTELTraceStatus,
)

# Import span types and models for testing
from amp_evaluation.trace.models import (
    AgentSpan,
    ToolSpan,
    UserStep,
    LLMStep,
    ToolExecutionStep,
    SystemMessage,
    UserMessage,
    AssistantMessage,
    ToolMessage,
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
        assert len(eval_trace.get_llm_calls()) == 0
        assert len(eval_trace.get_tool_calls()) == 0

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

        assert len(eval_trace.get_llm_calls()) == 1

        llm_span = eval_trace.get_llm_calls()[0]
        assert llm_span.span_id == "span_1"
        assert llm_span.model == "gpt-4"
        assert llm_span.vendor == "OpenAI"
        assert llm_span.temperature == 0.7
        assert llm_span.response == "Hi there!"
        assert llm_span.metrics.duration_ms == 500.0
        assert not llm_span.metrics.error

        # Check messages
        assert len(llm_span.messages) == 2
        assert isinstance(llm_span.messages[0], SystemMessage)
        assert llm_span.messages[0].content == "You are helpful."
        assert isinstance(llm_span.messages[1], UserMessage)

        # Check token usage
        assert llm_span.metrics.token_usage.input_tokens == 10
        assert llm_span.metrics.token_usage.output_tokens == 5
        assert llm_span.metrics.token_usage.total_tokens == 15

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

        assert len(eval_trace.get_tool_calls()) == 1

        tool_span = eval_trace.get_tool_calls()[0]
        assert tool_span.span_id == "span_tool_1"
        assert tool_span.name == "web_search"
        assert tool_span.arguments == {"query": "python tutorials"}
        assert tool_span.result == ["result1", "result2"]
        assert tool_span.metrics.duration_ms == 200.0

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

        assert len(eval_trace.get_retrievals()) == 1

        ret_span = eval_trace.get_retrievals()[0]
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

        agents = eval_trace.get_agents()
        assert len(agents) == 1

        agent_span = agents[0]
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
        """Test Trace convenience properties."""
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
        assert bool(eval_trace.output and eval_trace.output.strip())
        assert not eval_trace.metrics.has_errors
        assert [t.name for t in eval_trace.get_tool_calls()] == ["search", "calculate"]
        assert [t.result for t in eval_trace.get_tool_calls()] == ["search result", 42]
        assert [llm.response for llm in eval_trace.get_llm_calls()] == ["Response 1"]

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
        assert len(eval_trace.get_llm_calls()) == 1
        assert len(eval_trace.get_tool_calls()) == 0
        assert len(eval_trace.get_retrievals()) == 0

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
    """Test the Trace data structure itself."""

    def test_token_usage_addition(self):
        """Test that TokenUsage objects can be added."""
        t1 = TokenUsage(input_tokens=10, output_tokens=5, total_tokens=15)
        t2 = TokenUsage(input_tokens=8, output_tokens=4, total_tokens=12)

        combined = t1 + t2

        assert combined.input_tokens == 18
        assert combined.output_tokens == 9
        assert combined.total_tokens == 27

    def test_message_with_tool_calls(self):
        """Test AssistantMessage with tool calls."""
        msg = AssistantMessage(
            content="I'll search for that.",
            tool_calls=[ToolCall(id="tc1", name="search", arguments={"query": "test"})],
        )

        assert isinstance(msg, AssistantMessage)
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
        assert len(eval_trace.get_llm_calls()) >= 1

        # Check LLM span properties
        llm_span = eval_trace.get_llm_calls()[0]
        assert llm_span.span_id  # Has span ID
        assert llm_span.model  # Has model name (e.g., gpt-4o)
        assert llm_span.metrics.duration_ms > 0  # Duration converted from nanos

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
        agents = eval_trace.get_agents()
        assert len(agents) >= 1

        # Check: agent span has required fields
        agent = agents[0]
        assert agent.span_id  # Has span ID
        assert agent.metrics.duration_ms > 0  # Duration is converted

        # Check: agent name exists (may be empty string but field should exist)
        assert hasattr(agent, "name")

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
                for llm in eval_trace.get_llm_calls():
                    if llm.span_id == span_id:
                        assert llm.metrics.duration_ms == expected_ms
                        return
                for tool in eval_trace.get_tool_calls():
                    if tool.span_id == span_id:
                        assert tool.metrics.duration_ms == expected_ms
                        return
                for agent in eval_trace.get_agents():
                    if agent.span_id == span_id:
                        assert agent.metrics.duration_ms == expected_ms
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

        # If there were chain spans, they should have been skipped
        if chain_count > 0:
            # Total parsed spans should be less than total raw spans
            total_raw = sum(len(t.get("spans", [])) for t in sample_traces)
            total_parsed = sum(
                len(et.get_llm_calls()) + len(et.get_tool_calls()) + len(et.get_retrievals()) + len(et.get_agents())
                for et in eval_traces
            )
            assert total_parsed < total_raw

    def test_crewai_sequential_agents_reconstruction(self, sample_traces):
        """
        Test sequential agent execution with real CrewAI trace.

        This trace (66ea0b364e7397376b7c9edcc82e1f85) has 3 agents executing sequentially:
        - Activity Planner (first)
        - Restaurant Scout (second)
        - Itinerary Compiler (third)

        Each has multiple internal LLM calls. Tests that get_agent_steps()
        correctly reconstructs the execution flow.
        """
        # Load CrewAI trace
        crew_trace = next((t for t in sample_traces if t["traceId"] == "66ea0b364e7397376b7c9edcc82e1f85"), None)
        assert crew_trace is not None, "CrewAI multi-agent trace not found"

        # Parse to Trace
        trace = _parse_trace(crew_trace)
        trajectory = parse_trace_for_evaluation(trace)

        # VERIFY: 3 agents extracted from ampAttributes
        agents = trajectory.get_agents()
        assert len(agents) == 3, f"Expected 3 agents, got {len(agents)}"

        # VERIFY: Agent names from ampAttributes.data.name
        agent_names = [a.name for a in agents]
        assert "Activity Planner" in agent_names
        assert "Restaurant Scout" in agent_names
        assert "Itinerary Compiler" in agent_names

        # VERIFY: Framework from ampAttributes.data.framework
        for agent in agents:
            assert agent.framework == "crewai"
            assert agent.system_prompt, f"{agent.name} missing system prompt"
            assert agent.available_tools, f"{agent.name} has no tools"

        # VERIFY: get_agent_steps() returns steps for all agents
        all_steps = trajectory.get_agent_steps()
        assert len(all_steps) > 0, "No steps reconstructed"

        # VERIFY: System prompts are stored in AgentSpan metadata (not as steps)
        # System messages are no longer steps; they are in agent.system_prompt
        for agent in agents:
            assert agent.system_prompt, f"{agent.name} should have system_prompt set"

        # VERIFY: get_agent_steps(agent_span_id) for each agent
        for agent in agents:
            agent_steps = trajectory.get_agent_steps(agent_span_id=agent.span_id)
            assert len(agent_steps) > 0, f"No steps for {agent.name}"

            # Should have LLM steps (from LLM calls)
            assistant_steps = [s for s in agent_steps if isinstance(s, LLMStep)]
            assert len(assistant_steps) > 0, f"No assistant steps for {agent.name}"

    def test_sequential_agents_are_not_nested(self, sample_traces):
        """
        Verify sequential agents (siblings) are NOT confused with nested agents (parent-child).
        """
        crew_trace = next((t for t in sample_traces if t["traceId"] == "66ea0b364e7397376b7c9edcc82e1f85"), None)
        assert crew_trace is not None, "CrewAI multi-agent trace not found"

        trace = _parse_trace(crew_trace)
        trajectory = parse_trace_for_evaluation(trace)

        agents = trajectory.get_agents()
        agent_ids = {a.span_id for a in agents}

        # VERIFY: No agent has another agent as parent (they're sequential, not nested)
        for agent in agents:
            parent = agent.parent_span_id
            assert parent not in agent_ids, f"{agent.name} has agent parent - should be sequential not nested"

    def test_langgraph_parallel_tools_and_errors(self, sample_traces):
        """
        Test LangGraph trace with parallel tools and errors.

        Trace 789a4cc3a165ed330d3244aca8b61dbb has:
        - Multiple LLM calls with messages from ampAttributes.input
        - 5 parallel search_hotels tool calls
        - Tool errors in results
        """
        lg_trace = sample_traces[0]
        assert lg_trace["traceId"] == "789a4cc3a165ed330d3244aca8b61dbb"

        trace = _parse_trace(lg_trace)
        trajectory = parse_trace_for_evaluation(trace)

        # VERIFY: LLM calls extracted from ampAttributes
        llm_calls = trajectory.get_llm_calls()
        assert len(llm_calls) > 0, "No LLM calls extracted"

        for llm in llm_calls:
            # VERIFY: Messages from ampAttributes.input
            assert llm.messages, f"LLM {llm.span_id} has no messages"
            for msg in llm.messages:
                assert isinstance(msg, (SystemMessage, UserMessage, AssistantMessage, ToolMessage))

            # VERIFY: Model from ampAttributes.data.model
            if llm.model:
                assert "gpt" in llm.model.lower() or "claude" in llm.model.lower()

        # VERIFY: Tool calls extracted
        tool_calls = trajectory.get_tool_calls()
        assert len(tool_calls) >= 5, f"Expected >=5 tools, got {len(tool_calls)}"

        # VERIFY: 5 parallel search_hotels calls
        hotel_tools = [t for t in tool_calls if "search_hotels" in t.name]
        assert len(hotel_tools) == 5, f"Expected 5 hotel searches, got {len(hotel_tools)}"

        # VERIFY: Errors captured
        assert trajectory.metrics.error_count > 0, "Expected errors in this trace"

        # VERIFY: get_agent_steps() reconstruction
        steps = trajectory.get_agent_steps()

        # Should have LLM steps
        assistant_steps = [s for s in steps if isinstance(s, LLMStep)]
        assert len(assistant_steps) > 0, "Should have assistant steps"

        # Should have tool execution steps (tools were executed even if not in LLM tool_calls)
        tool_result_steps = [s for s in steps if isinstance(s, ToolExecutionStep)]
        assert len(tool_result_steps) >= 5, "Should have tool results"

        # VERIFY: Tool errors in steps
        error_tools = [s for s in tool_result_steps if s.error]
        assert len(error_tools) > 0, "Tool errors not in reconstructed steps"

    def test_ampattributes_extraction_llm(self, sample_traces):
        """
        Verify LLM span ampAttributes are correctly extracted into LLMSpan fields.
        """
        lg_trace = sample_traces[0]
        trace = _parse_trace(lg_trace)
        trajectory = parse_trace_for_evaluation(trace)

        llm_calls = trajectory.get_llm_calls()
        assert len(llm_calls) > 0

        for llm in llm_calls:
            # VERIFY: model from ampAttributes.data.model
            assert llm.model is not None

            # VERIFY: token usage from ampAttributes.data.tokenUsage
            if llm.metrics.token_usage.total_tokens > 0:
                assert llm.metrics.token_usage.input_tokens >= 0
                assert llm.metrics.token_usage.output_tokens >= 0

            # VERIFY: messages from ampAttributes.input
            assert isinstance(llm.messages, list)

            # VERIFY: tool_calls from ampAttributes.output
            # (may be empty, that's ok)

    def test_ampattributes_extraction_agent(self, sample_traces):
        """
        Verify Agent span ampAttributes.data fields are correctly extracted.
        """
        crew_trace = next((t for t in sample_traces if t["traceId"] == "66ea0b364e7397376b7c9edcc82e1f85"), None)
        assert crew_trace is not None, "CrewAI multi-agent trace not found"

        trace = _parse_trace(crew_trace)
        trajectory = parse_trace_for_evaluation(trace)

        agents = trajectory.get_agents()
        for agent in agents:
            # VERIFY: All from ampAttributes.data
            assert agent.framework == "crewai"
            assert agent.name in ["Activity Planner", "Restaurant Scout", "Itinerary Compiler"]
            assert agent.system_prompt  # From data.systemPrompt
            assert isinstance(agent.available_tools, list)
            assert len(agent.available_tools) > 0

    def test_ampattributes_extraction_tool(self, sample_traces):
        """
        Verify Tool span ampAttributes are correctly extracted.
        """
        lg_trace = sample_traces[0]
        trace = _parse_trace(lg_trace)
        trajectory = parse_trace_for_evaluation(trace)

        tool_calls = trajectory.get_tool_calls()
        for tool in tool_calls:
            # VERIFY: arguments from ampAttributes.input
            assert tool.arguments is not None  # Can be empty dict

            # VERIFY: result from ampAttributes.output
            assert tool.result is not None

            # VERIFY: error status from ampAttributes.status.error
            if "Error:" in str(tool.result):
                assert tool.metrics.error, f"Tool {tool.name} has error in result but flag not set"

    def test_root_level_spans_real_traces(self, sample_traces):
        """
        Test _get_root_level_spans() with real trace hierarchies.
        """
        # Test with LangGraph (has tool nesting)
        lg_trace = sample_traces[0]
        trace = _parse_trace(lg_trace)
        trajectory = parse_trace_for_evaluation(trace)

        root_spans = trajectory._get_root_level_spans()
        tool_span_ids = {s.span_id for s in trajectory.spans if isinstance(s, ToolSpan)}

        # VERIFY: No root span has tool as parent
        for span in root_spans:
            parent = getattr(span, "parent_span_id", None)
            assert parent not in tool_span_ids, f"Root span {span.span_id} has tool parent"

        # Test with CrewAI (has agents)
        crew_trace = next((t for t in sample_traces if t["traceId"] == "66ea0b364e7397376b7c9edcc82e1f85"), None)
        assert crew_trace is not None, "CrewAI multi-agent trace not found"

        trace = _parse_trace(crew_trace)
        trajectory = parse_trace_for_evaluation(trace)

        root_spans = trajectory._get_root_level_spans()
        agents = trajectory.get_agents()

        # VERIFY: All agents are in root level
        agent_ids_in_root = {s.span_id for s in root_spans if isinstance(s, AgentSpan)}
        assert len(agent_ids_in_root) == len(agents), "Not all agents in root level"

    def test_all_traces_agent_steps_reconstruction(self, sample_traces):
        """
        Run get_agent_steps() on ALL 14 real traces and verify correctness.
        """
        for i, trace_dict in enumerate(sample_traces):
            trace = _parse_trace(trace_dict)
            trajectory = parse_trace_for_evaluation(trace)

            # Should not crash
            steps = trajectory.get_agent_steps()

            # VERIFY: All steps are valid typed AgentStep variants
            assert isinstance(steps, list)
            for step in steps:
                assert isinstance(step, (UserStep, LLMStep, ToolExecutionStep))

                # VERIFY: Nested traces on ToolExecutionStep are also valid
                if isinstance(step, ToolExecutionStep) and step.nested_traces:
                    for nested in step.nested_traces:
                        # nested_traces contain LLMSpan or AgentTrace objects
                        assert nested is not None

            # VERIFY: If multi-agent, test agent-specific extraction
            agents = trajectory.get_agents()
            for agent in agents:
                agent_steps = trajectory.get_agent_steps(agent_span_id=agent.span_id)
                assert isinstance(agent_steps, list)


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
