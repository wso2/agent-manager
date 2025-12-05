# WSO2 Agent Management Platform (AMP) Instrumentation

Automatic OpenTelemetry instrumentation for Python agents using the Traceloop SDK, with trace visibility in the WSO2 Agent Management Platform.

## Overview

`amp-instrumentation` enables zero-code instrumentation for Python agents, automatically capturing traces for LLM calls, API requests, and other operations. It seamlessly wraps your agent’s execution with OpenTelemetry tracing powered by the Traceloop SDK.

## Features

- **Zero Code Changes**: Instrument existing applications without modifying code
- **Automatic Tracing**: Traces LLM calls, HTTP requests, database queries, and more
- **OpenTelemetry Compatible**: Uses industry-standard OpenTelemetry protocol
- **Flexible Configuration**: Configure via environment variables
- **Framework Agnostic**: Works with any Python application built using a wide range of agent frameworks supported by the TraceLoop SDK

## Installation

Install from Test PyPI (dependencies will be fetched from main PyPI):

```bash
pip install amp-instrumentation
```

## Quick Start

### 1. Set Required Environment Variables

```bash
export AMP_AGENT_NAME="my-agent" # Name assigned during agent registration
export AMP_OTEL_ENDPOINT="https://amp-otel-endpoint.com" # AMP OTEL endpoint
export AMP_AGENT_API_KEY="your-agent-api-key" # Agent-specific key generated after registration
```

### 2. Run Your Application

Use the `amp-instrument` command to wrap your application:

```bash
# Run a Python script
amp-instrument python my_script.py

# Run with uvicorn
amp-instrument uvicorn app:main --reload

# Run with any package manager
amp-instrument poetry run python script.py
amp-instrument uv run python script.py
```

That's it! Your application is now instrumented and sending traces to the configured endpoint.
