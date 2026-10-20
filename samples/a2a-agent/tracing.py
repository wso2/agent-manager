"""Manual OpenTelemetry instrumentation for the A2A agent.

One ``SendMessage`` produces one trace:

    A2A SendMessage                 server span: the A2A operation
    └── invoke_agent a2a-notes-agent    agent span: the task the executor ran
        └── chat gpt-5.6-luna               LLM span: the OpenAI call

The server span carries the A2A protocol details: the operation, the binding it
arrived on, the ``A2A-Version`` and ``A2A-Extensions`` service parameters, and
the message, task and context IDs with the task's final state. Its input and
output are the operation's request and result as A2A JSON. It also continues
the caller's W3C trace context, which is how the A2A spec's observability
guidance asks clients and servers to take part in distributed tracing, so a
calling agent's trace and this one join up.

The agent and LLM spans follow the OpenTelemetry GenAI semantic conventions AMP's
observer reads (``gen_ai.*``). OpenTelemetry has no semantic conventions for A2A
yet, so the protocol details live under ``a2a.*``, named after the spec's fields.
"""

from __future__ import annotations

import json
import logging
import os
from collections.abc import AsyncGenerator, Awaitable, Callable
from contextlib import contextmanager
from dataclasses import dataclass, field
from typing import Any, Iterator

from google.protobuf.json_format import MessageToDict
from google.protobuf.message import Message as ProtoMessage
from opentelemetry import propagate, trace
from opentelemetry.trace import Span, SpanKind, Status, StatusCode

from a2a.server.context import ServerCallContext
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.types.a2a_pb2 import (
    CancelTaskRequest,
    Message,
    SendMessageRequest,
    SubscribeToTaskRequest,
    Task,
    TaskArtifactUpdateEvent,
    TaskState,
    TaskStatusUpdateEvent,
)
from a2a.utils.constants import VERSION_HEADER

log = logging.getLogger("a2a-agent")

TRACER_NAME = "a2a-notes-agent"
AGENT_FRAMEWORK = "a2a-sdk"
LLM_PROVIDER = "openai"

BINDING_JSONRPC = "JSONRPC"
BINDING_HTTP_JSON = "HTTP+JSON"


def init_tracing() -> None:
    """Export spans to AMP, or run untraced when no AMP endpoint is configured.

    A deployed agent gets ``AMP_OTEL_ENDPOINT`` and ``AMP_AGENT_API_KEY`` from
    AMP's env-injection trait. A local run usually has neither, and the spans
    then go to OpenTelemetry's no-op tracer instead of failing the start-up.
    """
    if not os.getenv("AMP_OTEL_ENDPOINT"):
        log.info("AMP_OTEL_ENDPOINT is not set; spans will not be exported")
        return
    from amp_instrumentation import init_otel

    init_otel()


def _tracer() -> trace.Tracer:
    return trace.get_tracer(TRACER_NAME)


def _trace_content_enabled() -> bool:
    return os.getenv("AMP_TRACE_CONTENT", "true").lower() != "false"


def _text_messages(role: str, text: str, **extra: Any) -> str:
    """One OTel GenAI message in the ``parts`` shape, redacted when
    ``AMP_TRACE_CONTENT`` is false so the span keeps its structure.
    """
    content = text if _trace_content_enabled() else "[redacted]"
    return json.dumps([{"role": role, "parts": [{"type": "text", "content": content}], **extra}])


def record_exception(span: Span, exc: BaseException) -> None:
    """Record ``exc`` on ``span``, keeping only its type when
    ``AMP_TRACE_CONTENT`` is false: an exception's message can quote the model
    output or the notes.
    """
    if _trace_content_enabled():
        span.record_exception(exc)
    else:
        span.add_event("exception", {"exception.type": type(exc).__name__})


def _state_name(state: int) -> str:
    return TaskState.Name(state)


CONTENT_PART_FIELDS = {"text", "data", "raw", "url"}


def _redact_parts(value: Any) -> Any:
    """Redact the content of every A2A ``Part`` inside ``value``, keeping the
    IDs, states and structure around it.
    """
    if isinstance(value, list):
        return [_redact_parts(item) for item in value]
    if not isinstance(value, dict):
        return value
    redacted = {key: _redact_parts(item) for key, item in value.items()}
    # Caller-supplied metadata can hold a "parts" key that is not a Part list.
    parts = redacted.get("parts")
    if isinstance(parts, list):
        for part in parts:
            if isinstance(part, dict):
                for key in CONTENT_PART_FIELDS & part.keys():
                    part[key] = "[redacted]"
    return redacted


def _a2a_json(message: ProtoMessage) -> str:
    """``message`` in the A2A JSON form the wire uses, redacted when
    ``AMP_TRACE_CONTENT`` is false.
    """
    value = MessageToDict(message)
    return json.dumps(value if _trace_content_enabled() else _redact_parts(value))


# --- server span: one per A2A operation -------------------------------------


def _binding(context: ServerCallContext) -> str:
    # The JSON-RPC dispatcher records the method it decoded; the REST one does not.
    return BINDING_JSONRPC if "method" in context.state else BINDING_HTTP_JSON


