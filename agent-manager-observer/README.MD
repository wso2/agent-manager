# Agent Manager Observer

## Overview

Agent Manager Observer is a lightweight backend that provides an HTTP API for querying traces collected in the observability plane. It queries the OpenSearch span index the OTel collector writes to, enriches and aggregates the tracing data produced by agents, and exposes simple endpoints for the console. Traces are scoped by the `agent-manager/organization`, `agent-manager/project` and `agent-manager/agent` resource attributes (names) the collector stamps on every span. The `environment` query parameter is accepted for compatibility and ignored. Runtime logs, build logs and metrics (`/api/v1/logs`, `/api/v1/build-logs`, `/api/v1/metrics`) return `501 Not Implemented`.

## Features

- Query traces and span documents stored in OpenSearch
- Support time-range filtering and pagination
- Provide a health endpoint for readiness/liveness checks
- Serve as the backend for the console traces UI

## How it works

1. The console sends a tracing query to the HTTP API exposed by this service.
2. The service validates and translates the request into an OpenSearch query.
3. The OpenSearch client executes the query against the configured cluster/indices.
4. Results are formatted and returned to the caller (with paging and basic aggregation info where applicable).

Architecture:

HTTP API --> Request Handler --> Service/Query Layer --> OpenSearch Client --> OpenSearch Cluster

## Configuration

This service is configured via environment variables. The following are supported and commonly used:

```env
# Server Configuration
AM_OBSERVER_PORT=9098

# OpenSearch Configuration
OPENSEARCH_ADDRESS=http://localhost:9200   # required
OPENSEARCH_USERNAME=admin
OPENSEARCH_PASSWORD=admin
OPENSEARCH_TRACE_INDEX=otel-traces-*        # default
OPENSEARCH_TLS_INSECURE_SKIP_VERIFY=false   # true only for a self-signed OpenSearch
```

# Set the environment Variables

## Build and run — local (Go)

Prerequisites: Go 1.20+ and a running OpenSearch instance reachable by the service.

Build the binary:

```bash
go build -o agent-manager-observer ./...
```
Set the enviornment variables:
```bash
export OPENSEARCH_ADDRESS=https://localhost:9200 OPENSEARCH_USERNAME=admin OPENSEARCH_PASSWORD=ThisIsTheOpenSearchPassword1 AM_OBSERVER_PORT=9098
```

Run with environment variables set (example):

```bash
OPENSEARCH_ADDRESS=http://localhost:9200 OPENSEARCH_USERNAME=admin OPENSEARCH_PASSWORD=ThisIsTheOpenSearchPassword1  AM_OBSERVER_PORT=9098 \
./agent-manager-observer
```

Or run directly:

```bash
go run . # or `go run .` from the service root, depending on project layout
```

## Docker

Build the image:

```bash
docker build -t agent-manager-observer .
```

Run the container (example):

```bash
docker run --rm -p 9098:9098 \
	-e OPENSEARCH_ADDRESS=http://host.docker.internal:9200 \
	-e OPENSEARCH_USERNAME=admin \
	-e OPENSEARCH_PASSWORD=ThisIsTheOpenSearchPassword1 \
    -e AM_OBSERVER_PORT=9098 \
	agent-manager-observer
```

## API — Query Parameter Examples

All APIs use standard GET requests with query parameters

### 1. List traces - `GET /api/v1/traces`

Retrieves a list of trace overviews for a specific service with optional time filtering and pagination.

**Query Parameters:**

- `serviceName` (required) - Name of the service to query traces for
- `startTime` (optional) - Start time in RFC3339 format (e.g., `2025-11-03T00:00:00Z`)
- `endTime` (optional) - End time in RFC3339 format (e.g., `2025-11-08T23:59:59Z`)
- `limit` (optional) - Maximum number of traces to return (default: 10)
- `offset` (optional) - Number of traces to skip for pagination (default: 0)
- `sortOrder` (optional) - Sort order: `asc` or `desc` (default: `desc` - newest first)

**Example request:**

```bash
curl --location 'http://localhost:9098/api/v1/traces?serviceName=sample-app&startTime=2025-11-03T00:00:00Z&endTime=2025-11-08T23:59:59Z&limit=10&offset=0'
```

**Response (200):**

