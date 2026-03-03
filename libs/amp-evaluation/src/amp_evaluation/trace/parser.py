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

from dataclasses import replace as dataclass_replace
from typing import Dict, Any, List, Optional
import logging
import uuid

from .models import (
    Trace,
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
    SystemMessage,
    UserMessage,
    AssistantMessage,
    ToolMessage,
    ToolCall,
    RetrievedDoc,
)
from .fetcher import OTELTrace, OTELSpan, AmpAttributes, _parse_timestamp


logger = logging.getLogger(__name__)


# ============================================================================
# SPAN FILTERING UTILITIES
# ============================================================================


# Infrastructure span kinds that add no semantic value for evaluation
INFRASTRUCTURE_KINDS = {"chain", "unknown", "task", "crewaitask"}

# Semantic span kinds that should be kept for evaluation
SEMANTIC_KINDS = {"llm", "tool", "agent", "retriever", "embedding"}


def filter_infrastructure_spans(spans: List[OTELSpan], create_synthetic_root: bool = True) -> List[OTELSpan]:
    """
    Filter infrastructure spans while preserving trace tree structure.

    Removes spans with kind: chain, unknown, task, crewaitask
    Keeps semantic spans: llm, tool, agent, retriever, embedding
    Remaps parent references to maintain valid tree.

    Args:
        spans: List of OTEL spans to filter
        create_synthetic_root: If True, creates synthetic root when >1 orphaned semantic span

    Returns:
        Filtered list of OTEL spans with remapped parent references
    """
    if not spans:
        return spans

    # Phase 1: Build indices
    spans_by_id = {s.spanId: s for s in spans}

    # Phase 2: Calculate remappings
    remap_map = {}
    for span in spans:
        kind = span.ampAttributes.kind
        if kind in INFRASTRUCTURE_KINDS:
            ancestor = _find_semantic_ancestor(span.spanId, spans_by_id)
            remap_map[span.spanId] = ancestor

    # Phase 3: Detect orphans
    semantic_spans = [s for s in spans if s.ampAttributes.kind in SEMANTIC_KINDS]
    orphans = []

    for span in semantic_spans:
        parent_id = span.parentSpanId
        if parent_id:
            # Walk up to find semantic parent
            final_parent = remap_map.get(parent_id, parent_id)
            if final_parent is None:
                orphans.append(span)
        elif parent_id is None:
            # Root span - check if it's infrastructure
            kind = span.ampAttributes.kind
            if kind in INFRASTRUCTURE_KINDS:
                orphans.append(span)

    # Phase 4: Create synthetic root if needed
    synthetic_root = None
    if create_synthetic_root and len(orphans) > 1:
        # Find min start time
        start_times = [s.startTime for s in orphans if s.startTime]
        min_start = min(start_times) if start_times else ""

        # Find max end time (use start+1ms if no endTime)
        end_times = [s.endTime if hasattr(s, "endTime") and s.endTime else s.startTime for s in orphans if s.startTime]
        max_end = max(end_times) if end_times else min_start

        # Create synthetic root span using OTELSpan dataclass
        synthetic_root_id = f"_synthetic_root_{uuid.uuid4().hex[:8]}"

        # Get trace ID from first orphan
        trace_id = orphans[0].traceId if orphans else "unknown"

        synthetic_root = OTELSpan(  # type: ignore[call-arg]
            traceId=trace_id,
            spanId=synthetic_root_id,
            name="trace_root",
            service="synthetic",
            startTime=min_start,
            endTime=max_end,
            durationInNanos=0,
            kind="INTERNAL",
            status="OK",
            parentSpanId=None,
            ampAttributes=AmpAttributes(kind="unknown", synthetic=True),
            attributes={},
        )

    # Phase 5: Filter & Remap
    filtered_spans = []
    if synthetic_root:
        filtered_spans.append(synthetic_root)

    for span in spans:
        kind = span.ampAttributes.kind
        if kind in SEMANTIC_KINDS:
            # Remap parent
            old_parent = span.parentSpanId
            new_parent = remap_map.get(old_parent or "", old_parent)

            if new_parent is None and len(orphans) > 1:
                # Orphan, connect to synthetic root
                new_parent = synthetic_root.spanId if synthetic_root else None

            # Create a copy of the span with the remapped parent to avoid mutating the original
            filtered_spans.append(dataclass_replace(span, parentSpanId=new_parent))

    # Phase 6: Validate
    _validate_trace_structure(filtered_spans)

    return filtered_spans


def _find_semantic_ancestor(span_id: str, spans_by_id: Dict[str, OTELSpan]) -> Optional[str]:
    """
    Walk up parent chain to find first semantic ancestor.

    Args:
        span_id: Starting span ID
        spans_by_id: Lookup dict of span ID to OTELSpan

    Returns:
        Span ID of first semantic ancestor, or None if no semantic ancestor found
    """
    visited = set()
    current_id = span_id

    while current_id in spans_by_id:
        if current_id in visited:
            logger.warning(f"Cycle detected in span hierarchy at {current_id}")
            return None  # Cycle detected
        visited.add(current_id)

        current_span = spans_by_id[current_id]
        parent_id = current_span.parentSpanId

        if parent_id is None:
            return None  # Reached root

        if parent_id not in spans_by_id:
            logger.warning(f"Parent span {parent_id} not found for span {current_id}")
            return None

        parent_span = spans_by_id[parent_id]
        parent_kind = parent_span.ampAttributes.kind

        if parent_kind in SEMANTIC_KINDS:
            return parent_id  # Found semantic ancestor

        current_id = parent_id  # Continue walking

    return None


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
            # For non-important spans (embedding, rerank, task, chain, etc.),
            # still count token usage if available
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
        llm_call_count=len(llm_spans),
        tool_call_count=len(tool_spans),
        retrieval_count=len(retriever_spans),
        agent_span_count=len(agent_spans),
        total_span_count=trace.spanCount if trace.spanCount is not None else len(trace.spans),
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
        messages=messages,
        response=response,
        tool_calls=tool_calls,
        model=data.model,
        vendor=data.vendor,
        temperature=data.temperature,
        metrics=metrics,
    )


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
        arguments = {"input": raw_input}
    else:
        arguments = {}

    metrics = ToolMetrics(
        duration_ms=otel_span.duration_ms,
        error=st.error,
        error_type=st.error_type,
        error_message=st.error_message,
    )

    return ToolSpan(
        span_id=otel_span.spanId,
        parent_span_id=otel_span.parentSpanId,
        start_time=_parse_timestamp(otel_span.startTime),
        name=name,
        arguments=arguments,
        result=amp.output or "",
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

    # available_tools already normalised to List[str] in AmpSpanData
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
                            tool_calls=_parse_tool_calls(item.get("tool_calls", [])),
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
