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
Trace data models for evaluation.

This module defines the data structures for representing agent traces
in an evaluation-optimized format. These are intermediate representations
parsed from raw OTEL/AMP traces.

Key Design Principles:
1. Evaluation-friendly interface - evaluators get clean, reconstructed conversation steps
2. Framework-agnostic - works with LangChain, CrewAI, OpenAI Agents, etc.
3. Hierarchy-aware - supports nested tool calls and multi-agent systems
4. Metrics-aware - separate metrics from content for easy access

Vocabulary hierarchy:
- Trace → spans (raw OTEL execution records)
- AgentTrace → steps (reconstructed execution flow: UserInputStep, LLMReasoningStep, ToolExecutionStep)
- LLMSpan → messages (typed conversation: SystemMessage, UserMessage, AssistantMessage, ToolMessage)
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import List, Dict, Any, Optional, Union
from datetime import datetime


# ============================================================================
# METRIC DATACLASSES
# ============================================================================


@dataclass
class TokenUsage:
    """Token usage statistics from LLM calls."""

    input_tokens: int = field(default=0, metadata={"description": "Number of input tokens"})
    output_tokens: int = field(default=0, metadata={"description": "Number of output tokens"})
    total_tokens: int = field(default=0, metadata={"description": "Total tokens (input + output)"})
    cache_read_tokens: int = field(default=0, metadata={"description": "Cached prompt tokens (if supported)"})

    def __add__(self, other: "TokenUsage") -> "TokenUsage":
        """Combine token usage from multiple calls."""
        return TokenUsage(
            input_tokens=self.input_tokens + other.input_tokens,
            output_tokens=self.output_tokens + other.output_tokens,
            total_tokens=self.total_tokens + other.total_tokens,
            cache_read_tokens=self.cache_read_tokens + other.cache_read_tokens,
        )


@dataclass
class SpanMetrics:
    """
    Base metrics for any span type.

    These are the observable metrics we can reliably track
    regardless of the agent framework.
    """

    duration_ms: float = field(default=0.0, metadata={"description": "Span duration in milliseconds"})
    error: bool = field(default=False, metadata={"description": "Whether an error occurred"})
    error_type: Optional[str] = field(default=None, metadata={"description": "Error type if an error occurred"})
    error_message: Optional[str] = field(default=None, metadata={"description": "Error message if an error occurred"})


@dataclass
class LLMMetrics(SpanMetrics):
    """Metrics specific to LLM spans."""

    token_usage: TokenUsage = field(default_factory=TokenUsage, metadata={"description": "Token usage breakdown"})

    # Timing breakdown (if available)
    time_to_first_token_ms: Optional[float] = field(
        default=None, metadata={"description": "Time to first token in milliseconds"}
    )


@dataclass
class ToolMetrics(SpanMetrics):
    """Metrics specific to tool execution spans."""

    pass  # Currently only base metrics, but can extend later


@dataclass
class RetrieverMetrics(SpanMetrics):
    """Metrics specific to retriever spans."""

    documents_retrieved: int = field(default=0, metadata={"description": "Number of documents retrieved"})


@dataclass
class AgentMetrics(SpanMetrics):
    """Metrics specific to agent spans."""

    token_usage: TokenUsage = field(default_factory=TokenUsage, metadata={"description": "Token usage breakdown"})


@dataclass
class TraceMetrics:
    """
    Aggregated metrics for the entire trace.

    These are the observable counts we can reliably measure
    from any agent trace, regardless of framework.
    """

    # Duration
    total_duration_ms: float = field(default=0.0, metadata={"description": "Total trace duration in milliseconds"})

    # Token aggregates
    token_usage: TokenUsage = field(
        default_factory=TokenUsage, metadata={"description": "Aggregated token usage across all LLM calls"}
    )

    # Error tracking
    error_count: int = field(default=0, metadata={"description": "Number of spans with errors"})

    @property
    def has_errors(self) -> bool:
        """Check if any errors occurred in the trace."""
        return self.error_count > 0


# ============================================================================
# TOOL CALL AND RETRIEVAL STRUCTURES
# ============================================================================


@dataclass
class ToolCall:
    """Represents a tool call made by an LLM."""

    id: str = field(metadata={"description": "Unique tool call identifier"})
    name: str = field(metadata={"description": "Name of the tool"})
    arguments: Dict[str, Any] = field(default_factory=dict, metadata={"description": "Arguments passed to the tool"})


@dataclass
class ToolDefinition:
    """Tool/function definition available to an LLM."""

    name: str = field(default="", metadata={"description": "Tool name"})
    description: str = field(default="", metadata={"description": "Tool description"})
    parameters: str = field(default="", metadata={"description": "JSON schema of parameters"})


@dataclass
class RetrievedDoc:
    """Represents a retrieved document from a vector store."""

    id: str = field(default="", metadata={"description": "Document identifier"})
    content: str = field(default="", metadata={"description": "Document content"})
    score: float = field(default=0.0, metadata={"description": "Relevance score"})
    metadata: Dict[str, Any] = field(default_factory=dict, metadata={"description": "Document metadata"})

    def __str__(self) -> str:
        return self.content[:400] if self.content else "(empty document)"


# ============================================================================
# TYPED MESSAGES (for LLMSpan)
# ============================================================================


@dataclass
class SystemMessage:
    """System prompt / instructions."""

    content: str = field(default="", metadata={"description": "System prompt text"})


@dataclass
class UserMessage:
    """User input to the LLM."""

    content: str = field(default="", metadata={"description": "User input text"})


