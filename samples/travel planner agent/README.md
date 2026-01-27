# Travel Planner Agent

Minimal Python + React stack for the travel planner agent.

- **AI Agent (BFF)**: `backend/agent/`
- **Booking API**: `backend/booking_api/`
- **Frontend**: `frontend/`
- **Policy ingest (optional)**: `resources/ingest/`
- **Sample policy PDFs**: `resources/policy_pdfs/`

## Prerequisites
- Python 3.10+
- Node.js 22+
- Pinecone index (optional, for hotel policy retrieval)
- Mock hotel dataset (local file)

## Quick Start (local)

### 1) Start the AI agent (BFF)
Create `backend/agent/.env` from `backend/agent/.env.example`, then:

```bash
cd backend/agent
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
uvicorn app:app --host 0.0.0.0 --port 9090
```

### 2) Start the booking API
```bash
cd backend/booking_api
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
uvicorn booking_api:app --host 0.0.0.0 --port 9091
```

### 3) Start the frontend
Create `frontend/.env` as needed (see `frontend/README.md`), then:

```bash
cd frontend
npm install
npm start
```

## Optional: Seed Pinecone policies
Populate Pinecone from the sample policies in `resources/policy_pdfs`:

```bash
cd resources/ingest
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python ingest.py
```

## Notes
- `.env` files are intentionally excluded from this repo.
- The agent serves chat at `http://localhost:9090/travelPlanner/chat`.