def start_server_span(
    method: str, params: ProtoMessage, context: ServerCallContext
) -> Span:
    """Start, without making current, the server span for one A2A operation,
    parented to the caller's trace when the request carries ``traceparent``.
    """
    headers = context.state.get("headers", {})
    span = _tracer().start_span(
        f"A2A {method}", context=propagate.extract(headers), kind=SpanKind.SERVER
    )
    # AMP reads a span's input and output, and so its Overview, only for the
    # span kinds it recognises; an A2A operation wrapping the agent is a workflow.
    span.set_attribute("traceloop.span.kind", "workflow")
    span.set_attribute("traceloop.entity.input", _a2a_json(params))
    binding = _binding(context)
    span.set_attribute("a2a.method", method)
    span.set_attribute("a2a.protocol.binding", binding)
    span.set_attribute("rpc.method", method)
    if version := headers.get(VERSION_HEADER.lower()):
        span.set_attribute("a2a.protocol.version", version)
    if context.requested_extensions:
        span.set_attribute("a2a.extensions", sorted(context.requested_extensions))
    if binding == BINDING_JSONRPC and (request_id := context.state.get("request_id")) is not None:
        span.set_attribute("a2a.jsonrpc.request.id", str(request_id))
    if context.tenant:
        span.set_attribute("a2a.tenant", context.tenant)
    return span


async def _stream_in_span(
    span: Span,
    events: AsyncGenerator[Any, None],
    load_task: Callable[[str], Awaitable[Task | None]],
) -> AsyncGenerator[Any, None]:
    """Relay ``events``, with ``span`` current only while each one is produced.

    The SSE response pulls events from more than one task, so a span held
    current across a ``yield`` is detached in a context it was never attached
    in, and OpenTelemetry logs "Failed to detach context".

    A stream delivers its result in pieces, so the span's output is the task as
    the task store holds it once the stream ends.
    """
    task_id = ""
    try:
        while True:
            with trace.use_span(span, end_on_exit=False):
                try:
                    event = await anext(events)
                except StopAsyncIteration:
                    if task_id and (task := await load_task(task_id)):
                        _record_output(span, task)
                    return
                task_id = _record_stream_event(span, event) or task_id
            yield event
    finally:
        span.end()


def _record_message(span: Span, message: Message) -> None:
    span.set_attribute("a2a.message.id", message.message_id)
    if message.task_id:
        span.set_attribute("a2a.task.id", message.task_id)
    if message.context_id:
        _record_context_id(span, message.context_id)


def _record_context_id(span: Span, context_id: str) -> None:
    # An A2A context groups the tasks and messages of one conversation.
    span.set_attribute("a2a.context.id", context_id)
    span.set_attribute("gen_ai.conversation.id", context_id)


def _record_task(span: Span, task_id: str, context_id: str, state: int) -> None:
    span.set_attribute("a2a.task.id", task_id)
    _record_context_id(span, context_id)
    span.set_attribute("a2a.task.state", _state_name(state))
    if state == TaskState.TASK_STATE_FAILED:
        span.set_status(Status(StatusCode.ERROR, "task failed"))


def _record_output(span: Span, result: Task | Message) -> None:
    span.set_attribute("traceloop.entity.output", _a2a_json(result))


def _record_result(span: Span, result: Task | Message | None) -> None:
    if isinstance(result, Task):
        _record_task(span, result.id, result.context_id, result.status.state)
        _record_output(span, result)
    elif isinstance(result, Message):
        span.set_attribute("a2a.response.message.id", result.message_id)
        _record_output(span, result)


def _record_stream_event(span: Span, event: Any) -> str | None:
    """Keep the span's task fields at the latest state the stream reported.
    Returns the event's task ID.
    """
    if isinstance(event, Task):
        _record_task(span, event.id, event.context_id, event.status.state)
        return event.id
    if isinstance(event, TaskStatusUpdateEvent):
        _record_task(span, event.task_id, event.context_id, event.status.state)
        return event.task_id
    if isinstance(event, TaskArtifactUpdateEvent):
        span.set_attribute("a2a.task.id", event.task_id)
        return event.task_id
    if isinstance(event, Message):
        _record_result(span, event)
    return None


