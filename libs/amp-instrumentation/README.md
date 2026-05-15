# WSO2 Agent Manager Instrumentation

Zero-code OpenTelemetry instrumentation for Python agents using the Traceloop SDK, with trace visibility in the WSO2 Agent Manager.

## Overview

`amp-instrumentation` enables zero-code instrumentation for Python agents, automatically capturing traces for LLM calls, MCP requests, and other operations. It seamlessly wraps your agent’s execution with OpenTelemetry tracing powered by the Traceloop SDK.

For agents on a custom or non-frontier framework, or anywhere you want full control over the spans you emit, it also ships `init_otel()`, a one-line OpenTelemetry exporter setup so you can instrument the agent yourself against AMP's instrumentation contract. See [Manual instrumentation](#manual-instrumentation).

## Features

- **Zero Code Changes**: Instrument existing applications without modifying code
- **Automatic Tracing**: Traces LLM calls, MCP requests, database queries, and more
- **OpenTelemetry Compatible**: Uses industry-standard OpenTelemetry protocol
- **Flexible Configuration**: Configure via environment variables
- **Framework Agnostic**: Works with any Python application built using a wide range of agent frameworks supported by the TraceLoop SDK
- **Manual path**: `init_otel()` for agents that emit their own OpenTelemetry GenAI spans

## Installation

```bash
pip install amp-instrumentation
```

Each release of `amp-instrumentation` pins a specific Traceloop SDK version, so a given `amp-instrumentation` version always installs a fully determined SDK. To use a different Traceloop SDK version, install a different `amp-instrumentation` version.

## Quick Start

### 1. Register Your Agent

First, register your agent at the [WSO2 Agent Manager](https://github.com/wso2/agent-manager) to obtain your agent API key and configuration details.

### 2. Set Required Environment Variables

```bash
export AMP_OTEL_ENDPOINT="https://amp-otel-endpoint.com" # AMP OTEL endpoint
export AMP_AGENT_API_KEY="your-agent-api-key" # Agent-specific key generated after registration
```

Optional: prompt and completion content is captured by default. To suppress it, set `TRACELOOP_TRACE_CONTENT=false`.

### 3. Run Your Application

Use the `amp-instrument` command to wrap your application run command:

```bash
# Run a Python script
amp-instrument python my_script.py

# Run with uvicorn
amp-instrument uvicorn app:main --reload

# Run with any package manager
amp-instrument poetry run python script.py
amp-instrument uv run python script.py
```

That's it! Your application is now instrumented and sending traces to the WSO2 Agent Manager.

## Manual instrumentation

If your agent uses a framework the Traceloop SDK doesn't cover — or you want to emit your own spans — instrument it yourself and send the spans to AMP. `init_otel()` configures the OpenTelemetry exporter (the same `AMP_OTEL_ENDPOINT` / `AMP_AGENT_API_KEY` environment variables as above); it does no instrumentation itself, so you control the spans:

```python
import json
from opentelemetry import trace
from amp_instrumentation import init_otel

init_otel()  # reads AMP_OTEL_ENDPOINT and AMP_AGENT_API_KEY from the environment
tracer = trace.get_tracer("my-agent")

with tracer.start_as_current_span("chat") as span:
    span.set_attribute("gen_ai.operation.name", "chat")
    span.set_attribute("gen_ai.system", "openai")
    span.set_attribute("gen_ai.request.model", "gpt-4o-mini")
    span.set_attribute("gen_ai.input.messages", json.dumps(input_messages))
    response = call_model(...)
    span.set_attribute("gen_ai.response.model", response.model)
    span.set_attribute("gen_ai.output.messages", json.dumps(response.messages))
    span.set_attribute("gen_ai.usage.input_tokens", response.usage.input_tokens)
    span.set_attribute("gen_ai.usage.output_tokens", response.usage.output_tokens)
```

Spans that follow AMP's instrumentation contract — the OpenTelemetry GenAI semantic conventions — render with the full trace view in the Agent Manager and run through evaluators; spans that don't still appear, just without the rich view. The contract and the full supported-attribute reference are published in the WSO2 Agent Manager documentation: see [Manual instrumentation](https://wso2.github.io/agent-manager/docs/components/amp-instrumentation/#manual-instrumentation) on the WSO2 Agent Manager Instrumentation page.

`init_otel()` is idempotent and is roughly ten lines of vanilla OpenTelemetry SDK setup (`TracerProvider` + `BatchSpanProcessor` + an OTLP/HTTP exporter to `<AMP_OTEL_ENDPOINT>/v1/traces` with the `x-amp-api-key` header) — use it so you don't have to write that yourself.
