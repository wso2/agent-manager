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

The main entry point is the Trace class, which provides:
- get_agent_steps(): Reconstructed conversation flow for evaluators
- get_llm_calls(), get_tool_calls(), etc.: Filtered access to spans
- Aggregated metrics via the metrics property
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import List, Dict, Any, Optional, Tuple
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
    # Could add more agent-specific metrics later


@dataclass
class TraceMetrics:
    """
    Aggregated metrics for the entire trace.

    These are the observable counts we can reliably measure
    from any agent trace, regardless of framework.

    Note: We don't try to count "iterations" because:
    - Multi-agent systems have multiple agents
    - Agents can be chained in complex ways
    - Different frameworks structure things differently

    Instead, we track observable counts that are meaningful.
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
# MESSAGE AND TOOL CALL STRUCTURES
# ============================================================================


@dataclass
class ToolCall:
    """Represents a tool call made by an LLM."""

    id: str
    name: str
    arguments: Dict[str, Any] = field(default_factory=dict)


@dataclass
class Message:
    """Represents a message in a conversation."""

    role: str  # "system", "user", "assistant", "tool"
    content: str = ""
    tool_calls: List[ToolCall] = field(default_factory=list)
    tool_call_id: Optional[str] = None  # For tool response messages


@dataclass
class RetrievedDoc:
    """Represents a retrieved document from a vector store."""

    id: str = ""
    content: str = ""
    score: float = 0.0
    metadata: Dict[str, Any] = field(default_factory=dict)


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

    Content: Agent configuration and execution info
    Metrics: Agent-level performance
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
# AGENT STEP - Reconstructed conversation step for evaluators
# ============================================================================


@dataclass
class ToolCallInfo:
    """Info about a tool call request from an LLM."""

    id: str
    name: str
    arguments: Dict[str, Any] = field(default_factory=dict)


@dataclass
class AgentStep:
    """
    A single logical step in an agent's execution.

    This is the reconstructed conversation view that evaluators use.
    It represents what happened in a human-readable, evaluation-friendly format.

    Step types:
    - "system": System message/instructions
    - "user": User input
    - "assistant": LLM response (may include tool_calls)
    - "tool_result": Tool execution result (may include nested_steps)
    - "retrieval": Retrieved documents
    """

    step_type: str  # "system", "user", "assistant", "tool_result", "retrieval"
    content: str = ""

    # For assistant steps with tool calls
    tool_calls: List[ToolCallInfo] = field(default_factory=list)

    # For tool result steps (resolved from assistant tool_calls; falls back to tool_call_id)
    tool_name: Optional[str] = None
    tool_input: Optional[Dict[str, Any]] = None
    tool_output: Optional[Any] = None

    # For retrieval steps
    query: Optional[str] = None
    documents: List[RetrievedDoc] = field(default_factory=list)

    # Nested steps (for tools that call LLMs/agents)
    nested_steps: List["AgentStep"] = field(default_factory=list)

    # Metadata
    span_id: Optional[str] = None
    duration_ms: Optional[float] = None
    error: Optional[str] = None


# ============================================================================
# AGENT TRACE - Agent-scoped view for agent-level evaluation
# ============================================================================


@dataclass
class AgentTrace:
    """
    Agent-scoped view of a trace for agent-level evaluation.

    Contains the reconstructed conversation steps (deduplicated via AgentStep
    architecture), agent metadata, available tools, and agent-level metrics.

    Created via Trace.create_agent_trace(agent_span_id). Both class-based and
    function-based evaluators registered with level="agent" receive this object.

    Fields from AgentSpan:
        agent_id, agent_name, framework, model, system_prompt, available_tools, input, output

    steps: Reconstructed conversation via get_agent_steps(deduplicate_messages=True)
    metrics: Agent-level TraceMetrics (tokens, duration, span counts)
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

    # Reconstructed conversation steps (deduplicated)
    steps: List[AgentStep] = field(default_factory=list)

    # Agent-level metrics (same type as Trace.metrics)
    metrics: TraceMetrics = field(default_factory=TraceMetrics)


