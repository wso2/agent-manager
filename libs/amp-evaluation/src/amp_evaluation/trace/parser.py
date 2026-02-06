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
Trace parsing utilities for converting OTEL/AMP traces to evaluation format.

This module provides functions to parse traces with OTEL/AMP Attributes
and convert them to the Trajectory format used by evaluators.

The parser accepts Trace objects from the fetcher (OTEL/AMP attribute model)
and converts them to Trajectory (evaluation-optimized model).
"""

from typing import Dict, Any, List, Optional
import logging

from .models import (
    Trajectory,
    TraceMetrics,
    TokenUsage,
    LLMSpan,
    ToolSpan,
    RetrieverSpan,
    AgentSpan,
    LLMMetrics,
    ToolMetrics,
    RetrieverMetrics,
    AgentMetrics,
    Message,
    ToolCall,
    RetrievedDoc,
)
from .fetcher import Trace as OTELTrace, Span as OTELSpan


logger = logging.getLogger(__name__)


# ============================================================================
# MAIN PARSING FUNCTION
# ============================================================================


def parse_trace_for_evaluation(trace: OTELTrace) -> Trajectory:
    """
    Parse an OTEL/AMP Trace model into Trajectory format for evaluation.

    This function:
    1. Extracts trace_id and top-level I/O from the Trace model
    2. Parses spans into typed collections (LLM, Tool, Retriever, Agent)
    3. Aggregates metrics (tokens, duration, counts)

    Args:
        trace: Trace object from fetcher (OTEL/AMP attribute model)

    Returns:
        Trajectory: Evaluation-optimized trace structure with metrics
    """
    # Extract trace-level info from Trace model
    trace_id = trace.traceId
    trace_input = trace.input if trace.input is not None else ""
    trace_output = trace.output if trace.output is not None else ""
    timestamp = trace.timestamp  # Uses the @property that parses startTime

    # Initialize containers
    llm_spans: List[LLMSpan] = []
    tool_spans: List[ToolSpan] = []
    retriever_spans: List[RetrieverSpan] = []
    agent_spans: List[AgentSpan] = []
    steps: List[Any] = []

    # Metrics accumulators
    token_usage = TokenUsage()
    total_duration_ms = trace.duration_ms
    error_count = trace.status.errorCount if trace.status else 0

    # Process each span from the Trace model
    for otel_span in sorted(trace.spans, key=lambda s: s.startTime or ""):
        # Get semantic kind from ampAttributes (top-level field in span)
        amp_attrs = otel_span.ampAttributes
        semantic_kind = amp_attrs.get("kind", "unknown")

        # Parse based on semantic kind
        if semantic_kind == "llm":
            llm = _parse_llm_span_from_otel(otel_span)
            if llm:
                llm_spans.append(llm)
                steps.append(llm)  # Add to steps in execution order
                if llm.metrics and llm.metrics.token_usage:
                    token_usage = token_usage + llm.metrics.token_usage

        elif semantic_kind == "tool":
            tool = _parse_tool_span_from_otel(otel_span)
            if tool:
                tool_spans.append(tool)
                steps.append(tool)  # Add to steps in execution order

        elif semantic_kind == "retriever":
            retriever = _parse_retriever_span_from_otel(otel_span)
            if retriever:
                retriever_spans.append(retriever)
                steps.append(retriever)  # Add to steps in execution order

        elif semantic_kind == "agent":
            agent = _parse_agent_span_from_otel(otel_span)
            if agent:
                agent_spans.append(agent)  # Keep last agent span
                steps.append(agent)  # Add to steps in execution order

        else:
            # For non-important spans (embedding, rerank, task, chain, etc.),
            # still count token usage if available
            data = amp_attrs.get("data", {})
            token_data = data.get("tokenUsage", {})
            if token_data:
                input_tokens = token_data.get("inputTokens", 0)
                output_tokens = token_data.get("outputTokens", 0)
                total = token_data.get("totalTokens", input_tokens + output_tokens)
                token_usage = token_usage + TokenUsage(
                    input_tokens=input_tokens, output_tokens=output_tokens, total_tokens=total
                )

    # Build trace metrics
    metrics = TraceMetrics(
        total_duration_ms=total_duration_ms,
        token_usage=token_usage,
        llm_call_count=len(llm_spans),
        tool_call_count=len(tool_spans),
        retrieval_count=len(retriever_spans),
        agent_span_count=len(agent_spans),
        total_span_count=trace.spanCount if trace.spanCount is not None else len(trace.spans),
        error_count=error_count,
    )

    # Create Trajectory
    return Trajectory(
        trace_id=trace_id, input=trace_input, output=trace_output, steps=steps, metrics=metrics, timestamp=timestamp
    )


def parse_traces_for_evaluation(traces: List[OTELTrace]) -> List[Trajectory]:
    """
    Parse multiple OTEL/AMP Trace models into Trajectory format.

    Args:
        traces: List of Trace objects from fetcher

    Returns:
        List of Trajectory objects
    """
    return [parse_trace_for_evaluation(t) for t in traces]


# ============================================================================
# HELPER FUNCTIONS TO CONVERT OTEL SPAN TO DICT
# ============================================================================


def _otel_span_to_dict(otel_span: OTELSpan) -> Dict[str, Any]:
    """
    Convert OTELSpan to dict format for existing parsing functions.
    This bridges the gap between the OTEL model and dict-based parsers.
    """
    amp_attrs = otel_span.ampAttributes

    return {
        "span_id": otel_span.spanId,
        "kind": amp_attrs.get("kind", "unknown"),
        "input": amp_attrs.get("input"),
        "output": amp_attrs.get("output"),
        "status": {
            "error": otel_span.status == "ERROR",
            "error_message": amp_attrs.get("error", {}).get("message") if otel_span.status == "ERROR" else None,
        },
        "data": amp_attrs.get("data", {}),
        "duration_ms": otel_span.duration_ms,
    }


def _parse_llm_span_from_otel(otel_span: OTELSpan) -> Optional[LLMSpan]:
    """Parse LLM span from OTEL model."""
    span_dict = _otel_span_to_dict(otel_span)
    return _parse_llm_span(span_dict)


def _parse_tool_span_from_otel(otel_span: OTELSpan) -> Optional[ToolSpan]:
    """Parse Tool span from OTEL model."""
    span_dict = _otel_span_to_dict(otel_span)
    return _parse_tool_span(span_dict)


def _parse_retriever_span_from_otel(otel_span: OTELSpan) -> Optional[RetrieverSpan]:
    """Parse Retriever span from OTEL model."""
    span_dict = _otel_span_to_dict(otel_span)
    return _parse_retriever_span(span_dict)


def _parse_agent_span_from_otel(otel_span: OTELSpan) -> Optional[AgentSpan]:
    """Parse Agent span from OTEL model."""
    span_dict = _otel_span_to_dict(otel_span)
    return _parse_agent_span(span_dict)


# ============================================================================
# SPAN PARSERS
# ============================================================================


def _parse_llm_span(raw_span: Dict[str, Any]) -> LLMSpan:
    """Parse an LLM span from normalized data."""
    span_id = raw_span.get("span_id", raw_span.get("id", "unknown"))
    data = raw_span.get("data", {})
    status = raw_span.get("status", {})

    # Parse messages from input
    messages = _parse_messages(raw_span.get("input"))

    # Parse response from output
    response = _parse_llm_response(raw_span.get("output"))

    # Parse tool calls from output
    tool_calls = _parse_tool_calls_from_output(raw_span.get("output"))

    # Parse token usage
    token_usage = _parse_token_usage(data)

    # Build metrics
    metrics = LLMMetrics(
        duration_ms=raw_span.get("duration_ms", 0.0),
        error=status.get("error", False),
        error_type=status.get("errorType"),
        error_message=status.get("error_message"),
        token_usage=token_usage,
    )

    return LLMSpan(
        span_id=span_id,
        messages=messages,
        response=response,
        tool_calls=tool_calls,
        model=data.get("model", ""),
        vendor=data.get("vendor", ""),
        temperature=data.get("temperature"),
        metrics=metrics,
    )


def _parse_tool_span(raw_span: Dict[str, Any]) -> ToolSpan:
    """Parse a tool execution span from normalized data."""
    span_id = raw_span.get("span_id", raw_span.get("id", "unknown"))
    data = raw_span.get("data", {})
    status = raw_span.get("status", {})

    # Tool name from data or span name
    name = data.get("name", raw_span.get("name", "unknown"))

    # Arguments from input
    arguments = {}
    raw_input = raw_span.get("input")
    if isinstance(raw_input, dict):
        arguments = raw_input
    elif isinstance(raw_input, str):
        arguments = {"input": raw_input}

    # Result from output
    result = raw_span.get("output", "")

    # Build metrics
    metrics = ToolMetrics(
        duration_ms=raw_span.get("duration_ms", 0.0),
        error=status.get("error", False),
        error_type=status.get("errorType"),
        error_message=status.get("error_message"),
    )

    return ToolSpan(span_id=span_id, name=name, arguments=arguments, result=result, metrics=metrics)


def _parse_retriever_span(raw_span: Dict[str, Any]) -> RetrieverSpan:
    """Parse a retriever span from normalized data."""
    span_id = raw_span.get("span_id", raw_span.get("id", "unknown"))
    data = raw_span.get("data", {})
    status = raw_span.get("status", {})

    # Query from input
    query = ""
    raw_input = raw_span.get("input")
    if isinstance(raw_input, str):
        query = raw_input
    elif isinstance(raw_input, dict):
        query = raw_input.get("query", str(raw_input))

    # Parse retrieved documents
    documents = _parse_retrieved_docs(raw_span.get("output"))

    # Build metrics
    metrics = RetrieverMetrics(
        duration_ms=raw_span.get("duration_ms", 0.0),
        error=status.get("error", False),
        error_type=status.get("errorType"),
        error_message=status.get("error_message"),
        documents_retrieved=len(documents),
    )

    return RetrieverSpan(
        span_id=span_id,
        query=query,
        documents=documents,
        vector_db=data.get("vectorDB", data.get("vector_db", "")),
        top_k=data.get("topK", data.get("top_k", 0)),
        metrics=metrics,
    )


def _parse_agent_span(raw_span: Dict[str, Any]) -> AgentSpan:
    """Parse an agent span from normalized data."""
    span_id = raw_span.get("span_id", raw_span.get("id", "unknown"))
    data = raw_span.get("data", {})
    status = raw_span.get("status", {})

    # Parse available tools
    tools = []
    raw_tools = data.get("tools", [])
    for tool in raw_tools:
        if isinstance(tool, dict):
            tools.append(tool.get("name", ""))
        elif isinstance(tool, str):
            tools.append(tool)

    # Parse token usage
    token_usage = _parse_token_usage(data)

    # Build metrics
    metrics = AgentMetrics(
        duration_ms=raw_span.get("duration_ms", 0.0),
        error=status.get("error", False),
        error_type=status.get("errorType"),
        error_message=status.get("error_message"),
        token_usage=token_usage,
    )

    # Parse input/output
    agent_input = ""
    agent_output = ""
    raw_input = raw_span.get("input")
    raw_output = raw_span.get("output")

    if isinstance(raw_input, str):
        agent_input = raw_input
    elif isinstance(raw_input, dict):
        agent_input = raw_input.get("input", str(raw_input))

    if isinstance(raw_output, str):
        agent_output = raw_output
    elif isinstance(raw_output, dict):
        agent_output = raw_output.get("output", str(raw_output))

    return AgentSpan(
        span_id=span_id,
        name=data.get("name", raw_span.get("name", "")),
        framework=data.get("framework", ""),
        model=data.get("model", ""),
        system_prompt=data.get("systemPrompt", data.get("system_prompt", "")),
        available_tools=tools,
        max_iterations=data.get("maxIter", data.get("max_iterations")),
        input=agent_input,
        output=agent_output,
        metrics=metrics,
    )


# ============================================================================
# HELPER PARSERS
# ============================================================================


def _parse_token_usage(data: Dict[str, Any]) -> TokenUsage:
    """Parse token usage from data dict."""
    token_data = data.get("tokenUsage", data.get("token_usage", {}))

    if not token_data:
        return TokenUsage()

    return TokenUsage(
        input_tokens=token_data.get("inputTokens", token_data.get("input_tokens", 0)),
        output_tokens=token_data.get("outputTokens", token_data.get("output_tokens", 0)),
        total_tokens=token_data.get("totalTokens", token_data.get("total_tokens", 0)),
        cache_read_tokens=token_data.get("cacheReadTokens", token_data.get("cache_read_tokens", 0)),
    )


def _parse_messages(raw_input: Any) -> List[Message]:
    """Parse messages from LLM input."""
    messages = []

    if not raw_input:
        return messages

    if isinstance(raw_input, list):
        for item in raw_input:
            if isinstance(item, dict):
                msg = Message(
                    role=item.get("role", "user"),
                    content=item.get("content", ""),
                    tool_calls=_parse_tool_calls(item.get("tool_calls", [])),
                )
                messages.append(msg)
    elif isinstance(raw_input, str):
        messages.append(Message(role="user", content=raw_input))

    return messages


def _parse_tool_calls(raw_tool_calls: List[Any]) -> List[ToolCall]:
    """Parse tool calls from message."""
    tool_calls = []

    for tc in raw_tool_calls:
        if isinstance(tc, dict):
            tool_calls.append(
                ToolCall(
                    id=tc.get("id", ""),
                    name=tc.get("name", tc.get("function", {}).get("name", "")),
                    arguments=tc.get("arguments", tc.get("function", {}).get("arguments", {})),
                )
            )

    return tool_calls


def _parse_tool_calls_from_output(raw_output: Any) -> List[ToolCall]:
    """Parse tool calls from LLM output (assistant response)."""
    tool_calls = []

    if isinstance(raw_output, list):
        for item in raw_output:
            if isinstance(item, dict) and item.get("tool_calls"):
                tool_calls.extend(_parse_tool_calls(item["tool_calls"]))
    elif isinstance(raw_output, dict) and raw_output.get("tool_calls"):
        tool_calls.extend(_parse_tool_calls(raw_output["tool_calls"]))

    return tool_calls


def _parse_llm_response(raw_output: Any) -> str:
    """Parse LLM response text from output."""
    if raw_output is None:
        return ""

    if isinstance(raw_output, str):
        return raw_output

    if isinstance(raw_output, dict):
        return raw_output.get("content", str(raw_output))

    if isinstance(raw_output, list):
        # Usually a list of message dicts
        for item in raw_output:
            if isinstance(item, dict):
                content = item.get("content", "")
                if content:
                    return content
        return ""

    return str(raw_output)


def _parse_retrieved_docs(raw_output: Any) -> List[RetrievedDoc]:
    """Parse retrieved documents from retriever output."""
    docs = []

    if not raw_output:
        return docs

    if isinstance(raw_output, list):
        for item in raw_output:
            if isinstance(item, dict):
                docs.append(
                    RetrievedDoc(
                        id=item.get("id", ""),
                        content=item.get("content", item.get("text", "")),
                        score=item.get("score", 0.0),
                        metadata=item.get("metadata", {}),
                    )
                )

    return docs
