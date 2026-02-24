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
- AgentTrace → steps (reconstructed execution flow: UserStep, LLMStep, ToolExecutionStep)
- LLMSpan → messages (typed conversation: SystemMessage, UserMessage, AssistantMessage, ToolMessage)
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import List, Dict, Any, Optional, Tuple, Union
from datetime import datetime


# ============================================================================
# METRIC DATACLASSES
# ============================================================================


@dataclass
class TokenUsage:
    """Token usage statistics from LLM calls."""

    input_tokens: int = 0
    output_tokens: int = 0
    total_tokens: int = 0
    cache_read_tokens: int = 0  # For cached prompt tokens (if supported)

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

    duration_ms: float = 0.0
    error: bool = False
    error_type: Optional[str] = None
    error_message: Optional[str] = None


@dataclass
class LLMMetrics(SpanMetrics):
    """Metrics specific to LLM spans."""

    token_usage: TokenUsage = field(default_factory=TokenUsage)

    # Timing breakdown (if available)
    time_to_first_token_ms: Optional[float] = None


@dataclass
class ToolMetrics(SpanMetrics):
    """Metrics specific to tool execution spans."""

    pass  # Currently only base metrics, but can extend later


@dataclass
class RetrieverMetrics(SpanMetrics):
    """Metrics specific to retriever spans."""

    documents_retrieved: int = 0


@dataclass
class AgentMetrics(SpanMetrics):
    """Metrics specific to agent spans."""

    token_usage: TokenUsage = field(default_factory=TokenUsage)


@dataclass
class TraceMetrics:
    """
    Aggregated metrics for the entire trace.

    These are the observable counts we can reliably measure
    from any agent trace, regardless of framework.
    """

    # Duration
    total_duration_ms: float = 0.0

    # Token aggregates
    token_usage: TokenUsage = field(default_factory=TokenUsage)

    # Observable counts
    total_span_count: int = 0  # All spans parsed (excluding skipped)
    llm_call_count: int = 0  # Number of LLM spans
    tool_call_count: int = 0  # Number of tool spans
    retrieval_count: int = 0  # Number of retriever spans
    agent_span_count: int = 0  # Number of agent spans

    # Error tracking
    error_count: int = 0  # Spans with errors

    @property
    def has_errors(self) -> bool:
        """Check if any errors occurred in the trace."""
        return self.error_count > 0

    @property
    def avg_tokens_per_llm_call(self) -> float:
        """Average tokens per LLM call."""
        if self.llm_call_count == 0:
            return 0.0
        return self.token_usage.total_tokens / self.llm_call_count


# ============================================================================
# TOOL CALL AND RETRIEVAL STRUCTURES
# ============================================================================


@dataclass
class ToolCall:
    """Represents a tool call made by an LLM."""

    id: str
    name: str
    arguments: Dict[str, Any] = field(default_factory=dict)


@dataclass
class RetrievedDoc:
    """Represents a retrieved document from a vector store."""

    id: str = ""
    content: str = ""
    score: float = 0.0
    metadata: Dict[str, Any] = field(default_factory=dict)


# ============================================================================
# TYPED MESSAGES (for LLMSpan)
# ============================================================================


@dataclass
class SystemMessage:
    """System prompt / instructions."""

    content: str = ""


@dataclass
class UserMessage:
    """User input to the LLM."""

    content: str = ""


@dataclass
class AssistantMessage:
    """LLM's response, optionally requesting tool calls."""

    content: str = ""
    tool_calls: List[ToolCall] = field(default_factory=list)


