# Hotel Booking Agent

A LangGraph hotel assistant that searches hotels, checks availability, answers hotel policy questions, and creates, edits, and cancels bookings.

> **This is a multi-service sample.** The agent calls external APIs, and for the full experience you need to run a second service:
>
> - **Hotel API** (`services/hotel_api`): the agent's tools call this FastAPI service over HTTP for hotels, availability, bookings, and policy search. Run it first, somewhere the agent can reach it.
> - **Pinecone API key (optional, recommended)**: the Hotel API can store and search the hotel policy documents in Pinecone. Without a key it uses an in-memory store, which works for trying the sample but is rebuilt on every restart.
> - **External APIs**: OpenAI for the model and embeddings, and optionally Pinecone for policy search and WeatherAPI for forecasts.

## What this demonstrates

- A tool-calling LangGraph agent with 10 tools and per-session conversation memory.
- An agent whose tools are a separate HTTP service; each tool call appears as its own span.
- Retrieval over your own documents: policy PDFs are embedded by the Hotel API and searched per hotel.
- Zero-code tracing with `amp-instrument`.

## How it works

```
user ──POST /chat──▶ Agent (agent/) ──HTTP──▶ Hotel API (services/hotel_api/)
                        │                         ├─ hotels, availability: resources/hotel_data.json
                        │                         ├─ bookings: storage/bookings.json
                        ▼                         └─ policy search: Pinecone, or in-memory
                     OpenAI                           (policy PDFs loaded on startup)
```

- The agent exposes `POST /chat` on port `8000`.
- Every hotel-related tool, including policy questions, calls the Hotel API at `HOTEL_API_BASE_URL`.
- The agent itself only needs an OpenAI key. Pinecone is configured on the Hotel API only.
- Conversation memory is in-process, so it resets when the agent restarts.

## Prerequisites

