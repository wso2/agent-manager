"""FastAPI entrypoint for the manual-instrumentation sample agent.

Implements the AM chat-agent contract: ``POST /chat`` on port 8000 accepting
``{session_id, message}`` and returning ``{response, session_id}``.
``GET /health`` is provided for local checks.

``init_otel()`` is the only AMP-specific setup on the manual path — there is no
Traceloop SDK and no ``amp-instrument`` CLI here. It configures the OpenTelemetry
OTLP exporter to AMP; the agent emits its own spans (see ``instrumentation.py``).
"""

from __future__ import annotations

import logging

from amp_instrumentation import init_otel
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel

from agent import run_agent

logging.basicConfig(level=logging.INFO)
log = logging.getLogger("manual-instrumentation-agent")

# Configure the OTLP exporter to AMP. Reads AMP_OTEL_ENDPOINT and
# AMP_AGENT_API_KEY from the environment:
#   * platform-hosted (auto-instrumentation OFF) — AMP's env-injection trait sets both
#   * externally-hosted — you set both yourself
# init_otel() raises ValueError if either is missing, and is idempotent.
init_otel()

app = FastAPI(title="Manual Instrumentation Sample Agent")


class ChatRequest(BaseModel):
    session_id: str
    message: str
    # Optional W3C-baggage correlation — only set when an evaluation run drives the agent.
    task_id: str | None = None
    trial_id: str | None = None


class ChatResponse(BaseModel):
    response: str
    session_id: str


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/chat", response_model=ChatResponse)
def chat(req: ChatRequest) -> ChatResponse:
    if not req.message.strip():
        raise HTTPException(status_code=400, detail="message must not be empty")
    try:
        answer = run_agent(
            req.message,
            conversation_id=req.session_id,
            task_id=req.task_id,
            trial_id=req.trial_id,
        )
    except Exception as exc:  # noqa: BLE001 - surface agent failures as HTTP 500
        log.exception("agent run failed")
        raise HTTPException(status_code=500, detail=str(exc)) from exc
    return ChatResponse(response=answer, session_id=req.session_id)
