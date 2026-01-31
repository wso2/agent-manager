# Hotel Booking Agent

Minimal Python stack for the travel planner agent.

- **AI Agent**: `samples/hotel-booking-agent/agent/`
- **Hotel API**: `samples/hotel-booking-agent/services/hotel_api/`
- **Policy ingest**: `samples/hotel-booking-agent/services/hotel_api/ingest/`
- **Sample policy PDFs**: `samples/hotel-booking-agent/services/hotel_api/resources/policy_pdfs/`

## Quick Start

### Agent Manager deployment
Deploy the agent in your Agent Manager environment (details to be added). The flow below covers the required supporting services:

**Agent Manager**
- Repo URL: `https://github.com/wso2/agent-manager/tree/amp/v0/samples/hotel_booking_agent`
- Language/runtime: Python 3.11
- Run command: `python -m uvicorn app:app --host 0.0.0.0 --port 9090`
- Agent type: Chat API Agent
- Schema path: `openapi.yaml`
- Port: `9090`

**Agent environment variables**
Required:
- `OPENAI_API_KEY`
- `PINECONE_API_KEY`
- `PINECONE_SERVICE_URL`
- `OPENAI_MODEL` 
- `OPENAI_EMBEDDING_MODEL` 
- `WEATHER_API_KEY`
- `WEATHER_API_BASE_URL` (default: `http://api.weatherapi.com/v1`)
- `HOTEL_API_BASE_URL` (default: `http://localhost:9091`)

**Expose the agent endpoint after deploy**
Run this inside the WSO2-AMP dev container to expose the agent on `localhost:9090`:

```bash
kubectl -n dp-default-default-default-ccb66d74 port-forward svc/travel-planner-agent-is 9090:80
```

**Hotel API**
- Runs locally on `http://localhost:9091` when started via `uvicorn`.
- You can also deploy it to a cloud host; just point the agent configuration at the deployed base URL.

**Pinecone policies**
- Create a Pinecone index using your preferred embedding model.
- Set the Pinecone and embedding configurations when deploying or locally running the hotel api 
- Run the ingest to populate the index.

### Local services (Agent + Hotel API)
#### 1) Start the agent (local)
```bash
cd samples/hotel-booking-agent/agent
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python -m uvicorn app:app --host 0.0.0.0 --port 9090
```

#### 2) Start the Hotel API (local)
```bash
cd samples/hotel-booking-agent/services/hotel_api
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python -m uvicorn service:app --host 0.0.0.0 --port 9091
```

### Sample chat request
```bash
curl -s http://localhost:9090/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"Plan a 3-day trip to Tokyo","sessionId":"session_abc123","userId":"user_123","userName":"Traveler"}'
```

## Notes
- The agent serves chat at `http://localhost:9090/chat`.