@dataclass
class ToolMessage:
    """Tool result fed back to the LLM."""

    content: str = ""
    tool_call_id: str = ""


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
    span_id: str
    parent_span_id: Optional[str] = None  # For hierarchy reconstruction
    start_time: Optional[datetime] = None  # For ordering

    # Content
    messages: List[Message] = field(default_factory=list)
    response: str = ""
    tool_calls: List[ToolCall] = field(default_factory=list)

    # Model info
    model: str = ""
    vendor: str = ""
    temperature: Optional[float] = None

    # Metrics (separated)
    metrics: LLMMetrics = field(default_factory=LLMMetrics)

    # Convenience properties for filtering messages by type
    @property
    def system_messages(self) -> List[SystemMessage]:
        return [m for m in self.messages if isinstance(m, SystemMessage)]

    @property
    def user_messages(self) -> List[UserMessage]:
        return [m for m in self.messages if isinstance(m, UserMessage)]

    @property
    def assistant_messages(self) -> List[AssistantMessage]:
        return [m for m in self.messages if isinstance(m, AssistantMessage)]

    @property
    def tool_messages(self) -> List[ToolMessage]:
        return [m for m in self.messages if isinstance(m, ToolMessage)]


@dataclass
class ToolSpan:
    """
    Represents a tool execution span.

    Content: Tool name, arguments, and result
    Metrics: Execution performance
    """

    # Identity
    span_id: str
    parent_span_id: Optional[str] = None  # For hierarchy reconstruction
    start_time: Optional[datetime] = None  # For ordering

    # Content
    name: str = ""
    arguments: Dict[str, Any] = field(default_factory=dict)
    result: Any = None

    # Metrics (separated)
    metrics: ToolMetrics = field(default_factory=ToolMetrics)


@dataclass
class RetrieverSpan:
    """
    Represents a retrieval span (RAG).

    Content: Query and retrieved documents
    Metrics: Retrieval performance
    """

    # Identity
    span_id: str
    parent_span_id: Optional[str] = None  # For hierarchy reconstruction
    start_time: Optional[datetime] = None  # For ordering

    # Content
    query: str = ""
    documents: List[RetrievedDoc] = field(default_factory=list)

    # Configuration
    vector_db: str = ""
    top_k: int = 0

    # Metrics (separated)
    metrics: RetrieverMetrics = field(default_factory=RetrieverMetrics)


@dataclass
class AgentSpan:
    """
    Represents an agent orchestration span.

    This is a marker span: "I'm agent X" with metadata.
    It does NOT have steps. AgentTrace (created via create_agent_trace)
    is the reconstructed object that HAS steps.
    """

    # Identity
    span_id: str
    parent_span_id: Optional[str] = None  # For hierarchy reconstruction
    start_time: Optional[datetime] = None  # For ordering

    # Content
    name: str = ""
    framework: str = ""  # "crewai", "langchain", "openai_agents", etc.
    model: str = ""
    system_prompt: str = ""
    available_tools: List[str] = field(default_factory=list)
    max_iterations: Optional[int] = None

    # Input/Output
    input: str = ""
    output: str = ""

    # Metrics (separated)
    metrics: AgentMetrics = field(default_factory=AgentMetrics)


# ============================================================================
# SPAN UNION TYPE
# ============================================================================

# Union type for any span in the sequence
Span = LLMSpan | ToolSpan | RetrieverSpan | AgentSpan


# ============================================================================
# TYPED AGENT STEPS (for AgentTrace)
# ============================================================================


@dataclass
class ToolCallInfo:
    """Info about a tool call request from an LLM."""

    id: str
    name: str
    arguments: Dict[str, Any] = field(default_factory=dict)


@dataclass
class UserStep:
    """User input to the agent."""

    content: str = ""


@dataclass
class LLMStep:
    """
    LLM output — intermediate reasoning or final response.

    Both "reasoning" and "final answer" are LLM outputs. The only
    difference is whether tool_calls is populated. Use is_response
    to check.
    """

    content: str = ""
    tool_calls: List[ToolCallInfo] = field(default_factory=list)
    llm_span_id: Optional[str] = None  # Reference to full LLMSpan

    @property
    def is_response(self) -> bool:
        """True if this is a final response (no tool calls requested)."""
        return len(self.tool_calls) == 0


