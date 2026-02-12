# Hotel Booking Agent - Deployment Guide

## Overview

The Hotel Booking Agent is an AI-powered assistant that helps users search hotels, check availability, answer policy questions, and create/edit/cancel bookings. It is built with LangGraph and FastAPI.

## Prerequisites

Before deploying this agent, ensure you have:

### Required API Keys

- **OpenAI API Key**: for model inference
- **Pinecone API Key**: for policy retrieval
- **Pinecone Service URL**: for Pinecone host
- **Pinecone Index Name**: name of your Pinecone index

### Supporting Service

- **Hotel API**: the hotel API service must be running (locally or deployed). Set `HOTEL_API_BASE_URL` to point to it.

## Deployment Instructions

### Step 1: Access Agent Manager Platform

1. Navigate to the **Default** project
2. Click **"Add Agent"**
3. Select **Platform-Hosted Agent** Card

### Step 2: Configure Agent Details

Fill in the agent creation form with these values:

| Field                 | Value                                                   |
| --------------------- | ------------------------------------------------------- |
| **Display Name**      | `Hotel Booking Agent`                                   |
| **Description**       | `AI-powered hotel booking assistant`                    |
| **GitHub Repository** | `https://github.com/wso2/ai-agent-management-platform`  |
| **Branch**            | `main`                                                  |
| **App Path**          | `samples/hotel-booking-agent/agent`                     |
| **Language**          | `Python`                                                |
| **Language Version**  | `3.11`                                                  |
| **Start Command**     | `python -m uvicorn app:app --host 0.0.0.0 --port 9090`   |
| **Port**              | `9090`                                                  |
| **OpenAPI Spec Path** | `/openapi.yaml`                                         |

### Step 3: Select Agent Interface

- Choose **"Custom API Agent"** as the agent interface type

### Step 4: Configure Environment Variables

Add the following environment variables in the create form:

```env
OPENAI_API_KEY=<your-openai-api-key>
PINECONE_API_KEY=<your-pinecone-api-key>
PINECONE_SERVICE_URL=<your-pinecone-service-url>
PINECONE_INDEX_NAME=<your-pinecone-index-name>
HOTEL_API_BASE_URL=<your-hotel-api-base-url>
```

Optional (with defaults):

```env
OPENAI_MODEL=gpt-4o-mini
OPENAI_EMBEDDING_MODEL=text-embedding-3-small
WEATHER_API_KEY=
WEATHER_API_BASE_URL=http://api.weatherapi.com/v1
```

### Step 5: Deploy the Agent

1. Review all configuration details
2. Click **"Deploy"**
3. Wait for the build to complete

## Testing Your Agent

### Step 1: Navigate to Chat Interface

Click on the **"Try It"** section on the left navigation.

### Step 2: Test Sample Interactions

Try these sample questions in the chat interface:

**Hotel Search:**

```text
Find hotels in Tokyo for Feb 7 to Feb 8, 2026 for 1 guest.
```

**Availability:**

```text
Check availability for Brooklyn Heights Loft Hotel from Feb 7 to Feb 8, 2026 for 1 guest and 1 room.
```

**Booking:**

```text
Book 1 room at Brooklyn Heights Loft Hotel from Feb 7 to Feb 8, 2026 for 1 guest. Guest: Alex Doe, alex@example.com, +1-555-0100.
```

### Step 3: Observe Traces (Optional)

1. Click on the **"Observability"** tab on left navigation and select **Traces**
2. View traces

## Hotel API (Required Supporting Service)

The Hotel API must be running locally or deployed, and the agent must point to it via `HOTEL_API_BASE_URL`.

### Local Run (Hotel API)
```bash
cd samples/hotel-booking-agent/services/hotel_api
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python -m uvicorn service:app --host 0.0.0.0 --port 9091
```

### Deploy Hotel API
Deploy the hotel API as a separate service, then set:
```env
HOTEL_API_BASE_URL=<deployed-hotel-api-base-url>
```

### Pinecone Policy Ingestion (Optional)
- Create your own Pinecone index using your API key.
- Provide `PINECONE_API_KEY`, `PINECONE_SERVICE_URL`, and `PINECONE_INDEX_NAME` to the Hotel API deployment.
- If Pinecone settings are missing, ingestion is skipped and the Hotel API still runs.
- If the index exists and is empty, ingestion runs; if it already has vectors, ingestion is skipped.
- Policy PDFs live in `samples/hotel-booking-agent/services/hotel_api/resources/policy_pdfs/`.
