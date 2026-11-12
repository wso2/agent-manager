# WSO2 Agent Manager

[![Go Report Card](https://goreportcard.com/badge/github.com/wso2/agent-manager/agent-manager-service)](https://goreportcard.com/report/github.com/wso2/agent-manager/agent-manager-service)
[![Platform Release](https://img.shields.io/github/v/release/wso2/agent-manager?filter=amp/*&label=platform&color=orange)](https://github.com/wso2/agent-manager/releases?q=amp)
[![Python Instrumentation](https://img.shields.io/github/v/release/wso2/agent-manager?filter=amp-instrumentation/*&label=python-instrumentation&color=blue)](https://github.com/wso2/agent-manager/releases?q=amp-instrumentation)

An open control plane for enterprises to register, govern and observe AI agents.

## Overview

WSO2 Agent Manager provides a platform for enterprise AI agent management. Organizations register the AI agents they run, govern how those agents reach models, tools and other agents, and trace what they do.

This branch is **AMP Lite**, a development preview of Agent Manager without OpenChoreo. Lite connects external agents only: agents run wherever you host them and are registered in AMP, which issues their identities, fronts their traffic through the AI Gateway and collects their traces. Each organization has one internal `default` environment that the console and API never show. Hosted agents (builds and deployments) and evaluations are deferred; their API routes answer `501 Not Implemented`. Lite is not production-ready.

## Architecture

![AMP Lite architecture](docs/amp-lite/architecture.png)

- [Database schema](docs/amp-lite/database-schema.md) and [data model diagram](docs/amp-lite/data-model.png)
- Editable sources (draw.io, HTML and generators): [docs/amp-lite](docs/amp-lite/README.md)

## Key Features

- **External Agents** - Register agents hosted anywhere and manage their configuration from one control plane
- **Agent Identity** - Give every agent its own identity and OAuth client in ThunderID
- **AI Gateway** - Route agent traffic and LLM, MCP and A2A calls through a gateway with API keys, OAuth, guardrails and rate limits
- **Governance** - Enforce policies and access controls with role-based permissions and an audit log
- **Tracing** - Capture OpenTelemetry traces per organization, project and agent, and browse them in the console
- **Auto-Instrumentation** - OpenTelemetry-based instrumentation for AI frameworks with zero code changes

## Components

| Component | Description |
|-----------|-------------|
| **amp-console** ([console](console)) | Web-based management console for the platform |
| **amp-api** ([agent-manager-service](agent-manager-service)) | Backend API and MCP server powering the control plane |
| **amp-observer** ([agent-manager-observer](agent-manager-observer)) | API for querying trace data in OpenSearch |
| **amctl** ([cli](cli)) | Command-line client for the Agent Manager API |
| **amp-instrumentation** ([libs/amp-instrumentation](libs/amp-instrumentation)) | Python auto-instrumentation package for AI frameworks |
| **amp-python-instrumentation-provider** ([python-instrumentation-provider](python-instrumentation-provider)) | Kubernetes init container for automatic Python instrumentation of hosted agents (not used in Lite yet) |
| **amp-evaluation** ([libs/amp-evaluation](libs/amp-evaluation)) | Python library for scoring agent traces (evaluations are deferred in Lite) |

## Helm Charts

Charts are in [deployments/helm-charts](deployments/helm-charts):

| Chart | Description |
|-------|-------------|
| `wso2-agent-manager` | Main platform deployment: console and Agent Manager API |
| `wso2-amp-thunder-extension` | ThunderID identity provider for people, services and agents |
| `wso2-amp-api-platform-gateway-extension` | AI Gateway, registered with the platform |
| `wso2-amp-observability-extension` | Agent Manager Observer for querying traces |

## Getting Started

AMP Lite runs from source in development:

```bash
make setup
```

This installs the dev tools, starts a Colima VM and a local k3d cluster, installs the prerequisites, builds and starts the platform, and opens the console at http://localhost:3000 (API at http://localhost:9000). Run `make help` for the individual steps.

For the released Agent Manager, which runs on OpenChoreo, see the [Quick Start Guide](https://wso2.com/agent-platform/docs/latest/get-started/quick-start/).

## Contributing

We welcome contributions from the community! Here's how you can help:

1. **Report Issues** - Found a bug or have a feature request? Open an issue on GitHub
2. **Submit Pull Requests** - Fork the repository, make your changes, and submit a PR
3. **Improve Documentation** - Help us improve docs, tutorials, and examples

Please ensure your contributions adhere to our coding standards and include appropriate tests.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/wso2/agent-manager/issues)
- **Community**: [WSO2 Community](https://wso2.com/community/)

---
(c) Copyright 2012 - 2026 [WSO2 LLC](https://wso2.com/).