@dataclass
class AssistantMessage:
    """LLM's response, optionally requesting tool calls."""

    content: str = field(default="", metadata={"description": "Response text"})
    tool_calls: List[ToolCall] = field(default_factory=list, metadata={"description": "Tool calls requested"})


@dataclass
class ToolMessage:
    """Tool result fed back to the LLM."""

    content: str = field(default="", metadata={"description": "Tool result text"})
    tool_call_id: str = field(default="", metadata={"description": "ID of the originating tool call"})


Message = Union[SystemMessage, UserMessage, AssistantMessage, ToolMessage]


# ============================================================================
# SPAN DATACLASSES
# ============================================================================


@dataclass
class LLMSpan:
    """
    Represents an LLM inference span.

    Content: What the LLM received and produced
    Metrics: Performance and usage statistics
    """

    # Identity
    span_id: str = field(metadata={"description": "Unique span identifier", "internal": True})
    parent_span_id: Optional[str] = field(
        default=None, metadata={"description": "Parent span ID for hierarchy reconstruction", "internal": True}
    )
    start_time: Optional[datetime] = field(default=None, metadata={"description": "Span start time", "internal": True})

    # Content
    input: List[Message] = field(
        default_factory=list, metadata={"description": "Conversation messages sent to the LLM"}
    )
    output: str = field(default="", metadata={"description": "LLM response text"})
    available_tools: List[ToolDefinition] = field(
        default_factory=list, metadata={"description": "Tools available to the LLM for this call"}
    )

    # Internal: tool calls requested by the LLM in its output (used by _get_agent_steps)
    _tool_calls: List[ToolCall] = field(
        default_factory=list, metadata={"description": "Tool calls from LLM output", "internal": True}
    )

    # Model info
    model: str = field(default="", metadata={"description": "Model name (e.g. gpt-4o)"})
    vendor: str = field(default="", metadata={"description": "Model vendor (e.g. openai)"})
    temperature: Optional[float] = field(default=None, metadata={"description": "LLM temperature setting"})

    # Metrics (separated)
    metrics: LLMMetrics = field(
        default_factory=LLMMetrics, metadata={"description": "LLM-specific performance metrics"}
    )

    def get_system_messages(self) -> List[SystemMessage]:
        """Get system messages only."""
        return [m for m in self.input if isinstance(m, SystemMessage)]

    def get_user_messages(self) -> List[UserMessage]:
        """Get user messages only."""
        return [m for m in self.input if isinstance(m, UserMessage)]

    def get_assistant_messages(self) -> List[AssistantMessage]:
        """Get assistant messages only."""
        return [m for m in self.input if isinstance(m, AssistantMessage)]

    def get_tool_messages(self) -> List[ToolMessage]:
        """Get tool result messages only."""
        return [m for m in self.input if isinstance(m, ToolMessage)]

    def format_messages(self) -> str:
        """Format conversation messages for LLM-friendly display."""
        lines: list[str] = []
        for msg in self.input:
            if isinstance(msg, SystemMessage):
                lines.append(f"[System]: {msg.content[:500]}")
            elif isinstance(msg, UserMessage):
                lines.append(f"[User]: {msg.content[:500]}")
            elif isinstance(msg, AssistantMessage):
                text = msg.content[:500] if msg.content else ""
                if msg.tool_calls:
                    tool_names = ", ".join(tc.name for tc in msg.tool_calls)
                    text += f" [calls tools: {tool_names}]" if text else f"[calls tools: {tool_names}]"
                lines.append(f"[Assistant]: {text}")
            elif isinstance(msg, ToolMessage):
                lines.append(f"[Tool Result]: {msg.content[:300]}")
        return "\n".join(lines) if lines else "(no messages)"

    def __str__(self) -> str:
        model_info = f" ({self.model})" if self.model else ""
        response_preview = self.output[:200] if self.output else "(no output)"
        return f"LLM{model_info}: {response_preview}"


@dataclass
class ToolSpan:
    """
    Represents a tool execution span.

    Content: Tool name, arguments, and result
    Metrics: Execution performance
    """

    # Identity
    span_id: str = field(metadata={"description": "Unique span identifier", "internal": True})
    parent_span_id: Optional[str] = field(
        default=None, metadata={"description": "Parent span ID for hierarchy reconstruction", "internal": True}
    )
    start_time: Optional[datetime] = field(default=None, metadata={"description": "Span start time", "internal": True})

    # Content
    name: str = field(default="", metadata={"description": "Tool name"})
    arguments: Dict[str, Any] = field(default_factory=dict, metadata={"description": "Arguments passed to the tool"})
    result: Any = field(default=None, metadata={"description": "Execution result"})

    # Metrics (separated)
    metrics: ToolMetrics = field(
        default_factory=ToolMetrics, metadata={"description": "Tool execution metrics", "internal": True}
    )

    def __str__(self) -> str:
        result_str = str(self.result)[:500] if self.result else "(no result)"
        return f"Tool '{self.name}': {result_str}"


