# AMP Evaluation Job

Docker image for running AMP evaluation monitor jobs in Argo Workflows.

## Overview

This directory contains the evaluation job that uses the `amp-evaluation` SDK to run monitor evaluations against AI agent traces.

## Anthropic temperature compatibility

Monitor jobs omit `temperature` from Anthropic requests made by managed LLM-judge
evaluators, including custom prompt-based judges. A warning is logged once per
affected evaluator when the job starts. Anthropic uses its default sampling behavior,
including for older models that otherwise support temperature.

The monitor console disables the temperature input when an Anthropic provider is
selected and explains why in a tooltip. Saved values and custom evaluator definitions are retained, so changing
to another provider restores the configured value. Backend validation permits a
missing required temperature only for Anthropic LLM judges; supplied values still
undergo normal schema validation.

Custom code evaluators, standalone library usage, and experiments keep their
existing behavior. Direct SDK calls inside custom Python code are not intercepted.

Release the backend and rebuilt evaluation image before the console update. The
image must include both the job and evaluation-library changes; no database migration
is required. Verify an actual Anthropic monitor produces a score before rollout.

## Structure

- `main.py` - Job entrypoint that uses the amp-evaluation SDK
- `Dockerfile` - Production build (installs amp-evaluation from PyPI)
- `Dockerfile.dev` - Development build (installs from local libs/amp-evaluation)
- `Makefile` - Build commands

## Usage

### Development Build (Local SDK)

```bash
# Build dev image using local libs/amp-evaluation
make docker-build-dev

# Build and load into k3d cluster
make docker-load-k3d
```

### Production Build (PyPI)

```bash
# Build production image with specific SDK version
make docker-build AMP_EVALUATION_VERSION=1.0.0

# Or with custom tag
make docker-build TAG=1.0.0 AMP_EVALUATION_VERSION=1.0.0
```

### From Root Makefile

```bash
# Build dev image and generate evaluator catalog
make setup-evaluators

# Build dev image, load to k3d, and generate evaluator catalog
make setup-evaluators-k3d
```

## Image Details

- **Image Name**: `amp-evaluation-monitor:0.0.0-dev`
- **Base**: Python 3.11 Alpine
- **Entrypoint**: `python main.py`

## Workflow Integration

This image is used by the `amp-monitor-evaluation` ClusterWorkflowTemplate in the `wso2-amp-evaluation-extension` Helm chart.
