# CrewAI Agent

## Overview

A multi-agent sample built with [CrewAI](https://github.com/crewAIInc/crewAI) and FastAPI. Two agents work together to answer each question: the first answers it using a capital-city lookup tool, and the second rewrites that answer into a single short sentence. The service exposes `POST /chat`.

Because the request flows through two agents and the underlying model, a single call produces a small but complete trace — useful for seeing how a multi-agent CrewAI app looks in your OpenTelemetry backend.

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

You run the agent yourself; Agent Manager registers it and issues its API key. Start Agent Manager with `make up` from the repository root (see [`deployments/standalone`](../../deployments/standalone/README.md)), then:

1. Sign in to the console at <http://localhost:9000> (the port is `AMP_API_PORT` in `deployments/standalone/.env`) as `admin`; the password is `AMP_ADMIN_PASSWORD` in the same file.
2. A new install has no projects, so create one first: click **Add project** (**Create project** on the organization overview), enter a **Name** and click **Create**.
3. Open the project, click **New agent**, enter the name `CrewAI Agent` and click **Register**.
4. The **Setup Agent** panel opens with an API key already generated. Copy it as `AMP_AGENT_API_KEY`; it is shown only once. To change how long it is valid, pick another **Token Duration** and click **Regenerate**.

## Run it

Use an installed supported interpreter: replace `python3.11` below with `python3.12` or `python3.13` as appropriate.

```bash
cd samples/crewai-agent
python3.11 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt amp-instrumentation

export AMP_OTEL_ENDPOINT="http://localhost:4318"   # your own OpenTelemetry collector
export AMP_AGENT_API_KEY="<key-from-the-console>"
export OPENAI_API_KEY="<your-openai-api-key>"

amp-instrument python main.py    # serves on http://localhost:8000
```

Set these as shell variables, not in a `.env` file, so `amp-instrument` can read them. AMP Lite does not store traces, so `AMP_OTEL_ENDPOINT` is the OTLP/HTTP base URL (without `/v1/traces`) of an OpenTelemetry collector you run; see the [user guide](../../docs/user-guide.md). To run it without tracing, leave out `amp-instrument` and the two `AMP_*` variables.

## Try it

```bash
curl -s localhost:8000/chat \
  -H 'content-type: application/json' \
  -d '{"session_id": "s1", "message": "What is the capital of France?"}'
```

AMP Lite does not store traces, so open them in the backend behind your collector. Each trace shows the two agents and the model call for the request.

## Notes

- Pinned to `crewai==1.15.37` for dependency compatibility.
- Each request's `session_id` is attached to its trace as `traceloop.association.properties.session_id`, so a trace backend can group and filter a conversation's traces by it. Runs without `amp-instrument` skip this.
- The app disables CrewAI's hosted tracing and its interactive trace prompt so it runs non-interactively, and it uses the bundled model pricing data to avoid a network fetch on startup.
