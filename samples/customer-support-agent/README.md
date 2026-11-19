# Customer Support Agent

## Overview

The Customer Support Agent is an AI-powered customer service assistant that helps users with travel-related inquiries including flights, bookings, hotels, and car rentals. Built with LangGraph and FastAPI, this agent can search for information and simulate travel bookings.

**Based on**: [LangGraph Customer Support Tutorial](https://langchain-ai.github.io/langgraph/tutorials/customer-support/customer-support/#example-conversation)

## Prerequisites

You need:

- **OpenAI API key** for the chat model.
- Python 3.10 or later.
- **Optional Tavily API key** for public web searches, such as travel advisories and destination information.

Flights, hotels, rentals, excursions, and ticket operations use fictional
in-memory data. Booking changes belong to the current chat session and reset
when the service restarts. The app needs no database.

## Register it in Agent Manager

You run the agent yourself; Agent Manager registers it and issues its API key. Start Agent Manager with `make up` from the repository root (see [`deployments/standalone`](../../deployments/standalone/README.md)), then:

1. Sign in to the console at <http://localhost:9000> (the port is `AMP_API_PORT` in `deployments/standalone/.env`) as `admin`; the password is `AMP_ADMIN_PASSWORD` in the same file.
2. A new install has no projects, so create one first: click **Add project** (**Create project** on the organization overview), enter a **Name** and click **Create**.
3. Open the project, click **New agent**, enter the name `Support Agent` and click **Register**.
4. The **Setup Agent** panel opens with an API key already generated. Copy it as `AMP_AGENT_API_KEY`; it is shown only once. To change how long it is valid, pick another **Token Duration** and click **Regenerate**.

## Run it

```bash
cd samples/customer-support-agent
python3.11 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt amp-instrumentation

export AMP_OTEL_ENDPOINT="http://localhost:4318"   # your own OpenTelemetry collector
export AMP_AGENT_API_KEY="<key-from-the-console>"
export OPENAI_API_KEY="<your-openai-api-key>"
export TAVILY_API_KEY="<your-tavily-api-key>"   # optional

amp-instrument python main.py    # serves on http://localhost:8000
```

Set these as shell variables, not in a `.env` file, so `amp-instrument` can read them. AMP Lite does not store traces, so `AMP_OTEL_ENDPOINT` is the OTLP/HTTP base URL (without `/v1/traces`) of an OpenTelemetry collector you run; see the [user guide](../../docs/user-guide.md). To run it without tracing, leave out `amp-instrument` and the two `AMP_*` variables.

## Try it

```bash
curl -s localhost:8000/chat \
  -H 'content-type: application/json' \
  -d '{"session_id": "s1", "message": "What demo flights do I have booked?"}'
```

Reuse the same `session_id` for follow-ups. Other questions to try:

**Flight Inquiries:**

```text
What demo flights do I have booked?
Find an alternate flight from Zurich (ZRH) to Geneva (GVA).
Move my demo ticket to flight DEMO102.
Cancel my demo ticket.
```

**Hotel Search:**

```text
Find a demo hotel in Geneva for next week.
```

**Public web search (with Tavily configured):**

```text
Search the web for current public transport options from Geneva airport to the city center.
```

## Observe the traces

AMP Lite does not store traces, so the console has no traces page. Open them in the backend behind your collector: each chat turn shows the LangGraph run, the model calls and the tool calls.
