from __future__ import annotations

from datetime import datetime, timezone
import logging

from fastapi import FastAPI, HTTPException, status
from fastapi.middleware.cors import CORSMiddleware
from langchain_core.messages import HumanMessage
from pydantic import BaseModel
from typing import Literal
import re

from config import Settings
from graph import build_graph

_root_logger = logging.getLogger()
if not _root_logger.handlers:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )
else:
    _root_logger.setLevel(logging.INFO)

settings = Settings.from_env()
agent_graph = build_graph(settings)

class ChatRequest(BaseModel):
    message: str
    sessionId: str | None = None
    userId: str | None = None
    userName: str | None = None


class ChatResponse(BaseModel):
    message: str


app = FastAPI(title="Travel Planner Agent")
app.add_middleware(
    CORSMiddleware,
    allow_origins=["http://localhost:3001"],
    allow_credentials=True,
    allow_methods=["GET", "POST", "PUT", "DELETE", "OPTIONS"],
    allow_headers=["Content-Type", "Authorization", "Accept", "x-user-id"],
    max_age=84900,
)


def _wrap_user_message(user_message: str, user_id: str, user_name: str | None) -> str:
    now = datetime.now(timezone.utc).isoformat()
    resolved_user_id = user_id
    resolved_user_name = user_name or "Traveler"
    return (
        f"User Name: {resolved_user_name}\n"
        f"User Context (non-hotel identifiers): {resolved_user_name} ({resolved_user_id})\n"
        f"UTC Time now:\n{now}\n\n"
        f"User Query:\n{user_message}"
    )


@app.post("/travelPlanner/chat", response_model=ChatResponse)
def chat(request: ChatRequest) -> ChatResponse:
    session_id = request.sessionId or "default"
    if not request.userId:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="userId is required.",
        )
    user_id = request.userId
    wrapped_message = _wrap_user_message(
        request.message,
        user_id,
        request.userName,
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
