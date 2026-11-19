# IT Helpdesk Agent

An L1 IT helpdesk agent for the fictional **AcmeCorp**. Employees ask it to reset
passwords, request software, check ticket status, look up outages, and check the IT
team's issue tracker for known problems.

Built with LangGraph (`create_react_agent`) on FastAPI, serving the Agent Manager
chat contract: `POST /chat` on port `8000`.

## What it demonstrates

Nine in-process tools over mock JSON cover the core helpdesk flows. On top of those,
| Capability | Why it's here |
|---|---|
| `USE_MCP` toggle loading tools from an MCP proxy | Chapter 3 connects the agent to GitHub through an Agent Manager MCP proxy |
| `MCP_OAUTH` toggle for AgentID auth to that proxy | The agent mints a `client_credentials` token scoped to the proxy URL and sends it as a bearer token instead of an API key |
| Known-issue triage rules in the system prompt | The agent checks the IT team's issue tracker before opening a ticket, and may only read it |
| Session memory (LangGraph checkpointer) | Multi-turn "verify me, then act" flows work across turns |
| `AGENT_VERSION` echoed in `/health` and every chat response | Makes a rollout or rollback visible from the outside |
| `scripts/seed_traffic.py` | Sends scripted conversations to generate a batch of traffic |

With `USE_MCP=false` (the default) the agent runs entirely on its in-process tools,
which is all Chapters 1 and 2 need.

## Configuration

All configuration is environment variables. Only the LLM credential is required.

| Variable | Default | Purpose |
|---|---|---|
| `OPENAI_API_KEY` | — | Required unless `USE_LLM_PROVIDER=true` |
| `COMPANY_NAME` | `AcmeCorp` | Used in the system prompt |
| `TONE` | `professional and helpful` | Used in the system prompt |
| `ADDITIONAL_GUIDANCE` | *(empty)* | Extra prompt text appended verbatim |
| `MAX_TICKETS_PER_QUERY` | `20` | Caps ticket listings |
| `AGENT_VERSION` | `dev` | Echoed in `/health` and chat responses |
| `USE_LLM_PROVIDER` | `false` | Route through an Agent Manager LLM Service Provider |
| `LLM_PROVIDER_URL` / `LLM_PROVIDER_KEY` | — | From the agent's LLM configuration in the console; required when `USE_LLM_PROVIDER=true` |
| `USE_MCP` | `false` | Load tools from an MCP proxy in addition to the in-process ones |
| `GITHUB_MCP_URL` / `GITHUB_MCP_API_KEY` | — | From the agent's tool configuration for the `GitHub` MCP proxy (**Connect to MCP Server**); required when `USE_MCP=true` |
| `MCP_OAUTH` | `false` | Authenticate to the MCP proxy with the agent's AgentID (OAuth 2.0 bearer token) instead of an API key. Use when the proxy's **Security** tab is set to **OAuth**; `GITHUB_MCP_API_KEY` is then not needed |
| `AMP_AGENTID_CLIENT_ID` / `AMP_AGENTID_CLIENT_SECRET` / `AMP_AGENTID_TOKEN_ENDPOINT` / `AMP_AGENTID_SCOPES` | — | From the agent's **Agent ID → Overview** page; used when `MCP_OAUTH=true`. If they are not set, the agent starts without MCP tools |
| `ISSUE_TRACKER_REPO` | — | `owner/repo` of the IT team's known-issue tracker. Required when `USE_MCP=true`; issue searches are scoped to it |

`LLM_PROVIDER_*`, `GITHUB_*` and `AMP_AGENTID_*` are issued by Agent Manager; you copy them
from the console into the agent's environment. You never paste an upstream OpenAI or
GitHub credential into the agent.

## Register it in Agent Manager

You run the agent yourself; Agent Manager registers it and issues its API key.
Start Agent Manager with `make up` from the repository root
(see [`deployments/standalone`](../../deployments/standalone/README.md)), then:

1. Sign in to the console at <http://localhost:9000> (the port is
   `AMP_API_PORT` in `deployments/standalone/.env`) as `admin`; the password
   is `AMP_ADMIN_PASSWORD` in the same file.
2. A new install has no projects, so create one first: click **Add project**
   (**Create project** on the organization overview), enter a **Name** and
   click **Create**.
3. Open the project, click **New agent**, enter the name
   `IT Helpdesk Agent` and click **Register**.
4. The **Setup Agent** panel opens with an API key already generated. Copy it
   as `AMP_AGENT_API_KEY`; it is shown only once. To change how long it is
   valid, pick another **Token Duration** and click **Regenerate**.

## Run it

```bash
cd samples/it-helpdesk-agent
python -m venv env && source env/bin/activate
pip install -r requirements.txt amp-instrumentation

export AMP_OTEL_ENDPOINT="http://localhost:4318"   # your own OpenTelemetry collector
export AMP_AGENT_API_KEY="<key-from-the-console>"
export OPENAI_API_KEY=sk-...
amp-instrument python main.py
```

Set these as shell variables, not in a `.env` file, so `amp-instrument` can read
them. To run it without tracing, leave out `amp-instrument` and the two `AMP_*`
variables.

AMP Lite does not store traces, so `AMP_OTEL_ENDPOINT` is the OTLP/HTTP base
URL (without `/v1/traces`) of an OpenTelemetry collector you run; see the
[user guide](../../docs/user-guide.md).

Then:

```bash
curl -s localhost:8000/health

curl -s localhost:8000/chat -H 'Content-Type: application/json' -d '{
  "session_id": "demo-1",
  "message": "I need my password reset. alice.chen@acmecorp.com, E-1001."
}'
```

Each chat turn produces a trace in the backend behind your collector; AMP Lite
has no traces page.

Reuse the same `session_id` across calls to continue a conversation. Omit it and
each request starts its own.

## Test data

Mock data lives in `data/` and resets on restart; nothing is persisted.

- **Employees** — `E-1001` … `E-1010`. `E-1004`, `E-1006`, `E-1009`, `E-1010` are
  admin accounts, which the agent must refuse to reset and escalate instead.
- **Policies** — `POL-IT-001` … `POL-IT-008`, covering password resets, software
  access, escalation, privacy, and after-hours support.
- **Tickets**, **software catalog**, **system status** — supporting fixtures.

Useful for exercising the rules: `alice.chen@acmecorp.com` / `E-1001` is a normal
Engineering account; `david.kim@acmecorp.com` / `E-1004` is an admin account.

## Layout

```
agent.py               LLM binding, system prompt, MCP tool loading, checkpointer
agent_identity.py      AgentID token minting for MCP_OAUTH=true
app.py                 FastAPI app — /chat and /health
config.py              Environment configuration
main.py                Entrypoint (python main.py)
tools.py               The nine in-process tools
clients/               Mock backends behind the tools
data/                  Mock JSON fixtures
scripts/seed_traffic.py  Scripted conversations for seeding traces
```
