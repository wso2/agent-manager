# Insurance Support Agent

An end-to-end sample: a Strands agent deployed on WSO2 Agent Manager, plus a
browser UI that logs a customer in with OAuth2 and chats with the agent through
the gateway.

The agent works on its own — deploy it and chat from **Try It**. The login flow
is an optional second step, and the UI runs with or without it.

The browser client is a separate app with its own README — see [`web/`](web/README.md).

## What is in here

| Path                       | Purpose                                                                  |
| -------------------------- | ------------------------------------------------------------------------ |
| `agent/main.py`, `app.py`  | FastAPI service exposing `POST /chat`, port 8000 (override `PORT`)       |
| `agent/agent.py`           | Strands agent with OpenAI or Bedrock gateway model wiring                |
| `agent/tools.py`           | Five tools: list policies, list claims, lookup, claim status, file claim |
| `agent/mcp_client.py`      | AgentID OAuth client for the remote policy tools (`USE_MCP=true`)        |
| `agent/data.py`            | In-memory policies and claims — no database to set up                    |
| `agent/system_prompt.py`   | The agent's instructions. Edit this to change behaviour.                 |
| `web/`                     | React + TypeScript chat client. See [`web/README.md`](web/README.md).    |
| `mcp-server/`              | Read-only policy MCP server. See [MCP-TOOLS.md](MCP-TOOLS.md).           |
| `ecs/`                     | Fargate task definitions for the agent and MCP server                    |
| `tests/`                   | Offline Bedrock adapter and MCP tests                                    |

The agent handles five things: list the customer's policies, list their claims,
show the full cover on one policy, check a claim's status, and open a new claim.
It will say so if you ask "what can you do?".

Sample data (all Ada Lovelace): policies `OZ-AUTO-4417` (motor), `OZ-HOME-2280`
(home and contents), `OZ-TRAV-9153` (travel); claims `CLM-10432` (in review) and
`CLM-10876` (settled).

## External AWS deployment

For an instrumented ECS container, see [Run on Amazon ECS](ECS-EXTERNAL.md).
To use Amazon Bedrock through the Agent Manager AI gateway, see
[Bedrock gateway configuration](BEDROCK-GATEWAY.md). OpenAI remains the default.

For a supplied read-only insurance MCP server and AgentID OAuth client, follow
[Connect the insurance MCP tools](MCP-TOOLS.md). This mode requires configuration
and deployment, not custom server or client code.

## Prerequisites

- An OpenAI API key
- Python 3.11+ to run the agent locally

## Run locally

```bash
cd agent
python3.11 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export OPENAI_API_KEY=<your-key>
export CORS_ALLOW_ORIGINS="http://localhost:5173"
python main.py
```


```bash
curl -X POST http://localhost:8000/chat \
  -H 'Content-Type: application/json' \
  -d '{"session_id":"s1","message":"What policies do I have?"}'
```

Then run the web client against it — see [`web/README.md`](web/README.md) for
setup.

## Environment variables

### Agent

| Variable             | Required | Default          | Purpose                                                                  |
| -------------------- | -------- | ---------------- | ------------------------------------------------------------------------ |
| `OPENAI_API_KEY`     | yes      | —                | Model credential                                                         |
| `PORT`               | yes       | `8000`           | Override if required only;                 |
| `CORS_ALLOW_ORIGINS` | no       | —                | Comma-separated origins. Local testing only — see [CORS](#cors)          |
| `OPENAI_BASE_URL`    | no       | OpenAI's default | Any OpenAI-compatible endpoint — an AM LLM provider, a proxy             |
| `OPENAI_MODEL`       | no       | `gpt-4o-mini`    | Model id                                                                 |
| `COMPANY_NAME`       | no       | `O2 Insurance`   | Used in the system prompt and the UI header                              |

Bedrock gateway and MCP variables (`MODEL_PROVIDER`, `LLM_GATEWAY_URL`,
`USE_MCP`, `AMP_AGENTID_*`, and others) are listed in
[BEDROCK-GATEWAY.md](BEDROCK-GATEWAY.md) and [MCP-TOOLS.md](MCP-TOOLS.md).
With `MODEL_PROVIDER=bedrock-gateway`, `OPENAI_API_KEY` is not required.

Set `OPENAI_BASE_URL` when routing model calls through a gateway; the key you
supply in `OPENAI_API_KEY` is then the gateway's key.

## CORS

A browser calling the agent from another origin needs CORS headers. Where they
come from depends on how the agent is reached:

- **Deployed behind the gateway** — configure it in Agent Manager. Open the
  agent, go to its CORS settings, and add `http://localhost:5173` (or whatever
  origin serves the client) to the allowed origins. Leave
  `CORS_ALLOW_ORIGINS` unset on the agent: the gateway already sends the
  headers, and two sets of them make browsers reject the response.
- **Running the agent directly on your machine** — there is no gateway in the
  path, so set `CORS_ALLOW_ORIGINS=http://localhost:5173` on the agent instead.

## 1. Deploy in Agent Manager

### Step 1: Create the agent

1. Navigate to a project
2. Select the **Platform-Hosted Agent** card
3. Pick **Source Code** as the source type

### Step 2: Configure agent details

| Field                 | Value                                            |
| --------------------- | ------------------------------------------------ |
| **Display Name**      | `Insurance Support Agent`                        |
| **Description**       | `Customer support agent for policies and claims` |
| **GitHub Repository** | `https://github.com/wso2/agent-manager`          |
| **Branch**            | `main`                                           |
| **App Path**          | `/samples/insurance-support-agent/agent`               |
| **Language**          | `Python`                                         |
| **Language Version**  | `3.11`                                           |
| **Start Command**     | `python main.py`                                 |

### Step 3: Select the agent interface

Choose **Chat Agent**.

### Step 4: Environment variables

```env
OPENAI_API_KEY=<your-openai-api-key>
PORT=8000
```

### Step 5: Deploy

Review and click **Deploy**. The build takes roughly 6-10 minutes.

## 2. Invoke the agent

Use **Try It** in the left navigation, or point the web client at the deployed
agent — see [`web/README.md`](web/README.md). To call it from the browser, add
the client's origin to the agent's CORS settings first — see [CORS](#cors).

## Known limits

The agent keys conversation history on the caller-supplied `session_id`, not
the authenticated subject — the gateway checks the token, but the agent never
reads it, so it can't tell one signed-in customer from another. Anyone holding
a valid token who knows a `session_id` can resume that conversation. Sessions
also live in process memory, capped at 500 with the oldest evicted first —
fine for a demo, not for a real deployment. Both should be fixed (key on the
authenticated subject, move storage out of process) before this goes further
than a sample. See [`web/README.md`](web/README.md#known-limits) for the
client-side implications.

## Observability

Leave **auto instrumentation** enabled (it is on by default). The platform's
instrumentation installs a global OpenTelemetry tracer provider, and the Strands
agent emits its agent, event-loop and tool spans into it — so this sample needs
no tracing code and does not install `strands-agents[otel]`.

Two details matter if you adapt this sample:

- `agent/app.py` sets `OTEL_SEMCONV_STABILITY_OPT_IN=gen_ai_latest_experimental`
  before importing Strands. Without it, tool input and output are not recorded
  as span attributes and the trace view shows tool spans without their data.
- `agent/requirements.txt` pins `wrapt==1.17.3`. Strands otherwise resolves
  wrapt 2.x, which removed an argument that the platform's auto-instrumentation
  still uses.

Disabling auto instrumentation removes the OpenTelemetry SDK from the image
entirely, so the agent would then need to install and configure its own
exporter.
