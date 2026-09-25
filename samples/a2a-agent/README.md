# A2A Notes Agent - Deployment Guide

## Overview

An agent that speaks the [A2A protocol](https://a2a-protocol.org/) and is deployed
through AMP as an **A2A Agent**. Give it meeting notes and it does one of two
things, both backed by OpenAI:

| Skill | What it does | Result |
|---|---|---|
| `summarize-notes` | Condenses the notes into at most three sentences | A `text/plain` artifact, streamed as it is written |
| `extract-action-items` | Pulls out who owes what, and when | An `application/json` artifact |

It is built on the official [`a2a-sdk`](https://github.com/a2aproject/a2a-python)
and FastAPI. The point of the sample is the protocol surface: an agent card, both
of the transports AMP publishes, the task lifecycle, and streaming.

## What this demonstrates

An A2A agent is not called like a chat agent. There is no `/chat` endpoint, and
the Console's chat view has nothing to call here. A client resolves the agent
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
  `/rest`, plus the two skills. These are the paths AMP's gateway publishes an
  A2A agent under, and it rewrites the card it serves so clients dial the
  gateway rather than the agent.
- **One process serves both transports** (`app.py`), over one request handler and
  one task store, so a task sent over JSON-RPC is readable over HTTP+JSON.
- **Streaming works for free** (`SendStreamingMessage`, `/message:stream`): the
  executor publishes artifact chunks, and the SDK renders them either as an SSE
  stream or as one merged artifact, depending on how the client asked. This is
  the traffic AMP's gateway deliberately leaves its route timeout off for.
- **The task lifecycle is explicit**: `submitted -> working -> completed`, or
  `rejected` when there are no notes to work on and `failed` when the model call
  fails. Task state is readable by id afterwards.

## Prerequisites

- Python 3.10 to 3.13. The deployment steps below use 3.11.
- An OpenAI API key. Both skills call the model.

## Deploy it through AMP

### Step 1: Add the agent

1. Open the **Default** project.
2. Click **Add Agent** and select the **Platform-Hosted Agent** card.

### Step 2: Fill in the agent details

| Field | Value |
|---|---|
| **Display Name** | `A2A Notes Agent` |
| **Description** | `Summarizes meeting notes and extracts action items over A2A` |
| **GitHub Repository** | `https://github.com/wso2/agent-manager` |
| **Branch** | `main` |
| **App Path** | `samples/a2a-agent` |
| **Language** | `Python` |
| **Language Version** | `3.11` |
| **Start Command** | `python main.py` |
| **Port** | `9099` |
| **Enable auto instrumentation** | **On** |

Auto instrumentation works with this agent, but only because `requirements.txt`
pins `a2a-sdk` 1.1.5 or newer. AMP's instrumentation init container injects its
own Python packages onto `PYTHONPATH`, including `protobuf` 7.x, and SDK
releases 1.1.2 through 1.1.4 both required `protobuf<7` and read
`FieldDescriptor.label`, which protobuf 7 removed. An agent on one of those
fails every A2A message with JSON-RPC `-32603` and this in its logs:

```text
a2a/utils/proto_utils.py:217: AttributeError: 'google._upb._message.FieldDescriptor' object has no attribute 'label'
```

If you pin an older SDK, turn instrumentation off instead: the agent then runs
on the protobuf its own build installed, at the cost of the OpenAI calls no
longer emitting traces.

### Step 3: Select the agent interface

Choose **A2A Agent**. An A2A agent needs a port and nothing else: it serves its
own agent card, so there is no OpenAPI document and no base path to give it.

The gateway then publishes both A2A transports for it — JSON-RPC under
`/<agent-name>/rpc` and HTTP+JSON under `/<agent-name>/rest` — and serves the
agent's card at the well-known path.

### Step 4: Configure environment variables

| Key | Value |
|---|---|
| `OPENAI_API_KEY` | your OpenAI key (mark it **sensitive**) |

Optional, with defaults:

| Key | Default | Purpose |
|---|---|---|
| `OPENAI_MODEL` | `gpt-4o-mini` | Model used by both skills |
| `AGENT_PUBLIC_BASE_URL` | `http://localhost:9099` | The address the agent advertises in its own card. The gateway rewrites the card it serves, so leave this unset for deployed agents. |

### Step 5: Deploy

Review the configuration, click **Deploy**, and wait for the build to finish.

## Deploy it with amctl

The same deployment without the console. `amctl` needs a build that knows the
`a2a-agent` subtype — check `amctl agent create --help` says so before you start.

```bash
amctl agent create a2a-notes-agent \
  --display-name "A2A Notes Agent" \
  --subtype a2a-agent \
  --port 9099 \
  --repo-url https://github.com/wso2/agent-manager \
  --repo-branch main \
  --repo-path /samples/a2a-agent \
  --build-type buildpack \
  --language python \
  --language-version 3.11 \
  --run-command "python main.py" \
  --env-secret OPENAI_API_KEY=<your-openai-key>

amctl agent build create a2a-notes-agent     # takes a few minutes
amctl agent deploy a2a-notes-agent           # deploys the newest build
amctl agent status a2a-notes-agent           # waits out in-progress -> active
```

`amctl agent status` prints the agent's gateway URL. That is the base the
transports hang off:

```text
http://<gateway-host>/<agent-name>          card and context
http://<gateway-host>/<agent-name>/rpc      JSON-RPC transport
http://<gateway-host>/<agent-name>/rest     HTTP+JSON transport
```

## Call the deployed agent

Calls go to the gateway, not to the agent: `<gateway-url>/<agent-name>/rpc` for
JSON-RPC, `<gateway-url>/<agent-name>/rest/...` for HTTP+JSON. If you turned on
API key security for the agent, send its key in the `X-API-Key` header.

The card the gateway serves at the well-known path is the authority on those
URLs — it is the agent's card with its addresses rewritten to the gateway's —
so resolve it first when you are unsure how a deployment is exposed.

```bash
curl -X POST "https://<gateway-url>/<agent-name>/rpc" \
  -H "Content-Type: application/json" \
  -H "A2A-Version: 1.0" \
  -H "X-API-Key: <agent-api-key>" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "SendMessage",
    "params": {
      "message": {
        "messageId": "msg-1",
        "role": "ROLE_USER",
        "parts": [{"text": "Standup: the gateway rollout is on track. Priya will cut the release branch on Thursday. Sam is blocked on the staging certificate."}]
      }
    }
  }'
```

The reply is a `Task` in a terminal state carrying the `summary.txt` artifact.
Ask for the other skill by naming it in the message metadata, or by starting the
text with `Extract action items:` (`Summarize:` works the same way for the
default skill). A `metadata.skill` the agent does not have is rejected rather
than answered with a summary:

```json
{
  "message": {
    "messageId": "msg-2",
    "role": "ROLE_USER",
    "metadata": {"skill": "extract-action-items"},
    "parts": [{"text": "...the notes..."}]
  }
}
```

Any A2A client works too — see [Call it from an A2A client](#call-it-from-an-a2a-client).

## Run it locally

```bash
cd samples/a2a-agent
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt

export OPENAI_API_KEY="<your-openai-key>"
python main.py          # serves on http://localhost:9099
```

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
talking to a locally running sample — point the URL at the gateway and add an
`X-API-Key` header to call a deployed agent instead.

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
  release that accepts `protobuf` 7.x — the reason auto instrumentation can
  stay on (see Step 2).
- **Tasks live in memory.** `InMemoryTaskStore` means a restart forgets task
  history. Swap in the SDK's database task store if you need tasks to survive;
  the sample is about the protocol, not about durable task storage.
- **Push notifications are declared off** in the card, so the four
  `*PushNotificationConfig` operations answer `FAILED_PRECONDITION` rather than
  pretending to store a webhook. The agent publishes **no extended card**
  either: `extendedAgentCard` is false, and the gateway only serves an extended
  card to a caller whose policy chain authenticated the request.
- **Observability comes from auto instrumentation.** With it on (see Step 2),
  the OpenAI calls emit traces through AMP's instrumentation. An agent pinned
  to `a2a-sdk` below 1.1.5 has to give that up; point `amp-instrumentation` at
  the exporter yourself in that case — see the `manual-instrumentation-agent`
  sample for that path.
- **The card the gateway serves is the agent's own body.** As of this writing the
  gateway's passthrough rewrite does not touch A2A 1.0 `supportedInterfaces`
  URLs, so a card fetched through the gateway still advertises the agent's own
  address. Use the gateway paths above (`/<agent-name>/rpc`, `/rest`) when you
  wire up a client, or resolve the card and rewrite the URL yourself.

## File guide

| File | Role |
|---|---|
| `agent.py` | The agent: the OpenAI calls, the two skills, and the task events they publish |
| `app.py` | The A2A server: agent card, JSON-RPC and HTTP+JSON routes, one handler and task store behind both |
| `main.py` | Local runner (`python main.py`), and the deployed start command |
| `requirements.txt` | `a2a-sdk[fastapi]`, `openai`, `uvicorn`, `python-dotenv` |
| `.env.example` | Environment variable template for local runs |
