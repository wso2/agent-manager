# API Monitoring

Demonstrates the `TraceFetcher` API for fetching and evaluating production traces from a trace service. This is the recommended approach for continuous monitoring of live agent behavior.

## What it shows

- Using `TraceFetcher` to fetch traces from a real trace service API
- Health check before fetching
- CLI arguments for time range and limit
- Environment variable configuration
- Parsing OTEL traces into evaluation-ready format with `parse_traces_for_evaluation()`
- Combining custom evaluators with built-in evaluators
- Running `Monitor` on fetched traces

## TraceFetcher API

```python
from amp_evaluation.trace import TraceFetcher, parse_traces_for_evaluation

fetcher = TraceFetcher(
    base_url="http://localhost:8001",
    agent_uid="my-agent",
    environment_uid="production",
)

# Health check
if fetcher.health_check():
    # Fetch traces for a time range
    otel_traces = fetcher.fetch_traces(
        start_time="2025-01-01T00:00:00Z",
        end_time="2025-01-02T00:00:00Z",
        limit=100,
    )

    # Parse into evaluation-ready Trace objects
    traces = parse_traces_for_evaluation(otel_traces)
```

The `TraceFetcher` calls the `/api/v1/traces/export` endpoint with parameters:
- `startTime` / `endTime`: ISO 8601 time range
- `componentUid`: Agent identifier
- `environmentUid`: Environment identifier
- `limit` / `offset`: Pagination

## Prerequisites

1. A running trace service that implements the traces export API
2. Configure environment variables (see `.env.example`):
   ```bash
   export TRACE_SERVICE_URL=http://localhost:8001
   export AGENT_UID=my-agent
   export ENVIRONMENT_UID=production
   ```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `TRACE_SERVICE_URL` | `http://localhost:8001` | Base URL of the trace service |
| `AGENT_UID` | `my-agent` | Agent unique identifier |
| `ENVIRONMENT_UID` | `production` | Environment unique identifier |

## How to run

```bash
pip install amp-evaluation

# Default: last 24 hours, up to 100 traces
python run.py

# Custom time range
python run.py --start 2025-01-01T00:00:00Z --end 2025-01-02T00:00:00Z

# Custom limit
python run.py --limit 50
```

## CLI arguments

| Argument | Default | Description |
|---|---|---|
| `--start` | 24 hours ago | Start time in ISO 8601 format |
| `--end` | Now | End time in ISO 8601 format |
| `--limit` | 100 | Maximum number of traces to fetch |

## Expected output

```
Trace service: http://localhost:8001
Agent UID:     my-agent
Environment:   production

Fetching traces from ... to ... (limit: 100)...
Fetched and parsed N traces
Evaluators: ['response-quality', 'error-free', 'latency', 'hallucination']

Evaluation Run: run... (EvalMode.MONITOR)
  ...
Scores:
  response-quality:
    mean: ...
    pass_rate: ...
    count: N
  error-free:
    mean: ...
    pass_rate: ...
    count: N
  latency:
    mean: ...
    pass_rate: ...
    count: N
  hallucination:
    mean: ...
    pass_rate: ...
    count: N

Pass rates:
  response-quality          pass_rate=...  mean=...
  error-free                pass_rate=...  mean=...
  latency                   pass_rate=...  mean=...
  hallucination             pass_rate=...  mean=...
```
