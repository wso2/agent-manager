from __future__ import annotations

from datetime import datetime, timezone
import json
import logging
from typing import Any

from fastapi import FastAPI, HTTPException, status
from langchain_core.messages import HumanMessage
from pydantic import BaseModel, Field, field_validator

from graph import build_graph

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)

agent_graph = build_graph()

class ChatRequest(BaseModel):
    message: str
    session_id: str
    context: dict[str, Any] = Field(default_factory=dict)

    @field_validator("session_id", mode="before")
    @classmethod
    def validate_session_id(cls, value: Any) -> str:
        if value is None:
            raise ValueError("session_id must be a non-empty string")
        if not isinstance(value, str):
            value = str(value)
        trimmed = value.strip()
        if not trimmed:
            raise ValueError("session_id must be a non-empty string")
        return trimmed


class ChatResponse(BaseModel):
    response: str

app = FastAPI(title="Hotel Booking Agent")

def _wrap_user_message(user_message: str, context: dict[str, Any]) -> str:
    now = datetime.now(timezone.utc).isoformat()
    context_json = json.dumps(context, default=str, ensure_ascii=True)
    return (
        f"Request Context JSON:\n{context_json}\n"
        f"UTC Time now:\n{now}\n\n"
        f"User Query:\n{user_message}"
    )

def _resolve_thread_id(session_id: str, context: dict[str, Any]) -> str:
    session_id = session_id.strip()
    if not session_id:
        raise ValueError("session_id must be a non-empty string")
    context_user_id = context.get("user_id")
    if isinstance(context_user_id, str) and context_user_id.strip():
        return f"{context_user_id.strip()}:{session_id}"
    return f"anonymous:{session_id}"


@app.post("/chat", response_model=ChatResponse)
def chat(request: ChatRequest) -> ChatResponse:
    wrapped_message = _wrap_user_message(request.message, request.context)
    thread_id = _resolve_thread_id(request.session_id, request.context)
    try:
        result = agent_graph.invoke(
            {"messages": [HumanMessage(content=wrapped_message)]},
            config={
                "recursion_limit": 50,
                "configurable": {"thread_id": thread_id},
            },
        )
    except Exception:
        logging.exception(
            "chat invoke failed: thread_id=%s session_id=%s",
            thread_id,
            request.session_id,
        )
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Internal server error",
        )

    messages = result.get("messages") if isinstance(result, dict) else None
    if not messages:
        return ChatResponse(response="")

    last_message = messages[-1]
    content = last_message.content
    if isinstance(content, str):
        response_text = content
    elif isinstance(content, list):
        response_text = "\n".join(str(part) for part in content)
    else:
        response_text = str(content)
    return ChatResponse(response=response_text)
