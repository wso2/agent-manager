"""Manual instrumentation helpers — AMP's instrumentation contract, in code.

This module is the point of the sample. Each helper opens one OpenTelemetry span
and sets exactly the attributes AMP's observer reads for that span kind, so the
span renders with the full trace view in the WSO2 Agent Manager Console and runs
through evaluators.

The contract is layered:

  * Layer 1 — OpenTelemetry GenAI semantic conventions (``gen_ai.*``, plus
    ``db.*`` for retriever spans). The primary set.
  * Layer 2 — OpenLLMetry / Traceloop extension keys (``traceloop.*``) for the
    few decisions OTel has not standardized yet: the ``chain`` span kind,
    ``rerank``, and tool-call arguments / result.

Full reference: the "Manual instrumentation" section of the WSO2 Agent Manager
Instrumentation docs page.

Every helper is a context manager. Helpers whose response data is only known
after a call (LLM, embedding, agent) yield a small result object for the caller
to fill; the helper writes the response attributes when the block exits.
"""

from __future__ import annotations

import json
import os
from contextlib import contextmanager
from dataclasses import dataclass, field
from typing import Any, Iterator

from opentelemetry import baggage, context, trace
from opentelemetry.trace import Span, SpanKind, Status, StatusCode

_TRACER_NAME = "manual-instrumentation-agent"


def _tracer() -> trace.Tracer:
    # Fetched per call so it always binds to the provider init_otel() installed,
    # regardless of import order.
    return trace.get_tracer(_TRACER_NAME)


# --- Trace content toggle --------------------------------------------------
# On the auto path the Traceloop SDK reads TRACELOOP_TRACE_CONTENT. On the
# manual path you control span content yourself — this sample honours
# AMP_TRACE_CONTENT (the variable the platform env-injection trait sets) so
# prompt/completion text can be suppressed without dropping the spans.

def _trace_content_enabled() -> bool:
    return os.getenv("AMP_TRACE_CONTENT", "true").lower() != "false"


def _content(text: str) -> str:
    return text if _trace_content_enabled() else "[redacted]"


def _messages(msgs: list[dict[str, str]]) -> str:
    if _trace_content_enabled():
        return json.dumps(msgs)
    return json.dumps([{"role": m["role"], "content": "[redacted]"} for m in msgs])


# --- Evaluation correlation -------------------------------------------------

@contextmanager
def evaluation_baggage(task_id: str | None, trial_id: str | None) -> Iterator[None]:
    """Attach W3C baggage so a trace can be joined to the evaluation trial that
    produced it. Optional — only relevant when the agent is driven by an
    evaluation run. Baggage propagates to every span started inside the block.
    """
    ctx = context.get_current()
    if task_id:
        ctx = baggage.set_baggage("task_id", task_id, context=ctx)
    if trial_id:
        ctx = baggage.set_baggage("trial_id", trial_id, context=ctx)
    token = context.attach(ctx)
    try:
        yield
    finally:
        context.detach(token)


def mark_error(span: Span, message: str) -> None:
    """Flag a span as failed. The observer turns this into an error badge on the
    span and bumps the trace's error count. Works on any span kind.
    """
    span.set_status(Status(StatusCode.ERROR, message))
    span.set_attribute("error.type", "AgentError")


# --- Result objects (filled by the caller, written out on block exit) ------

@dataclass
class LLMResult:
    response_model: str = ""
    output_messages: list[dict[str, str]] = field(default_factory=list)
    input_tokens: int = 0
    output_tokens: int = 0


@dataclass
class EmbeddingResult:
    response_model: str = ""
    input_tokens: int = 0


@dataclass
class AgentResult:
    output_messages: list[dict[str, str]] = field(default_factory=list)
    input_tokens: int = 0
    output_tokens: int = 0


# --- agent -----------------------------------------------------------------

@contextmanager
def agent_span(
    *,
    name: str,
    description: str,
    framework: str,
    model: str,
    system_instructions: str,
    conversation_id: str,
    input_messages: list[dict[str, str]],
    tools: list[dict[str, Any]] | None = None,
) -> Iterator[tuple[Span, AgentResult]]:
    """Root span for one agent invocation. `gen_ai.operation.name = invoke_agent`."""
    with _tracer().start_as_current_span("invoke_agent", kind=SpanKind.INTERNAL) as span:
        span.set_attribute("gen_ai.operation.name", "invoke_agent")
        span.set_attribute("gen_ai.system", framework)            # framework chip
        span.set_attribute("gen_ai.agent.name", name)             # required
        span.set_attribute("gen_ai.agent.description", description)
        span.set_attribute("gen_ai.request.model", model)
        span.set_attribute("gen_ai.system_instructions", _content(system_instructions))
        span.set_attribute("gen_ai.conversation.id", conversation_id)
        span.set_attribute("gen_ai.input.messages", _messages(input_messages))
        if tools:
            span.set_attribute("gen_ai.agent.tools", json.dumps(tools))
        result = AgentResult()
        try:
            yield span, result
        finally:
            if result.output_messages:
                span.set_attribute("gen_ai.output.messages", _messages(result.output_messages))
            span.set_attribute("gen_ai.usage.input_tokens", result.input_tokens)
            span.set_attribute("gen_ai.usage.output_tokens", result.output_tokens)


# --- chain / workflow ------------------------------------------------------

