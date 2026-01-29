---
sidebar_position: 2
---

# Single Cluster Installation

Install the Agent Management Platform on an existing OpenChoreo cluster.

## Prerequisites

### Required Tools

Before installation, ensure you have the following tools installed:

- **kubectl** - Kubernetes command-line tool
- **helm** (v3.x) - Package manager for Kubernetes
- **curl** - Command-line tool for transferring data
- **Docker** - Container runtime (if using k3d for local development)

Verify tools are installed:

```bash
kubectl version --client
helm version
curl --version
docker --version  # If using k3d
```

### OpenChoreo Cluster Requirements

The Agent Management Platform requires an **OpenChoreo cluster (v0.9.0)** with the following components installed:

- **OpenChoreo Control Plane** - Core orchestration and management
- **OpenChoreo Data Plane** - Runtime environment for agents
- **OpenChoreo Build Plane** - Build and CI/CD capabilities
- **OpenChoreo Observability Plane** - Observability and monitoring stack

### Installing OpenChoreo with Custom Values

If you need to install OpenChoreo components, this repository provides custom values files optimized for single-cluster setups:

- **Control Plane**: `deployments/single-cluster/values-cp.yaml`
- **Build Plane**: `deployments/single-cluster/values-bp.yaml`
- **Data Plane**: `deployments/single-cluster/values-dp.yaml`
- **Observability Plane**: `deployments/single-cluster/values-op.yaml`

These values files configure:

- Development mode settings for local development
- Single-cluster installation mode (non-HA)
- Standalone OpenSearch (instead of operator-managed cluster)
- Traefik ingress configuration for k3d
- Cluster gateway configuration
- Enable API Platform

#### Install OpenChoreo Control Plane

```bash
# Install Control Plane
helm install openchoreo-control-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-control-plane \
  --version 0.9.0 \
  --namespace openchoreo-control-plane \
  --create-namespace \
  --values https://raw.githubusercontent.com/wso2/ai-agent-management-platform/v0.0.0-dev/deployments/single-cluster/values-cp.yaml
```

#### Install OpenChoreo Build Plane

```bash
# Install Build Plane
helm install openchoreo-build-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-build-plane \
  --version 0.9.0 \
  --namespace openchoreo-build-plane \
  --create-namespace \
  --values https://raw.githubusercontent.com/wso2/ai-agent-management-platform/v0.0.0-dev/deployments/single-cluster/values-bp.yaml
```

#### Install OpenChoreo Data Plane

```bash
# Install Data Plane
helm install openchoreo-data-plane \
  oci://ghcr.io/openchoreo/helm-charts/openchoreo-data-plane \
  --version 0.9.0 \
  --namespace openchoreo-data-plane \
  --create-namespace \
  --values https://raw.githubusercontent.com/wso2/ai-agent-management-platform/v0.0.0-dev/deployments/single-cluster/values-dp.yaml
```

#### Install OpenChoreo Observability Plane

Create namespace `openchoreo-observability-plane`:

```bash
kubectl create namespace openchoreo-observability-plane
```

:::tip
For complete OpenChoreo installation instructions, refer to the [OpenChoreo documentation](https://openchoreo.dev/docs/getting-started/quick-start-guide).
:::

## Install Agent Management Platform

Once OpenChoreo is installed and running, you can install the Agent Management Platform.

### Step 1: Add Helm Repository

```bash
helm repo add wso2 https://wso2.github.io/ai-agent-management-platform/charts
helm repo update
```

### Step 2: Install the Platform

```bash
helm install wso2-amp wso2/wso2-ai-agent-management-platform \
  --version v0.0.0-dev \
  --namespace amp-system \
  --create-namespace
```

### Step 3: Verify Installation

Check that all pods are running:

```bash
kubectl get pods -n amp-system
```

You should see pods for:
- PostgreSQL database
- Agent Manager API
- Console UI
- Traces Observer

## Access the Platform

### Console UI

Access the console at [http://localhost:3000](http://localhost:3000)

### API Endpoints

- **Agent Manager API**: `http://localhost:8080`
- **Traces Observer API**: `http://localhost:8081`

## Next Steps

Your Agent Management Platform is now ready!

## Troubleshooting

### Pods not starting

Check pod logs:
```bash
kubectl logs -n amp-system <pod-name>
```

### Database connection issues

Verify PostgreSQL is running:
```bash
kubectl get pods -n amp-system -l app=postgresql
```

For more help, visit our [GitHub Discussions](https://github.com/wso2/ai-agent-management-platform/discussions).
