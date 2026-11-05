# CrewAI Agent - Deployment Guide

## Overview

A multi-agent sample built with [CrewAI](https://github.com/crewAIInc/crewAI) and FastAPI. Two agents work together to answer each question: the first answers it using a capital-city lookup tool, and the second rewrites that answer into a single short sentence. The service exposes `POST /chat`.

Because the request flows through two agents and the underlying model, a single call produces a small but complete trace — useful for seeing how a multi-agent CrewAI app appears in the Agent Manager console.

## Prerequisites

### Python runtime

Python 3.11, 3.12, or 3.13 is required. The sample pins
`crewai==1.15.34`, which requires Python below 3.14. Its `pdfplumber` dependency
installs Pillow. The sample requires `pillow>=13.0.0,<14`, which needs Python
3.11 or later.

The compatibility checks and heavy instrumentation tests cover this sample on
Python 3.11–3.13. Other samples retain their existing Python 3.10 coverage.

### Required API Keys

- **OpenAI API Key**: for model inference (the agents use `gpt-4o-mini`)

## Deployment Instructions

### Step 1: Access Agent Manager

1. Navigate to the **Default** project
2. Click **"Add Agent"**
3. Select **Platform-Hosted Agent** Card

### Step 2: Configure Agent Details

Fill in the agent creation form with these values:

| Field                 | Value                                     |
| --------------------- | ----------------------------------------- |
| **Display Name**      | `CrewAI Agent`                            |
| **Description**       | `Multi-agent CrewAI sample`               |
| **GitHub Repository** | `https://github.com/wso2/agent-manager`   |
| **Branch**            | `main`                                    |
| **App Path**          | `samples/crewai-agent`                    |
| **Language**          | `Python`                                  |
| **Language Version**  | `3.11`                                    |
| **Start Command**     | `python main.py`                          |
| **Port**              | `8000`                                    |

### Step 3: Select Agent Interface

- Choose **"Chat Agent"** as the agent interface type

### Step 4: Configure Environment Variables

Add the following environment variables in the create form:

```env
OPENAI_API_KEY=<your-openai-api-key>
HOME=/tmp
CREWAI_STORAGE_DIR=/tmp/crewai
```

`HOME` and `CREWAI_STORAGE_DIR` are required. CrewAI writes files under `$HOME` when it is first imported, and with auto-instrumentation enabled that import happens at process startup — before the app runs — while the build container's default `HOME` is read-only. Pointing both at a writable path (`/tmp`) avoids a `Read-only file system: '/nonexistent'` crash and the restart loop it causes.

### Step 5: Deploy the Agent

1. Review all configuration details
2. Click **"Deploy"**
3. Wait for the build to complete

## Testing Your Agent

### Step 1: Navigate to Chat Interface

Click on the **"Try It"** section on the left navigation.

### Step 2: Test Sample Interactions

Try this question in the chat interface:

```text
What is the capital of France?
```

### Step 3: Observe Traces

1. Click on the **"Observability"** tab on the left navigation and select **Traces**
2. Open a trace to see the two agents and the model call for each interaction

## Run Locally

Use an installed supported interpreter: replace `python3.11` below with
`python3.12` or `python3.13` as appropriate.

```bash
cd samples/crewai-agent
python3.11 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt
export OPENAI_API_KEY=<your-openai-api-key>
python main.py    # serves on http://localhost:8000
```

```bash
curl -s localhost:8000/chat \
  -H 'content-type: application/json' \
  -d '{"session_id": "s1", "message": "What is the capital of France?"}'
```

## Notes

- Pinned to `crewai==1.15.34` for dependency compatibility.
- Each request's `session_id` is attached to its trace as `traceloop.association.properties.session_id`, so the Traces page's Conversation column shows it and can filter on it. Local runs without the platform's instrumentation skip this.
- The app disables CrewAI's hosted tracing and its interactive trace prompt so it runs non-interactively, and it uses the bundled model pricing data to avoid a network fetch on startup.