@dataclass
class RetrieverSpan:
    """
    Represents a retrieval span (RAG).

    Content: Query and retrieved documents
    Metrics: Retrieval performance
    """

    # Identity
    span_id: str = field(metadata={"description": "Unique span identifier", "internal": True})
    parent_span_id: Optional[str] = field(
        default=None, metadata={"description": "Parent span ID for hierarchy reconstruction", "internal": True}
    )
    start_time: Optional[datetime] = field(default=None, metadata={"description": "Span start time", "internal": True})

    # Content
    query: str = field(default="", metadata={"description": "Retrieval query"})
    documents: List[RetrievedDoc] = field(default_factory=list, metadata={"description": "Retrieved documents"})

    # Configuration
    vector_db: str = field(default="", metadata={"description": "Vector database used"})
    top_k: int = field(default=0, metadata={"description": "Number of documents requested"})

    # Metrics (separated)
    metrics: RetrieverMetrics = field(
        default_factory=RetrieverMetrics, metadata={"description": "Retrieval performance metrics"}
    )

    def __str__(self) -> str:
        query_part = f" [query: '{self.query[:100]}']" if self.query else ""
        doc_count = len(self.documents)
        return f"Retriever{query_part}: {doc_count} document{'s' if doc_count != 1 else ''}"


@dataclass
class AgentSpan:
    """
    Represents an agent orchestration span.

    This is a marker span: "I'm agent X" with metadata.
    It does NOT have steps. AgentTrace (created via _create_agent_trace)
    is the reconstructed object that HAS steps.
    """

    # Identity
    span_id: str = field(metadata={"description": "Unique span identifier", "internal": True})
    parent_span_id: Optional[str] = field(
        default=None, metadata={"description": "Parent span ID for hierarchy reconstruction", "internal": True}
    )
    start_time: Optional[datetime] = field(default=None, metadata={"description": "Span start time", "internal": True})

    # Content
    name: str = field(default="", metadata={"description": "Name of the agent"})
    framework: str = field(default="", metadata={"description": "Framework (crewai, langchain, openai_agents, etc.)"})
    model: str = field(default="", metadata={"description": "LLM model used by the agent"})
    system_prompt: str = field(default="", metadata={"description": "System prompt / instructions"})
    available_tools: List[ToolDefinition] = field(
        default_factory=list, metadata={"description": "Tools available to the agent"}
    )
    max_iterations: Optional[int] = field(default=None, metadata={"description": "Maximum iterations allowed"})

    # Input/Output
    input: str = field(default="", metadata={"description": "Agent input"})
    output: str = field(default="", metadata={"description": "Agent output"})

    # Metrics (separated)
    metrics: AgentMetrics = field(default_factory=AgentMetrics, metadata={"description": "Agent performance metrics"})

    def __str__(self) -> str:
        parts = [f"Agent '{self.name}'"] if self.name else ["Agent"]
        details = []
        if self.framework:
            details.append(self.framework)
        if self.model:
            details.append(self.model)
        if details:
            parts.append(f"({', '.join(details)})")
        return " ".join(parts)


@dataclass
class ChainSpan:
    """
    Represents a structural/infrastructure span (chain, unknown, synthetic, etc.).

    These spans carry no semantic value for evaluation but are kept in
    trace.spans so the full trace tree can be reconstructed.
    """

    span_id: str = field(metadata={"internal": True})
    parent_span_id: Optional[str] = field(default=None, metadata={"internal": True})
    start_time: Optional[datetime] = field(default=None, metadata={"internal": True})
    name: str = field(default="", metadata={"internal": True})

    def __str__(self) -> str:
        return f"Chain '{self.name}'" if self.name else "Chain"


# ============================================================================
# SPAN UNION TYPE
# ============================================================================

# Union type for any span in the sequence
Span = LLMSpan | ToolSpan | RetrieverSpan | AgentSpan | ChainSpan


# ============================================================================
# TYPED AGENT STEPS (for AgentTrace)
# ============================================================================


@dataclass
class ToolCallInfo:
    """Info about a tool call request from an LLM."""

    id: str = field(metadata={"description": "Unique tool call identifier"})
    name: str = field(metadata={"description": "Name of the tool"})
    arguments: Dict[str, Any] = field(default_factory=dict, metadata={"description": "Arguments passed"})


@dataclass
class UserInputStep:
    """User input to the agent."""

    content: str = field(default="", metadata={"description": "User message content"})


@dataclass
class LLMReasoningStep:
    """
    LLM output — intermediate reasoning or final response.

    Both "reasoning" and "final answer" are LLM outputs. The only
    difference is whether tool_calls is populated. Use is_response
    to check.
    """

    content: str = field(default="", metadata={"description": "LLM response text"})
    tool_calls: List[ToolCallInfo] = field(
        default_factory=list, metadata={"description": "Tool calls requested by the LLM"}
    )
    llm_span_id: Optional[str] = field(
        default=None, metadata={"description": "Reference to full LLMSpan", "internal": True}
    )

    @property
    def is_response(self) -> bool:
        """True if this is a final response (no tool calls requested)."""
        return len(self.tool_calls) == 0

    def __str__(self) -> str:
        content_preview = self.content[:200] if self.content else "(empty)"
        if self.tool_calls:
            tool_names = ", ".join(tc.name for tc in self.tool_calls)
            return f"[LLM -> calls tools: {tool_names}]: {content_preview}"
        return f"[LLM response]: {content_preview}"


@dataclass
class ToolExecutionStep:
    """Tool execution and its result."""

    tool_name: str = field(default="", metadata={"description": "Name of the tool"})
    tool_call_id: Optional[str] = field(
        default=None, metadata={"description": "Correlates with LLMReasoningStep.tool_calls", "internal": True}
    )
    tool_input: Optional[Dict[str, Any]] = field(default=None, metadata={"description": "Input passed to the tool"})
    tool_output: Optional[Any] = field(default=None, metadata={"description": "Output returned by the tool"})
    content: str = field(default="", metadata={"description": "What was fed back to the LLM"})
    error: Optional[str] = field(default=None, metadata={"description": "Error message if failed"})
    duration_ms: Optional[float] = field(default=None, metadata={"description": "Execution duration in milliseconds"})
    nested_traces: List[Union[LLMSpan, "AgentTrace"]] = field(
        default_factory=list, metadata={"description": "Nested LLM calls or sub-agent traces"}
    )

    def __str__(self) -> str:
        result_preview = str(self.tool_output)[:200] if self.tool_output else "(no result)"
        error_note = f" [ERROR: {self.error}]" if self.error else ""
        return f"[Tool '{self.tool_name}']{error_note}: {result_preview}"


