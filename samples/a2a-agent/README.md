# A2A Notes Agent

## Overview

An agent that speaks the [A2A protocol](https://a2a-protocol.org/) and sends its
traces to AMP as an external agent. Give it meeting notes and it does one of two
things, both backed by OpenAI:

| Skill | What it does | Result |
|---|---|---|
| `summarize-notes` | Condenses the notes into at most three sentences | A `text/plain` artifact, streamed as it is written |
| `extract-action-items` | Pulls out who owes what, and when | An `application/json` artifact |

It is built on the official [`a2a-sdk`](https://github.com/a2aproject/a2a-python)
and FastAPI. The point of the sample is the protocol surface: an agent card, two
transports, the task lifecycle, and streaming.

## What this demonstrates

An A2A agent is not called like a chat agent. There is no `/chat` endpoint. A client resolves the agent
card, picks a transport, and sends messages; the agent answers with a task.

```text
client                                                 agent
  |  GET /.well-known/agent-card.json                     |
  |------------------------------------------------------>|  name, interfaces, skills
  |                                                       |
  |  POST /rpc          SendMessage                       |
  |  POST /rest/message:send                              |
  |------------------------------------------------------>|  Task: submitted -> working
  |                                                       |        -> completed
  |<------------------------------------------------------|  artifacts: summary.txt /
  |                                                       |             action-items.json
  |  POST /rpc          GetTask  /  CancelTask            |
  |------------------------------------------------------>|
```

What that means in code:

- **The card** (`/.well-known/agent-card.json`) declares both interfaces at
  `protocolBinding: JSONRPC` under `/rpc` and `protocolBinding: HTTP+JSON` under
  `/rest`, plus the two skills.
- **One process serves both transports** (`app.py`), over one request handler and
  one task store, so a task sent over JSON-RPC is readable over HTTP+JSON.
- **Streaming works for free** (`SendStreamingMessage`, `/message:stream`): the
  executor publishes artifact chunks, and the SDK renders them either as an SSE
  stream or as one merged artifact, depending on how the client asked.
- **The task lifecycle is explicit**: `submitted -> working -> completed`, or
  `rejected` when there are no notes to work on and `failed` when the model call
  fails. Task state is readable by id afterwards.

## Prerequisites

- Python 3.10 to 3.13. The steps below use 3.11.
- An OpenAI API key. Both skills call the model.
- Agent Manager running. From the repository root, `make up` starts it (see
  [`deployments/standalone`](../../deployments/standalone/README.md)).

## Register it in Agent Manager

You run the agent yourself; Agent Manager registers it and issues the token its
traces are sent with.

1. Sign in to the console at <http://localhost:3000> as `admin` (the password is
   `AMP_ADMIN_PASSWORD` in `deployments/standalone/.env`).
2. Open the **Default** project, click **Add Agent**, and register an agent
   named `A2A Notes Agent`.
3. In the **Set up agent** panel that opens, choose **Python**, pick a token
   duration and click **Generate**. Copy the `AMP_OTEL_ENDPOINT` and
   `AMP_AGENT_API_KEY` values it shows.

## Run it

```bash
cd samples/a2a-agent
python3.11 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt amp-instrumentation

export AMP_OTEL_ENDPOINT="http://default-default.gateway.localhost:19080/otel"
export AMP_AGENT_API_KEY="<key-from-the-console>"
export OPENAI_API_KEY="<your-openai-key>"

amp-instrument python main.py    # serves on http://localhost:9099
```

Set the `AMP_*` values as shell variables, not in `.env`, so `amp-instrument`
can read them. To run it without tracing, leave out `amp-instrument` and the two
`AMP_*` variables.

Tracing works with this agent only because `requirements.txt` pins `a2a-sdk`
1.1.5 or newer. `amp-instrumentation` installs into the same environment and
brings `protobuf` 7.x, and SDK releases 1.1.2 through 1.1.4 both required
`protobuf<7` and read `FieldDescriptor.label`, which protobuf 7 removed. An
agent on one of those fails every A2A message with JSON-RPC `-32603` and this
in its logs:

```text
a2a/utils/proto_utils.py:217: AttributeError: 'google._upb._message.FieldDescriptor' object has no attribute 'label'
```

## Call it

The agent card:

```bash
curl -s http://localhost:9099/.well-known/agent-card.json
```

Summarize notes over JSON-RPC:

```bash
curl -s -X POST http://localhost:9099/rpc \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "SendMessage",
    "params": {
      "message": {
        "messageId": "msg-1",
        "role": "ROLE_USER",
        "parts": [{"text": "Standup: Priya cuts the release branch Thursday; Sam is blocked on the staging certificate."}]
      }
    }
  }'
```

The reply is a `Task` in a terminal state carrying the `summary.txt` artifact.
Ask for the other skill by naming it in the message metadata, or by starting the
text with `Extract action items:` (`Summarize:` works the same way for the
default skill). A `metadata.skill` the agent does not have is rejected rather
than answered with a summary.

Extract action items over HTTP+JSON, choosing the skill through metadata:

```bash
curl -s -X POST http://localhost:9099/rest/message:send \
  -H "Content-Type: application/json" \
  -d '{
    "message": {
      "messageId": "msg-2",
      "role": "ROLE_USER",
      "metadata": {"skill": "extract-action-items"},
      "parts": [{"text": "Standup: Priya cuts the release branch Thursday; Sam is blocked on the staging certificate."}]
    }
  }'
```

Stream it instead, with `-N` so `curl` does not buffer:

```bash
curl -s -N -X POST http://localhost:9099/rpc \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "SendStreamingMessage",
    "params": {"message": {"messageId": "msg-3", "role": "ROLE_USER",
      "parts": [{"text": "Standup: Priya cuts the release branch Thursday."}]}}
  }'
```

Each event is a line of SSE: first the `Task`, then a `working` status update,
then the summary arriving as artifact chunks, then `completed`.

## Call it from an A2A client

The same protocol, without hand-written JSON. This is the `a2a-sdk` client
talking to the running sample.

```python
import asyncio

from a2a.client import ClientConfig, create_client
from a2a.types.a2a_pb2 import Message, Part, Role, SendMessageRequest


async def main() -> None:
    # Resolves /.well-known/agent-card.json, then picks a transport from the card.
    # ClientConfig(supported_protocol_bindings=["HTTP+JSON"]) forces the other one.
    client = await create_client("http://localhost:9099", ClientConfig())
    message = Message(
        message_id="client-1",
        role=Role.ROLE_USER,
        parts=[Part(text="...notes...")],
    )
    message.metadata.update({"skill": "extract-action-items"})
    async for event in client.send_message(SendMessageRequest(message=message)):
        print(event)


asyncio.run(main())
```

## Notes

- **`A2A-Version: 1.0` is defaulted, not required.** A request without the header
  is read as A2A 0.3 by the SDK, which this agent does not serve. Clients that
  talk to the agent directly may omit it, and a proxy in front of the agent may
  drop it, so `app.py` stamps the version the agent serves when a request
  arrives without one.
- **`a2a-sdk` is pinned to `1.1.5`.** The sample is written against that
  generation of the SDK's routing helpers (`create_jsonrpc_routes`,
  `create_rest_routes`, `create_agent_card_routes`), and 1.1.5 is the first
  release that accepts `protobuf` 7.x — the reason it can run under
  `amp-instrument` (see [Run it](#run-it)).
- **Tasks live in memory.** `InMemoryTaskStore` means a restart forgets task
  history. Swap in the SDK's database task store if you need tasks to survive;
  the sample is about the protocol, not about durable task storage.
- **Push notifications are declared off** in the card, so the four
  `*PushNotificationConfig` operations answer `FAILED_PRECONDITION` rather than
  pretending to store a webhook. The agent publishes **no extended card**
  either: `extendedAgentCard` is false.
- **Observability comes from `amp-instrument`.** Run under it, the OpenAI
  calls emit traces to AMP. An agent pinned to `a2a-sdk` below 1.1.5 has to give
  that up; point `amp-instrumentation` at the exporter yourself in that case —
  see the `manual-instrumentation-agent` sample for that path.
- **Set `AGENT_PUBLIC_BASE_URL` when clients reach the agent elsewhere.** The
  card advertises `http://localhost:9099` by default, and A2A clients dial the
  URLs in the card.

## File guide

| File | Role |
|---|---|
| `agent.py` | The agent: the OpenAI calls, the two skills, and the task events they publish |
| `app.py` | The A2A server: agent card, JSON-RPC and HTTP+JSON routes, one handler and task store behind both |
| `main.py` | Runner (`python main.py`) |
| `requirements.txt` | `a2a-sdk[fastapi]`, `openai`, `uvicorn`, `python-dotenv` |
| `.env.example` | Environment variable template |
