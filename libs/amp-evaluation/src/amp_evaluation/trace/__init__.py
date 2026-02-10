"""
Trace module for agent observability data.

This module provides data structures and parsing utilities for working
with agent execution traces in an evaluation context.

Key Components:
- Trajectory: The main trace container for evaluation
- Span Types: LLMSpan, ToolSpan, RetrieverSpan, AgentSpan
- Metrics: TraceMetrics, SpanMetrics, TokenUsage
- Parser: parse_trace_for_evaluation()

Example Usage:
    from amp_eval.trace import Trajectory, parse_trace_for_evaluation

    # Parse a raw OTEL trace
    raw_trace = {...}  # From trace service
    trajectory = parse_trace_for_evaluation(raw_trace)

    # Access spans
    for llm in trajectory.llm_spans:
        print(f"Model: {llm.model}, Tokens: {llm.metrics.token_usage.total_tokens}")

    # Access aggregated metrics
    print(f"Total LLM calls: {trajectory.metrics.llm_call_count}")
    print(f"Total tokens: {trajectory.metrics.total_token_usage.total_tokens}")
"""

# Models
from .models import (
    # Core trace class
    Trajectory,  # The main trace class with sequential steps
    # Span types
    LLMSpan,
    ToolSpan,
    RetrieverSpan,
    AgentSpan,
    # Metrics classes
    TraceMetrics,
    SpanMetrics,
    LLMMetrics,
    ToolMetrics,
    RetrieverMetrics,
    AgentMetrics,
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
from .fetcher import (
    # OTEL/AMP attribute models (raw trace models from API)
    Trace as OTELTrace,
    Span as OTELSpan,
    TokenUsage as OTELTokenUsage,
    TraceStatus as OTELTraceStatus,
    # Fetcher classes
    TraceFetcher,
    TraceFetchConfig,
    TraceLoader,
)


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
    "SpanMetrics",
    "LLMMetrics",
    "ToolMetrics",
    "RetrieverMetrics",
    "AgentMetrics",
    "TokenUsage",
    # Supporting types
    "Message",
    "ToolCall",
    "RetrievedDoc",
    # Parser functions
    "parse_trace_for_evaluation",
    "parse_traces_for_evaluation",
    # OTEL/AMP attribute models (raw trace models from API)
    "OTELTrace",
    "OTELSpan",
    "OTELTokenUsage",
    "OTELTraceStatus",
    # Fetcher
    "TraceFetcher",
    "TraceFetchConfig",
    "TraceLoader",
]
