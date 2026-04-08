# API Monitoring

Demonstrates the `TraceFetcher` API for fetching and evaluating production traces from a trace service. This is the recommended approach for continuous monitoring of live agent behavior.

## What it shows

- Using `TraceFetcher` to fetch traces from a real trace service API
- Health check before fetching
- CLI arguments for time range
- Environment variable configuration
- Parsing OTEL traces into evaluation-ready format with `parse_traces_for_evaluation()`
- Combining custom evaluators with built-in evaluators
- Running `Monitor` on fetched traces

## TraceFetcher API

```python
from amp_evaluation.trace import TraceFetcher, parse_traces_for_evaluation

fetcher = TraceFetcher(
    base_url="http://localhost:8001",
    namespace="default",
    project="my-project",
    component="my-agent",
    environment="production",
)

# Health check
if fetcher.health_check():
    # Fetch traces for a time range
    otel_traces = fetcher.fetch_traces(
        start_time="2025-01-01T00:00:00Z",
        end_time="2025-01-02T00:00:00Z",
    )

    # Parse into evaluation-ready Trace objects
    traces = parse_traces_for_evaluation(otel_traces)
```

The `TraceFetcher` calls the `/api/v1/traces/export` endpoint with parameters:
- `startTime` / `endTime`: ISO 8601 time range
- `namespace`: Kubernetes namespace / organisation name
- `project`: Project name
- `component`: Component (agent) name
- `environment`: Environment name

## Prerequisites

1. A running trace service that implements the traces export API
2. Configure environment variables (see `.env.example`):
   ```bash
   export TRACE_SERVICE_URL=http://localhost:8001
   export AMP_NAMESPACE=default
   export AMP_PROJECT=my-project
   export AMP_COMPONENT=my-agent
   export AMP_ENVIRONMENT=production
   ```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `TRACE_SERVICE_URL` | `http://localhost:8001` | Base URL of the trace service |
| `AMP_NAMESPACE` | `default` | Kubernetes namespace / organisation name |
| `AMP_PROJECT` | `my-project` | Project name |
| `AMP_COMPONENT` | `my-agent` | Component (agent) name |
| `AMP_ENVIRONMENT` | `production` | Environment name |

## How to run

```bash
pip install amp-evaluation

# Default: last 24 hours
python run.py

# Custom time range
python run.py --start 2025-01-01T00:00:00Z --end 2025-01-02T00:00:00Z
```

## CLI arguments

| Argument | Default | Description |
|---|---|---|
| `--start` | 24 hours ago | Start time in ISO 8601 format |
| `--end` | Now | End time in ISO 8601 format |

## Expected output

```text
Trace service: http://localhost:8001
Agent UID:     my-agent
Environment:   production

Fetching traces from ... to ...
Fetched and parsed N traces
Evaluators: ['response-quality', 'error-free', 'latency', 'hallucination']

Evaluation Run: run... (EvalMode.MONITOR)
  ...
Scores:
  response-quality:
    level: trace
    count: N
    mean: ...
  error-free:
    level: trace
    count: N
    mean: ...
  latency:
    level: trace
    count: N
    mean: ...
  hallucination:
    level: llm
    count: N
    skipped: ...
    mean: ...
```
