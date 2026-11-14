# Finance Insight Service

## Overview
Finance Insight Service is a stateless, async report generator for financial research and analysis. It combines current market context with deterministic calculations to deliver concise, evidence-aware responses.

## Key Features
- **Evidence-aware research** - Collects relevant financial context before answering
- **Deterministic quant** - All numbers are computed, not guessed
- **Audit + report split** - Validation and reporting are separated for clarity
- **Stateless requests** - Each request runs independently (no chat memory)


## Architecture

The system runs a sequential, agent-based workflow with explicit quality gates:

1. **Research** - Collects relevant financial context and evidence
2. **Quant** - Computes required metrics and scenarios deterministically
3. **Audit** - Validates outputs and flags issues
4. **Report** - Produces the final user-facing response

Request flow (stateless):
- UI submits a request to `/chat/async`
- API server starts a background job and emits progress traces
- UI polls `/chat/async/<job_id>/status` and fetches `/result` when complete

Core components:
- **Scenario UI** (web) for request submission and report viewing
- **API server (Flask)** for async job execution and status/result APIs
- **Agent orchestrator (CrewAI)** running Research → Quant → Audit → Report
- **Tools**: SerpAPI news search, Twelve Data OHLCV, Alpha Vantage fundamentals, Safe Python Exec
- **LLM provider**: OpenAI for reasoning and embeddings in tools

Design principles:
- Strict handoff between stages to preserve context and quality
- Deterministic computations instead of LLM-generated numbers
- Transparent limitations whenever data is missing or uncertain

![Architecture diagram](image.png)

## AMP / Choreo Deployment

AMP repo: <https://github.com/wso2/ai-agent-management-platform/tree/amp/v0>

### Prerequisites
- Kubernetes cluster (k3d or equivalent)
- AMP installed (see AMP quick start guide)
- Docker registry accessible to the cluster
- API keys for OpenAI, SerpAPI, Twelve Data, Alpha Vantage (the service will start without them, but related features will fail or degrade at runtime)

### Create Component Definition
Create `.choreo/component.yaml` in your project root:

```yaml
schemaVersion: "1.0"
id: finance-insight
name: Finance Insight Service
type: service
description: AI-powered financial research assistant
runtime: python
buildType: dockerfile
image: Dockerfile
ports:
  - port: 8000
    type: http
env:
  - name: OPENAI_API_KEY
    valueFrom: SECRET
  - name: SERPAPI_API_KEY
    valueFrom: SECRET
  - name: TWELVE_DATA_API_KEY
    valueFrom: SECRET
  - name: ALPHAVANTAGE_API_KEY
    valueFrom: SECRET
```

### Build and Push Image
```bash
docker build -t finance-insight-service:latest .
docker tag finance-insight-service:latest \
  localhost:10082/default-finance-insight-image:v1
docker push localhost:10082/default-finance-insight-image:v1
```

### Deploy via AMP Console
1. Open AMP Console at `http://default.localhost:9080`
2. Create Agent → Service → Python → Port 8000
3. Add environment variables listed above
4. Deploy and verify health at `/finance-insight/health`

## Configuration
Copy `.env.example` to `.env` and set these keys:
- `OPENAI_API_KEY`
- `SERPAPI_API_KEY`
- `TWELVE_DATA_API_KEY`
- `ALPHAVANTAGE_API_KEY`
- `CORS_ALLOWED_ORIGINS` (optional, comma-separated origins for allowed frontend URLs, e.g. `http://localhost:3000,http://localhost:3001`)

See key setup details in [API_KEYS.md](API_KEYS.md).

## Contributing
Open an issue or submit a pull request with clear context and test notes.
