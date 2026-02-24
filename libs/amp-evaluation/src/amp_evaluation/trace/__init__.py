"""
Trace module for agent observability data.

This module provides data structures and parsing utilities for working
with agent execution traces in an evaluation context.

Public API:
    >>> from amp_evaluation.trace import (
    ...     Trace,                               # Complete agent execution path
    ...     LLMSpan, ToolSpan, RetrieverSpan, AgentSpan,  # Span types
    ...     TraceMetrics, TokenUsage,            # Metrics
    ...     Message, ToolCall, RetrievedDoc,     # Supporting types
    ...     parse_trace_for_evaluation,          # Parser
    ...     parse_traces_for_evaluation,
    ...     TraceFetcher, TraceLoader,           # Fetch traces from platform or files
    ... )
"""

# Models
from .models import (
    # Core trace class
    Trace,
    # Agent-scoped trace for agent-level evaluation
    AgentTrace,
    # Span types
    LLMSpan,
    ToolSpan,
    RetrieverSpan,
    AgentSpan,
    # Metrics classes
    TraceMetrics,
    TokenUsage,
    # Supporting types
    Message,
    ToolCall,
    RetrievedDoc,
    # Typed messages
    SystemMessage,
    UserMessage,
    AssistantMessage,
    ToolMessage,
    # Reconstructed step types
    AgentStep,
    ToolCallInfo,
    # Typed steps
    UserStep,
    LLMStep,
    ToolExecutionStep,
)

# Parser
from .parser import (
    parse_trace_for_evaluation,
    parse_traces_for_evaluation,
)

# Fetcher
from .fetcher import TraceFetcher, TraceLoader


__all__ = [
    # Core trace
    "Trace",
    # Agent-scoped trace
    "AgentTrace",
    # Span types
    "LLMSpan",
    "ToolSpan",
    "RetrieverSpan",
    "AgentSpan",
    # Metrics
    "TraceMetrics",
    "TokenUsage",
    # Supporting types
    "Message",
    "ToolCall",
    "RetrievedDoc",
    # Typed messages
    "SystemMessage",
    "UserMessage",
    "AssistantMessage",
    "ToolMessage",
    # Reconstructed step types
    "AgentStep",
    "ToolCallInfo",
    # Typed steps
    "UserStep",
    "LLMStep",
    "ToolExecutionStep",
    # Parser functions
    "parse_trace_for_evaluation",
    "parse_traces_for_evaluation",
    # Fetchers
    "TraceFetcher",
    "TraceLoader",
]
