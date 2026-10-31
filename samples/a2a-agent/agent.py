"""The agent behind the sample: an OpenAI-backed A2A ``AgentExecutor``.

Nothing here is A2A-server plumbing. The A2A SDK's request handler calls
``execute()`` once per inbound ``SendMessage`` / ``SendStreamingMessage``, and
the executor's job is to publish task events. The events are the same either
way: the SDK renders them as a single ``Task`` for a blocking call, or as an
SSE stream for a streaming one.

Two skills are implemented (the card advertises both):

  * ``summarize-notes`` - streams a short summary as a text artifact.
  * ``extract-action-items`` - returns action items as a JSON data artifact.

A caller picks one with ``message.metadata.skill``, or by starting the text
with the skill's prefix - ``Summarize:`` or ``Extract action items:`` - which
is the form the card's examples use. Neither means ``summarize-notes``.
"""

from __future__ import annotations

import json
import logging
import os
import uuid
from typing import Any, Awaitable, Callable

from a2a.helpers import new_data_part, new_task_from_user_message, new_text_part
from a2a.server.agent_execution import AgentExecutor, RequestContext
from a2a.server.events import EventQueue
from a2a.server.tasks import TaskUpdater
from a2a.types.a2a_pb2 import TaskState
from google.protobuf.json_format import MessageToDict
from openai import AsyncOpenAI
from opentelemetry.trace import Span

from tracing import AgentRun, agent_span, llm_span, record_exception

log = logging.getLogger("a2a-agent")

AGENT_NAME = "a2a-notes-agent"
AGENT_DESCRIPTION = "Summarizes meeting notes and extracts their action items."

SKILL_SUMMARIZE = "summarize-notes"
SKILL_ACTION_ITEMS = "extract-action-items"

DEFAULT_MODEL = "gpt-5.6-luna"

# A streaming summary is flushed to the task every this many characters, so a
# client sees the text arrive in pieces instead of in one lump at the end.
STREAM_CHUNK_CHARS = 120

SUMMARY_PROMPT = (
    "You summarize meeting notes. Write a short summary (at most three "
    "sentences) of the notes the user provides. Reply with the summary only."
)

ACTION_ITEMS_PROMPT = (
    "You extract action items from meeting notes. Reply with JSON only, in "
    'exactly this shape: {"action_items": [{"owner": "...", "task": "...", '
    '"due": "..."}]}. Use an empty string for anything the notes do not say. '
    "The owner is the person or team who has to do the task, not someone "
    "who is waiting on it. Return an empty list when the notes contain no "
    "action items."
)

TEXT_PREFIXES = {
    SKILL_SUMMARIZE: "summarize:",
    SKILL_ACTION_ITEMS: "extract action items:",
}

_client: AsyncOpenAI | None = None


def client() -> AsyncOpenAI:
    """The OpenAI client, created on first use.

    Reads ``OPENAI_API_KEY`` from the environment, which the platform injects
    as a secret when the agent is deployed through AMP.
    """
    global _client
    if _client is None:
        if not os.getenv("OPENAI_API_KEY"):
            raise RuntimeError("OPENAI_API_KEY is required to run this sample.")
        _client = AsyncOpenAI()
    return _client


def model() -> str:
    return os.getenv("OPENAI_MODEL") or DEFAULT_MODEL


async def stream_summary(
    notes: str, on_chunk: Callable[[str], Awaitable[None]], run: AgentRun
) -> str:
    """Summarize ``notes``, handing each piece of text to ``on_chunk`` as it
    arrives. Returns the complete summary.
    """
    summary = ""
    pending = ""
    with llm_span(
        model=model(),
        system_instructions=SUMMARY_PROMPT,
        user_input=notes,
    ) as (_, call):
        stream = await client().chat.completions.create(
            model=model(),
            messages=[
                {"role": "system", "content": SUMMARY_PROMPT},
                {"role": "user", "content": notes},
            ],
            stream=True,
            # A stream reports token usage only when asked, in a final chunk with no choices.
            stream_options={"include_usage": True},
        )
        async for chunk in stream:
            call.response_id = chunk.id
            call.response_model = chunk.model
            if chunk.usage:
                call.input_tokens = chunk.usage.prompt_tokens
                call.output_tokens = chunk.usage.completion_tokens
            if not chunk.choices:
                continue
            choice = chunk.choices[0]
            if choice.finish_reason:
                call.finish_reasons = [choice.finish_reason]
            delta = choice.delta.content or ""
            summary += delta
            pending += delta
            if len(pending) >= STREAM_CHUNK_CHARS:
                await on_chunk(pending)
                pending = ""
        call.output = summary.strip()
    run.add_usage(call)
    if pending:
        await on_chunk(pending.rstrip())
    return summary.strip()


async def extract_action_items(notes: str, run: AgentRun) -> dict[str, Any]:
    """Extract action items from ``notes`` as the JSON object the skill promises."""
    with llm_span(
        model=model(),
        system_instructions=ACTION_ITEMS_PROMPT,
        user_input=notes,
    ) as (_, call):
        response = await client().chat.completions.create(
            model=model(),
            messages=[
                {"role": "system", "content": ACTION_ITEMS_PROMPT},
                {"role": "user", "content": notes},
            ],
            response_format={"type": "json_object"},
        )
        choice = response.choices[0]
        content = choice.message.content or "{}"
        call.response_id = response.id
        call.response_model = response.model
        call.finish_reasons = [choice.finish_reason]
        call.output = content
        if response.usage:
            call.input_tokens = response.usage.prompt_tokens
            call.output_tokens = response.usage.completion_tokens
    run.add_usage(call)
    parsed = json.loads(content)
    items = parsed.get("action_items")
    if not isinstance(items, list):
        raise ValueError(f"the model did not return an action_items list: {content}")
    return parsed