class TracedRequestHandler(DefaultRequestHandler):
    """The SDK's request handler, with a server span around each operation that
    runs or changes a task.

    Wrapping the handler rather than the HTTP routes gives both bindings the
    same span, named after the spec's operation instead of a URL. The reads
    (``GetTask``, ``ListTasks``, push-config and extended-card lookups) are left
    untraced so polling a task does not fill the trace list.
    """

    async def on_message_send(
        self, params: SendMessageRequest, context: ServerCallContext
    ) -> Task | Message:
        span = start_server_span("SendMessage", params, context)
        with trace.use_span(span, end_on_exit=True):
            _record_message(span, params.message)
            result = await super().on_message_send(params, context)
            _record_result(span, result)
            return result

    async def on_message_send_stream(
        self, params: SendMessageRequest, context: ServerCallContext
    ) -> AsyncGenerator[Any, None]:
        span = start_server_span("SendStreamingMessage", params, context)
        _record_message(span, params.message)
        async for event in _stream_in_span(
            span, super().on_message_send_stream(params, context), self._load_task(context)
        ):
            yield event

    async def on_cancel_task(
        self, params: CancelTaskRequest, context: ServerCallContext
    ) -> Task | None:
        span = start_server_span("CancelTask", params, context)
        with trace.use_span(span, end_on_exit=True):
            span.set_attribute("a2a.task.id", params.id)
            result = await super().on_cancel_task(params, context)
            _record_result(span, result)
            return result

    async def on_subscribe_to_task(
        self, params: SubscribeToTaskRequest, context: ServerCallContext
    ) -> AsyncGenerator[Any, None]:
        span = start_server_span("SubscribeToTask", params, context)
        span.set_attribute("a2a.task.id", params.id)
        async for event in _stream_in_span(
            span, super().on_subscribe_to_task(params, context), self._load_task(context)
        ):
            yield event

    def _load_task(
        self, context: ServerCallContext
    ) -> Callable[[str], Awaitable[Task | None]]:
        return lambda task_id: self.task_store.get(task_id, context)


# --- agent span: the task the executor runs ---------------------------------


@dataclass
class LLMCall:
    response_id: str = ""
    response_model: str = ""
    output: str = ""
    finish_reasons: list[str] = field(default_factory=list)
    input_tokens: int = 0
    output_tokens: int = 0


@dataclass
class AgentRun:
    """What the executor learns while it works, written to the agent span when
    the block exits.
    """

    skill: str = ""
    output: str = ""
    state: int = TaskState.TASK_STATE_UNSPECIFIED
    input_tokens: int = 0
    output_tokens: int = 0

    def add_usage(self, call: LLMCall) -> None:
        self.input_tokens += call.input_tokens
        self.output_tokens += call.output_tokens


@contextmanager
def agent_span(
    *,
    agent_name: str,
    agent_description: str,
    model: str,
    task_id: str,
    context_id: str,
    message_id: str,
    user_input: str,
) -> Iterator[tuple[Span, AgentRun]]:
    with _tracer().start_as_current_span(
        f"invoke_agent {agent_name}", kind=SpanKind.INTERNAL
    ) as span:
        span.set_attribute("gen_ai.operation.name", "invoke_agent")
        span.set_attribute("gen_ai.system", AGENT_FRAMEWORK)
        span.set_attribute("gen_ai.agent.name", agent_name)
        span.set_attribute("gen_ai.agent.description", agent_description)
        span.set_attribute("gen_ai.request.model", model)
        span.set_attribute("gen_ai.input.messages", _text_messages("user", user_input))
        span.set_attribute("a2a.task.id", task_id)
        span.set_attribute("a2a.message.id", message_id)
        _record_context_id(span, context_id)
        run = AgentRun()
        try:
            yield span, run
        finally:
            if run.skill:
                span.set_attribute("a2a.skill.id", run.skill)
            if run.output:
                span.set_attribute("gen_ai.output.messages", _text_messages("assistant", run.output))
            span.set_attribute("a2a.task.state", _state_name(run.state))
            if run.state == TaskState.TASK_STATE_FAILED:
                span.set_status(Status(StatusCode.ERROR, "task failed"))
            span.set_attribute("gen_ai.usage.input_tokens", run.input_tokens)
            span.set_attribute("gen_ai.usage.output_tokens", run.output_tokens)


# --- LLM span: one OpenAI call ----------------------------------------------


@contextmanager
def llm_span(
    *,
    model: str,
    system_instructions: str,
    user_input: str,
) -> Iterator[tuple[Span, LLMCall]]:
    with _tracer().start_as_current_span(f"chat {model}", kind=SpanKind.CLIENT) as span:
        span.set_attribute("gen_ai.operation.name", "chat")
        span.set_attribute("gen_ai.system", LLM_PROVIDER)
        span.set_attribute("gen_ai.provider.name", LLM_PROVIDER)
        span.set_attribute("gen_ai.request.model", model)
        span.set_attribute(
            "gen_ai.system_instructions",
            system_instructions if _trace_content_enabled() else "[redacted]",
        )
        span.set_attribute("gen_ai.input.messages", _text_messages("user", user_input))
        call = LLMCall()
        try:
            yield span, call
        finally:
            if call.response_id:
                span.set_attribute("gen_ai.response.id", call.response_id)
            if call.response_model:
                span.set_attribute("gen_ai.response.model", call.response_model)
            if call.finish_reasons:
                span.set_attribute("gen_ai.response.finish_reasons", call.finish_reasons)
            if call.output:
                finish_reason = call.finish_reasons[0] if call.finish_reasons else "stop"
                span.set_attribute(
                    "gen_ai.output.messages",
                    _text_messages("assistant", call.output, finish_reason=finish_reason),
                )
            span.set_attribute("gen_ai.usage.input_tokens", call.input_tokens)
            span.set_attribute("gen_ai.usage.output_tokens", call.output_tokens)
