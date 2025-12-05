# WSO2 Agent Instrumentation

Automatic OpenTelemetry instrumentation for Python agents using the Traceloop SDK, with trace visibility in the WSO2 Agent Management Platform.

## Overview

`wso2-agent-instrumentation` enables zero-code instrumentation for Python agents, automatically capturing traces for LLM calls, API requests, and other operations. It seamlessly wraps your agent’s execution with OpenTelemetry tracing powered by the Traceloop SDK.

## Features

- **Zero Code Changes**: Instrument existing applications without modifying code
- **Automatic Tracing**: Traces LLM calls, HTTP requests, database queries, and more
- **OpenTelemetry Compatible**: Uses industry-standard OpenTelemetry protocol
- **Flexible Configuration**: Configure via environment variables
- **Framework Agnostic**: Works with any Python application or framework that traceloop SDK supports

## Installation

Install from Test PyPI (dependencies will be fetched from main PyPI):

```bash
pip install amp-instrumentation
```

## Quick Start

### 1. Set Required Environment Variables

```bash
export AMP_APP_NAME="my-application"
export AMP_OTEL_EXPORTER_OTLP_ENDPOINT="https://your-otel-endpoint.com"
export AMP_API_KEY="your-api-key"
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

That's it! Your application is now instrumented and sending traces to your configured endpoint.
