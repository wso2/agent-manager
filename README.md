# WSO2 Agent Manager

An open control plane designed for enterprises to deploy, manage, and govern AI agents at scale.

## Overview

WSO2 Agent Manager provides a comprehensive platform for enterprise AI agent management. It enables organizations to deploy AI agents (both internally hosted and externally deployed), monitor their behavior through full-stack observability, and enforce governance policies at scale.

Built on [OpenChoreo](https://github.com/openchoreo/openchoreo) for internal agent deployments, the platform leverages OpenTelemetry for extensible instrumentation across multiple AI frameworks.

## Key Features

- **Deploy at Scale** - Deploy and run AI agents on Kubernetes with production-ready configurations
- **Lifecycle Management** - Manage agent versions, configurations, and deployments from a unified control plane
- **Governance** - Enforce policies, manage access controls, and ensure compliance across all agents
- **Full Observability** - Capture traces, metrics, and logs for complete visibility into agent behavior
- **Auto-Instrumentation** - OpenTelemetry-based instrumentation for AI frameworks with zero code changes
- **External Agent Support** - Monitor and govern externally deployed agents alongside internal ones

## Components

| Component | Description |
|-----------|-------------|
| **amp-instrumentation** | Python auto-instrumentation package for AI frameworks | 
| **amp-console** | Web-based management console for the platform |
| **amp-api** | Backend API powering the control plane | 
| **amp-trace-observer** | API for querying and analyzing trace data | 
| **amp-python-instrumentation-provider** | Kubernetes init container for automatic Python instrumentation |

## Helm Charts

Deploy WSO2 Agent Manager on Kubernetes using our Helm charts:

| Chart | Description |
|-------|-------------|
| `wso2-agent-management-platform` | Main platform deployment |
| `wso2-amp-build-extension` | Build extension for OpenChoreo |
| `wso2-amp-observability-extension` | Observability stack extension for OpenChoreo |

## Getting Started

For installation instructions and a step-by-step guide, see the [Quick Start Guide](docs/quick-start.md).

## Contributing

We welcome contributions from the community! Here's how you can help:

1. **Report Issues** - Found a bug or have a feature request? Open an issue on GitHub
2. **Submit Pull Requests** - Fork the repository, make your changes, and submit a PR
3. **Improve Documentation** - Help us improve docs, tutorials, and examples

Please ensure your contributions adhere to our coding standards and include appropriate tests.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Support

- **Issues**: [GitHub Issues](https://github.com/wso2/ai-agent-management-platform/issues)
- **Community**: [WSO2 Community](https://wso2.com/community/)

---
(c) Copyright 2012 - 2025 [WSO2 LLC](https://wso2.com/).