class UnknownSkillError(ValueError):
    pass


def metadata_skill(context: RequestContext) -> str | None:
    message = context.message
    if message is None or not message.HasField("metadata"):
        return None
    return MessageToDict(message.metadata).get("skill")


def route(context: RequestContext, text: str) -> tuple[str, str]:
    """Pick the skill and the notes it runs on.

    An explicit ``metadata.skill`` wins, and naming a skill this agent does not
    have is an error rather than a silent summary: the caller asked for
    something specific and would otherwise get an answer to a different
    question. A text prefix is stripped so the model sees only the notes.
    """
    skill = metadata_skill(context)
    if skill is not None:
        if skill not in TEXT_PREFIXES:
            raise UnknownSkillError(skill)
        return skill, text

    for skill, prefix in TEXT_PREFIXES.items():
        if text.lower().startswith(prefix):
            return skill, text[len(prefix):].strip()
    return SKILL_SUMMARIZE, text


class NotesAgentExecutor(AgentExecutor):
    """Runs one task: read the notes, do the requested work, publish the result."""

    async def execute(self, context: RequestContext, event_queue: EventQueue) -> None:
        user_input = context.get_user_input().strip()
        with agent_span(
            agent_name=AGENT_NAME,
            agent_description=AGENT_DESCRIPTION,
            model=model(),
            task_id=context.task_id,
            context_id=context.context_id,
            message_id=context.message.message_id,
            user_input=user_input,
        ) as (span, run):
            await self._run_task(context, event_queue, user_input, span, run)

    async def _run_task(
        self,
        context: RequestContext,
        event_queue: EventQueue,
        user_input: str,
        span: Span,
        run: AgentRun,
    ) -> None:
        # The agent owns the task: it publishes the Task before touching its
        # status, carrying the caller's message as the first history entry.
        await event_queue.enqueue_event(new_task_from_user_message(context.message))
        updater = TaskUpdater(event_queue, context.task_id, context.context_id)

        try:
            skill, notes = route(context, user_input)
        except UnknownSkillError as exc:
            run.state = TaskState.TASK_STATE_REJECTED
            await updater.reject(
                updater.new_agent_message(
                    [
                        new_text_part(
                            f"Unknown skill {str(exc)!r}. This agent has: "
                            f"{', '.join(TEXT_PREFIXES)}."
                        )
                    ]
                )
            )
            return
        run.skill = skill
        if not notes:
            run.state = TaskState.TASK_STATE_REJECTED
            await updater.reject(
                updater.new_agent_message(
                    [new_text_part("Send the meeting notes as the message text.")]
                )
            )
            return

        await updater.start_work()
        try:
            if skill == SKILL_ACTION_ITEMS:
                await self._extract_action_items(updater, notes, run)
            else:
                await self._summarize(updater, notes, run)
        except Exception as exc:  # noqa: BLE001 - report any failure on the task
            log.exception("task %s failed", context.task_id)
            record_exception(span, exc)
            run.state = TaskState.TASK_STATE_FAILED
            await updater.failed(
                updater.new_agent_message(
                    [new_text_part(f"The agent could not finish this task: {exc}")]
                )
            )
            return
        run.state = TaskState.TASK_STATE_COMPLETED

    async def _summarize(self, updater: TaskUpdater, notes: str, run: AgentRun) -> None:
        # One artifact, built up from several chunks. A streaming client renders
        # them as they arrive; a blocking client gets the merged artifact.
        artifact_id = str(uuid.uuid4())
        first = True

        async def add_chunk(text: str) -> None:
            nonlocal first
            await updater.add_artifact(
                [new_text_part(text)],
                artifact_id=artifact_id,
                name="summary.txt",
                append=not first,
            )
            first = False

        summary = await stream_summary(notes, add_chunk, run)
        run.output = summary
        if first:  # the model returned nothing to stream
            await add_chunk("")
        await updater.complete(
            updater.new_agent_message([new_text_part("Summary ready.")])
        )
        log.info("summarized %d characters into %d", len(notes), len(summary))

    async def _extract_action_items(
        self, updater: TaskUpdater, notes: str, run: AgentRun
    ) -> None:
        items = await extract_action_items(notes, run)
        run.output = json.dumps(items)
        await updater.add_artifact(
            [new_data_part(items, media_type="application/json")],
            name="action-items.json",
            last_chunk=True,
        )
        count = len(items["action_items"])
        await updater.complete(
            updater.new_agent_message(
                [new_text_part(f"Extracted {count} action item(s).")]
            )
        )

    async def cancel(self, context: RequestContext, event_queue: EventQueue) -> None:
        """A cancelled task is marked cancelled; the in-flight model call is
        abandoned, since the notes it was working on are no longer wanted.
        """
        updater = TaskUpdater(event_queue, context.task_id, context.context_id)
        await updater.cancel()
