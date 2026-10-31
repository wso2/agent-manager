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
- **Every task is traced by hand** (`tracing.py`): one trace per A2A operation,
  carrying the protocol details — operation, binding, version, task, context and
  skill — and continuing the caller's W3C trace context, as the A2A spec's
  observability guidance asks. See [Traces](#traces).

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
| **Enable auto instrumentation** | **Off** |

The agent instruments itself (see [Traces](#traces)), so auto instrumentation
stays off. With it off, AMP attaches the env-injection trait instead of the
instrumentation init container, and the trait still supplies the
`AMP_OTEL_ENDPOINT` and `AMP_AGENT_API_KEY` the agent exports its spans with.
Turning it on would add Traceloop's OpenAI spans next to the agent's own, so
every model call would appear twice.

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
| `OPENAI_MODEL` | `gpt-5.6-luna` | Model used by both skills |
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
  --no-auto-instrumentation \
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

## Traces

The agent emits its own OpenTelemetry spans — no Traceloop, no `amp-instrument`
— and `init_otel()` from `amp-instrumentation` exports them to AMP. One
`SendMessage` is one trace:

```text
A2A SendMessage                     server span: the A2A operation
└── invoke_agent a2a-notes-agent    agent span: the task the executor ran
    └── chat gpt-5.6-luna           LLM span: the OpenAI call
```

`SendStreamingMessage`, `CancelTask` and `SubscribeToTask` get the same server
span. The reads — `GetTask`, `ListTasks`, the push-config and extended-card
operations — are left untraced, so a client polling a task does not fill the
trace list.

**Trace context.** The server span continues the caller's trace when the request
carries a W3C `traceparent` header, which is how the A2A spec's enterprise
guidance asks clients and servers to take part in distributed tracing. A calling
agent that propagates its context sees this agent's work inside its own trace.
Without the header, the server span is the trace's root.

**A2A attributes.** OpenTelemetry has no semantic conventions for A2A yet, so
the protocol details sit under `a2a.*`, named after the spec's fields:

| Attribute | Span | Value |
|---|---|---|
| `a2a.method` | server | The spec's operation name: `SendMessage`, `SendStreamingMessage`, `CancelTask`, `SubscribeToTask` |
| `a2a.protocol.binding` | server | `JSONRPC` or `HTTP+JSON`: the interface the request arrived on |
| `a2a.protocol.version` | server | The `A2A-Version` service parameter |
| `a2a.extensions` | server | The extension URIs the `A2A-Extensions` service parameter requested |
| `a2a.jsonrpc.request.id` | server | The JSON-RPC request `id` (JSON-RPC binding only) |
| `a2a.message.id` | server, agent | The inbound `Message.messageId` |
| `a2a.task.id` | server, agent | The `Task.id` the operation ran or touched |
| `a2a.context.id` | server, agent | The `contextId`; also set as `gen_ai.conversation.id`, so AMP groups a context's tasks as one conversation |
| `a2a.task.state` | server, agent | The task's final `TaskState`, such as `TASK_STATE_COMPLETED` or `TASK_STATE_REJECTED` |
| `a2a.skill.id` | agent | The skill the task ran: `summarize-notes` or `extract-action-items` |

A `TASK_STATE_FAILED` task, or an operation that answers with an A2A error such
as `TaskNotCancelableError`, marks its span as an error.

**The server span's Overview.** Its input is the operation's request and its
output the resulting `Task` (or `Message`), both in the A2A JSON form the wire
uses, so the Console shows exactly what crossed the protocol. A streamed
operation's output is the task as it stands when the stream ends. The span is
marked `traceloop.span.kind=workflow`: AMP reads input and output only for span
kinds it recognises, and an A2A operation wrapping the agent is a workflow.

**GenAI attributes.** The agent and LLM spans follow the OpenTelemetry GenAI
semantic conventions AMP's observer reads: `gen_ai.operation.name`
(`invoke_agent`, `chat`), the agent's name and description, the model,
`gen_ai.input.messages` and `gen_ai.output.messages`, the response ID, finish
reason and token usage.

**Message content.** Like AMP's own instrumentation, the sample records message
content by default, so anyone who can read the agent's traces can read the
notes it was sent. Set `AMP_TRACE_CONTENT=false` when that content may be
sensitive. The spans then redact message text, part content, the system prompt
and exception messages, and keep the spans, their IDs and their states.

**The SDK's own spans are off.** With OpenTelemetry installed, `a2a-sdk` wraps
its internal methods in spans named after the Python method, with no A2A
attributes, and FastAPI adds a request span named after the URL. `main.py`
sets `OTEL_INSTRUMENTATION_A2A_SDK_ENABLED=false` before anything imports `a2a`,
and `app.py` turns FastAPI's tracing off, so the trace holds only the spans
above. Start the agent through `python main.py`, or set that variable yourself
if you run it with `uvicorn app:app`.

## Run it locally

```bash
cd samples/a2a-agent
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt

export OPENAI_API_KEY="<your-openai-key>"
python main.py          # serves on http://localhost:9099
```

Without `AMP_OTEL_ENDPOINT` the agent runs untraced. To send a local run's spans
to AMP, register the agent in the Console and set `AMP_OTEL_ENDPOINT` and
`AMP_AGENT_API_KEY` before starting it.

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
  `create_rest_routes`, `create_agent_card_routes`), and against
  `DefaultRequestHandler`'s operation methods, which `tracing.py` wraps.
  `fastapi` is pinned too: `FastAPI(telemetry=...)`, which turns its request
  spans off, is recent.
- **Tasks live in memory.** `InMemoryTaskStore` means a restart forgets task
  history. Swap in the SDK's database task store if you need tasks to survive;
  the sample is about the protocol, not about durable task storage.
- **Push notifications are declared off** in the card, so the four
  `*PushNotificationConfig` operations answer `FAILED_PRECONDITION` rather than
  pretending to store a webhook. The agent publishes **no extended card**
  either: `extendedAgentCard` is false, and the gateway only serves an extended
  card to a caller whose policy chain authenticated the request.
- **Observability is manual instrumentation.** See [Traces](#traces). The
  `manual-instrumentation-agent` sample covers the remaining span kinds
  (embedding, retriever, rerank, tool) against the same contract.
- **The card the gateway serves advertises the gateway.** The gateway's passthrough
  rewrite points each gateway-fronted A2A 1.0 `supportedInterfaces[].url` at
  itself (unfronted entries and `securitySchemes` are left as-is), so resolve the card
  through the gateway and use the URLs it returns.

## File guide

| File | Role |
|---|---|
| `agent.py` | The agent: the OpenAI calls, the two skills, and the task events they publish |
| `app.py` | The A2A server: agent card, JSON-RPC and HTTP+JSON routes, one handler and task store behind both |
| `tracing.py` | The manual instrumentation: the A2A server span, trace-context propagation, and the agent and LLM spans |
| `main.py` | Local runner (`python main.py`), and the deployed start command; switches the SDK's own spans off |
| `requirements.txt` | `a2a-sdk[fastapi]`, `amp-instrumentation`, `fastapi`, `openai`, `opentelemetry-api`, `uvicorn`, `python-dotenv` |
| `.env.example` | Environment variable template for local runs |
