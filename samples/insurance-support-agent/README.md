# Insurance Support Agent

An end-to-end sample: a Strands agent that sends its traces to WSO2 Agent
Manager as an external agent, plus a browser UI that chats with it and can log a
customer in with OAuth2.

The agent works on its own — run it and call `/chat`. The UI is an optional
second step, and it runs with or without a login.

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

## Running on AWS

For an instrumented ECS container, see [Run on Amazon ECS](ECS-EXTERNAL.md).
To use Amazon Bedrock through the Agent Manager AI gateway, see
[Bedrock gateway configuration](BEDROCK-GATEWAY.md). OpenAI remains the default.

For a supplied read-only insurance MCP server and AgentID OAuth client, follow
[Connect the insurance MCP tools](MCP-TOOLS.md). This mode requires configuration
and hosting the server, not custom server or client code.

## Prerequisites

- An OpenAI API key
- Python 3.11+
- Agent Manager running. From the repository root, `make up` starts it (see
  [`deployments/standalone`](../../deployments/standalone/README.md)).

## Register it in Agent Manager

You run the agent yourself; Agent Manager registers it and issues the token its
traces are sent with.

1. Sign in to the console at <http://localhost:3000> as `admin` (the password is
   `THUNDER_ADMIN_PASSWORD` in `deployments/standalone/.env`).
2. Open the **Default** project, click **Add Agent**, and register an agent
   named `Insurance Support Agent`.
3. In the **Set up agent** panel that opens, choose **Python**, pick a token
   duration and click **Generate**. Copy the `AMP_OTEL_ENDPOINT` and
   `AMP_AGENT_API_KEY` values it shows.

## Run it

```bash
cd samples/insurance-support-agent/agent
python3.11 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt amp-instrumentation==0.4.1

export AMP_OTEL_ENDPOINT="http://default-default.gateway.localhost:19080/otel"
export AMP_AGENT_API_KEY="<key-from-the-console>"
export OPENAI_API_KEY=<your-key>
export CORS_ALLOW_ORIGINS="http://localhost:5173"

amp-instrument python main.py
```

Set the `AMP_*` values as shell variables, not in `.env`, so `amp-instrument`
can read them. To run it without tracing, leave out `amp-instrument` and the two
`AMP_*` variables.

Then call it:

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
| `PORT`               | no       | `8000`           | Listening port                                                           |
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

A browser calling the agent from another origin needs CORS headers. With the
agent called directly, set `CORS_ALLOW_ORIGINS=http://localhost:5173` (or
whatever origin serves the client) on the agent. If you put a proxy in front of
the agent that already sends CORS headers, leave `CORS_ALLOW_ORIGINS` unset: two
sets of them make browsers reject the response.

## Known limits

The agent keys conversation history on the caller-supplied `session_id`, not
the authenticated subject — whatever checks the token in front of the agent,
the agent never reads it, so it can't tell one signed-in customer from another.
Anyone holding a valid token who knows a `session_id` can resume that conversation. Sessions
also live in process memory, capped at 500 with the oldest evicted first —
fine for a demo, not for production. Both should be fixed (key on the
authenticated subject, move storage out of process) before this goes further
than a sample. See [`web/README.md`](web/README.md#known-limits) for the
client-side implications.

## Observability

Run the agent under `amp-instrument`. It installs a global OpenTelemetry tracer
provider, and the Strands agent emits its agent, event-loop and tool spans into
it — so this sample needs no tracing code and does not install
`strands-agents[otel]`. Each chat turn appears under the agent's
**Observability → Traces** in the console.

Two details matter if you adapt this sample:

- `agent/app.py` sets `OTEL_SEMCONV_STABILITY_OPT_IN=gen_ai_latest_experimental`
  before importing Strands. Without it, tool input and output are not recorded
  as span attributes and the trace view shows tool spans without their data.
- `agent/requirements.txt` pins `wrapt==1.17.3`. Strands otherwise resolves
  wrapt 2.x, which removed an argument that `amp-instrumentation` still uses.
