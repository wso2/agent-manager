# Hotel Booking Agent

Minimal Python + React stack for the travel planner agent.

- **AI Agent**: `backend/agent/`
- **Booking API**: `backend/booking_api/`
- **Frontend**: `frontend/`
- **Policy ingest**: `resources/ingest/`
- **Sample policy PDFs**: `resources/policy_pdfs/`

## Quick Start

### Agent Manager deployment
Deploy the agent in your Agent Manager environment (details to be added). The flow below covers the required supporting services:

**Agent Manager**
- Repo URL: `https://github.com/wso2/agent-manager/tree/amp/v0/samples/travel_planner_agent`
- Language/runtime: Python 3.11
- Run command: `uvicorn app:app --host 0.0.0.0 --port 9090`
- Agent type: Chat API Agent
- Schema path: `openapi.yaml`
- Port: `9090`

**Agent environment variables**
Required:
- `OPENAI_API_KEY`
- `ASGARDEO_BASE_URL`
- `ASGARDEO_CLIENT_ID`
- `PINECONE_API_KEY`
- `PINECONE_SERVICE_URL`

Optional (defaults are applied if unset):
- `OPENAI_MODEL` (default: `gpt-4o-mini`)
- `OPENAI_EMBEDDING_MODEL` (default: `text-embedding-3-small`)
- `WEATHER_API_KEY`
- `WEATHER_API_BASE_URL` (default: `http://api.weatherapi.com/v1`)
- `BOOKING_API_BASE_URL` (default: `http://localhost:9091`)

**Expose the agent endpoint after deploy**
Run this inside the WSO2-AMP dev container to expose the agent on `localhost:9090`:

```bash
kubectl -n dp-default-default-default-ccb66d74 port-forward svc/travel-planner-agent-is 9090:80
```

**Booking API**
- Runs locally on `http://localhost:9091` when started via `uvicorn`.
- You can also deploy it to a cloud host; just point the agent configuration at the deployed base URL.

**Pinecone policies (required)**
- Create a Pinecone index using your preferred embedding model.
- Set the Pinecone and embedding configuration in `resources/ingest/.env`.
- Run the ingest to populate the index.

### Local services (Booking API + Frontend)
#### 1) Start the booking API (local)
```bash
cd backend/booking_api
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
uvicorn booking_api:app --host 0.0.0.0 --port 9091
```

#### 2) Start the frontend (local)
The frontend runs on `http://localhost:3000`, then:

```bash
cd frontend
npm install
npm start
```

## Seed Pinecone policies (required)
Populate Pinecone from the sample policies in `resources/policy_pdfs`.
Make sure you have created a Pinecone index with your preferred embedding model and set these values in `resources/ingest/.env`:
`PINECONE_SERVICE_URL`, `PINECONE_API_KEY`, `PINECONE_INDEX_NAME`, `OPENAI_API_KEY`, `OPENAI_EMBEDDING_MODEL`, and optional chunk settings.

```bash
cd resources/ingest
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python ingest.py
```

## Notes
- The agent serves chat at `http://localhost:9090/chat`.
