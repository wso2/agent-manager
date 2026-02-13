# AMP Evaluation Job

Docker image for running AMP evaluation monitor jobs in Argo Workflows.

## Overview

This directory contains the evaluation job that uses the `amp-evaluation` SDK to run monitor evaluations against AI agent traces.

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
