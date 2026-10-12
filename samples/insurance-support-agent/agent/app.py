import os

# Set before importing Strands: without it, tool input/output never reach span attributes.
os.environ.setdefault("OTEL_SEMCONV_STABILITY_OPT_IN", "gen_ai_latest_experimental")

import logging
import asyncio
from contextlib import asynccontextmanager
import threading
import uuid
from collections import OrderedDict

from dotenv import load_dotenv
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from strands import Agent

load_dotenv()

from agent import build_agent
from config import Config
from mcp_client import create_mcp_client

logging.basicConfig(level=logging.INFO)
log = logging.getLogger("insurance-support")

CONFIG = Config.from_env()

# Demo-grade: keyed on the caller-supplied session_id, not the authenticated subject.
MAX_SESSIONS = 500
SESSIONS: "OrderedDict[str, Agent]" = OrderedDict()
_sessions_lock = threading.Lock()

MCP_TOOLS = None


@asynccontextmanager
async def lifespan(app):
    global MCP_TOOLS
    if not CONFIG.use_mcp:
        yield
        return
    client = create_mcp_client(CONFIG)
    try:
        await asyncio.to_thread(client.start)
        MCP_TOOLS = await asyncio.to_thread(client.list_tools_sync)
        if {tool.tool_name for tool in MCP_TOOLS} != {"list_policies", "lookup_policy"}:
            raise RuntimeError("Both read-only insurance MCP tools must be authorized")
        yield
    finally:
        MCP_TOOLS = None
        await asyncio.to_thread(client.stop, None, None, None)


app = FastAPI(title="Insurance Support Agent", version="1.0.0", lifespan=lifespan)

# Local only: when deployed, CORS belongs in Agent Manager — two sets of headers break browsers.
_cors_origins = [o for o in os.environ.get("CORS_ALLOW_ORIGINS", "").split(",") if o]
if _cors_origins:
    app.add_middleware(
        CORSMiddleware,
        allow_origins=_cors_origins,
        allow_methods=["POST", "GET", "OPTIONS"],
        allow_headers=["Authorization", "Content-Type"],
    )


class ChatRequest(BaseModel):
    message: str
    session_id: str | None = None


class ChatResponse(BaseModel):
    response: str
    session_id: str


def _agent_for(session_id: str) -> Agent:
    # Sync endpoint runs on a threadpool, so lookup, insert and eviction share one lock.
    with _sessions_lock:
        agent = SESSIONS.get(session_id)
        if agent is None:
            agent = build_agent(CONFIG, MCP_TOOLS)
            SESSIONS[session_id] = agent
            while len(SESSIONS) > MAX_SESSIONS:
                SESSIONS.popitem(last=False)
        else:
            SESSIONS.move_to_end(session_id)
        return agent


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "company": CONFIG.company_name}


@app.post("/chat", response_model=ChatResponse)
def chat(req: ChatRequest) -> ChatResponse:
    if not req.message.strip():
        raise HTTPException(status_code=400, detail="message must not be empty")

    session_id = req.session_id or str(uuid.uuid4())

    try:
        result = _agent_for(session_id)(req.message)
    except Exception as exc:
        # Upstream errors can carry gateway URLs and provider payloads — log, do not return.
        log.exception("agent invocation failed", extra={"session_id": session_id})
        raise HTTPException(status_code=500, detail="agent invocation failed") from exc

    return ChatResponse(response=str(result), session_id=session_id)
