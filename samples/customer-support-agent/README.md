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

You run the agent yourself; Agent Manager registers it and issues the token its traces are sent with. Start Agent Manager with `make up` from the repository root (see [`deployments/standalone`](../../deployments/standalone/README.md)), then:

1. Sign in to the console at <http://localhost:3000> as `admin` (the password is `AMP_ADMIN_PASSWORD` in `deployments/standalone/.env`).
2. Open the **Default** project, click **Add Agent**, and register an agent named `Support Agent`.
3. In the **Set up agent** panel that opens, choose **Python**, pick a token duration and click **Generate**. Copy the `AMP_OTEL_ENDPOINT` and `AMP_AGENT_API_KEY` values it shows.

## Run it

```bash
cd samples/customer-support-agent
python3.11 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt amp-instrumentation

export AMP_OTEL_ENDPOINT="http://default-default.gateway.localhost:19080/otel"
export AMP_AGENT_API_KEY="<key-from-the-console>"
export OPENAI_API_KEY="<your-openai-api-key>"
export TAVILY_API_KEY="<your-tavily-api-key>"   # optional

amp-instrument python main.py    # serves on http://localhost:8000
```

Set these as shell variables, not in a `.env` file, so `amp-instrument` can read them. To run it without tracing, leave out `amp-instrument` and the two `AMP_*` variables.

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

Open the agent in the console and go to **Observability → Traces**. Each chat turn shows the LangGraph run, the model calls and the tool calls.