AgentStep = Union[UserInputStep, LLMReasoningStep, ToolExecutionStep]


# ============================================================================
# AGENT TRACE - Agent-scoped view for agent-level evaluation
# ============================================================================


@dataclass
class AgentTrace:
    """
    Agent-scoped view of a trace for agent-level evaluation.

    Contains the reconstructed execution steps (typed: UserInputStep, LLMReasoningStep,
    ToolExecutionStep), agent metadata, available tools, and agent-level metrics.

    Created via Trace._create_agent_trace(agent_span_id).
    """

    # Identity
    agent_id: str = field(metadata={"description": "Agent span identifier", "internal": True})

    # I/O (from AgentSpan)
    input: str = field(default="", metadata={"description": "Agent input"})
    output: str = field(default="", metadata={"description": "Agent output"})

    # Reconstructed execution steps (typed)
    steps: List[AgentStep] = field(
        default_factory=list,
        metadata={"description": "Execution steps: UserInputStep, LLMReasoningStep, or ToolExecutionStep"},
    )

    # Metadata (from AgentSpan)
    agent_name: str = field(default="", metadata={"description": "Name of the agent"})
    framework: str = field(
        default="", metadata={"description": "Framework (crewai, langchain, openai_agents, etc.)", "internal": True}
    )
    model: str = field(default="", metadata={"description": "LLM model used by the agent"})
    system_prompt: str = field(default="", metadata={"description": "System prompt / instructions"})
    available_tools: List[ToolDefinition] = field(
        default_factory=list, metadata={"description": "Tools available to the agent"}
    )

    # Agent-level metrics
    metrics: TraceMetrics = field(
        default_factory=TraceMetrics, metadata={"description": "Aggregated performance metrics"}
    )

    def get_tool_steps(self) -> List[ToolExecutionStep]:
        """Get all tool execution steps."""
        return [s for s in self.steps if isinstance(s, ToolExecutionStep)]

    def get_llm_steps(self) -> List[LLMReasoningStep]:
        """Get all LLM output steps (both intermediate reasoning and final response)."""
        return [s for s in self.steps if isinstance(s, LLMReasoningStep)]

    def get_error_steps(self) -> List[ToolExecutionStep]:
        """Get tool steps that produced errors."""
        return [s for s in self.get_tool_steps() if s.error]

    def get_sub_agents(self) -> List["AgentTrace"]:
        """Get all sub-agent traces from nested tool executions."""
        traces = []
        for step in self.get_tool_steps():
            for t in step.nested_traces:
                if isinstance(t, AgentTrace):
                    traces.append(t)
        return traces

    def format_steps(self) -> str:
        """Format execution steps as a numbered list for LLM-friendly display."""
        if not self.steps:
            return "  (no steps recorded)"
        lines = []
        for i, step in enumerate(self.steps):
            lines.append(f"  Step {i + 1}: {step}")
        return "\n".join(lines)


# ============================================================================
# HELPER FUNCTIONS
# ============================================================================


def _hash_message(msg: Message) -> str:
    """
    Create hash of message for deduplication.

    Args:
        msg: Message object to hash

    Returns:
        SHA256 hash of message content
    """
    import hashlib

    # Determine role from type
    if isinstance(msg, SystemMessage):
        role = "system"
    elif isinstance(msg, UserMessage):
        role = "user"
    elif isinstance(msg, AssistantMessage):
        role = "assistant"
    elif isinstance(msg, ToolMessage):
        role = "tool"
    else:
        role = "unknown"

    content = f"{role}:{msg.content or ''}"
    if isinstance(msg, AssistantMessage) and msg.tool_calls:
        tool_sig = ",".join(f"{tc.name}:{tc.arguments}" for tc in msg.tool_calls)
        content += f"|tools:{tool_sig}"
    return hashlib.sha256(content.encode()).hexdigest()


# ============================================================================
# TRACE CLASS
# ============================================================================