@dataclass
class ToolExecutionStep:
    """Tool execution and its result."""

    tool_name: str = ""
    tool_call_id: Optional[str] = None  # Correlates with LLMStep.tool_calls
    tool_input: Optional[Dict[str, Any]] = None
    tool_output: Optional[Any] = None  # Actual structured result from ToolSpan
    content: str = ""  # What was fed back to the LLM (may differ)
    error: Optional[str] = None
    duration_ms: Optional[float] = None
    # Nested execution — LLM calls or sub-agent calls within this tool
    nested_traces: List[Union[LLMSpan, "AgentTrace"]] = field(default_factory=list)


AgentStep = Union[UserStep, LLMStep, ToolExecutionStep]


# ============================================================================
# AGENT TRACE - Agent-scoped view for agent-level evaluation
# ============================================================================


@dataclass
class AgentTrace:
    """
    Agent-scoped view of a trace for agent-level evaluation.

    Contains the reconstructed execution steps (typed: UserStep, LLMStep,
    ToolExecutionStep), agent metadata, available tools, and agent-level metrics.

    Created via Trace.create_agent_trace(agent_span_id).
    """

    # Identity
    agent_id: str  # AgentSpan.span_id

    # Metadata (from AgentSpan)
    agent_name: str = ""
    framework: str = ""
    model: str = ""
    system_prompt: str = ""
    available_tools: List[str] = field(default_factory=list)

    # I/O (from AgentSpan)
    input: str = ""
    output: str = ""

    # Reconstructed execution steps (typed)
    steps: List[AgentStep] = field(default_factory=list)

    # Agent-level metrics
    metrics: TraceMetrics = field(default_factory=TraceMetrics)

    # Convenience properties
    @property
    def tool_steps(self) -> List[ToolExecutionStep]:
        return [s for s in self.steps if isinstance(s, ToolExecutionStep)]

    @property
    def llm_steps(self) -> List[LLMStep]:
        """All LLM output steps (both intermediate reasoning and final response)."""
        return [s for s in self.steps if isinstance(s, LLMStep)]

    @property
    def response_step(self) -> Optional[LLMStep]:
        """The final response step (last LLMStep where is_response=True), or None."""
        for s in reversed(self.steps):
            if isinstance(s, LLMStep) and s.is_response:
                return s
        return None

    @property
    def tool_names_used(self) -> List[str]:
        return [s.tool_name for s in self.tool_steps]

    @property
    def has_errors(self) -> bool:
        return any(s.error for s in self.tool_steps)

    @property
    def error_steps(self) -> List[ToolExecutionStep]:
        return [s for s in self.tool_steps if s.error]

    @property
    def sub_agent_traces(self) -> List["AgentTrace"]:
        """Get all sub-agent traces from nested tool executions."""
        traces = []
        for step in self.tool_steps:
            for t in step.nested_traces:
                if isinstance(t, AgentTrace):
                    traces.append(t)
        return traces


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

    This is the main data structure used by evaluators. It provides:

    1. **Reconstructed conversation steps** via get_agent_steps()
       - Logical conversation flow for evaluators
       - Handles nested tool calls and multi-agent scenarios

    2. **Filtered span access** via get_llm_calls(), get_tool_calls(), etc.
       - Easy access to specific span types
       - Option to include/exclude nested spans

    3. **Aggregated metrics** via the metrics property
       - Token usage, latency, error counts

    Vocabulary: Trace.spans contains raw OTEL execution records (Span objects).
    """

    # Identity
    trace_id: str

    # Trace-level I/O
    input: str = ""
    output: str = ""

    # Sequential execution spans (raw spans, ordered by start_time)
    spans: List[Span] = field(default_factory=list)

    # Aggregated metrics
    metrics: TraceMetrics = field(default_factory=TraceMetrics)

    # Metadata
    timestamp: Optional[datetime] = None
    metadata: Dict[str, Any] = field(default_factory=dict)

    # ========================================================================
    # PRIMARY INTERFACE: Reconstructed conversation steps
    # ========================================================================

    def get_agent_steps(
        self, agent_span_id: Optional[str] = None, deduplicate_messages: bool = False
    ) -> List[AgentStep]:
        """
        Get reconstructed conversation steps for evaluation.

        Returns a logical conversation flow using typed steps:
        - UserStep: User input
        - LLMStep: LLM responses (with tool_calls if any, is_response=True for final)
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

        Args:
            spans: List of spans to reconstruct
            deduplicate_messages: If True, remove duplicate messages across LLM spans
        """
        steps: List[AgentStep] = []
        seen_messages: Optional[set] = set() if deduplicate_messages else None

        for span in spans:
            if isinstance(span, LLMSpan):
                steps.extend(self._reconstruct_llm_steps(span, seen_messages))
            elif isinstance(span, ToolSpan):
                steps.append(self._reconstruct_tool_step(span))
            elif isinstance(span, RetrieverSpan):
                steps.append(self._reconstruct_tool_step_from_retriever(span))
            elif isinstance(span, AgentSpan):
                # Agent spans are markers — system_prompt is metadata, not a step
                pass

        return steps

    def _reconstruct_llm_steps(self, llm_span: LLMSpan, seen_messages: Optional[set] = None) -> List[AgentStep]:
        """
        Reconstruct typed steps from an LLM span with optional deduplication.

        Args:
            llm_span: LLM span to reconstruct
            seen_messages: Set of message hashes for deduplication (or None to disable)
        """
        steps: List[AgentStep] = []

        # Build a lookup from tool_call_id -> tool name from assistant messages
        tool_call_names: Dict[str, str] = {}
        for msg in llm_span.messages:
            if isinstance(msg, AssistantMessage):
                for tc in msg.tool_calls:
                    tool_call_names[tc.id] = tc.name

        # Extract messages into typed steps
        for msg in llm_span.messages:
            # Deduplication logic
            if seen_messages is not None:
                msg_hash = _hash_message(msg)
                if msg_hash in seen_messages:
                    continue  # Skip duplicate
                seen_messages.add(msg_hash)

            if isinstance(msg, SystemMessage):
                # System messages are metadata, skip as steps
                # (stored in AgentTrace.system_prompt instead)
                pass
            elif isinstance(msg, UserMessage):
                steps.append(UserStep(content=msg.content))
            elif isinstance(msg, ToolMessage):
                # Tool result message in conversation
                resolved_name = tool_call_names.get(msg.tool_call_id or "", msg.tool_call_id or "")
                steps.append(
                    ToolExecutionStep(
                        tool_name=resolved_name,
                        tool_call_id=msg.tool_call_id,
                        content=msg.content,
                    )
                )

        # Add LLM response as LLMStep
        if llm_span.response or llm_span.tool_calls:
            tool_call_infos = [
                ToolCallInfo(id=tc.id, name=tc.name, arguments=tc.arguments) for tc in llm_span.tool_calls
            ]
            steps.append(
                LLMStep(
                    content=llm_span.response,
                    tool_calls=tool_call_infos,
                    llm_span_id=llm_span.span_id,
                )
            )

        return steps

    def _reconstruct_tool_step(self, tool_span: ToolSpan) -> ToolExecutionStep:
        """Reconstruct a tool step, including any nested LLM/tool calls."""
        # Find nested spans (children of this tool)
        nested_spans = self._get_children_of(tool_span.span_id)

        # Build nested traces
        nested_traces: List[Union[LLMSpan, AgentTrace]] = []
        for nested in nested_spans:
            if isinstance(nested, LLMSpan):
                nested_traces.append(nested)
            elif isinstance(nested, AgentSpan):
                try:
                    nested_traces.append(self.create_agent_trace(nested.span_id))
                except ValueError:
                    pass

        # Set error field
        error_info = None
        if tool_span.metrics.error:
            error_info = tool_span.metrics.error_message or tool_span.metrics.error_type or "Error"

        return ToolExecutionStep(
            tool_name=tool_span.name,
            tool_input=tool_span.arguments,
            tool_output=tool_span.result,
            content=str(tool_span.result) if tool_span.result is not None else "",
            nested_traces=nested_traces,
            duration_ms=tool_span.metrics.duration_ms,
            error=error_info,
        )

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

    def get_context(self) -> str:
        """
        Get combined retrieval context (for RAG evaluation).

        Returns:
            Combined context string from all retrievals.
        """
        contexts = []
        for retrieval in self.get_retrievals():
            for doc in retrieval.documents:
                if doc.content:
                    contexts.append(doc.content)
        return "\n\n".join(contexts)

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
            for msg in llm_span.messages:
                msg_hash = _hash_message(msg)
                if msg_hash not in seen_messages:
                    unique_messages.append(msg)
                    seen_messages.add(msg_hash)

            if unique_messages or llm_span.response or llm_span.tool_calls:
                new_span = dataclasses.replace(llm_span, messages=unique_messages)
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

    def get_agent_metrics(self, agent_span_id: str) -> Dict[str, Any]:
        """
        Get metrics for a specific agent.

        Args:
            agent_span_id: The agent span ID to get metrics for

        Returns:
            Dict with agent metrics
        """
        agent_spans = self._get_descendant_spans(agent_span_id)
        llm_calls = [s for s in agent_spans if isinstance(s, LLMSpan)]
        tool_calls = [s for s in agent_spans if isinstance(s, ToolSpan)]

        total_tokens = 0
        for llm in llm_calls:
            if llm.metrics and llm.metrics.token_usage:
                total_tokens += llm.metrics.token_usage.total_tokens

        total_duration = sum(s.metrics.duration_ms for s in agent_spans if hasattr(s, "metrics"))

        return {
            "agent_id": agent_span_id,
            "total_duration_ms": total_duration,
            "total_tokens": total_tokens,
            "llm_call_count": len(llm_calls),
            "tool_call_count": len(tool_calls),
        }

    def create_agent_trace(self, agent_span_id: str) -> AgentTrace:
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

        # Reconstructed typed steps (deduplicated) via existing method
        agent_steps = self.get_agent_steps(agent_span_id=agent_span_id, deduplicate_messages=True)

        # Calculate agent-level metrics from descendant spans
        descendant_spans = self._get_descendant_spans(agent_span_id)
        llm_spans = [s for s in descendant_spans if isinstance(s, LLMSpan)]
        tool_spans = [s for s in descendant_spans if isinstance(s, ToolSpan)]
        retriever_spans = [s for s in descendant_spans if isinstance(s, RetrieverSpan)]

        token_usage = TokenUsage()
        for llm in llm_spans:
            if llm.metrics and llm.metrics.token_usage:
                token_usage = token_usage + llm.metrics.token_usage

        agent_metrics = TraceMetrics(
            total_duration_ms=agent_span.metrics.duration_ms or 0,
            token_usage=token_usage,
            llm_call_count=len(llm_spans),
            tool_call_count=len(tool_spans),
            retrieval_count=len(retriever_spans),
            agent_span_count=0,
            total_span_count=len(descendant_spans),
            error_count=sum(1 for s in descendant_spans if getattr(getattr(s, "metrics", None), "error", False)),
        )

        return AgentTrace(
            agent_id=agent_span.span_id,
            agent_name=agent_span.name,
            framework=agent_span.framework,
            model=agent_span.model,
            system_prompt=agent_span.system_prompt,
            available_tools=list(agent_span.available_tools),
            input=agent_span.input,
            output=agent_span.output,
            steps=agent_steps,
            metrics=agent_metrics,
        )

    # ========================================================================
    # CONVENIENCE HELPER METHODS
    # ========================================================================

    def get_user_input(self) -> str:
        """
        Extract the initial user input from the trace.

        Returns the first user message from agent steps, or falls back to
        trace.input.
        """
        steps = self.get_agent_steps()
        for step in steps:
            if isinstance(step, UserStep):
                return step.content or ""
        return self.input or ""

    def get_final_response(self) -> str:
        """
        Extract the final assistant response.

        Returns the last LLMStep with is_response=True, or trace.output.
        """
        steps = self.get_agent_steps()
        for step in reversed(steps):
            if isinstance(step, LLMStep) and step.is_response:
                return step.content or ""
        return self.output or ""

    def get_conversation_turns(self, deduplicate_messages: bool = True) -> List[Tuple[str, str]]:
        """
        Extract conversation as (user, assistant) turn pairs.

        Args:
            deduplicate_messages: If True, removes duplicate messages (default: True)

        Returns:
            List of tuples: [(user_input_1, assistant_response_1), ...]
        """
        steps = self.get_agent_steps(deduplicate_messages=deduplicate_messages)
        turns = []
        current_user = None

        for step in steps:
            if isinstance(step, UserStep):
                current_user = step.content
            elif isinstance(step, LLMStep) and step.is_response and current_user:
                turns.append((current_user, step.content or ""))
                current_user = None

        return turns

    def get_tool_execution_sequence(self) -> List[Dict[str, Any]]:
        """
        Get tools executed in order with their inputs and outputs.

        Returns:
            List of dicts with tool execution info.
        """
        tool_calls = self.get_tool_calls()
        return [
            {
                "tool": t.name,
                "input": t.arguments,
                "output": t.result,
                "duration_ms": t.metrics.duration_ms,
                "error": t.metrics.error_message if t.metrics.error else None,
            }
            for t in tool_calls
        ]

    def get_tool_call_count(self, tool_name: Optional[str] = None) -> int:
        """
        Count tool invocations, optionally filtered by name.

        Args:
            tool_name: Optional tool name to filter by

        Returns:
            Number of tool calls
        """
        tools = self.get_tool_calls()
        if tool_name:
            return sum(1 for t in tools if t.name == tool_name)
        return len(tools)

    def get_retrieved_documents(self) -> List[Dict[str, Any]]:
        """
        Get all retrieved documents with metadata.

        Returns:
            List of dicts: [{"content": "...", "score": 0.95, "metadata": {...}}, ...]
        """
        retrievals = self.get_retrievals()
        docs = []
        for retrieval in retrievals:
            for doc in retrieval.documents:
                docs.append(
                    {
                        "content": doc.content,
                        "score": doc.score,
                        "metadata": doc.metadata,
                    }
                )
        return docs

    def get_execution_metrics(self) -> Dict[str, Any]:
        """
        Get consolidated metrics in one dict.

        Returns:
            Dict with key metrics.
        """
        return {
            "total_duration_ms": self.metrics.total_duration_ms,
            "total_tokens": self.metrics.token_usage.total_tokens,
            "llm_call_count": self.metrics.llm_call_count,
            "tool_call_count": self.metrics.tool_call_count,
            "error_count": self.metrics.error_count,
            "retrieval_count": self.metrics.retrieval_count,
        }

    def get_error_summary(self) -> Dict[str, Any]:
        """
        Get error summary.

        Returns:
            Dict: {"llm_errors": [...], "tool_errors": [...], "total": N}
        """
        llm_errors = [
            llm.metrics.error_message for llm in self.get_llm_calls() if llm.metrics.error and llm.metrics.error_message
        ]
        tool_errors = [
            tool.metrics.error_message
            for tool in self.get_tool_calls()
            if tool.metrics.error and tool.metrics.error_message
        ]
        return {
            "llm_errors": llm_errors,
            "tool_errors": tool_errors,
            "total": len(llm_errors) + len(tool_errors),
        }