@contextmanager
def chain_span(*, name: str, workflow_input: Any) -> Iterator[tuple[Span, dict]]:
    """A workflow / pipeline step. OTel has no key for this kind, so the
    observer reads the Layer-2 `traceloop.span.kind`. Put the chain's output in
    `result["output"]` before the block exits.
    """
    with _tracer().start_as_current_span(name, kind=SpanKind.INTERNAL) as span:
        span.set_attribute("traceloop.span.kind", "workflow")     # -> kind = chain
        span.set_attribute("traceloop.entity.input", json.dumps({"input": workflow_input}))
        result: dict[str, Any] = {"output": None}
        try:
            yield span, result
        finally:
            span.set_attribute(
                "traceloop.entity.output", json.dumps({"output": result["output"]})
            )


# --- embedding -------------------------------------------------------------

@contextmanager
def embedding_span(
    *, system: str, request_model: str, texts: list[str]
) -> Iterator[tuple[Span, EmbeddingResult]]:
    """An embedding call. `gen_ai.operation.name = embeddings`."""
    with _tracer().start_as_current_span("embeddings", kind=SpanKind.CLIENT) as span:
        span.set_attribute("gen_ai.operation.name", "embeddings")
        span.set_attribute("gen_ai.system", system)
        span.set_attribute("gen_ai.request.model", request_model)
        for i, text in enumerate(texts):
            # Indexed embedded text. The OTel key here is not fully settled;
            # this is the de-facto form the observer reads.
            span.set_attribute(f"gen_ai.prompt.{i}.content", _content(text))
        result = EmbeddingResult()
        try:
            yield span, result
        finally:
            if result.response_model:
                span.set_attribute("gen_ai.response.model", result.response_model)
            span.set_attribute("gen_ai.usage.input_tokens", result.input_tokens)


# --- retriever -------------------------------------------------------------

@contextmanager
def retriever_span(
    *, vector_db: str, collection: str, top_k: int
) -> Iterator[Span]:
    """A vector-DB retrieval. Uses the OTel database semantic conventions; the
    observer keys `kind = retriever` off `db.system.name`. Retrieved documents
    are not extracted in v1, so they aren't set here.
    """
    with _tracer().start_as_current_span("vector_search", kind=SpanKind.CLIENT) as span:
        span.set_attribute("db.system.name", vector_db)           # required
        span.set_attribute("db.collection.name", collection)
        span.set_attribute("db.vector.query.top_k", top_k)
        yield span


# --- rerank ----------------------------------------------------------------

@contextmanager
def rerank_span(*, model: str, query: str, candidate_count: int) -> Iterator[Span]:
    """A reranking step. Recognized as a *kind* only — a rerank span gets the
    rerank icon, no data card (no RerankData payload in v1). The observer reads
    the Layer-2 `traceloop.span.kind`; the other keys are de-facto signals.
    """
    with _tracer().start_as_current_span("rerank", kind=SpanKind.CLIENT) as span:
        span.set_attribute("traceloop.span.kind", "rerank")       # -> kind = rerank
        span.set_attribute("gen_ai.operation.name", "rerank")     # de-facto, not standard OTel
        span.set_attribute("rerank.model", model)
        span.set_attribute("gen_ai.request.model", model)
        span.set_attribute("traceloop.entity.input", json.dumps({
            "query": _content(query), "candidate_count": candidate_count,
        }))
        yield span


# --- tool ------------------------------------------------------------------

@contextmanager
def tool_span(
    *,
    name: str,
    description: str,
    call_id: str,
    arguments: dict[str, Any],
) -> Iterator[tuple[Span, dict]]:
    """A tool / function call. `gen_ai.operation.name = execute_tool`. OTel has
    no stable key for tool I/O, so arguments/result go in the Layer-2
    `traceloop.entity.*` keys. Put the tool's result in `result["output"]`.
    """
    with _tracer().start_as_current_span("execute_tool", kind=SpanKind.INTERNAL) as span:
        span.set_attribute("gen_ai.operation.name", "execute_tool")
        span.set_attribute("gen_ai.tool.name", name)              # required
        span.set_attribute("gen_ai.tool.description", description)
        span.set_attribute("gen_ai.tool.call.id", call_id)
        span.set_attribute("traceloop.entity.input", json.dumps(arguments))
        result: dict[str, Any] = {"output": None}
        try:
            yield span, result
        finally:
            span.set_attribute(
                "traceloop.entity.output", json.dumps({"result": result["output"]})
            )


# --- llm -------------------------------------------------------------------

@contextmanager
def llm_span(
    *,
    system: str,
    request_model: str,
    input_messages: list[dict[str, str]],
    temperature: float | None = None,
    tools: list[dict[str, Any]] | None = None,
) -> Iterator[tuple[Span, LLMResult]]:
    """An LLM chat call. `gen_ai.operation.name = chat`. LLM evaluators need the
    input and output messages, so those are required for a useful span.
    """
    with _tracer().start_as_current_span("chat", kind=SpanKind.CLIENT) as span:
        span.set_attribute("gen_ai.operation.name", "chat")
        span.set_attribute("gen_ai.system", system)
        span.set_attribute("gen_ai.request.model", request_model)
        if temperature is not None:
            span.set_attribute("gen_ai.request.temperature", temperature)
        span.set_attribute("gen_ai.input.messages", _messages(input_messages))
        if tools:
            span.set_attribute("gen_ai.input.tools", json.dumps(tools))
        result = LLMResult()
        try:
            yield span, result
        finally:
            if result.response_model:
                span.set_attribute("gen_ai.response.model", result.response_model)
            if result.output_messages:
                span.set_attribute("gen_ai.output.messages", _messages(result.output_messages))
            span.set_attribute("gen_ai.usage.input_tokens", result.input_tokens)
            span.set_attribute("gen_ai.usage.output_tokens", result.output_tokens)
