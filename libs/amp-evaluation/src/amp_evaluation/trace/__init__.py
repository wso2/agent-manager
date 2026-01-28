"""
Trace module for agent observability data.

This module provides data structures and parsing utilities for working
with agent execution traces in an evaluation context.

Key Components:
- EvalTrace: The main trace container for evaluation
- Span Types: LLMSpan, ToolSpan, RetrieverSpan, AgentSpan
- Metrics: TraceMetrics, SpanMetrics, TokenUsage
- Parser: parse_trace_for_evaluation()

Example Usage:
    from amp_eval.trace import EvalTrace, parse_trace_for_evaluation
    
    # Parse a raw OTEL trace
    raw_trace = {...}  # From trace service
    eval_trace = parse_trace_for_evaluation(raw_trace)
    
    # Access spans
    for llm in eval_trace.llm_spans:
        print(f"Model: {llm.model}, Tokens: {llm.metrics.token_usage.total_tokens}")
    
    # Access aggregated metrics
    print(f"Total LLM calls: {eval_trace.metrics.llm_call_count}")
    print(f"Total tokens: {eval_trace.metrics.total_token_usage.total_tokens}")
"""

# Models
from .models import (
    # Core trace class
    EvalTrace,
    
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
    'EvalTrace',
    
    # Span types
    'LLMSpan',
    'ToolSpan', 
    'RetrieverSpan',
    'AgentSpan',
    
    # Metrics
    'TraceMetrics',
    'SpanMetrics',
    'LLMMetrics',
    'ToolMetrics',
    'RetrieverMetrics',
    'AgentMetrics',
    'TokenUsage',
    
    # Supporting types
    'Message',
    'ToolCall',
    'RetrievedDoc',
    
    # Parser functions
    'parse_trace_for_evaluation',
    'parse_traces_for_evaluation',
    
    # OTEL/AMP attribute models (raw trace models from API)
    'OTELTrace',
    'OTELSpan',
    'OTELTokenUsage',
    'OTELTraceStatus',
    
    # Fetcher
    'TraceFetcher',
    'TraceFetchConfig',
    'TraceLoader',
]
