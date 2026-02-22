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
Comprehensive tests for trace/models.py

Tests the evaluation-friendly Trace interface including:
- Typed AgentStep union (UserStep, LLMStep, ToolExecutionStep)
- Typed Message union (SystemMessage, UserMessage, AssistantMessage, ToolMessage)
- Trace reconstruction with get_agent_steps()
- Filtered span access (get_llm_calls, get_tool_calls, etc.)
- Various scenarios: simple, parallel, nested, multi-agent
"""

import pytest
from datetime import datetime

from amp_evaluation.trace.models import (
    # Core types
    Trace,
    TokenUsage,
    # Span types
    LLMSpan,
    ToolSpan,
    RetrieverSpan,
    AgentSpan,
    LLMMetrics,
    ToolMetrics,
    RetrieverMetrics,
    AgentMetrics,
    # Typed messages
    SystemMessage,
    UserMessage,
    AssistantMessage,
    ToolMessage,
    Message,
    # Typed steps
    UserStep,
    LLMStep,
    ToolExecutionStep,
    AgentStep,
    # Step/tool helpers
    ToolCallInfo,
    ToolCall,
    RetrievedDoc,
    # Backward compatibility
    TraceMetrics,
)


# ============================================================================
# FIXTURES - Common test data
# ============================================================================


@pytest.fixture
def simple_llm_span():
    """A simple LLM span with messages and response."""
    return LLMSpan(
        span_id="llm-1",
        parent_span_id=None,
        start_time=datetime(2026, 1, 1, 12, 0, 0),
        messages=[
            SystemMessage(content="You are a helpful assistant."),
            UserMessage(content="What is 2+2?"),
        ],
        response="2+2 equals 4.",
        model="gpt-4",
        vendor="openai",
        metrics=LLMMetrics(
            duration_ms=150.0,
            token_usage=TokenUsage(input_tokens=20, output_tokens=10, total_tokens=30),
        ),
    )


@pytest.fixture
def llm_span_with_tool_calls():
    """An LLM span that requests tool calls."""
    return LLMSpan(
        span_id="llm-2",
        parent_span_id=None,
        start_time=datetime(2026, 1, 1, 12, 0, 0),
        messages=[
            UserMessage(content="What's the weather in NYC?"),
        ],
        response="I'll check the weather for you.",
        tool_calls=[
            ToolCall(id="tc-1", name="get_weather", arguments={"city": "NYC"}),
        ],
        model="gpt-4",
        metrics=LLMMetrics(duration_ms=200.0),
    )


@pytest.fixture
def tool_span():
    """A simple tool execution span."""
    return ToolSpan(
        span_id="tool-1",
        parent_span_id="llm-2",
        start_time=datetime(2026, 1, 1, 12, 0, 1),
        name="get_weather",
        arguments={"city": "NYC"},
        result="72°F and sunny",
        metrics=ToolMetrics(duration_ms=500.0),
    )


@pytest.fixture
def nested_llm_in_tool():
    """An LLM span nested inside a tool (tool calls LLM)."""
    return LLMSpan(
        span_id="llm-nested",
        parent_span_id="tool-complex",
        start_time=datetime(2026, 1, 1, 12, 0, 2),
        messages=[
            UserMessage(content="Confirm the reservation"),
        ],
        response="Reservation confirmed for 7pm.",
        model="gpt-4",
        metrics=LLMMetrics(duration_ms=100.0),
    )


@pytest.fixture
def retriever_span():
    """A retrieval span with documents."""
    return RetrieverSpan(
        span_id="retriever-1",
        parent_span_id=None,
        start_time=datetime(2026, 1, 1, 12, 0, 0),
        query="machine learning basics",
        documents=[
            RetrievedDoc(id="doc-1", content="Machine learning is...", score=0.95),
            RetrievedDoc(id="doc-2", content="Deep learning is a subset...", score=0.87),
        ],
        vector_db="pinecone",
        top_k=5,
        metrics=RetrieverMetrics(duration_ms=50.0, documents_retrieved=2),
    )


@pytest.fixture
def agent_span():
    """An agent orchestration span."""
    return AgentSpan(
        span_id="agent-1",
        parent_span_id=None,
        start_time=datetime(2026, 1, 1, 12, 0, 0),
        name="CustomerServiceAgent",
        framework="langchain",
        model="gpt-4",
        system_prompt="You are a customer service agent.",
        available_tools=["get_order_status", "refund_order"],
        input="I want to check my order status",
        output="Your order #123 is being shipped.",
        metrics=AgentMetrics(duration_ms=2000.0),
    )


# ============================================================================
# TESTS: Basic Dataclass Functionality
# ============================================================================


class TestTokenUsage:
    """Tests for TokenUsage dataclass."""

    def test_creation(self):
        usage = TokenUsage(input_tokens=100, output_tokens=50, total_tokens=150)
        assert usage.input_tokens == 100
        assert usage.output_tokens == 50
        assert usage.total_tokens == 150

    def test_default_values(self):
        usage = TokenUsage()
        assert usage.input_tokens == 0
        assert usage.output_tokens == 0
        assert usage.total_tokens == 0
        assert usage.cache_read_tokens == 0

    def test_addition(self):
        usage1 = TokenUsage(input_tokens=100, output_tokens=50, total_tokens=150)
        usage2 = TokenUsage(input_tokens=200, output_tokens=100, total_tokens=300)
        combined = usage1 + usage2
        assert combined.input_tokens == 300
        assert combined.output_tokens == 150
        assert combined.total_tokens == 450


class TestTrajectoryMetrics:
    """Tests for TraceMetrics dataclass."""

    def test_creation(self):
        metrics = TraceMetrics(
            total_duration_ms=1000.0,
            token_usage=TokenUsage(input_tokens=100, output_tokens=50, total_tokens=150),
            llm_call_count=2,
            tool_call_count=1,
        )
        assert metrics.total_duration_ms == 1000.0
        assert metrics.llm_call_count == 2

    def test_has_errors(self):
        metrics = TraceMetrics(error_count=0)
        assert not metrics.has_errors

        metrics_with_errors = TraceMetrics(error_count=2)
        assert metrics_with_errors.has_errors

    def test_avg_tokens_per_llm_call(self):
        metrics = TraceMetrics(
            token_usage=TokenUsage(total_tokens=300),
            llm_call_count=3,
        )
        assert metrics.avg_tokens_per_llm_call == 100.0

    def test_avg_tokens_per_llm_call_zero_calls(self):
        metrics = TraceMetrics(llm_call_count=0)
        assert metrics.avg_tokens_per_llm_call == 0.0


class TestAgentStep:
    """Tests for typed AgentStep union types (UserStep, LLMStep, ToolExecutionStep)."""

    def test_creation_assistant(self):
        step = LLMStep(
            content="Hello, how can I help?",
            tool_calls=[ToolCallInfo(id="1", name="search", arguments={"q": "test"})],
        )
        assert isinstance(step, LLMStep)
        assert step.content == "Hello, how can I help?"
        assert len(step.tool_calls) == 1
        assert step.tool_calls[0].name == "search"

    def test_creation_tool_result(self):
        step = ToolExecutionStep(
            tool_name="get_weather",
            tool_input={"city": "NYC"},
            tool_output="72°F",
        )
        assert isinstance(step, ToolExecutionStep)
        assert step.tool_name == "get_weather"
        assert step.tool_output == "72°F"

    def test_nested_steps(self):
        nested_llm = LLMSpan(
            span_id="nested-llm-1",
            messages=[UserMessage(content="Confirming...")],
            response="Confirmed.",
        )
        step = ToolExecutionStep(
            tool_name="book_restaurant",
            nested_traces=[nested_llm],
        )
        assert len(step.nested_traces) == 1
        assert isinstance(step.nested_traces[0], LLMSpan)
        assert step.nested_traces[0].response == "Confirmed."


# ============================================================================
# TESTS: Span Types with New Fields
# ============================================================================


class TestLLMSpan:
    """Tests for LLMSpan with new fields."""

    def test_new_fields(self, simple_llm_span):
        assert simple_llm_span.parent_span_id is None
        assert simple_llm_span.start_time == datetime(2026, 1, 1, 12, 0, 0)

    def test_metrics_access(self, simple_llm_span):
        assert simple_llm_span.metrics.duration_ms == 150.0
        assert simple_llm_span.metrics.error is False
        assert simple_llm_span.metrics.token_usage.total_tokens == 30


class TestToolSpan:
    """Tests for ToolSpan with new fields."""

    def test_new_fields(self, tool_span):
        assert tool_span.parent_span_id == "llm-2"
        assert tool_span.start_time == datetime(2026, 1, 1, 12, 0, 1)

    def test_metrics_access(self, tool_span):
        assert tool_span.metrics.duration_ms == 500.0
        assert tool_span.metrics.error is False


class TestRetrieverSpan:
    """Tests for RetrieverSpan with new fields."""

    def test_new_fields(self, retriever_span):
        assert retriever_span.parent_span_id is None
        assert retriever_span.start_time == datetime(2026, 1, 1, 12, 0, 0)

    def test_documents(self, retriever_span):
        assert len(retriever_span.documents) == 2
        assert retriever_span.documents[0].content == "Machine learning is..."


class TestAgentSpanType:
    """Tests for AgentSpan with new fields."""

    def test_new_fields(self, agent_span):
        assert agent_span.parent_span_id is None
        assert agent_span.start_time == datetime(2026, 1, 1, 12, 0, 0)

    def test_content(self, agent_span):
        assert agent_span.name == "CustomerServiceAgent"
        assert agent_span.framework == "langchain"
        assert len(agent_span.available_tools) == 2


# ============================================================================
# TESTS: Trace - Simple Scenarios
# ============================================================================


class TestTrajectorySimple:
    """Tests for basic Trace functionality."""

    def test_creation(self, simple_llm_span):
        trajectory = Trace(
            trace_id="trace-1",
            input="What is 2+2?",
            output="2+2 equals 4.",
            spans=[simple_llm_span],
            metrics=TraceMetrics(llm_call_count=1),
        )
        assert trajectory.trace_id == "trace-1"
        assert trajectory.input == "What is 2+2?"
        assert trajectory.output == "2+2 equals 4."

    def test_metrics_access(self, simple_llm_span):
        trajectory = Trace(
            trace_id="trace-1",
            input="test",
            output="result",
            spans=[simple_llm_span],
            metrics=TraceMetrics(error_count=0),
        )
        assert not trajectory.metrics.has_errors


class TestTrajectoryGetLLMCalls:
    """Tests for get_llm_calls() method."""

    def test_simple(self, simple_llm_span):
        trajectory = Trace(trace_id="trace-1", spans=[simple_llm_span])
        llm_calls = trajectory.get_llm_calls()
        assert len(llm_calls) == 1
        assert llm_calls[0].span_id == "llm-1"

    def test_include_nested(self, llm_span_with_tool_calls, tool_span, nested_llm_in_tool):
        # Create a tool that has a nested LLM
        complex_tool = ToolSpan(
            span_id="tool-complex",
            parent_span_id="llm-2",
            start_time=datetime(2026, 1, 1, 12, 0, 1),
            name="complex_operation",
            result="done",
        )
        trajectory = Trace(
            trace_id="trace-1",
            spans=[llm_span_with_tool_calls, complex_tool, nested_llm_in_tool],
        )

        # Include nested (default)
        all_llm = trajectory.get_llm_calls(include_nested=True)
        assert len(all_llm) == 2

        # Exclude nested
        root_llm = trajectory.get_llm_calls(include_nested=False)
        assert len(root_llm) == 1
        assert root_llm[0].span_id == "llm-2"


class TestTrajectoryGetToolCalls:
    """Tests for get_tool_calls() method."""

    def test_simple(self, tool_span):
        trajectory = Trace(trace_id="trace-1", spans=[tool_span])
        tools = trajectory.get_tool_calls()
        assert len(tools) == 1
        assert tools[0].name == "get_weather"

    def test_include_nested(self):
        parent_tool = ToolSpan(
            span_id="tool-parent",
            parent_span_id=None,
            name="parent_op",
        )
        nested_tool = ToolSpan(
            span_id="tool-nested",
            parent_span_id="tool-parent",
            name="nested_op",
        )
        trajectory = Trace(trace_id="trace-1", spans=[parent_tool, nested_tool])

        all_tools = trajectory.get_tool_calls(include_nested=True)
        assert len(all_tools) == 2

        root_tools = trajectory.get_tool_calls(include_nested=False)
        assert len(root_tools) == 1
        assert root_tools[0].name == "parent_op"


class TestTrajectoryGetRetrievals:
    """Tests for get_retrievals() method."""

    def test_simple(self, retriever_span):
        trajectory = Trace(trace_id="trace-1", spans=[retriever_span])
        retrievals = trajectory.get_retrievals()
        assert len(retrievals) == 1
        assert retrievals[0].query == "machine learning basics"


class TestTrajectoryGetContext:
    """Tests for get_context() method."""

    def test_simple(self, retriever_span):
        trajectory = Trace(trace_id="trace-1", spans=[retriever_span])
        context = trajectory.get_context()
        assert "Machine learning is..." in context
        assert "Deep learning is a subset..." in context

    def test_multiple_retrievals(self):
        retrieval1 = RetrieverSpan(
            span_id="r1",
            query="q1",
            documents=[RetrievedDoc(content="Doc 1")],
        )
        retrieval2 = RetrieverSpan(
            span_id="r2",
            query="q2",
            documents=[RetrievedDoc(content="Doc 2")],
        )
        trajectory = Trace(trace_id="trace-1", spans=[retrieval1, retrieval2])
        context = trajectory.get_context()
        assert "Doc 1" in context
        assert "Doc 2" in context


class TestTrajectoryGetAgents:
    """Tests for get_agents() method."""

    def test_simple(self, agent_span):
        trajectory = Trace(trace_id="trace-1", spans=[agent_span])
        agents = trajectory.get_agents()
        assert len(agents) == 1
        assert agents[0].name == "CustomerServiceAgent"

    def test_multi_agent(self):
        agent1 = AgentSpan(span_id="a1", name="Manager")
        agent2 = AgentSpan(span_id="a2", name="Worker", parent_span_id="a1")
        trajectory = Trace(trace_id="trace-1", spans=[agent1, agent2])
        agents = trajectory.get_agents()
        assert len(agents) == 2


# ============================================================================
# TESTS: Trace - get_agent_steps() Reconstruction
# ============================================================================


class TestTrajectoryGetAgentSteps:
    """Tests for get_agent_steps() conversation reconstruction."""

    def test_simple_llm(self, simple_llm_span):
        """Test reconstruction of a simple LLM conversation.

        System prompts are metadata, not steps. Only user and assistant steps
        are returned.
        """
        trajectory = Trace(trace_id="trace-1", spans=[simple_llm_span])
        steps = trajectory.get_agent_steps()

        # Should have: user, assistant (system is metadata, not a step)
        assert len(steps) >= 2  # At least user + assistant

        # Find the steps by type
        user_steps = [s for s in steps if isinstance(s, UserStep)]
        llm_steps = [s for s in steps if isinstance(s, LLMStep)]

        assert len(user_steps) >= 1
        assert len(llm_steps) >= 1

        # Verify content
        assert user_steps[0].content == "What is 2+2?"
        assert llm_steps[0].content == "2+2 equals 4."

    def test_llm_with_tool_calls(self, llm_span_with_tool_calls, tool_span):
        """Test reconstruction with tool calls."""
        trajectory = Trace(
            trace_id="trace-1",
            spans=[llm_span_with_tool_calls, tool_span],
        )
        steps = trajectory.get_agent_steps()

        # Should have: user, assistant (with tool_calls), tool_result
        assistant_steps = [s for s in steps if isinstance(s, LLMStep)]
        assert len(assistant_steps) >= 1
        assert len(assistant_steps[0].tool_calls) == 1
        assert assistant_steps[0].tool_calls[0].name == "get_weather"

        tool_steps = [s for s in steps if isinstance(s, ToolExecutionStep)]
        assert len(tool_steps) >= 1
        assert tool_steps[0].tool_name == "get_weather"
        assert tool_steps[0].tool_output == "72°F and sunny"

    def test_nested_tool_with_llm(self):
        """Test reconstruction when a tool calls an LLM internally."""
        parent_llm = LLMSpan(
            span_id="llm-parent",
            messages=[UserMessage(content="Book a restaurant")],
            response="I'll book that for you.",
            tool_calls=[ToolCall(id="tc-1", name="book_restaurant", arguments={})],
        )
        tool = ToolSpan(
            span_id="tool-book",
            parent_span_id="llm-parent",
            name="book_restaurant",
            arguments={"restaurant": "Luigi's"},
            result="Booked!",
        )
        nested_llm = LLMSpan(
            span_id="llm-nested",
            parent_span_id="tool-book",
            messages=[UserMessage(content="Confirm booking")],
            response="Booking confirmed.",
        )

        trajectory = Trace(
            trace_id="trace-1",
            spans=[parent_llm, tool, nested_llm],
        )
        steps = trajectory.get_agent_steps()

        # Find the tool execution step
        tool_steps = [s for s in steps if isinstance(s, ToolExecutionStep)]
        assert len(tool_steps) >= 1

        # Check for nested traces
        book_step = next((s for s in tool_steps if s.tool_name == "book_restaurant"), None)
        assert book_step is not None
        assert len(book_step.nested_traces) > 0

    def test_parallel_tool_calls(self):
        """Test reconstruction with parallel tool calls."""
        llm = LLMSpan(
            span_id="llm-1",
            messages=[UserMessage(content="Get weather and news")],
            response="I'll check both.",
            tool_calls=[
                ToolCall(id="tc-1", name="get_weather", arguments={}),
                ToolCall(id="tc-2", name="get_news", arguments={}),
            ],
        )
        tool1 = ToolSpan(
            span_id="tool-weather",
            parent_span_id="llm-1",
            name="get_weather",
            result="Sunny",
            start_time=datetime(2026, 1, 1, 12, 0, 1),
        )
        tool2 = ToolSpan(
            span_id="tool-news",
            parent_span_id="llm-1",
            name="get_news",
            result="Headlines...",
            start_time=datetime(2026, 1, 1, 12, 0, 1),  # Same time (parallel)
        )

        trajectory = Trace(
            trace_id="trace-1",
            spans=[llm, tool1, tool2],
        )
        steps = trajectory.get_agent_steps()

        # Should have tool results for both
        tool_steps = [s for s in steps if isinstance(s, ToolExecutionStep)]
        tool_names = {s.tool_name for s in tool_steps}
        assert "get_weather" in tool_names
        assert "get_news" in tool_names

    def test_with_retrieval(self, retriever_span):
        """Test reconstruction with retrieval step.

        Retrieval is now a ToolExecutionStep with tool_name == 'retrieval'.
        The query is in tool_input and documents are in tool_output.
        """
        trajectory = Trace(trace_id="trace-1", spans=[retriever_span])
        steps = trajectory.get_agent_steps()

        # Retrieval is represented as a ToolExecutionStep with tool_name="retrieval"
        retrieval_steps = [
            s for s in steps
            if isinstance(s, ToolExecutionStep) and s.tool_name == "retrieval"
        ]
        assert len(retrieval_steps) == 1

        # Check tool_input contains the query
        assert retrieval_steps[0].tool_input is not None
        assert retrieval_steps[0].tool_input.get("query") == "machine learning basics"

        # Check tool_output contains the documents
        assert retrieval_steps[0].tool_output is not None
        docs = retrieval_steps[0].tool_output.get("documents", [])
        assert len(docs) == 2

    def test_with_agent_system_prompt(self, agent_span, simple_llm_span):
        """Test that agent's system prompt is preserved as metadata, not as a step.

        System prompts are NOT steps anymore. The agent span's system_prompt
        field holds this metadata, and it can be accessed via create_agent_trace().
        """
        simple_llm_span.parent_span_id = agent_span.span_id
        trajectory = Trace(
            trace_id="trace-1",
            spans=[agent_span, simple_llm_span],
        )
        steps = trajectory.get_agent_steps()

        # System prompts are NOT steps, so there should be no system steps
        # (AgentSpan is a marker, system prompt is metadata)
        # The agent span's system_prompt is accessible via the span itself
        assert agent_span.system_prompt == "You are a customer service agent."

        # Steps should only contain user and assistant steps (no system step type)
        for s in steps:
            assert isinstance(s, (UserStep, LLMStep, ToolExecutionStep))

    def test_for_specific_agent(self):
        """Test getting steps for a specific agent in multi-agent system."""
        agent1 = AgentSpan(
            span_id="agent-manager",
            name="Manager",
            system_prompt="You manage tasks.",
        )
        agent2 = AgentSpan(
            span_id="agent-worker",
            parent_span_id="agent-manager",
            name="Worker",
            system_prompt="You do the work.",
        )
        llm1 = LLMSpan(
            span_id="llm-1",
            parent_span_id="agent-manager",
            messages=[UserMessage(content="Delegate task")],
            response="Delegating...",
        )
        llm2 = LLMSpan(
            span_id="llm-2",
            parent_span_id="agent-worker",
            messages=[UserMessage(content="Do the work")],
            response="Done!",
        )

        trajectory = Trace(
            trace_id="trace-1",
            spans=[agent1, agent2, llm1, llm2],
        )

        # Get steps for worker agent only
        worker_steps = trajectory.get_agent_steps(agent_span_id="agent-worker")

        # Should include the worker's LLM call
        assert any(
            s.content == "Done!"
            for s in worker_steps
            if isinstance(s, LLMStep)
        )


# ============================================================================
# TESTS: Edge Cases
# ============================================================================


class TestEdgeCases:
    """Tests for edge cases and error handling."""

    def test_empty_trajectory(self):
        trajectory = Trace(trace_id="trace-1")
        assert trajectory.get_llm_calls() == []
        assert trajectory.get_tool_calls() == []
        assert trajectory.get_retrievals() == []
        assert trajectory.get_agents() == []
        assert trajectory.get_context() == ""
        assert trajectory.get_agent_steps() == []

    def test_missing_parent_span_id(self):
        """Test that missing parent_span_id is handled gracefully."""
        llm = LLMSpan(span_id="llm-1")  # No parent_span_id
        trajectory = Trace(trace_id="trace-1", spans=[llm])
        steps = trajectory.get_agent_steps()
        # Should not crash
        assert isinstance(steps, list)

    def test_llm_with_empty_messages(self):
        """Test LLM span with no messages."""
        llm = LLMSpan(span_id="llm-1", response="Just a response")
        trajectory = Trace(trace_id="trace-1", spans=[llm])
        steps = trajectory.get_agent_steps()
        assistant_steps = [s for s in steps if isinstance(s, LLMStep)]
        assert len(assistant_steps) == 1
        assert assistant_steps[0].content == "Just a response"

    def test_tool_with_error(self):
        """Test tool span with error."""
        tool = ToolSpan(
            span_id="tool-1",
            name="failing_tool",
            metrics=ToolMetrics(error=True, error_message="Connection failed"),
        )
        trajectory = Trace(trace_id="trace-1", spans=[tool])
        steps = trajectory.get_agent_steps()
        tool_steps = [s for s in steps if isinstance(s, ToolExecutionStep)]
        assert len(tool_steps) == 1
        assert tool_steps[0].error == "Connection failed"

    def test_deeply_nested_tools(self):
        """Test deeply nested tool calls (tool -> tool -> tool)."""
        tool1 = ToolSpan(span_id="t1", name="level1")
        tool2 = ToolSpan(span_id="t2", name="level2", parent_span_id="t1")
        tool3 = ToolSpan(span_id="t3", name="level3", parent_span_id="t2")

        trajectory = Trace(trace_id="trace-1", spans=[tool1, tool2, tool3])

        # All tools with nested
        all_tools = trajectory.get_tool_calls(include_nested=True)
        assert len(all_tools) == 3

        # Root only
        root_tools = trajectory.get_tool_calls(include_nested=False)
        assert len(root_tools) == 1
        assert root_tools[0].name == "level1"

        # Check reconstruction: only the root tool appears as a step
        # (nested tools are children with tool parent, so excluded from root steps).
        # nested_traces only stores LLMSpan and AgentTrace, not child ToolSpans.
        steps = trajectory.get_agent_steps()
        level1_step = next(
            (s for s in steps if isinstance(s, ToolExecutionStep) and s.tool_name == "level1"),
            None,
        )
        assert level1_step is not None
        # Child ToolSpans are not added to nested_traces (only LLMSpan/AgentTrace are)
        assert len(level1_step.nested_traces) == 0

    def test_deeply_nested_tool_with_llm(self):
        """Test nested tool containing an LLM call appears in nested_traces."""
        tool1 = ToolSpan(span_id="t1", name="level1")
        nested_llm = LLMSpan(
            span_id="llm-in-tool",
            parent_span_id="t1",
            messages=[UserMessage(content="Nested question")],
            response="Nested answer",
        )

        trajectory = Trace(trace_id="trace-1", spans=[tool1, nested_llm])
        steps = trajectory.get_agent_steps()

        level1_step = next(
            (s for s in steps if isinstance(s, ToolExecutionStep) and s.tool_name == "level1"),
            None,
        )
        assert level1_step is not None
        assert len(level1_step.nested_traces) == 1
        assert isinstance(level1_step.nested_traces[0], LLMSpan)
        assert level1_step.nested_traces[0].response == "Nested answer"
