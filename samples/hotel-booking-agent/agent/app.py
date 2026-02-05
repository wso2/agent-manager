from __future__ import annotations

from datetime import datetime, timezone
import logging

from fastapi import FastAPI, HTTPException, Request, status
from fastapi.middleware.cors import CORSMiddleware
from langchain_core.messages import HumanMessage
from pydantic import BaseModel

from config import Settings
from graph import build_graph

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)

configs = Settings()
agent_graph = build_graph(configs)

class ChatRequest(BaseModel):
    message: str
    sessionId: str 
    userId: str
    userName: str | None = None


class ChatResponse(BaseModel):
    message: str

app = FastAPI(title="Hotel Booking Agent")
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=False,
    allow_methods=["GET", "POST", "PUT", "DELETE", "OPTIONS"],
    allow_headers=["Content-Type", "Authorization", "Accept", "x-user-id"],
    max_age=84900,
)


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
    user_id = request.userId
    if not user_id:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
            detail="Missing userId in request payload.",
        )
    return user_id, request.userName


@app.post("/chat", response_model=ChatResponse)
def chat(request: ChatRequest, http_request: Request) -> ChatResponse:
    session_id = request.sessionId
    user_id, user_name = _extract_user_from_payload(request)
    wrapped_message = _wrap_user_message(
        request.message,
        user_id,
        user_name,
    )
    thread_id = f"{user_id}:{session_id}"
    result = agent_graph.invoke(
        {"messages": [HumanMessage(content=wrapped_message)]},
        config={
            "recursion_limit": 50,
            "configurable": {"thread_id": thread_id},
        },
    )

    last_message = result["messages"][-1]
    return ChatResponse(message=last_message.content)
