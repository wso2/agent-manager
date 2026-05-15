"""FastAPI entrypoint for the IT helpdesk agent.

Implements the AM chat-agent contract: ``POST /chat`` on port 8000 accepting
``{session_id, message, context}`` and returning ``{response, session_id}``.
``GET /health`` is provided for local checks (AM does not require it).
"""

from __future__ import annotations

import logging
from typing import Any

from fastapi import FastAPI, HTTPException
from langchain_core.messages import AIMessage, HumanMessage
from pydantic import BaseModel

from agent import build_agent
from config import Config

logging.basicConfig(level=logging.INFO)
log = logging.getLogger("it-helpdesk")

CONFIG = Config.from_env()
AGENT = build_agent(CONFIG)
log.info(
    "IT helpdesk agent ready (company=%s, tone=%s)",
    CONFIG.company_name,
    CONFIG.tone,
)


class ChatRequest(BaseModel):
    message: str
    session_id: str | None = None
    context: dict[str, Any] | None = None


class ChatResponse(BaseModel):
    response: str
    session_id: str | None = None


app = FastAPI(title="IT Helpdesk Agent", version="0.1.0")


@app.get("/health")
def health() -> dict[str, Any]:
    return {"status": "ok", "company": CONFIG.company_name}


@app.post("/chat", response_model=ChatResponse)
def chat(req: ChatRequest) -> ChatResponse:
    try:
        result = AGENT.invoke({"messages": [HumanMessage(content=req.message)]})
    except Exception as exc:  # noqa: BLE001
        log.exception("agent invocation failed")
        raise HTTPException(status_code=500, detail=str(exc)) from exc

    final: Any = None
    for m in reversed(result.get("messages", [])):
        if isinstance(m, AIMessage):
            final = m.content
            break
    if final is None:
        final = "(no response)"
    if isinstance(final, list):
        final = "\n".join(
            part.get("text", "") if isinstance(part, dict) else str(part) for part in final
        )
    return ChatResponse(response=str(final), session_id=req.session_id)
