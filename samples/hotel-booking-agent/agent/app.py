from __future__ import annotations

from datetime import datetime, timezone
import logging

from fastapi import FastAPI, HTTPException, status
from langchain_core.messages import HumanMessage
from pydantic import BaseModel

from graph import build_graph

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)

agent_graph = build_graph()

class ChatRequest(BaseModel):
    message: str
    session_id: str | None = None
    user_id: str
    user_name: str | None = None


class ChatResponse(BaseModel):
    message: str

app = FastAPI(title="Hotel Booking Agent")

def _wrap_user_message(user_message: str, user_id: str, user_name: str | None) -> str:
    now = datetime.now(timezone.utc).isoformat()
    resolved_user_id = user_id
    resolved_user_name = user_name or "Traveler"
    return (
        f"User ID: {resolved_user_id}\n"
        f"User Name: {resolved_user_name}\n"
        f"User Context (non-hotel identifiers): {resolved_user_name} ({resolved_user_id})\n"
        f"UTC Time now:\n{now}\n\n"
        f"User Query:\n{user_message}"
    )


def _extract_user_from_payload(request: ChatRequest) -> tuple[str, str | None]:
    user_id = request.user_id
    if not user_id:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail="Missing user_id in request payload.",
        )
    return user_id, request.user_name


@app.post("/chat", response_model=ChatResponse)
def chat(request: ChatRequest) -> ChatResponse:
    session_id = request.session_id
    user_id, user_name = _extract_user_from_payload(request)
    wrapped_message = _wrap_user_message(
        request.message,
        user_id,
        user_name,
    )
    resolved_session_id = session_id or "default"
    thread_id = f"{user_id}:{resolved_session_id}"
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
            session_id,
        )
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="Internal server error",
        )

    messages = result.get("messages") if isinstance(result, dict) else None
    if not messages:
        return ChatResponse(message="")

    last_message = messages[-1]
    return ChatResponse(message=last_message.content)