# ============================================================================
# HELPER FUNCTIONS
# ============================================================================


def _hash_message(msg: "Message") -> str:
    """
    Create hash of message for deduplication.

    Args:
        msg: Message object to hash

    Returns:
        SHA256 hash of message content
    """
    import hashlib

    # Hash by role and content (tool_calls excluded as they vary)
    content = f"{msg.role}:{msg.content or ''}"
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

    Design principles:
    - Framework-agnostic (works with any agent framework)
    - Evaluation-focused (clean interface for evaluators)
    - Hierarchy-aware (supports nested tool calls and multi-agent)
    - Metrics-aware (separate metrics from content)

    Example usage:
        # Get reconstructed conversation for evaluation
        steps = trace.get_agent_steps()
        for step in steps:
            if step.step_type == "assistant":
                check_hallucination(step.content)

        # Get all tool calls
        tools = trace.get_tool_calls()

        # Get combined retrieval context
        context = trace.get_context()
    """

    # Identity
    trace_id: str

    # Trace-level I/O
    input: str = ""
    output: str = ""

    # Sequential execution steps (raw spans, ordered by start_time)
    steps: List[Span] = field(default_factory=list)

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

        Returns a logical conversation flow:
        - System message (if available)
        - User input
        - Assistant responses (with tool_calls if any)
        - Tool results (with nested steps if tool called LLM/agent)
        - Final assistant answer

        Args:
            agent_span_id: Specific agent to get steps for (for multi-agent).
                          If None, returns steps for the entire trace.
            deduplicate_messages: If True, remove duplicate messages across
                                LLM spans (useful for multi-turn conversations).
                                Default: False

        Returns:
            List of AgentStep objects representing the conversation flow.

        Example:
            steps = trace.get_agent_steps()
            for step in steps:
                if step.step_type == "assistant":
                    # Evaluate LLM response
                    pass
                elif step.step_type == "tool_result":
                    # Check tool execution
                    if step.nested_steps:
                        # Tool called another LLM/agent
                        pass
        """
        # Get relevant spans
        if agent_span_id:
            spans = self._get_descendant_spans(agent_span_id)
        else:
            # Get root-level spans (no parent or parent is agent)
            spans = self._get_root_level_spans()

        return self._reconstruct_steps(spans, deduplicate_messages=deduplicate_messages)

    def _get_root_level_spans(self) -> List[Span]:
        """Get spans that are at the root level (not nested inside tools)."""
        # Find all tool span IDs
        tool_span_ids = {s.span_id for s in self.steps if isinstance(s, ToolSpan)}

        # Root spans are those whose parent is not a tool
        # (parent is None, or parent is an agent span)
        root_spans = []
        for span in self.steps:
            parent_id = getattr(span, "parent_span_id", None)
            if parent_id is None or parent_id not in tool_span_ids:
                root_spans.append(span)
        return root_spans

    def _get_descendant_spans(self, parent_id: str, _visited: Optional[set] = None) -> List[Span]:
        """Get all descendants of a span (recursive)."""
        if _visited is None:
            _visited = set()
        descendants = []
        for span in self.steps:
            if getattr(span, "parent_span_id", None) == parent_id and span.span_id not in _visited:
                _visited.add(span.span_id)
                descendants.append(span)
                descendants.extend(self._get_descendant_spans(span.span_id, _visited))
        return descendants

    def _get_children_of(self, parent_id: str) -> List[Span]:
        """Get direct children of a span."""
        return [s for s in self.steps if getattr(s, "parent_span_id", None) == parent_id]

    def _reconstruct_steps(self, spans: List[Span], deduplicate_messages: bool = False) -> List[AgentStep]:
        """
        Reconstruct logical conversation steps from spans.

        Args:
            spans: List of spans to reconstruct
            deduplicate_messages: If True, remove duplicate messages across LLM spans
        """
        steps: List[AgentStep] = []
        seen_messages: Optional[set[str]] = set() if deduplicate_messages else None

        for span in spans:
            if isinstance(span, LLMSpan):
                steps.extend(self._reconstruct_llm_steps(span, seen_messages))
            elif isinstance(span, ToolSpan):
                steps.append(self._reconstruct_tool_step(span))
            elif isinstance(span, RetrieverSpan):
                steps.append(self._reconstruct_retrieval_step(span))
            elif isinstance(span, AgentSpan):
                # Agent spans contribute system message if available
                if span.system_prompt:
                    steps.append(
                        AgentStep(
                            step_type="system",
                            content=span.system_prompt,
                            span_id=span.span_id,
                        )
                    )

        return steps

    def _reconstruct_llm_steps(self, llm_span: LLMSpan, seen_messages: Optional[set] = None) -> List[AgentStep]:
        """
        Reconstruct steps from an LLM span with optional deduplication.

        Args:
            llm_span: LLM span to reconstruct
            seen_messages: Set of message hashes for deduplication (or None to disable)
        """
        steps: List[AgentStep] = []

        # Build a lookup from tool_call_id -> tool name from assistant messages
        tool_call_names: Dict[str, str] = {}
        for msg in llm_span.messages:
            if msg.role == "assistant":
                for tc in msg.tool_calls:
                    tool_call_names[tc.id] = tc.name

        # Extract messages
        for msg in llm_span.messages:
            # Deduplication logic
            if seen_messages is not None:
                msg_hash = _hash_message(msg)
                if msg_hash in seen_messages:
                    continue  # Skip duplicate
                seen_messages.add(msg_hash)

            if msg.role == "system":
                steps.append(
                    AgentStep(
                        step_type="system",
                        content=msg.content,
                        span_id=llm_span.span_id,
                    )
                )
            elif msg.role == "user":
                steps.append(
                    AgentStep(
                        step_type="user",
                        content=msg.content,
                        span_id=llm_span.span_id,
                    )
                )
            elif msg.role == "tool":
                # Tool result message in conversation
                # Resolve human-readable tool name from prior assistant tool_calls;
                # falls back to the opaque tool_call_id if no match is found.
                resolved_name = tool_call_names.get(msg.tool_call_id or "", msg.tool_call_id or "")
                steps.append(
                    AgentStep(
                        step_type="tool_result",
                        content=msg.content,
                        tool_name=resolved_name,
                        span_id=llm_span.span_id,
                    )
                )

        # Add assistant response
        if llm_span.response or llm_span.tool_calls:
            tool_call_infos = [
                ToolCallInfo(id=tc.id, name=tc.name, arguments=tc.arguments) for tc in llm_span.tool_calls
            ]
            steps.append(
                AgentStep(
                    step_type="assistant",
                    content=llm_span.response,
                    tool_calls=tool_call_infos,
                    span_id=llm_span.span_id,
                    duration_ms=llm_span.metrics.duration_ms,
                    error=llm_span.metrics.error_message if llm_span.metrics.error else None,
                )
            )

        return steps

    def _reconstruct_tool_step(self, tool_span: ToolSpan) -> AgentStep:
        """Reconstruct a tool step, including any nested LLM/tool calls."""
        # Find nested spans (children of this tool)
        nested_spans = self._get_children_of(tool_span.span_id)

        # Recursively reconstruct nested steps
        nested_steps = self._reconstruct_steps(nested_spans)

        # Set error field: prefer error_message, fallback to error_type, or just the error flag
        error_info = None
        if tool_span.metrics.error:
            error_info = tool_span.metrics.error_message or tool_span.metrics.error_type or "Error"

        return AgentStep(
            step_type="tool_result",
            tool_name=tool_span.name,
            tool_input=tool_span.arguments,
            tool_output=tool_span.result,
            nested_steps=nested_steps,
            span_id=tool_span.span_id,
            duration_ms=tool_span.metrics.duration_ms,
            error=error_info,
        )

    def _reconstruct_retrieval_step(self, retriever_span: RetrieverSpan) -> AgentStep:
        """Reconstruct a retrieval step."""
        return AgentStep(
            step_type="retrieval",
            query=retriever_span.query,
            documents=retriever_span.documents,
            span_id=retriever_span.span_id,
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
            deduplicate_messages: If True, remove duplicate messages across LLM calls
                                (HIGH PRIORITY - removes repeated system messages).

        Returns:
            List of LLMSpan objects.

        Example:
            >>> # Get all LLM calls with deduplication
            >>> llms = trace.get_llm_calls(deduplicate_messages=True)
            >>> # Get LLM calls for specific agent
            >>> agent = trace.get_agents()[0]
            >>> agent_llms = trace.get_llm_calls(agent_span_id=agent.span_id)
        """
        # Start with all or root-level LLM spans
        if include_nested:
            llms = [s for s in self.steps if isinstance(s, LLMSpan)]
        else:
            # Exclude nested (those whose parent is a tool)
            tool_span_ids = {s.span_id for s in self.steps if isinstance(s, ToolSpan)}
            llms = [
                s
                for s in self.steps
                if isinstance(s, LLMSpan) and getattr(s, "parent_span_id", None) not in tool_span_ids
            ]

        # Filter by agent if specified
        if agent_span_id:
            llms = [llm for llm in llms if self._is_descendant_of(llm, agent_span_id)]

        # Deduplicate messages if requested (HIGH PRIORITY)
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

        Example:
            >>> # Get tool calls for specific agent
            >>> agent = trace.get_agents()[0]
            >>> agent_tools = trace.get_tool_calls(agent_span_id=agent.span_id)
        """
        if include_nested:
            tools = [s for s in self.steps if isinstance(s, ToolSpan)]
        else:
            # Exclude nested (those whose parent is a tool)
            tool_span_ids = {s.span_id for s in self.steps if isinstance(s, ToolSpan)}
            tools = [
                s
                for s in self.steps
                if isinstance(s, ToolSpan) and getattr(s, "parent_span_id", None) not in tool_span_ids
            ]

        # Filter by agent if specified
        if agent_span_id:
            tools = [tool for tool in tools if self._is_descendant_of(tool, agent_span_id)]

        return tools

    def get_retrievals(self, agent_span_id: Optional[str] = None) -> List[RetrieverSpan]:
        """
        Get all retrieval operations with agent filtering.

        Args:
            agent_span_id: If provided, only return retrievals that are descendants
                          of this agent span (for multi-agent filtering).

        Returns:
            List of RetrieverSpan objects.

        Example:
            >>> # Get retrievals for specific agent
            >>> agent = trace.get_agents()[0]
            >>> agent_retrievals = trace.get_retrievals(agent_span_id=agent.span_id)
        """
        retrievals = [s for s in self.steps if isinstance(s, RetrieverSpan)]

        # Filter by agent if specified
        if agent_span_id:
            retrievals = [r for r in retrievals if self._is_descendant_of(r, agent_span_id)]

        return retrievals

    def get_agents(self) -> List[AgentSpan]:
        """
        Get all agent spans (for multi-agent systems).

        Returns:
            List of AgentSpan objects.
        """
        return [s for s in self.steps if isinstance(s, AgentSpan)]

    def get_context(self) -> str:
        """
        Get combined retrieval context (for RAG evaluation).

        Concatenates all retrieved documents into a single string.

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

        This is critical for accurate per-LLM-call evaluation, as repeated system
        messages shouldn't inflate evaluation metrics.

        Args:
            llm_spans: List of LLM spans to deduplicate

        Returns:
            List of LLMSpan objects with unique messages only

        Example:
            >>> llm_calls = trace.get_llm_calls()
            >>> deduped = trace._deduplicate_llm_messages(llm_calls)
            >>> # System messages only appear in first LLM call
        """
        import dataclasses

        seen_messages = set()
        deduplicated = []

        for llm_span in llm_spans:
            # Create new span with deduplicated messages
            unique_messages = []
            for msg in llm_span.messages:
                msg_hash = _hash_message(msg)
                if msg_hash not in seen_messages:
                    unique_messages.append(msg)
                    seen_messages.add(msg_hash)

            # Only include span if it has content (messages, response, or tool_calls)
            if unique_messages or llm_span.response or llm_span.tool_calls:
                # Create new LLMSpan with deduplicated messages, preserving all other data
                new_span = dataclasses.replace(llm_span, messages=unique_messages)
                deduplicated.append(new_span)

        return deduplicated

    def _is_descendant_of(self, span: Span, ancestor_span_id: str) -> bool:
        """
        Check if span is a descendant of ancestor by walking parent chain.

        Args:
            span: The span to check
            ancestor_span_id: The potential ancestor's span ID

        Returns:
            True if span is a descendant of ancestor, False otherwise

        Example:
            >>> tool_span = trace.steps[5]
            >>> agent_span = trace.get_agents()[0]
            >>> is_child = trace._is_descendant_of(tool_span, agent_span.span_id)
        """
        current_id = getattr(span, "parent_span_id", None)
        visited = set()  # Prevent infinite loops

        while current_id:
            if current_id in visited:
                return False  # Cycle detected
            visited.add(current_id)

            if current_id == ancestor_span_id:
                return True

            # Find parent span
            parent_span = next((s for s in self.steps if hasattr(s, "span_id") and s.span_id == current_id), None)
            if not parent_span:
                break

            current_id = getattr(parent_span, "parent_span_id", None)

        return False

    def get_agent_metrics(self, agent_span_id: str) -> Dict[str, Any]:
        """
        Get metrics for a specific agent.

        Calculates agent-specific metrics by aggregating metrics from all spans
        that are descendants of the agent span.

        Args:
            agent_span_id: The agent span ID to get metrics for

        Returns:
            Dict with keys:
                - agent_id: str
                - total_duration_ms: float
                - total_tokens: int
                - llm_call_count: int
                - tool_call_count: int

        Example:
            >>> agent = trace.get_agents()[0]
            >>> metrics = trace.get_agent_metrics(agent.span_id)
            >>> print(f"Agent used {metrics['total_tokens']} tokens")
        """
        # Get all descendant spans
        agent_steps = self._get_descendant_spans(agent_span_id)

        # Calculate agent-specific metrics
        llm_calls = [s for s in agent_steps if isinstance(s, LLMSpan)]
        tool_calls = [s for s in agent_steps if isinstance(s, ToolSpan)]

        total_tokens = 0
        for llm in llm_calls:
            if llm.metrics and llm.metrics.token_usage:
                total_tokens += llm.metrics.token_usage.total_tokens

        total_duration = sum(s.metrics.duration_ms for s in agent_steps if hasattr(s, "metrics"))

        return {
            "agent_id": agent_span_id,
            "total_duration_ms": total_duration,
            "total_tokens": total_tokens,
            "llm_call_count": len(llm_calls),
            "tool_call_count": len(tool_calls),
        }

    def create_agent_trace(self, agent_span_id: str) -> "AgentTrace":
        """
        Create an AgentTrace scoped to a specific agent's execution.

        Gathers agent metadata from AgentSpan, reconstructs conversation steps
        with deduplication, and calculates agent-level metrics from descendant spans.

        Args:
            agent_span_id: The span_id of the AgentSpan to create a trace for

        Returns:
            AgentTrace with steps, metadata, and metrics for this agent

        Raises:
            ValueError: If agent_span_id not found in trace steps
        """
        agent_span = next(
            (s for s in self.steps if isinstance(s, AgentSpan) and s.span_id == agent_span_id),
            None,
        )
        if agent_span is None:
            raise ValueError(f"Agent span '{agent_span_id}' not found in trace '{self.trace_id}'")

        # Reconstructed steps (deduplicated) via existing method
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
    # TODO how to handle multi-agent scenarios? Maybe allow passing agent_span_id to filter steps by agent?
    def get_user_input(self) -> str:
        """
        Extract the initial user input from the trace.

        Returns the first user message from agent steps, or falls back to
        trace.input.

        Returns:
            The initial user query as a string.

        Example:
            >>> query = trace.get_user_input()
            >>> print(query)
            "Hi im looking to travel with my family to Spain..."
        """
        steps = self.get_agent_steps()
        for step in steps:
            if step.step_type == "user":
                return step.content or ""
        return self.input or ""

    # TODO: How to handle multi-agent scenarios? Maybe allow passing agent_span_id to filter steps by agent?
    def get_final_response(self) -> str:
        """
        Extract the final assistant response.

        Returns the last assistant message or trace.output.

        Returns:
            The final response as a string.

        Example:
            >>> response = trace.get_final_response()
            >>> print(response)
            "Here are some fantastic family-friendly destinations..."
        """
        steps = self.get_agent_steps()
        for step in reversed(steps):
            if step.step_type == "assistant":
                return step.content or ""
        return self.output or ""

    def get_conversation_turns(self, deduplicate_messages: bool = True) -> List[Tuple[str, str]]:
        """
        Extract conversation as (user, assistant) turn pairs.

        Automatically handles message deduplication across LLM spans.

        Args:
            deduplicate_messages: If True, removes duplicate messages (default: True)

        Returns:
            List of tuples: [(user_input_1, assistant_response_1), ...]

        Example:
            >>> turns = trace.get_conversation_turns()
            >>> for user_msg, asst_msg in turns:
            ...     evaluate_turn(user_msg, asst_msg)
        """
        steps = self.get_agent_steps(deduplicate_messages=deduplicate_messages)
        turns = []
        current_user = None

        for step in steps:
            if step.step_type == "user":
                current_user = step.content
            elif step.step_type == "assistant" and current_user:
                turns.append((current_user, step.content or ""))
                current_user = None

        return turns

    def get_tool_execution_sequence(self) -> List[Dict[str, Any]]:
        """
        Get tools executed in order with their inputs and outputs.

        Returns:
            List of dicts with structure:
            [
                {
                    "tool": "search_hotels",
                    "input": {"query": "..."},
                    "output": "...",
                    "duration_ms": 150.0,
                    "error": None,
                },
                ...
            ]

        Example:
            >>> sequence = trace.get_tool_execution_sequence()
            >>> tool_names = [t["tool"] for t in sequence]
            >>> assert "search_hotels" in tool_names
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

        Example:
            >>> total_tools = trace.get_tool_call_count()
            >>> hotel_searches = trace.get_tool_call_count("search_hotels")
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

        Example:
            >>> docs = trace.get_retrieved_documents()
            >>> for doc in docs:
            ...     print(f"Score: {doc['score']}, Content: {doc['content'][:50]}...")
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
            Dict with keys:
            {
                "total_duration_ms": 1500.0,
                "total_tokens": 5000,
                "llm_call_count": 2,
                "tool_call_count": 5,
                "error_count": 0,
                "retrieval_count": 0,
            }

        Example:
            >>> metrics = trace.get_execution_metrics()
            >>> print(f"Total tokens: {metrics['total_tokens']}")
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

        Example:
            >>> errors = trace.get_error_summary()
            >>> if errors["total"] > 0:
            ...     print(f"Found {errors['total']} errors")
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
