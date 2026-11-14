# CrewAI Agent

## Overview

A multi-agent sample built with [CrewAI](https://github.com/crewAIInc/crewAI) and FastAPI. Two agents work together to answer each question: the first answers it using a capital-city lookup tool, and the second rewrites that answer into a single short sentence. The service exposes `POST /chat`.

Because the request flows through two agents and the underlying model, a single call produces a small but complete trace — useful for seeing how a multi-agent CrewAI app appears in the Agent Manager console.

## Prerequisites

### Python runtime

Python 3.11, 3.12, or 3.13 is required. The sample pins
`crewai==1.15.37`, which requires Python below 3.14. Its `pdfplumber` dependency
installs Pillow. The sample requires `pillow>=13.0.0,<14`, which needs Python
3.11 or later.

The compatibility checks and heavy instrumentation tests cover this sample on
Python 3.11–3.13. Other samples retain their existing Python 3.10 coverage.

### Required API Keys

- **OpenAI API Key**: for model inference (the agents use `gpt-4o-mini`)

## Register it in Agent Manager

You run the agent yourself; Agent Manager registers it and issues the token its traces are sent with. Start Agent Manager with `make up` from the repository root (see [`deployments/standalone`](../../deployments/standalone/README.md)), then:

1. Sign in to the console at <http://localhost:3000> as `admin` (the password is `AMP_ADMIN_PASSWORD` in `deployments/standalone/.env`).
2. Open the **Default** project, click **Add Agent**, and register an agent named `CrewAI Agent`.
3. In the **Set up agent** panel that opens, choose **Python**, pick a token duration and click **Generate**. Copy the `AMP_OTEL_ENDPOINT` and `AMP_AGENT_API_KEY` values it shows.

## Run it

Use an installed supported interpreter: replace `python3.11` below with `python3.12` or `python3.13` as appropriate.

```bash
cd samples/crewai-agent
python3.11 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt amp-instrumentation

export AMP_OTEL_ENDPOINT="http://default-default.gateway.localhost:19080/otel"
export AMP_AGENT_API_KEY="<key-from-the-console>"
export OPENAI_API_KEY="<your-openai-api-key>"

amp-instrument python main.py    # serves on http://localhost:8000
```

Set these as shell variables, not in a `.env` file, so `amp-instrument` can read them. To run it without tracing, leave out `amp-instrument` and the two `AMP_*` variables.

## Try it

```bash
curl -s localhost:8000/chat \
  -H 'content-type: application/json' \
  -d '{"session_id": "s1", "message": "What is the capital of France?"}'
```

Then open the agent in the console and go to **Observability → Traces**. Each trace shows the two agents and the model call for the request.

## Notes

- Pinned to `crewai==1.15.37` for dependency compatibility.
- Each request's `session_id` is attached to its trace as `traceloop.association.properties.session_id`, so the Traces page's Conversation column shows it and can filter on it. Runs without `amp-instrument` skip this.
- The app disables CrewAI's hosted tracing and its interactive trace prompt so it runs non-interactively, and it uses the bundled model pricing data to avoid a network fetch on startup.