@dataclass
class Trace:
    """
    Evaluation-optimized trace representation.

    A trace is the complete execution path of an agent, preserving
    the temporal sequence of all operations (LLM calls, tool executions, etc.).

    This is the main data structure used by trace-level evaluators. It provides:

    1. **Filtered span access** via get_llm_calls(), get_tool_calls(), get_retrievals(), get_agents()
       - Easy access to specific span types
       - Option to include/exclude nested spans

    2. **Formatting methods** for LLM-friendly display:
       - format_spans() — full span tree visualization
       - format_evidence() — structured tool results + retrieved docs

    3. **Aggregated metrics** via the metrics property
       - Token usage, latency, error counts
    """

    # Identity
    trace_id: str = field(metadata={"description": "Unique trace identifier", "internal": True})

    # Trace-level I/O
    input: str = field(default="", metadata={"description": "User input / query"})
    output: str = field(default="", metadata={"description": "Agent output / final response"})

    # Sequential execution spans (raw spans, ordered by start_time)
    spans: List[Span] = field(
        default_factory=list, metadata={"description": "All execution spans ordered by start time"}
    )

    # Aggregated metrics
    metrics: TraceMetrics = field(
        default_factory=TraceMetrics, metadata={"description": "Aggregated performance metrics"}
    )

    # Metadata
    timestamp: Optional[datetime] = field(default=None, metadata={"description": "Trace timestamp", "internal": True})

    # ========================================================================
    # INTERNAL: Reconstructed conversation steps (used by SDK framework)
    # ========================================================================

    def _get_agent_steps(
        self, agent_span_id: Optional[str] = None, deduplicate_messages: bool = False
    ) -> List[AgentStep]:
        """
        Get reconstructed conversation steps for evaluation.

        Returns a logical conversation flow using typed steps:
        - UserInputStep: User input
        - LLMReasoningStep: LLM responses (with tool_calls if any, is_response=True for final)
        - ToolExecutionStep: Tool results (with nested_traces if tool called LLM/agent)

        Args:
            agent_span_id: Specific agent to get steps for (for multi-agent).
                          If None, returns steps for the entire trace.
            deduplicate_messages: If True, remove duplicate messages across
                                LLM spans (useful for multi-turn conversations).
                                Default: False

        Returns:
            List of AgentStep objects representing the conversation flow.
        """
        # Get relevant spans
        if agent_span_id:
            spans = self._get_root_level_spans(agent_span_id)
        else:
            spans = self._get_root_level_spans()

        return self._reconstruct_steps(spans, deduplicate_messages=deduplicate_messages)

    def _get_root_level_spans(self, agent_span_id: Optional[str] = None) -> List[Span]:
        """Get spans that are at the root level (not nested inside tools).

        Args:
            agent_span_id: If provided, restrict to descendants of this agent span
                           while still excluding any span that has a tool ancestor.
        """
        # Find all tool span IDs
        tool_span_ids = {s.span_id for s in self.spans if isinstance(s, ToolSpan)}

        if agent_span_id:
            candidate_spans = self._get_descendant_spans(agent_span_id)
        else:
            candidate_spans = self.spans

        # Build a lookup map for ancestor traversal
        span_by_id: Dict[str, Span] = {s.span_id: s for s in self.spans}

        def has_tool_ancestor(span_id: str) -> bool:
            """Walk up parent chain to check if any ancestor is a tool span."""
            visited: set = set()
            current_id: Optional[str] = span_id
            while current_id:
                if current_id in visited:
                    break
                visited.add(current_id)
                if current_id in tool_span_ids:
                    return True
                parent_span = span_by_id.get(current_id)
                if parent_span is None:
                    break
                current_id = getattr(parent_span, "parent_span_id", None)
            return False

        # Root spans are those with no tool ancestor
        root_spans = []
        for span in candidate_spans:
            parent_id = getattr(span, "parent_span_id", None)
            if parent_id is None or not has_tool_ancestor(parent_id):
                root_spans.append(span)
        return root_spans

    def _get_descendant_spans(self, parent_id: str, _visited: Optional[set] = None) -> List[Span]:
        """Get all descendants of a span (recursive)."""
        if _visited is None:
            _visited = set()
        descendants = []
        for span in self.spans:
            if getattr(span, "parent_span_id", None) == parent_id and span.span_id not in _visited:
                _visited.add(span.span_id)
                descendants.append(span)
                descendants.extend(self._get_descendant_spans(span.span_id, _visited))
        return descendants

    def _get_children_of(self, parent_id: str) -> List[Span]:
        """Get direct children of a span."""
        return [s for s in self.spans if getattr(s, "parent_span_id", None) == parent_id]

    def _reconstruct_steps(self, spans: List[Span], deduplicate_messages: bool = False) -> List[AgentStep]:
        """
        Reconstruct logical conversation steps from spans using typed step classes.

        LLMSpans are the single source of truth for conversation history: they contain
        the full message sequence (UserInput, AssistantReasoning, ToolResults) accumulated
        across all turns. ToolSpans and RetrieverSpans are used as enrichment — when
        present they replace the ToolMessage-derived steps with richer execution data
        (actual tool_input, error details, duration).

        Args:
            spans: List of spans to reconstruct
            deduplicate_messages: If True, remove duplicate messages across LLM spans
        """
        llm_spans = [s for s in spans if isinstance(s, LLMSpan)]
        tool_spans = [s for s in spans if isinstance(s, ToolSpan)]
        retriever_spans = [s for s in spans if isinstance(s, RetrieverSpan)]

        seen_messages: Optional[set] = set() if deduplicate_messages else None
        steps: List[AgentStep] = []

        # Step 1: Extract all steps from LLM conversation history (primary source)
        for llm_span in llm_spans:
            steps.extend(self._reconstruct_llm_steps(llm_span, seen_messages))

        # Step 2: Enrich ToolExecutionSteps with ToolSpan data (richer execution details)
        if tool_spans:
            steps = self._enrich_tool_steps(steps, tool_spans)

        # Step 3: Append RetrieverSpan steps (retrievals have no LLM message equivalent)
        for retriever_span in retriever_spans:
            steps.append(self._reconstruct_tool_step_from_retriever(retriever_span))

        return steps

    def _enrich_tool_steps(self, steps: List[AgentStep], tool_spans: List[ToolSpan]) -> List[AgentStep]:
        """
        Enrich ToolExecutionSteps derived from ToolMessages with richer ToolSpan data.

        Matching strategy (in priority order):
        1. Exact tool name match among remaining unmatched spans.
        2. Positional fallback — first remaining span when names differ (e.g. the
           framework did not report a name on the span, or naming conventions differ).

        An unmatched span never advances the matching index for subsequent steps,
        preventing cascading misalignment when a ToolSpan has no corresponding ToolMessage.

        ToolSpans provide: actual tool_input (parsed args), real tool_output (unwrapped),
        error details, and duration_ms. ToolMessages provide: tool_output as the content
        fed back to the LLM (may differ from raw result for error-handling wrappers).
        """
        remaining_spans = list(tool_spans)  # mutable; consumed via pop()
        enriched: List[AgentStep] = []
        for step in steps:
            if isinstance(step, ToolExecutionStep):
                # 1. Prefer exact name match
                matched_idx: Optional[int] = None
                for i, ts in enumerate(remaining_spans):
                    if ts.name == step.tool_name:
                        matched_idx = i
                        break
                # 2. Positional fallback: first remaining span
                if matched_idx is None and remaining_spans:
                    matched_idx = 0

                if matched_idx is not None:
                    ts = remaining_spans.pop(matched_idx)
                    error_info = None
                    if ts.metrics.error:
                        error_info = ts.metrics.error_message or ts.metrics.error_type or "Error"
                    enriched.append(
                        ToolExecutionStep(
                            tool_name=ts.name,
                            tool_input=ts.arguments if ts.arguments is not None else step.tool_input,
                            tool_output=ts.result if ts.result is not None else step.tool_output,
                            tool_call_id=step.tool_call_id,
                            content=step.content,
                            nested_traces=self._get_tool_nested_traces(ts),
                            duration_ms=ts.metrics.duration_ms,
                            error=error_info,
                        )
                    )
                else:
                    enriched.append(step)
            else:
                enriched.append(step)

        # Append any ToolSpans not matched to a ToolMessage (e.g. tool was called but
        # no subsequent LLM call with the result was recorded in the trace).
        for ts in remaining_spans:
            error_info = None
            if ts.metrics.error:
                error_info = ts.metrics.error_message or ts.metrics.error_type or "Error"
            enriched.append(
                ToolExecutionStep(
                    tool_name=ts.name,
                    tool_input=ts.arguments,
                    tool_output=ts.result,
                    content=str(ts.result) if ts.result is not None else "",
                    nested_traces=self._get_tool_nested_traces(ts),
                    duration_ms=ts.metrics.duration_ms,
                    error=error_info,
                )
            )
        return enriched

    def _get_tool_nested_traces(self, tool_span: ToolSpan) -> List[Union[LLMSpan, "AgentTrace"]]:
        """Get nested LLM spans or AgentTraces inside a tool span."""
        nested_spans = self._get_children_of(tool_span.span_id)
        nested_traces: List[Union[LLMSpan, "AgentTrace"]] = []
        for nested in nested_spans:
            if isinstance(nested, LLMSpan):
                nested_traces.append(nested)
            elif isinstance(nested, AgentSpan):
                try:
                    nested_traces.append(self._create_agent_trace(nested.span_id))
                except ValueError:
                    pass
        return nested_traces

    def _reconstruct_llm_steps(
        self,
        llm_span: LLMSpan,
        seen_messages: Optional[set] = None,
    ) -> List[AgentStep]:
        """
        Reconstruct typed steps from an LLM span with optional deduplication.

        Extracts UserInputStep, ToolExecutionStep (from ToolMessages), and
        LLMReasoningStep from the span's input/output. ToolExecutionSteps produced
        here carry tool_output from the message content; callers can enrich them
        with ToolSpan data via _enrich_tool_steps().

        Args:
            llm_span: LLM span to reconstruct
            seen_messages: Set of message hashes for deduplication (or None to disable)
        """
        steps: List[AgentStep] = []

        # Build a lookup from tool_call_id -> tool name from assistant messages.
        tool_call_names: Dict[str, str] = {}
        for msg in llm_span.input:
            if isinstance(msg, AssistantMessage):
                for tc in msg.tool_calls:
                    tool_call_names[tc.id] = tc.name

        # Track the pending tool calls from the most recent AssistantMessage (for
        # positional fallback when tool_call_id is absent from ToolMessages).
        pending_tool_calls: List[ToolCall] = []
        pending_index = 0

        # Extract messages into typed steps
        for msg in llm_span.input:
            # Deduplication: track seen status but don't skip AssistantMessages early —
            # we need to reset positional tracking even for duplicates so that any new
            # ToolMessages following a duplicate AssistantMessage resolve correctly.
            is_duplicate = False
            if seen_messages is not None:
                msg_hash = _hash_message(msg)
                if msg_hash in seen_messages:
                    is_duplicate = True
                else:
                    seen_messages.add(msg_hash)

            if isinstance(msg, AssistantMessage):
                # Always reset positional tracking for each group of tool calls,
                # regardless of whether this AssistantMessage was seen before.
                pending_tool_calls = list(msg.tool_calls)
                pending_index = 0
                continue  # AssistantMessages don't produce steps

            if is_duplicate:
                continue  # Skip all other duplicate messages

            if isinstance(msg, SystemMessage):
                # System messages are metadata, skip as steps
                # (stored in AgentTrace.system_prompt instead)
                pass
            elif isinstance(msg, UserMessage):
                steps.append(UserInputStep(content=msg.content))
            elif isinstance(msg, ToolMessage):
                # Resolve tool name: id-based lookup only when id is present,
                # otherwise fall back to positional matching against the preceding
                # AssistantMessage's tool_calls list.
                resolved_name = None
                if msg.tool_call_id:
                    resolved_name = tool_call_names.get(msg.tool_call_id)
                if not resolved_name and pending_index < len(pending_tool_calls):
                    resolved_name = pending_tool_calls[pending_index].name
                pending_index += 1
                steps.append(
                    ToolExecutionStep(
                        tool_name=resolved_name or "",
                        tool_call_id=msg.tool_call_id,
                        tool_output=msg.content,
                        content=msg.content,
                    )
                )

        # Add LLM response as LLMReasoningStep
        if llm_span.output or llm_span._tool_calls:
            tool_call_infos = [
                ToolCallInfo(id=tc.id, name=tc.name, arguments=tc.arguments) for tc in llm_span._tool_calls
            ]
            steps.append(
                LLMReasoningStep(
                    content=llm_span.output,
                    tool_calls=tool_call_infos,
                    llm_span_id=llm_span.span_id,
                )
            )

        return steps

    def _reconstruct_tool_step_from_retriever(self, retriever_span: RetrieverSpan) -> ToolExecutionStep:
        """Reconstruct a retrieval as a ToolExecutionStep."""
        docs_content = "\n".join(doc.content for doc in retriever_span.documents if doc.content)
        return ToolExecutionStep(
            tool_name="retrieval",
            tool_input={"query": retriever_span.query} if retriever_span.query else None,
            tool_output={"documents": [{"content": d.content, "score": d.score} for d in retriever_span.documents]},
            content=docs_content,
            duration_ms=retriever_span.metrics.duration_ms,
            error=retriever_span.metrics.error_message if retriever_span.metrics.error else None,
        )

    # ========================================================================
    # FILTERED SPAN ACCESS
    # ========================================================================

    def get_llm_calls(
        self,
        include_nested: bool = True,
        agent_span_id: Optional[str] = None,
        deduplicate_messages: bool = False,
    ) -> List[LLMSpan]:
        """
        Get all LLM calls with enhanced filtering and deduplication.

        Args:
            include_nested: If True, includes LLM calls nested inside tools.
                           If False, only returns root-level LLM calls.
            agent_span_id: If provided, only return LLM calls that are descendants
                          of this agent span (for multi-agent filtering).
            deduplicate_messages: If True, remove duplicate messages across LLM calls.

        Returns:
            List of LLMSpan objects.
        """
        # Start with all or root-level LLM spans
        if include_nested:
            llms = [s for s in self.spans if isinstance(s, LLMSpan)]
        else:
            tool_span_ids = {s.span_id for s in self.spans if isinstance(s, ToolSpan)}
            llms = [
                s
                for s in self.spans
                if isinstance(s, LLMSpan) and getattr(s, "parent_span_id", None) not in tool_span_ids
            ]

        # Filter by agent if specified
        if agent_span_id:
            llms = [llm for llm in llms if self._is_descendant_of(llm, agent_span_id)]

        # Deduplicate messages if requested
        if deduplicate_messages:
            llms = self._deduplicate_llm_messages(llms)

        return llms

    def get_tool_calls(
        self,
        include_nested: bool = True,
        agent_span_id: Optional[str] = None,
    ) -> List[ToolSpan]:
        """
        Get all tool executions with agent filtering.

        Args:
            include_nested: If True, includes nested tool calls (tools calling tools).
                           If False, only returns root-level tool calls.
            agent_span_id: If provided, only return tool calls that are descendants
                          of this agent span (for multi-agent filtering).

        Returns:
            List of ToolSpan objects.
        """
        if include_nested:
            tools = [s for s in self.spans if isinstance(s, ToolSpan)]
        else:
            tool_span_ids = {s.span_id for s in self.spans if isinstance(s, ToolSpan)}
            tools = [
                s
                for s in self.spans
                if isinstance(s, ToolSpan) and getattr(s, "parent_span_id", None) not in tool_span_ids
            ]

        if agent_span_id:
            tools = [tool for tool in tools if self._is_descendant_of(tool, agent_span_id)]

        return tools

    def get_retrievals(self, agent_span_id: Optional[str] = None) -> List[RetrieverSpan]:
        """
        Get all retrieval operations with agent filtering.

        Args:
            agent_span_id: If provided, only return retrievals that are descendants
                          of this agent span.

        Returns:
            List of RetrieverSpan objects.
        """
        retrievals = [s for s in self.spans if isinstance(s, RetrieverSpan)]

        if agent_span_id:
            retrievals = [r for r in retrievals if self._is_descendant_of(r, agent_span_id)]

        return retrievals

    def get_agents(self) -> List[AgentSpan]:
        """
        Get all agent spans (for multi-agent systems).

        Returns:
            List of AgentSpan objects.
        """
        return [s for s in self.spans if isinstance(s, AgentSpan)]

    def _get_root_span(self) -> Optional[Span]:
        """Get the root span of the trace (the span with no parent)."""
        return next(
            (s for s in self.spans if getattr(s, "parent_span_id", None) is None),
            None,
        )

    def format_evidence(self) -> str:
        """
        Format tool results and retrieved documents for LLM-friendly display.

        Produces structured output suitable for evaluation prompts.
        Replaces the old get_context() with a more structured format.
        """
        sections: list[str] = []

        tools = self.get_tool_calls()
        if tools:
            tool_lines = [f"  {t}" for t in tools[:10]]
            sections.append("Tool Results:\n" + "\n".join(tool_lines))

        retrievals = self.get_retrievals()
        if retrievals:
            doc_lines: list[str] = []
            for r in retrievals[:5]:
                doc_lines.append(f"  {r}")
                for doc in r.documents[:3]:
                    doc_lines.append(f"    - {doc}")
            sections.append("Retrieved Documents:\n" + "\n".join(doc_lines))

        return "\n\n".join(sections) if sections else "(no evidence available)"

    def format_spans(self) -> str:
        """
        Render the full span tree using parent_span_id hierarchy.

        Produces a tree visualization of all spans in the trace.
        """
        # Build parent -> children map
        children_map: Dict[Optional[str], list] = {}
        for span in self.spans:
            parent_id = getattr(span, "parent_span_id", None)
            children_map.setdefault(parent_id, []).append(span)

        lines: list[str] = []

        def _render(span_id: Optional[str], prefix: str, is_last: bool, is_root: bool) -> None:
            span = next((s for s in self.spans if s.span_id == span_id), None) if span_id else None
            if span is not None:
                connector = "" if is_root else ("└── " if is_last else "├── ")
                lines.append(f"{prefix}{connector}{span}")

            children = children_map.get(span_id, [])
            child_prefix = prefix if is_root else (prefix + ("    " if is_last else "│   "))
            for i, child in enumerate(children):
                _render(child.span_id, child_prefix, i == len(children) - 1, False)

        # Find root spans (no parent or parent not in trace)
        all_span_ids = {s.span_id for s in self.spans}
        root_spans = [
            s
            for s in self.spans
            if getattr(s, "parent_span_id", None) is None or getattr(s, "parent_span_id", None) not in all_span_ids
        ]

        for i, root in enumerate(root_spans):
            if i > 0:
                lines.append("")  # blank line between root trees
            connector = ""
            lines.append(f"{connector}{root}")
            children = children_map.get(root.span_id, [])
            for j, child in enumerate(children):
                _render(child.span_id, "", j == len(children) - 1, False)

        return "\n".join(lines) if lines else "(no spans)"

    # ========================================================================
    # DEDUPLICATION AND FILTERING HELPERS
    # ========================================================================

    def _deduplicate_llm_messages(self, llm_spans: List[LLMSpan]) -> List[LLMSpan]:
        """
        Remove duplicate messages across LLM spans (primarily system messages).

        Args:
            llm_spans: List of LLM spans to deduplicate

        Returns:
            List of LLMSpan objects with unique messages only
        """
        import dataclasses

        seen_messages = set()
        deduplicated = []

        for llm_span in llm_spans:
            unique_messages = []
            for msg in llm_span.input:
                msg_hash = _hash_message(msg)
                if msg_hash not in seen_messages:
                    unique_messages.append(msg)
                    seen_messages.add(msg_hash)

            if unique_messages or llm_span.output or llm_span._tool_calls:
                new_span = dataclasses.replace(llm_span, input=unique_messages)
                deduplicated.append(new_span)

        return deduplicated

    def _is_descendant_of(self, span: Span, ancestor_span_id: str) -> bool:
        """Check if span is a descendant of ancestor by walking parent chain."""
        current_id = getattr(span, "parent_span_id", None)
        visited = set()

        while current_id:
            if current_id in visited:
                return False
            visited.add(current_id)

            if current_id == ancestor_span_id:
                return True

            parent_span = next((s for s in self.spans if hasattr(s, "span_id") and s.span_id == current_id), None)
            if not parent_span:
                break

            current_id = getattr(parent_span, "parent_span_id", None)

        return False

    def _create_agent_trace(self, agent_span_id: str) -> AgentTrace:
        """
        Create an AgentTrace scoped to a specific agent's execution.

        Gathers agent metadata from AgentSpan, reconstructs conversation steps
        with deduplication, and calculates agent-level metrics from descendant spans.

        Args:
            agent_span_id: The span_id of the AgentSpan to create a trace for

        Returns:
            AgentTrace with typed steps, metadata, and metrics for this agent

        Raises:
            ValueError: If agent_span_id not found in trace spans
        """
        agent_span = next(
            (s for s in self.spans if isinstance(s, AgentSpan) and s.span_id == agent_span_id),
            None,
        )
        if agent_span is None:
            raise ValueError(f"Agent span '{agent_span_id}' not found in trace '{self.trace_id}'")

        # Reconstruct steps: LLMSpans as source of truth, ToolSpans as enrichment
        agent_steps = self._get_agent_steps(agent_span_id=agent_span_id, deduplicate_messages=True)

        # Calculate agent-level metrics from descendant spans
        descendant_spans = self._get_descendant_spans(agent_span_id)
        llm_spans = [s for s in descendant_spans if isinstance(s, LLMSpan)]

        token_usage = TokenUsage()
        for llm in llm_spans:
            if llm.metrics and llm.metrics.token_usage:
                token_usage = token_usage + llm.metrics.token_usage

        agent_metrics = TraceMetrics(
            total_duration_ms=agent_span.metrics.duration_ms or 0,
            token_usage=token_usage,
            error_count=sum(1 for s in descendant_spans if getattr(getattr(s, "metrics", None), "error", False)),
        )

        # Fall back to SystemMessage from the first LLM span if agent span lacks it
        system_prompt = agent_span.system_prompt
        if not system_prompt:
            for llm in llm_spans:
                for msg in llm.input:
                    if isinstance(msg, SystemMessage) and msg.content:
                        system_prompt = msg.content
                        break
                if system_prompt:
                    break

        return AgentTrace(
            agent_id=agent_span.span_id,
            agent_name=agent_span.name,
            framework=agent_span.framework,
            model=agent_span.model,
            system_prompt=system_prompt,
            available_tools=list(agent_span.available_tools),
            input=agent_span.input,
            output=agent_span.output,
            steps=agent_steps,
            metrics=agent_metrics,
        )
