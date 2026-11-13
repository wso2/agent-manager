import os

# Keep CrewAI non-interactive and offline-friendly, and set these BEFORE
# importing crewai (they are read at import time): no hosted-trace upload, no
# interactive trace prompt, and use the bundled model pricing data instead of
# fetching it over the network on startup.
os.environ.setdefault("CREWAI_TRACING_ENABLED", "false")
os.environ.setdefault("CREWAI_DISABLE_TRACING_PROMPT", "true")
os.environ.setdefault("LITELLM_LOCAL_MODEL_COST_MAP", "True")
# CrewAI writes under $HOME at import (a storage dir) and at Crew() construction
# (a credentials dir). This block points both at /tmp when HOME is read-only
# (as in a container); it no-ops when HOME is already writable or preset.
os.environ.setdefault("CREWAI_STORAGE_DIR", "/tmp/crewai")
if not os.access(os.path.expanduser("~"), os.W_OK):
    os.environ["HOME"] = "/tmp"

import dotenv
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from agent.crew import create_crew

# Present when the agent runs under amp-instrument; absent in a plain run.
try:
    from traceloop.sdk import Traceloop
except ImportError:
    Traceloop = None

app = FastAPI()
# Load environment variables from a .env file, if present.
dotenv.load_dotenv()
crew = create_crew()


class ChatRequest(BaseModel):
    session_id: str
    message: str


# Sync `def` (not `async`): crew.kickoff() is blocking, so FastAPI runs this in
# a threadpool instead of stalling the event loop. The Pydantic model gives a
# 422 (not an opaque 500) when `message` is missing, matching openapi.yaml.
@app.post("/chat")
def chat(payload: ChatRequest):
    # Tags every span the crew starts with the session, so the Traces page can
    # group a conversation's traces. It is set in this request's copy of the
    # context, so it doesn't carry over to other requests.
    if Traceloop is not None:
        Traceloop.set_association_properties({"session_id": payload.session_id})
    result = crew.kickoff(inputs={"question": payload.message})
    return JSONResponse(content={"response": str(result)})
