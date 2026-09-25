"""FastAPI entrypoint: the A2A server for the sample agent.

Three surfaces, which together are everything an A2A client needs:

  * ``GET /.well-known/agent-card.json`` - the agent card: identity, the two
    transports, and the skills this agent offers.
  * ``POST /rpc`` - the JSON-RPC transport.
  * ``POST /rest/...`` - the HTTP+JSON transport (``/message:send``,
    ``/tasks/{id}``, and so on).

The paths are the ones AMP's gateway publishes an ``a2a-agent`` under: it
routes ``/<agent-name>/rpc`` and ``/<agent-name>/rest`` here and rewrites the
card it serves so clients dial the gateway, not this process.
"""

from __future__ import annotations

import os
from typing import Any, Awaitable, Callable

from dotenv import load_dotenv
from fastapi import FastAPI
from a2a.server.request_handlers import DefaultRequestHandler
from a2a.server.routes.agent_card_routes import create_agent_card_routes
from a2a.server.routes.fastapi_routes import add_a2a_routes_to_fastapi
from a2a.server.routes.jsonrpc_routes import create_jsonrpc_routes
from a2a.server.routes.rest_routes import create_rest_routes
from a2a.server.tasks import InMemoryTaskStore
from a2a.types.a2a_pb2 import (
    AgentCapabilities,
    AgentCard,
    AgentInterface,
    AgentSkill,
)

from agent import SKILL_ACTION_ITEMS, SKILL_SUMMARIZE, NotesAgentExecutor

# For local runs only: the deployed pod gets its environment from AMP. Loading
# here means the agent card below sees AGENT_PUBLIC_BASE_URL either way.
load_dotenv()

PORT = 9099
RPC_PATH = "/rpc"
REST_PATH = "/rest"
PROTOCOL_VERSION = "1.0"
VERSION_HEADER = b"a2a-version"


class DefaultProtocolVersion:
    """Stamp ``A2A-Version: 1.0`` on requests that arrive without one.

    A2A 1.0 clients send the header, and a request without it is read as 0.3 -
    which this agent does not serve. Clients that talk to the agent directly
    may omit it, so the version this agent serves is applied here rather than
    left to the caller.

    This only helps direct calls, such as a local run. Behind AMP's gateway a
    request without the header is rejected with a 400 before it reaches the
    agent, so deployed clients must send it themselves.
    """

    def __init__(self, app: Callable) -> None:
        self.app = app

    async def __call__(
        self,
        scope: dict[str, Any],
        receive: Callable[[], Awaitable[dict[str, Any]]],
        send: Callable[[dict[str, Any]], Awaitable[None]],
    ) -> None:
        if scope["type"] == "http" and not any(
            key.lower() == VERSION_HEADER and value for key, value in scope["headers"]
        ):
            scope = {**scope, "headers": [*scope["headers"], (VERSION_HEADER, b"1.0")]}
        await self.app(scope, receive, send)

SUMMARY_EXAMPLE = (
    "Summarize: standup covered the gateway rollout, Priya will cut the release "
    "branch on Thursday, Sam is blocked on the staging certificate."
)
ACTION_ITEMS_EXAMPLE = (
    "Extract action items: standup covered the gateway rollout, Priya will cut "
    "the release branch on Thursday, Sam is blocked on the staging certificate."
)


def build_agent_card() -> AgentCard:
    """Describe the agent: who it is, how to reach it, and what it can do.

    ``supportedInterfaces`` lists one entry per transport, which is what a
    client picks from. The URLs are this agent's own address; the gateway
    rewrites them to its own when it serves the card.
    """
    base_url = os.getenv("AGENT_PUBLIC_BASE_URL", f"http://localhost:{PORT}").rstrip("/")
    return AgentCard(
        name="a2a-notes-agent",
        description=(
            "Summarizes meeting notes and extracts their action items. "
            "The worked example of AMP's A2A agent support."
        ),
        version="1.0.0",
        capabilities=AgentCapabilities(streaming=True, push_notifications=False),
        default_input_modes=["text/plain"],
        default_output_modes=["text/plain", "application/json"],
        supported_interfaces=[
            AgentInterface(
                url=f"{base_url}{RPC_PATH}",
                protocol_binding="JSONRPC",
                protocol_version=PROTOCOL_VERSION,
            ),
            AgentInterface(
                url=f"{base_url}{REST_PATH}",
                protocol_binding="HTTP+JSON",
                protocol_version=PROTOCOL_VERSION,
            ),
        ],
        skills=[
            AgentSkill(
                id=SKILL_SUMMARIZE,
                name="Summarize notes",
                description=(
                    "Condenses meeting notes into a short summary. Select it "
                    'with message metadata {"skill": "summarize-notes"} or a '
                    '"Summarize:" text prefix; it is also the default.'
                ),
                tags=["summarize", "notes"],
                examples=[SUMMARY_EXAMPLE],
            ),
            AgentSkill(
                id=SKILL_ACTION_ITEMS,
                name="Extract action items",
                description=(
                    "Pulls the action items out of meeting notes, with owner and "
                    "due date where the notes give them, as JSON. Select it with "
                    'message metadata {"skill": "extract-action-items"} or an '
                    '"Extract action items:" text prefix.'
                ),
                tags=["extract", "action-items", "json"],
                examples=[ACTION_ITEMS_EXAMPLE],
            ),
        ],
    )


card = build_agent_card()

# One handler and one task store behind both transports, so a task sent over
# JSON-RPC can be read back over HTTP+JSON.
handler = DefaultRequestHandler(
    agent_executor=NotesAgentExecutor(),
    task_store=InMemoryTaskStore(),
    agent_card=card,
)

app = FastAPI(title="A2A Notes Agent")
app.add_middleware(DefaultProtocolVersion)
add_a2a_routes_to_fastapi(
    app,
    agent_card_routes=create_agent_card_routes(card),
    jsonrpc_routes=create_jsonrpc_routes(handler, rpc_url=RPC_PATH),
    rest_routes=create_rest_routes(handler, path_prefix=REST_PATH),
)


@app.get("/health")
def health() -> dict[str, str]:
    """Readiness probe, and a quick check that the process is up."""
    return {"status": "ok"}