```json
{
  "traces": [
    {
      "traceId": "5974d036b3d7709f2fc9f2b48461c176",
      "rootSpanId": "58f16238f09ae1b2",
      "rootSpanName": "LangGraph.workflow",
      "startTime": "2025-11-07T06:23:24.035086494Z",
      "endTime": "2025-11-07T06:23:27.545584559Z",
      "spanCount": 8
    }
  ],
  "totalCount": 1
}
```

### 2. Get trace spans - `GET /api/v1/trace`

Retrieves all spans for a specific trace ID and service.

**Query Parameters:**

- `traceId` (required) - The trace ID to retrieve spans for
- `serviceName` (required) - Name of the service
- `sortOrder` (optional) - Sort order for spans: `asc` or `desc` (default: `asc` - chronological)
- `limit` (optional) - Maximum number of spans to return (default: 100)

**Example request:**

```bash
curl --location 'http://localhost:9098/api/v1/trace?traceId=21a29d5d24837ca724b8751494e70a95&serviceName=langchain-docker-app&sortOrder=asc&limit=100'
```

**Response (200):**

```json
{
  "spans": [
    {
      "traceId": "21a29d5d24837ca724b8751494e70a95",
      "spanId": "e2c22d3d4b7736bd",
      "name": "LangGraph.workflow",
      "service": "langchain-docker-app",
      "startTime": "2025-11-03T11:42:18.329535246Z",
      "endTime": "2025-11-03T11:42:20.55201954Z",
      "duration": 2222484,
      "kind": "SPAN_KIND_INTERNAL",
      "status": "0",
      "attributes": {
        "traceloop.entity.input": "{...}",
        "traceloop.entity.output": "{...}",
        "traceloop.workflow.name": "LangGraph"
      }
    },
    {
      "traceId": "21a29d5d24837ca724b8751494e70a95",
      "spanId": "c189ec26ae2a0bb5",
      "parentSpanId": "e2c22d3d4b7736bd",
      "name": "agent.task",
      "service": "langchain-docker-app",
      "startTime": "2025-11-03T11:42:18.331008193Z",
      "endTime": "2025-11-03T11:42:20.55134794Z",
      "duration": 2220339,
      "kind": "SPAN_KIND_INTERNAL",
      "status": "0",
      "attributes": {
        "traceloop.entity.name": "agent",
        "traceloop.workflow.name": "LangGraph"
      }
    }
  ],
  "totalCount": 2
}
```

### 3. MCP server - `POST /mcp` (also `/mcp/`)

`am-obs-mcp` is a streamable-HTTP MCP server mounted on the root mux (not under `/api/v1/`), behind the same `JWTAuth` middleware as the REST routes. It exposes four trace tools that call the controllers directly (no HTTP hop): `list_traces`, `get_traces`, `get_trace_details`, `get_span_details`. Every tool takes an explicit, required `organization` parameter (no claims parsing), except `get_span_details`, which only needs a `trace_id`/`span_id`. See `mcp/setup.go` and `mcp/tools/` for the implementation.

### 4. Health check - `GET /health`

```bash
curl http://localhost:9098/health
```

**Response (200):**

```json
{
  "status": "healthy",
  "timestamp": "2025-11-09T12:34:56Z"
}
```

### Error responses

All endpoints return appropriate HTTP status codes:

- `200 OK` - Success
- `400 Bad Request` - Invalid parameters (missing required fields, invalid format)
- `500 Internal Server Error` - Server/OpenSearch errors
- `501 Not Implemented` - Runtime logs, build logs and metrics

# Mock Trace Generation (OTLP HTTP)

Use `make mock-traces` from `agent-manager-observer`.

## Prerequisites

Install:

```bash
go install github.com/open-telemetry/opentelemetry-collector-contrib/cmd/telemetrygen@latest
```

Ensure Go bin is in `PATH` (for example `$(go env GOPATH)/bin`).

## Make Targets

From `agent-manager-observer`:

```bash
export AMP_AGENT_API_KEY="<your-api-key>"
make mock-traces
```

Set values directly in `agent-manager-observer/Makefile` under:
- `OTLP_ENDPOINT`
- `OTLP_HTTP_PATH`
- `TRACE_SERVICE_NAME`
- `TRACE_COUNT`
- `SPANS_PER_TRACE`
- `TRACE_WORKERS`
- `TRACE_RATE`
- `TRACE_DURATION`
- `OTLP_INSECURE`

`AMP_AGENT_API_KEY` is intentionally not stored in `Makefile`; provide it from your environment.

`SPANS_PER_TRACE` includes the root span (`child_spans = SPANS_PER_TRACE - 1`).
