from __future__ import annotations

from datetime import datetime, timezone
import logging

from fastapi import FastAPI, HTTPException, Request, status
from fastapi.middleware.cors import CORSMiddleware
from langchain_core.messages import HumanMessage
import jwt
from pydantic import BaseModel
from typing import Any

from config import Settings
from graph import build_graph

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)

configs = Settings.from_env()
agent_graph = build_graph(configs)

class ChatRequest(BaseModel):
    message: str
    sessionId: str | None = None


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
        f"User Name: {resolved_user_name}\n"
        f"User Context (non-hotel identifiers): {resolved_user_name} ({resolved_user_id})\n"
        f"UTC Time now:\n{now}\n\n"
        f"User Query:\n{user_message}"
    )


def _get_bearer_token(request: Request) -> str:
    auth_header = request.headers.get("authorization", "")
    if not auth_header.lower().startswith("bearer "):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Missing or invalid Authorization header.",
        )
    token = auth_header.split(" ", 1)[1].strip()
    if not token:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Missing bearer token.",
        )
    return token


def _decode_access_token(token: str) -> dict[str, Any]:
    return jwt.decode(
        token,
        options={
            "verify_signature": False,
            "verify_aud": False,
            "verify_iss": False,
            "verify_exp": False,
        },
    )


def _extract_user_from_token(request: Request) -> tuple[str, str | None]:
    token = _get_bearer_token(request)
    try:
        claims = _decode_access_token(token)
    except jwt.PyJWTError:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid access token.",
        )
    user_id = claims.get("sub")
    if not user_id:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Access token missing subject.",
        )
    user_name = (
        claims.get("preferred_username")
        or claims.get("given_name")
        or claims.get("name")
        or claims.get("email")
    )
    return user_id, user_name


@app.post("/chat", response_model=ChatResponse)
def chat(request: ChatRequest, http_request: Request) -> ChatResponse:
    session_id = request.sessionId
    user_id, user_name = _extract_user_from_token(http_request)
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
