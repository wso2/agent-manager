# Mock Trace Generation (OTLP HTTP)

Use `make mock-traces` from `traces-observer-service`.

## Prerequisites

Install:

```bash
go install github.com/open-telemetry/opentelemetry-collector-contrib/cmd/telemetrygen@latest
```

Ensure Go bin is in `PATH` (for example `$(go env GOPATH)/bin`).

## Configuration

## Make Targets

From `traces-observer-service`:

```bash
export AMP_API_KEY="<your-api-key>"
make mock-traces
```

Set values directly in `traces-observer-service/Makefile` under:
- `OTLP_ENDPOINT`
- `OTLP_HTTP_PATH`
- `TRACE_SERVICE_NAME`
- `TRACE_COUNT`
- `SPANS_PER_TRACE`
- `TRACE_WORKERS`
- `TRACE_RATE`
- `TRACE_DURATION`
- `OTLP_INSECURE`

`AMP_API_KEY` is intentionally not stored in `Makefile`; provide it from your environment.

`SPANS_PER_TRACE` includes the root span (`child_spans = SPANS_PER_TRACE - 1`).