- Python 3.11.
- An [OpenAI API key](https://platform.openai.com/api-keys).
- Optional but recommended: a Pinecone API key for policy search. Without one, the Hotel API keeps policies in memory. [Create a free Pinecone account](https://app.pinecone.io/), then [create an API key](https://docs.pinecone.io/guides/projects/manage-api-keys). You don't need to create an index; the Hotel API creates `hotel-policies` on first start.
- Optional: a [WeatherAPI](https://www.weatherapi.com/) key for weather forecasts.
- Agent Manager running. From the repository root, `make up` starts it (see [`deployments/standalone`](../../deployments/standalone/README.md)).

## Step 1: Run the Hotel API

```bash
cd samples/hotel-booking-agent/services/hotel_api
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt

export OPENAI_API_KEY="<your-openai-api-key>"
export PINECONE_API_KEY="<your-pinecone-api-key>"  # optional; omit to use the in-memory policy store

python -m uvicorn service:app --host 0.0.0.0 --port 9091
```

With a Pinecone key, the first start creates the `hotel-policies` index and ingests the policy PDFs. Without one, the logs show `policy search ready (in-memory)`. Check it is up with `curl http://localhost:9091/health`.

The agent reaches it at `http://localhost:9091` when both run on the same machine.

> The Hotel API has no authentication. Anyone who can reach it can create or cancel bookings in the sample data, so do not expose it beyond your machine.

## Step 2: Register the agent

You run the agent yourself; Agent Manager registers it and issues the token its traces are sent with.

1. Sign in to the console at <http://localhost:3000> as `admin` (the password is `AMP_ADMIN_PASSWORD` in `deployments/standalone/.env`).
2. Open the **Default** project, click **Add Agent**, and register an agent named `Hotel Booking Agent`.
3. In the **Set up agent** panel that opens, choose **Python**, pick a token duration and click **Generate**. Copy the `AMP_OTEL_ENDPOINT` and `AMP_AGENT_API_KEY` values it shows.

## Step 3: Run the agent

Install the agent with the [AMP instrumentation package](https://wso2.com/agent-platform/docs/latest/guides/amp-instrumentation/) and run it through `amp-instrument`:

```bash
cd samples/hotel-booking-agent/agent
python -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt amp-instrumentation==0.4.1

export AMP_OTEL_ENDPOINT="http://default-default.gateway.localhost:19080/otel"
export AMP_AGENT_API_KEY="<key-from-the-console>"
export OPENAI_API_KEY="<your-openai-api-key>"
export HOTEL_API_BASE_URL="http://localhost:9091"

amp-instrument uvicorn app:app --host 0.0.0.0 --port 8000
```

Set these as shell variables, not in a `.env` file, so `amp-instrument` can read them.

## Step 4: Try it

```bash
curl -X POST http://localhost:8000/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "Find hotels in Tokyo for Feb 7 to Feb 8, 2027 for 1 guest.", "session_id": "demo"}'
```

Sample questions (reuse the same `session_id` for follow-ups):

```text
Find hotels in Tokyo for Feb 7 to Feb 8, 2027 for 1 guest.
Check availability for Brooklyn Heights Loft Hotel from Feb 7 to Feb 8, 2027 for 1 guest and 1 room.
What is the pet policy at The Crimson Tower Tokyo?
Book 1 room at Brooklyn Heights Loft Hotel from Feb 7 to Feb 8, 2027 for 1 guest. Guest: Alex Doe, alex@example.com, +1-555-0100.
```

## Step 5: Observe the traces

Open the agent in the console and go to **Observability → Traces**. Each chat turn shows the LangGraph run, the OpenAI calls, and the tool calls to the Hotel API.

## Configuration reference

### Agent

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `OPENAI_API_KEY` | Yes | | Model calls |
| `HOTEL_API_BASE_URL` | Yes | `http://localhost:9091` | Where the tools call the Hotel API |
| `OPENAI_MODEL` | No | `gpt-4o-mini` | Chat model |
| `WEATHER_API_KEY` | No | | Enables the weather tool |
| `WEATHER_API_BASE_URL` | No | `http://api.weatherapi.com/v1` | WeatherAPI endpoint |

### Hotel API

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `OPENAI_API_KEY` | For policy search | | Embeds the policy documents and queries |
| `PINECONE_API_KEY` | For Pinecone | | Stores policies in Pinecone instead of in memory |
| `PINECONE_INDEX_NAME` | No | `hotel-policies` | Index to create or use |
| `PINECONE_SERVICE_URL` | No | | Use an existing index host directly |
| `OPENAI_EMBEDDING_MODEL` | No | `text-embedding-3-small` | Embedding model |

How the Hotel API picks its policy store:

| Hotel API env | Policy store |
|---|---|
| `OPENAI_API_KEY` + `PINECONE_API_KEY` | Pinecone. The index is created (serverless, AWS `us-east-1`) if missing, and the policies are upserted on every start. |
| `OPENAI_API_KEY` only | In-memory. Rebuilt on every start. |
| No `OPENAI_API_KEY` | None. `GET /hotels/{hotel_id}/policies/search` returns 503 and the agent says policy search is unavailable. Everything else works. |

## Smoke test

`scripts/smoke_test.sh` installs both services, runs the Hotel API logging regression tests, starts the services without API keys, and checks they respond. CI runs it on pull requests that touch this sample.

The Hotel API writes application logs as single-line JSON events, including escaped exception tracebacks. To run the logging tests separately after installing the Hotel API requirements, run this from the sample directory:

```bash
PYTHONPATH=services/hotel_api python -m unittest discover -s services/hotel_api/tests -v
```

## Files

| Path | What it is |
|---|---|
| `agent/app.py` | FastAPI app exposing `POST /chat` |
| `agent/graph.py` | LangGraph graph: model node, tool node, in-memory checkpointer |
| `agent/tools.py` | The agent's tools, mostly HTTP calls to the Hotel API |
| `services/hotel_api/service.py` | Hotel API app |
| `services/hotel_api/search.py`, `booking.py` | Hotel search, availability, and bookings |
| `services/hotel_api/policies.py` | Policy search endpoint and store selection |
| `services/hotel_api/ingest.py` | Loads the policy PDFs and prepares the Pinecone index |
| `services/hotel_api/resources/` | Mock hotel data and policy PDFs |
| `scripts/smoke_test.sh` | Install-and-start check for both services |
