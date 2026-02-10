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
1. Separate metrics from content - each span has its own metrics
2. Observable counts only - we track what we can reliably measure
3. Framework-agnostic - works with LangChain, CrewAI, OpenAI Agents, etc.
"""

from dataclasses import dataclass, field
from typing import List, Dict, Any, Optional
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

    # Convenience accessors for backwards compatibility
    @property
    def duration_ms(self) -> float:
        return self.metrics.duration_ms

    @property
    def error(self) -> bool:
        return self.metrics.error

    @property
    def token_usage(self) -> TokenUsage:
        return self.metrics.token_usage


@dataclass
class ToolSpan:
    """
    Represents a tool execution span.

    Content: Tool name, arguments, and result
    Metrics: Execution performance
    """

    # Identity
    span_id: str

    # Content
    name: str = ""
    arguments: Dict[str, Any] = field(default_factory=dict)
    result: Any = None

    # Metrics (separated)
    metrics: ToolMetrics = field(default_factory=ToolMetrics)

    @property
    def duration_ms(self) -> float:
        return self.metrics.duration_ms

    @property
    def error(self) -> bool:
        return self.metrics.error


@dataclass
class RetrieverSpan:
    """
    Represents a retrieval span (RAG).

    Content: Query and retrieved documents
    Metrics: Retrieval performance
    """

    # Identity
    span_id: str

    # Content
    query: str = ""
    documents: List[RetrievedDoc] = field(default_factory=list)

    # Configuration
    vector_db: str = ""
    top_k: int = 0

    # Metrics (separated)
    metrics: RetrieverMetrics = field(default_factory=RetrieverMetrics)

    @property
    def duration_ms(self) -> float:
        return self.metrics.duration_ms

    @property
    def error(self) -> bool:
        return self.metrics.error


@dataclass
class AgentSpan:
    """
    Represents an agent orchestration span.

    Content: Agent configuration and execution info
    Metrics: Agent-level performance
    """

    # Identity
    span_id: str

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

    @property
    def duration_ms(self) -> float:
        return self.metrics.duration_ms

    @property
    def error(self) -> bool:
        return self.metrics.error

    @property
    def token_usage(self) -> TokenUsage:
        return self.metrics.token_usage


# ============================================================================
# SPAN UNION TYPE
# ============================================================================

# Union type for any span in the sequence
Span = LLMSpan | ToolSpan | RetrieverSpan | AgentSpan


# ============================================================================
# MAIN TRAJECTORY DATACLASS
# ============================================================================


@dataclass
class Trajectory:
    """
    Evaluation-optimized trajectory representation.

    A trajectory is the complete execution path of an agent, preserving
    the temporal sequence of all operations (LLM calls, tool executions, etc.).

    This is the main data structure used by evaluators. It contains:
    1. Trace identity and I/O
    2. **Sequential** list of all spans (preserves execution order!)
    3. Aggregated metrics for the entire trace

    Design principles:
    - Framework-agnostic (works with any agent framework)
    - Evaluation-focused (easy access to what evaluators need)
    - Sequence-preserving (critical for reasoning about agent behavior)
    - Metrics-aware (separate metrics from content)

    Example sequence:
        steps[0]: LLMSpan (planning)
        steps[1]: ToolSpan (search)
        steps[2]: RetrieverSpan (RAG)
        steps[3]: LLMSpan (synthesis)
        steps[4]: ToolSpan (action)
    """

    # Identity
    trace_id: str

    # Trace-level I/O
    input: str = ""
    output: str = ""

    # Sequential execution steps (PRESERVES ORDER!)
    steps: List[Span] = field(default_factory=list)

    # Aggregated metrics
    metrics: TraceMetrics = field(default_factory=TraceMetrics)

    # Metadata
    timestamp: Optional[datetime] = None
    metadata: Dict[str, Any] = field(default_factory=dict)

    # ========================================================================
    # CONVENIENCE PROPERTIES
    # ========================================================================

    @property
    def has_output(self) -> bool:
        """Check if trace has non-empty output."""
        return bool(self.output and self.output.strip())

    @property
    def has_errors(self) -> bool:
        """Check if any spans had errors."""
        return self.metrics.has_errors

    @property
    def success(self) -> bool:
        """
        Check if the trace was successful (no errors).

        A trace is considered successful if it has no errors.
        This is derived from the error count in the trace status.
        """
        return not self.has_errors

    # ========================================================================
    # SPAN ACCESSORS (Filter by span type)
    # ========================================================================

    @property
    def llm_spans(self) -> List[LLMSpan]:
        """Get all LLM spans in execution order."""
        return [s for s in self.steps if isinstance(s, LLMSpan)]

    @property
    def tool_spans(self) -> List[ToolSpan]:
        """Get all tool spans in execution order."""
        return [s for s in self.steps if isinstance(s, ToolSpan)]

    @property
    def retriever_spans(self) -> List[RetrieverSpan]:
        """Get all retriever spans in execution order."""
        return [s for s in self.steps if isinstance(s, RetrieverSpan)]

    @property
    def agent_span(self) -> Optional[AgentSpan]:
        """Get first agent span (if any)."""
        agent_steps = [s for s in self.steps if isinstance(s, AgentSpan)]
        return agent_steps[0] if agent_steps else None

    # ========================================================================
    # CONVENIENT DATA ACCESS
    # ========================================================================

    @property
    def all_tool_names(self) -> List[str]:
        """Get list of all tools used in this trajectory (in order)."""
        return [t.name for t in self.tool_spans]

    @property
    def unique_tool_names(self) -> List[str]:
        """Get list of unique tools used (preserves first occurrence order)."""
        seen = set()
        unique = []
        for name in self.all_tool_names:
            if name not in seen:
                seen.add(name)
                unique.append(name)
        return unique

    @property
    def all_tool_results(self) -> List[Any]:
        """Get list of all tool results in execution order."""
        return [t.result for t in self.tool_spans]

    @property
    def all_llm_responses(self) -> List[str]:
        """Get list of all LLM responses in execution order."""
        return [llm.response for llm in self.llm_spans]

    @property
    def unique_models_used(self) -> List[str]:
        """Get list of unique models used."""
        models = set()
        for llm in self.llm_spans:
            if llm.model:
                models.add(llm.model)
        for agent_step in self.steps:
            if isinstance(agent_step, AgentSpan) and agent_step.model:
                models.add(agent_step.model)
        return list(models)

    @property
    def framework(self) -> str:
        """Get the agent framework used (if detected)."""
        for step in self.steps:
            if isinstance(step, AgentSpan):
                return step.framework
        return ""
