"""
Trace module for agent observability data.

This module provides data structures and parsing utilities for working
with agent execution traces in an evaluation context.

Public API:
    >>> from amp_evaluation.trace import (
    ...     Trajectory,                          # Complete agent execution path
    ...     LLMSpan, ToolSpan, RetrieverSpan, AgentSpan,  # Span types
    ...     TraceMetrics, TokenUsage,            # Metrics
    ...     Message, ToolCall, RetrievedDoc,     # Supporting types
    ...     parse_trace_for_evaluation,          # Parser
    ...     parse_traces_for_evaluation,
    ...     TraceFetcher,                        # Fetch traces from platform API
    ... )
"""

# Models
from .models import (
    # Core trace class
    Trajectory,
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
)

# Parser
from .parser import (
    parse_trace_for_evaluation,
    parse_traces_for_evaluation,
)

# Fetcher
from .fetcher import TraceFetcher


__all__ = [
    # Core trace
    "Trajectory",
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
    # Parser functions
    "parse_trace_for_evaluation",
    "parse_traces_for_evaluation",
    # Fetcher
    "TraceFetcher",
]
