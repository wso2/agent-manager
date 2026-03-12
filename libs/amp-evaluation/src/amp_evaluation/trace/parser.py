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
and convert them to the Trace format used by evaluators.

The parser accepts Trace objects from the fetcher (OTEL/AMP attribute model)
and converts them to Trace (evaluation-optimized model).
"""

import json
from collections import defaultdict
from dataclasses import replace as dataclass_replace
from typing import Dict, Any, List, Optional, Set
import logging

from .models import (
    Trace,
    TraceMetrics,
    TokenUsage,
    LLMSpan,
    ToolSpan,
    RetrieverSpan,
    AgentSpan,
    ChainSpan,
    LLMMetrics,
    ToolMetrics,
    RetrieverMetrics,
    AgentMetrics,
    SystemMessage,
    UserMessage,
    AssistantMessage,
    ToolMessage,
    ToolCall,
    RetrievedDoc,
)
from .fetcher import OTELTrace, OTELSpan, _parse_timestamp


logger = logging.getLogger(__name__)


# ============================================================================
# SPAN FILTERING UTILITIES
# ============================================================================


# Infrastructure span kinds that add no semantic value for evaluation
INFRASTRUCTURE_KINDS = {"chain", "unknown", "task", "crewaitask"}

# Semantic span kinds that should be kept for evaluation
SEMANTIC_KINDS = {"llm", "tool", "agent", "retriever", "embedding"}


def filter_infrastructure_spans(spans: List[OTELSpan]) -> List[OTELSpan]:
    """
    Filter infrastructure spans while preserving trace tree structure.

    Removes pass-through infrastructure spans (chain, unknown, task, crewaitask)
    but keeps **bridge** infrastructure spans that are needed to maintain the
    tree hierarchy:
      - Root spans (no parent) are always kept.
      - Infrastructure spans with 2+ child branches that each lead to at
        least one semantic descendant are kept as structural bridges.

    Pass-through infrastructure spans (single child path) are removed and their
    children are remapped to the nearest kept ancestor.

    Args:
        spans: List of OTEL spans to filter

    Returns:
        Filtered list of OTEL spans with remapped parent references
    """
    if not spans:
        return spans

    # Phase 1: Build indices
    spans_by_id: Dict[str, OTELSpan] = {s.spanId: s for s in spans}
    children_map: Dict[str, List[str]] = defaultdict(list)
    for s in spans:
        if s.parentSpanId:
            children_map[s.parentSpanId].append(s.spanId)

    # Phase 2: Compute which subtrees contain semantic spans (memoized)
    _semantic_cache: Dict[str, bool] = {}

    def has_semantic_descendant(span_id: str) -> bool:
        if span_id in _semantic_cache:
            return _semantic_cache[span_id]
        span = spans_by_id.get(span_id)
        if not span:
            _semantic_cache[span_id] = False
            return False
        if span.ampAttributes.kind in SEMANTIC_KINDS:
            _semantic_cache[span_id] = True
            return True
        result = any(has_semantic_descendant(cid) for cid in children_map.get(span_id, []))
        _semantic_cache[span_id] = result
        return result

    # Phase 3: Identify bridge infrastructure spans
    bridge_ids: Set[str] = set()
    for span in spans:
        if span.ampAttributes.kind not in INFRASTRUCTURE_KINDS:
            continue
        # Root is always a bridge (if it has semantic descendants)
        if span.parentSpanId is None:
            if has_semantic_descendant(span.spanId):
                bridge_ids.add(span.spanId)
            continue
        # Branching rule: 2+ child branches with semantic descendants
        semantic_branches = sum(1 for cid in children_map.get(span.spanId, []) if has_semantic_descendant(cid))
        if semantic_branches >= 2:
            bridge_ids.add(span.spanId)

    # Phase 4: Build keep set (semantic + bridge spans)
    keep_ids: Set[str] = {s.spanId for s in spans if s.ampAttributes.kind in SEMANTIC_KINDS} | bridge_ids

    # Phase 5: Remap parents of kept spans whose parent was removed
    def _find_kept_ancestor(span_id: str) -> Optional[str]:
        """Walk up from span (exclusive) to find nearest kept ancestor."""
        visited: Set[str] = set()
        current = spans_by_id.get(span_id)
        if not current:
            return None
        current_id = current.parentSpanId
        while current_id and current_id not in visited:
            visited.add(current_id)
            if current_id in keep_ids:
                return current_id
            parent = spans_by_id.get(current_id)
            if not parent:
                return None
            current_id = parent.parentSpanId
        return None

    filtered_spans: List[OTELSpan] = []
    for span in spans:
        if span.spanId not in keep_ids:
            continue
        parent_id = span.parentSpanId
        if parent_id and parent_id not in keep_ids:
            # Parent was removed — remap to nearest kept ancestor
            parent_id = _find_kept_ancestor(span.spanId)
        filtered_spans.append(dataclass_replace(span, parentSpanId=parent_id))

    # Phase 6: Validate
    _validate_trace_structure(filtered_spans)

    return filtered_spans


def _validate_trace_structure(spans: List[OTELSpan]) -> None:
    """
    Validate trace has single root, no cycles, all reachable.

    Args:
        spans: List of spans to validate

    Raises:
        ValueError: If trace structure is invalid
    """
    if not spans:
        return

    span_ids = {s.spanId for s in spans}
    roots = [s for s in spans if s.parentSpanId is None]

    if len(roots) != 1:
        logger.warning(f"Expected 1 root span, got {len(roots)}")
        # Don't raise error, just warn - some traces may have multiple roots

    # Verify all parent IDs exist
    for span in spans:
        parent_id = span.parentSpanId
        if parent_id and parent_id not in span_ids:
            raise ValueError(f"Span {span.spanId} has invalid parent {parent_id}")


# ============================================================================
# MAIN PARSING FUNCTION
# ============================================================================


def parse_trace_for_evaluation(trace: OTELTrace, filter_infrastructure: bool = True) -> Trace:
    """
    Parse an OTEL/AMP Trace model into Trace format for evaluation.

    This function:
    1. Extracts trace_id and top-level I/O from the Trace model
    2. Optionally filters infrastructure spans (chain, unknown, task, crewaitask)
    3. Parses spans into typed collections (LLM, Tool, Retriever, Agent)
    4. Aggregates metrics (tokens, duration, counts)

    Args:
        trace: Trace object from fetcher (OTEL/AMP attribute model)
        filter_infrastructure: If True, removes infrastructure spans (default: True)

    Returns:
        Trace: Evaluation-optimized trace structure with metrics
    """
    # Extract trace-level info from Trace model
    trace_id = trace.traceId
    trace_input = trace.input if trace.input is not None else ""
    trace_output = trace.output if trace.output is not None else ""
    timestamp = trace.timestamp  # Uses the @property that parses startTime

    # Filter infrastructure spans if requested
    spans_to_process = trace.spans
    if filter_infrastructure:
        try:
            spans_to_process = filter_infrastructure_spans(trace.spans)
            logger.debug(f"Filtered spans from {len(trace.spans)} to {len(spans_to_process)}")
        except Exception as e:
            logger.warning(f"Failed to filter infrastructure spans: {e}. Using all spans.")
            spans_to_process = trace.spans

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
    for otel_span in sorted(spans_to_process, key=lambda s: s.startTime or ""):
        # Get semantic kind from typed AmpAttributes
        semantic_kind = otel_span.ampAttributes.kind

        # Parse based on semantic kind
        if semantic_kind == "llm":
            llm = _parse_llm_span(otel_span)
            if llm:
                llm_spans.append(llm)
                steps.append(llm)  # Add to steps in execution order
                if llm.metrics and llm.metrics.token_usage:
                    token_usage = token_usage + llm.metrics.token_usage

        elif semantic_kind == "tool":
            tool = _parse_tool_span(otel_span)
            if tool:
                tool_spans.append(tool)
                steps.append(tool)  # Add to steps in execution order

        elif semantic_kind == "retriever":
            retriever = _parse_retriever_span(otel_span)
            if retriever:
                retriever_spans.append(retriever)
                steps.append(retriever)  # Add to steps in execution order

        elif semantic_kind == "agent":
            agent = _parse_agent_span(otel_span)
            if agent:
                agent_spans.append(agent)  # Keep last agent span
                steps.append(agent)  # Add to steps in execution order

        else:
            # Bridge infrastructure spans kept by the filter for tree structure
            if otel_span.ampAttributes.kind in INFRASTRUCTURE_KINDS:
                chain = ChainSpan(
                    span_id=otel_span.spanId,
                    parent_span_id=otel_span.parentSpanId,
                    start_time=_parse_timestamp(otel_span.startTime) if otel_span.startTime else None,
                    name=otel_span.name or "",
                )
                steps.append(chain)

            # Still count token usage if available
            tu = otel_span.ampAttributes.data.token_usage
            if tu:
                token_usage = token_usage + TokenUsage(
                    input_tokens=tu.inputTokens,
                    output_tokens=tu.outputTokens,
                    total_tokens=tu.totalTokens,
                )

    # Build trace metrics
    metrics = TraceMetrics(
        total_duration_ms=total_duration_ms,
        token_usage=token_usage,
        error_count=error_count,
    )

    # Create Trace
    return Trace(
        trace_id=trace_id, input=trace_input, output=trace_output, spans=steps, metrics=metrics, timestamp=timestamp
    )


def parse_traces_for_evaluation(traces: List[OTELTrace]) -> List[Trace]:
    """
    Parse multiple OTEL/AMP Trace models into Trace format.

    Args:
        traces: List of Trace objects from fetcher

    Returns:
        List of Trajectory objects
    """
    return [parse_trace_for_evaluation(t) for t in traces]


# ============================================================================
# SPAN PARSERS
# ============================================================================


def _parse_llm_span(otel_span: OTELSpan) -> LLMSpan:
    """Parse an LLM span directly from a typed OTELSpan."""
    amp = otel_span.ampAttributes
    data = amp.data
    st = amp.status

    # Parse messages from input
    messages = _parse_messages(amp.input)

    # Parse response from output
    response = _parse_llm_response(amp.output)

    # Parse tool calls from output
    tool_calls = _parse_tool_calls_from_output(amp.output)

    # Token usage (already typed in AmpSpanData)
    tu = data.token_usage
    token_usage = (
        TokenUsage(
            input_tokens=tu.inputTokens,
            output_tokens=tu.outputTokens,
            total_tokens=tu.totalTokens,
        )
        if tu
        else TokenUsage()
    )

    metrics = LLMMetrics(
        duration_ms=otel_span.duration_ms,
        error=st.error,
        error_type=st.error_type,
        error_message=st.error_message,
        token_usage=token_usage,
    )

    return LLMSpan(
        span_id=otel_span.spanId,
        parent_span_id=otel_span.parentSpanId,
        start_time=_parse_timestamp(otel_span.startTime),
        input=messages,
        output=response,
        available_tools=data.available_tools,
        _tool_calls=tool_calls,
        model=data.model,
        vendor=data.vendor,
        temperature=data.temperature,
        metrics=metrics,
    )


def _extract_tool_result(raw_output: Any) -> Any:
    """Extract the actual tool result string from a raw output value.

    Handles LangChain-style wrapped ToolMessage objects:
      {"output": {"lc": 1, "type": "constructor", "id": [..., "ToolMessage"],
                  "kwargs": {"content": "<actual result>"}}}
    Falls back to the raw string or empty string.
    """
    if raw_output is None:
        return ""
    if isinstance(raw_output, str):
        try:
            raw_output = json.loads(raw_output)
        except (json.JSONDecodeError, ValueError):
            return raw_output
    if isinstance(raw_output, dict):
        # LangChain ToolMessage wrapper: {"output": {"lc": 1, ..., "kwargs": {"content": ...}}}
        inner = raw_output.get("output") or raw_output
        if isinstance(inner, dict) and inner.get("lc") == 1:
            kwargs = inner.get("kwargs") or {}
            content = kwargs.get("content")
            if content is not None:
                return content
        return raw_output
    return raw_output


def _parse_tool_span(otel_span: OTELSpan) -> ToolSpan:
    """Parse a tool execution span directly from a typed OTELSpan."""
    amp = otel_span.ampAttributes
    data = amp.data
    st = amp.status

    # Tool name from data.name or span name
    name = data.name or otel_span.name or "unknown"

    # Arguments from input
    raw_input = amp.input
    if isinstance(raw_input, dict):
        arguments = raw_input
    elif isinstance(raw_input, str):
        try:
            parsed = json.loads(raw_input)
            arguments = parsed if isinstance(parsed, dict) else {"input": raw_input}
        except (json.JSONDecodeError, ValueError):
            arguments = {"input": raw_input}
    else:
        arguments = {}

    metrics = ToolMetrics(
        duration_ms=otel_span.duration_ms,
        error=st.error,
        error_type=st.error_type,
        error_message=st.error_message,
    )

    result = _extract_tool_result(amp.output)

    return ToolSpan(
        span_id=otel_span.spanId,
        parent_span_id=otel_span.parentSpanId,
        start_time=_parse_timestamp(otel_span.startTime),
        name=name,
        arguments=arguments,
        result=result,
        metrics=metrics,
    )


def _parse_retriever_span(otel_span: OTELSpan) -> RetrieverSpan:
    """Parse a retriever span directly from a typed OTELSpan."""
    amp = otel_span.ampAttributes
    data = amp.data
    st = amp.status

    # Query from input
    raw_input = amp.input
    if isinstance(raw_input, str):
        query = raw_input
    elif isinstance(raw_input, dict):
        query = raw_input.get("query", str(raw_input))
    else:
        query = ""

    # Parse retrieved documents
    documents = _parse_retrieved_docs(amp.output)

    metrics = RetrieverMetrics(
        duration_ms=otel_span.duration_ms,
        error=st.error,
        error_type=st.error_type,
        error_message=st.error_message,
        documents_retrieved=len(documents),
    )

    return RetrieverSpan(
        span_id=otel_span.spanId,
        parent_span_id=otel_span.parentSpanId,
        start_time=_parse_timestamp(otel_span.startTime),
        query=query,
        documents=documents,
        vector_db=data.vector_db,
        top_k=data.top_k,
        metrics=metrics,
    )


def _parse_agent_span(otel_span: OTELSpan) -> AgentSpan:
    """Parse an agent span directly from a typed OTELSpan."""
    amp = otel_span.ampAttributes
    data = amp.data
    st = amp.status

    # available_tools already normalised to List[ToolDefinition] in AmpSpanData
    tu = data.token_usage
    token_usage = (
        TokenUsage(
            input_tokens=tu.inputTokens,
            output_tokens=tu.outputTokens,
            total_tokens=tu.totalTokens,
        )
        if tu
        else TokenUsage()
    )

    metrics = AgentMetrics(
        duration_ms=otel_span.duration_ms,
        error=st.error,
        error_type=st.error_type,
        error_message=st.error_message,
        token_usage=token_usage,
    )

    raw_input = amp.input
    raw_output = amp.output

    if isinstance(raw_input, str):
        agent_input = raw_input
    elif isinstance(raw_input, dict):
        agent_input = raw_input.get("input", str(raw_input))
    else:
        agent_input = ""

    if isinstance(raw_output, str):
        agent_output = raw_output
    elif isinstance(raw_output, dict):
        agent_output = raw_output.get("output", str(raw_output))
    else:
        agent_output = ""

    return AgentSpan(
        span_id=otel_span.spanId,
        parent_span_id=otel_span.parentSpanId,
        start_time=_parse_timestamp(otel_span.startTime),
        name=data.name or otel_span.name or "",
        framework=data.framework,
        model=data.model,
        system_prompt=data.system_prompt,
        available_tools=data.available_tools,
        max_iterations=data.max_iter,
        input=agent_input,
        output=agent_output,
        metrics=metrics,
    )


# ============================================================================
# HELPER PARSERS
# ============================================================================


def _parse_messages(raw_input: Any) -> list:
    """Parse messages from LLM input into typed message instances."""
    messages: list = []

    if not raw_input:
        return messages

    if isinstance(raw_input, list):
        for item in raw_input:
            if isinstance(item, dict):
                role = item.get("role", "user")
                content = item.get("content", "")
                if role == "system":
                    messages.append(SystemMessage(content=content))
                elif role == "user":
                    messages.append(UserMessage(content=content))
                elif role == "assistant":
                    messages.append(
                        AssistantMessage(
                            content=content,
                            tool_calls=_parse_tool_calls(item.get("tool_calls") or item.get("toolCalls") or []),
                        )
                    )
                elif role == "tool":
                    messages.append(
                        ToolMessage(
                            content=content,
                            tool_call_id=item.get("tool_call_id", ""),
                        )
                    )
                else:
                    # Unknown role, default to user message
                    messages.append(UserMessage(content=content))
    elif isinstance(raw_input, str):
        messages.append(UserMessage(content=raw_input))

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
            if isinstance(item, dict):
                raw_tcs = item.get("tool_calls") or item.get("toolCalls")
                if raw_tcs:
                    tool_calls.extend(_parse_tool_calls(raw_tcs))
    elif isinstance(raw_output, dict):
        raw_tcs = raw_output.get("tool_calls") or raw_output.get("toolCalls")
        if raw_tcs:
            tool_calls.extend(_parse_tool_calls(raw_tcs))

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
    docs: List[RetrievedDoc] = []

    if not raw_output:
        return docs

    if isinstance(raw_output, list):
        for item in raw_output:
            if isinstance(item, dict):
                docs.append(
                    RetrievedDoc(
                        id=item.get("id", ""),
                        content=item.get("content") or item.get("text") or "",
                        score=item.get("score", 0.0),
                        metadata=item.get("metadata", {}),
                    )
                )

    return docs
